// SPDX-License-Identifier: GPL-3.0-or-later
package raster

import (
	"bytes"
	"encoding/binary"
	"image/color"
	"io"
	"strings"
	"testing"
)

func media(rows uint32) []byte {
	b := []byte{0x1b, 'i', 'z', 0xce, 0x0b, 62, 29, 0, 0, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(b[7:11], rows)
	return b
}

func TestDecodeResetsPages(t *testing.T) {
	var job bytes.Buffer
	job.Write([]byte{0, 0, 0x1b, '@'})
	job.Write(media(2))
	job.Write([]byte{'g', 0, 1, 0x80, 'Z', 0x0c})
	job.Write(media(1))
	job.Write([]byte{'g', 0, 1, 1, 0x1a})
	pages, err := Decode(&job)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Height != 2 || pages[1].Height != 1 {
		t.Fatalf("page dimensions: %+v", pages)
	}
	if pages[0].MediaLength != 29 || pages[0].MediaWidth != 62 || pages[0].MediaType != 0x0b || pages[0].Width != 8 {
		t.Fatalf("media: %+v", pages[0])
	}
	assertColor(t, pages[0], 7, 0, color.Black)
	assertColor(t, pages[0], 0, 0, color.White)
	assertColor(t, pages[0], 7, 1, color.White)
	assertColor(t, pages[1], 0, 0, color.Black)
}

func TestDecodeColor(t *testing.T) {
	job := append(media(1), 0x1b, 'i', 'K', 0x41, 'w', 1, 1, 0x80, 'w', 2, 1, 0x81, 0x1a)
	pages, err := Decode(bytes.NewReader(job))
	if err != nil {
		t.Fatal(err)
	}
	if !pages[0].HighResolution {
		t.Fatal("missing high-resolution flag")
	}
	assertColor(t, pages[0], 7, 0, color.Black)
	assertColor(t, pages[0], 0, 0, color.RGBA{R: 255, A: 255})
}

func TestDecodePackBits(t *testing.T) {
	job := []byte{'M', 2, 'g', 0, 4, 128, 255, 0xaa, 128, 0x1a}
	pages, err := Decode(bytes.NewReader(job))
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Width != 16 {
		t.Fatalf("width=%d", pages[0].Width)
	}
	assertColor(t, pages[0], 15, 0, color.Black)
	assertColor(t, pages[0], 14, 0, color.White)
}

func TestDecodePT(t *testing.T) {
	job := []byte{'G', 2, 0, 0x80, 1, 0x1a}
	pages, err := Decode(bytes.NewReader(job))
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Width != 16 {
		t.Fatalf("width=%d", pages[0].Width)
	}
	assertColor(t, pages[0], 15, 0, color.Black)
	assertColor(t, pages[0], 0, 0, color.Black)
}

func TestDecodeZeroOnlyWithWidth(t *testing.T) {
	pages, err := DecodeWithOptions(bytes.NewReader([]byte{'Z', 'Z', 0x1a}), DecodeOptions{RasterBytes: 90})
	if err != nil {
		t.Fatal(err)
	}
	if pages[0].Width != 720 || pages[0].Height != 2 {
		t.Fatalf("page=%+v", pages[0])
	}
	assertColor(t, pages[0], 719, 1, color.White)
}

func TestDecodeKnownMetadata(t *testing.T) {
	job := []byte{0x1b, 'i', 'a', 1, 0x1b, 'i', '!', 0, 0x1b, 'i', 'M', 0x40, 0x1b, 'i', 'A', 1, 0x1b, 'i', 'd', 35, 0, 0x1b, 'i', 'S', 0x1b, 'i', 'X', 'G'}
	job = append(job, 0x1b, 'i', 'U', 'w', 1)
	job = append(job, make([]byte, 127)...)
	job = append(job, 0x1b, 'i', 'U', 'J')
	job = append(job, make([]byte, 14)...)
	job = append(job, 0x80, 0x20, 0x42)
	job = append(job, make([]byte, 29)...)
	job = append(job, 'g', 0, 1, 0, 0x1a)
	pages, err := Decode(bytes.NewReader(job))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages[0].Commands) < 10 {
		t.Fatalf("missing command descriptions: %+v", pages[0].Commands)
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
		part string
	}{
		{"no pages", nil, "no pages"}, {"unknown", []byte{0xff}, "unknown"},
		{"truncated escape", []byte{0x1b}, "truncated"}, {"truncated metadata", []byte{0x1b, 'i', 'z', 0}, "truncated"},
		{"truncated row", []byte{'g', 0, 2, 0}, "truncated"}, {"unfinished page", []byte{'g', 0, 1, 0}, "print command"},
		{"different widths", []byte{'g', 0, 1, 0, 'g', 0, 2, 0, 0, 0x1a}, "width"},
		{"unknown compression", []byte{'M', 1}, "compression"}, {"bad color", []byte{'w', 3, 1, 0, 0x1a}, "color"},
		{"missing red", []byte{0x1b, 'i', 'K', 1, 'w', 1, 1, 0, 0x1a}, "red"},
		{"metadata count", append(media(2), 'g', 0, 1, 0, 0x1a), "rows"},
		{"zero only", []byte{'Z', 0x1a}, "width"}, {"empty print", []byte{0x1a}, "empty"},
		{"oversized PT", []byte{'G', 0xff, 0xff}, "limit"},
		{"reset midpage", []byte{'g', 0, 1, 0, 0x1b, '@'}, "unfinished"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Decode(bytes.NewReader(tt.src))
			if err == nil || !strings.Contains(err.Error(), tt.part) {
				t.Fatalf("got %v, want %q", err, tt.part)
			}
		})
	}
}

