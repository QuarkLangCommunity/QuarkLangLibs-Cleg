package main

// 纯 Go TTF 光栅器（零外部依赖，跨系统：Linux/macOS/Windows 相同路径）。
// 支持：sfnt/glyf cmap（format 4 + 12）、二次贝塞尔轮廓 + 非零环绕扫描线填充、glyph 缓存。
// font 回退链：名字链（"DejaVu Sans, Consolas, monospace"）逐个系统字体路径尝试 → 内置 5x7 位图兜底。

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type ttfFont struct {
	data       []byte
	numGlyphs  int
	unitsPerEm int
	cmap4      []byte // 主 cmap 子表（format 4 或 12）
	cmap4Data  []byte
	cmapAlt    []byte // 次选 cmap 子表（主表未命中时回落）
	cmap4Sub   uint16
	locafmt    byte // 0=short 1=long（head.indexToLocFormat）
	loca       []byte
	locaLen    int
	glyf       []byte
	hmtx       []byte
	advanceW   uint16
	headOff    int
	name       string // 字体全名（nameID 4）
	family     string // 字体族名（nameID 1/16）
	path       string // 来源文件路径（调试/日志）
}

var ttfCacheMu sync.Mutex
var ttfCache = map[string]*ttfFont{}

func tag(b []byte, i int) string { return string(b[i : i+4]) }

// sfntFaceOffset 返回 sfnt 数据起点：普通 .ttf/.otf → 0；TTC/OTC（ttcf）→ 第 0 个 face 偏移。
func sfntFaceOffset(data []byte) (int, error) {
	if len(data) < 12 {
		return 0, os.ErrInvalid
	}
	if string(data[0:4]) != "ttcf" {
		return 0, nil
	}
	if len(data) < 16 {
		return 0, os.ErrInvalid
	}
	n := int(binary.BigEndian.Uint32(data[8:12]))
	if n <= 0 || len(data) < 12+4*n {
		return 0, os.ErrInvalid
	}
	base := int(binary.BigEndian.Uint32(data[12:16]))
	if base <= 0 || base+12 > len(data) {
		return 0, os.ErrInvalid
	}
	return base, nil
}

// parseTTF 解析 sfnt（.ttf/.otf）或 TrueType Collection（.ttc/.otc，取第 0 个 face）。
// TTC 的表偏移规范为"文件绝对"，个别字体为"相对 face"，用 head 魔数兜底二选一。
func parseTTF(data []byte) (*ttfFont, error) {
	base, err := sfntFaceOffset(data)
	if err != nil {
		return nil, err
	}
	f, err := parseTTFAt(data, base, false)
	if err == nil {
		return f, nil
	}
	if base > 0 {
		if f2, err2 := parseTTFAt(data, base, true); err2 == nil {
			return f2, nil
		}
	}
	return nil, err
}

