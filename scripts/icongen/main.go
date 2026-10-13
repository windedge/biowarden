// Command icongen writes resources/icon.png: the square app icon of
// biowarden, a blue ring matching the tray icon. Standard library only, no
// drawing dependency.
//
//	go run ./scripts/icongen
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

// The same blue the tray icon uses, so both read as one app on light and
// dark taskbars.
var ink = color.NRGBA{R: 79, G: 124, B: 255, A: 255}

func main() {
	out := flag.String("o", "resources/icon.png", "output PNG")
	size := flag.Int("size", 1024, "edge length in pixels")
	flag.Parse()

	if err := writeRing(*out, *size); err != nil {
		fmt.Fprintln(os.Stderr, "icongen:", err)
		os.Exit(1)
	}
}

func writeRing(path string, size int) error {
	img := ring(size)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

// ring draws a transparent canvas with an antialiased blue ring centered in
// it.
func ring(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := float64(size) / 2
	outer := float64(size) * 0.40
	inner := outer - float64(size)*0.12
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-c, float64(y)+0.5-c)
			// Feathered 1px edges: coverage of the annulus at this pixel.
			a := clamp(outer+0.5-d, 0, 1) * clamp(d-inner+0.5, 0, 1)
			if a == 0 {
				continue
			}
			px := ink
			px.A = uint8(a*255 + 0.5)
			img.SetNRGBA(x, y, px)
		}
	}
	return img
}

func clamp(v, lo, hi float64) float64 {
	return math.Min(hi, math.Max(lo, v))
}
