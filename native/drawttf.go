package main

import (
	"sort"
	"strings"
)

// fontStack 文本字体栈：主字体 + 回退字体，按 rune 逐个选（脚本回退）。
// 必要性：CJK 回退字体（如 Droid Sans Fallback）往往只有 CJK 字形、没有拉丁，
// 而拉丁字体没有 CJK 字形；只有逐 rune 选择才能让混排文本全部可读。
type fontStack []*ttfFont

// pick 返回含该 rune 字形的第一个字体；都缺失时用主字体（.notdef）。
func (fs fontStack) pick(ch rune) *ttfFont {
	for _, f := range fs {
		if f == nil {
			continue
		}
		if gid := f.glyphIndex(ch); gid != 0 && gid < uint32(f.numGlyphs) {
			return f
		}
	}
	if len(fs) > 0 {
		return fs[0]
	}
	return nil
}

// hasLatin 文本是否含拉丁/ASCII 可打印字符。
func hasLatin(s string) bool {
	for _, r := range s {
		if r >= 0x20 && r <= 0x24F {
			return true
		}
	}
	return false
}

// coverageRatio 文本字形覆盖率（去重抽样 0..1）。
func (f *ttfFont) coverageRatio(text string) float64 {
	if f == nil {
		return 0
	}
	var seen []rune
	total, hit := 0, 0
	for _, r := range text {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		dup := false
		for _, s := range seen {
			if s == r {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		seen = append(seen, r)
		total++
		if gid := f.glyphIndex(r); gid != 0 && gid < uint32(f.numGlyphs) {
			hit++
		}
		if total >= 64 {
			break
		}
	}
	if total == 0 {
		return 1
	}
	return float64(hit) / float64(total)
}

// pickFontStack 组装字体栈：名字链候选 +（含 CJK 时）系统 CJK 字体 +（含拉丁时）拉丁默认字体，
// 去重后按文本覆盖率稳定降序（同覆盖率保持链上优先级）。
func pickFontStack(text string, size int, fontChain string) fontStack {
	if strings.TrimSpace(fontChain) == "" {
		fontChain = defaultFontChain // 空链 → 默认 monospace（避免直接掉到 5x7 位图）
	}
	var cands []*ttfFont
	add := func(f *ttfFont) {
		if f == nil {
			return
		}
		for _, e := range cands {
			if e == f {
				return
			}
		}
		cands = append(cands, f)
	}
	for _, name := range strings.Split(fontChain, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if f, err := loadFont(name, size); err == nil {
			add(f)
		}
	}
	if hasCJK(text) {
		add(loadCJKFont(size))
	}
	if hasLatin(text) {
		for _, name := range []string{"monospace", "sans-serif", "serif"} {
			if f, err := loadFont(name, size); err == nil {
				add(f)
				break
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].coverageRatio(text) > cands[j].coverageRatio(text)
	})
	return fontStack(cands)
}

// pickFont 字体栈主字体（覆盖率最高者）；无可用字体返回 nil（走内置 5x7 位图）。
func pickFont(text string, size int, fontChain string) *ttfFont {
	fs := pickFontStack(text, size, fontChain)
	if len(fs) == 0 {
		return nil
	}
	return fs[0]
}

// drawStack 字体栈绘制（折行 + 逐 rune 选字体 + 字距；裁剪感知）。
func (fb *framebuffer) drawStack(fs fontStack, x, y int, text string, size int, c uint32, a0, spacing int) {
	runes := []rune(text)
	cx := x
	for i, ch := range runes {
		if ch == '\n' {
			cx = x
			y += size + 4
			continue
		}
		f := fs.pick(ch)
		if f == nil {
			cx += size / 2
		} else {
			w, h, adv, a := f.rasterGlyph(ch, size)
			if adv <= 0 && w == 0 {
				cx += size / 2
			} else {
				fb.blitAlpha(cx, y, w, h, a, c, a0)
				cx += adv
			}
		}
		if i < len(runes)-1 {
			cx += spacing
		}
	}
}

// drawTextChain 纯 Go 跨系统文本：字体回退链（自研 TTF 光栅）→ 内置 5x7 位图（链末端）。
func (fb *framebuffer) drawTextChain(x, y int, text string, size int, c uint32, fontChain string) {
	fs := pickFontStack(text, size, fontChain)
	if len(fs) == 0 {
		fb.drawText(x, y, text, scaleFor(size), c)
		return
	}
	fb.drawStack(fs, x, y, text, size, c, 255, 0)
}

// drawTTF 自研 TTF 光栅文本（rune 级；alpha 混合抗锯齿；受当前裁剪区约束）。
func (fb *framebuffer) drawTTF(f *ttfFont, x, y int, text string, size int, c uint32) {
	cx := x
	for _, ch := range text {
		if ch == '\n' {
			cx = x
			y += size + 4
			continue
		}
		w, h, adv, a := f.rasterGlyph(ch, size)
		if adv <= 0 && w == 0 {
			cx += size / 2
			continue
		}
		fb.blitAlpha(cx, y, w, h, a, c, 255)
		cx += adv
	}
}

// blendPixel 前景色按 alpha 混合（RGB）。
func blendPixel(dst *uint32, c uint32, a int) {
	d := *dst
	inv := 255 - a
	or := (int(c>>16&0xFF)*a + int(d>>16&0xFF)*inv) / 255
	og := (int(c>>8&0xFF)*a + int(d>>8&0xFF)*inv) / 255
	ob := (int(c&0xFF)*a + int(d&0xFF)*inv) / 255
	*dst = uint32(or)<<16 | uint32(og)<<8 | uint32(ob)
}
