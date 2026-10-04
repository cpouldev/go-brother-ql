//go:build linux

package printer

import (
	"context"
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestKernelRejectsRegularFile(t *testing.T) {
	name := filepath.Join(t.TempDir(), "not-a-printer")
	if err := os.WriteFile(name, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := openKernel(context.Background(), name); err == nil {
		t.Fatal("accepted regular file")
	}
	b, err := os.ReadFile(name)
	if err != nil || string(b) != "keep me" {
		t.Fatal("modified file")
	}
}

func TestKernelReadCancellation(t *testing.T) {
	var fds [2]int
	if err := unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	k := &kernelTransport{fd: fds[0]}
	defer k.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := k.Read(ctx, make([]byte, 32)); done <- err }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("read ignored cancellation")
	}
}

func TestKernelReadWrite(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[1])
	k := &kernelTransport{fd: fds[0]}
	defer k.Close()
	if _, err := unix.Write(fds[1], []byte("reply")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 32)
	n, err := k.Read(timeout(t), buf)
	if err != nil || string(buf[:n]) != "reply" {
		t.Fatalf("read=%q error=%v", buf[:n], err)
	}
	if n, err := k.Write(timeout(t), []byte("raster")); err != nil || n != 6 {
		t.Fatalf("write=%d error=%v", n, err)
	}
	n, err = unix.Read(fds[1], buf)
	if err != nil || string(buf[:n]) != "raster" {
		t.Fatalf("received=%q error=%v", buf[:n], err)
	}
}

func TestKernelZeroPacketWaitsForContext(t *testing.T) {
	fd, err := unix.Open("/dev/null", unix.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	k := &kernelTransport{fd: fd}
	defer k.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := k.Read(ctx, make([]byte, 32)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("zero packet read returned %v", err)
	}
}

func TestKernelDrainCancellation(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM|unix.SOCK_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unix.Close(fds[0])
	defer unix.Close(fds[1])
	buf := make([]byte, 65536)
	for {
		_, err := unix.Write(fds[0], buf)
		if errors.Is(err, unix.EAGAIN) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := waitKernelWritable(ctx, fds[0]); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("drain returned %v", err)
	}
}
