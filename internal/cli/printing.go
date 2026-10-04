// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	brotherql "github.com/cpouldev/go-brother-ql"
	"github.com/cpouldev/go-brother-ql/printer"
	"github.com/cpouldev/go-brother-ql/raster"
	"github.com/spf13/cobra"
)

func (c *config) endpoint() (string, error) {
	if c.timeout <= 0 {
		return "", fmt.Errorf("--timeout must be positive")
	}
	if c.printer == "" {
		return "", fmt.Errorf("--printer is required (or set BROTHER_QL_PRINTER)")
	}
	uri := c.printer
	switch c.backend {
	case "":
		return uri, nil
	case "network", "tcp":
		if !strings.Contains(uri, "://") {
			uri = "tcp://" + uri
		}
		if strings.HasPrefix(uri, "tcp://") || strings.HasPrefix(uri, "network://") {
			return uri, nil
		}
	case "usb", "pyusb":
		if !strings.Contains(uri, "://") {
			uri = "usb://" + uri
		}
		if strings.HasPrefix(uri, "usb://") {
			return uri, nil
		}
	case "linux_kernel":
		if strings.HasPrefix(uri, "/dev/") {
			return uri, nil
		}
	default:
		return "", fmt.Errorf("unknown backend %q (use usb, network, or linux_kernel)", c.backend)
	}
	return "", fmt.Errorf("printer %q conflicts with backend %q", c.printer, c.backend)
}

func networkURI(uri string) bool {
	return strings.HasPrefix(uri, "tcp://") || strings.HasPrefix(uri, "network://")
}

func (c *config) printCommand() *cobra.Command {
	var noWait bool
	cmd := &cobra.Command{Use: "print IMAGE...", Short: "Convert, validate, and print images", Args: cobra.MinimumNArgs(1)}
	opts := c.imageFlags(cmd, true)
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Return after delivery without waiting for completion (TCP always uses delivery mode)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		uri, err := c.endpoint()
		if err != nil {
			return err
		}
		options, err := c.validatedOptions(cmd, opts)
		if err != nil {
			return err
		}
		model, err := brotherql.LookupModel(options.Model)
		if err != nil {
			return err
		}
		if model.Experimental {
			return fmt.Errorf("%s is an experimental conversion-only profile; hardware delivery is not supported", model.ID)
		}
		spool, err := spoolJob(cmd.Context(), cmd.InOrStdin(), args, opts.copies, options)
		if err != nil {
			return err
		}
		defer removeTemp(spool)
		label, err := brotherql.LookupLabel(options.Label)
		if err != nil {
			return err
		}
		mediaType := byte(0x0b)
		if label.Kind == "continuous" {
			mediaType = 0x0a
		} else if label.Kind == "pt" {
			mediaType = 0
		}
		return c.deliver(cmd, uri, spool, printer.PrintOptions{Wait: !noWait && !networkURI(uri), Pages: len(args) * opts.copies, Media: &printer.Media{Width: label.WidthMM, Length: label.LengthMM, Type: mediaType}})
	}
	return cmd
}

func (c *config) sendCommand() *cobra.Command {
	var noWait bool
	cmd := &cobra.Command{Use: "send RASTER", Short: "Validate and send existing raster instructions (- for stdin)", Long: "Validate and send existing raster instructions (- for stdin). Supply --model to check printer capabilities and supported media before connecting. Without --model, you are responsible for choosing a compatible target printer.", Args: cobra.ExactArgs(1)}
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "Return after delivery without waiting for completion (TCP always uses delivery mode)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		uri, err := c.endpoint()
		if err != nil {
			return err
		}
		spool, pages, err := c.readRaster(cmd, args[0])
		if err != nil {
			return err
		}
		defer removeTemp(spool)
		if len(pages) == 0 {
			return fmt.Errorf("raster contains no printable pages")
		}
		commands := pages[len(pages)-1].Commands
		info, err := spool.Stat()
		if err != nil {
			return err
		}
		if len(commands) == 0 || commands[len(commands)-1].Name != "print final page" || commands[len(commands)-1].Offset+1 != info.Size() {
			return fmt.Errorf("raw job must end with a final-page command and contain no trailing instructions")
		}
		if c.model != "" {
			model, err := brotherql.LookupModel(c.model)
			if err != nil {
				return err
			}
			if model.Experimental {
				return fmt.Errorf("%s is an experimental conversion-only profile; hardware delivery is not supported", model.ID)
			}
			if err := validateRasterModel(model, pages); err != nil {
				return err
			}
		}
		first := pages[0]
		for _, page := range pages[1:] {
			if page.MediaWidth != first.MediaWidth || page.MediaLength != first.MediaLength || page.MediaType != first.MediaType {
				return fmt.Errorf("raw job changes media; send separate jobs for each media size")
			}
		}
		var media *printer.Media
		if first.MediaWidth > 0 {
			media = &printer.Media{Width: first.MediaWidth, Length: first.MediaLength, Type: first.MediaType}
		}
		return c.deliver(cmd, uri, spool, printer.PrintOptions{Wait: !noWait && !networkURI(uri), Pages: len(pages), Media: media})
	}
	return cmd
}

