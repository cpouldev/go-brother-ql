package printer

import (
	"bytes"
	"testing"
)

func statusFrame(kind, phase, notification byte) []byte {
	b := make([]byte, 32)
	copy(b, []byte{0x80, 0x20, 0x42, 0x34, 0x38, 0x30, 0x30})
	b[10], b[11], b[18], b[19], b[22] = 62, 0x4a, kind, phase, notification
	return b
}

func TestParseStatus(t *testing.T) {
	b := statusFrame(5, 1, 3)
	b[8], b[9], b[20], b[21] = 4, 16, 1, 2
	s, err := ParseStatus(b)
	if err != nil {
		t.Fatal(err)
	}
	if s.MediaWidth != 62 || s.MediaType != 0x4a || s.PhaseNumber != 258 || s.Notification != 3 {
		t.Fatalf("incorrect status: %+v", s)
	}
	if len(s.Errors) != 2 || s.Err() == nil {
		t.Fatalf("errors not decoded: %+v", s)
	}
	b[10] = 29
	if s.Raw[10] != 62 {
		t.Fatal("raw status aliases input")
	}
}

func TestParseStatusRejectsMalformed(t *testing.T) {
	for _, b := range [][]byte{nil, statusFrame(0, 0, 0)[:31], append(statusFrame(0, 0, 0), 0), bytes.Repeat([]byte{0}, 32)} {
		if _, err := ParseStatus(b); err == nil {
			t.Fatalf("accepted malformed frame %x", b)
		}
	}
}

func FuzzParseStatus(f *testing.F) {
	f.Add(statusFrame(0, 0, 0))
	f.Add([]byte{0x80, 0x20})
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := ParseStatus(b)
		if err == nil && (len(b) != 32 || !bytes.Equal(s.Raw[:], b)) {
			t.Fatal("invalid accepted frame")
		}
	})
}
