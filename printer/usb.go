//go:build libusb && cgo

package printer

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/gousb"
)

// USBEnabled reports whether this build includes libusb support.
func USBEnabled() bool { return true }

type usbInterface struct{ config, number, alternate, in, out int }

// Use descriptor numbers, not slice positions; composite USB devices may have
// their printer interface at a nonzero index or alternate setting.
func printerInterface(desc *gousb.DeviceDesc) (usbInterface, error) {
	configs := make([]int, 0, len(desc.Configs))
	for id := range desc.Configs {
		configs = append(configs, id)
	}
	sort.Ints(configs)
	for _, id := range configs {
		config := desc.Configs[id]
		for _, intf := range config.Interfaces {
			for _, alt := range intf.AltSettings {
				if alt.Class != gousb.ClassPrinter {
					continue
				}
				in, out := -1, -1
				for _, ep := range alt.Endpoints {
					if ep.TransferType != gousb.TransferTypeBulk {
						continue
					}
					if ep.Direction == gousb.EndpointDirectionIn {
						if in < 0 || ep.Number < in {
							in = ep.Number
						}
					} else {
						if out < 0 || ep.Number < out {
							out = ep.Number
						}
					}
				}
				if in >= 0 && out >= 0 {
					return usbInterface{config.Number, alt.Number, alt.Alternate, in, out}, nil
				}
			}
		}
	}
	return usbInterface{}, errors.New("no bidirectional USB printer interface with bulk endpoints")
}

func usbError(err error) error {
	if errors.Is(err, gousb.ErrorAccess) {
		return fmt.Errorf("USB access denied; check Linux udev permissions or macOS USB access: %w", err)
	}
	if errors.Is(err, gousb.ErrorBusy) {
		return fmt.Errorf("USB printer is busy; close other printing software or release the interface: %w", err)
	}
	return err
}

func openUSB(ctx context.Context, selector usbSelector) (Transport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	usb := gousb.NewContext()
	devices, err := usb.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		return ctx.Err() == nil && uint16(desc.Vendor) == selector.vendor && uint16(desc.Product) == selector.product
	})
	cleanup := func() {
		for _, d := range devices {
			_ = d.Close()
		}
		_ = usb.Close()
	}
	if err != nil {
		cleanup()
		return nil, usbError(err)
	}
	var candidates []usbSelector
	var candidateDevices []*gousb.Device
	for _, d := range devices {
		if err := ctx.Err(); err != nil {
			cleanup()
			return nil, err
		}
		if _, err := printerInterface(d.Desc); err != nil {
			continue
		}
		serial := ""
		if selector.serial != "" {
			serial, err = d.SerialNumber()
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("cannot verify requested USB serial: %w", usbError(err))
			}
		}
		candidates = append(candidates, usbSelector{uint16(d.Desc.Vendor), uint16(d.Desc.Product), serial})
		candidateDevices = append(candidateDevices, d)
	}
	index, err := selectUSBDevice(selector, candidates)
	if err != nil {
		cleanup()
		return nil, err
	}
	selected := candidateDevices[index]
	for _, d := range devices {
		if d != selected {
			_ = d.Close()
		}
	}
	devices = []*gousb.Device{selected}
	iface, err := printerInterface(selected.Desc)
	if err != nil {
		cleanup()
		return nil, err
	}
	if err := selected.SetAutoDetach(true); err != nil && !errors.Is(err, gousb.ErrorNotSupported) {
		cleanup()
		return nil, usbError(err)
	}
	config, err := selected.Config(iface.config)
	if err != nil {
		cleanup()
		return nil, usbError(err)
	}
	intf, err := config.Interface(iface.number, iface.alternate)
	if err != nil {
		_ = config.Close()
		cleanup()
		return nil, usbError(err)
	}
	in, err := intf.InEndpoint(iface.in)
	if err != nil {
		intf.Close()
		_ = config.Close()
		cleanup()
		return nil, err
	}
	out, err := intf.OutEndpoint(iface.out)
	if err != nil {
		intf.Close()
		_ = config.Close()
		cleanup()
		return nil, err
	}
	transport := &usbTransport{usb: usb, device: selected, config: config, intf: intf, in: in, out: out}
	if err := ctx.Err(); err != nil {
		_ = transport.Close()
		return nil, err
	}
	return transport, nil
}

func discoverUSB(ctx context.Context) ([]Device, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	usb := gousb.NewContext()
	defer usb.Close()
	devices, enumerationErr := usb.OpenDevices(func(desc *gousb.DeviceDesc) bool {
		if ctx.Err() != nil || uint16(desc.Vendor) != 0x04f9 {
			return false
		}
		_, err := printerInterface(desc)
		return err == nil
	})
	defer func() {
		for _, d := range devices {
			_ = d.Close()
		}
	}()
	var result []Device
	errs := []error{usbError(enumerationErr)}
	for _, d := range devices {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		serial, serialErr := d.SerialNumber()
		model, modelErr := d.Product()
		result = append(result, Device{URI: usbURI(uint16(d.Desc.Vendor), uint16(d.Desc.Product), serial), Model: strings.TrimSpace(strings.TrimPrefix(model, "Brother")), Serial: serial})
		if serialErr != nil {
			errs = append(errs, fmt.Errorf("read USB serial: %w", usbError(serialErr)))
		}
		if modelErr != nil {
			errs = append(errs, fmt.Errorf("read USB model: %w", usbError(modelErr)))
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].URI < result[j].URI })
	return result, errors.Join(errs...)
}

type usbTransport struct {
	usb    *gousb.Context
	device *gousb.Device
	config *gousb.Config
	intf   *gousb.Interface
	in     *gousb.InEndpoint
	out    *gousb.OutEndpoint
}

func (t *usbTransport) SupportsStatus() bool { return true }
func (t *usbTransport) Read(ctx context.Context, b []byte) (int, error) {
	for {
		n, err := t.in.ReadContext(ctx, b)
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		if n != 0 || err != nil {
			return n, usbError(err)
		}
		// Zero-length packets are expected during cooling. Avoid a tight loop,
		// keeping the context deadline as the bound rather than a retry count.
		timer := time.NewTimer(time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0, ctx.Err()
		case <-timer.C:
		}
	}
}
func (t *usbTransport) Write(ctx context.Context, b []byte) (int, error) {
	n, err := t.out.WriteContext(ctx, b)
	if ctx.Err() != nil {
		return n, ctx.Err()
	}
	return n, usbError(err)
}
func (t *usbTransport) Close() error {
	t.intf.Close()
	return errors.Join(t.config.Close(), t.device.Close(), t.usb.Close())
}