func validateRasterModel(model brotherql.Model, pages []raster.Page) error {
	red := false
	for i, page := range pages {
		if page.HighResolution && !model.HighResolution {
			return fmt.Errorf("page %d: %s does not support high resolution", i+1, model.ID)
		}
		for _, command := range page.Commands {
			unsupported := ""
			switch command.Name {
			case "initialize":
				red = false
			case "zero raster":
				if model.ID == "QL-800" {
					unsupported = "zero raster commands"
				}
			case "compression":
				if command.Data[0] == 2 && !model.Compression {
					unsupported = "compression"
				}
			case "various mode":
				if command.Data[0]&0x40 != 0 && !model.Cutting {
					unsupported = "automatic cutting"
				}
			case "cut every":
				if !model.CutEvery {
					unsupported = "cut intervals"
				}
			case "expanded mode":
				red = command.Data[0]&1 != 0
				if !model.ExpandedMode {
					unsupported = "expanded mode"
				} else if red && !model.TwoColor {
					unsupported = "two-color printing"
				} else if red && command.Data[0]&0x40 != 0 {
					unsupported = "two-color printing with high resolution"
				}
			case "margins":
				feed := int(binary.LittleEndian.Uint16(command.Data))
				scale := 1
				if page.HighResolution {
					scale = 2
				}
				if feed < model.MinFeed*scale || feed > model.MaxFeed*scale {
					return fmt.Errorf("page %d: margins %d exceed %s range %d..%d", i+1, feed, model.ID, model.MinFeed*scale, model.MaxFeed*scale)
				}
			}
			if unsupported != "" {
				return fmt.Errorf("page %d: %s does not support %s", i+1, model.ID, unsupported)
			}
		}
		scale := 1
		if page.HighResolution {
			scale = 2
		}
		if page.Height < model.MinLength*scale || page.Height > model.MaxLength*scale {
			return fmt.Errorf("page %d: %d rows exceed %s range %d..%d", i+1, page.Height, model.ID, model.MinLength*scale, model.MaxLength*scale)
		}
		if page.MediaWidth > 0 {
			matched := false
			for _, label := range brotherql.Labels() {
				kind := byte(0x0b)
				if label.Kind == "continuous" {
					kind = 0x0a
				} else if label.Kind == "pt" {
					kind = 0
				}
				if label.WidthMM != page.MediaWidth || label.LengthMM != page.MediaLength || kind != (page.MediaType&0x3f) {
					continue
				}
				if _, err := brotherql.ValidateOptions(brotherql.Options{Model: model.ID, Label: label.ID, Red: red, DPI600: page.HighResolution}); err == nil {
					matched = true
					break
				}
			}
			if !matched {
				return fmt.Errorf("page %d: %s does not support media %dx%d mm (type 0x%02x) with the selected color/resolution", i+1, model.ID, page.MediaWidth, page.MediaLength, page.MediaType)
			}
		}
	}
	return nil
}

func (c *config) deliver(cmd *cobra.Command, uri string, r io.Reader, opts printer.PrintOptions) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), c.timeout)
	defer cancel()
	client, err := printer.Open(ctx, uri)
	if err != nil {
		return err
	}
	result, printErr := client.Print(ctx, r, opts)
	closeErr := client.Close()
	err = errors.Join(printErr, closeErr)
	var outputErr error
	if c.json {
		report := struct {
			printer.Result
			Error string `json:"error,omitempty"`
		}{Result: result}
		if err != nil {
			report.Error = err.Error()
		}
		outputErr = outputJSON(cmd.OutOrStdout(), report)
	} else {
		if result.Outcome == "printed" {
			_, outputErr = fmt.Fprintf(cmd.OutOrStdout(), "printed: %d page(s), %d bytes sent\n", result.PagesPrinted, result.BytesSent)
		} else {
			_, outputErr = fmt.Fprintf(cmd.OutOrStdout(), "%s: %d bytes sent\n", result.Outcome, result.BytesSent)
		}
	}
	return errors.Join(err, outputErr)
}

func (c *config) statusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Query loaded media and printer status", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			uri, err := c.endpoint()
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), c.timeout)
			defer cancel()
			client, err := printer.Open(ctx, uri)
			if err != nil {
				return err
			}
			defer client.Close()
			status, err := client.Status(ctx)
			if err != nil {
				return err
			}
			if c.json {
				return outputJSON(cmd.OutOrStdout(), status)
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Media: %d x %d mm (type 0x%02x)\nPhase: %d\nStatus: 0x%02x\n", status.MediaWidth, status.MediaLength, status.MediaType, status.Phase, status.Type)
			return err
		},
	}
}

func (c *config) discoverCommand() *cobra.Command {
	return &cobra.Command{Use: "discover", Short: "Enumerate attached printers (USB by default)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if c.timeout <= 0 {
				return fmt.Errorf("--timeout must be positive")
			}
			backend := c.backend
			if backend == "" || backend == "pyusb" {
				backend = "usb"
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), c.timeout)
			defer cancel()
			devices, err := printer.Discover(ctx, backend)
			if err != nil {
				return err
			}
			if devices == nil {
				devices = []printer.Device{}
			}
			if c.json {
				return outputJSON(cmd.OutOrStdout(), devices)
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "PRINTER\tMODEL\tSERIAL")
			for _, device := range devices {
				fmt.Fprintf(w, "%s\t%s\t%s\n", device.URI, device.Model, device.Serial)
			}
			return w.Flush()
		},
	}
}
