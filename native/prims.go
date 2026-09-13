package main

// cleg Style v2 渲染原语内核（纯 Go，零系统依赖）。
//
// 本文件实现 STYLE-SPEC §9 冻结签名背后的绘制内核：
//   - alpha 混合与裁剪感知的逐像素写入；
//   - 圆角矩形解析覆盖（1px 抗锯齿带，角圆距离场）；
//   - 线性渐变（角度/枚举 dir）+ 径向渐变；
//   - 四边不同宽度/颜色的边框（solid/dashed/dotted，圆角轮廓）；
//   - 外阴影（多层递减 alpha 近似高斯）+ 内阴影（圆角盒 SDF 过渡带）；
//   - 裁剪栈（clip push/pop 嵌套）与基础变换（平移 + 有理缩放）。
//
// 像素格式沿用 v1 帧缓冲：线性 []uint32，值为 0x00RRGGBB（高 8 位不存 alpha），
// 逐像素 source-over 混合到 RGB；savePNG 时统一补 alpha=0xFF。
//
// alpha 约定（qk 侧颜色常为 "r,g,b" 无 A，故按原语做兼容推断）：
//   - cleg_blend_rect：argb 的 A==0 视为 255，透明度由独立 alpha 参数控制；
//   - cleg_gradient / cleg_radial：两端 A 同时为 0 → 视为不透明；否则 A 按原义
//     （支持一端 0x00 表示渐隐到透明）；
//   - cleg_shadow / cleg_border / cleg_text_ex：A==0 视为 255。
//
// 数值近似策略：颜色按通道整数线性插值；圆角覆盖用 (r+0.5-d) 线性带；
// 阴影 blur 不实现真实高斯，用 N 层外扩形状 + 透射率配平的多层 alpha。

import "math"

// ---------- alpha 与逐像素写入 ----------

// alphaChannelOr 取 0xAARRGGBB 的 A 通道；A==0 时返回 def（兼容无 alpha 的颜色）。
func alphaChannelOr(argb uint32, def int) int {
	a := int(argb >> 24 & 0xFF)
	if a == 0 {
		return def
	}
	return a
}

func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// mulAlpha 两个 0..255 alpha 相乘。
func mulAlpha(a, b int) int { return a * b / 255 }

// putPixel 裁剪感知的逐像素混合写入：c 为 0xAARRGGBB（只用低 24 位），a 为 0..255。
func (fb *framebuffer) putPixel(x, y int, c uint32, a int) {
	if a <= 0 || x < fb.clipX0 || x >= fb.clipX1 || y < fb.clipY0 || y >= fb.clipY1 {
		return
	}
	p := &fb.buf[y*fb.w+x]
	if a >= 255 {
		*p = c & 0x00FFFFFF
		return
	}
	blendPixel(p, c, a)
}

// blendSpan 单行整数区间（半开 [x0,x1)）恒定颜色混合（rect 快速路径）。
func (fb *framebuffer) blendSpan(y, x0, x1 int, c uint32, a int) {
	if a <= 0 {
		return
	}
	if x0 < fb.clipX0 {
		x0 = fb.clipX0
	}
	if x1 > fb.clipX1 {
		x1 = fb.clipX1
	}
	if x1 <= x0 || y < fb.clipY0 || y >= fb.clipY1 {
		return
	}
	row := y * fb.w
	c &= 0x00FFFFFF
	if a >= 255 {
		for x := x0; x < x1; x++ {
			fb.buf[row+x] = c
		}
		return
	}
	for x := x0; x < x1; x++ {
		blendPixel(&fb.buf[row+x], c, a)
	}
}

// ---------- 圆角几何 ----------