func TestDecodeShortReads(t *testing.T) {
	pages, err := Decode(oneByteReader{bytes.NewReader([]byte{'g', 0, 2, 0x80, 0, 0x1a})})
	if err != nil {
		t.Fatal(err)
	}
	assertColor(t, pages[0], 15, 0, color.Black)
}

func TestDecodeRowLimits(t *testing.T) {
	for _, n := range []int{maxRows - 1, maxRows, maxRows + 1} {
		job := append(bytes.Repeat([]byte{'Z'}, n), 0x1a)
		_, err := DecodeWithOptions(bytes.NewReader(job), DecodeOptions{RasterBytes: 1})
		if (err == nil) != (n <= maxRows) {
			t.Fatalf("rows %d: %v", n, err)
		}
	}
}

func TestDecodeJobRowLimit(t *testing.T) {
	var job bytes.Buffer
	for i := 0; i < maxJobRows/maxRows; i++ {
		job.Write(bytes.Repeat([]byte{'Z'}, maxRows))
		job.WriteByte(0x0c)
	}
	job.Write([]byte{'Z', 0x1a})
	if _, err := DecodeWithOptions(&job, DecodeOptions{RasterBytes: 1}); err == nil || !strings.Contains(err.Error(), "job rows") {
		t.Fatalf("expected job row limit, got %v", err)
	}
}

func TestDecodeBlankExpansionLimit(t *testing.T) {
	job := append(bytes.Repeat([]byte{'Z'}, maxOutputBytes/4096+1), 0x1a)
	if _, err := DecodeWithOptions(bytes.NewReader(job), DecodeOptions{RasterBytes: 4096}); err == nil || !strings.Contains(err.Error(), "output") {
		t.Fatalf("expected output limit for blank expansion, got %v", err)
	}
}

func TestDecodeBlankExpansionAfterInferredWidth(t *testing.T) {
	job := bytes.Repeat([]byte{'Z'}, maxOutputBytes/4096)
	job = append(job, 'G', 0, 16)
	job = append(job, make([]byte, 4096)...)
	job = append(job, 0x1a)
	if _, err := Decode(bytes.NewReader(job)); err == nil || !strings.Contains(err.Error(), "output") {
		t.Fatalf("expected output limit for preceding blanks, got %v", err)
	}
}

func TestDecodeRejectsUnprintedMetadata(t *testing.T) {
	job := append([]byte{'g', 0, 1, 0, 0x0c}, media(1)...)
	if _, err := Decode(bytes.NewReader(job)); err == nil || !strings.Contains(err.Error(), "print command") {
		t.Fatalf("expected incomplete second page, got %v", err)
	}
}

func TestDecodePreservesTrailingCommands(t *testing.T) {
	job := []byte{'g', 0, 1, 0x80, 0x1a, 0x1b, 'i', 'K', 1}
	pages, err := Decode(bytes.NewReader(job))
	if err != nil {
		t.Fatal(err)
	}
	commands := pages[0].Commands
	last := commands[len(commands)-1]
	if last.Name != "expanded mode" || last.Offset != 5 || !bytes.Equal(last.Data, []byte{1}) {
		t.Fatalf("trailing metadata lost: %+v", commands)
	}
	if pages[0].HighResolution || len(pages[0].red) != 0 {
		t.Fatal("trailing mode command changed the already printed image")
	}
	assertColor(t, pages[0], 7, 0, color.Black)
}

func TestDecodeAcceptsIntermediatePageFragment(t *testing.T) {
	pages, err := Decode(bytes.NewReader([]byte{'g', 0, 1, 0, 0x0c}))
	if err != nil {
		t.Fatal(err)
	}
	commands := pages[0].Commands
	if commands[len(commands)-1].Name != "print page" {
		t.Fatalf("incorrect page finalization: %+v", commands)
	}
}

type oneByteReader struct{ io.Reader }

func (r oneByteReader) Read(b []byte) (int, error) {
	if len(b) > 1 {
		b = b[:1]
	}
	return r.Reader.Read(b)
}

func assertColor(t *testing.T, p Page, x, y int, want color.Color) {
	t.Helper()
	got := color.RGBAModel.Convert(p.Image().At(x, y))
	want = color.RGBAModel.Convert(want)
	if got != want {
		t.Fatalf("pixel (%d,%d)=%v, want %v", x, y, got, want)
	}
}

func FuzzDecode(f *testing.F) {
	for _, seed := range [][]byte{nil, {'g', 0, 1, 0x80, 0x1a}, {'Z', 0x1a}, {'M', 2, 'g', 0, 2, 129, 0, 0x1a}, append(media(1), 'g', 0, 1, 0, 0x1a)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1<<20 {
			t.Skip()
		}
		pages, err := Decode(bytes.NewReader(data))
		if err != nil {
			return
		}
		for _, p := range pages {
			if p.Width <= 0 || p.Height <= 0 {
				t.Fatalf("invalid page size: %+v", p)
			}
			_ = p.Image().At(0, 0)
			_ = p.Image().At(p.Width-1, p.Height-1)
		}
	})
}

func BenchmarkDecode(b *testing.B) {
	var job bytes.Buffer
	job.Write(media(150))
	for range 150 {
		job.Write([]byte{'g', 0, 90})
		job.Write(bytes.Repeat([]byte{0xaa}, 90))
	}
	job.WriteByte(0x1a)
	data := job.Bytes()
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := Decode(bytes.NewReader(data)); err != nil {
			b.Fatal(err)
		}
	}
}
