package main

// cleg Style v2 文本度量与 cleg_text_ex 内核（复用既有字体回退链：drawttf.go/fontttf.go）。
//
//   - cleg_text_width / cleg_text_height：纯度量（不含变换），供 align/valign 与省略号截断；
//   - cleg_text_ex：多行 + align/valign + letter-spacing + line-height + decoration + ellipsis；
//   - advanceOf 是 rasterGlyph 推进量的等价快速路径（不分配位图），二者由单测锁定一致。
//
// 锚点语义（§9 无盒子高/宽参数，故按锚点解释）：
//   align  0=left(x 为左边界) 1=center(x 为水平中心) 2=right(x 为右边界)；
//   valign 0=top(y 为文本块顶) 1=middle(y 为文本块垂直中心) 2=bottom(y 为文本块底)；
//   align 的参照宽度 = max_w（>0）否则为最宽行宽。

import (
	"encoding/binary"
	"strings"
)

// blitAlpha 字形 alpha 位图混合写屏（裁剪感知；a0 为整体 alpha 0..255）。
func (fb *framebuffer) blitAlpha(x, y, w, h int, a []byte, c uint32, a0 int) {
	if w <= 0 || h <= 0 || len(a) < w*h || a0 <= 0 {
		return
	}
	c &= 0x00FFFFFF
	for yy := 0; yy < h; yy++ {
		rowY := y + yy
		if rowY < fb.clipY0 || rowY >= fb.clipY1 {
			continue
		}
		rowBase := rowY * fb.w
		for xx := 0; xx < w; xx++ {
			al := int(a[yy*w+xx])
			if al == 0 {
				continue
			}
			fx := x + xx
			if fx < fb.clipX0 || fx >= fb.clipX1 {
				continue
			}
			if a0 < 255 {
				al = al * a0 / 255
				if al == 0 {
					continue
				}
			}
			if al >= 255 {
				fb.buf[rowBase+fx] = c
				continue
			}
			blendPixel(&fb.buf[rowBase+fx], c, al)
		}
	}
}

// glyphRect 字形像素块写入（a>=255 直写；否则按 alpha 混合）。
func (fb *framebuffer) glyphRect(x, y, w, h int, c uint32, a int) {
	if a >= 255 {
		fb.fillRect(x, y, w, h, c)
		return
	}
	for yy := y; yy < y+h; yy++ {
		fb.blendSpan(yy, x, x+w, c, a)
	}
}

// advanceOf 单字符推进量（与 rasterGlyph 返回的 adv 完全一致，但不分配位图）。
// ok=false 表示 drawTTF 会走 "w==0&&adv<=0 → 前进 size/2" 分支。
func (f *ttfFont) advanceOf(ch rune, px int) (int, bool) {
	gid := f.glyphIndex(ch)
	if gid >= uint32(f.numGlyphs) {
		return 0, false
	}
	a, b := f.glyphOffsets(gid)
	if b <= a || b > len(f.glyf) {
		return 0, false
	}
	g := f.glyf[a:b]
	if len(g) < 10 {
		return 0, false
	}
	if f.unitsPerEm <= 0 {
		return 0, false
	}
	scale := float64(px) / float64(f.unitsPerEm)
	xMin := int(int16(binary.BigEndian.Uint16(g[2:4])))
	yMin := int(int16(binary.BigEndian.Uint16(g[4:6])))
	xMax := int(int16(binary.BigEndian.Uint16(g[6:8])))
	yMax := int(int16(binary.BigEndian.Uint16(g[8:10])))
	w := int((float64(xMax-xMin) * scale) + 0.5)
	h := int((float64(yMax-yMin) * scale) + 0.5)
	if w <= 0 || h <= 0 {
		return 0, false
	}
	adv := 0
	if int(gid)*4+2 <= len(f.hmtx) {
		adv = int(float64(binary.BigEndian.Uint16(f.hmtx[int(gid)*4:int(gid)*4+2])) * scale)
	}
	if adv <= 0 {
		adv = w + int(scale*120)
	}
	return adv, true
}

// measure 单行宽度（与 drawStack 的推进规则一致；spacing 只加在字符之间）。
// 空字体栈 → 内置 5x7 位图度量（6*scale/字符）。
func (fs fontStack) measure(s string, size, spacing int) int {
	if size <= 0 || s == "" {
		return 0
	}
	w := 0
	n := 0
	for _, ch := range s {
		f := fs.pick(ch)
		if f != nil {
			if adv, ok := f.advanceOf(ch, size); ok {
				w += adv
			} else {
				w += size / 2
			}
		} else {
			w += 6 * scaleFor(size)
		}
		n++
	}
	if n > 1 && spacing != 0 {
		w += spacing * (n - 1)
	}
	return w
}

// measureLine 单字体度量（兼容入口）。
func measureLine(f *ttfFont, s string, size, spacing int) int {
	return fontStack{f}.measure(s, size, spacing)
}

// lineHeightPx 默认行高（与 drawTTF 换行推进一致）：size + 4。
func lineHeightPx(size int) int {
	if size <= 0 {
		return 0
	}
	return size + 4
}

// textWidthCore cleg_text_width 内核：多行取最宽行（不含变换）。
func textWidthCore(text string, size int, font string) int {
	if size <= 0 || text == "" {
		return 0
	}
	fs := pickFontStack(text, size, font)
	maxW := 0
	for _, ln := range strings.Split(text, "\n") {
		if w := fs.measure(ln, size, 0); w > maxW {
			maxW = w
		}
	}
	return maxW
}

