// Package printer provides cancellable Brother raster transports and printing
// sessions. It never retries a job after partial transmission.
package printer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrStatusUnsupported = errors.New("printer transport does not support confirmed status")
	ErrUSBUnavailable    = errors.New("USB support is unavailable; install libusb and build with CGO_ENABLED=1 and -tags libusb")
	ErrMediaMismatch     = errors.New("loaded printer media does not match the job")
	ErrClosed            = errors.New("printer client is closed")
	ErrUnusable          = errors.New("printer connection cannot be reused; close it and open a new client")
	ErrBusy              = errors.New("printer is already printing")
)

const (
	StatusReply        byte = 0x00
	StatusCompleted    byte = 0x01
	StatusError        byte = 0x02
	StatusOff          byte = 0x04
	StatusNotification byte = 0x05
	StatusPhaseChange  byte = 0x06
	CoolingStarted     byte = 0x03
	CoolingFinished    byte = 0x04
)

// Status is one 32-byte Brother status response. MediaType is the raw wire
// value: older models report 0x0a/0x0b, QL-800 series report 0x4a/0x4b.
// MediaWidth and MediaLength are millimetres; continuous media has zero length.
type Status struct {
	MediaWidth   int      `json:"media_width"`
	MediaLength  int      `json:"media_length"`
	MediaType    byte     `json:"media_type"`
	Type         byte     `json:"type"`
	Phase        byte     `json:"phase"`
	PhaseNumber  uint16   `json:"phase_number"`
	Notification byte     `json:"notification"`
	Errors       []string `json:"errors,omitempty"`
	Raw          [32]byte `json:"-"`
}

// ParseStatus parses exactly one status frame, retaining unknown flag bits as
// errors rather than silently discarding them. Its error reports invalid frame
// structure; use Status.Err to check whether a parsed frame reports a printer
// failure.
func ParseStatus(b []byte) (Status, error) {
	var s Status
	if len(b) != 32 || b[0] != 0x80 || b[1] != 0x20 || b[2] != 0x42 {
		return s, fmt.Errorf("invalid Brother status frame: expected 32 bytes with header 80 20 42")
	}
	copy(s.Raw[:], b)
	s.MediaWidth, s.MediaLength, s.MediaType = int(b[10]), int(b[17]), b[11]
	s.Type, s.Phase, s.PhaseNumber, s.Notification = b[18], b[19], binary.BigEndian.Uint16(b[20:22]), b[22]
	names := [2][8]string{
		{"no media", "end of media", "cutter jam", "unknown error 1 bit 3", "printer in use", "printer turned off", "high-voltage adapter error", "fan motor error"},
		{"replace media", "expansion buffer full", "communication error", "communication buffer full", "cover open", "print cancelled", "media cannot be fed", "system error"},
	}
	for group := range names {
		for bit, name := range names[group] {
			if b[8+group]&(1<<bit) != 0 {
				s.Errors = append(s.Errors, name)
			}
		}
	}
	return s, nil
}

// StatusErrorDetail preserves the status that caused a print to fail.
type StatusErrorDetail struct{ Status Status }

func (e *StatusErrorDetail) Error() string {
	if len(e.Status.Errors) > 0 {
		return "printer: " + strings.Join(e.Status.Errors, ", ")
	}
	if e.Status.Type == StatusOff {
		return "printer turned off"
	}
	return "printer reported an unspecified error"
}

// Err reports error flags, an error status, or a powered-off status.
func (s Status) Err() error {
	if len(s.Errors) > 0 || s.Type == StatusError || s.Type == StatusOff {
		return &StatusErrorDetail{Status: s}
	}
	return nil
}
