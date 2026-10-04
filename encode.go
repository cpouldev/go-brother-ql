// SPDX-License-Identifier: GPL-3.0-or-later
package brotherql

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"strings"

	"github.com/cpouldev/go-brother-ql/internal/packbits"
)

// Encoder writes a raster job one page at a time. It is not safe for concurrent
// use. A write error permanently stops the encoder; create a new encoder for a
// new job, and never automatically replay a partially transmitted job.
type Encoder struct {
	w       io.Writer
	options Options
	model   Model
	label   Label
	pages   int
	done    bool
	err     error
}

// NewEncoder validates options without writing to w. Every page is fully
// prepared before its first command is emitted.
func NewEncoder(w io.Writer, opts Options) (*Encoder, error) {
	if w == nil {
		return nil, fmt.Errorf("nil raster writer")
	}
	o, err := ValidateOptions(opts)
	if err != nil {
		return nil, err
	}
	m, _ := LookupModel(o.Model)
	l, _ := LookupLabel(o.Label)
	return &Encoder{w: w, options: o, model: m, label: l}, nil
}

// Encode is a convenience wrapper for in-memory images. Use NewEncoder for
// sequential image decoding and bounded multi-page memory use. An empty images
// slice is an error. Earlier pages may already be written if a later page fails;
// spool the output first when the whole job must be validated before delivery.
func Encode(w io.Writer, images []image.Image, opts Options) error {
	if len(images) == 0 {
		return fmt.Errorf("no images to encode")
	}
	e, err := NewEncoder(w, opts)
	if err != nil {
		return err
	}
	for i, im := range images {
		if err = e.WritePage(im, i == len(images)-1); err != nil {
			return fmt.Errorf("page %d: %w", i+1, err)
		}
	}
	return nil
}

// WritePage emits one image. last must be true only for the job's final page.
// A successful final page finishes the encoder; subsequent writes fail. Image
// preparation errors write nothing for that page and leave the encoder usable.
// Writer errors stop the encoder permanently and may leave a partial page.
// Status requests are intentionally excluded: printer.Client owns status I/O.
func (e *Encoder) WritePage(im image.Image, last bool) error {
	if e.err != nil {
		return e.err
	}
	if e.done {
		return fmt.Errorf("raster job already finished")
	}
	p, err := prepare(im, e.options)
	if err != nil {
		return err
	}
	o, m, l := e.options, e.model, e.label
	header := make([]byte, 0, m.InvalidateBytes+64)
	if e.pages == 0 {
		if m.ModeSwitch {
			header = append(header, 0x1b, 0x69, 0x61, 1)
		}
		header = append(header, make([]byte, m.InvalidateBytes)...)
		header = append(header, 0x1b, 0x40)
		if m.ModeSwitch {
			header = append(header, 0x1b, 0x69, 0x61, 1)
		}
	}
	// The QL-8xx status mode persists until power-off, so restore notifications
	// explicitly even when a previous application disabled them (manual p. 30).
	if strings.HasPrefix(m.ID, "QL-8") {
		header = append(header, 0x1b, 0x69, 0x21, 0)
	}
	mediaType := byte(0x0b)
	if l.Kind == "continuous" {
		mediaType = 0x0a
	} else if l.Kind == "pt" {
		mediaType = 0
	}
	flags := byte(0x8e)
	if !o.LowQuality {
		flags |= 0x40
	}
	header = append(header, 0x1b, 0x69, 0x7a, flags, mediaType, byte(l.WidthMM), byte(l.LengthMM))
	header = binary.LittleEndian.AppendUint32(header, uint32(p.height))
	pageFlag := byte(0)
	if e.pages > 0 {
		pageFlag = 1
	}
	header = append(header, pageFlag, 0)
	if m.Cutting {
		auto := byte(0)
		if o.Cut == "each" || o.Cut == "every" {
			auto = 0x40
		}
		header = append(header, 0x1b, 0x69, 0x4d, auto)
		if auto != 0 && m.CutEvery {
			n := 1
			if o.Cut == "every" {
				n = o.CutEvery
			}
			header = append(header, 0x1b, 0x69, 0x41, byte(n))
		}
	}
	if m.ExpandedMode {
		expanded := byte(0)
		if o.Cut != "never" {
			expanded |= 8
		}
		if o.DPI600 {
			expanded |= 0x40
		}
		if o.Red {
			expanded |= 1
		}
		header = append(header, 0x1b, 0x69, 0x4b, expanded)
	}
	feed := l.Feed
	if o.DPI600 {
		feed *= 2
	}
	header = append(header, 0x1b, 0x69, 0x64)
	header = binary.LittleEndian.AppendUint16(header, uint16(feed))
	if o.Compress {
		header = append(header, 0x4d, 2)
	}
	if err = e.write(header); err != nil {
		return err
	}
	for y := 0; y < p.height; y++ {
		if err = e.row(p.black[y*m.RasterBytes:(y+1)*m.RasterBytes], 1); err != nil {
			return err
		}
		if o.Red {
			if err = e.row(p.red[y*m.RasterBytes:(y+1)*m.RasterBytes], 2); err != nil {
				return err
			}
		}
	}
	end := byte(0x0c)
	if last {
		end = 0x1a
	}
	if err = e.write([]byte{end}); err != nil {
		return err
	}
	e.pages++
	e.done = last
	return nil
}

func (e *Encoder) row(row []byte, plane byte) error {
	if e.options.Compress {
		row = packbits.Encode(row)
	}
	var prefix [3]byte
	if e.label.Kind == "pt" {
		prefix[0] = 0x47
		binary.LittleEndian.PutUint16(prefix[1:], uint16(len(row)))
	} else {
		prefix = [3]byte{0x67, 0, byte(len(row))}
		if e.options.Red {
			prefix[0] = 0x77
			prefix[1] = plane
		}
	}
	if err := e.write(prefix[:]); err != nil {
		return err
	}
	return e.write(row)
}

func (e *Encoder) write(p []byte) error {
	for len(p) > 0 {
		n, err := e.w.Write(p)
		if n < 0 || n > len(p) {
			e.err = fmt.Errorf("writer returned invalid count %d", n)
			return e.err
		}
		if err != nil {
			e.err = err
			return err
		}
		if n == 0 {
			e.err = io.ErrShortWrite
			return e.err
		}
		p = p[n:]
	}
	return nil
}
