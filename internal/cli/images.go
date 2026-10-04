// SPDX-License-Identifier: GPL-3.0-or-later
package cli

import (
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"

	brotherql "github.com/cpouldev/go-brother-ql"
	"github.com/spf13/cobra"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
)

const (
	maxImageBytes     = 128 << 20
	maxImagePixels    = 25_000_000
	maxImageDimension = 100_000
	maxJobPages       = 100_000
)

type imageOptions struct {
	options   brotherql.Options
	threshold float64
	noCut     bool
	copies    int
}

func (c *config) imageFlags(cmd *cobra.Command, copies bool) *imageOptions {
	opts := &imageOptions{}
	f := cmd.Flags()
	f.StringVarP(&opts.options.Label, "label", "l", os.Getenv("BROTHER_QL_LABEL"), "Label identifier (see info labels)")
	f.StringVarP(&opts.options.Rotate, "rotate", "r", "auto", "Counterclockwise rotation: auto, 0, 90, 180, 270")
	f.StringVar(&opts.options.Fit, "fit", "auto", "Image fitting: auto, strict, contain")
	f.Float64VarP(&opts.threshold, "threshold", "t", 70, "Black/white threshold in percent (0–100)")
	f.BoolVarP(&opts.options.Dither, "dither", "d", false, "Use Floyd–Steinberg dithering")
	f.BoolVarP(&opts.options.Compress, "compress", "c", false, "Use PackBits compression when supported")
	f.BoolVar(&opts.options.Red, "red", false, "Print on black/red media")
	f.BoolVar(&opts.options.DPI600, "600dpi", false, "Use 300×600 dpi (head×feed) when supported")
	f.BoolVar(&opts.options.LowQuality, "lq", false, "Use faster low-quality mode")
	f.BoolVar(&opts.noCut, "no-cut", false, "Do not cut (alias for --cut never)")
	f.StringVar(&opts.options.Cut, "cut", "", "Cut policy: each, end, never, every (default each; never on models without a cutter)")
	f.IntVar(&opts.options.CutEvery, "cut-every", 0, "Cut interval for --cut every")
	opts.copies = 1
	if copies {
		f.IntVar(&opts.copies, "copies", 1, "Consecutive copies of each source image")
	}
	return opts
}

func (c *config) validatedOptions(cmd *cobra.Command, opts *imageOptions) (brotherql.Options, error) {
	if c.model == "" {
		return brotherql.Options{}, fmt.Errorf("--model is required (or set BROTHER_QL_MODEL)")
	}
	if opts.options.Label == "" {
		return brotherql.Options{}, fmt.Errorf("--label is required (or set BROTHER_QL_LABEL)")
	}
	if opts.noCut && cmd.Flags().Changed("cut") {
		return brotherql.Options{}, fmt.Errorf("--no-cut conflicts with --cut")
	}
	if cmd.Flags().Changed("cut-every") && opts.options.Cut != "every" {
		return brotherql.Options{}, fmt.Errorf("--cut-every requires --cut every")
	}
	if opts.copies < 1 || opts.copies > maxJobPages {
		return brotherql.Options{}, fmt.Errorf("--copies must be between 1 and %d", maxJobPages)
	}
	opts.options.Model = c.model
	opts.options.Threshold = &opts.threshold
	if opts.noCut {
		opts.options.Cut = "never"
	}
	return brotherql.ValidateOptions(opts.options)
}

func (c *config) convertCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{Use: "convert IMAGE...", Short: "Convert images to raster instructions (use - for stdin)", Args: cobra.MinimumNArgs(1)}
	opts := c.imageFlags(cmd, true)
	cmd.Flags().StringVarP(&output, "output", "o", "-", "Output raster file (- for stdout)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if c.json {
			return fmt.Errorf("--json is not valid for binary raster output")
		}
		options, err := c.validatedOptions(cmd, opts)
		if err != nil {
			return err
		}
		spool, err := spoolJob(cmd.Context(), cmd.InOrStdin(), args, opts.copies, options)
		if err != nil {
			return err
		}
		defer removeTemp(spool)
		return writeOutput(output, cmd.OutOrStdout(), func(w io.Writer) error { _, err := io.Copy(w, spool); return err })
	}
	return cmd
}