// roundRectRowSpan 返回行 yy 在圆角矩形内的覆盖 x 区间（浮点，半开），无覆盖时 ok=false。
// 中间带整行实体；上下圆角带按角圆方程取半宽。供逐行遍历限定扫描范围。
func roundRectRowSpan(yy, x, y, w, h, r int) (x0, x1 float64, ok bool) {
	if w <= 0 || h <= 0 || yy < y || yy >= y+h {
		return 0, 0, false
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	if r <= 0 {
		return float64(x), float64(x + w), true
	}
	if yy >= y+r && yy < y+h-r {
		return float64(x), float64(x + w), true
	}
	rf := float64(r)
	cy := float64(y + r)
	if yy >= y+h-r {
		cy = float64(y + h - r)
	}
	dy := float64(yy) + 0.5 - cy
	d2 := rf*rf - dy*dy
	if d2 < 0 {
		d2 = 0
	}
	half := math.Sqrt(d2)
	return float64(x+r) - half, float64(x+w-r) + half, true
}

// roundRectCovPixel 圆角矩形在像素中心 (px+0.5, py+0.5) 的覆盖度 0..1。
// 直边：整数边界 + 像素中心 → 0/1（与 fillRect 一致）；圆角：到角圆心距离解析覆盖
// （r+0.5-d 的 1px 线性抗锯齿带）。内部区域双比较快速返回 1。
func roundRectCovPixel(px, py, x, y, w, h, r int) float64 {
	if w <= 0 || h <= 0 {
		return 0
	}
	fx := float64(px) + 0.5
	fy := float64(py) + 0.5
	x0 := float64(x)
	y0 := float64(y)
	x1 := float64(x + w)
	y1 := float64(y + h)
	if fx < x0 || fx > x1 || fy < y0 || fy > y1 {
		return 0
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	if r <= 0 {
		return 1
	}
	rf := float64(r)
	if fx >= x0+rf && fx <= x1-rf {
		return 1
	}
	if fy >= y0+rf && fy <= y1-rf {
		return 1
	}
	cx := x0 + rf
	if fx > x1-rf {
		cx = x1 - rf
	}
	cy := y0 + rf
	if fy > y1-rf {
		cy = y1 - rf
	}
	dx := fx - cx
	dy := fy - cy
	d := math.Sqrt(dx*dx + dy*dy)
	cov := rf + 0.5 - d
	if cov <= 0 {
		return 0
	}
	if cov >= 1 {
		return 1
	}
	return cov
}

// blendRoundRect 圆角矩形 alpha 混合填充（圆角解析抗锯齿；直边走整数快速路径）。
func (fb *framebuffer) blendRoundRect(x, y, w, h, r int, c uint32, a int) {
	if w <= 0 || h <= 0 || a <= 0 {
		return
	}
	if r > w/2 {
		r = w / 2
	}
	if r > h/2 {
		r = h / 2
	}
	if r <= 0 {
		for yy := y; yy < y+h; yy++ {
			fb.blendSpan(yy, x, x+w, c, a)
		}
		return
	}
	// 中间带整行实体 → 整数区间快速路径；仅上下圆角带的两侧角区逐像素求覆盖。
	for yy := y; yy < y+h; yy++ {
		if yy < fb.clipY0 || yy >= fb.clipY1 {
			continue
		}
		if yy >= y+r && yy < y+h-r {
			fb.blendSpan(yy, x, x+w, c, a)
			continue
		}
		fb.blendSpan(yy, x+r, x+w-r, c, a)
		fb.blendCorner(yy, x, x+r, x, y, w, h, r, c, a)
		fb.blendCorner(yy, x+w-r, x+w, x, y, w, h, r, c, a)
	}
}

// blendCorner 圆角带行的角区逐像素覆盖混合（[xa,xb) 为角方块的水平范围）。
func (fb *framebuffer) blendCorner(yy, xa, xb, x, y, w, h, r int, c uint32, a int) {
	for xx := xa; xx < xb; xx++ {
		cov := roundRectCovPixel(xx, yy, x, y, w, h, r)
		if cov <= 0 {
			continue
		}
		av := a
		if cov < 1 {
			av = int(float64(a)*cov + 0.5)
		}
		fb.putPixel(xx, yy, c, av)
	}
}

// ---------- 变换 ----------

// xfReset 复位为单位变换。
func (fb *framebuffer) xfReset() {
	fb.xfDX, fb.xfDY = 0, 0
	fb.xfNumX, fb.xfDenX = 1, 1
	fb.xfNumY, fb.xfDenY = 1, 1
}

// normRatio 归一化有理缩放（分母 0 → 1；符号归一到分子；num==0&&den==0 → 1:1）。
func normRatio(num, den int) (int, int) {
	if den == 0 {
		if num == 0 {
			return 1, 1
		}
		den = 1
	}
	if den < 0 {
		num, den = -num, -den
	}
	return num, den
}

func divRound(a, b int) int {
	if b <= 0 {
		return a
	}
	if a >= 0 {
		return (a + b/2) / b
	}
	return -((-a + b/2) / b)
}

// xfPoint 逻辑点 → 设备点： device = p*scale + translate。
func (fb *framebuffer) xfPoint(x, y int) (int, int) {
	return divRound(x*fb.xfNumX, fb.xfDenX) + fb.xfDX, divRound(y*fb.xfNumY, fb.xfDenY) + fb.xfDY
}

// xfRect 逻辑矩形 → 设备矩形（宽高按同轴缩放，就近取整，负值截 0）。
func (fb *framebuffer) xfRect(x, y, w, h int) (int, int, int, int) {
	dx, dy := fb.xfPoint(x, y)
	dw := divRound(w*fb.xfNumX, fb.xfDenX)
	dh := divRound(h*fb.xfNumY, fb.xfDenY)
	if dw < 0 {
		dw = 0
	}
	if dh < 0 {
		dh = 0
	}
	return dx, dy, dw, dh
}

// xfLenY 纵向长度缩放（字号）。
func (fb *framebuffer) xfLenY(v int) int {
	if v <= 0 {
		return v
	}
	n := divRound(v*fb.xfNumY, fb.xfDenY)
	if n < 1 {
		n = 1
	}
	return n
}

// xfLenX 横向长度缩放（边距/字距）。
func (fb *framebuffer) xfLenX(v int) int {
	return divRound(v*fb.xfNumX, fb.xfDenX)
}

// xfSet 设置绝对变换 device = p*scale + translate（cleg_transform）。
// sx/sy 为有理数；分母 0 视为 1；sy 缺省（sy_num==sy_den==0）时与 sx 相同；
// 全 0 调用（0,0,0,0,0,0）→ 单位变换。
func (fb *framebuffer) xfSet(dx, dy, sxNum, sxDen, syNum, syDen int) {
	nx, ndx := normRatio(sxNum, sxDen)
	ny, ndy := normRatio(syNum, syDen)
	if syNum == 0 && syDen == 0 {
		ny, ndy = nx, ndx
	}
	fb.xfDX, fb.xfDY = dx, dy
	fb.xfNumX, fb.xfDenX = nx, ndx
	fb.xfNumY, fb.xfDenY = ny, ndy
}

// ---------- 裁剪栈 ----------

const clipStackMax = 1024

// clipPush 入栈并收窄裁剪区（入参为逻辑坐标，先过当前变换）。
func (fb *framebuffer) clipPush(x, y, w, h int) int {
	x, y, w, h = fb.xfRect(x, y, w, h)
	if len(fb.clipStack) >= clipStackMax {
		return -1
	}
	x0, y0 := x, y
	x1, y1 := x+w, y+h
	if x0 < fb.clipX0 {
		x0 = fb.clipX0
	}
	if y0 < fb.clipY0 {
		y0 = fb.clipY0
	}
	if x1 > fb.clipX1 {
		x1 = fb.clipX1
	}
	if y1 > fb.clipY1 {
		y1 = fb.clipY1
	}
	if x1 < x0 {
		x1 = x0
	}
	if y1 < y0 {
		y1 = y0
	}
	fb.clipStack = append(fb.clipStack, [4]int{fb.clipX0, fb.clipY0, fb.clipX1, fb.clipY1})
	fb.clipX0, fb.clipY0, fb.clipX1, fb.clipY1 = x0, y0, x1, y1
	return 0
}

// clipPop 出栈恢复上一层裁剪区；栈空返回 -1（无副作用）。
func (fb *framebuffer) clipPop() int {
	n := len(fb.clipStack)
	if n == 0 {
		return -1
	}
	last := fb.clipStack[n-1]
	fb.clipStack = fb.clipStack[:n-1]
	fb.clipX0, fb.clipY0, fb.clipX1, fb.clipY1 = last[0], last[1], last[2], last[3]
	return 0
}

// ---------- 颜色辅助 ----------

// lerpChan 通道线性插值（t 已 clamp）。
func lerpChan(a, b int, t float64) int {
	v := float64(a) + (float64(b)-float64(a))*t
	if v <= 0 {
		return 0
	}
	if v >= 255 {
		return 255
	}
	return int(v + 0.5)
}

// gradPairAlpha 渐变两端 alpha：两端 A 同时为 0 → 视为不透明（兼容 "r,g,b" 颜色），
// 否则按原义（允许一端 0x00 表示透明）。
func gradPairAlpha(c1, c2 uint32) (int, int) {
	a1 := int(c1 >> 24 & 0xFF)
	a2 := int(c2 >> 24 & 0xFF)
	if a1 == 0 && a2 == 0 {
		return 255, 255
	}
	return a1, a2
}

// ---------- 矩形混合 / 渐变 ----------

// blendRectCore cleg_blend_rect 内核。
func (fb *framebuffer) blendRectCore(x, y, w, h int, argb uint32, alpha, radius int) {
	x, y, w, h = fb.xfRect(x, y, w, h)
	a := mulAlpha(clamp255(alpha), alphaChannelOr(argb, 255))
	fb.blendRoundRect(x, y, w, h, fb.xfLenX(radius), argb, a)
}

// gradientAxis 渐变轴：返回单位方向 (ux,uy) 与四角投影的最小/最大值。
// dir 语义：0..3 为枚举（0=左→右 1=上→下 2=右→左 3=下→上）；
// 其它值视为角度（度，屏幕坐标 y 向下：0=左→右 90=上→下 180=右→左 270=下→上）。
func gradientAxis(dir, x, y, w, h int) (ux, uy, pmin, pmax float64) {
	var ang float64
	switch dir {
	case 0:
		ang = 0
	case 1:
		ang = 90
	case 2:
		ang = 180
	case 3:
		ang = 270
	default:
		ang = float64(dir)
	}
	rad := ang * math.Pi / 180
	ux, uy = math.Cos(rad), math.Sin(rad)
	corners := [4][2]float64{
		{float64(x), float64(y)},
		{float64(x + w), float64(y)},
		{float64(x), float64(y + h)},
		{float64(x + w), float64(y + h)},
	}
	pmin, pmax = math.Inf(1), math.Inf(-1)
	for _, c := range corners {
		p := c[0]*ux + c[1]*uy
		if p < pmin {
			pmin = p
		}
		if p > pmax {
			pmax = p
		}
	}
	return ux, uy, pmin, pmax
}

// gradientCore cleg_gradient 内核：线性 2 色 + 圆角裁剪。
func (fb *framebuffer) gradientCore(x, y, w, h, dir int, c1, c2 uint32, radius int) {
	if w <= 0 || h <= 0 {
		return
	}
	x, y, w, h = fb.xfRect(x, y, w, h)
	radius = fb.xfLenX(radius)
	if w <= 0 || h <= 0 {
		return
	}
	ux, uy, pmin, pmax := gradientAxis(dir, x, y, w, h)
	span := pmax - pmin
	if span <= 0 {
		return
	}
	a1, a2 := gradPairAlpha(c1, c2)
	r1, g1, b1 := int(c1>>16&0xFF), int(c1>>8&0xFF), int(c1&0xFF)
	r2, g2, b2 := int(c2>>16&0xFF), int(c2>>8&0xFF), int(c2&0xFF)
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}
	for yy := y; yy < y+h; yy++ {
		if yy < fb.clipY0 || yy >= fb.clipY1 {
			continue
		}
		x0, x1, ok := roundRectRowSpan(yy, x, y, w, h, radius)
		if !ok {
			continue
		}
		xa := int(math.Floor(x0))
		xb := int(math.Ceil(x1))
		if xa < fb.clipX0 {
			xa = fb.clipX0
		}
		if xb > fb.clipX1 {
			xb = fb.clipX1
		}
		fy := float64(yy) + 0.5
		for xx := xa; xx < xb; xx++ {
			cov := 1.0
			if radius > 0 {
				cov = roundRectCovPixel(xx, yy, x, y, w, h, radius)
				if cov <= 0 {
					continue
				}
			}
			fx := float64(xx) + 0.5
			t := ((fx*ux + fy*uy) - pmin) / span
			if t < 0 {
				t = 0
			} else if t > 1 {
				t = 1
			}
			col := uint32(lerpChan(r1, r2, t))<<16 | uint32(lerpChan(g1, g2, t))<<8 | uint32(lerpChan(b1, b2, t))
			a := int(float64(lerpChan(a1, a2, t))*cov + 0.5)
			fb.putPixel(xx, yy, col, a)
		}
	}
}

// radialCore cleg_radial 内核：中心 c1 → 边缘 c2，边缘 1px 抗锯齿。
func (fb *framebuffer) radialCore(cx, cy, r int, c1, c2 uint32) {
	if r <= 0 {
		return
	}
	cx, cy = fb.xfPoint(cx, cy)
	rr := fb.xfLenX(r)
	if rr <= 0 {
		return
	}
	a1, a2 := gradPairAlpha(c1, c2)
	r1, g1, b1 := int(c1>>16&0xFF), int(c1>>8&0xFF), int(c1&0xFF)
	r2, g2, b2 := int(c2>>16&0xFF), int(c2>>8&0xFF), int(c2&0xFF)
	rf := float64(rr)
	for yy := cy - rr; yy <= cy+rr; yy++ {
		if yy < fb.clipY0 || yy >= fb.clipY1 {
			continue
		}
		for xx := cx - rr; xx <= cx+rr; xx++ {
			if xx < fb.clipX0 || xx >= fb.clipX1 {
				continue
			}
			dx := float64(xx) + 0.5 - float64(cx)
			dy := float64(yy) + 0.5 - float64(cy)
			d := math.Sqrt(dx*dx + dy*dy)
			edge := rf + 0.5 - d
			if edge <= 0 {
				continue
			}
			if edge > 1 {
				edge = 1
			}
			t := d / rf
			if t > 1 {
				t = 1
			}
			col := uint32(lerpChan(r1, r2, t))<<16 | uint32(lerpChan(g1, g2, t))<<8 | uint32(lerpChan(b1, b2, t))
			a := int(float64(lerpChan(a1, a2, t))*edge + 0.5)
			fb.putPixel(xx, yy, col, a)
		}
	}
}

// ---------- 边框 ----------

// borderDashOn 虚线/点线模式：按沿边位置 pos 与边宽 width 判定实/空。
// dashed：实 3*width（>=4）/ 空 2*width（>=2）；dotted：实 width / 空 2*width。
func borderDashOn(style, pos, width int) bool {
	if style <= 0 {
		return true
	}
	if width < 1 {
		width = 1
	}
	var on, gap int
	if style == 1 {
		on = width * 3
		if on < 4 {
			on = 4
		}
		gap = width * 2
		if gap < 2 {
			gap = 2
		}
	} else {
		on = width
		gap = width * 2
		if gap < 2 {
			gap = 2
		}
	}
	period := on + gap
	if period <= 0 {
		return true
	}
	if pos < 0 {
		pos = -pos
	}
	return pos%period < on
}

// borderCore cleg_border 内核：四边不同宽度/颜色 + 圆角 + solid/dashed/dotted。
// 角归属：上/下带优先（与 QSS 四矩形拼接一致）；内轮廓半径按平均边框宽内缩近似。
func (fb *framebuffer) borderCore(x, y, w, h, wt, wr, wb, wl int, ct, cr, cb, cl uint32, radius, style int) {
	if w <= 0 || h <= 0 {
		return
	}
	x, y, w, h = fb.xfRect(x, y, w, h)
	if w <= 0 || h <= 0 {
		return
	}
	if wt < 0 {
		wt = 0
	}
	if wr < 0 {
		wr = 0
	}
	if wb < 0 {
		wb = 0
	}
	if wl < 0 {
		wl = 0
	}
	if wl+wr > w {
		wl = w / 2
		wr = w - wl
	}
	if wt+wb > h {
		wt = h / 2
		wb = h - wt
	}
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}
	if radius < 0 {
		radius = 0
	}
	ix, iy := x+wl, y+wt
	iw, ih := w-wl-wr, h-wt-wb
	innerR := radius - (wt+wb+wl+wr)/4
	if innerR < 0 {
		innerR = 0
	}
	if iw > 0 && innerR > iw/2 {
		innerR = iw / 2
	}
	if ih > 0 && innerR > ih/2 {
		innerR = ih / 2
	}
	for yy := y; yy < y+h; yy++ {
		if yy < fb.clipY0 || yy >= fb.clipY1 {
			continue
		}
		ox0, ox1, ok := roundRectRowSpan(yy, x, y, w, h, radius)
		if !ok {
			continue
		}
		type span struct{ a, b float64 }
		spans := [2]span{{ox0, ox1}}
		ns := 1
		if iw > 0 && ih > 0 {
			if ix0, ix1, okIn := roundRectRowSpan(yy, ix, iy, iw, ih, innerR); okIn {
				spans[0] = span{ox0, ix0}
				spans[1] = span{ix1, ox1}
				ns = 2
			}
		}
		for s := 0; s < ns; s++ {
			sp := spans[s]
			if sp.b <= sp.a {
				continue
			}
			xa := int(math.Floor(sp.a))
			xb := int(math.Ceil(sp.b))
			if xa < fb.clipX0 {
				xa = fb.clipX0
			}
			if xb > fb.clipX1 {
				xb = fb.clipX1
			}
			for xx := xa; xx < xb; xx++ {
				cov := roundRectCovPixel(xx, yy, x, y, w, h, radius)
				if cov <= 0 {
					continue
				}
				if iw > 0 && ih > 0 {
					cov -= roundRectCovPixel(xx, yy, ix, iy, iw, ih, innerR)
					if cov <= 0 {
						continue
					}
				}
				var col uint32
				var pos, sideW int
				switch {
				case yy < y+wt:
					col, pos, sideW = ct, xx-x, wt
				case yy >= y+h-wb:
					col, pos, sideW = cb, xx-x, wb
				case xx < x+wl:
					col, pos, sideW = cl, yy-y, wl
				case xx >= x+w-wr:
					col, pos, sideW = cr, yy-y, wr
				default:
					continue
				}
				if !borderDashOn(style, pos, sideW) {
					continue
				}
				a := int(float64(alphaChannelOr(col, 255))*cov + 0.5)
				if a <= 0 {
					continue
				}
				fb.putPixel(xx, yy, col, a)
			}
		}
	}
}

