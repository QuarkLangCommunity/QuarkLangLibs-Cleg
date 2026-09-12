package main

import "testing"

func TestPureGoTTF(t *testing.T) {
	f, err := loadFont("Adwaita Sans", 48)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range []rune{'A', 'H', 'o'} {
		w, h, adv, a := f.rasterGlyph(ch, 48)
		nz := 0
		for _, v := range a {
			if v != 0 {
				nz++
			}
		}
		t.Logf("%c: w=%d h=%d adv=%d nz=%d", ch, w, h, adv, nz)
	}
}
