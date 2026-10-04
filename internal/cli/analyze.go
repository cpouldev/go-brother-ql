// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"fmt"
	"image/png"
	"io"
	"os"

	brotherql "github.com/cpouldev/go-brother-ql"
	"github.com/cpouldev/go-brother-ql/raster"
	"github.com/spf13/cobra"
)

func (c *config) readRaster(cmd *cobra.Command, path string) (*os.File, []raster.Page, error) {
	var options raster.DecodeOptions
	if c.model != "" {
		model, err := brotherql.LookupModel(c.model)
		if err != nil {
			return nil, nil, err
		}
		options.RasterBytes = model.RasterBytes
	}
	source := cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil {
			return nil, nil, err
		}
		if !stat.Mode().IsRegular() {
			return nil, nil, fmt.Errorf("%s: raster input must be a regular file (use - for stdin)", path)
		}
		source = f
	}
	spool, err := os.CreateTemp("", "brother-ql-raster-*.bin")
	if err != nil {
		return nil, nil, err
	}
	success := false
	defer func() {
		if !success {
			removeTemp(spool)
		}
	}()
	const maxRasterBytes = 256 << 20
	n, err := io.Copy(contextWriter{cmd.Context(), spool}, io.LimitReader(source, maxRasterBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if n > maxRasterBytes {
		return nil, nil, fmt.Errorf("raster exceeds %d MiB", maxRasterBytes>>20)
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	pages, err := raster.DecodeWithOptions(spool, options)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", path, err)
	}
	if _, err = spool.Seek(0, io.SeekStart); err != nil {
		return nil, nil, err
	}
	success = true
	return spool, pages, nil
}

func (c *config) analyzeCommand() *cobra.Command {
	var prefix string
	cmd := &cobra.Command{Use: "analyze RASTER", Short: "Inspect raster commands and optionally reconstruct PNG pages", Args: cobra.ExactArgs(1)}
	cmd.Flags().StringVar(&prefix, "output-prefix", "", "Write PNG pages as PREFIX0001.png, PREFIX0002.png, ...")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		spool, pages, err := c.readRaster(cmd, args[0])
		if err != nil {
			return err
		}
		defer removeTemp(spool)
		if prefix != "" {
			for i, page := range pages {
				path := fmt.Sprintf("%s%04d.png", prefix, i+1)
				if err := writeOutput(path, cmd.OutOrStdout(), func(w io.Writer) error { return png.Encode(w, page.Image()) }); err != nil {
					return err
				}
			}
		}
		if c.json {
			return outputJSON(cmd.OutOrStdout(), struct {
				Pages []raster.Page `json:"pages"`
			}{Pages: pages})
		}
		for i, page := range pages {
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Page %d: %d x %d dots; media %d x %d mm; 600 dpi: %t\n", i+1, page.Width, page.Height, page.MediaWidth, page.MediaLength, page.HighResolution); err != nil {
				return err
			}
			for _, command := range page.Commands {
				if _, err := fmt.Fprintf(cmd.OutOrStdout(), "  %08x  %s (%d)\n", command.Offset, command.Name, command.Count); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return cmd
}
