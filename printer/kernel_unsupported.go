//go:build !linux

package printer

import (
	"context"
	"errors"
)

func openKernel(context.Context, string) (Transport, error) {
	return nil, errors.New("kernel USB device files are supported only on Linux; use libusb on macOS")
}
func discoverKernel(context.Context) ([]Device, error) {
	return nil, errors.New("kernel USB discovery is supported only on Linux")
}
