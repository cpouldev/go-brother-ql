// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func imageInput(t *testing.T) []byte {
	t.Helper()
	im := image.NewGray(image.Rect(0, 0, 696, 150))
	for y := 0; y < 150; y++ {
		for x := 0; x < 696; x++ {
			im.SetGray(x, y, color.Gray{Y: 240})
		}
	}
	im.SetGray(0, 0, color.Gray{Y: 0})
	var buf bytes.Buffer
	if err := png.Encode(&buf, im); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func imageFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "label.png")
	if err := os.WriteFile(path, imageInput(t), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConvertBinaryStdout(t *testing.T) {
	out, stderr, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 200 || !bytes.HasPrefix(out, []byte{0x1b, 0x69, 0x61, 1}) || out[len(out)-1] != 0x1a {
		t.Fatalf("invalid raster output (%d bytes)", len(out))
	}
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
}

func TestLowQualityChangesBytes(t *testing.T) {
	input := imageInput(t)
	normal, _, err := run(t, input, "convert", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	low, _, err := run(t, input, "convert", "-m", "QL-800", "-l", "62", "--lq", "-")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(normal, low) {
		t.Fatal("--lq did not change raster instructions")
	}
}

func TestPreviewPNG(t *testing.T) {
	out, stderr, err := run(t, imageInput(t), "preview", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	im, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if im.Bounds().Dx() != 720 || im.Bounds().Dy() != 150 {
		t.Fatalf("bounds: %v", im.Bounds())
	}
	if stderr != "" {
		t.Fatalf("stderr: %s", stderr)
	}
}

func TestConvertOutputFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "print.bin")
	out, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-o", path, "-")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("unexpected stdout: %q", out)
	}
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 200 {
		t.Fatalf("output: %d bytes, %v", len(b), err)
	}
}

func TestInvalidJobKeepsOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.bin")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	out, _, err := run(t, nil, "convert", "-m", "QL-800", "-l", "62", "-o", path, imageFile(t), "missing.png")
	if err == nil {
		t.Fatal("expected missing image error")
	}
	if len(out) != 0 {
		t.Fatalf("unexpected stdout: %q", out)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "original" {
		t.Fatalf("original changed: %q (%v)", b, err)
	}
}

func TestInvalidOptions(t *testing.T) {
	for _, extra := range [][]string{
		{"--copies", "0"}, {"--copies", "100001"}, {"--cut", "each", "--no-cut"},
		{"--cut-every", "3"}, {"--json"}, {"--compress"}, {"--threshold", "101"},
		{"--threshold", "NaN"}, {"--rotate", "7"},
	} {
		t.Run(strings.Join(extra, " "), func(t *testing.T) {
			args := append([]string{"convert", "-m", "QL-800", "-l", "62"}, extra...)
			args = append(args, "-")
			out, _, err := run(t, imageInput(t), args...)
			if err == nil {
				t.Fatal("expected option error")
			}
			if len(out) != 0 {
				t.Fatal("invalid job emitted bytes")
			}
		})
	}
}

func TestDuplicateStdinRejected(t *testing.T) {
	out, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-", "-")
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("error: %v", err)
	}
	if len(out) != 0 {
		t.Fatal("invalid job emitted bytes")
	}
}

func TestEnvironmentDefaults(t *testing.T) {
	t.Setenv("BROTHER_QL_MODEL", "QL-500")
	t.Setenv("BROTHER_QL_LABEL", "62")
	_, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-")
	if err != nil {
		t.Fatal(err)
	}
}

func TestModelsJSON(t *testing.T) {
	out, _, err := run(t, nil, "info", "models", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var models []map[string]any
	if err := json.Unmarshal(out, &models); err != nil {
		t.Fatal(err)
	}
	if len(models) < 17 {
		t.Fatalf("only %d models", len(models))
	}
}

func TestDoctor(t *testing.T) {
	out, _, err := run(t, nil, "doctor", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	if _, ok := report["usb_enabled"]; !ok {
		t.Fatalf("report: %s", out)
	}
}

func TestNoCutterDefault(t *testing.T) {
	var input bytes.Buffer
	if err := png.Encode(&input, image.NewGray(image.Rect(0, 0, 696, 300))); err != nil {
		t.Fatal(err)
	}
	_, _, err := run(t, input.Bytes(), "convert", "-m", "QL-500", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
}

func TestImageFormats(t *testing.T) {
	im, err := png.Decode(bytes.NewReader(imageInput(t)))
	if err != nil {
		t.Fatal(err)
	}
	formats := map[string]func(io.Writer, image.Image) error{
		"PNG":  png.Encode,
		"JPEG": func(w io.Writer, im image.Image) error { return jpeg.Encode(w, im, nil) },
		"GIF":  func(w io.Writer, im image.Image) error { return gif.Encode(w, im, nil) },
		"BMP":  bmp.Encode,
		"TIFF": func(w io.Writer, im image.Image) error { return tiff.Encode(w, im, nil) },
	}
	for name, encode := range formats {
		t.Run(name, func(t *testing.T) {
			var input bytes.Buffer
			if err := encode(&input, im); err != nil {
				t.Fatal(err)
			}
			_, _, err := run(t, input.Bytes(), "convert", "-m", "QL-800", "-l", "62", "-")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOversizedImageHeader(t *testing.T) {
	input := imageInput(t)
	binary.BigEndian.PutUint32(input[16:20], 50_000)
	binary.BigEndian.PutUint32(input[20:24], 50_000)
	binary.BigEndian.PutUint32(input[29:33], crc32.ChecksumIEEE(input[12:29]))
	_, _, err := run(t, input, "convert", "-m", "QL-800", "-l", "62", "-")
	if err == nil || !strings.Contains(err.Error(), "image dimensions") {
		t.Fatalf("error: %v", err)
	}
}

func TestCancelledConversion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, stderr bytes.Buffer
	err := Execute(ctx, []string{"convert", "-m", "QL-800", "-l", "62", "-"}, bytes.NewReader(imageInput(t)), &out, &stderr)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatal("canceled command emitted raster")
	}
}

type failingOutput struct{}

func (failingOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	err := Execute(context.Background(), []string{"convert", "-m", "QL-800", "-l", "62", "-"}, bytes.NewReader(imageInput(t)), failingOutput{}, &stderr)
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("error: %v", err)
	}
}
