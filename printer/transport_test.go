package printer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestTCPDelivery(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	received := make(chan []byte, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b, _ := io.ReadAll(conn)
		received <- b
	}()
	c, err := Open(timeout(t), "tcp://"+l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Print(timeout(t), bytes.NewBufferString("raster"), PrintOptions{})
	c.Close()
	if err != nil || got.Outcome != "sent" {
		t.Fatalf("result=%+v err=%v", got, err)
	}
	select {
	case b := <-received:
		if string(b) != "raster" {
			t.Fatalf("received %q", b)
		}
	case <-time.After(time.Second):
		t.Fatal("no TCP data")
	}
}

func TestTCPAddress(t *testing.T) {
	for input, want := range map[string]string{"tcp://printer": "printer:9100", "network://printer:123": "printer:123", "tcp://[::1]": "[::1]:9100", "tcp://[::1]:9101": "[::1]:9101"} {
		got, err := tcpAddress(input)
		if err != nil || got != want {
			t.Fatalf("%s => %q,%v want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"tcp://", "tcp://user:pass@host", "tcp://host/path", "tcp://host?x=1", "tcp://host:0", "tcp://host:65536", "tcp://::1"} {
		if _, err := tcpAddress(input); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
}

func TestUSBSelectorSerial(t *testing.T) {
	s, err := parseUSBSelector("usb://0x04f9:0x209b/ABC%2F123")
	if err != nil {
		t.Fatal(err)
	}
	if !s.matches(0x04f9, 0x209b, "ABC/123") || s.matches(0x04f9, 0x209b, "other") || s.matches(0x04f9, 0x2015, "ABC/123") {
		t.Fatalf("selector matched wrong device: %+v", s)
	}
	for _, uri := range []string{"usb://", "usb://04f9", "usb://zzzz:1234", "usb://04f9:10000", "usb://04f9:209b/?x=y"} {
		if _, err := parseUSBSelector(uri); err == nil {
			t.Fatalf("accepted %q", uri)
		}
	}
}

func TestUSBSelectionRejectsAmbiguity(t *testing.T) {
	candidates := []usbSelector{{0x04f9, 0x209b, "FIRST"}, {0x04f9, 0x209b, "SECOND"}, {0x1234, 0x5678, "SECOND"}}
	if _, err := selectUSBDevice(usbSelector{vendor: 0x04f9, product: 0x209b}, candidates); err == nil {
		t.Fatal("accepted ambiguous USB selector")
	}
	i, err := selectUSBDevice(usbSelector{0x04f9, 0x209b, "SECOND"}, candidates)
	if err != nil || i != 1 {
		t.Fatalf("selected %d: %v", i, err)
	}
	if _, err := selectUSBDevice(usbSelector{0x04f9, 0x2015, "SECOND"}, candidates); err == nil {
		t.Fatal("matched serial with the wrong product")
	}
}

func TestCancelledOpen(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, "tcp://localhost:9100"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

func TestTCPBlockedWriteCancellation(t *testing.T) {
	conn, peer := net.Pipe()
	defer peer.Close()
	c := NewClient(&tcpTransport{conn: conn})
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	result, err := c.Print(ctx, bytes.NewReader(make([]byte, 32768)), PrintOptions{})
	if !errors.Is(err, context.DeadlineExceeded) || result.BytesSent != 0 {
		t.Fatalf("result=%+v error=%v", result, err)
	}
}
