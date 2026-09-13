package main

/*
#include <stdlib.h>
*/
import "C"

// cleg Style v2 渲染原语 C ABI（STYLE-SPEC §9 冻结签名，导出名/参数顺序逐字一致）。
// 既有导出（cleg_create/clear/rect/roundrect/text/frame/screen_*）见 exports.go，保持不变。

// goStr NULL 安全的 C 字符串转换。
func goStr(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

//export cleg_blend_rect
func cleg_blend_rect(x, y, w, h C.int, argb C.uint, alpha, radius C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.blendRectCore(int(x), int(y), int(w), int(h), uint32(argb), int(alpha), int(radius))
	return 0
}

//export cleg_gradient
func cleg_gradient(x, y, w, h, dir C.int, c1, c2 C.uint, radius C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.gradientCore(int(x), int(y), int(w), int(h), int(dir), uint32(c1), uint32(c2), int(radius))
	return 0
}

//export cleg_radial
func cleg_radial(cx, cy, r C.int, c1, c2 C.uint) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.radialCore(int(cx), int(cy), int(r), uint32(c1), uint32(c2))
	return 0
}

//export cleg_border
func cleg_border(x, y, w, h, wt, wr, wb, wl C.int,
	ct, cr, cb, cl C.uint, radius, style C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.borderCore(int(x), int(y), int(w), int(h), int(wt), int(wr), int(wb), int(wl),
		uint32(ct), uint32(cr), uint32(cb), uint32(cl), int(radius), int(style))
	return 0
}

//export cleg_shadow
func cleg_shadow(x, y, w, h, dx, dy, blur, spread C.int, argb C.uint, inset, radius C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.shadowCore(int(x), int(y), int(w), int(h), int(dx), int(dy), int(blur), int(spread),
		uint32(argb), int(inset), int(radius))
	return 0
}

//export cleg_clip_push
func cleg_clip_push(x, y, w, h C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	return C.int(gfb.clipPush(int(x), int(y), int(w), int(h)))
}

//export cleg_clip_pop
func cleg_clip_pop() C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	return C.int(gfb.clipPop())
}

//export cleg_text_width
func cleg_text_width(text *C.char, size C.int, font *C.char) C.int {
	if int(size) <= 0 {
		return 0
	}
	return C.int(textWidthCore(goStr(text), int(size), goStr(font)))
}

//export cleg_text_height
func cleg_text_height(size C.int) C.int {
	return C.int(lineHeightPx(int(size)))
}

//export cleg_text_ex
func cleg_text_ex(text *C.char, x, y, size C.int, font *C.char, color C.uint,
	align, valign, letter_spacing, line_height, decoration, ellipsis, max_w C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.textExCore(goStr(text), int(x), int(y), int(size), goStr(font), uint32(color),
		int(align), int(valign), int(letter_spacing), int(line_height),
		int(decoration), int(ellipsis), int(max_w))
	return 0
}

//export cleg_image
func cleg_image(path *C.char, x, y, w, h, repeat C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	return C.int(gfb.imageCore(goStr(path), int(x), int(y), int(w), int(h), int(repeat)))
}

//export cleg_transform
func cleg_transform(dx, dy, sx_num, sx_den, sy_num, sy_den C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.xfSet(int(dx), int(dy), int(sx_num), int(sx_den), int(sy_num), int(sy_den))
	return 0
}
