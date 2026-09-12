package main

import (
	"encoding/binary"
	"testing"
)

func TestCmapLoca(t *testing.T) {
	f, _ := loadFont("Adwaita Sans", 48)
	gid := f.glyphIndex('A')
	t.Logf("gid('A')=%d numGlyphs=%d locafmt=%d uem=%d", gid, f.numGlyphs, f.locafmt, f.unitsPerEm)
	for g := uint32(0); g < 5; g++ {
		a, b := f.glyphOffsets(g)
		t.Logf("gid %d -> [%d,%d)", g, a, b)
	}
	if len(f.glyf) >= 4 {
		t.Logf("glyf head: %x", f.glyf[:4])
	}
	t.Logf("loca len=%d glyf len=%d", len(f.loca), len(f.glyf))
	// cmap 格式
	t.Logf("cmap fmt=%d sublen=%d", binary.BigEndian.Uint16(f.cmap4[:2]), len(f.cmap4))
}