// ellipsisFor 省略号字形：字体栈无 U+2026 时退化为 "..."（5x7 位图路径只能用 ASCII）。
func ellipsisFor(fs fontStack, size int) string {
	if len(fs) == 0 {
		return "..."
	}
	if f := fs.pick('…'); f != nil {
		if adv, ok := f.advanceOf('…', size); ok && adv > 0 {
			return "…"
		}
	}
	return "..."
}

// fitLineStack ellipsis 截断：超出 max_w（>0）时收缩到最长可容纳前缀 + 省略号。
func fitLineStack(fs fontStack, s string, size, spacing, ellipsis, maxW int) (string, int) {
	w := fs.measure(s, size, spacing)
	if ellipsis == 0 || maxW <= 0 || w <= maxW {
		return s, w
	}
	ell := ellipsisFor(fs, size)
	ew := fs.measure(ell, size, 0)
	runes := []rune(s)
	for n := len(runes) - 1; n > 0; n-- {
		cand := string(runes[:n])
		cw := fs.measure(cand, size, spacing)
		if n < len(runes) {
			cw += spacing
		}
		cw += ew
		if cw <= maxW {
			return cand + ell, cw
		}
	}
	return ell, ew
}

// fitLine 单字体截断（兼容入口）。
func fitLine(f *ttfFont, s string, size, spacing, ellipsis, maxW int) (string, int) {
	return fitLineStack(fontStack{f}, s, size, spacing, ellipsis, maxW)
}

// drawBitmapSpaced 内置 5x7 位图绘制（letter-spacing 版；裁剪感知）。
func (fb *framebuffer) drawBitmapSpaced(x, y int, text string, scale int, c uint32, a0, spacing int) {
	cx := x
	for i := 0; i < len(text); i++ {
		ch := text[i]
		fb.drawGlyphA(cx, y, ch, scale, c, a0)
		cx += 6 * scale
		if i < len(text)-1 {
			cx += spacing
		}
	}
}

// drawGlyphA 5x7 字形（带整体 alpha）。
func (fb *framebuffer) drawGlyphA(x, y int, glyph uint8, scale int, c uint32, a0 int) {
	if glyph < 32 || glyph > 126 {
		glyph = '?'
	}
	rows := font5x7[glyph-32]
	if rows[0] == 0 && rows[1] == 0 {
		return
	}
	for r := 0; r < 7; r++ {
		bits := rows[r]
		for ci := 0; ci < 5; ci++ {
			if bits&(1<<uint(ci)) == 0 {
				continue
			}
			fb.glyphRect(x+ci*scale, y+r*scale, scale, scale, c, a0)
		}
	}
}

// decorateLine 文本装饰线：1=underline 2=line-through。
func (fb *framebuffer) decorateLine(x, y, w, size int, c uint32, a, decoration int) {
	if decoration <= 0 || w <= 0 || size <= 0 {
		return
	}
	th := size / 14
	if th < 1 {
		th = 1
	}
	switch decoration {
	case 1:
		fb.glyphRect(x, y+size*4/5+2, w, th, c, a)
	case 2:
		fb.glyphRect(x, y+size*2/5, w, th, c, a)
	}
}

// textExCore cleg_text_ex 内核。
func (fb *framebuffer) textExCore(text string, x, y, size int, font string, color uint32, align, valign, letterSpacing, lineHeight, decoration, ellipsis, maxW int) {
	if text == "" || size <= 0 {
		return
	}
	x, y = fb.xfPoint(x, y)
	size = fb.xfLenY(size)
	if size <= 0 {
		return
	}
	letterSpacing = fb.xfLenX(letterSpacing)
	if maxW > 0 {
		maxW = fb.xfLenX(maxW)
	}
	if lineHeight > 0 {
		lineHeight = fb.xfLenY(lineHeight)
	}
	fs := pickFontStack(text, size, font)
	lh := lineHeight
	if lh <= 0 {
		lh = lineHeightPx(size)
	}
	lines := strings.Split(text, "\n")
	type lineBox struct {
		s string
		w int
	}
	boxes := make([]lineBox, len(lines))
	blockW := 0
	for i, ln := range lines {
		s, w := fitLineStack(fs, ln, size, letterSpacing, ellipsis, maxW)
		boxes[i] = lineBox{s, w}
		if w > blockW {
			blockW = w
		}
	}
	boxW := maxW
	if boxW <= 0 {
		boxW = blockW
	}
	blockH := (len(lines)-1)*lh + lineHeightPx(size)
	yoff := 0
	switch valign {
	case 1:
		yoff = -blockH / 2
	case 2:
		yoff = -blockH
	}
	c := color & 0x00FFFFFF
	a := alphaChannelOr(color, 255)
	for i, b := range boxes {
		xoff := 0
		switch align {
		case 1:
			xoff = (boxW - b.w) / 2
		case 2:
			xoff = boxW - b.w
		}
		px := x + xoff
		py := y + yoff + i*lh
		if len(fs) > 0 {
			fb.drawStack(fs, px, py, b.s, size, c, a, letterSpacing)
		} else {
			fb.drawBitmapSpaced(px, py, b.s, scaleFor(size), c, a, letterSpacing)
		}
		fb.decorateLine(px, py, b.w, size, c, a, decoration)
	}
}
