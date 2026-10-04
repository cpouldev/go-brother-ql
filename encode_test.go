// SPDX-License-Identifier: GPL-3.0-or-later
package brotherql

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"io"
	"math"
	"os"
	"testing"
)

func solid(w, h int, c color.Color) *image.NRGBA {
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			im.Set(x, y, c)
		}
	}
	return im
}

func TestValidationRejectsUnsupportedOptions(t *testing.T) {
	bad := []Options{
		{Model: "QL-800", Label: "102x51"}, {Model: "QL-800", Label: "62", Compress: true},
		{Model: "QL-700", Label: "62red"}, {Model: "QL-800", Label: "29", Red: true},
		{Model: "missing", Label: "62"}, {Model: "QL-800", Label: "missing"},
		{Model: "QL-800", Label: "62", Rotate: "45"}, {Model: "QL-800", Label: "62", Fit: "crop"},
		{Model: "QL-800", Label: "62", Cut: "every", CutEvery: 0},
		{Model: "QL-800", Label: "62", Cut: "every", CutEvery: 256},
	}
	for _, o := range bad {
		t.Run(o.Model+"/"+o.Label+"/"+o.Rotate+o.Fit+o.Cut, func(t *testing.T) {
			if _, err := ValidateOptions(o); err == nil {
				t.Fatalf("accepted %+v", o)
			}
		})
	}
	for _, v := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		o := Options{Model: "QL-800", Label: "62", Threshold: &v}
		if _, err := ValidateOptions(o); err == nil {
			t.Errorf("accepted threshold %v", v)
		}
	}
}

func TestRedMediaEnablesColor(t *testing.T) {
	o, err := ValidateOptions(Options{Model: "QL-800", Label: "62red"})
	if err != nil || !o.Red {
		t.Fatalf("%+v %v", o, err)
	}
	o, err = ValidateOptions(Options{Model: "QL-800", Label: "62", Red: true})
	if err != nil || o.Label != "62red" {
		t.Fatalf("%+v %v", o, err)
	}
}

func TestRedMediaRejectsHighResolution(t *testing.T) {
	if _, err := ValidateOptions(Options{Model: "QL-800", Label: "62red", DPI600: true}); err == nil {
		t.Fatal("accepted unsupported high-resolution two-color mode")
	}
}

func TestHighResolutionRequiresExplicitCapability(t *testing.T) {
	for _, model := range []string{"QL-500", "QL-550", "QL-560", "QL-650TD", "QL-1050", "QL-1060N", "QL-1100", "QL-1110NWB", "QL-1115NWB"} {
		if _, err := ValidateOptions(Options{Model: model, Label: "62", DPI600: true}); err == nil {
			t.Errorf("%s accepted unverified high resolution", model)
		}
	}
	for _, model := range []string{"QL-570", "QL-580N", "QL-700", "QL-710W", "QL-720NW", "QL-800", "QL-810W", "QL-820NWB"} {
		if _, err := ValidateOptions(Options{Model: model, Label: "62", DPI600: true}); err != nil {
			t.Errorf("%s: %v", model, err)
		}
	}
}

func TestLegacyCutCapabilities(t *testing.T) {
	for _, model := range []string{"QL-550", "QL-650TD"} {
		if _, err := ValidateOptions(Options{Model: model, Label: "62", Cut: "every", CutEvery: 2}); err == nil {
			t.Errorf("%s accepted unsupported cut interval", model)
		}
		var out bytes.Buffer
		if err := Encode(&out, []image.Image{solid(696, 295, color.White)}, Options{Model: model, Label: "62"}); err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(out.Bytes(), []byte{0x1b, 0x69, 0x41}) {
			t.Errorf("%s emitted unsupported cut interval", model)
		}
		if model == "QL-550" && bytes.Contains(out.Bytes(), []byte{0x1b, 0x69, 0x4b}) {
			t.Error("QL-550 emitted unsupported expanded mode")
		}
	}
	if _, err := ValidateOptions(Options{Model: "QL-550", Label: "62", Cut: "end"}); err == nil {
		t.Fatal("QL-550 accepted unsupported end-cut mode")
	}
}

func TestQL800EnablesStatusNotifications(t *testing.T) {
	var b bytes.Buffer
	if err := Encode(&b, []image.Image{solid(696, 150, color.White)}, Options{Model: "QL-800", Label: "62"}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Bytes()[:440], []byte{0x1b, 0x69, 0x21, 0}) {
		t.Fatal("automatic status notifications not enabled")
	}
}

