package printer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Device struct {
	URI    string `json:"uri"`
	Model  string `json:"model"`
	Serial string `json:"serial"`
}

// Open opens a tcp://host[:port], network://host[:port],
// usb://VID:PID[/SERIAL], or linux_kernel:///dev/usb/lpN endpoint.
// Bare /dev/usb/lpN paths are accepted. TCP defaults to port 9100.
// TCP does not support status or confirmed completion. Direct USB requires
// libusb, CGO_ENABLED=1, and the libusb build tag. The caller must close the client;
// ctx controls opening the connection, not the returned client's lifetime.
func Open(ctx context.Context, uri string) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var t Transport
	var err error
	switch {
	case strings.HasPrefix(uri, "tcp://"), strings.HasPrefix(uri, "network://"):
		var address string
		address, err = tcpAddress(uri)
		if err == nil {
			var conn net.Conn
			conn, err = (&net.Dialer{}).DialContext(ctx, "tcp", address)
			if err == nil {
				t = &tcpTransport{conn: conn}
			}
		}
	case strings.HasPrefix(uri, "usb://"):
		var selector usbSelector
		selector, err = parseUSBSelector(uri)
		if err == nil {
			t, err = openUSB(ctx, selector)
		}
	case strings.HasPrefix(uri, "/dev/"), strings.HasPrefix(uri, "linux_kernel://"):
		t, err = openKernel(ctx, strings.TrimPrefix(uri, "linux_kernel://"))
	default:
		return nil, fmt.Errorf("unsupported printer URI %q; use tcp://, usb://, or linux_kernel:///dev/usb/lpN", uri)
	}
	if err != nil {
		return nil, fmt.Errorf("open printer %q: %w", uri, err)
	}
	return NewClient(t), nil
}

// Discover enumerates USB printers or Linux kernel device nodes. It does not
// send print data, claim interfaces, or probe network hosts.
// backend must be "usb" or "linux_kernel".
func Discover(ctx context.Context, backend string) ([]Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	switch backend {
	case "usb":
		return discoverUSB(ctx)
	case "linux_kernel":
		return discoverKernel(ctx)
	default:
		return nil, fmt.Errorf("unknown discovery backend %q; use usb or linux_kernel", backend)
	}
}

func tcpAddress(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if (u.Scheme != "tcp" && u.Scheme != "network") || u.Hostname() == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", errors.New("expected tcp://hostname[:port] or tcp://[IPv6][:port]")
	}
	if strings.Contains(u.Hostname(), ":") && !strings.HasPrefix(u.Host, "[") {
		return "", errors.New("IPv6 addresses must be enclosed in brackets")
	}
	port := u.Port()
	if port == "" {
		if strings.HasSuffix(u.Host, ":") {
			return "", errors.New("empty TCP port")
		}
		port = "9100"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("TCP port must be between 1 and 65535")
	}
	return net.JoinHostPort(u.Hostname(), port), nil
}

type tcpTransport struct{ conn net.Conn }

func (t *tcpTransport) SupportsStatus() bool { return false }
func (t *tcpTransport) Close() error         { return t.conn.Close() }
func (t *tcpTransport) Read(ctx context.Context, p []byte) (int, error) {
	return connectionIO(ctx, t.conn.SetReadDeadline, func() (int, error) { return t.conn.Read(p) })
}
func (t *tcpTransport) Write(ctx context.Context, p []byte) (int, error) {
	return connectionIO(ctx, t.conn.SetWriteDeadline, func() (int, error) { return t.conn.Write(p) })
}

func connectionIO(ctx context.Context, setDeadline func(time.Time) error, perform func() (int, error)) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deadline, _ := ctx.Deadline()
	if err := setDeadline(deadline); err != nil {
		return 0, err
	}
	cancelled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { _ = setDeadline(time.Now()); close(cancelled) })
	n, err := perform()
	if !stop() {
		<-cancelled
	}
	if ctx.Err() != nil {
		err = ctx.Err()
	} else if timeout, ok := err.(net.Error); ok && timeout.Timeout() && !deadline.IsZero() && !time.Now().Before(deadline) {
		// The socket deadline and context timer can become runnable together.
		// Preserve context error semantics even if the network poller runs first.
		err = context.DeadlineExceeded
	}
	return n, err
}

type usbSelector struct {
	vendor, product uint16
	serial          string
}

func (s usbSelector) matches(vendor, product uint16, serial string) bool {
	return s.vendor == vendor && s.product == product && (s.serial == "" || s.serial == serial)
}

func selectUSBDevice(selector usbSelector, candidates []usbSelector) (int, error) {
	selected := -1
	for i, candidate := range candidates {
		if !selector.matches(candidate.vendor, candidate.product, candidate.serial) {
			continue
		}
		if selected >= 0 {
			return -1, errors.New("USB selector matches multiple printers; include the serial number from discover")
		}
		selected = i
	}
	if selected < 0 {
		return -1, errors.New("no USB printer matches the requested vendor, product and serial")
	}
	return selected, nil
}
func parseUSBSelector(raw string) (usbSelector, error) {
	var s usbSelector
	if !strings.HasPrefix(raw, "usb://") {
		return s, errors.New("expected usb://VID:PID[/SERIAL]")
	}
	// USB identifiers are hexadecimal pairs, not URL host:port numbers.
	authority, serial, _ := strings.Cut(strings.TrimPrefix(raw, "usb://"), "/")
	if strings.ContainsAny(raw, "?#@") {
		return s, errors.New("USB URI cannot contain a query, fragment or user information")
	}
	vendor, product, ok := strings.Cut(authority, ":")
	if !ok {
		return s, errors.New("USB URI requires hexadecimal VID:PID")
	}
	v, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(vendor), "0x"), 16, 16)
	if err != nil {
		return s, fmt.Errorf("invalid USB vendor ID: %w", err)
	}
	p, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(product), "0x"), 16, 16)
	if err != nil {
		return s, fmt.Errorf("invalid USB product ID: %w", err)
	}
	decoded, err := url.PathUnescape(serial)
	if err != nil {
		return s, fmt.Errorf("invalid USB serial: %w", err)
	}
	if strings.ContainsRune(decoded, 0) {
		return s, errors.New("USB serial cannot contain a NUL byte")
	}
	return usbSelector{uint16(v), uint16(p), decoded}, nil
}

func usbURI(vendor, product uint16, serial string) string {
	uri := fmt.Sprintf("usb://0x%04x:0x%04x", vendor, product)
	if serial != "" {
		uri += "/" + url.PathEscape(serial)
	}
	return uri
}
