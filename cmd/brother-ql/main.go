// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"sync"

	"github.com/cpouldev/go-brother-ql/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	pipe, writer := io.Pipe()
	input := &stdinReader{pipe: pipe, writer: writer}
	defer pipe.Close()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			stop() // Restore default signal handling for a second interrupt.
			_ = pipe.CloseWithError(ctx.Err())
			_ = os.Stdin.Close() // Interrupt an owned pipe read waiting for input.
		case <-done:
		}
	}()
	err := cli.Execute(ctx, os.Args[1:], input, os.Stdout, os.Stderr)
	close(done)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		stop()
		fmt.Fprintln(os.Stderr, "brother-ql:", err)
		os.Exit(1)
	}
}

// Reading starts only when requested. The pipe interrupts the command even when
// inherited stdin uses blocking system calls. Process exit releases that source.
type stdinReader struct {
	pipe   *io.PipeReader
	writer *io.PipeWriter
	once   sync.Once
}

func (r *stdinReader) Read(p []byte) (int, error) {
	r.once.Do(func() { go func() { _, err := io.Copy(r.writer, os.Stdin); _ = r.writer.CloseWithError(err) }() })
	return r.pipe.Read(p)
}
