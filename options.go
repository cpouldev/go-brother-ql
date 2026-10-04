// SPDX-License-Identifier: GPL-3.0-or-later
package brotherql

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Options controls media preparation and raster encoding. Model and Label must
// be supplied. Use ValidateOptions to apply defaults and check capabilities.
type Options struct {
	Model, Label string
	// Rotate is auto (the default), 0, 90, 180, or 270 degrees counterclockwise.
	// Auto rotates only an exact swapped-size die-cut image.
	Rotate string
	// Fit is auto (the default), strict, or contain. Auto scales continuous
	// media to printable width and requires exact dimensions for die-cut media.
	// Strict also requires the expected width for continuous media. Contain
	// centers and scales within die-cut media; for continuous media it is auto.
	Fit string
	// Cut is each, end, never, or every. The default is each on models with a
	// cutter and never otherwise. Every requires CutEvery; both every and end
	// require model support.
	Cut string
	// Threshold is a black threshold percentage from 0 to 100. Nil selects 70.
	// Dither ignores the threshold, but the supplied value must still be valid.
	Threshold *float64
	// Dither applies Floyd–Steinberg dithering to the black plane.
	Dither bool
	// Compress enables PackBits on models that support it.
	Compress bool
	// Red selects black/red output. With Label "62", it selects "62red" media.
	// Label "62red" also enables Red. Both the media and model must support it.
	Red bool
	// DPI600 selects 300 dpi across the head and 600 dpi along the feed. Supply
	// artwork at twice the catalog's 300 dpi dimensions; horizontal pixels are
	// reduced to the head width. This requires model support and excludes Red.
	DPI600 bool
	// LowQuality clears the high-quality flag, which is enabled by default.
	LowQuality bool
	// CutEvery is the interval from 1 to 255 for Cut "every". It must be zero
	// for all other cut policies.
	CutEvery int
}

// ValidateOptions returns normalized options with a private copy of Threshold.
// Unsupported requested capabilities are errors rather than silently ignored.
func ValidateOptions(o Options) (Options, error) {
	m, err := LookupModel(o.Model)
	if err != nil {
		return o, err
	}
	o.Model = m.ID
	if o.Red && o.Label == "62" {
		o.Label = "62red"
	}
	l, err := LookupLabel(o.Label)
	if err != nil {
		return o, err
	}
	if len(l.Models) > 0 && !slices.Contains(l.Models, m.ID) {
		return o, fmt.Errorf("label %s is not supported by %s", l.ID, m.ID)
	}
	if (l.Kind == "pt") != strings.HasPrefix(m.ID, "PT-") {
		return o, fmt.Errorf("label %s belongs to a different printer family than %s", l.ID, m.ID)
	}
	if l.Width+l.Offset+m.Offset > m.RasterBytes*8 {
		return o, fmt.Errorf("label %s exceeds %s print-head width", l.ID, m.ID)
	}
	o.Red = o.Red || l.Red
	if o.Red && o.DPI600 {
		return o, fmt.Errorf("two-color media does not support high-resolution mode")
	}
	if o.Red && (!m.TwoColor || !l.Red) {
		return o, fmt.Errorf("black/red printing requires a two-color model and 62red media")
	}
	if o.Compress && !m.Compression {
		return o, fmt.Errorf("%s does not support raster compression", m.ID)
	}
	if o.DPI600 && !m.HighResolution {
		return o, fmt.Errorf("%s has no verified high-resolution mode", m.ID)
	}
	if o.Rotate == "" {
		o.Rotate = "auto"
	}
	if !slices.Contains([]string{"auto", "0", "90", "180", "270"}, o.Rotate) {
		return o, fmt.Errorf("rotation must be auto, 0, 90, 180, or 270")
	}
	if o.Fit == "" {
		o.Fit = "auto"
	}
	if !slices.Contains([]string{"auto", "strict", "contain"}, o.Fit) {
		return o, fmt.Errorf("fit must be auto, strict, or contain")
	}
	threshold := 70.0
	if o.Threshold != nil {
		threshold = *o.Threshold
	}
	if math.IsNaN(threshold) || math.IsInf(threshold, 0) || threshold < 0 || threshold > 100 {
		return o, fmt.Errorf("threshold must be between 0 and 100 percent")
	}
	o.Threshold = &threshold
	if o.Cut == "" {
		o.Cut = "each"
		if !m.Cutting {
			o.Cut = "never"
		}
	}
	if !slices.Contains([]string{"each", "end", "never", "every"}, o.Cut) {
		return o, fmt.Errorf("cut must be each, end, never, or every")
	}
	if o.Cut != "never" && !m.Cutting {
		return o, fmt.Errorf("%s has no automatic cutter", m.ID)
	}
	if o.Cut == "end" && !m.ExpandedMode {
		return o, fmt.Errorf("%s does not support cutting only at job end", m.ID)
	}
	if o.Cut == "every" {
		if !m.CutEvery {
			return o, fmt.Errorf("%s does not support configurable cut intervals", m.ID)
		}
		if o.CutEvery < 1 || o.CutEvery > 255 {
			return o, fmt.Errorf("cut-every must be between 1 and 255")
		}
	} else if o.CutEvery != 0 {
		return o, fmt.Errorf("cut-every requires cut=every")
	}
	return o, nil
}
