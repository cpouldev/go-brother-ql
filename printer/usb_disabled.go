//go:build !libusb || !cgo

package printer

import "context"

// USBEnabled reports whether this build includes libusb support.
func USBEnabled() bool                                        { return false }
func openUSB(context.Context, usbSelector) (Transport, error) { return nil, ErrUSBUnavailable }
func discoverUSB(context.Context) ([]Device, error)           { return nil, ErrUSBUnavailable }
