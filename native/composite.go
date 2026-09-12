package main

import "encoding/binary"

// 复合字形标志位。
const (
	argWords     = 0x0001
	argsXY       = 0x0002
	haveScale    = 0x0008
	moreComp     = 0x0020
	haveXYScale  = 0x0040
	haveTwoByTwo = 0x0080
)

// parseCompositeGlyph 复合字形：递归组件（scale/offset 变换；point-matching 近似为 0 偏移）。
func (f *ttfFont) parseCompositeGlyph(gid uint32, depth int) ([][2]int, []bool, []int, bool) {
	if depth > 6 {
		return nil, nil, nil, false
	}
	a, b := f.glyphOffsets(gid)
	if b <= a || b > len(f.glyf) {
		return nil, nil, nil, false
	}
	g := f.glyf[a:b]
	if len(g) < 10 {
		return nil, nil, nil, false
	}
	i := 10
	var pts [][2]int
	var on []bool
	var ends []int
	for {
		if i+4 > len(g) {
			break
		}
		flags := int(binary.BigEndian.Uint16(g[i : i+2]))
		i += 2
		sub := uint32(binary.BigEndian.Uint16(g[i : i+2]))
		i += 2
		var a1, a2 int
		if flags&argWords != 0 {
			a1 = int(int16(binary.BigEndian.Uint16(g[i : i+2])))
			i += 2
			a2 = int(int16(binary.BigEndian.Uint16(g[i : i+2])))
			i += 2
		} else {
			a1 = int(int8(g[i]))
			i++
			a2 = int(int8(g[i]))
			i++
		}
		sx, sy, sxy, syx := 1.0, 1.0, 0.0, 0.0
		if flags&haveScale != 0 {
			v := f2dot14(g[i : i+2])
			sx, sy = v, v
			i += 2
		}
		if flags&haveXYScale != 0 {
			sx = f2dot14(g[i : i+2])
			i += 2
			sy = f2dot14(g[i : i+2])
			i += 2
		}
		if flags&haveTwoByTwo != 0 {
			sx = f2dot14(g[i : i+2])
			i += 2
			syx = f2dot14(g[i : i+2])
			i += 2
			sxy = f2dot14(g[i : i+2])
			i += 2
			sy = f2dot14(g[i : i+2])
			i += 2
		}
		ox, oy := 0, 0
		if flags&argsXY != 0 {
			ox, oy = a1, a2
		}
		// 取组件点集
		var cpts [][2]int
		var con []bool
		var cends []int
		ca, cb := f.glyphOffsets(sub)
		if cb > ca && cb <= len(f.glyf) && cb-ca >= 10 {
			cg := f.glyf[ca:cb]
			cc := int(int16(binary.BigEndian.Uint16(cg[0:2])))
			if cc < 0 {
				cpts, con, cends, _ = f.parseCompositeGlyph(sub, depth+1)
			} else {
				cpts, con, cends, _ = parseGlyphPts(cg, cc)
			}
		}
		base := len(pts)
		for k, p := range cpts {
			x := float64(p[0])
			y := float64(p[1])
			nx := sx*x + sxy*y + float64(ox)
			ny := syx*x + sy*y + float64(oy)
			pts = append(pts, [2]int{int(nx), int(ny)})
			on = append(on, con[k])
		}
		for _, e := range cends {
			ends = append(ends, base+e)
		}
		if flags&moreComp == 0 {
			break
		}
	}
	return pts, on, ends, true
}

// f2dot14 定点数（2.14）→ float64。
func f2dot14(b []byte) float64 {
	return float64(int16(binary.BigEndian.Uint16(b))) / 16384.0
}
