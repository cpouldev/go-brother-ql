// SPDX-License-Identifier: GPL-3.0-or-later
package brotherql

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strconv"

	xdraw "golang.org/x/image/draw"
)

// MaxImagePixels limits source and prepared image sizes before allocating.
const MaxImagePixels = 40_000_000

type prepared struct {
	width, height int
	black, red    []byte // packed in print-head transmission order, MSB first
}

// Preview returns the actual monochrome/two-color raster in viewing orientation,
// including unused print-head margins. It never communicates with a printer.
// With DPI600, it retains the doubled vertical dot count, so an image viewer
// displaying square pixels may show the result taller than the printed label.
func Preview(im image.Image, opts Options) (image.Image, error) {
	o, err := ValidateOptions(opts)
	if err != nil {
		return nil, err
	}
	p, err := prepare(im, o)
	if err != nil {
		return nil, err
	}
	return &previewImage{p}, nil
}

type previewImage struct{ *prepared }

func (p *previewImage) ColorModel() color.Model { return color.NRGBAModel }
func (p *previewImage) Bounds() image.Rectangle { return image.Rect(0, 0, p.width, p.height) }
func (p *previewImage) At(x, y int) color.Color {
	if !image.Pt(x, y).In(p.Bounds()) {
		return color.NRGBA{}
	}
	wireX := p.width - 1 - x
	idx := y*(p.width/8) + wireX/8
	bit := byte(0x80 >> uint(wireX%8))
	if p.black[idx]&bit != 0 {
		return color.NRGBA{0, 0, 0, 255}
	}
	if len(p.red) > 0 && p.red[idx]&bit != 0 {
		return color.NRGBA{255, 0, 0, 255}
	}
	return color.NRGBA{255, 255, 255, 255}
}

func dimensions(w, h int) error {
	if w <= 0 || h <= 0 {
		return fmt.Errorf("image dimensions must be positive")
	}
	if w > MaxImagePixels/h {
		return fmt.Errorf("image exceeds %d-pixel limit", MaxImagePixels)
	}
	return nil
}

func prepare(im image.Image, o Options) (*prepared, error) {
	if im == nil {
		return nil, fmt.Errorf("nil image")
	}
	b := im.Bounds()
	w, h := b.Dx(), b.Dy()
	if err := dimensions(w, h); err != nil {
		return nil, err
	}
	m, _ := LookupModel(o.Model)
	l, _ := LookupLabel(o.Label)
	scale := 1
	if o.DPI600 {
		scale = 2
	}
	rotation := 0
	if o.Rotate == "auto" {
		if l.Height > 0 && w == l.Height*scale && h == l.Width*scale {
			rotation = 90
		}
	} else {
		rotation, _ = strconv.Atoi(o.Rotate)
	}
	rw, rh := w, h
	if rotation == 90 || rotation == 270 {
		rw, rh = h, w
	}
	targetW, targetH := l.Width, l.Height*scale
	if l.Height == 0 {
		if o.Fit == "strict" && rw != l.Width*scale {
			return nil, fmt.Errorf("image width %d; label %s requires %d pixels", rw, l.ID, l.Width*scale)
		}
		targetH = int(float64(rh) * float64(l.Width*scale) / float64(rw))
		if targetH < m.MinLength*scale || targetH > m.MaxLength*scale {
			return nil, fmt.Errorf("image produces %d rows; %s requires %d..%d rows at selected resolution", targetH, m.ID, m.MinLength*scale, m.MaxLength*scale)
		}
	} else if o.Fit != "contain" && (rw != l.Width*scale || rh != l.Height*scale) {
		return nil, fmt.Errorf("image dimensions %dx%d; label %s requires %dx%d pixels (or fit=contain)", rw, rh, l.ID, l.Width*scale, l.Height*scale)
	}
	if err := dimensions(m.RasterBytes*8, targetH); err != nil {
		return nil, err
	}
	// Composite against white while rotating. NRGBA sources avoid conversion
	// allocations; other standard image types retain full image.Image support.
	rotated := image.NewNRGBA(image.Rect(0, 0, rw, rh))
	sample := pixelReader(im)
	for y := 0; y < rh; y++ {
		for x := 0; x < rw; x++ {
			sx, sy := x, y
			switch rotation {
			case 90:
				sx, sy = w-1-y, x
			case 180:
				sx, sy = w-1-x, h-1-y
			case 270:
				sx, sy = y, h-1-x
			}
			r, g, bl := sample(b.Min.X+sx, b.Min.Y+sy)
			i := rotated.PixOffset(x, y)
			rotated.Pix[i] = r
			rotated.Pix[i+1] = g
			rotated.Pix[i+2] = bl
			rotated.Pix[i+3] = 255
		}
	}
	target := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	for i := range target.Pix {
		target.Pix[i] = 255
	}
	dest := target.Bounds()
	if l.Height > 0 && o.Fit == "contain" {
		ratio := math.Min(float64(l.Width*scale)/float64(rw), float64(targetH)/float64(rh))
		dw, dh := max(1, int(math.Round(float64(rw)*ratio/float64(scale)))), max(1, int(math.Round(float64(rh)*ratio)))
		dw = min(dw, targetW)
		dh = min(dh, targetH)
		x, y := (targetW-dw)/2, (targetH-dh)/2
		dest = image.Rect(x, y, x+dw, y+dh)
	}
	if dest.Dx() == rw && dest.Dy() == rh {
		xdraw.Copy(target, dest.Min, rotated, rotated.Bounds(), xdraw.Src, nil)
	} else {
		xdraw.CatmullRom.Scale(target, dest, rotated, rotated.Bounds(), xdraw.Src, nil)
	}
	p := &prepared{width: m.RasterBytes * 8, height: targetH, black: make([]byte, m.RasterBytes*targetH)}
	if o.Red {
		p.red = make([]byte, len(p.black))
	}
	offset := p.width - l.Width - l.Offset - m.Offset
	// Dithering error is carried only within printable pixels, never into margins.
	current, next := make([]float64, targetW+2), make([]float64, targetW+2)
	cutoff := math.Floor((100 - *o.Threshold) * 255 / 100)
	for y := 0; y < targetH; y++ {
		for x := 0; x < targetW; x++ {
			i := target.PixOffset(x, y)
			r, g, bl := target.Pix[i], target.Pix[i+1], target.Pix[i+2]
			gray := float64((19595*uint32(r) + 38470*uint32(g) + 7471*uint32(bl) + 32768) >> 16)
			isRed := o.Red && redPixel(r, g, bl)
			ink := 255-gray >= cutoff
			if o.Dither && !isRed {
				value := gray + current[x+1]
				ink = value < 128
				quant := 255.0
				if ink {
					quant = 0
				}
				diff := value - quant
				current[x+2] += diff * 7 / 16
				next[x] += diff * 3 / 16
				next[x+1] += diff * 5 / 16
				next[x+2] += diff / 16
			}
			// Black and red never overlap, including when dithering is enabled.
			if isRed || ink {
				wireX := p.width - 1 - (offset + x)
				index := y*m.RasterBytes + wireX/8
				bit := byte(0x80 >> uint(wireX%8))
				if isRed {
					p.red[index] |= bit
				} else {
					p.black[index] |= bit
				}
			}
		}
		current, next = next, current
		clear(next)
	}
	return p, nil
}

