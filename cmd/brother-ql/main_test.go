// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInterruptWhileWaitingForStdin(t *testing.T) {
	if os.Getenv("BROTHER_QL_SIGNAL_TEST") == "1" {
		os.Args = []string{"brother-ql", "convert", "-m", "QL-800", "-l", "62", "-"}
		main()
		return
	}
	if runtime.GOOS == "windows" {
		t.Skip("interrupt signals are tested on macOS and Linux")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestInterruptWhileWaitingForStdin$")
	tempDir := t.TempDir()
	cmd.Env = append(os.Environ(), "BROTHER_QL_SIGNAL_TEST=1", "TMPDIR="+tempDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	// Leave stdin open without data. The child must interrupt that blocked read.
	ready := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(tempDir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "brother-ql-input-") {
				ready = true
				break
			}
		}
		if ready {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !ready {
		t.Fatal("child did not reach stdin reading")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("interrupted conversion unexpectedly succeeded")
		}
		if !strings.Contains(stderr.String(), "context canceled") {
			t.Fatalf("stderr: %q", stderr.String())
		}
	case <-time.After(2 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatal("SIGINT did not interrupt blocked stdin")
	}
}