func parseTTFAt(data []byte, base int, relOffsets bool) (*ttfFont, error) {
	if base < 0 || base+12 > len(data) {
		return nil, os.ErrInvalid
	}
	f := &ttfFont{data: data}
	numTables := int(binary.BigEndian.Uint16(data[base+4 : base+6]))
	if numTables <= 0 || base+12+16*numTables > len(data) {
		return nil, os.ErrInvalid
	}
	adj := func(off int) int {
		if relOffsets {
			return base + off
		}
		return off
	}
	var cmapOff, locaOff, glyfOff, hmtxOff, headOff, maxpOff, nameOff, hheaOff int
	var locaLen int
	for i := 0; i < numTables; i++ {
		rec := base + 12 + i*16
		switch tag(data, rec) {
		case "cmap":
			cmapOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		case "loca":
			locaOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
			locaLen = int(binary.BigEndian.Uint32(data[rec+12 : rec+16]))
		case "glyf":
			glyfOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		case "hmtx":
			hmtxOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		case "head":
			headOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		case "maxp":
			maxpOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		case "name":
			nameOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		case "hhea":
			hheaOff = adj(int(binary.BigEndian.Uint32(data[rec+8 : rec+12])))
		}
	}
	if cmapOff == 0 || glyfOff == 0 || locaOff == 0 || headOff == 0 || maxpOff == 0 || hmtxOff == 0 || hheaOff == 0 {
		return nil, os.ErrInvalid
	}
	// 结构校验（head 魔数 0x5F0F3CF5）：既防垃圾数据，也用于 TTC 偏移解释二选一。
	if headOff+54 > len(data) || binary.BigEndian.Uint32(data[headOff+12:headOff+16]) != 0x5F0F3CF5 {
		return nil, os.ErrInvalid
	}
	if maxpOff+6 > len(data) || cmapOff+4 > len(data) || glyfOff >= len(data) || locaOff >= len(data) || hmtxOff > len(data) {
		return nil, os.ErrInvalid
	}
	f.headOff = headOff
	f.numGlyphs = int(binary.BigEndian.Uint16(data[maxpOff+4 : maxpOff+6]))
	f.unitsPerEm = int(binary.BigEndian.Uint16(data[headOff+18 : headOff+20]))
	if locaLen >= (f.numGlyphs+1)*4 {
		f.locafmt = 1
	} else {
		f.locafmt = data[headOff+50] & 1
	}
	f.locaLen = locaLen
	f.loca = data[locaOff:]
	f.glyf = data[glyfOff:]
	f.hmtx = data[hmtxOff:]
	// cmap：选 format 4 或 12。旧实现把"编码 ID"当"平台 ID"比对（要求 encoding==3），
	// 于是 platform 3/enc 1（BMP）与 3/10（完整 Unicode）这类标准子表全被丢弃，
	// Droid Sans Fallback 等字体直接解析失败。现按平台优先 + format 优先排序。
	best := -1
	bestKey := 0
	alt := -1
	altKey := 0
	nSub := int(binary.BigEndian.Uint16(data[cmapOff+2 : cmapOff+4]))
	for i := 0; i < nSub; i++ {
		rec := cmapOff + 4 + i*8
		if rec+8 > len(data) {
			break
		}
		off := int(binary.BigEndian.Uint32(data[rec+4 : rec+8]))
		start := cmapOff + off
		if start+2 > len(data) {
			continue
		}
		format := int(binary.BigEndian.Uint16(data[start : start+2]))
		if format != 4 && format != 12 {
			continue
		}
		pid := binary.BigEndian.Uint16(data[rec : rec+2])
		eid := binary.BigEndian.Uint16(data[rec+2 : rec+4])
		score := 0
		switch {
		case pid == 3 && (eid == 10 || eid == 1):
			score = 2 // Windows Unicode（完整 / BMP）
		case pid == 0:
			score = 1 // Unicode 平台
		}
		if score == 0 {
			continue
		}
		key := score*100 + format
		if key > bestKey {
			altKey, alt = bestKey, best
			bestKey, best = key, start
		} else if key > altKey {
			altKey, alt = key, start
		}
	}
	if best < 0 {
		return nil, os.ErrInvalid
	}
	f.cmap4Data = data[best:]
	f.cmap4 = data[best:]
	if alt >= 0 {
		f.cmapAlt = data[alt:]
	}
	// 字体内部名（family/full；平台 3 为 UTF-16BE，需解码——否则族名匹配永远失配）
	if nameOff > 0 {
		f.family, f.name = ttfNames(data, nameOff)
	}
	return f, nil
}

