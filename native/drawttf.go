package main

import "strings"

// drawTextChain 纯 Go 跨系统文本：字体回退链（自研 TTF 光栅）→ 内置 5x7 位图（链末端）。
func (fb *framebuffer) drawTextChain(x, y int, text string, size int, c uint32, fontChain string) {
	for _, name := range strings.Split(fontChain, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if f, err := loadFont(name, size); err == nil {
			fb.drawTTF(f, x, y, text, size, c)
			return
		}
	}
	if hasCJK(text) {
		if p := cjkFontPath(); p != "" {
			if f, err := loadFontFile(p, size); err == nil {
				fb.drawTTF(f, x, y, text, size, c)
				return
			}
		}
	}
	fb.drawText(x, y, text, scaleFor(size), c)
}

// drawTTF 自研 TTF 光栅文本（rune 级；alpha 混合抗锯齿）。
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
		for yy := 0; yy < h; yy++ {
			rowY := y + yy
			if rowY < 0 || rowY >= fb.h {
				continue
			}
			rowBase := rowY * fb.w
			for xx := 0; xx < w; xx++ {
				al := a[yy*w+xx]
				if al == 0 {
					continue
				}
				fx := cx + xx
				if fx < 0 || fx >= fb.w {
					continue
				}
				if al == 255 {
					fb.buf[rowBase+fx] = c
					continue
				}
				blendPixel(&fb.buf[rowBase+fx], c, int(al))
			}
		}
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
