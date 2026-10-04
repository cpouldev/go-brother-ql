# Completion checks

- go test -race ./...; go vet ./...; repeat with -tags libusb when native dependencies exist. CI targets macOS/Linux; offline tests must never require printers.
- Protocol regressions use upstream byte fixtures plus independent decoder fixtures; preserve provenance and explicitly document normalized command differences. Do not regenerate goldens from the encoder under test.
- Consult docs/DEVELOPMENT.md for PDCA evidence, fuzz targets, capability sources; docs/HARDWARE.md for pending physical validation. Never equate simulation/build success with hardware support.
- Print outcomes distinguish sent/printed/failed/unknown; never automatically replay a partial job. Interrupted exchanges and bidirectional no-wait jobs make a client unusable until reopened.
- Model capabilities must be backed by command documentation; ExpandedMode does not imply high resolution. Older/11xx modes have restrictions documented in development notes.
- Read-only status and discovery still access hardware: invoke only when task-authorized; normal validation uses scripted transports/localhost.
