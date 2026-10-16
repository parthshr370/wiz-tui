package tui

import (
	"fmt"
	"math"
	"strings"
)

// Orb geometry in half-block pixels: each terminal cell is two pixels tall,
// which makes pixels roughly square.
const (
	orbCols       = 26
	orbRows       = 11
	orbCoreRadius = 6.5
	orbGlowRadius = 12.5
)

var (
	orbBackground = rgb{0x18, 0x16, 0x15} // dark-hearth "hearth"
	orbGlassOff   = rgb{0x2a, 0x25, 0x22}
	orbRimOff     = rgb{0x3c, 0x38, 0x36}
	orbMetalLight = rgb{0x7c, 0x6f, 0x64}
	orbMetalDark  = rgb{0x50, 0x49, 0x45}
)

func ansiCell(top, bottom rgb, glyph string) string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm%s", top.R, top.G, top.B, bottom.R, bottom.G, bottom.B, glyph)
}

// renderOrb draws a lit bulb in colour c. glow scales the halo (1 = normal);
// lit=false draws the dark glass.
func renderOrb(c rgb, lit bool, glow float64) string {
	centerX := float64(orbCols-1) / 2
	centerY := float64(orbRows*2) - orbCoreRadius - 1 // glass rests on the screw base
	pixel := func(x, y int) rgb {
		dx, dy := float64(x)-centerX, float64(y)-centerY
		dist := math.Hypot(dx, dy)
		if !lit {
			switch {
			case dist <= orbCoreRadius-1:
				return mix(orbGlassOff, orbRimOff, 0.15-dy/orbCoreRadius*0.15)
			case dist <= orbCoreRadius:
				return orbRimOff
			}
			return orbBackground
		}
		if dist <= orbCoreRadius {
			hot := 1 - dist/orbCoreRadius
			// A highlight up and to the left sells the glass.
			shine := math.Max(0, 1-math.Hypot(dx+2.2, dy+2.2)/2.6)
			return mix(mix(c, rgb{255, 255, 255}, 0.28*hot), rgb{255, 255, 255}, 0.55*shine)
		}
		reach := orbCoreRadius + (orbGlowRadius-orbCoreRadius)*glow
		if dist <= reach {
			fade := 1 - (dist-orbCoreRadius)/(reach-orbCoreRadius)
			return mix(orbBackground, c, 0.55*fade*fade)
		}
		return orbBackground
	}
	var out strings.Builder
	for row := range orbRows {
		for col := range orbCols {
			out.WriteString(ansiCell(pixel(col, row*2), pixel(col, row*2+1), "▀"))
		}
		out.WriteString("\x1b[0m\n")
	}
	// The screw base: alternating metal bands, narrowing to the contact.
	bands := []struct {
		width int
		color rgb
	}{{8, orbMetalLight}, {8, orbMetalDark}, {6, orbMetalLight}, {4, orbMetalDark}}
	for i := 0; i < len(bands); i += 2 {
		top, bottom := bands[i], bands[i+1]
		for col := range orbCols {
			upper, lower := orbBackground, orbBackground
			if math.Abs(float64(col)-centerX) < float64(top.width)/2 {
				upper = top.color
			}
			if math.Abs(float64(col)-centerX) < float64(bottom.width)/2 {
				lower = bottom.color
			}
			out.WriteString(ansiCell(upper, lower, "▀"))
		}
		out.WriteString("\x1b[0m")
		if i+2 < len(bands) {
			out.WriteString("\n")
		}
	}
	return out.String()
}
