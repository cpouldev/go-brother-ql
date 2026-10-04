# Project map

- Go port of Philipp Klaus's brother_ql; GPL-3.0-or-later. Catalog and fixtures retain pinned upstream provenance in NOTICE and testdata/reference/README.md.
- Root package prepares image.Image and encodes raster; printer owns transport/status/session; raster independently decodes; internal/cli and cmd/brother-ql expose command interface.
- Main hardware target: QL-800 USB on macOS/Linux. No physical validation completed; docs/HARDWARE.md is the pending checklist. TCP reports delivery only; PT profiles are experimental conversion only.
- AGENTS.md governs collaboration. go.mod determines module identity/toolchain.
- For native USB build constraints consult `mem:tech_stack`; validation and delivery semantics consult `mem:task_completion`; Serena maintenance consult `mem:suggested_commands`; editing rules consult `mem:conventions`.
