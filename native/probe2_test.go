package main

import (
	"encoding/binary"
	"testing"
)

func TestPointDecode(t *testing.T) {
	f, _ := loadFont("Adwaita Sans", 48)
	gid := f.glyphIndex('A')
	a, b := f.glyphOffsets(gid)
	g := f.glyf[a:b]
	contours := int(int16(binary.BigEndian.Uint16(g[0:2])))
	pts, flags, endPts, _ := parseGlyphPts(g, contours)
	t.Logf("contours=%d npts=%d endPts=%v", contours, len(pts), endPts)
	for i := 0; i < 8 && i < len(pts); i++ {
		t.Logf("p%d=(%d,%d) on=%v", i, pts[i][0], pts[i][1], flags[i])
	}
	xMin := int(int16(binary.BigEndian.Uint16(g[2:4])))
	yMin := int(int16(binary.BigEndian.Uint16(g[4:6])))
	xMax := int(int16(binary.BigEndian.Uint16(g[6:8])))
	yMax := int(int16(binary.BigEndian.Uint16(g[8:10])))
	t.Logf("bbox=(%d,%d,%d,%d) uem=%d", xMin, yMin, xMax, yMax, f.unitsPerEm)
}
