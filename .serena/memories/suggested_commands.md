# Commands

- go test ./...; go test -race ./...; go vet ./...
- Native USB: go test -race -tags libusb ./...; go build -tags libusb -o bin/brother-ql ./cmd/brother-ql (requires libusb/pkg-config and CGO).
- Offline: go run ./cmd/brother-ql doctor; go run ./cmd/brother-ql info models; go run ./examples/convert > /tmp/label.bin
- CLI doctor never contacts hardware; discover/status/print/send can.
- Benchmark: go test . -run '^$' -bench BenchmarkEncodeQL800 -benchmem
- Use MCP's launcher for maintenance: uvx --from git+https://github.com/oraios/serena serena. Bare serena may resolve an incompatible install.
- Reindex: launcher + project index. Validate memory references: launcher + memories check (check CLI availability first).
