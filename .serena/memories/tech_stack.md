# Toolchain

- Use go.mod's Go requirement; PATH Go may be older and auto-select a newer toolchain. Do not lower requirements as a setup workaround.
- Image conversion/protocol/analysis/TCP are pure Go. Direct USB requires CGO + build tag libusb + system libusb/pkg-config (gousb). Default builds deliberately return ErrUSBUnavailable for usb://.
- Cobra CLI; x/image resizing and BMP/TIFF; x/sys Linux device operations.
- USB data supports context cancellation; gousb enumeration/control metadata calls are synchronous. Generic caller io.Reader cancellation remains caller-owned.
- Serena Go language server; project settings .serena/project.yml, machine overrides .serena/project.local.yml.
