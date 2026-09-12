package main

/*
#include <stdlib.h>
*/
import "C"

import "sync"

// cleg 渲染服务 C ABI（纯 Go，零系统依赖，跨系统构建 .so/.dll/.dylib）。
// qk 侧 library 绑定：create/clear/rect/roundrect/text/frame/screen_*。

var (
	gfb   *framebuffer
	gfbMu sync.Mutex
)

//export cleg_create
func cleg_create(w, h C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		gfb = &framebuffer{}
	}
	gfb.reset(int(w), int(h))
	return 0
}

//export cleg_clear
func cleg_clear(r, g, b C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.fillPixels(rgb(byte(r), byte(g), byte(b)))
	return 0
}

//export cleg_rect
func cleg_rect(x, y, w, h, r, g, b C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.fillRect(int(x), int(y), int(w), int(h), rgb(byte(r), byte(g), byte(b)))
	return 0
}

//export cleg_roundrect
func cleg_roundrect(x, y, w, h, radius, r, g, b C.int) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.fillRoundRect(int(x), int(y), int(w), int(h), int(radius), rgb(byte(r), byte(g), byte(b)))
	return 0
}

//export cleg_text
func cleg_text(x, y, size, r, g, b C.int, text, font *C.char) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	gfb.drawTextChain(int(x), int(y), C.GoString(text), int(size), rgb(byte(r), byte(g), byte(b)), C.GoString(font))
	return 0
}

//export cleg_frame
func cleg_frame(path *C.char) C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	if err := gfb.savePNG(C.GoString(path)); err != nil {
		return -2
	}
	return 0
}

// screen_*：无依赖跨系统语义——screen_open 记录尺寸，present 输出帧（PNG），close 置空。
// （窗口宿主由使用方/各平台可选实现负责；核心渲染零系统依赖。）

var (
	screenW, screenH int
)

//export cleg_screen_open
func cleg_screen_open(w, h C.int, title *C.char) C.int {
	screenW, screenH = int(w), int(h)
	_ = title
	return 0
}

//export cleg_screen_present
func cleg_screen_present() C.int {
	gfbMu.Lock()
	defer gfbMu.Unlock()
	if gfb == nil {
		return -1
	}
	if err := gfb.savePNG(".cleg-frame.png"); err != nil {
		return -2
	}
	return 0
}

//export cleg_screen_close
func cleg_screen_close() C.int { return 0 }

func main() {}
