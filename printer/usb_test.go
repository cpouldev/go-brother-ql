//go:build libusb && cgo

package printer

import (
	"github.com/google/gousb"
	"testing"
)

func TestUSBPrinterInterfaceSelection(t *testing.T) {
	endpoints := map[gousb.EndpointAddress]gousb.EndpointDesc{
		0x81: {Number: 1, Direction: gousb.EndpointDirectionIn, TransferType: gousb.TransferTypeInterrupt},
		0x83: {Number: 3, Direction: gousb.EndpointDirectionIn, TransferType: gousb.TransferTypeBulk},
		0x04: {Number: 4, Direction: gousb.EndpointDirectionOut, TransferType: gousb.TransferTypeBulk},
	}
	desc := &gousb.DeviceDesc{Configs: map[int]gousb.ConfigDesc{2: {Number: 2, Interfaces: []gousb.InterfaceDesc{{Number: 7, AltSettings: []gousb.InterfaceSetting{{Number: 7, Alternate: 1, Class: gousb.ClassPrinter, Endpoints: endpoints}}}}}}}
	selected, err := printerInterface(desc)
	if err != nil {
		t.Fatal(err)
	}
	if selected.config != 2 || selected.number != 7 || selected.alternate != 1 || selected.in != 3 || selected.out != 4 {
		t.Fatalf("wrong interface: %+v", selected)
	}
	delete(endpoints, 0x83)
	if _, err := printerInterface(desc); err == nil {
		t.Fatal("accepted unidirectional interface")
	}
}
