// SPDX-License-Identifier: GPL-3.0-or-later
// Command genicon renders the Scribe app icon as a 1024x1024 PNG with the
// same visual language as the sidebar header: a violet → fuchsia → pink
// diagonal gradient on a rounded squircle, with six white "audio lines"
// bars at the center mirroring Lucide's AudioLines glyph.
//
// Run with:
//
//	go run ./scripts/genicon > /dev/null && ls -lh build/appicon.png
//
// The output path is always build/appicon.png relative to the repo root,
// so callers should run from the workspace root.
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
)

const (
	canvasSize  = 1024
	cornerRadius = 225.0 // ≈ iOS squircle corner radius for 1024px

	// AudioLines is rendered in its native 24x24 viewBox space, then
	// scaled and translated into the canvas.
	glyphView = 24.0
	// Fraction of the canvas occupied by the glyph's full viewBox.
	glyphFill = 0.62
	// Stroke width in viewBox units (Lucide ships 2.25, but the icon
	// reads better a hair thicker against the dark gradient).
	strokeWidth = 2.4
)

type rgbaF struct{ r, g, b, a float64 }

func fromHex(r, g, b uint8) rgbaF {
	return rgbaF{float64(r), float64(g), float64(b), 255}
}

func (c rgbaF) toRGBA() color.RGBA {
	return color.RGBA{
		R: uint8(clamp(c.r, 0, 255)),
		G: uint8(clamp(c.g, 0, 255)),
		B: uint8(clamp(c.b, 0, 255)),
		A: uint8(clamp(c.a, 0, 255)),
	}
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func lerp(a, b float64, t float64) float64 { return a*(1-t) + b*t }

func lerpColor(a, b rgbaF, t float64) rgbaF {
	return rgbaF{
		r: lerp(a.r, b.r, t),
		g: lerp(a.g, b.g, t),
		b: lerp(a.b, b.b, t),
		a: lerp(a.a, b.a, t),
	}
}

// gradientAt returns the gradient color at (x, y). Mirrors Tailwind's
// `bg-gradient-to-br from-violet-500 via-fuchsia-500 to-pink-500`.
var (
	stopViolet  = fromHex(0x8B, 0x5C, 0xF6)
	stopFuchsia = fromHex(0xD9, 0x46, 0xEF)
	stopPink    = fromHex(0xEC, 0x48, 0x99)
)

func gradientAt(x, y float64) color.RGBA {
	t := (x + y) / (2 * (canvasSize - 1))
	t = clamp(t, 0, 1)
	var c rgbaF
	if t < 0.5 {
		c = lerpColor(stopViolet, stopFuchsia, t/0.5)
	} else {
		c = lerpColor(stopFuchsia, stopPink, (t-0.5)/0.5)
	}
	return c.toRGBA()
}

// roundedRectCoverage returns the fractional coverage (0..1) of pixel (x, y)
// inside a rounded rect anchored at (0, 0) with size (canvasSize) and
// corner radius cornerRadius. Cheap 4×4 supersampling for the corner band;
// interior pixels return 1 immediately.
func roundedRectCoverage(x, y int) float64 {
	fx := float64(x)
	fy := float64(y)
	// Quick interior test: anywhere beyond the corner band is fully inside.
	if fx >= cornerRadius && fx <= canvasSize-1-cornerRadius {
		return 1
	}
	if fy >= cornerRadius && fy <= canvasSize-1-cornerRadius {
		return 1
	}
	const ss = 4
	hits := 0
	for sy := 0; sy < ss; sy++ {
		for sx := 0; sx < ss; sx++ {
			px := fx + (float64(sx)+0.5)/float64(ss)
			py := fy + (float64(sy)+0.5)/float64(ss)
			cx := clamp(px, cornerRadius, canvasSize-1-cornerRadius)
			cy := clamp(py, cornerRadius, canvasSize-1-cornerRadius)
			dx := px - cx
			dy := py - cy
			if dx*dx+dy*dy <= cornerRadius*cornerRadius {
				hits++
			}
		}
	}
	return float64(hits) / float64(ss*ss)
}

// Lucide AudioLines bars expressed in viewBox space as (cx, y_top, y_bottom).
var audioLineBars = [6][3]float64{
	{2, 10, 13},
	{6, 6, 17},
	{10, 3, 21},
	{14, 8, 15},
	{18, 5, 18},
	{22, 10, 13},
}

// barInside returns whether (gx, gy) (in glyph viewBox units) lies inside
// any of the six rounded-cap bars. Hard inside/outside test — the caller
// supersamples in pixel space for anti-aliasing.
func barInside(gx, gy float64) bool {
	half := strokeWidth / 2
	for _, b := range audioLineBars {
		cx, top, bot := b[0], b[1], b[2]
		dx := gx - cx
		var dy float64
		if gy < top {
			dy = gy - top
		} else if gy > bot {
			dy = gy - bot
		} else {
			dy = 0
		}
		if dx*dx+dy*dy <= half*half {
			return true
		}
	}
	return false
}

func main() {
	// NRGBA = straight (non-premultiplied) alpha, which is what PNG
	// readers expect. Using image.RGBA here would silently darken
	// partially-transparent edges.
	img := image.NewNRGBA(image.Rect(0, 0, canvasSize, canvasSize))

	glyphScale := canvasSize * glyphFill / glyphView
	glyphOffset := (float64(canvasSize) - glyphScale*glyphView) / 2

	white := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}

	for y := 0; y < canvasSize; y++ {
		for x := 0; x < canvasSize; x++ {
			rrCov := roundedRectCoverage(x, y)
			if rrCov <= 0 {
				continue
			}
			bgRGBA := gradientAt(float64(x), float64(y))
			bg := color.NRGBA{R: bgRGBA.R, G: bgRGBA.G, B: bgRGBA.B, A: 255}

			// Pixel-space supersampling: 4×4 subpixels per output pixel.
			// We project each subpixel into glyph space and do a hard
			// inside/outside test against the union of bar SDFs.
			const ss = 4
			hits := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/float64(ss)
					py := float64(y) + (float64(sy)+0.5)/float64(ss)
					gx := (px - glyphOffset) / glyphScale
					gy := (py - glyphOffset) / glyphScale
					if barInside(gx, gy) {
						hits++
					}
				}
			}
			barCov := float64(hits) / float64(ss*ss)

			out := bg
			if barCov > 0 {
				out = color.NRGBA{
					R: uint8(lerp(float64(bg.R), float64(white.R), barCov)),
					G: uint8(lerp(float64(bg.G), float64(white.G), barCov)),
					B: uint8(lerp(float64(bg.B), float64(white.B), barCov)),
					A: 255,
				}
			}
			out.A = uint8(255 * rrCov)
			img.SetNRGBA(x, y, out)
		}
	}

	outPath := "build/appicon.png"
	f, err := os.Create(outPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", outPath)
}