func (c *config) previewCommand() *cobra.Command {
	var output string
	cmd := &cobra.Command{Use: "preview IMAGE", Short: "Write the prepared printable image as PNG", Args: cobra.ExactArgs(1)}
	opts := c.imageFlags(cmd, false)
	cmd.Flags().StringVarP(&output, "output", "o", "-", "Output PNG file (- for stdout)")
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if c.json {
			return fmt.Errorf("--json is not valid for PNG output")
		}
		options, err := c.validatedOptions(cmd, opts)
		if err != nil {
			return err
		}
		im, err := readImage(cmd.Context(), cmd.InOrStdin(), args[0])
		if err != nil {
			return err
		}
		preview, err := brotherql.Preview(im, options)
		if err != nil {
			return err
		}
		return writeOutput(output, cmd.OutOrStdout(), func(w io.Writer) error { return png.Encode(w, preview) })
	}
	return cmd
}

func spoolJob(ctx context.Context, stdin io.Reader, paths []string, copies int, opts brotherql.Options) (*os.File, error) {
	if len(paths) > maxJobPages/copies {
		return nil, fmt.Errorf("job exceeds %d pages", maxJobPages)
	}
	stdinCount := 0
	for _, path := range paths {
		if path == "-" {
			stdinCount++
		}
	}
	if stdinCount > 1 {
		return nil, fmt.Errorf("stdin (-) can only appear once")
	}
	f, err := os.CreateTemp("", "brother-ql-job-*.bin")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			removeTemp(f)
		}
	}()
	encoder, err := brotherql.NewEncoder(contextWriter{ctx, f}, opts)
	if err != nil {
		return nil, err
	}
	for i, path := range paths {
		im, err := readImage(ctx, stdin, path)
		if err != nil {
			return nil, err
		}
		for copy := 0; copy < copies; copy++ {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if err := encoder.WritePage(im, i == len(paths)-1 && copy == copies-1); err != nil {
				return nil, fmt.Errorf("%s: %w", path, err)
			}
		}
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	success = true
	return f, nil
}

func readImage(ctx context.Context, stdin io.Reader, path string) (image.Image, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var f *os.File
	var err error
	if path == "-" {
		f, err = os.CreateTemp("", "brother-ql-input-*")
		if err != nil {
			return nil, err
		}
		defer removeTemp(f)
		n, err := io.Copy(contextWriter{ctx, f}, io.LimitReader(stdin, maxImageBytes+1))
		if err != nil {
			return nil, fmt.Errorf("stdin: %w", err)
		}
		if n > maxImageBytes {
			return nil, fmt.Errorf("stdin image exceeds %d MiB", maxImageBytes>>20)
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	} else {
		f, err = os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("%s: image input must be a regular file (use - for stdin)", path)
		}
		if info.Size() > maxImageBytes {
			return nil, fmt.Errorf("%s: image exceeds %d MiB", path, maxImageBytes>>20)
		}
	}
	config, _, err := image.DecodeConfig(io.LimitReader(f, maxImageBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: decode image header: %w", path, err)
	}
	if config.Width <= 0 || config.Height <= 0 || config.Width > maxImageDimension || config.Height > maxImageDimension || config.Width > maxImagePixels/config.Height {
		return nil, fmt.Errorf("%s: image dimensions %dx%d exceed the %d-pixel or %d-side limit", path, config.Width, config.Height, maxImagePixels, maxImageDimension)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	im, _, err := image.Decode(io.LimitReader(f, maxImageBytes))
	if err != nil {
		return nil, fmt.Errorf("%s: decode image: %w", path, err)
	}
	return im, nil
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.w.Write(p)
}

func removeTemp(f *os.File) { _ = f.Close(); _ = os.Remove(f.Name()) }

// Write files by rename so a failed conversion cannot destroy an existing output.
func writeOutput(path string, stdout io.Writer, write func(io.Writer) error) error {
	if path == "-" {
		return write(stdout)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".brother-ql-output-*")
	if err != nil {
		return err
	}
	defer removeTemp(f)
	if err := write(f); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
