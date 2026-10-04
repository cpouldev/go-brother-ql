//go:build linux

package printer

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

type kernelTransport struct{ fd int }

func openKernel(ctx context.Context, path string) (Transport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) {
		return nil, errors.New("kernel device path must be absolute")
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open USB device file (check usblp and device permissions): %w", err)
	}
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFCHR {
		_ = unix.Close(fd)
		return nil, errors.New("kernel printer path must name a character device")
	}
	return &kernelTransport{fd: fd}, nil
}

func discoverKernel(ctx context.Context) ([]Device, error) {
	paths, err := filepath.Glob("/dev/usb/lp*")
	if err != nil {
		return nil, err
	}
	var result []Device
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var stat unix.Stat_t
		if err := unix.Stat(path, &stat); err != nil {
			return result, err
		}
		if stat.Mode&unix.S_IFMT == unix.S_IFCHR {
			result = append(result, Device{URI: "linux_kernel://" + path})
		}
	}
	return result, nil
}

func (t *kernelTransport) SupportsStatus() bool { return true }
func (t *kernelTransport) Close() error         { return unix.Close(t.fd) }
func (t *kernelTransport) Read(ctx context.Context, b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}
	for {
		n, err := kernelIO(ctx, func() (int, error) { return unix.Read(t.fd, b) })
		if n != 0 || err != nil {
			return n, err
		}
		// usblp exposes cooling zero-length USB packets as successful reads;
		// disconnection is reported as ENODEV, not an EOF-like zero packet.
		if err := kernelDelay(ctx); err != nil {
			return 0, err
		}
	}
}
func (t *kernelTransport) Write(ctx context.Context, b []byte) (int, error) {
	n, err := kernelIO(ctx, func() (int, error) { return unix.Write(t.fd, b) })
	if err == nil && n > 0 {
		err = waitKernelWritable(ctx, t.fd)
	}
	return n, err
}

func waitKernelWritable(ctx context.Context, fd int) error {
	// usblp's nonblocking write can return before its USB transfer completes.
	// POLLOUT is the driver's documented drain barrier before closing. See
	// drivers/usb/class/usblp.c, usblp_wwait, in the Linux source tree.
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLOUT}}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := unix.Poll(fds, 20)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		if fds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			return errors.New("kernel printer disconnected or failed while draining USB output")
		}
		if fds[0].Revents&unix.POLLOUT != 0 {
			return nil
		}
	}
}

func kernelDelay(ctx context.Context) error {
	timer := time.NewTimer(5 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func kernelIO(ctx context.Context, operation func() (int, error)) (int, error) {
	for {
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		n, err := operation()
		if n > 0 {
			return n, err
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EAGAIN) && !errors.Is(err, unix.EWOULDBLOCK) {
			if n < 0 {
				n = 0
			}
			return n, err
		}
		// Nonblocking I/O avoids leaving a goroutine stuck inside a device read.
		if err := kernelDelay(ctx); err != nil {
			return 0, err
		}
	}
}
