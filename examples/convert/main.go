// SPDX-License-Identifier: GPL-3.0-or-later
// This example creates a simple monochrome label without contacting hardware.
package main

import (
	"image"
	"image/color"
	"image/draw"
	"log"
	"os"

	brotherql "github.com/cpouldev/go-brother-ql"
)

func main() {
	img := image.NewGray(image.Rect(0, 0, 696, 300))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(20, 20, 676, 40), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(20, 260, 676, 280), image.Black, image.Point{}, draw.Src)
	if err := brotherql.Encode(os.Stdout, []image.Image{img}, brotherql.Options{
		Model: "QL-800", Label: "62",
	}); err != nil {
		log.Fatal(err)
	}
}
