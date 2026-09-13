package main

// cleg Style v2 渲染原语自测（Go 侧，直接调内部内核，避免 cgo 边界）。
// 覆盖：blend_rect 半透明像素正确、gradient 两端色、border 四边色/虚线、
// clip 栈（含 text/image 受裁剪）、text_width/height、image 解码+缓存+4 种 repeat、
// transform、radial、外/内阴影、advanceOf 与 rasterGlyph 一致性。

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestFB(w, h int) *framebuffer {
	fb := &framebuffer{}
	fb.reset(w, h)
	return fb
}

func rgbAt(fb *framebuffer, x, y int) (int, int, int) {
	v := fb.buf[y*fb.w+x]
	return int(v >> 16 & 0xFF), int(v >> 8 & 0xFF), int(v & 0xFF)
}

func wantRGB(t *testing.T, fb *framebuffer, x, y, r, g, b int, what string) {
	t.Helper()
	gr, gg, gb := rgbAt(fb, x, y)
	if gr != r || gg != g || gb != b {
		t.Fatalf("%s: (%d,%d)=(%d,%d,%d)，期望 (%d,%d,%d)", what, x, y, gr, gg, gb, r, g, b)
	}
}

func near(t *testing.T, got, want, tol int, what string) {
	t.Helper()
	d := got - want
	if d < 0 {
		d = -d
	}
	if d > tol {
		t.Fatalf("%s: got %d，期望 %d±%d", what, got, want, tol)
	}
}

func inkCount(fb *framebuffer, x0, y0, x1, y1 int) int {
	n := 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if fb.buf[y*fb.w+x] != 0 {
				n++
			}
		}
	}
	return n
}

// ---------- 1. blend_rect：半透明像素正确 ----------

func TestBlendRectAlpha(t *testing.T) {
	fb := newTestFB(64, 64)
	fb.fillPixels(0xFFFFFF)
	fb.blendRectCore(10, 10, 20, 20, 0x00FF0000, 128, 0)
	wantRGB(t, fb, 20, 20, 255, 127, 127, "50% 红覆盖白底")
	wantRGB(t, fb, 9, 9, 255, 255, 255, "混合区外不受影响")
	wantRGB(t, fb, 30, 30, 255, 255, 255, "右下边界外")
	fb.blendRectCore(40, 40, 10, 10, 0x00FF0000, 255, 0)
	wantRGB(t, fb, 45, 45, 255, 0, 0, "不透明红")
	fb.blendRectCore(0, 0, 64, 64, 0x0000FF00, 0, 0)
	wantRGB(t, fb, 45, 45, 255, 0, 0, "alpha=0 无副作用")
	// argb 的 A 通道与 alpha 参数相乘：0x80 × 128 ≈ 64/255
	fb2 := newTestFB(8, 8)
	fb2.blendRectCore(0, 0, 8, 8, 0x80FF0000, 128, 0)
	near(t, mustR(t, fb2, 4, 4), 64, 1, "A 通道×alpha 参数")
	// 黑底 50% 红 → (128,0,0)（255*128/255 恰好取整为 128）
	fb3 := newTestFB(8, 8)
	fb3.blendRectCore(0, 0, 8, 8, 0x00FF0000, 128, 0)
	wantRGB(t, fb3, 4, 4, 128, 0, 0, "50% 红覆盖黑底")
}

func mustR(t *testing.T, fb *framebuffer, x, y int) int {
	t.Helper()
	r, _, _ := rgbAt(fb, x, y)
	return r
}

func TestBlendRectRadius(t *testing.T) {
	fb := newTestFB(48, 48)
	fb.fillPixels(0xFFFFFF)
	fb.blendRectCore(0, 0, 32, 32, 0x00FF0000, 255, 8)
	wantRGB(t, fb, 16, 16, 255, 0, 0, "圆角中心")
	wantRGB(t, fb, 1, 1, 255, 255, 255, "圆角外角被裁掉")
	wantRGB(t, fb, 30, 30, 255, 255, 255, "右下圆角同样裁掉")
	wantRGB(t, fb, 24, 24, 255, 0, 0, "圆角内侧保留")
	wantRGB(t, fb, 16, 1, 255, 0, 0, "上边中点保留")
	wantRGB(t, fb, 16, 30, 255, 0, 0, "下边中点保留")
	// 边缘抗锯齿：角过渡带存在非 0/255 的混合像素
	// 源色为纯红、底色为白：r 恒 255，只有 g 能体现部分覆盖（g=255-alpha）
	partial := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			_, g, _ := rgbAt(fb, x, y)
			if g > 0 && g < 255 {
				partial++
			}
		}
	}
	if partial == 0 {
		t.Fatalf("圆角边缘缺少抗锯齿过渡像素")
	}
}

// ---------- 2. gradient：两端色正确 ----------

