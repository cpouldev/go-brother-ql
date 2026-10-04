// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzeCopiedPages(t *testing.T) {
	raw, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "--copies", "2", "-")
	if err != nil {
		t.Fatal(err)
	}
	prefix := filepath.Join(t.TempDir(), "page")
	out, _, err := run(t, raw, "analyze", "--json", "--output-prefix", prefix, "-")
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Pages []struct {
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(out, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Pages) != 2 {
		t.Fatalf("report: %s", out)
	}
	for _, page := range report.Pages {
		if page.Height != 150 {
			t.Fatalf("height: %d", page.Height)
		}
	}
	for _, name := range []string{"page0001.png", "page0002.png"} {
		b, err := os.ReadFile(filepath.Join(filepath.Dir(prefix), name))
		if err != nil {
			t.Fatal(err)
		}
		im, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if im.Bounds().Dy() != 150 {
			t.Fatalf("image: %v", im.Bounds())
		}
	}
}

func TestAnalyzeMalformed(t *testing.T) {
	out, _, err := run(t, []byte("broken"), "analyze", "-")
	if err == nil {
		t.Fatal("expected parse error")
	}
	if len(out) != 0 {
		t.Fatalf("unexpected output: %q", out)
	}
}
