// Run from this directory: go run gen.go
// Generates margin's demo images: a still (review.png) and an animation
// (loop.gif), in margin's palette.
package main

import (
	"image"
	"image/color"
	"image/color/palette"
	"image/draw"
	"image/gif"
	"image/png"
	"math"
	"os"
)

var (
	bg     = color.RGBA{0x1e, 0x1f, 0x29, 0xff}
	panel  = color.RGBA{0x2a, 0x2c, 0x38, 0xff}
	text   = color.RGBA{0xd0, 0xd0, 0xd0, 0xff}
	dim    = color.RGBA{0x58, 0x58, 0x58, 0xff}
	pink   = color.RGBA{0xff, 0x87, 0xd7, 0xff}
	violet = color.RGBA{0xd7, 0xaf, 0xff, 0xff}
	green  = color.RGBA{0x87, 0xd7, 0x87, 0xff}
	orange = color.RGBA{0xff, 0xaf, 0x87, 0xff}
	blue   = color.RGBA{0x87, 0xaf, 0xff, 0xff}
)

func rect(m draw.Image, x, y, w, h int, c color.Color) {
	draw.Draw(m, image.Rect(x, y, x+w, y+h), &image.Uniform{c}, image.Point{}, draw.Src)
}

func disc(m draw.Image, cx, cy, r float64, c color.Color) {
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			if math.Hypot(float64(x)-cx, float64(y)-cy) <= r {
				m.Set(x, y, c)
			}
		}
	}
}

// review.png: a page of "prose" with a gutter of review marks — what margin
// looks like, drawn as bars.
func still() {
	W, H := 640, 330
	m := image.NewRGBA(image.Rect(0, 0, W, H))
	rect(m, 0, 0, W, H, bg)
	rect(m, 24, 20, W-48, H-40, panel)
	lines := []struct {
		w    int
		mark color.Color
		c    color.Color
	}{
		{300, pink, pink}, {0, nil, nil},
		{520, green, dim}, {480, green, dim}, {350, green, dim}, {0, nil, nil},
		{510, orange, text}, {530, orange, text}, {200, orange, text}, {0, nil, nil},
		{440, nil, text}, {380, nil, text},
	}
	y := 44
	for _, l := range lines {
		if l.w > 0 {
			if l.mark != nil {
				rect(m, 40, y, 4, 12, l.mark)
			}
			rect(m, 60, y+2, l.w, 8, l.c)
		}
		y += 18
	}
	// A thread beside the flagged paragraph.
	rect(m, 60, y+4, 4, 30, violet)
	rect(m, 74, y+8, 260, 8, blue)
	rect(m, 74, y+24, 180, 8, text)
	f, _ := os.Create("review.png")
	png.Encode(f, m)
	f.Close()
}

// loop.gif: the review loop — a dot travels agent → review → agent, each
// stop lighting up as it arrives.
func anim() {
	W, H := 480, 120
	stops := []struct {
		x float64
		c color.Color
	}{{80, blue}, {240, pink}, {400, green}}
	pal := append(color.Palette{}, palette.Plan9...)
	var g gif.GIF
	const n = 36
	for i := 0; i < n; i++ {
		m := image.NewPaletted(image.Rect(0, 0, W, H), pal)
		draw.Draw(m, m.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
		rect(m, 80, 59, 320, 2, dim)
		t := float64(i) / n
		// Out and back: 0 → 1 → 0 along the track.
		p := 1 - math.Abs(1-2*t)
		x := 80 + 320*p
		for _, s := range stops {
			r, c := 14.0, color.Color(dim)
			if math.Abs(s.x-x) < 24 {
				r, c = 18, s.c
			}
			disc(m, s.x, 60, r, c)
			disc(m, s.x, 60, r-5, bg)
		}
		disc(m, x, 60, 7, text)
		g.Image = append(g.Image, m)
		g.Delay = append(g.Delay, 6)
	}
	f, _ := os.Create("loop.gif")
	gif.EncodeAll(f, &g)
	f.Close()
}

func main() { still(); anim() }