func TestGradientEndpoints(t *testing.T) {
	fb := newTestFB(100, 20)
	fb.gradientCore(0, 0, 100, 20, 0, 0xFFFF0000, 0xFF0000FF, 0)
	r, _, b := rgbAt(fb, 0, 10)
	if r < 250 || b > 5 {
		t.Fatalf("dir=0 左端应≈纯红，got (%d,_,%d)", r, b)
	}
	r, _, b = rgbAt(fb, 99, 10)
	if b < 250 || r > 5 {
		t.Fatalf("dir=0 右端应≈纯蓝，got (%d,_,%d)", r, b)
	}
	_, _, _ = rgbAt(fb, 50, 10)
	rm, _, bm := rgbAt(fb, 50, 10)
	near(t, rm, 128, 3, "dir=0 中点红分量")
	near(t, bm, 127, 3, "dir=0 中点蓝分量")
	// dir=1 上→下
	fb2 := newTestFB(20, 100)
	fb2.gradientCore(0, 0, 20, 100, 1, 0x00FF0000, 0x0000FF00, 0)
	r, g, _ := rgbAt(fb2, 10, 0)
	if r < 250 || g > 5 {
		t.Fatalf("dir=1 上端应为红，got (%d,%d,_)", r, g)
	}
	r, g, _ = rgbAt(fb2, 10, 99)
	if g < 250 || r > 5 {
		t.Fatalf("dir=1 下端应为绿，got (%d,%d,_)", r, g)
	}
	// 角度 270 == dir 3（下→上）：底部红
	fb3 := newTestFB(20, 100)
	fb3.gradientCore(0, 0, 20, 100, 270, 0x00FF0000, 0x0000FF00, 0)
	r, g, _ = rgbAt(fb3, 10, 99)
	if r < 250 || g > 5 {
		t.Fatalf("角度 270 底部应为红，got (%d,%d,_)", r, g)
	}
	// 圆角渐变：角外不画
	fb4 := newTestFB(40, 40)
	fb4.gradientCore(0, 0, 32, 32, 0, 0x00FF0000, 0x0000FF00, 8)
	wantRGB(t, fb4, 1, 1, 0, 0, 0, "圆角渐变角外")
	if _, _, _ = rgbAt(fb4, 16, 16); fb4.buf[16*fb4.w+16] == 0 {
		t.Fatalf("圆角渐变中心未绘制")
	}
	// 透明端点：A=0x01 表示近乎透明（非"两端全 0"的不透明兼容路径）
	fb5 := newTestFB(8, 8)
	fb5.fillPixels(0xFFFFFF)
	fb5.gradientCore(0, 0, 8, 8, 0, 0x01FF0000, 0x01FF0000, 0)
	wantRGB(t, fb5, 4, 4, 255, 254, 254, "低 alpha 端点渐隐")
}

// ---------- 3. border：四边色不同 + 虚线 ----------

func TestBorderFourSides(t *testing.T) {
	fb := newTestFB(120, 120)
	fb.fillPixels(0x000000)
	fb.borderCore(10, 10, 100, 100, 4, 4, 4, 4,
		0x00FF0000, 0x0000FF00, 0x000000FF, 0x00FFFF00, 0, 0)
	wantRGB(t, fb, 60, 11, 255, 0, 0, "上边框红")
	wantRGB(t, fb, 109, 60, 0, 255, 0, "右边框绿")
	wantRGB(t, fb, 60, 108, 0, 0, 255, "下边框蓝")
	wantRGB(t, fb, 11, 60, 255, 255, 0, "左边框黄")
	wantRGB(t, fb, 60, 60, 0, 0, 0, "边框内部不填充")
	wantRGB(t, fb, 5, 5, 0, 0, 0, "边框外部不绘制")
	// 四边不同宽度：wt=1 wr=20 wb=1 wl=10
	fb2 := newTestFB(120, 120)
	fb2.borderCore(0, 0, 100, 100, 1, 20, 1, 10,
		0x00FF0000, 0x0000FF00, 0x000000FF, 0x00FFFF00, 0, 0)
	wantRGB(t, fb2, 5, 50, 255, 255, 0, "左 10px 黄")
	wantRGB(t, fb2, 15, 50, 0, 0, 0, "左内边界之后为内部")
	wantRGB(t, fb2, 79, 50, 0, 0, 0, "右内边界之前为内部")
	wantRGB(t, fb2, 95, 50, 0, 255, 0, "右 20px 绿")
	wantRGB(t, fb2, 50, 90, 0, 0, 0, "下 1px 之上为内部")
	wantRGB(t, fb2, 50, 99, 0, 0, 255, "下 1px 蓝")
	// 圆角边框：四角裁掉
	fb3 := newTestFB(64, 64)
	fb3.borderCore(0, 0, 48, 48, 3, 3, 3, 3,
		0x00FF0000, 0x0000FF00, 0x000000FF, 0x00FFFF00, 12, 0)
	wantRGB(t, fb3, 1, 1, 0, 0, 0, "圆角边框角外")
	wantRGB(t, fb3, 24, 1, 255, 0, 0, "圆角边框上边中点")
	// dashed / dotted：沿边出现实/空交替
	for _, style := range []int{1, 2} {
		f := newTestFB(200, 200)
		f.borderCore(0, 0, 200, 200, 4, 4, 4, 4,
			0x00FF0000, 0x00FF0000, 0x00FF0000, 0x00FF0000, 0, style)
		on, off := 0, 0
		for x := 0; x < 200; x++ {
			r, _, _ := rgbAt(f, x, 1)
			if r == 255 {
				on++
			} else {
				off++
			}
		}
		if on == 0 || off == 0 {
			t.Fatalf("style=%d 虚线未产生实/空交替（on=%d off=%d）", style, on, off)
		}
	}
}