// ---------- 阴影 ----------

// sdRoundBox 圆角盒有符号距离（<0 在内，>0 在外）。
func sdRoundBox(px, py, cx, cy, hw, hh, r float64) float64 {
	if r > hw {
		r = hw
	}
	if r > hh {
		r = hh
	}
	if r < 0 {
		r = 0
	}
	qx := math.Abs(px-cx) - (hw - r)
	qy := math.Abs(py-cy) - (hh - r)
	ax, ay := qx, qy
	if ax < 0 {
		ax = 0
	}
	if ay < 0 {
		ay = 0
	}
	m := qx
	if qy > m {
		m = qy
	}
	if m > 0 {
		m = 0
	}
	return math.Sqrt(ax*ax+ay*ay) + m - r
}

// shadowCore cleg_shadow 内核。
func (fb *framebuffer) shadowCore(x, y, w, h, dx, dy, blur, spread int, argb uint32, inset, radius int) {
	base := alphaChannelOr(argb, 255)
	if base <= 0 || w <= 0 || h <= 0 {
		return
	}
	x, y, w, h = fb.xfRect(x, y, w, h)
	if w <= 0 || h <= 0 {
		return
	}
	// 变换：dx/dy 平移按轴缩放，blur/spread/radius 按横向缩放（各向同性近似）。
	dx = fb.xfLenX(dx)
	dy = fb.xfLenY(dy)
	blur = fb.xfLenX(blur)
	spread = fb.xfLenX(spread)
	radius = fb.xfLenX(radius)
	if inset != 0 {
		fb.insetShadowCore(x, y, w, h, dx, dy, blur, spread, argb, radius)
		return
	}
	// 外阴影：形状 = 本体平移 (dx,dy) 并外扩 spread；blur 用 N 层外扩形状递增近似高斯。
	// 每层 alpha 由透射率配平：Π(1-a_i/255) = 1-base/255（核心区精确等于 base，
	// 边缘自然衰减；b→1 时钳到 0.999 以避免多层全不透明退化为硬边）。
	sx := x + dx - spread
	sy := y + dy - spread
	sw := w + 2*spread
	sh := h + 2*spread
	sr := radius + spread
	if sr < 0 {
		sr = 0
	}
	if sw <= 0 || sh <= 0 {
		return
	}
	if blur < 0 {
		blur = -blur
	}
	n := blur
	if n < 1 {
		n = 1
	}
	if n > 12 {
		n = 12
	}
	b := float64(base) / 255
	if b > 0.999 {
		b = 0.999
	}
	sum := float64(n * (n + 1) / 2)
	for i := n - 1; i >= 0; i-- {
		grow := i * blur / n
		wgt := float64(n-i) / sum
		ai := int(255*(1-math.Pow(1-b, wgt)) + 0.5)
		if ai <= 0 {
			continue
		}
		if ai > 255 {
			ai = 255
		}
		fb.blendRoundRect(sx-grow, sy-grow, sw+2*grow, sh+2*grow, sr+grow, argb, ai)
	}
}

