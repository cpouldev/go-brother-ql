// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/cpouldev/go-brother-ql/raster"
)

func receiveTCP(t *testing.T) (string, <-chan []byte) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	received := make(chan []byte, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			received <- nil
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		data, _ := io.ReadAll(conn)
		received <- data
	}()
	return "tcp://" + l.Addr().String(), received
}

func TestPrintTCPReportsSent(t *testing.T) {
	uri, received := receiveTCP(t)
	out, stderr, err := run(t, imageInput(t), "print", "-m", "QL-800", "-l", "62", "-p", uri, "--json", "--copies", "2", "-")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Outcome      string `json:"outcome"`
		PagesPrinted int    `json:"pages_printed"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatal(err)
	}
	if result.Outcome != "sent" || result.PagesPrinted != 0 {
		t.Fatalf("result: %s", out)
	}
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
	data := <-received
	pages, err := raster.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || pages[0].Height != 150 || pages[1].Height != 150 {
		t.Fatalf("invalid copied job: %v", pages)
	}
}

func TestInvalidImageBeforeConnection(t *testing.T) {
	_, _, err := run(t, nil, "print", "-m", "QL-800", "-l", "62", "-p", "tcp://127.0.0.1:1", "missing.png")
	if err == nil || !strings.Contains(err.Error(), "missing.png") {
		t.Fatalf("error: %v", err)
	}
}

func TestConnectionError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	uri := "tcp://" + l.Addr().String()
	_ = l.Close()
	_, _, err = run(t, imageInput(t), "print", "-m", "QL-800", "-l", "62", "-p", uri, "-")
	if err == nil {
		t.Fatal("expected connection error")
	}
}

func TestSendTCP(t *testing.T) {
	raw, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	uri, received := receiveTCP(t)
	out, _, err := run(t, raw, "send", "-p", uri, "-")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "sent") || strings.Contains(string(out), "printed") {
		t.Fatalf("delivery report: %s", out)
	}
	if data := <-received; !bytes.Equal(data, raw) {
		t.Fatal("send changed raster data")
	}
}

func TestMalformedRawBeforeConnection(t *testing.T) {
	_, _, err := run(t, []byte("broken"), "send", "-p", "tcp://127.0.0.1:1", "-")
	if err == nil || strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("error: %v", err)
	}
}

func TestNetworkStatusFails(t *testing.T) {
	uri, _ := receiveTCP(t)
	_, _, err := run(t, nil, "status", "-p", uri)
	if err == nil {
		t.Fatal("expected unsupported status error")
	}
}

func TestInvalidTimeout(t *testing.T) {
	_, _, err := run(t, nil, "status", "-p", "tcp://127.0.0.1:1", "--timeout", "0s")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("error: %v", err)
	}
}

func TestConflictingBackend(t *testing.T) {
	_, _, err := run(t, nil, "status", "-p", "tcp://127.0.0.1:1", "--backend", "usb")
	if err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("error: %v", err)
	}
}

func TestRawSendRejectsModelCapabilitiesBeforeConnecting(t *testing.T) {
	for _, tc := range []struct {
		name, source, target, want string
		flags                      []string
	}{
		{"compression", "QL-810W", "QL-800", "compression", []string{"--compress"}},
		{"red", "QL-800", "QL-700", "two-color", []string{"--red"}},
		{"expanded mode", "QL-800", "QL-500", "mode", []string{"--no-cut"}},
		{"high resolution", "QL-800", "QL-550", "high resolution", []string{"--600dpi"}},
		{"cut interval", "QL-800", "QL-650TD", "cut intervals", []string{"--cut", "every", "--cut-every", "2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := append([]string{"convert", "-m", tc.source, "-l", "62"}, tc.flags...)
			args = append(args, "-")
			raw, _, err := run(t, imageInput(t), args...)
			if err != nil {
				t.Fatal(err)
			}
			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer l.Close()
			_ = l.(*net.TCPListener).SetDeadline(time.Now().Add(100 * time.Millisecond))
			_, _, err = run(t, raw, "send", "-m", tc.target, "-p", "tcp://"+l.Addr().String(), "-")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted %s error, got %v", tc.want, err)
			}
			conn, err := l.Accept()
			if err == nil {
				conn.Close()
				t.Fatal("invalid raster opened a printer connection")
			}
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				t.Fatalf("accept: %v", err)
			}
		})
	}
}

func TestExperimentalPrintIsConversionOnly(t *testing.T) {
	_, _, err := run(t, nil, "print", "-m", "PT-P750W", "-l", "pt24", "-p", "tcp://127.0.0.1:1", "-")
	if err == nil || !strings.Contains(err.Error(), "conversion-only") {
		t.Fatalf("error: %v", err)
	}
}

func TestRawSendRejectsUnfinishedJobBeforeConnecting(t *testing.T) {
	raw, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] = 0x0c
	_, _, err = run(t, raw, "send", "-p", "tcp://127.0.0.1:1", "-")
	if err == nil || !strings.Contains(err.Error(), "final") {
		t.Fatalf("expected final-page error, got %v", err)
	}
}

func TestRawSendRejectsTrailingInstructions(t *testing.T) {
	raw, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, 0x1b, 0x69, 0x4b, 0)
	_, _, err = run(t, raw, "send", "-p", "tcp://127.0.0.1:1", "-")
	if err == nil || !strings.Contains(err.Error(), "final") {
		t.Fatalf("expected final-page error, got %v", err)
	}
}

func TestRawSendValidModel(t *testing.T) {
	for _, flags := range [][]string{nil, {"--red"}, {"--600dpi"}} {
		t.Run(strings.Join(flags, " "), func(t *testing.T) {
			args := append([]string{"convert", "-m", "QL-800", "-l", "62"}, flags...)
			args = append(args, "-")
			raw, _, err := run(t, imageInput(t), args...)
			if err != nil {
				t.Fatal(err)
			}
			uri, received := receiveTCP(t)
			_, _, err = run(t, raw, "send", "-m", "QL-800", "-p", uri, "-")
			if err != nil {
				t.Fatal(err)
			}
			if data := <-received; !bytes.Equal(data, raw) {
				t.Fatal("raw raster changed")
			}
		})
	}
}

func TestRawSendRejectsQL800ZeroRows(t *testing.T) {
	raw, _, err := run(t, imageInput(t), "convert", "-m", "QL-800", "-l", "62", "-")
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(raw, []byte{0x67, 0, 90})
	if start < 0 {
		t.Fatal("no raster rows")
	}
	raw = append(append(raw[:start:start], bytes.Repeat([]byte{'Z'}, 150)...), 0x1a)
	_, _, err = run(t, raw, "send", "-m", "QL-800", "-p", "tcp://127.0.0.1:1", "-")
	if err == nil || !strings.Contains(err.Error(), "zero raster") {
		t.Fatalf("expected zero-raster error, got %v", err)
	}
}
