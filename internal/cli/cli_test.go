// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func run(t *testing.T, input []byte, args ...string) ([]byte, string, error) {
	t.Helper()
	var out, stderr bytes.Buffer
	err := Execute(context.Background(), args, bytes.NewReader(input), &out, &stderr)
	return out.Bytes(), stderr.String(), err
}

func TestHelp(t *testing.T) {
	out, stderr, err := run(t, nil, "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Brother QL") {
		t.Fatalf("help: %s", out)
	}
	if stderr != "" {
		t.Fatalf("stderr: %q", stderr)
	}
}

func TestVersion(t *testing.T) {
	out, _, err := run(t, nil, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "brother-ql") {
		t.Fatalf("version: %q", out)
	}
}

func TestUnknownCommandReturnsError(t *testing.T) {
	out, _, err := run(t, nil, "does-not-exist")
	if err == nil {
		t.Fatal("expected error")
	}
	if len(out) != 0 {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestCompletion(t *testing.T) {
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			out, _, err := run(t, nil, "completion", shell)
			if err != nil {
				t.Fatal(err)
			}
			if len(out) == 0 {
				t.Fatal("empty completion")
			}
		})
	}
}
