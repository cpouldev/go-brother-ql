// SPDX-License-Identifier: GPL-3.0-or-later
// Package cli implements the brother-ql command-line interface.
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// Version is replaced at release time with -ldflags.
var Version = "dev"

// Execute runs a command with explicit streams; binary output is never mixed
// with diagnostics. The caller prints a returned error and selects the exit code.
func Execute(ctx context.Context, args []string, in io.Reader, out, stderr io.Writer) error {
	cmd := New(in, out, stderr)
	cmd.SetArgs(args)
	return cmd.ExecuteContext(ctx)
}

// New creates an independent command tree.
func New(in io.Reader, out, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use: "brother-ql", Short: "Convert images and print labels with Brother QL printers",
		SilenceErrors: true, SilenceUsage: true,
		Version: Version,
	}
	cmd.SetIn(in)
	cmd.SetOut(out)
	cmd.SetErr(stderr)
	cfg := &config{}
	f := cmd.PersistentFlags()
	f.StringVarP(&cfg.model, "model", "m", os.Getenv("BROTHER_QL_MODEL"), "Printer model (see info models)")
	f.StringVarP(&cfg.printer, "printer", "p", os.Getenv("BROTHER_QL_PRINTER"), "Printer URI: usb://..., tcp://host:9100, or /dev/usb/lp0")
	f.StringVarP(&cfg.backend, "backend", "b", os.Getenv("BROTHER_QL_BACKEND"), "Transport: usb, network, or linux_kernel (otherwise inferred from URI)")
	f.DurationVar(&cfg.timeout, "timeout", 60*time.Second, "Printer I/O timeout")
	f.BoolVar(&cfg.json, "json", false, "Machine-readable output for informational commands")
	cmd.AddCommand(cfg.convertCommand(), cfg.previewCommand(), cfg.infoCommand(), cfg.doctorCommand())
	cmd.AddCommand(cfg.printCommand(), cfg.sendCommand(), cfg.statusCommand(), cfg.discoverCommand(), cfg.analyzeCommand())
	cmd.AddCommand(&cobra.Command{Use: "version", Short: "Show version", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "brother-ql %s\n", Version)
			return err
		},
	})
	cmd.AddCommand(completionCommand())
	return cmd
}

type config struct {
	model, printer, backend string
	timeout                 time.Duration
	json                    bool
}

func completionCommand() *cobra.Command {
	return &cobra.Command{Use: "completion [bash|zsh|fish|powershell]", Short: "Generate shell completion", Args: cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs), ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return cmd.Root().GenPowerShellCompletionWithDesc(cmd.OutOrStdout())
			}
		},
	}
}
