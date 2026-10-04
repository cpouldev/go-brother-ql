# Hardware validation

No physical printer was available during implementation. Automated tests use
scripted status responses, in-memory fixtures, and localhost TCP. Complete this
checklist with a QL-800 on macOS and Linux before claiming hardware verification.

See the [build instructions](../README.md#build) for prerequisites and the
[development notes](DEVELOPMENT.md) for automated validation and protocol research.
Commands below refer to `bin/brother-ql`, built from the project root.

Record OS/version, CPU architecture, CLI version, libusb version, printer firmware,
connection, roll ID, options, observed output, and any errors. Keep serial numbers
private when publishing logs. Run each test with a small job first.

## QL-800 USB

1. Install libusb, build with `-tags libusb`, and turn Editor Lite off.
2. Run `doctor`, `discover -b usb`, then `status` with the exact returned URI.
   Confirm the media width/type and cover status match the physical printer.
3. Preview and print a 696×300 black/white image on 62 mm continuous media.
   Check orientation, both margins, fine text, alignment, and final cutting.
4. Print two different pages. Check neither page is duplicated or appended to the
   other; completion should report both pages.
5. Check `--copies 2`, `--cut each`, `--cut end`, `--cut never`, and
   `--cut every --cut-every 2` on a three-page job.
6. Repeat with an available die-cut label using its exact dimensions. Check
   automatic rotation and `--fit contain` against the preview.
7. Compare threshold and dither output on a grayscale ramp. Check fully transparent
   pixels print white. Compare default quality with `--lq`.
8. On supported black/red stock, check pure black, red, and white regions,
   grayscale and dither output. Check no overlapping black/red dots.
9. On monochrome stock, print a 1392×600 source with `--600dpi`. The physical size
   should match the 696×300 source at 300 dpi.
10. Open the cover or load a roll whose reported width, length, or type differs
    from the requested media. Confirm a failing exit and no job transmission.
    These fields cannot distinguish every roll property, including black/red
    stock. Close/reload and verify a fresh job works.
11. Disconnect USB during a small job. Confirm an error with an ambiguous outcome
    after any delivery; no automatic replay. Check the actual labels before retrying.
12. Cancel a job with Ctrl-C and test a deliberately short timeout. The process
    should exit, release USB, and permit the next invocation to open it.
13. If a normal workload causes cooling, capture start/end status behavior and
    verify delivery resumes without corruption. Do not force overheating.
14. When possible, attach two devices and confirm serial-specific selection;
    an ambiguous selector should fail.

On Linux, separately test `/dev/usb/lp0` with an appropriate kernel driver and
permissions. Confirm cancellation, status, and completion work there as well.

## Network-capable models

The QL-800 cannot directly validate this transport. With a compatible network
printer or print server, test `tcp://HOST:9100`, a multi-page job, a refused
connection, timeout, and interrupted delivery. Verify the CLI reports `sent`
after delivery and never claims confirmed `printed`. Network media checks,
discovery, SNMP, and printer completion are not implemented.

## Recording results

| Date | OS / build | Printer / firmware | Connection / roll | Check | Result |
| --- | --- | --- | --- | --- | --- |
| Pending | macOS | QL-800 | USB | All physical checks | Not run |
| Pending | Linux | QL-800 | USB | All physical checks | Not run |

If output differs from the preview, retain the source image and converted raster
file. Inspect the saved job and reconstruct its pages before changing protocol
constants:

```sh
bin/brother-ql analyze label.bin --json
bin/brother-ql analyze label.bin --output-prefix decoded-
```

The second command writes `decoded-0001.png`, and subsequent files for later
pages. Attach the smallest reproducer to an issue.
