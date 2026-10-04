# Implementation and validation

This Go port derives from Philipp Klaus's brother_ql at commit
`0dee94b5fc3721159bae556b8bd2d975f92b7b8b`, with Brother's raster command
reference used to resolve protocol behavior. Primary hardware target: QL-800
over USB on macOS and Linux. Hardware is not currently available.

See the [README](../README.md) for installation and usage, the
[reference fixtures](../testdata/reference/README.md) for independent byte
comparisons, and the [hardware checklist](HARDWARE.md) for physical validation.

## Development checks

Run these commands from the project root. Use the Go version declared in
[go.mod](../go.mod); see the [build instructions](../README.md#build) for native
USB prerequisites. These checks use fixtures, scripted transports, and localhost
TCP without contacting a physical printer.

```sh
test -z "$(gofmt -l .)"
go vet ./...
go test -race ./...
CGO_ENABLED=0 go build ./...

# With libusb, pkg-config, and a C toolchain installed:
CGO_ENABLED=1 go vet -tags libusb ./...
CGO_ENABLED=1 go test -race -tags libusb ./...
CGO_ENABLED=1 go build -tags libusb -o bin/brother-ql ./cmd/brother-ql
```

[CI](../.github/workflows/ci.yml) runs those checks on macOS and Linux. It also
runs each fuzz target for ten seconds on Linux:

```sh
go test ./internal/packbits -run '^$' -fuzz '^FuzzRoundTrip$' -fuzztime 10s
go test ./internal/packbits -run '^$' -fuzz '^FuzzDecode$' -fuzztime 10s
go test ./raster -run '^$' -fuzz '^FuzzDecode$' -fuzztime 10s
go test ./printer -run '^$' -fuzz '^FuzzParseStatus$' -fuzztime 10s
```

Measure the encoder and decoder locally with:

```sh
go test . -run '^$' -bench '^BenchmarkEncodeQL800$' -benchmem
go test ./raster -run '^$' -bench '^BenchmarkDecode$' -benchmem
```

## PDCA cycles

1. **Raster correctness.** Baseline Python reproductions: both pages use first-page
   flags and final-page commands; the analyzer combines pages; incompatible wide
   media is accepted. Hypothesis: explicit capabilities, validation, and independent
   encoding/decoding prevent these errors. Acceptance: known-good reference bytes,
   correct multi-page markers, independent pixel fixtures, malformed input rejection.
2. **Delivery and CLI.** Baseline: serial matching can select the wrong USB printer,
   short reads fail, CLI ignores reported errors, and low quality has no effect.
   Hypothesis: context-aware transports, framed status processing, and shared CLI
   options produce reliable outcomes. Acceptance: deterministic transport/CLI
   regressions, cancellation, short writes, cooling, and honest sent/printed results.
3. **Standardize.** Run race checks, fuzzing, platform builds, and benchmarks. Record
   measured results here; document installation, API, CLI, licensing, and a separate
   physical-printer checklist. Automated verification does not establish hardware support.

## Package layout

| Package | Responsibility |
| --- | --- |
| [brotherql](../doc.go) | Model and label catalogs, capability validation, image preparation, previews, and raster encoding; independent of transports and the CLI |
| [raster](../raster/decode.go) | Independent raster decoding with bounded allocations, command metadata, and reconstructed page images |
| [printer](../printer/client.go) | Transport selection, device discovery, framed status, printer sessions, and delivery outcomes |
| [internal/cli](../internal/cli) | Cobra commands, image/file handling, validation, spooling, and output formatting; invoked by [cmd/brother-ql](../cmd/brother-ql) |
| [internal/packbits](../internal/packbits) | Raster row compression and bounded decompression |

Read API signatures and comments directly from the source with:

```sh
go doc .
go doc ./raster
go doc ./printer
go doc . Options
go doc ./printer Client.Print
```

The [library guide](../README.md#library) explains streaming and client lifetime;
the [conversion example](../examples/convert/main.go) runs without hardware.
The CLI validates and spools image jobs before opening a printer. The decoder
also accepts intermediate page fragments for analysis; raw CLI sending requires
a finalized job. USB discovery contacts attached devices, while `doctor` only
reports configuration and build capabilities.

## Test strategy

```yaml
test_strategy:
  artifact: Go library and CLI for Brother raster printing
  rationale: Protocol mistakes waste labels and ambiguous delivery can duplicate prints.
  criticality: MEDIUM-HIGH
  selected_types:
    - rationale: Image conversion, model validation, codecs, and status have deterministic logic.
      type: unit
      size: small
      framework: go test
      dependencies: [in-memory image and byte fixtures]
      gate: Gate 1
    - rationale: CLI files and TCP/session boundaries need real I/O behavior.
      type: integration
      size: medium
      framework: go test
      dependencies: [temporary files, localhost TCP, scripted transport]
      gate: Gate 2
    - rationale: Binary parsers and encoders have large input domains and stable bounds.
      type: property-based
      size: small
      framework: Go fuzzing
      dependencies: [seed corpus]
      gate: Gate 6
  rejected_types:
    - reason: No graphical UI.
      type: component
    - reason: No browser application.
      type: e2e
    - reason: No independently deployed service API or broker contract.
      type: contract
    - reason: No deployed service.
      type: smoke
  deliberately_skipped:
    - why: No printer is available; simulated success cannot prove physical output.
      what: Physical USB and network printing
    - why: Declarative configuration and documentation have no business logic.
      what: Tests mirroring catalogs or checking prose text
```

Coverage includes:

- [unit] Exact monochrome raster matches a pinned upstream fixture.
- [unit] Multi-page output uses intermediate and final commands with correct page flags.
- [unit] QL-800 rejects wide media and compression; red media selects both planes.
- [unit] Dimensions, threshold, cut interval, and compression lengths cover boundaries.
- [unit] Decoder resets pages and rejects truncated data and oversized expansion.
- [integration] Fragmented status, partial writes, cooling, errors, and cancellation preserve outcomes.
- [integration] CLI quality changes bytes; errors produce a failing exit; pipes keep binary output clean.
- [property-based] PackBits round trips; arbitrary raster/status input cannot panic or allocate without bounds.

## Research and protocol decisions

The baseline was inspected and reproduced from the pinned upstream source, not
inferred from issue titles. Besides the cases above, status reads shorter than
32 bytes reached a `NameError`; a mocked failed send still yielded CLI exit 0;
and the CLI's `lq` keyword did not select the converter's `hq` flag. These now
have focused regression coverage. No Python runtime is required by the port.

The Go implementation follows these primary references:

- [QL-800/810W/820NWB raster manual](https://download.brother.com/welcome/docp100278/cv_ql800_eng_raster_101.pdf):
  status framing/notifications, page flags, raster plane layout, lengths, and
  high-resolution bit 6. Automatic status notifications are explicitly restored
  because that setting can persist until power-off.
- [Brother two-color resolution FAQ](https://support.brother.com/g/b/sp/faqend.aspx?c=th&faqid=faqp00100320_000&lang=en&prod=lpql800eas):
  two-color stock cannot use high resolution.
- [Legacy QL command reference, pp. 19–20](https://download.brother.com/welcome/docp000678/cv_qlseries_eng_raster_600.pdf):
  ordinary cutting, interval cutting, expanded mode, and high resolution have
  different model lists. QL-550 has ordinary cutting only. QL-650TD lacks interval
  cutting and early firmware lacks end cutting. Detailed command tables include
  QL-560 despite an omission from the overview.
- [QL-710W/720NW reference, p. 33](https://download.brother.com/welcome/docp000698/cv_ql600710720_eng_raster_102.pdf):
  high resolution is documented for both profiles.
- [QL-11xx reference](https://download.brother.com/welcome/docp100366/cv_ql1100_eng_raster_100.pdf)
  defines a high-resolution command bit but lists 300×300 dpi in specifications;
  [QL-1100 product specifications](https://support.brother.com/g/b/spec.aspx?c=us_ot&lang=en&prod=lpql1100eus)
  also list 300×300. High resolution is conservatively disabled pending evidence.
- [PT-P900W reference, p. 27](https://download.brother.com/welcome/docp100407/cv_ptp900_eng_raster_102.pdf):
  PT media/status fields differ from QL. PT profiles remain experimental conversion
  and preview only; CLI printing rejects them.
- [Linux usblp driver](https://github.com/torvalds/linux/blob/master/drivers/usb/class/usblp.c):
  successful zero-length reads rearm USB input rather than establish EOF;
  nonblocking output must drain before close. The kernel transport handles both.

Two catalog corrections were checked independently against the QL-800 manual:
23×23 media has 236×202 printable dots (pp. 15 and 20), and d12 die-cut media
uses a zero feed command (p. 28). Golden fixtures cover QL-800 mono, QL-800 red,
and QL-1100 wide raster output; their command normalization is documented in the
[fixture notes](../testdata/reference/README.md).

## Check and act

| Experiment | Result and standardized behavior |
| --- | --- |
| Upstream page/quality regressions | Correct first/subsequent page flags, FF/SUB termination, independent decoder pages, and effective low-quality flag |
| Invalid inputs before delivery | Image jobs spool first; raw jobs require finalization and selected-model capability checks; failed conversions preserve existing output |
| Framed printer session | Fragmented/coalesced status, short writes, explicit errors, cooling between writes, cancellation, and unknown outcomes covered by deterministic transports |
| Ambiguous connection reuse | Interrupted exchanges and bidirectional no-wait delivery return an unusable client; no automatic replay |
| CLI interrupt while stdin is blocked | Subprocess/pipe regression reproduces SIGINT hang; lazy input pipe now unblocks cancellation |
| Binary parser robustness | Raster: 1,320,572 fuzz executions; PackBits round trip: 1,038,311; arbitrary PackBits: 414,551; status: 496,007; no observed failures |
| Image allocation hotspot | Specialized standard image readers remove per-pixel boxed color allocation; exhaustive alpha/color comparison preserves generic conversion semantics |

Local benchmark samples on Apple M3 Max, Go 1.27.0, darwin/arm64, for encoding
a 696×1000 QL-800 monochrome image:

| Revision within this port | Time/op | Bytes/op | Allocations/op |
| --- | ---: | ---: | ---: |
| Initial generic pixel reads | 10,069,639 ns | 8,461,212 | 697,012 |
| Specialized standard image reads | 5,754,045 ns | 5,677,203 | 1,012 |

These are local single-run Go-to-Go measurements, not a comparison with Python
or a cross-platform performance guarantee. Use the benchmark commands under
[Development checks](#development-checks) to measure the current implementation.
Raster decoding of a 150×720 monochrome page measured 13,211 ns/op and 27,019 B/op.

CI standardizes race tests, vet, and pure/native builds on macOS and Linux,
with parser fuzzing on Linux. Physical printer behavior remains the next
validation cycle; use the checklist in [HARDWARE.md](HARDWARE.md). USB endpoint claims,
actual label positioning/cutting, color output, cooling, and disconnection need
real QL-800 runs on both operating systems. Network completion/SNMP and automatic
network discovery are deliberately deferred; TCP only establishes byte delivery.

### Completed verification

Verified locally on 2026-10-01 using Go 1.27.0:

- macOS/arm64: default and libusb builds pass all race tests and `go vet`.
- Linux/arm64 in a Debian Bookworm container: default and libusb race tests pass;
  native vet and pure Go build pass. Linux device tests use pipes and fake endpoints,
  so this verifies Linux runtime paths without claiming real USB hardware coverage.
- Linux/amd64 CLI cross-build succeeds with CGO disabled.
- GitHub Actions configuration passes actionlint; `gofmt` reports no changes.
- Offline executable smoke passes: compile/run the example, decode raster to PNG,
  preview, convert two copies, analyze JSON pages, catalog counts, USB build
  diagnostics, nonzero invalid-input exit, and zsh completion generation.
- Statement coverage (macOS default build): root library 86.2%, CLI 72.3%,
  PackBits 100%, printer 80.2%, raster decoder 81.9%. Coverage is supporting
  evidence; independent fixtures and error/cancellation regressions are the gates.

During this verification, the macOS executable was built in `bin/brother-ql` with
libusb enabled. Build outputs in `bin/` are excluded by [.gitignore](../.gitignore).
No upstream repository, printer, remote application, or release was modified or
published during verification.