// ttfDecodeName name 表字符串解码：平台 3/0 为 UTF-16BE，其它按 Latin-1 直读。
func ttfDecodeName(b []byte, pid uint16) string {
	if pid != 3 && pid != 0 {
		return string(b)
	}
	var sb strings.Builder
	for i := 0; i+1 < len(b); i += 2 {
		r := rune(binary.BigEndian.Uint16(b[i : i+2]))
		if r == 0 {
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

// ttfNames 取字体内部名：family（nameID 1，排版族名 16 优先）与 full（nameID 4）。
func ttfNames(data []byte, nameOff int) (family, full string) {
	if nameOff <= 0 || nameOff+6 > len(data) {
		return "", ""
	}
	n := int(binary.BigEndian.Uint16(data[nameOff+2 : nameOff+4]))
	strOff := nameOff + int(binary.BigEndian.Uint16(data[nameOff+4:nameOff+6]))
	for i := 0; i < n; i++ {
		rec := nameOff + 6 + i*12
		if rec+12 > len(data) {
			break
		}
		pid := binary.BigEndian.Uint16(data[rec : rec+2])
		nameID := binary.BigEndian.Uint16(data[rec+6 : rec+8])
		l := int(binary.BigEndian.Uint16(data[rec+8 : rec+10]))
		off := int(binary.BigEndian.Uint16(data[rec+10 : rec+12]))
		if l == 0 || strOff+off+l > len(data) {
			continue
		}
		s := ttfDecodeName(data[strOff+off:strOff+off+l], pid)
		if s == "" {
			continue
		}
		switch nameID {
		case 1:
			if family == "" {
				family = s
			}
		case 16:
			family = s // 排版族名优先
		case 4:
			if full == "" {
				full = s
			}
		}
	}
	return family, full
}

// glyphIndex cmap 查字符→glyph：主选子表未命中时回落次选子表。
// 有的字体（如 Droid Sans Fallback）format 12 子表只覆盖 CJK 稀疏段，
// ASCII 只在 format 4 子表里——只认一个子表会得到 .notdef（渲染成空心方框）。
func (f *ttfFont) glyphIndex(ch rune) uint32 {
	if gid := glyphIndexIn(f.cmap4, ch); gid != 0 {
		return gid
	}
	if len(f.cmapAlt) > 0 {
		return glyphIndexIn(f.cmapAlt, ch)
	}
	return 0
}

func glyphIndexIn(d []byte, ch rune) uint32 {
	if len(d) < 4 {
		return 0
	}
	switch binary.BigEndian.Uint16(d[:2]) {
	case 12:
		return glyphIndex12In(d, uint32(ch))
	case 4:
		if ch > 0xFFFF {
			return 0
		}
		return glyphIndex4In(d, uint16(ch))
	}
	return 0
}

func glyphIndex4In(d []byte, ch uint16) uint32 {
	if len(d) < 14 {
		return 0
	}
	segCount := int(binary.BigEndian.Uint16(d[6:8])) / 2
	// 序：endCode(segCount*2), pad2, startCode, idDelta, idRangeOffset, glyphIdArray
	base := 14
	startBase := base + segCount*2 + 2
	deltaBase := startBase + segCount*2
	roBase := deltaBase + segCount*2
	for i := 0; i < segCount; i++ {
		end := binary.BigEndian.Uint16(d[base+i*2 : base+i*2+2])
		start := binary.BigEndian.Uint16(d[startBase+i*2 : startBase+i*2+2])
		if ch >= start && ch <= end {
			delta := binary.BigEndian.Uint16(d[deltaBase+i*2 : deltaBase+i*2+2])
			ro := binary.BigEndian.Uint16(d[roBase+i*2 : roBase+i*2+2])
			if ro == 0 {
				return uint32(uint16(int(ch) + int(int16(delta))))
			}
			idx := roBase + i*2 + int(ro) + int(ch-start)*2
			if idx+2 > len(d) {
				return 0
			}
			g := binary.BigEndian.Uint16(d[idx : idx+2])
			if g == 0 {
				return 0
			}
			return uint32(uint16(int(g) + int(int16(delta))))
		}
	}
	return 0
}

func glyphIndex12In(d []byte, ch uint32) uint32 {
	if len(d) < 16 {
		return 0
	}
	n := int(binary.BigEndian.Uint32(d[12:16]))
	for i := 0; i < n; i++ {
		rec := 16 + i*12
		if rec+12 > len(d) {
			return 0
		}
		start := binary.BigEndian.Uint32(d[rec : rec+4])
		end := binary.BigEndian.Uint32(d[rec+4 : rec+8])
		if ch >= start && ch <= end {
			gid := binary.BigEndian.Uint32(d[rec+8 : rec+12])
			return gid + (ch - start)
		}
	}
	return 0
}

// glyphOffsets loca 取 glyph 数据区间。
func (f *ttfFont) glyphOffsets(gid uint32) (int, int) {
	if f.locafmt == 1 {
		o := int(gid) * 4
		if o+8 > len(f.loca) {
			return 0, 0
		}
		a := int(binary.BigEndian.Uint32(f.loca[o : o+4]))
		b := int(binary.BigEndian.Uint32(f.loca[o+4 : o+8]))
		return a, b
	}
	o := int(gid) * 2
	if o+4 > len(f.loca) {
		return 0, 0
	}
	a := int(binary.BigEndian.Uint16(f.loca[o:o+2])) * 2
	b := int(binary.BigEndian.Uint16(f.loca[o+2:o+4])) * 2
	return a, b
}

// rasterGlyph 光栅化字符：返回 w/h/advance/alpha（packed 字节行，已用 px 缩放）。
func (f *ttfFont) rasterGlyph(ch rune, px int) (w, h, adv int, alpha []byte) {
	gid := f.glyphIndex(ch)
	if gid >= uint32(f.numGlyphs) {
		return 0, 0, 0, nil
	}
	a, b := f.glyphOffsets(gid)
	if b <= a || b > len(f.glyf) {
		return 0, 0, 0, nil
	}
	g := f.glyf[a:b]
	if len(g) < 10 {
		return 0, 0, 0, nil
	}
	xMin := int(int16(binary.BigEndian.Uint16(g[2:4])))
	yMin := int(int16(binary.BigEndian.Uint16(g[4:6])))
	xMax := int(int16(binary.BigEndian.Uint16(g[6:8])))
	yMax := int(int16(binary.BigEndian.Uint16(g[8:10])))
	scale := float64(px) / float64(f.unitsPerEm)
	w = int((float64(xMax-xMin) * scale) + 0.5)
	h = int((float64(yMax-yMin) * scale) + 0.5)
	if w <= 0 || h <= 0 {
		if f.advanceW > 0 {
			adv = int(float64(f.advanceW) * scale)
		}
		return
	}
	contours := int(int16(binary.BigEndian.Uint16(g[0:2])))
	// 点位解析（复合字形递归）
	var pts [][2]int
	var flags []bool
	var endPts []int
	var err error
	if contours < 0 {
		pts, flags, endPts, _ = f.parseCompositeGlyph(gid, 0)
		for i := range pts {
			if pts[i][0] < xMin {
				xMin = pts[i][0]
			}
			if pts[i][1] < yMin {
				yMin = pts[i][1]
			}
		}
	} else {
		pts, flags, endPts, err = parseGlyphPts(g, contours)
	}
	if err != nil {
		return 0, 0, 0, nil
	}
	// y 翻转（字形 y-up → 屏幕 y-down）
	for i := range pts {
		pts[i][1] = yMax - pts[i][1]
	}
	yMin = 0
	// 轮廓折线（二次贝塞尔扁平化）
	polys := outlinePolys(pts, flags, endPts, scale, xMin, yMin)
	// 扫描线填充
	alpha = fillPolys(polys, w, h, scale)
	// advance：hmtx 表（gid 项）
	if int(gid)*4+2 <= len(f.hmtx) {
		adv = int(float64(binary.BigEndian.Uint16(f.hmtx[int(gid)*4:int(gid)*4+2])) * scale)
	}
	if adv <= 0 {
		adv = w + int(scale*120)
	}
	return w, h, adv, alpha
}

// parseGlyphPts 解析简单字形的点序列（flags 重复位）。
func parseGlyphPts(g []byte, contours int) ([][2]int, []bool, []int, error) {
	i := 10
	endPts := make([]int, contours)
	for c := 0; c < contours; c++ {
		if i+2 > len(g) {
			return nil, nil, nil, os.ErrInvalid
		}
		endPts[c] = int(binary.BigEndian.Uint16(g[i : i+2]))
		i += 2
	}
	nPts := 0
	if contours > 0 {
		nPts = endPts[contours-1] + 1
	}
	if i+2 <= len(g) {
		// instructionLength 是"指令字节数"，跳过 2 + ins 字节（旧实现误乘 2，
		// 导致 flags 起点整体偏移、坐标流全错）。
		ins := int(binary.BigEndian.Uint16(g[i : i+2]))
		i += 2 + ins
	}
	// flags 段只出现一次（repeat 扩展）；x 与 y 数据随后各自按 flags 解码。
	// REPEAT_FLAG = 0x08：置位时"紧随其后的一字节"是重复次数 count，
	// 该 flag 共出现 count+1 次（旧实现误按低 3 位/位移取次数，
	// 使 flag 流与 x/y 字节流失步，大量字形轮廓变空）。
	flags := make([]byte, nPts)
	for p := 0; p < nPts; {
		if i >= len(g) {
			break
		}
		f := g[i]
		i++
		rep := 0
		if f&0x08 != 0 {
			if i >= len(g) {
				break
			}
			rep = int(g[i])
			i++
		}
		flags[p] = f
		p++
		for k := 0; k < rep && p < nPts; k++ {
			flags[p] = f
			p++
		}
	}
	on := make([]bool, nPts)
	for p := 0; p < nPts; p++ {
		on[p] = flags[p]&1 == 1
	}
	// x 数据
	xc := make([]int, nPts)
	v := 0
	for p := 0; p < nPts; p++ {
		f := flags[p]
		var dx int
		if i >= len(g) {
			break
		}
		if f&2 != 0 {
			dx = int(g[i])
			i++
			if f&16 == 0 {
				dx = -dx
			}
		} else if f&16 != 0 {
			dx = 0 // X_SAME：与上一点相同（无字节）
		} else {
			if i+2 > len(g) {
				break
			}
			dx = int(int16(binary.BigEndian.Uint16(g[i : i+2])))
			i += 2
		}
		v += dx
		xc[p] = v
	}
	// y 数据（复用同一 flags）
	yc := make([]int, nPts)
	v = 0
	for p := 0; p < nPts; p++ {
		f := flags[p]
		var dy int
		if i >= len(g) {
			break
		}
		if f&4 != 0 {
			dy = int(g[i])
			i++
			if f&32 == 0 {
				dy = -dy
			}
		} else if f&32 != 0 {
			dy = 0 // Y_SAME：与上一点相同（无字节）
		} else {
			if i+2 > len(g) {
				break
			}
			dy = int(int16(binary.BigEndian.Uint16(g[i : i+2])))
			i += 2
		}
		v += dy
		yc[p] = v
	}
	pts := make([][2]int, nPts)
	for p := 0; p < nPts; p++ {
		pts[p] = [2]int{xc[p], yc[p]}
	}
	return pts, on, endPts, nil
}

// outlinePolys 按轮廓终点列表切分，逐轮廓扁平化（二次贝塞尔 8 段取样）。
func outlinePolys(pts [][2]int, flags []bool, endPts []int, scale float64, xMin, yMin int) [][][4]float64 {
	var polys [][][4]float64
	prevEnd := -1
	for _, ep := range endPts {
		if ep >= len(pts) {
			break
		}
		seg := flattenContour(pts, flags, prevEnd+1, ep, scale, xMin, yMin)
		if len(seg) > 2 {
			polys = append(polys, seg)
		}
		prevEnd = ep
	}
	return polys
}

// flattenContour 单个轮廓 → 折线（二次贝塞尔按 8 段扁平化）。
// TrueType 允许连续 off-curve 点：两点之间隐含一个 on-curve 中点，
// 必须以"当前点 cur"贯穿推进。旧实现遇到连续 off-curve 时把下一段的起点
// 取成轮廓起点（seq[0]），导致轮廓自交、字腔被填死或笔画错乱。
func flattenContour(pts [][2]int, flags []bool, start, end int, scale float64, xMin, yMin int) [][4]float64 {
	n := end - start + 1
	if n < 2 {
		return nil
	}
	at := func(k int) [2]int { return pts[start+((k%n)+n)%n] }
	on := func(k int) bool { return flags[start+((k%n)+n)%n] }

	// 起点 cur：首点 on → 用它；否则末点 on → 用末点；否则用首末中点。
	var cur [2]int
	k0 := 0
	switch {
	case on(0):
		cur = at(0)
		k0 = 1
	case on(n - 1):
		cur = at(n - 1)
	default:
		a, b := at(n-1), at(0)
		cur = [2]int{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
	}
	first := cur

	var poly [][4]float64
	poly = append(poly, toF(cur, scale, xMin, yMin))
	pending := false
	var ctrl [2]int
	for done, k := 0, k0; done < n; done, k = done+1, k+1 {
		p := at(k)
		if on(k) {
			if pending {
				poly = append(poly, quadBez(cur, ctrl, p, scale, xMin, yMin)...)
				pending = false
			} else {
				poly = append(poly, toF(p, scale, xMin, yMin))
			}
			cur = p
			continue
		}
		if pending { // 连续 off-curve：以中点收段，并把中点作为下一段起点
			mid := [2]int{(ctrl[0] + p[0]) / 2, (ctrl[1] + p[1]) / 2}
			poly = append(poly, quadBez(cur, ctrl, mid, scale, xMin, yMin)...)
			cur = mid
		}
		ctrl = p
		pending = true
	}
	if pending { // 闭合回起点
		poly = append(poly, quadBez(cur, ctrl, first, scale, xMin, yMin)...)
	}
	return poly
}

func toF(p [2]int, scale float64, xMin, yMin int) [4]float64 {
	return [4]float64{(float64(p[0]-xMin) * scale), (float64(p[1]-yMin) * scale), 0, 0}
}

// quadBez 二次贝塞尔（p0, ctrl, p2）→ 8 段折线。
func quadBez(p0, ctrl, p2 [2]int, scale float64, xMin, yMin int) [][4]float64 {
	out := make([][4]float64, 0, 9)
	const n = 8
	for k := 1; k <= n; k++ {
		t := float64(k) / float64(n)
		mt := 1 - t
		x := mt*mt*float64(p0[0]) + 2*mt*t*float64(ctrl[0]) + t*t*float64(p2[0])
		y := mt*mt*float64(p0[1]) + 2*mt*t*float64(ctrl[1]) + t*t*float64(p2[1])
		out = append(out, [4]float64{(x - float64(xMin)) * scale, (y - float64(yMin)) * scale, 0, 0})
	}
	return out
}

// fillPolys 扫描线非零环绕填充（poly=[x,y] 折线列）。
func fillPolys(polys [][][4]float64, w, h int, scale float64) []byte {
	alpha := make([]byte, w*h)
	for y := 0; y < h; y++ {
		yy := float64(y) + 0.5
		var xs []float64
		for _, poly := range polys {
			n := len(poly)
			for i := 0; i < n; i++ {
				p1 := poly[i]
				p2 := poly[(i+1)%n]
				if (p1[1] <= yy && p2[1] > yy) || (p2[1] <= yy && p1[1] > yy) {
					if !(p2[1] == p1[1]) {
						t := (yy - p1[1]) / (p2[1] - p1[1])
						xs = append(xs, p1[0]+t*(p2[0]-p1[0]))
					}
				}
			}
		}
		// 简单排序（交叉数少，插入排序）
		for i := 1; i < len(xs); i++ {
			for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
				xs[j], xs[j-1] = xs[j-1], xs[j]
			}
		}
		// 奇偶填充 + 水平抗锯齿：像素 x 覆盖 [x,x+1)，覆盖率 = 区间与像素的交集长度。
		// 旧实现取 int(a-0.5)..int(b+0.5) 两端各外扩约 1px，会把窄字腔（如 o/m 的内孔）
		// 整个吃掉，且完全没有抗锯齿。
		for i := 0; i+1 < len(xs); i += 2 {
			a, b := xs[i], xs[i+1]
			if b <= a {
				continue
			}
			x0 := int(math.Floor(a))
			x1 := int(math.Floor(b))
			for x := x0; x <= x1; x++ {
				if x < 0 || x >= w || y < 0 || y >= h {
					continue
				}
				cov := math.Min(b, float64(x+1)) - math.Max(a, float64(x))
				if cov <= 0 {
					continue
				}
				v := byte(0)
				if cov >= 1 {
					v = 255
				} else {
					v = byte(cov*255 + 0.5)
				}
				if v > alpha[y*w+x] {
					alpha[y*w+x] = v
				}
			}
		}
	}
	return alpha
}

// fontPathsOf 字名→候选文件路径（跨系统扫描：linux /usr/share/fonts + /usr/local/share/fonts；macOS /System/Library/Fonts + /Library/Fonts；Windows C:\\Windows\\Fonts）。
var fontRoots = func() []string {
	var r []string
	if os.PathSeparator == '/' {
		home := os.Getenv("HOME")
		r = []string{"/usr/share/fonts", "/usr/local/share/fonts",
			filepath.Join(home, ".fonts"), filepath.Join(home, ".local", "share", "fonts")}
		if _, err := os.Stat("/System/Library/Fonts"); err == nil {
			r = append(r, "/System/Library/Fonts", "/Library/Fonts")
		}
	} else {
		wd := os.Getenv("WINDIR")
		if wd == "" {
			wd = "C:\\Windows"
		}
		r = []string{filepath.Join(wd, "Fonts")}
	}
	return r
}()

var fontIndexOnce sync.Once
var fontIndex []string // 所有候选字体路径（启动时递归扫描一次并缓存）

// fontIndexAll 递归扫描系统字体目录，收集 .ttf/.otf/.ttc/.otc（零系统依赖，纯文件系统扫描）。
func fontIndexAll() []string {
	fontIndexOnce.Do(func() {
		for _, root := range fontRoots {
			_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
				if err == nil && !info.IsDir() {
					ext := strings.ToLower(filepath.Ext(p))
					if ext == ".ttf" || ext == ".otf" || ext == ".ttc" || ext == ".otc" {
						fontIndex = append(fontIndex, p)
					}
				}
				return nil
			})
		}
	})
	return fontIndex
}

// sniffGlyf 只读文件头判断字体是否含 glyf（TrueType 轮廓）。
// .otf/.ttc 中大量是 CFF/CFF2 轮廓（本光栅器不支持），先嗅探可避免为解析失败整文件读取。
func sniffGlyf(path string) bool {
	fh, err := os.Open(path)
	if err != nil {
		return false
	}
	defer fh.Close()
	buf := make([]byte, 8192)
	n, _ := fh.ReadAt(buf, 0)
	if n < 12 {
		return false
	}
	buf = buf[:n]
	base := 0
	if string(buf[0:4]) == "ttcf" {
		if n < 16 {
			return false
		}
		base = int(binary.BigEndian.Uint32(buf[12:16]))
		if base <= 0 || base+12 > n {
			return false
		}
	}
	numTables := int(binary.BigEndian.Uint16(buf[base+4 : base+6]))
	if numTables <= 0 || base+12+16*numTables > n {
		return false
	}
	for i := 0; i < numTables; i++ {
		if string(buf[base+12+i*16:base+12+i*16+4]) == "glyf" {
			return true
		}
	}
	return false
}

// loadFont 加载字名（回退链单项）：文件名匹配 →（CJK 名）候选内部族名匹配 → CJK 候选链。
func loadFont(name string, px int) (*ttfFont, error) {
	name = normalizeFontName(name)
	ttfCacheMu.Lock()
	if f, ok := ttfCache[name+"|"+itoa(px)]; ok {
		ttfCacheMu.Unlock()
		return f, nil
	}
	ttfCacheMu.Unlock()
	if name == "" {
		return nil, os.ErrNotExist
	}
	// 0) 通用族名（QSS/CSS 的 monospace/sans-serif/serif…）→ 具体族名候选。
	//    否则 qk 侧默认链 "monospace" 会一路落到内置 5x7 位图（该表有空洞，表现为丢字）。
	if alts, ok := genericFontAliases[name]; ok {
		for _, alt := range alts {
			if f, err := loadFont(alt, px); err == nil {
				return f, nil
			}
		}
	}
	// 1) 文件名匹配（既有行为 + 风格偏好：未显式要求 bold/italic 时优先 Regular/Book，
	//    否则同一族会随机命中 Bold/Black/Italic 变体，观感与度量都不对）
	if !isCJKName(name) {
		bestPath, bestPenalty := "", 1<<30
		for _, p := range fontIndexAll() {
			base := normalizeFontName(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
			if base == "" {
				continue
			}
			if !strings.Contains(base, name) && !strings.Contains(name, base) {
				continue
			}
			if pen := fontStylePenalty(name, base); pen < bestPenalty {
				bestPenalty, bestPath = pen, p
			}
		}
		if bestPath != "" {
			if f, err := loadFontFile(bestPath, px); err == nil {
				return f, nil
			}
		}
		return nil, os.ErrNotExist
	}
	// 2) CJK 族名：候选集很小，逐个比对内部族名/全名（.ttc 文件名常与族名不一致）
	for _, p := range cjkFontPaths() {
		if f, err := loadFontFile(p, px); err == nil && fontNameMatches(f, name) {
			return f, nil
		}
	}
	// 3) 仍无 → 取任意可用 CJK 字体，保证 CJK 文本可渲染
	if f := loadCJKFont(px); f != nil {
		return f, nil
	}
	return nil, os.ErrNotExist
}

// fontStylePenalty 文件名风格词惩罚：请求名里没写的风格词会加惩罚，
// 从而在未指定 bold/italic 时优先命中 Regular/Book（数值越小越好）。
func fontStylePenalty(req, base string) int {
	p := 0
	for _, w := range []string{"bold", "black", "heavy", "semibold", "demibold", "medium", "light", "thin"} {
		if strings.Contains(base, w) && !strings.Contains(req, w) {
			p += 4
		}
	}
	for _, w := range []string{"italic", "oblique"} {
		if strings.Contains(base, w) && !strings.Contains(req, w) {
			p += 3
		}
	}
	for _, w := range []string{"condensed", "narrow", "expanded"} {
		if strings.Contains(base, w) && !strings.Contains(req, w) {
			p += 2
		}
	}
	if strings.Contains(base, "regular") || strings.Contains(base, "book") {
		p--
	}
	return p
}

// genericFontAliases 通用族名（规范化后）→ 具体族名候选（按优先级）。
var genericFontAliases = map[string][]string{
	"monospace": {"adwaitamono", "notosansmono", "dejavusansmono", "liberationmono", "freesansmono", "couriernew", "mono"},
	"sansserif": {"adwaitasans", "notosans", "dejavusans", "liberationsans", "freesans", "droidsans", "arial", "helvetica"},
	"sans":      {"adwaitasans", "notosans", "dejavusans", "liberationsans", "freesans", "droidsans", "arial", "helvetica"},
	"systemui":  {"adwaitasans", "notosans", "dejavusans", "liberationsans", "freesans", "droidsans"},
	"default":   {"adwaitasans", "notosans", "dejavusans", "liberationsans", "freesans", "droidsans"},
	"serif":     {"notoserif", "dejavuserif", "liberationserif", "freeserif", "timesnewroman", "adwaitasans", "notosans"},
	"cursive":   {"freesans", "notosans", "dejavusans"},
	"fantasy":   {"freesans", "notosans", "dejavusans"},
}

// fontNameMatches 请求名（已规范化）与字体内部族名/全名互含匹配。
func fontNameMatches(f *ttfFont, norm string) bool {
	if f == nil || norm == "" {
		return false
	}
	for _, n := range []string{f.family, f.name} {
		nn := normalizeFontName(n)
		if nn == "" {
			continue
		}
		if strings.Contains(nn, norm) || strings.Contains(norm, nn) {
			return true
		}
	}
	return false
}

// fontPathOf 字名→命中路径（回退链单项探测）。
func fontPathOf(name string) string {
	name = normalizeFontName(name)
	for _, p := range fontIndexAll() {
		base := normalizeFontName(strings.TrimSuffix(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), ""))
		if strings.Contains(base, name) || strings.Contains(name, base) {
			return p
		}
	}
	return ""
}

// normalizeFontName 字体名规范化（去大小写/空格/连字符）。
func normalizeFontName(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if r == ' ' || r == '-' || r == '_' || r == ',' {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ---------- CJK 字体发现（Style v2：CJK 必须命中系统字体，而不是退化成 5x7 '?'） ----------

// cjkFontKeywords 命中关键字（小写；比较前已去空格/连字符/下划线）。
var cjkFontKeywords = []string{
	"notosanscjk", "notosansmonocjk", "sourcehan", "wenquanyi", "wqy",
	"droidsansfallback", "microsoftyahei", "msyh", "pingfang", "simhei", "simsun", "msung", "cjk",
}

// cjkKeywordScore 关键字优先级（0 = 不命中）。含完整 CJK 的 glyf 字体优先；CFF 会在嗅探阶段跳过。
func cjkKeywordScore(norm string) int {
	hit := false
	for _, k := range cjkFontKeywords {
		if strings.Contains(norm, k) {
			hit = true
			break
		}
	}
	if !hit {
		return 0
	}
	switch {
	case strings.Contains(norm, "droidsansfallback"), strings.Contains(norm, "wenquanyi"),
		strings.Contains(norm, "wqy"), strings.Contains(norm, "microsoftyahei"),
		strings.Contains(norm, "msyh"), strings.Contains(norm, "pingfang"),
		strings.Contains(norm, "simhei"), strings.Contains(norm, "simsun"):
		return 3
	case strings.Contains(norm, "notosanscjk"), strings.Contains(norm, "notosansmonocjk"),
		strings.Contains(norm, "sourcehan"):
		return 2
	}
	return 1
}

// isCJKName 请求的族名是否属于 CJK 关键字族。
func isCJKName(norm string) bool { return cjkKeywordScore(norm) > 0 }

var (
	cjkPathsOnce sync.Once
	cjkPaths     []string
)

// cjkFontPaths CJK 候选路径（关键字命中，按优先级排序；扫描一次后缓存）。
func cjkFontPaths() []string {
	cjkPathsOnce.Do(func() {
		type cand struct {
			path  string
			score int
		}
		var cs []cand
		for _, p := range fontIndexAll() {
			base := normalizeFontName(strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)))
			if s := cjkKeywordScore(base); s > 0 {
				cs = append(cs, cand{p, s})
			}
		}
		sort.SliceStable(cs, func(i, j int) bool { return cs[i].score > cs[j].score })
		for _, c := range cs {
			cjkPaths = append(cjkPaths, c.path)
		}
	})
	return cjkPaths
}

var (
	cjkMu       sync.Mutex
	cjkResolved string
	cjkRejected = map[string]bool{}
)

// loadCJKFont 按优先级取一个"可解析且含 CJK 字形"的字体；结果缓存，进程内只完整探测一次。
// 依次跳过：CFF/CFF2 轮廓（无 glyf）、解析失败、无 CJK 字形。
func loadCJKFont(px int) *ttfFont {
	cjkMu.Lock()
	done := cjkResolved
	cjkMu.Unlock()
	if done != "" {
		if f, err := loadFontFile(done, px); err == nil {
			return f
		}
		return nil
	}
	for _, p := range cjkFontPaths() {
		cjkMu.Lock()
		bad := cjkRejected[p]
		cjkMu.Unlock()
		if bad {
			continue
		}
		if !sniffGlyf(p) {
			cjkMu.Lock()
			cjkRejected[p] = true
			cjkMu.Unlock()
			continue
		}
		f, err := loadFontFile(p, px)
		if err != nil || !f.coversCJK() {
			cjkMu.Lock()
			cjkRejected[p] = true
			cjkMu.Unlock()
			continue
		}
		cjkMu.Lock()
		cjkResolved = p
		cjkMu.Unlock()
		return f
	}
	return nil
}

// coversCJK 是否含常用 CJK 字形（CJK 回退判定）。
func (f *ttfFont) coversCJK() bool {
	if f == nil {
		return false
	}
	for _, r := range []rune{'中', '文', '日', '한', 'あ'} {
		if gid := f.glyphIndex(r); gid != 0 && gid < uint32(f.numGlyphs) {
			return true
		}
	}
	return false
}

const defaultFontChain = "monospace"
