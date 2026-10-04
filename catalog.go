// SPDX-License-Identifier: GPL-3.0-or-later
// Model and media data derived from Philipp Klaus's brother_ql and contributors.
package brotherql

import (
	"fmt"
	"slices"
	"strings"
)

// Model describes the raster capabilities of a printer. Experimental profiles
// are retained for upstream compatibility and have not been hardware validated.
type Model struct {
	ID                                                                           string
	RasterBytes, MinLength, MaxLength, MinFeed, MaxFeed, Offset, InvalidateBytes int
	ModeSwitch, Cutting, ExpandedMode, Compression, TwoColor, Experimental       bool
	// HighResolution is documented support for 600 dpi in the feed direction.
	HighResolution bool
	// CutEvery supports configurable label intervals beyond ordinary auto-cut.
	CutEvery bool
}

// Label describes physical media and its printable dimensions at 300 dpi.
// Continuous media have Height and LengthMM equal to zero.
type Label struct {
	ID                                             string
	WidthMM, LengthMM, Width, Height, Offset, Feed int
	Kind                                           string
	Red                                            bool
	Models                                         []string
}

// Models returns independent copies of all built-in printer profiles.
func Models() []Model { return slices.Clone(models) }

// LookupModel finds a built-in model, accepting case-insensitive model names.
func LookupModel(id string) (Model, error) {
	for _, m := range models {
		if strings.EqualFold(id, m.ID) {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("unknown printer model %q", id)
}

// Labels returns independent copies of all built-in media definitions.
func Labels() []Label {
	out := slices.Clone(labels)
	for i := range out {
		out[i].Models = slices.Clone(out[i].Models)
	}
	return out
}

// LookupLabel finds a media definition by its upstream identifier.
func LookupLabel(id string) (Label, error) {
	for _, l := range labels {
		if l.ID == id {
			l.Models = slices.Clone(l.Models)
			return l, nil
		}
	}
	return Label{}, fmt.Errorf("unknown label %q", id)
}
