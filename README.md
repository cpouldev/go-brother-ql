# go-brother-ql

A Go library and CLI for converting images into Brother QL raster instructions
and sending them over USB or TCP. The conversion library is pure Go; direct USB
access is an optional build using libusb.

This project is a port of **[brother_ql](https://github.com/pklaus/brother_ql)**,
created by **[Philipp Klaus](https://github.com/pklaus)**. Its protocol work,
printer and label catalogs, and community contributions made this port possible.
Thank you to Philipp and everyone who contributed to the original library.
The port is maintained by [cpouldev](https://github.com/cpouldev).

QL-800 USB on macOS and Linux is the primary hardware target.
Other catalog entries inherit upstream knowledge;
their presence is not a claim of hardware verification. The QL-800 itself has no
network interface; TCP is for network-capable models or a suitable print server.

## Features and improvements

- Image conversion, previews, raster decoding, model and label catalogs.
- PNG, JPEG, GIF (first frame), BMP, and TIFF (first image) CLI input.
- Monochrome and black/red output, thresholding, Floyd–Steinberg dithering,
  rotation, image fitting, 300 dpi and supported 300×600 dpi output.
- Multiple images and copies with intermediate/final page markers and cut policies.
- Capability checks reject incompatible media, compression, and color modes.
- USB serial selection, fragmented status responses, partial writes, cancellation,
  cooling notifications, and printer errors are handled explicitly.
- `sent`, `printed`, `failed`, and `unknown` outcomes distinguish delivery from
  confirmed completion. Interrupted jobs are never retried automatically.
- The CLI validates and spools image jobs before opening a printer, propagates
  errors as nonzero exits, and keeps binary stdout separate from diagnostics.

## Build

Requires Go 1.27 or newer. From this checkout:

```sh
# Pure Go: conversion, analysis, TCP, and Linux kernel USB device support.
go build -o bin/brother-ql ./cmd/brother-ql

# Or install into GOBIN, or GOPATH/bin when GOBIN is unset (normally ~/go/bin).
go install ./cmd/brother-ql
```

For direct USB on macOS:

```sh
brew install libusb pkg-config
CGO_ENABLED=1 go build -tags libusb -o bin/brother-ql ./cmd/brother-ql
```

For direct USB on Debian/Ubuntu:

```sh
sudo apt-get install libusb-1.0-0-dev pkg-config build-essential
CGO_ENABLED=1 go build -tags libusb -o bin/brother-ql ./cmd/brother-ql
```

Use `bin/brother-ql doctor` to inspect the build without contacting hardware.
Direct USB applications importing `printer` need the same build tag and native
dependency. Pure Go builds return a clear error for `usb://` URIs.

## CLI

The examples below use `bin/brother-ql` from the build above. Start with an
offline example that generates its own image:

```sh
go run ./examples/convert > example.bin
bin/brother-ql -m QL-800 analyze example.bin --output-prefix example-
```

This creates `example-0001.png`, a 720×300 image with two black bars, and reports
one page on 62 mm continuous media. No printer or source image is needed.

To convert your own artwork, replace `label.png` below with your image path:

```sh
bin/brother-ql info models
bin/brother-ql info labels
bin/brother-ql -m QL-800 preview -l 62 label.png -o preview.png
bin/brother-ql -m QL-800 convert -l 62 label.png -o label.bin
bin/brother-ql analyze label.bin --json
bin/brother-ql analyze label.bin --output-prefix decoded-
```

`analyze` writes `decoded-0001.png`, etc. Previews include the print head margins
and actual black/red pixels. They do not show paper texture or simulate heat.

With a libusb build, discover USB devices, then replace the example URI below
with the exact URI returned, including the serial:

```sh
bin/brother-ql discover -b usb
export BROTHER_QL_PRINTER='usb://0x04f9:0x209b/SERIAL'
bin/brother-ql status
bin/brother-ql -m QL-800 print -l 62 label.png
```

Other printing examples use that printer default. Replace image filenames,
network addresses, and device paths with your own values:

```sh
# Black/red stock; requires the compatible 62 mm roll.
bin/brother-ql -m QL-800 -p "$BROTHER_QL_PRINTER" print -l 62red label.png

# Two consecutive copies of each image, cutting at the end of the job.
bin/brother-ql -m QL-800 -p "$BROTHER_QL_PRINTER" \
  print -l 62 --copies 2 --cut end first.png second.png

# Network-capable printer: success means sent, not confirmed printed.
bin/brother-ql -m QL-820NWB -p tcp://192.0.2.10:9100 print -l 62 label.png

# Linux kernel USB device, using a build without libusb.
bin/brother-ql -m QL-800 -p /dev/usb/lp0 print -l 62 label.png

# Validate saved raster instructions for the target model; '-' reads stdin.
bin/brother-ql -m QL-800 -p "$BROTHER_QL_PRINTER" send label.bin
```

| Command | Purpose |
| --- | --- |
| `info models`, `info labels`, `info env` | List capabilities, media, or environment defaults |
| `discover` | Enumerate USB or Linux kernel devices |
| `status` | Read printer errors and loaded media over a supported transport |
| `print` | Convert and deliver image files |
| `convert`, `preview` | Write raster instructions or a PNG, using `-o FILE` or stdout |
| `send` | Validate and deliver a raster file |
| `analyze` | Decode pages and command metadata, optionally saving PNGs |
| `doctor` | Show platform and build capabilities without hardware access |
| `completion`, `version` | Shell completion and build version |

Use `COMMAND --help` for the full interface. `convert`, `preview`, and `print`
require a model and label; `print`, `send`, and `status` require a printer URI.
Set `BROTHER_QL_MODEL`, `BROTHER_QL_PRINTER`, `BROTHER_QL_LABEL`, and
`BROTHER_QL_BACKEND` for defaults. Explicit flags override environment values.
The backend is inferred from the URI unless specified. `network://host:9100`
is accepted as an alias for `tcp://host:9100`.

`send` validates the raster structure before connecting. Supply `--model` to
also check target capabilities and supported media; without it, choosing a
compatible target printer is your responsibility. `analyze` and `send` also use
the model's print head width to decode streams containing only blank rows.

`info models`, `info labels`, `info env`, `doctor`, `discover`, `status`, `analyze`,
`print`, and `send` support `--json`. `convert` and `preview` reject it. Printing
reports a result after opening the printer; validation or connection failures
can exit with only a diagnostic on stderr. Check the exit status as well as
the result's `outcome` when scripting.

### Image and print options

| Option | Behavior |
| --- | --- |
| `--rotate auto\|0\|90\|180\|270` | Counterclockwise; auto rotates an exact swapped-size die-cut image |
| `--fit auto` | Scale continuous stock to printable width; require exact die-cut dimensions |
| `--fit strict` | Require the expected width and, for die-cut stock, height |
| `--fit contain` | Center and scale inside die-cut stock; continuous stock behaves like auto |
| `--threshold 70` | Black threshold in percent, 0–100; ignored when dithering |
| `--dither` | Floyd–Steinberg black/white conversion |
| `--red` | Use compatible 62 mm black/red media; also selected by label `62red` |
| `--600dpi` | 300 dots/in across the head, 600 along the feed; supply artwork at 600 dpi |
| `--compress` | PackBits on models supporting it; QL-800 does not |
| `--lq` | Clear the high-quality flag |
| `--cut each\|end\|never\|every` | Default each, or never on models without a cutter; `--no-cut` aliases never |
| `--cut every --cut-every N` | Cut every 1–255 labels and at job end |
| `--copies N` | Consecutive copies of each image (`convert` and `print`) |
| `--timeout 60s` | Printer I/O deadline; does not limit image conversion |
| `--no-wait` | Return after delivery (`print` and `send`); USB otherwise waits for completion |

Transparency is composited on white. For label `62`, the printable width at
300 dpi is 696 pixels. For `62x29`, the expected image is 696×271 pixels.
At high resolution, supply twice those source dimensions; the output preserves
600 dpi vertically and reduces the horizontal dimension to the physical head.
Black/red media cannot use high resolution. Resampling uses Go's Catmull–Rom
filter, so resized and dithered pixels can differ from Pillow's output.
High resolution is enabled for QL-570, QL-580N, QL-700, QL-710W, QL-720NW,
QL-800, QL-810W, and QL-820NWB. The 11xx manuals conflict with their listed
specifications; that mode remains disabled for those profiles pending verification.
High-resolution previews and decoded PNGs retain raw raster dimensions, including
the doubled vertical dot count; ordinary image viewers may therefore show them tall.
QL-550 supports cutting each label but not end-only or interval cutting. QL-650TD
does not support configurable cut intervals; its cut-at-end mode requires newer
firmware (Brother does not specify the minimum version).

## Library

Import `github.com/cpouldev/go-brother-ql` as `brotherql`. The encoder takes an
`image.Image` and writes to any `io.Writer`. Decode input images using Go's
`image` packages or supply an image generated by your application.
This function spools one QL-800 label and returns the print result even on failure.
Pass a context with a deadline and a USB printer URI:

```go
package labels

import (
    "bytes"
    "context"
    "errors"
    "image"

    brotherql "github.com/cpouldev/go-brother-ql"
    "github.com/cpouldev/go-brother-ql/printer"
)

func PrintLabel(ctx context.Context, uri string, img image.Image) (printer.Result, error) {
    var job bytes.Buffer
    options := brotherql.Options{Model: "QL-800", Label: "62"}
    if err := brotherql.Encode(&job, []image.Image{img}, options); err != nil {
        return printer.Result{}, err
    }
    client, err := printer.Open(ctx, uri)
    if err != nil {
        return printer.Result{}, err
    }
    result, err := client.Print(ctx, &job, printer.PrintOptions{
        Wait: true, Pages: 1,
        Media: &printer.Media{Width: 62, Type: 0x0a},
    })
    return result, errors.Join(err, client.Close())
}
```

For TCP, set `Wait: false`. Requesting confirmed completion on TCP returns
`printer.ErrStatusUnsupported` before sending data. `Status` is also unsupported
over TCP; status/SNMP discovery is outside this version's scope.

Use `NewEncoder` and `WritePage(img, last)` to prepare pages one at a time. A
streaming writer can already contain earlier pages if a later page fails;
spool first when the whole job must be validated before delivery. `Preview`
returns the prepared image; `raster.Decode` independently reconstructs pages
from a raster stream. `DecodeWithOptions` can supply the head width for an
otherwise ambiguous stream consisting only of blank-row commands.

See the [compilable example](examples/convert/main.go) and package documentation
with `go doc` for the full API. A `printer.Client` serializes its jobs; use a
deadline for every hardware operation. Preserve `Result` when an error occurs:
`failed` can include completed pages, and `unknown` means completion is uncertain.
Inspect the physical output before deciding whether to retry.
Interrupted exchanges and bidirectional jobs sent with `Wait: false` make a client
unusable; close it and open a new client before another operation. This prevents
stale replies from being mistaken for the next job's status. A print source must
be finite or independently interruptible: a context cannot cancel an arbitrary
caller's `io.Reader`. USB data transfers support cancellation; gousb's synchronous
device enumeration and metadata calls are checked between operations.

## Compatibility and limits

The catalog includes QL-500, QL-550, QL-560, QL-570, QL-580N, QL-650TD, QL-700,
QL-710W, QL-720NW, QL-800, QL-810W, QL-820NWB, QL-1050, QL-1060N, QL-1100,
QL-1110NWB, and QL-1115NWB. PT-P750W and PT-P900W are experimental conversion and
preview profiles only. PT printing/status support requires separate protocol
work; the CLI rejects printing with these profiles. Run `info labels --json`
for all 27 media entries and restrictions.
Round labels retain a rectangular printable image; artwork must fit the circle.

The CLI accepts regular input files or `-` for stdin; `-` may appear only once
per image job. Each image input is limited to 128 MiB and 25 million decoded
pixels. Image jobs accept at most 100,000 pages, including copies. `send` and
`analyze` enforce the raster decoder's 128 MiB input limit and its expansion,
page, and metadata bounds. The library limits source/prepared images to
40 million pixels. CLI conversion, printing, and raster analysis use temporary
disk space.
Animated GIF frames and multipage TIFF pages are not expanded into print jobs;
pass separate images. Windows hardware support has not been tested.

Compared with Python `brother_ql`, the binary is `brother-ql`, `pyusb` is replaced
by the `usb` backend, and `analyze` provides the old analyzer functionality.
Common model/label/image flags and environment names are preserved. Error
handling and validation are deliberately stricter; this is not a drop-in Python
API or byte-identical port for every image-processing operation. The upstream
23×23 profile is corrected to 236×202 printable dots and the d12 feed to zero,
following Brother's raster manual.

## Troubleshooting and development

- Turn off Editor Lite on the QL-800 before opening its printer interface.
- For USB access errors, check device permissions and competing applications or
  print queues. On Linux, grant access through a narrowly scoped udev rule or
  use the kernel device's printer group; avoid running the whole application as root.
- Use the serial-specific URI from `discover` when several printers are attached.
- A reported media width, length, or type mismatch fails before job transmission
  on status-capable transports. These fields cannot identify every roll property;
  choose the correct stock, especially for black/red printing.
- A timeout after sending bytes is ambiguous. The library does not replay the job.
- Increase `--timeout` for long jobs or cooling. TCP cannot report media or
  completion status in this version.

Run `go test ./...`, `go test -race ./...`, and `go vet ./...`. With libusb installed,
also run `go test -race -tags libusb ./...`. The
[development notes](docs/DEVELOPMENT.md) record research, PDCA experiments,
benchmarks, and verification. The [hardware checklist](docs/HARDWARE.md) describes
QL-800 validation procedures. CI is configured for macOS and Linux.

## Attribution and license

Licensed under **GPL-3.0-or-later**, consistent with the original project. See
[LICENSE](LICENSE) for the full license and [NOTICE](NOTICE) for provenance.
This port used upstream commit
[`0dee94b5fc3721159bae556b8bd2d975f92b7b8b`](https://github.com/pklaus/brother_ql/tree/0dee94b5fc3721159bae556b8bd2d975f92b7b8b)
and Brother's [QL-800 raster command reference](https://download.brother.com/welcome/docp100278/cv_ql800_eng_raster_101.pdf).
Brother is a trademark of Brother Industries; this project is independent.
