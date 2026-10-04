// SPDX-License-Identifier: GPL-3.0-or-later
package packbits

import (
	"bytes"
	"testing"
)

func TestDecodeVectors(t *testing.T) {
	tests := []struct {
		name      string
		src, want []byte
	}{
		{"literal", []byte{2, 10, 20, 30}, []byte{10, 20, 30}},
		{"repeat", []byte{254, 9}, []byte{9, 9, 9}},
		{"noop", []byte{128, 0, 7, 128}, []byte{7}},
		{"max repeat", []byte{129, 42}, bytes.Repeat([]byte{42}, 128)},
		{"empty", []byte{128}, []byte{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Decode(tt.src, len(tt.want))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("got %x, want %x", got, tt.want)
			}
		})
	}
}

func TestDecodeRejectsMalformed(t *testing.T) {
	for _, tt := range []struct {
		name  string
		src   []byte
		width int
	}{
		{"short literal", []byte{2, 1, 2}, 3}, {"short repeat", []byte{255}, 2},
		{"overflow repeat", []byte{129, 0}, 127}, {"overflow literal", []byte{1, 0, 0}, 1},
		{"underflow", []byte{0, 0}, 2}, {"negative width", nil, -1}, {"huge width", nil, 1 << 30},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decode(tt.src, tt.width); err == nil {
				t.Fatal("expected malformed input error")
			}
		})
	}
}

func TestEncodeRunBoundaries(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 127, 128, 129, 255, 256, 257} {
		src := bytes.Repeat([]byte{42}, n)
		encoded := Encode(src)
		got, err := Decode(encoded, n)
		if err != nil || !bytes.Equal(got, src) {
			t.Fatalf("length %d: %x, %v", n, got, err)
		}
		if n == 128 && !bytes.Equal(encoded, []byte{129, 42}) {
			t.Fatalf("128-byte repeat encoding: %x", encoded)
		}
	}
}

func TestDecodeExpansionLimit(t *testing.T) {
	encoded := bytes.Repeat([]byte{129, 0}, MaxDecodedSize/128+1)
	if _, err := Decode(encoded, MaxDecodedSize); err == nil {
		t.Fatal("expected expansion-limit error")
	}
}

func TestEncodeLiteralBoundaries(t *testing.T) {
	for _, n := range []int{127, 128, 129, 255, 256, 257} {
		src := make([]byte, n)
		for i := range src {
			src[i] = byte(i)
		}
		got, err := Decode(Encode(src), n)
		if err != nil || !bytes.Equal(got, src) {
			t.Fatalf("length %d: %x, %v", n, got, err)
		}
	}
}

func FuzzRoundTrip(f *testing.F) {
	for _, src := range [][]byte{nil, {0}, {128}, bytes.Repeat([]byte{1}, 129), {1, 2, 3, 3, 3, 4}} {
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src []byte) {
		if len(src) > 4096 {
			t.Skip()
		}
		got, err := Decode(Encode(src), len(src))
		if err != nil || !bytes.Equal(got, src) {
			t.Fatalf("roundtrip: %v", err)
		}
	})
}

func FuzzDecode(f *testing.F) {
	f.Add([]byte{128}, uint16(0))
	f.Add([]byte{129, 1}, uint16(128))
	f.Add([]byte{127}, uint16(1))
	f.Fuzz(func(t *testing.T, src []byte, width uint16) {
		if len(src) > 8192 {
			t.Skip()
		}
		got, err := Decode(src, int(width))
		if err == nil && len(got) != int(width) {
			t.Fatalf("length=%d, want %d", len(got), width)
		}
	})
}