func TestDitherChangesGrayscale(t *testing.T) {
	im := solid(696, 150, color.Gray{128})
	got, err := Preview(im, Options{Model: "QL-800", Label: "62", Dither: true})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for y := 0; y < 150; y++ {
		for x := 12; x < 708; x++ {
			r, _, _, _ := got.At(x, y).RGBA()
			if r == 0 {
				count++
			}
		}
	}
	if count < 50000 || count > 54000 {
		t.Fatalf("expected approximately half black pixels; got %d", count)
	}
}

func TestHighResolutionPreservesPhysicalSize(t *testing.T) {
	im := solid(1392, 300, color.White)
	got, err := Preview(im, Options{Model: "QL-800", Label: "62", DPI600: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 720, 300) {
		t.Fatal(got.Bounds())
	}
}

func TestContainFitsDieCut(t *testing.T) {
	got, err := Preview(solid(100, 100, color.Black), Options{Model: "QL-800", Label: "17x54", Fit: "contain"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := got.At(555, 0).RGBA(); r != 65535 {
		t.Fatal("contain lost top padding")
	}
	if r, _, _, _ := got.At(555, 283).RGBA(); r != 0 {
		t.Fatal("contain lost center content")
	}
}

func TestPreviewNonZeroOrigin(t *testing.T) {
	im := image.NewNRGBA(image.Rect(20, 30, 716, 180))
	for y := 30; y < 180; y++ {
		for x := 20; x < 716; x++ {
			im.Set(x, y, color.White)
		}
	}
	im.Set(20, 30, color.Black)
	got, err := Preview(im, Options{Model: "QL-800", Label: "62"})
	if err != nil {
		t.Fatal(err)
	}
	if r, _, _, _ := got.At(12, 0).RGBA(); r != 0 {
		t.Fatal("non-zero origin shifted image")
	}
}

func TestReferenceMonochrome(t *testing.T) {
	im := solid(696, 150, color.White)
	for y := 0; y < 150; y++ {
		for x := 0; x < 696; x++ {
			if (x/13+y/7)%2 == 0 {
				im.Set(x, y, color.Black)
			}
		}
	}
	var got bytes.Buffer
	if err := Encode(&got, []image.Image{im}, Options{Model: "QL-800", Label: "62"}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/reference/ql800-62-checker.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("reference differs: got %d bytes, want %d", got.Len(), len(want))
	}
}

func TestReferenceColorAndWideHead(t *testing.T) {
	for _, tc := range []struct {
		model, label, file string
		width, height      int
		red                bool
	}{
		{"QL-800", "62red", "ql800-62red-checker.bin", 696, 150, true},
		{"QL-1100", "102", "ql1100-102-checker.bin", 1164, 301, false},
	} {
		t.Run(tc.model+"/"+tc.label, func(t *testing.T) {
			colors := []color.Color{color.White, color.Black}
			if tc.red {
				colors = append(colors, color.NRGBA{R: 255, A: 255})
			}
			im := image.NewNRGBA(image.Rect(0, 0, tc.width, tc.height))
			for y := 0; y < tc.height; y++ {
				for x := 0; x < tc.width; x++ {
					im.Set(x, y, colors[(x/13+y/7)%len(colors)])
				}
			}
			var got bytes.Buffer
			if err := Encode(&got, []image.Image{im}, Options{Model: tc.model, Label: tc.label}); err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile("testdata/reference/" + tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got.Bytes(), want) {
				t.Fatalf("reference differs: got %d bytes, want %d", got.Len(), len(want))
			}
		})
	}
}

func TestPageBoundaries(t *testing.T) {
	var b bytes.Buffer
	enc, err := NewEncoder(&b, Options{Model: "QL-800", Label: "62", Cut: "end"})
	if err != nil {
		t.Fatal(err)
	}
	im := solid(696, 150, color.White)
	if err = enc.WritePage(im, false); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), b.Bytes()...)
	if first[len(first)-1] != 0x0c {
		t.Fatal("intermediate page missing FF")
	}
	if err = enc.WritePage(im, true); err != nil {
		t.Fatal(err)
	}
	second := b.Bytes()[len(first):]
	if second[len(second)-1] != 0x1a {
		t.Fatal("final page missing SUB")
	}
	for i, p := range [][]byte{first, second} {
		j := bytes.Index(p, []byte{0x1b, 0x69, 0x7a})
		if j < 0 || p[j+11] != byte(i) {
			t.Fatalf("page %d has wrong first-page flag", i)
		}
	}
	if !bytes.Contains(first, []byte{0x1b, 0x69, 0x4d, 0}) {
		t.Fatal("cut-at-end must disable per-label autocut")
	}
	if err = enc.WritePage(im, true); err == nil {
		t.Fatal("accepted page after final page")
	}
}

func TestQualityAffectsProtocol(t *testing.T) {
	var high, low bytes.Buffer
	im := solid(696, 150, color.White)
	if err := Encode(&high, []image.Image{im}, Options{Model: "QL-800", Label: "62"}); err != nil {
		t.Fatal(err)
	}
	if err := Encode(&low, []image.Image{im}, Options{Model: "QL-800", Label: "62", LowQuality: true}); err != nil {
		t.Fatal(err)
	}
	idx := bytes.Index(high.Bytes(), []byte{0x1b, 0x69, 0x7a}) + 3
	if high.Bytes()[idx]^low.Bytes()[idx] != 0x40 {
		t.Fatal("quality flag did not change")
	}
}

func TestContinuousLengthBoundaries(t *testing.T) {
	for _, h := range []int{149, 150, 151, 11810, 11811, 11812} {
		t.Run(string(rune(h)), func(t *testing.T) {
			// Uniform avoids allocating the deliberately invalid input.
			im := &boundedUniform{Rectangle: image.Rect(0, 0, 696, h), C: color.White}
			_, err := Preview(im, Options{Model: "QL-800", Label: "62"})
			if (err == nil) != (h >= 150 && h <= 11811) {
				t.Fatalf("height %d: %v", h, err)
			}
		})
	}
}

type boundedUniform struct {
	image.Rectangle
	C color.Color
}

func (i *boundedUniform) ColorModel() color.Model { return color.NRGBAModel }
func (i *boundedUniform) Bounds() image.Rectangle { return i.Rectangle }
func (i *boundedUniform) At(x, y int) color.Color { return i.C }

func TestPreviewPreservesTransparentWhite(t *testing.T) {
	im := solid(696, 150, color.NRGBA{A: 0})
	got, err := Preview(im, Options{Model: "QL-800", Label: "62"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 720, 150) {
		t.Fatal(got.Bounds())
	}
	if color.NRGBAModel.Convert(got.At(12, 0)) != (color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("transparent background printed black")
	}
}

func TestPreviewRedPlanes(t *testing.T) {
	im := solid(696, 150, color.White)
	im.Set(0, 0, color.NRGBA{255, 0, 0, 255})
	im.Set(1, 0, color.Black)
	got, err := Preview(im, Options{Model: "QL-800", Label: "62red"})
	if err != nil {
		t.Fatal(err)
	}
	if color.NRGBAModel.Convert(got.At(12, 0)) != (color.NRGBA{255, 0, 0, 255}) {
		t.Fatal("red pixel lost")
	}
	if color.NRGBAModel.Convert(got.At(13, 0)) != (color.NRGBA{0, 0, 0, 255}) {
		t.Fatal("black pixel lost")
	}
}

func TestDieCutRotation(t *testing.T) {
	im := solid(566, 165, color.White)
	im.Set(0, 0, color.Black)
	got, err := Preview(im, Options{Model: "QL-800", Label: "17x54"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Bounds() != image.Rect(0, 0, 720, 566) {
		t.Fatal(got.Bounds())
	}
	if color.NRGBAModel.Convert(got.At(555, 565)) != (color.NRGBA{0, 0, 0, 255}) {
		t.Fatal("rotation or offset wrong")
	}
}

func TestValidationPrecedesOutput(t *testing.T) {
	var b bytes.Buffer
	e, err := NewEncoder(&b, Options{Model: "QL-800", Label: "17x54"})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.WritePage(solid(2, 2, color.Black), true); err == nil {
		t.Fatal("accepted wrong die-cut size")
	}
	if b.Len() != 0 {
		t.Fatal("wrote before validating page")
	}
}

var errWrite = errors.New("write failed")

type badWriter struct{}

func (badWriter) Write(p []byte) (int, error) { return 0, errWrite }
func TestEncodePropagatesWriteError(t *testing.T) {
	err := Encode(badWriter{}, []image.Image{solid(696, 150, color.White)}, Options{Model: "QL-800", Label: "62"})
	if !errors.Is(err, errWrite) {
		t.Fatal(err)
	}
}

type shortWriter struct{ bytes.Buffer }

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > 3 {
		p = p[:3]
	}
	return w.Buffer.Write(p)
}
func TestEncodeHandlesShortWrites(t *testing.T) {
	var w shortWriter
	im := solid(696, 150, color.White)
	if err := Encode(&w, []image.Image{im}, Options{Model: "QL-800", Label: "62"}); err != nil {
		t.Fatal(err)
	}
	if w.Len() < 13000 {
		t.Fatal("truncated output")
	}
}
func BenchmarkEncodeQL800(b *testing.B) {
	im := solid(696, 1000, color.Black)
	o := Options{Model: "QL-800", Label: "62"}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := Encode(io.Discard, []image.Image{im}, o); err != nil {
			b.Fatal(err)
		}
	}
}
