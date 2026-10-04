// SPDX-License-Identifier: GPL-3.0-or-later
package brotherql

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// Hiding concrete types exercises the generic image.Image conversion path.
type genericImage struct{ image.Image }

func TestPixelSpecializationsPreserveAlpha(t *testing.T) {
	bounds := image.Rect(7, 13, 263, 269)
	nrgba := image.NewNRGBA(bounds)
	rgba := image.NewRGBA(bounds)
	gray := image.NewGray(bounds)
	for a := 0; a < 256; a++ {
		for v := 0; v < 256; v++ {
			c := color.NRGBA{uint8(v), uint8(255 - v), uint8(v / 2), uint8(a)}
			nrgba.SetNRGBA(v+7, a+13, c)
			rgba.Set(v+7, a+13, c)
			gray.SetGray(v+7, a+13, color.Gray{uint8(v)})
		}
	}
	for _, src := range []image.Image{nrgba, rgba, gray} {
		fast, reference := pixelReader(src), pixelReader(genericImage{src})
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				r, g, b := fast(x, y)
				rr, gg, bb := reference(x, y)
				if r != rr || g != gg || b != bb {
					t.Fatalf("%T (%d,%d): got %d/%d/%d; want %d/%d/%d", src, x, y, r, g, b, rr, gg, bb)
				}
			}
		}
	}
}

func TestSquare23MediaUsesManualPrintableWidth(t *testing.T) {
	// Brother QL-800 raster manual pp.15,20: 236 head pins, 202 rows, right margin42.
	im := solid(236, 202, color.Black)
	got, err := Preview(im, Options{Model: "QL-800", Label: "23x23", Rotate: "0"})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		x     int
		black bool
	}{{441, false}, {442, true}, {677, true}, {678, false}} {
		r, _, _, _ := got.At(tc.x, 0).RGBA()
		if (r == 0) != tc.black {
			t.Errorf("x=%d black=%v, expected %v", tc.x, r == 0, tc.black)
		}
	}
}

func TestRoundDieCutHasZeroFeed(t *testing.T) {
	var out bytes.Buffer
	if err := Encode(&out, []image.Image{solid(94, 94, color.White)}, Options{Model: "QL-800", Label: "d12"}); err != nil {
		t.Fatal(err)
	}
	// Brother manual: only zero is valid for die-cut margin command ESC i d.
	if !bytes.Contains(out.Bytes(), []byte{0x1b, 0x69, 0x64, 0, 0}) {
		t.Fatal("round die-cut emitted a nonzero feed")
	}
}