// ---------- 4. clip 栈 ----------

func TestClipStack(t *testing.T) {
	fb := newTestFB(100, 100)
	fb.fillPixels(0x000000)
	if rc := fb.clipPush(10, 10, 30, 30); rc != 0 {
		t.Fatalf("clipPush rc=%d", rc)
	}
	fb.fillRect(0, 0, 100, 100, 0x00FFFFFF)
	wantRGB(t, fb, 20, 20, 255, 255, 255, "裁剪区内")
	wantRGB(t, fb, 50, 50, 0, 0, 0, "裁剪区外(右)")
	wantRGB(t, fb, 5, 5, 0, 0, 0, "裁剪区外(左上)")
	wantRGB(t, fb, 39, 39, 255, 255, 255, "裁剪区右下角内")
	wantRGB(t, fb, 40, 40, 0, 0, 0, "裁剪区右下角外")
	// 嵌套：交集 [10,15)×[10,15)
	if rc := fb.clipPush(0, 0, 15, 15); rc != 0 {
		t.Fatalf("嵌套 clipPush rc=%d", rc)
	}
	fb.fillRect(0, 0, 100, 100, 0x00FF0000)
	wantRGB(t, fb, 12, 12, 255, 0, 0, "嵌套裁剪内")
	wantRGB(t, fb, 20, 20, 255, 255, 255, "嵌套裁剪外(第一层内)")
	// blend_rect 同样受裁剪约束
	fb.blendRectCore(0, 0, 100, 100, 0x0000FF00, 255, 0)
	wantRGB(t, fb, 12, 12, 0, 255, 0, "blend_rect 受裁剪")
	wantRGB(t, fb, 20, 20, 255, 255, 255, "blend_rect 裁剪外不变")
	if rc := fb.clipPop(); rc != 0 {
		t.Fatalf("clipPop rc=%d", rc)
	}
	fb.fillRect(0, 0, 100, 100, 0x000000FF)
	wantRGB(t, fb, 20, 20, 0, 0, 255, "pop 恢复第一层裁剪")
	if rc := fb.clipPop(); rc != 0 {
		t.Fatalf("clipPop rc=%d", rc)
	}
	fb.fillRect(0, 0, 100, 100, 0x00FFFF00)
	wantRGB(t, fb, 50, 50, 255, 255, 0, "pop 恢复全屏")
	if rc := fb.clipPop(); rc != -1 {
		t.Fatalf("空栈 pop 应返回 -1，got %d", rc)
	}
}

// ---------- 5. text_width / text_height / text_ex ----------

func TestTextMetrics(t *testing.T) {
	w := textWidthCore("Hello", 24, "monospace")
	if w <= 0 {
		t.Fatalf("text_width(Hello,24) 应 > 0，got %d", w)
	}
	if w2 := textWidthCore("Hello", 48, "monospace"); w2 <= w {
		t.Fatalf("字号翻倍宽度应增大: 24px=%d 48px=%d", w, w2)
	}
	if w3 := textWidthCore("Hello World", 24, "monospace"); w3 <= w {
		t.Fatalf("更长文本应更宽: %d vs %d", w3, w)
	}
	if textWidthCore("", 24, "monospace") != 0 {
		t.Fatalf("空文本宽度应为 0")
	}
	if h := lineHeightPx(24); h != 28 {
		t.Fatalf("text_height(24)=%d，期望 28（size+4，与 drawTTF 行推进一致）", h)
	}
	if h := lineHeightPx(0); h != 0 {
		t.Fatalf("text_height(0)=%d，期望 0", h)
	}
	if cw := textWidthCore("中文测试", 20, "monospace"); cw <= 0 {
		t.Fatalf("CJK 文本宽度应 > 0（回退链），got %d", cw)
	}
	if mw := textWidthCore("ab\nabcdef", 24, "monospace"); mw < textWidthCore("abcdef", 24, "monospace") {
		t.Fatalf("多行取最宽行")
	}
}

