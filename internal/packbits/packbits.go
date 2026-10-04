// SPDX-License-Identifier: GPL-3.0-or-later

// Package packbits implements the TIFF PackBits byte encoding used by Brother
// raster printers. The decoder is restricted to printer-sized rows.
package packbits

import "fmt"

// MaxDecodedSize bounds a decoded row, including malformed expansion attempts.
const MaxDecodedSize = 4096

// Encode returns a PackBits representation of src. Runs and literal groups are
// at most 128 bytes, and the reserved 0x80 no-op is never emitted.
func Encode(src []byte) []byte {
	dst := make([]byte, 0, len(src)+len(src)/128+1)
	for pos := 0; pos < len(src); {
		run := runLength(src[pos:])
		if run >= 3 {
			dst = append(dst, byte(257-run), src[pos])
			pos += run
			continue
		}
		start := pos
		pos += run
		for pos < len(src) && pos-start < 128 {
			run = runLength(src[pos:])
			if run >= 3 {
				break
			}
			pos += min(run, 128-(pos-start))
		}
		dst = append(dst, byte(pos-start-1))
		dst = append(dst, src[start:pos]...)
	}
	return dst
}

func runLength(src []byte) int {
	n := 1
	for n < len(src) && n < 128 && src[n] == src[0] {
		n++
	}
	return n
}

// DecodedLen validates src and returns its decoded length without allocating.
// A 0x80 control byte is a no-op, not a repeat of 129 bytes.
func DecodedLen(src []byte) (int, error) {
	total := 0
	for i := 0; i < len(src); {
		control := src[i]
		i++
		var size int
		switch {
		case control < 128:
			size = int(control) + 1
			if len(src)-i < size {
				return 0, fmt.Errorf("packbits: truncated literal at byte %d", i-1)
			}
			i += size
		case control > 128:
			if i == len(src) {
				return 0, fmt.Errorf("packbits: truncated repeat at byte %d", i-1)
			}
			size = 257 - int(control)
			i++
		}
		total += size
		if total > MaxDecodedSize {
			return 0, fmt.Errorf("packbits: decoded row exceeds %d-byte limit", MaxDecodedSize)
		}
	}
	return total, nil
}

// Decode decodes exactly expectedWidth bytes. Truncated packets, excessive
// expansion and an unexpected decoded width return errors before allocation.
func Decode(src []byte, expectedWidth int) ([]byte, error) {
	if expectedWidth < 0 || expectedWidth > MaxDecodedSize {
		return nil, fmt.Errorf("packbits: expected width outside 0..%d", MaxDecodedSize)
	}
	n, err := DecodedLen(src)
	if err != nil {
		return nil, err
	}
	if n != expectedWidth {
		return nil, fmt.Errorf("packbits: decoded width %d, expected %d", n, expectedWidth)
	}
	dst := make([]byte, 0, n)
	for i := 0; i < len(src); {
		control := src[i]
		i++
		switch {
		case control < 128:
			n := int(control) + 1
			dst = append(dst, src[i:i+n]...)
			i += n
		case control > 128:
			for n := 257 - int(control); n > 0; n-- {
				dst = append(dst, src[i])
			}
			i++
		}
	}
	return dst, nil
}