// Avoid a boxed color allocation per source pixel for common image formats.
// The fallback still supports arbitrary image.Image implementations.
func pixelReader(im image.Image) func(int, int) (byte, byte, byte) {
	switch src := im.(type) {
	case *image.NRGBA:
		return func(x, y int) (byte, byte, byte) {
			i := src.PixOffset(x, y)
			a := uint32(src.Pix[i+3])
			background := 65535 - a*257 + 128
			return byte((uint32(src.Pix[i])*257*a/255 + background) / 257),
				byte((uint32(src.Pix[i+1])*257*a/255 + background) / 257),
				byte((uint32(src.Pix[i+2])*257*a/255 + background) / 257)
		}
	case *image.RGBA:
		return func(x, y int) (byte, byte, byte) {
			i := src.PixOffset(x, y)
			background := 255 - src.Pix[i+3]
			return src.Pix[i] + background, src.Pix[i+1] + background, src.Pix[i+2] + background
		}
	case *image.Gray:
		return func(x, y int) (byte, byte, byte) {
			v := src.Pix[src.PixOffset(x, y)]
			return v, v, v
		}
	default:
		return func(x, y int) (byte, byte, byte) {
			r, g, b, a := im.At(x, y).RGBA()
			return byte((r + 65535 - a + 128) / 257), byte((g + 65535 - a + 128) / 257), byte((b + 65535 - a + 128) / 257)
		}
	}
}

// redPixel uses the same broad hue/saturation/value regions as upstream.
func redPixel(r, g, b byte) bool {
	hi, lo := max(r, g, b), min(r, g, b)
	if hi <= 80 || hi == lo {
		return false
	}
	saturation := 255 * float64(hi-lo) / float64(hi)
	if saturation <= 100 {
		return false
	}
	delta := float64(hi - lo)
	var hue float64
	switch hi {
	case r:
		hue = (float64(g) - float64(b)) / delta
	case g:
		hue = 2 + (float64(b)-float64(r))/delta
	default:
		hue = 4 + (float64(r)-float64(g))/delta
	}
	hue *= 255.0 / 6
	if hue < 0 {
		hue += 255
	}
	return hue < 40 || hue > 210
}