func TestTextExDrawsAndClips(t *testing.T) {
	fb := newTestFB(200, 60)
	fb.textExCore("Hello", 10, 10, 24, "monospace", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	if ink := inkCount(fb, 0, 0, 200, 60); ink == 0 {
		t.Fatalf("text_ex 未绘制任何像素")
	}
	// 裁剪约束文本
	fb2 := newTestFB(200, 60)
	fb2.clipPush(0, 0, 40, 20)
	fb2.textExCore("Hello", 10, 10, 24, "monospace", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	if ink := inkCount(fb2, 0, 20, 200, 60); ink != 0 {
		t.Fatalf("text_ex 越出裁剪区（y>=20 有 %d 个像素）", ink)
	}
	if ink := inkCount(fb2, 40, 0, 200, 60); ink != 0 {
		t.Fatalf("text_ex 越出裁剪区（x>=40 有 %d 个像素）", ink)
	}
	if ink := inkCount(fb2, 0, 0, 40, 20); ink == 0 {
		t.Fatalf("裁剪区内应有文本像素")
	}
	// align=1 居中（max_w 为参照盒）
	fb3 := newTestFB(200, 60)
	fb3.textExCore("Hi", 0, 10, 24, "monospace", 0x00FFFFFF, 1, 0, 0, 0, 0, 0, 100)
	lw := inkCount(fb3, 0, 10, 50, 40)
	rw := inkCount(fb3, 50, 10, 100, 40)
	if lw == 0 || rw == 0 {
		t.Fatalf("居中文本应跨中线: 左%d 右%d", lw, rw)
	}
	// align=2 右对齐：墨水不越过 max_w
	fb4 := newTestFB(200, 60)
	fb4.textExCore("Hi", 0, 10, 24, "monospace", 0x00FFFFFF, 2, 0, 0, 0, 0, 0, 60)
	if ink := inkCount(fb4, 60, 0, 200, 60); ink != 0 {
		t.Fatalf("右对齐越界 %d 像素", ink)
	}
	if ink := inkCount(fb4, 0, 10, 60, 40); ink == 0 {
		t.Fatalf("右对齐应绘制在参照盒内")
	}
	// valign=2 bottom：y 为文本块底 → 墨水应全部在 y 之上
	fb5 := newTestFB(200, 60)
	fb5.textExCore("Hi", 10, 40, 24, "monospace", 0x00FFFFFF, 0, 2, 0, 0, 0, 0, 0)
	if ink := inkCount(fb5, 0, 40, 200, 60); ink != 0 {
		t.Fatalf("valign=bottom 越出 %d 像素", ink)
	}
	if ink := inkCount(fb5, 10, 10, 200, 40); ink == 0 {
		t.Fatalf("valign=bottom 应在 y 之上绘制")
	}
	// decoration 增加装饰线像素
	fb6 := newTestFB(200, 60)
	fb6.textExCore("Hi", 10, 10, 24, "monospace", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	plain := inkCount(fb6, 0, 0, 200, 60)
	fb6.decorateLine(10, 10, 999, 24, 0x00FFFFFF, 255, 1)
	if inkCount(fb6, 0, 0, 200, 60) <= plain {
		t.Fatalf("underline 未增加像素")
	}
	// letter-spacing 拉宽
	fb7 := newTestFB(400, 60)
	fb7.textExCore("iiii", 0, 10, 24, "monospace", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	sp := 0
	for x := 0; x < 400; x++ {
		for y := 0; y < 60; y++ {
			if fb7.buf[y*fb7.w+x] != 0 {
				sp = x
			}
		}
	}
	fb8 := newTestFB(400, 60)
	fb8.textExCore("iiii", 0, 10, 24, "monospace", 0x00FFFFFF, 0, 0, 10, 0, 0, 0, 0)
	sp2 := 0
	for x := 0; x < 400; x++ {
		for y := 0; y < 60; y++ {
			if fb8.buf[y*fb8.w+x] != 0 {
				sp2 = x
			}
		}
	}
	if sp2 <= sp {
		t.Fatalf("letter-spacing 应拉宽文本: %d -> %d", sp, sp2)
	}
}

func TestEllipsisTruncation(t *testing.T) {
	f := pickFont("Hello World", 24, "monospace")
	long := "Hello World Hello World Hello World"
	full := measureLine(f, long, 24, 0)
	s, w := fitLine(f, long, 24, 0, 1, 120)
	if w > 120 {
		t.Fatalf("省略号截断宽度 %d 超过 max_w=120", w)
	}
	if w >= full {
		t.Fatalf("截断后应短于原文: %d vs %d", w, full)
	}
	if !strings.HasSuffix(s, "…") && !strings.HasSuffix(s, "...") {
		t.Fatalf("截断结果应以省略号结尾，got %q", s)
	}
	// max_w<=0 表示不限
	s2, w2 := fitLine(f, long, 24, 0, 1, 0)
	if s2 != long || w2 != full {
		t.Fatalf("max_w<=0 不应截断")
	}
	// ellipsis=0 不截断
	if s3, _ := fitLine(f, long, 24, 0, 0, 40); s3 != long {
		t.Fatalf("ellipsis=0 不应截断")
	}
}

// advanceOf 必须与 rasterGlyph 的推进量逐字符一致（度量 = 绘制）。
func TestAdvanceMatchesRaster(t *testing.T) {
	var f *ttfFont
	for _, p := range fontIndexAll() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if ff, err := parseTTF(data); err == nil {
			f = ff
			break
		}
	}
	if f == nil {
		t.Skip("系统无可用 TTF，跳过（5x7 回退路径由 text_width 测试覆盖）")
	}
	for _, size := range []int{10, 16, 24, 40} {
		for ch := rune(32); ch < 127; ch++ {
			w, _, adv, _ := f.rasterGlyph(ch, size)
			got, ok := f.advanceOf(ch, size)
			if adv <= 0 && w == 0 {
				if ok {
					t.Fatalf("size=%d ch=%q: rasterGlyph 空字形但 advanceOf ok", size, ch)
				}
				continue
			}
			if !ok || got != adv {
				t.Fatalf("size=%d ch=%q: advanceOf=%d ok=%v 与 rasterGlyph adv=%d 不一致", size, ch, got, ok, adv)
			}
		}
	}
}

// ---------- 6. image：解码 + 缓存 + repeat ----------

func writePNG(t *testing.T, path string, w, h int, c color.NRGBA) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

func TestImageLoadRepeatAndCache(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tile.png")
	writePNG(t, p, 4, 4, color.NRGBA{R: 255, A: 255})
	fb := newTestFB(64, 64)
	if rc := fb.imageCore(p, 8, 8, 0, 0, 0); rc != 0 {
		t.Fatalf("imageCore rc=%d", rc)
	}
	wantRGB(t, fb, 8, 8, 255, 0, 0, "自然尺寸左上")
	wantRGB(t, fb, 11, 11, 255, 0, 0, "自然尺寸右下")
	wantRGB(t, fb, 12, 12, 0, 0, 0, "自然尺寸之外不绘制")
	// 缩放到 8×8
	fb.imageCore(p, 20, 20, 8, 8, 0)
	wantRGB(t, fb, 20, 20, 255, 0, 0, "缩放左上")
	wantRGB(t, fb, 27, 27, 255, 0, 0, "缩放右下")
	wantRGB(t, fb, 28, 28, 0, 0, 0, "缩放之外")
	// repeat：4×4 瓦片铺满 20×20 盒
	fb2 := newTestFB(64, 64)
	fb2.imageCore(p, 0, 0, 20, 20, 1)
	wantRGB(t, fb2, 3, 3, 255, 0, 0, "第 1 瓦片")
	wantRGB(t, fb2, 19, 19, 255, 0, 0, "第 5 瓦片（铺满盒子）")
	wantRGB(t, fb2, 20, 20, 0, 0, 0, "盒子之外不铺")
	// repeat-x：只有一行
	fb3 := newTestFB(64, 64)
	fb3.imageCore(p, 0, 0, 20, 20, 2)
	wantRGB(t, fb3, 19, 3, 255, 0, 0, "repeat-x 横向铺开")
	wantRGB(t, fb3, 19, 8, 0, 0, 0, "repeat-x 不纵向铺开")
	// repeat-y：只有一列
	fb4 := newTestFB(64, 64)
	fb4.imageCore(p, 0, 0, 20, 20, 3)
	wantRGB(t, fb4, 3, 19, 255, 0, 0, "repeat-y 纵向铺开")
	wantRGB(t, fb4, 8, 19, 0, 0, 0, "repeat-y 不横向铺开")
	// 带 alpha 的 PNG 正确混合：50% 红覆盖白底
	pa := filepath.Join(dir, "alpha.png")
	writePNG(t, pa, 2, 2, color.NRGBA{R: 255, A: 128})
	fb5 := newTestFB(16, 16)
	fb5.fillPixels(0xFFFFFF)
	fb5.imageCore(pa, 2, 2, 0, 0, 0)
	wantRGB(t, fb5, 2, 2, 255, 127, 127, "PNG alpha 混合白底")
	// 同一路径只解码一次（指针相同 + 缓存条目）
	s1, err := loadSprite(p)
	if err != nil {
		t.Fatal(err)
	}
	s2, err := loadSprite(p)
	if err != nil {
		t.Fatal(err)
	}
	if s1 != s2 {
		t.Fatalf("同一路径应命中解码缓存")
	}
	spriteMu.Lock()
	n := len(spriteCache)
	spriteMu.Unlock()
	if n != 2 {
		t.Fatalf("精灵缓存条目应恰为 2（tile+alpha），got %d", n)
	}
	// 缺失文件
	if rc := fb.imageCore(filepath.Join(dir, "nope.png"), 0, 0, 0, 0, 0); rc != -2 {
		t.Fatalf("缺失图片应返回 -2，got %d", rc)
	}
}

// ---------- 7. transform / radial / shadow ----------

func TestTransform(t *testing.T) {
	fb := newTestFB(100, 100)
	fb.xfSet(10, 20, 2, 1, 3, 1)
	fb.blendRectCore(0, 0, 5, 5, 0x00FFFFFF, 255, 0)
	wantRGB(t, fb, 10, 20, 255, 255, 255, "变换矩形左上")
	wantRGB(t, fb, 19, 34, 255, 255, 255, "变换矩形右下")
	wantRGB(t, fb, 9, 20, 0, 0, 0, "变换矩形左外")
	wantRGB(t, fb, 20, 35, 0, 0, 0, "变换矩形右下外")
	// 复位（全 0 = 单位变换）
	fb.xfSet(0, 0, 0, 0, 0, 0)
	fb.blendRectCore(50, 50, 2, 2, 0x00FF0000, 255, 0)
	wantRGB(t, fb, 50, 50, 255, 0, 0, "复位后按原坐标绘制")
	// 变换 + 裁剪：逻辑 [0,10) → 设备 [5,15)
	fb2 := newTestFB(100, 100)
	fb2.xfSet(5, 5, 1, 1, 1, 1)
	fb2.clipPush(0, 0, 10, 10)
	fb2.blendRectCore(0, 0, 100, 100, 0x00FFFFFF, 255, 0)
	wantRGB(t, fb2, 10, 10, 255, 255, 255, "变换后裁剪区内")
	wantRGB(t, fb2, 1, 1, 0, 0, 0, "变换后裁剪区外(左上)")
	wantRGB(t, fb2, 16, 16, 0, 0, 0, "变换后裁剪区外(右下)")
	// 缩放文本：字号随 sy 放大
	fb3 := newTestFB(200, 200)
	fb3.textExCore("H", 0, 0, 20, "monospace", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	h1 := 0
	for y := 0; y < 200; y++ {
		if inkCount(fb3, 0, y, 200, y+1) > 0 {
			h1 = y + 1
		}
	}
	fb4 := newTestFB(200, 200)
	fb4.xfSet(0, 0, 1, 1, 3, 1)
	fb4.textExCore("H", 0, 0, 20, "monospace", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	h2 := 0
	for y := 0; y < 200; y++ {
		if inkCount(fb4, 0, y, 200, y+1) > 0 {
			h2 = y + 1
		}
	}
	if h2 <= h1 {
		t.Fatalf("sy=3 应放大文本高度: %d -> %d", h1, h2)
	}
}

func TestRadial(t *testing.T) {
	fb := newTestFB(64, 64)
	fb.radialCore(32, 32, 20, 0x00FF0000, 0x000000FF)
	// 像素中心 (32.5,32.5) 距圆心 0.71px → t≈0.035，中心色≈(246,_,9)
	r, _, b := rgbAt(fb, 32, 32)
	if r < 240 || b > 15 {
		t.Fatalf("radial 中心应≈红，got (%d,_,%d)", r, b)
	}
	// 边缘像素 (51,32) 距圆心 19.5px → t≈0.975，边缘色≈(6,_,249)
	r, _, b = rgbAt(fb, 51, 32)
	if b < 240 || r > 15 {
		t.Fatalf("radial 边缘应≈蓝，got (%d,_,%d)", r, b)
	}
	wantRGB(t, fb, 32, 5, 0, 0, 0, "radial 半径外不绘制")
	wantRGB(t, fb, 0, 0, 0, 0, 0, "radial 四角外不绘制")
}

func TestShadowOuter(t *testing.T) {
	fb := newTestFB(200, 200)
	fb.fillPixels(0xFFFFFF)
	fb.shadowCore(50, 50, 60, 60, 4, 4, 8, 0, 0x80000000, 0, 0)
	// 核心（被全部 8 层覆盖）≈ 128 alpha
	near(t, mustR(t, fb, 80, 80), 127, 4, "外阴影核心 alpha≈base")
	// 近边缘：淡一些但仍有阴影
	near(t, mustR(t, fb, 48, 80), 227, 40, "外阴影边缘过渡")
	// 远处：blur 之外不受影响
	wantRGB(t, fb, 20, 80, 255, 255, 255, "外阴影范围外")
	// blur=0 → 硬边、无过渡
	fb2 := newTestFB(200, 200)
	fb2.fillPixels(0xFFFFFF)
	fb2.shadowCore(50, 50, 60, 60, 0, 0, 0, 0, 0x80000000, 0, 0)
	near(t, mustR(t, fb2, 80, 80), 127, 2, "blur=0 核心")
	wantRGB(t, fb2, 45, 80, 255, 255, 255, "blur=0 边界外无阴影")
	// spread 外扩
	fb3 := newTestFB(200, 200)
	fb3.fillPixels(0xFFFFFF)
	fb3.shadowCore(50, 50, 60, 60, 0, 0, 0, 10, 0x80000000, 0, 0)
	near(t, mustR(t, fb3, 45, 80), 127, 4, "spread=10 外扩覆盖")
	wantRGB(t, fb3, 39, 80, 255, 255, 255, "spread 之外仍无阴影")
}

func TestShadowInset(t *testing.T) {
	fb := newTestFB(200, 200)
	fb.fillPixels(0xFFFFFF)
	fb.shadowCore(50, 50, 60, 60, 0, 4, 8, 0, 0x80000000, 1, 0)
	r := mustR(t, fb, 80, 51)
	if r >= 250 {
		t.Fatalf("内阴影：上内缘应被压暗，got %d", r)
	}
	wantRGB(t, fb, 80, 80, 255, 255, 255, "内阴影：中心不变")
	wantRGB(t, fb, 80, 108, 255, 255, 255, "内阴影：洞内不变")
	wantRGB(t, fb, 20, 80, 255, 255, 255, "内阴影：框外不变")
	// inset 反方向（dy=-4）应压暗下内缘
	fb2 := newTestFB(200, 200)
	fb2.fillPixels(0xFFFFFF)
	fb2.shadowCore(50, 50, 60, 60, 0, -4, 8, 0, 0x80000000, 1, 0)
	if r2 := mustR(t, fb2, 80, 107); r2 >= 250 {
		t.Fatalf("内阴影 dy=-4：下内缘应被压暗，got %d", r2)
	}
	wantRGB(t, fb2, 80, 51, 255, 255, 255, "内阴影 dy=-4：上内缘不变")
}

// ---------- 性能基准（"性能优先"策略的实测依据） ----------

func BenchmarkBlendRoundRect(b *testing.B) {
	fb := newTestFB(1024, 768)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fb.blendRoundRect(10, 10, 1000, 700, 16, 0x80446688, 200)
	}
}

func BenchmarkGradient(b *testing.B) {
	fb := newTestFB(1024, 768)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fb.gradientCore(0, 0, 1024, 768, 1, 0xFFFF0000, 0xFF0000FF, 8)
	}
}

func BenchmarkShadowBlur16(b *testing.B) {
	fb := newTestFB(1024, 768)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fb.shadowCore(200, 200, 400, 300, 4, 6, 16, 0, 0x80000000, 0, 12)
	}
}

func BenchmarkBorderDashed(b *testing.B) {
	fb := newTestFB(1024, 768)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fb.borderCore(10, 10, 1000, 700, 3, 3, 3, 3,
			0x00FF0000, 0x0000FF00, 0x000000FF, 0x00FFFF00, 12, 1)
	}
}

// ---------- 字体链路回归（Style v2 文字修复） ----------

// glyphInkSimple 字形墨水像素数 + 包围盒 + 推进量。
func glyphInkSimple(f *ttfFont, ch rune, size int) (ink, w, h, adv int) {
	w, h, adv, a := f.rasterGlyph(ch, size)
	for _, v := range a {
		if v > 0 {
			ink++
		}
	}
	return ink, w, h, adv
}

// anyFont 取一个可用字体用于字形断言（无字体环境跳过）。
func anyFont(t *testing.T, size int) *ttfFont {
	t.Helper()
	for _, name := range []string{"monospace", "sans-serif", "serif"} {
		if f := pickFont("ocean theme style v2", size, name); f != nil {
			return f
		}
	}
	t.Skip("系统中无可用 TTF，跳过字形断言")
	return nil
}

// TestASCIIInkCoverage ASCII 26 字母 + 10 数字每个字形都必须有墨水（不得空字形）。
func TestASCIIInkCoverage(t *testing.T) {
	f := anyFont(t, 40)
	var empty []rune
	for _, span := range [][2]rune{{'A', 'Z'}, {'a', 'z'}, {'0', '9'}} {
		for ch := span[0]; ch <= span[1]; ch++ {
			if ink, _, _, _ := glyphInkSimple(f, ch, 40); ink == 0 {
				empty = append(empty, ch)
			}
		}
	}
	if len(empty) > 0 {
		t.Fatalf("字体 %s 存在空字形 %q（共 %d 个）", f.path, string(empty), len(empty))
	}
}

// hasHole 字形中部是否存在连续横向透明像素（字腔）。
func hasHole(a []byte, w, h int) bool {
	for y := h / 3; y < h*2/3; y++ {
		run := 0
		for x := 0; x < w; x++ {
			if a[y*w+x] == 0 {
				run++
				if run >= 2 {
					return true
				}
			} else {
				run = 0
			}
		}
	}
	return false
}

func inkOf(a []byte) int {
	n := 0
	for _, v := range a {
		if v > 0 {
			n++
		}
	}
	return n
}

// TestGlyphShapesRecognizable 形状可辨：'o'/'0' 有字腔、'l' 细、'm' 比 'l' 重、数字有墨。
func TestGlyphShapesRecognizable(t *testing.T) {
	f := anyFont(t, 40)
	mk := func(ch rune) ([]byte, int, int) {
		w, h, _, a := f.rasterGlyph(ch, 40)
		return a, w, h
	}
	ao, wo, ho := mk('o')
	if wo < 4 || ho < 6 {
		t.Fatalf("'o' 尺寸异常 %dx%d", wo, ho)
	}
	if !hasHole(ao, wo, ho) {
		t.Fatalf("'o' 缺少字腔（轮廓被填死）")
	}
	a0, w0, h0 := mk('0')
	if !hasHole(a0, w0, h0) {
		t.Fatalf("'0' 缺少字腔")
	}
	al, wl, hl := mk('l')
	inkL := inkOf(al)
	if wl <= 0 || hl <= 0 || inkL*3 > wl*hl {
		t.Fatalf("'l' 形状异常：墨水 %d / 包围盒 %dx%d", inkL, wl, hl)
	}
	am, wm, _ := mk('m')
	inkM := inkOf(am)
	if inkM <= inkL {
		t.Fatalf("'m'(墨水 %d) 应重于 'l'(墨水 %d)", inkM, inkL)
	}
	if wm < wl {
		t.Fatalf("'m'(%d) 应不窄于 'l'(%d)", wm, wl)
	}
}

// TestCJKFontResolution font 为空/族名/通用名时，CJK 文本都能命中含 CJK 字形的字体。
func TestCJKFontResolution(t *testing.T) {
	if len(fontIndexAll()) == 0 {
		t.Skip("系统无字体目录")
	}
	cjkText := "海洋控制台数据设置页面切换"
	if loadCJKFont(24) == nil {
		t.Skip("本机无可用 CJK 字形字体（CFF/OTF-only 环境），跳过")
	}
	for _, font := range []string{"", "Noto Sans CJK SC", "Noto Sans Mono CJK SC", "Source Han Sans CN", "Droid Sans Fallback", "monospace"} {
		f := pickFont(cjkText, 24, font)
		if f == nil {
			t.Fatalf("pickFont(font=%q) = nil", font)
		}
		if !f.coversCJK() {
			t.Fatalf("pickFont(font=%q) 选中无 CJK 字形的字体 %s", font, f.path)
		}
		for _, ch := range cjkText {
			if ink, _, _, _ := glyphInkSimple(f, ch, 24); ink == 0 {
				t.Fatalf("字体 %s 的 CJK 字形 %q 无墨水", f.path, ch)
			}
		}
	}
	if w1, w2 := textWidthCore(cjkText, 16, ""), textWidthCore(cjkText, 32, ""); w1 <= 0 || w2 <= w1 {
		t.Fatalf("CJK 文本度量异常: 16px=%d 32px=%d", w1, w2)
	}
}

// TestGenericFontFamilies 通用族名（monospace/sans-serif/serif）必须解析到真实字体。
func TestGenericFontFamilies(t *testing.T) {
	if len(fontIndexAll()) == 0 {
		t.Skip("系统无字体目录")
	}
	for _, name := range []string{"monospace", "sans-serif", "serif"} {
		f, err := loadFont(name, 24)
		if err != nil || f == nil {
			t.Fatalf("loadFont(%q) 失败: %v（qk 侧默认族名会退化到 5x7 位图）", name, err)
		}
		if ink, _, _, _ := glyphInkSimple(f, 'e', 24); ink == 0 {
			t.Fatalf("loadFont(%q) → %s 的 'e' 无墨水", name, f.path)
		}
	}
}

// TestEmptyChainLatinInk 空 font 参数的拉丁文本也必须走真实字体（不得落到带空洞的 5x7 位图）。
func TestEmptyChainLatinInk(t *testing.T) {
	f := pickFont("ocean theme style v2", 24, "")
	if f == nil {
		t.Skip("系统无可用 TTF")
	}
	fb := newTestFB(400, 60)
	fb.textExCore("ocean theme style v2", 0, 0, 24, "", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	ink := inkCount(fb, 0, 0, 400, 60)
	if ink < 80 {
		t.Fatalf("文本墨水过少（%d），可能退化到 5x7 位图", ink)
	}
}

// TestTTCFace0 TTC（.ttc）集合取第 0 个 face 解析。
func TestTTCFace0(t *testing.T) {
	ttcPath := ""
	for _, p := range fontIndexAll() {
		low := strings.ToLower(p)
		if !strings.HasSuffix(low, ".ttc") && !strings.HasSuffix(low, ".otc") {
			continue
		}
		if sniffGlyf(p) { // 优先挑 TrueType 轮廓的集合（CFF/CFF2 face 本光栅器不支持）
			ttcPath = p
			break
		}
		if ttcPath == "" {
			ttcPath = p
		}
	}
	if ttcPath == "" {
		t.Skip("系统无 .ttc/.otc 字体")
	}
	data, err := os.ReadFile(ttcPath)
	if err != nil {
		t.Skipf("读取 %s 失败: %v", ttcPath, err)
	}
	if string(data[0:4]) != "ttcf" {
		t.Skipf("%s 不是 TTC 容器", ttcPath)
	}
	base, err := sfntFaceOffset(data)
	if err != nil || base <= 0 {
		t.Fatalf("sfntFaceOffset(%s) = %d, %v", ttcPath, base, err)
	}
	f, err := parseTTF(data)
	if err != nil {
		if !sniffGlyf(ttcPath) {
			t.Skipf("%s face0 为 CFF/CFF2 轮廓（本光栅器不支持），跳过", ttcPath)
		}
		t.Fatalf("parseTTF(%s) 失败: %v", ttcPath, err)
	}
	if f.numGlyphs <= 0 || f.unitsPerEm <= 0 {
		t.Fatalf("TTC face0 解析异常: numGlyphs=%d upem=%d", f.numGlyphs, f.unitsPerEm)
	}
}

// TestMixedScriptFallback 混排文本逐 rune 回退：每个字符都必须有字体覆盖且渲染出墨水。
func TestMixedScriptFallback(t *testing.T) {
	if len(fontIndexAll()) == 0 {
		t.Skip("系统无字体目录")
	}
	mixed := "海洋 data 控制台 v2"
	fs := pickFontStack(mixed, 24, "Noto Sans CJK SC")
	if len(fs) == 0 {
		t.Skip("系统无可用 TTF")
	}
	if len(fs) < 2 {
		t.Skipf("本机只有 %d 个候选字体，无法验证跨脚本回退", len(fs))
	}
	for _, r := range mixed {
		if r == ' ' {
			continue
		}
		f := fs.pick(r)
		if f == nil {
			t.Fatalf("rune %q 无可用字体", r)
		}
		if gid := f.glyphIndex(r); gid == 0 || gid >= uint32(f.numGlyphs) {
			t.Fatalf("rune %q 在字体栈中无字形覆盖（栈=%d 个字体）", r, len(fs))
		}
		if ink, _, _, _ := glyphInkSimple(f, r, 24); ink == 0 {
			t.Fatalf("rune %q 用 %s 渲染无墨水", r, f.path)
		}
	}
	// 整行绘制：中英混排都要有墨水
	fb := newTestFB(400, 60)
	fb.textExCore(mixed, 0, 0, 24, "Noto Sans CJK SC", 0x00FFFFFF, 0, 0, 0, 0, 0, 0, 0)
	if ink := inkCount(fb, 0, 0, 400, 60); ink < 100 {
		t.Fatalf("混排文本墨水过少: %d", ink)
	}
}
