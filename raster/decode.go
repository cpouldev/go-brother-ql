// SPDX-License-Identifier: GPL-3.0-or-later

// Package raster independently decodes Brother QL and P-touch raster streams.
// It accepts the command set understood by Philipp Klaus's brother_ql reader,
// validates page boundaries, and exposes packed images in viewing orientation.
package raster

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"

	"github.com/cpouldev/go-brother-ql/internal/packbits"
)

const (
	maxInputBytes     = 128 << 20
	maxOutputBytes    = 64 << 20
	maxRows           = 100_000
	maxJobRows        = 1_000_000
	maxPages          = 10_000
	maxCommands       = 2_000_000
	maxCommandRecords = 65_536
)

// DecodeOptions supplies head width when the instruction stream cannot express
// it. RasterBytes is the uncompressed byte width from 1 to 4096; zero infers it
// from row data. A nonzero value also checks that decoded rows match that width.
type DecodeOptions struct {
	RasterBytes int
}

// Command describes a parsed instruction. Consecutive raster and invalidation
// commands are summarized by Count. Data holds metadata, never pixel bytes.
// Commands after the last printed page are attached to that page so analysis
// retains the whole stream; they do not modify the already printed image.
// Offset is the byte offset of the instruction, or the first in a summarized run.
type Command struct {
	Offset int64  `json:"offset"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
	Data   []byte `json:"data,omitempty"`
}

// Page describes one printed page. Width and Height are raster dots, including
// head margins; MediaWidth and MediaLength are millimetres. HighResolution means
// 600 DPI in the feed direction; Image preserves these raster dimensions.
type Page struct {
	Width          int       `json:"width"`
	Height         int       `json:"height"`
	MediaWidth     int       `json:"media_width"`
	MediaLength    int       `json:"media_length"`
	MediaType      byte      `json:"media_type"`
	HighResolution bool      `json:"high_resolution"`
	Commands       []Command `json:"commands,omitempty"`
	black          [][]byte
	red            [][]byte
}

// Image returns a read-only packed view without allocating an RGBA image.
// Pixels are mirrored horizontally from wire order into viewing orientation.
// Black takes precedence if a stream marks the same pixel in both planes.
func (p Page) Image() image.Image { return pageImage{p} }

type pageImage struct{ p Page }

// Reuse boxed colors: PNG exporters can call At millions of times per page.
var (
	transparentPixel color.Color = color.RGBA{}
	blackPixel       color.Color = color.RGBA{A: 255}
	redPixel         color.Color = color.RGBA{R: 255, A: 255}
	whitePixel       color.Color = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

func (im pageImage) ColorModel() color.Model { return color.RGBAModel }
func (im pageImage) Bounds() image.Rectangle { return image.Rect(0, 0, im.p.Width, im.p.Height) }
func (im pageImage) At(x, y int) color.Color {
	if x < 0 || y < 0 || x >= im.p.Width || y >= im.p.Height {
		return transparentPixel
	}
	x = im.p.Width - 1 - x
	if ink(im.p.black, x, y) {
		return blackPixel
	}
	if ink(im.p.red, x, y) {
		return redPixel
	}
	return whitePixel
}

func ink(rows [][]byte, x, y int) bool {
	return y < len(rows) && x/8 < len(rows[y]) && rows[y][x/8]&(0x80>>uint(x%8)) != 0
}

// Decode reads through EOF, parses raster pages, and infers the head width from
// non-empty rows. It returns no partial pages on error.
// An all-Z page needs DecodeWithOptions because Z carries no head width.
// Bounds: 128 MiB input, 64 MiB packed output, 100,000 rows/page, 1,000,000
// plane rows/job, 10,000 pages, 4,096 bytes/row, 2,000,000 commands, and 65,536
// command records.
// Intermediate-page fragments ending in FF are accepted. Decode does not open
// a printer or establish that a stream is a complete, safe-to-print job.
func Decode(r io.Reader) ([]Page, error) { return DecodeWithOptions(r, DecodeOptions{}) }

// DecodeWithOptions parses pages while checking an optional known head width.
// Malformed commands return an error with the byte offset and no partial pages.
func DecodeWithOptions(r io.Reader, opts DecodeOptions) ([]Page, error) {
	if r == nil {
		return nil, fmt.Errorf("raster: nil reader")
	}
	if opts.RasterBytes < 0 || opts.RasterBytes > packbits.MaxDecodedSize {
		return nil, fmt.Errorf("raster: head width outside 0..%d bytes", packbits.MaxDecodedSize)
	}
	d := decoder{r: bufio.NewReader(io.LimitReader(r, maxInputBytes+1)), opts: opts, width: opts.RasterBytes}
	for {
		start := d.offset
		op, err := d.byte()
		if err == io.EOF {
			if d.hasRows() || d.expectedRows > 0 {
				return nil, d.fail(start, fmt.Errorf("page is missing a print command"))
			}
			if len(d.pages) == 0 {
				return nil, d.fail(start, fmt.Errorf("no pages in stream"))
			}
			last := &d.pages[len(d.pages)-1]
			last.Commands = append(last.Commands, d.page.Commands...)
			return d.pages, nil
		}
		if err != nil {
			return nil, d.fail(start, err)
		}
		d.commandCount++
		if d.commandCount > maxCommands {
			return nil, d.fail(start, fmt.Errorf("command count exceeds limit"))
		}
		if err := d.command(op, start); err != nil {
			return nil, d.fail(start, err)
		}
	}
}

type decoder struct {
	r                                               *bufio.Reader
	opts                                            DecodeOptions
	offset                                          int64
	commandCount, recordCount, jobRows, outputBytes int
	width, blankRows                                int
	compressed, twoColor                            bool
	expectedRows                                    uint32
	page                                            Page
	pages                                           []Page
}

func (d *decoder) fail(offset int64, err error) error {
	return fmt.Errorf("raster: byte %d: %w", offset, err)
}
func (d *decoder) hasRows() bool { return len(d.page.black) > 0 || len(d.page.red) > 0 }

func (d *decoder) byte() (byte, error) {
	b, err := d.r.ReadByte()
	if err == nil {
		d.offset++
		if d.offset > maxInputBytes {
			return 0, fmt.Errorf("input exceeds %d-byte limit", maxInputBytes)
		}
	}
	return b, err
}

func (d *decoder) take(n int) ([]byte, error) {
	if d.offset+int64(n) > maxInputBytes {
		return nil, fmt.Errorf("input exceeds %d-byte limit", maxInputBytes)
	}
	b := make([]byte, n)
	nread, err := io.ReadFull(d.r, b)
	d.offset += int64(nread)
	if err != nil {
		return nil, fmt.Errorf("truncated command: %w", err)
	}
	return b, nil
}

func (d *decoder) record(offset int64, name string, data []byte) error {
	n := len(d.page.Commands)
	if data == nil && n > 0 && d.page.Commands[n-1].Name == name {
		d.page.Commands[n-1].Count++
		return nil
	}
	d.recordCount++
	if d.recordCount > maxCommandRecords {
		return fmt.Errorf("command metadata exceeds limit")
	}
	d.page.Commands = append(d.page.Commands, Command{Offset: offset, Name: name, Count: 1, Data: data})
	return nil
}

func (d *decoder) command(op byte, offset int64) error {
	switch op {
	case 0:
		return d.record(offset, "invalidate", nil)
	case 'M':
		b, err := d.take(1)
		if err != nil {
			return err
		}
		if b[0] != 0 && b[0] != 2 {
			return fmt.Errorf("unsupported compression mode %d", b[0])
		}
		d.compressed = b[0] == 2
		return d.record(offset, "compression", b)
	case 'g', 'G', 'w':
		return d.raster(op, offset)
	case 'Z':
		if d.twoColor && len(d.page.black) != len(d.page.red) {
			return fmt.Errorf("zero row interrupts black/red pair")
		}
		if err := d.appendRow(nil, false); err != nil {
			return err
		}
		if d.twoColor {
			if err := d.appendRow(nil, true); err != nil {
				return err
			}
		}
		return d.record(offset, "zero raster", nil)
	case 0x0c, 0x1a:
		name := "print page"
		if op == 0x1a {
			name = "print final page"
		}
		if err := d.record(offset, name, nil); err != nil {
			return err
		}
		return d.finishPage()
	case 0x1b:
		return d.escape(offset)
	case 0x80:
		b, err := d.take(31)
		if err != nil {
			return err
		}
		if b[0] != 0x20 || b[1] != 0x42 {
			return fmt.Errorf("unknown status response header")
		}
		return d.record(offset, "status response", b)
	default:
		return fmt.Errorf("unknown opcode 0x%02x", op)
	}
}

func (d *decoder) escape(offset int64) error {
	b, err := d.take(1)
	if err != nil {
		return err
	}
	if b[0] == '@' {
		if d.hasRows() {
			return fmt.Errorf("initialization discards an unfinished page")
		}
		d.compressed, d.twoColor = false, false
		d.width = d.opts.RasterBytes
		d.expectedRows = 0
		d.page.MediaWidth, d.page.MediaLength = 0, 0
		d.page.MediaType = 0
		d.page.HighResolution = false
		return d.record(offset, "initialize", nil)
	}
	if b[0] != 'i' {
		return fmt.Errorf("unknown escape opcode 0x%02x", b[0])
	}
	b, err = d.take(1)
	if err != nil {
		return err
	}
	var name string
	var n int
	switch b[0] {
	case 'a':
		name, n = "mode setting", 1
	case '!':
		name, n = "automatic status", 1
	case 'z':
		name, n = "media/quality", 10
	case 'M':
		name, n = "various mode", 1
	case 'A':
		name, n = "cut every", 1
	case 'K':
		name, n = "expanded mode", 1
	case 'd':
		name, n = "margins", 2
	case 'S':
		name = "status request"
	case 'X':
		suffix, err := d.take(1)
		if err != nil {
			return err
		}
		if suffix[0] != 'G' {
			return fmt.Errorf("unknown configuration opcode 0x%02x", suffix[0])
		}
		name = "request configuration"
	case 'U':
		suffix, err := d.take(1)
		if err != nil {
			return err
		}
		switch suffix[0] {
		case 'J':
			name, n = "job ID", 14
		case 'w':
			version, err := d.take(1)
			if err != nil {
				return err
			}
			if version[0] != 1 {
				return fmt.Errorf("unknown additional media version %d", version[0])
			}
			name, n = "additional media", 127
		default:
			return fmt.Errorf("unknown media opcode 0x%02x", suffix[0])
		}
	default:
		return fmt.Errorf("unknown ESC i opcode 0x%02x", b[0])
	}
	data, err := d.take(n)
	if err != nil {
		return err
	}
	switch b[0] {
	case 'a':
		if data[0] != 1 {
			return fmt.Errorf("unsupported non-raster mode %d", data[0])
		}
	case 'z':
		if d.hasRows() {
			return fmt.Errorf("media command interrupts an unfinished page")
		}
		d.expectedRows = binary.LittleEndian.Uint32(data[4:8])
		if d.expectedRows > maxRows {
			return fmt.Errorf("declared rows exceed %d-row limit", maxRows)
		}
		if data[0]&4 != 0 {
			d.page.MediaWidth = int(data[2])
		}
		if data[0]&2 != 0 {
			d.page.MediaType = data[1]
		}
		if data[0]&8 != 0 {
			d.page.MediaLength = int(data[3])
		}
	case 'K':
		if d.hasRows() {
			return fmt.Errorf("expanded mode interrupts an unfinished page")
		}
		d.twoColor = data[0]&1 != 0
		d.page.HighResolution = data[0]&0x40 != 0
	}
	return d.record(offset, name, data)
}

func (d *decoder) raster(op byte, offset int64) error {
	hdr, err := d.take(2)
	if err != nil {
		return err
	}
	n := int(hdr[1])
	red := false
	switch op {
	case 'G':
		if d.twoColor {
			return fmt.Errorf("monochrome row in two-color mode")
		}
		n = int(binary.LittleEndian.Uint16(hdr))
	case 'g':
		if hdr[0] != 0 {
			return fmt.Errorf("unknown QL raster flag 0x%02x", hdr[0])
		}
		if d.twoColor {
			return fmt.Errorf("monochrome row in two-color mode")
		}
	case 'w':
		if hdr[0] != 1 && hdr[0] != 2 {
			return fmt.Errorf("unknown raster color %d", hdr[0])
		}
		red = hdr[0] == 2
		if !d.twoColor {
			return fmt.Errorf("two-color row without expanded color mode")
		}
		if (!red && len(d.page.black) != len(d.page.red)) || (red && len(d.page.black) != len(d.page.red)+1) {
			return fmt.Errorf("invalid black/red row sequence")
		}
	}
	if n == 0 {
		return fmt.Errorf("empty raster row; use zero-raster command")
	}
	if n > packbits.MaxDecodedSize+packbits.MaxDecodedSize/128+1 {
		return fmt.Errorf("raster payload exceeds row limit")
	}
	row, err := d.take(n)
	if err != nil {
		return err
	}
	if d.compressed {
		width, err := packbits.DecodedLen(row)
		if err != nil {
			return err
		}
		if width == 0 {
			return fmt.Errorf("empty compressed raster row")
		}
		row, err = packbits.Decode(row, width)
		if err != nil {
			return err
		}
	}
	if len(row) > packbits.MaxDecodedSize {
		return fmt.Errorf("raster width exceeds limit")
	}
	if d.width == 0 {
		d.width = len(row)
	}
	if len(row) != d.width {
		return fmt.Errorf("raster width %d bytes differs from %d", len(row), d.width)
	}
	if err := d.appendRow(row, red); err != nil {
		return err
	}
	return d.record(offset, "raster", nil)
}

func (d *decoder) appendRow(row []byte, red bool) error {
	d.jobRows++
	if d.jobRows > maxJobRows {
		return fmt.Errorf("job rows exceed limit")
	}
	d.outputBytes += len(row)
	if len(row) == 0 {
		// Width can be learned after a run of Z commands. Charge their logical
		// expansion on page completion, once the width is known.
		d.blankRows++
	}
	if d.outputBytes > maxOutputBytes {
		return fmt.Errorf("decoded output exceeds %d-byte limit", maxOutputBytes)
	}
	rows := &d.page.black
	if red {
		rows = &d.page.red
	}
	if len(*rows) >= maxRows {
		return fmt.Errorf("page rows exceed %d-row limit", maxRows)
	}
	*rows = append(*rows, row)
	return nil
}

func (d *decoder) finishPage() error {
	if !d.hasRows() {
		return fmt.Errorf("cannot print an empty page")
	}
	if d.width == 0 {
		return fmt.Errorf("cannot infer head width from zero rows; supply RasterBytes")
	}
	if d.twoColor && len(d.page.black) != len(d.page.red) {
		return fmt.Errorf("black/red row counts differ; missing red plane")
	}
	if d.expectedRows != 0 && uint32(len(d.page.black)) != d.expectedRows {
		return fmt.Errorf("page has %d rows, metadata declares %d", len(d.page.black), d.expectedRows)
	}
	d.outputBytes += d.blankRows * d.width
	if d.outputBytes > maxOutputBytes {
		return fmt.Errorf("decoded output exceeds %d-byte limit", maxOutputBytes)
	}
	if len(d.pages) >= maxPages {
		return fmt.Errorf("page count exceeds limit")
	}
	d.page.Width, d.page.Height = d.width*8, len(d.page.black)
	d.pages = append(d.pages, d.page)
	d.page = Page{MediaWidth: d.page.MediaWidth, MediaLength: d.page.MediaLength, MediaType: d.page.MediaType, HighResolution: d.page.HighResolution}
	d.width = d.opts.RasterBytes
	d.blankRows = 0
	d.expectedRows = 0
	return nil
}
