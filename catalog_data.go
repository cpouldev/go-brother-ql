// SPDX-License-Identifier: GPL-3.0-or-later
// Catalog derived from pklaus/brother_ql at 0dee94b5fc3721159bae556b8bd2d975f92b7b8b.
package brotherql

var models = []Model{
	{ID: "QL-500", RasterBytes: 90, MinLength: 295, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: false, Cutting: false, ExpandedMode: false, Compression: false, TwoColor: false, Experimental: false},
	{ID: "QL-550", RasterBytes: 90, MinLength: 295, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: false, Cutting: true, ExpandedMode: false, Compression: false, TwoColor: false, Experimental: false},
	{ID: "QL-560", RasterBytes: 90, MinLength: 295, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: false, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: false, TwoColor: false, Experimental: false},
	{ID: "QL-570", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: false, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: false, TwoColor: false, Experimental: false},
	{ID: "QL-580N", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-650TD", RasterBytes: 90, MinLength: 295, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-700", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: false, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: false, TwoColor: false, Experimental: false},
	{ID: "QL-710W", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-720NW", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-800", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 400, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: false, TwoColor: true, Experimental: false},
	{ID: "QL-810W", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 400, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: true, Experimental: false},
	{ID: "QL-820NWB", HighResolution: true, RasterBytes: 90, MinLength: 150, MaxLength: 11811, MinFeed: 35, MaxFeed: 1500, Offset: 0, InvalidateBytes: 400, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: true, Experimental: false},
	{ID: "QL-1050", RasterBytes: 162, MinLength: 295, MaxLength: 35433, MinFeed: 35, MaxFeed: 1500, Offset: 44, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-1060N", RasterBytes: 162, MinLength: 295, MaxLength: 35433, MinFeed: 35, MaxFeed: 1500, Offset: 44, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-1100", RasterBytes: 162, MinLength: 301, MaxLength: 35434, MinFeed: 35, MaxFeed: 1500, Offset: 44, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-1110NWB", RasterBytes: 162, MinLength: 301, MaxLength: 35434, MinFeed: 35, MaxFeed: 1500, Offset: 44, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "QL-1115NWB", RasterBytes: 162, MinLength: 301, MaxLength: 35434, MinFeed: 35, MaxFeed: 1500, Offset: 44, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: false},
	{ID: "PT-P750W", RasterBytes: 16, MinLength: 31, MaxLength: 14172, MinFeed: 0, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: true},
	{ID: "PT-P900W", RasterBytes: 70, MinLength: 57, MaxLength: 28346, MinFeed: 0, MaxFeed: 1500, Offset: 0, InvalidateBytes: 200, ModeSwitch: true, Cutting: true, CutEvery: true, ExpandedMode: true, Compression: true, TwoColor: false, Experimental: true},
}

var labels = []Label{
	{ID: "12", WidthMM: 12, LengthMM: 0, Width: 106, Height: 0, Offset: 29, Feed: 35, Kind: "continuous"},
	{ID: "29", WidthMM: 29, LengthMM: 0, Width: 306, Height: 0, Offset: 6, Feed: 35, Kind: "continuous"},
	{ID: "38", WidthMM: 38, LengthMM: 0, Width: 413, Height: 0, Offset: 12, Feed: 35, Kind: "continuous"},
	{ID: "50", WidthMM: 50, LengthMM: 0, Width: 554, Height: 0, Offset: 12, Feed: 35, Kind: "continuous"},
	{ID: "54", WidthMM: 54, LengthMM: 0, Width: 590, Height: 0, Offset: 0, Feed: 35, Kind: "continuous"},
	{ID: "62", WidthMM: 62, LengthMM: 0, Width: 696, Height: 0, Offset: 12, Feed: 35, Kind: "continuous"},
	{ID: "62red", WidthMM: 62, LengthMM: 0, Width: 696, Height: 0, Offset: 12, Feed: 35, Kind: "continuous", Red: true},
	{ID: "102", WidthMM: 102, LengthMM: 0, Width: 1164, Height: 0, Offset: 12, Feed: 35, Kind: "continuous", Models: []string{"QL-1050", "QL-1060N", "QL-1100", "QL-1110NWB", "QL-1115NWB"}},
	{ID: "103", WidthMM: 104, LengthMM: 0, Width: 1200, Height: 0, Offset: 12, Feed: 35, Kind: "continuous", Models: []string{"QL-1050", "QL-1060N", "QL-1100", "QL-1110NWB", "QL-1115NWB"}},
	{ID: "17x54", WidthMM: 17, LengthMM: 54, Width: 165, Height: 566, Offset: 0, Feed: 0, Kind: "die-cut"},
	{ID: "17x87", WidthMM: 17, LengthMM: 87, Width: 165, Height: 956, Offset: 0, Feed: 0, Kind: "die-cut"},
	// QL-800 manual pp.15,20 corrects upstream's 202-dot width to 236.
	{ID: "23x23", WidthMM: 23, LengthMM: 23, Width: 236, Height: 202, Offset: 42, Feed: 0, Kind: "die-cut"},
	{ID: "29x42", WidthMM: 29, LengthMM: 42, Width: 306, Height: 425, Offset: 6, Feed: 0, Kind: "die-cut"},
	{ID: "29x90", WidthMM: 29, LengthMM: 90, Width: 306, Height: 991, Offset: 6, Feed: 0, Kind: "die-cut"},
	{ID: "39x90", WidthMM: 38, LengthMM: 90, Width: 413, Height: 991, Offset: 12, Feed: 0, Kind: "die-cut"},
	{ID: "39x48", WidthMM: 39, LengthMM: 48, Width: 425, Height: 495, Offset: 6, Feed: 0, Kind: "die-cut"},
	{ID: "52x29", WidthMM: 52, LengthMM: 29, Width: 578, Height: 271, Offset: 0, Feed: 0, Kind: "die-cut"},
	{ID: "60x86", WidthMM: 60, LengthMM: 87, Width: 672, Height: 954, Offset: 18, Feed: 0, Kind: "die-cut"},
	{ID: "62x29", WidthMM: 62, LengthMM: 29, Width: 696, Height: 271, Offset: 12, Feed: 0, Kind: "die-cut"},
	{ID: "62x100", WidthMM: 62, LengthMM: 100, Width: 696, Height: 1109, Offset: 12, Feed: 0, Kind: "die-cut"},
	{ID: "102x51", WidthMM: 102, LengthMM: 51, Width: 1164, Height: 526, Offset: 12, Feed: 0, Kind: "die-cut", Models: []string{"QL-1050", "QL-1060N", "QL-1100", "QL-1110NWB", "QL-1115NWB"}},
	{ID: "102x152", WidthMM: 102, LengthMM: 153, Width: 1164, Height: 1660, Offset: 12, Feed: 0, Kind: "die-cut", Models: []string{"QL-1050", "QL-1060N", "QL-1100", "QL-1110NWB", "QL-1115NWB"}},
	{ID: "103x164", WidthMM: 104, LengthMM: 164, Width: 1200, Height: 1822, Offset: 12, Feed: 0, Kind: "die-cut", Models: []string{"QL-1100", "QL-1110NWB"}},
	// Die-cut media use zero feed (QL-800 manual p.28), including round stock.
	{ID: "d12", WidthMM: 12, LengthMM: 12, Width: 94, Height: 94, Offset: 113, Feed: 0, Kind: "round"},
	{ID: "d24", WidthMM: 24, LengthMM: 24, Width: 236, Height: 236, Offset: 42, Feed: 0, Kind: "round"},
	{ID: "d58", WidthMM: 58, LengthMM: 58, Width: 618, Height: 618, Offset: 51, Feed: 0, Kind: "round"},
	{ID: "pt24", WidthMM: 24, LengthMM: 0, Width: 128, Height: 0, Offset: 0, Feed: 14, Kind: "pt"},
}
