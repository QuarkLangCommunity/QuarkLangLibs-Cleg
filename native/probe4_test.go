package main

import (
	"encoding/binary"
	"testing"
)

func TestOffsets(t *testing.T) {
	data, err := osReadFile(fontOf("Adwaita Sans"))
	if err != nil {
		t.Skip(err)
	}
	n := int(binary.BigEndian.Uint16(data[4:6]))
	t.Logf("numTables=%d", n)
	for i := 0; i < n; i++ {
		rec := 12 + i*16
		tag := string(data[rec : rec+4])
		off := binary.BigEndian.Uint32(data[rec+8 : rec+12])
		ln := binary.BigEndian.Uint32(data[rec+12 : rec+16])
		t.Logf("%s off=%d len=%d", tag, off, ln)
	}
}
