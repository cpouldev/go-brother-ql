// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"text/tabwriter"

	brotherql "github.com/cpouldev/go-brother-ql"
	"github.com/cpouldev/go-brother-ql/printer"
	"github.com/spf13/cobra"
)

func outputJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func (c *config) infoCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "info", Short: "Show models, media, and environment variables"}
	cmd.AddCommand(&cobra.Command{Use: "models", Short: "List model capabilities", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			models := brotherql.Models()
			if c.json {
				return outputJSON(cmd.OutOrStdout(), models)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "MODEL\tRASTER DOTS\tCUTTER\tCUT INTERVAL\tCOMPRESSION\tRED\t300×600 DPI\tEXPERIMENTAL")
			for _, m := range models {
				fmt.Fprintf(w, "%s\t%d\t%t\t%t\t%t\t%t\t%t\t%t\n", m.ID, m.RasterBytes*8, m.Cutting, m.CutEvery, m.Compression, m.TwoColor, m.HighResolution, m.Experimental)
			}
			return w.Flush()
		},
	})
	cmd.AddCommand(&cobra.Command{Use: "labels", Short: "List label dimensions in millimeters and printable dots", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			labels := brotherql.Labels()
			if c.json {
				return outputJSON(cmd.OutOrStdout(), labels)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "LABEL\tKIND\tMEDIA MM\tPRINTABLE DOTS\tRED")
			for _, l := range labels {
				length, height := fmt.Sprint(l.LengthMM), fmt.Sprint(l.Height)
				if l.LengthMM == 0 {
					length, height = "continuous", "variable"
				}
				fmt.Fprintf(w, "%s\t%s\t%d x %s\t%d x %s\t%t\n", l.ID, l.Kind, l.WidthMM, length, l.Width, height, l.Red)
			}
			return w.Flush()
		},
	})
	cmd.AddCommand(&cobra.Command{Use: "env", Short: "Show recognized environment defaults (flags take precedence)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			keys := []string{"BROTHER_QL_MODEL", "BROTHER_QL_PRINTER", "BROTHER_QL_BACKEND", "BROTHER_QL_LABEL"}
			values := make(map[string]string, len(keys))
			for _, key := range keys {
				values[key] = os.Getenv(key)
			}
			if c.json {
				return outputJSON(cmd.OutOrStdout(), values)
			}
			for _, key := range keys {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s=%s\n", key, values[key]); err != nil {
					return err
				}
			}
			return nil
		},
	})
	return cmd
}

func (c *config) doctorCommand() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Report build features and configuration without contacting hardware", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := struct {
				Version    string   `json:"version"`
				Platform   string   `json:"platform"`
				Go         string   `json:"go"`
				USBEnabled bool     `json:"usb_enabled"`
				Model      string   `json:"model,omitempty"`
				Printer    string   `json:"printer,omitempty"`
				Backend    string   `json:"backend,omitempty"`
				Notes      []string `json:"notes"`
			}{Version: Version, Platform: runtime.GOOS + "/" + runtime.GOARCH, Go: runtime.Version(), USBEnabled: printer.USBEnabled(), Model: c.model, Printer: c.printer, Backend: c.backend,
				Notes: []string{"Hardware was not contacted; use discover to enumerate attached printers.", "TCP reports successful delivery, not confirmed printing."},
			}
			if !report.USBEnabled {
				report.Notes = append(report.Notes, "USB requires libusb, CGO, and building with -tags libusb.")
			}
			if runtime.GOOS == "linux" {
				report.Notes = append(report.Notes, "For USB permission errors, configure a Brother udev rule; the usblp driver may hold the interface.")
			}
			if c.model != "" {
				if _, err := brotherql.LookupModel(c.model); err != nil {
					report.Notes = append(report.Notes, err.Error())
				}
			}
			if c.json {
				return outputJSON(cmd.OutOrStdout(), report)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "brother-ql %s (%s; %s)\nUSB enabled: %t\nModel: %s\nPrinter: %s\nBackend: %s\n", report.Version, report.Platform, report.Go, report.USBEnabled, report.Model, report.Printer, report.Backend); err != nil {
				return err
			}
			for _, note := range report.Notes {
				if _, err := fmt.Fprintln(cmd.OutOrStdout(), note); err != nil {
					return err
				}
			}
			return nil
		},
	}
}