// insetShadowCore 内阴影：外框内、按 (dx,dy) 平移且内缩 spread 的"洞"之外为阴影，
// blur 为洞边界两侧的线性过渡带宽度（SDF 近似，一次遍历）。
func (fb *framebuffer) insetShadowCore(x, y, w, h, dx, dy, blur, spread int, argb uint32, radius int) {
	base := alphaChannelOr(argb, 255)
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}
	if radius < 0 {
		radius = 0
	}
	hx := x + dx + spread
	hy := y + dy + spread
	hw := w - 2*spread
	hh := h - 2*spread
	hr := radius - spread
	if hr < 0 {
		hr = 0
	}
	if hw < 1 || hh < 1 {
		fb.blendRoundRect(x, y, w, h, radius, argb, base)
		return
	}
	edge := float64(blur)
	if edge < 1 {
		edge = 1
	}
	halfW := float64(hw) / 2
	halfH := float64(hh) / 2
	ccx := float64(hx) + halfW
	ccy := float64(hy) + halfH
	for yy := y; yy < y+h; yy++ {
		if yy < fb.clipY0 || yy >= fb.clipY1 {
			continue
		}
		for xx := x; xx < x+w; xx++ {
			if xx < fb.clipX0 || xx >= fb.clipX1 {
				continue
			}
			cov := roundRectCovPixel(xx, yy, x, y, w, h, radius)
			if cov <= 0 {
				continue
			}
			sd := sdRoundBox(float64(xx)+0.5, float64(yy)+0.5, ccx, ccy, halfW, halfH, float64(hr))
			f := 0.5 + sd/edge
			if f <= 0 {
				continue
			}
			if f > 1 {
				f = 1
			}
			a := int(float64(base)*f*cov + 0.5)
			if a <= 0 {
				continue
			}
			fb.putPixel(xx, yy, argb, a)
		}
	}
}
