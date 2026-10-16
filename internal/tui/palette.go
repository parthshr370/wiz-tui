package tui

import (
	"math"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

type rgb struct{ R, G, B int }

type itemKind int

const (
	kindColor itemKind = iota
	kindWhite
	kindScene
)

// item is one preset the user can land on and apply with enter.
type item struct {
	name   string
	kind   itemKind
	color  rgb
	kelvin int
	scene  wiz.Scene
}

// swatch is the on-screen colour of the item at full brightness.
func (it item) swatch() rgb {
	switch it.kind {
	case kindWhite:
		r, g, b := wiz.KelvinRGB(it.kelvin)
		return rgb{r, g, b}
	default:
		return it.color
	}
}

func (it item) change(dimming, speed int) wiz.Change {
	switch it.kind {
	case kindWhite:
		return wiz.White(it.kelvin, dimming)
	case kindScene:
		return wiz.SceneChange(it.scene, speed, dimming)
	default:
		return wiz.Color(it.color.R, it.color.G, it.color.B, dimming)
	}
}

// matches reports whether the light's reported state is this item.
func (it item) matches(state wiz.State) bool {
	switch it.kind {
	case kindWhite:
		return state.Mode() == wiz.ModeWhite && state.Temp == it.kelvin
	case kindScene:
		return state.SceneID == it.scene.ID
	default:
		r, g, b := state.RGB()
		return state.Mode() == wiz.ModeColor && rgb{r, g, b} == it.color
	}
}

// Colours open with the dark-hearth omp theme palette, then a full hue sweep.
var colorItems = []item{
	{name: "Ember", color: rgb{254, 128, 25}},
	{name: "Herdr red", color: rgb{251, 70, 76}},
	{name: "Hearth gold", color: rgb{250, 189, 47}},
	{name: "Aqua", color: rgb{142, 192, 124}},
	{name: "Teal", color: rgb{111, 200, 194}},
	{name: "Rose", color: rgb{211, 134, 155}},
	{name: "Violet", color: rgb{168, 130, 255}},
	{name: "Sea blue", color: rgb{131, 165, 152}},
	{name: "Red", color: rgb{255, 0, 0}},
	{name: "Coral", color: rgb{255, 80, 50}},
	{name: "Orange", color: rgb{255, 110, 0}},
	{name: "Amber", color: rgb{255, 160, 20}},
	{name: "Yellow", color: rgb{255, 220, 0}},
	{name: "Lime", color: rgb{160, 255, 0}},
	{name: "Green", color: rgb{0, 255, 0}},
	{name: "Mint", color: rgb{60, 255, 150}},
	{name: "Cyan", color: rgb{0, 230, 255}},
	{name: "Sky", color: rgb{60, 150, 255}},
	{name: "Blue", color: rgb{0, 0, 255}},
	{name: "Indigo", color: rgb{75, 0, 255}},
	{name: "Purple", color: rgb{150, 0, 255}},
	{name: "Magenta", color: rgb{255, 0, 255}},
	{name: "Pink", color: rgb{255, 60, 150}},
	{name: "Blush", color: rgb{255, 140, 170}},
}

var whiteItems = []item{
	{name: "Candle", kind: kindWhite, kelvin: 2200},
	{name: "Warm", kind: kindWhite, kelvin: 2700},
	{name: "Soft", kind: kindWhite, kelvin: 3000},
	{name: "Evening", kind: kindWhite, kelvin: 3500},
	{name: "Neutral", kind: kindWhite, kelvin: 4000},
	{name: "Studio", kind: kindWhite, kelvin: 4500},
	{name: "Daylight", kind: kindWhite, kelvin: 5000},
	{name: "Overcast", kind: kindWhite, kelvin: 5700},
	{name: "Cool", kind: kindWhite, kelvin: 6500},
}

// sceneMood is a representative colour per built-in scene, for swatches only.
var sceneMood = map[int]rgb{
	1: {0, 120, 255}, 2: {255, 40, 90}, 3: {255, 110, 40}, 4: {200, 0, 255},
	5: {255, 90, 10}, 6: {255, 140, 60}, 7: {40, 200, 60}, 8: {255, 170, 220},
	9: {255, 190, 120}, 10: {255, 120, 40}, 11: {255, 167, 87}, 12: {255, 228, 206},
	13: {255, 254, 250}, 14: {255, 100, 20}, 15: {255, 238, 227}, 16: {255, 180, 110},
	17: {255, 209, 163}, 18: {90, 90, 255}, 19: {255, 0, 200}, 20: {120, 255, 120},
	21: {255, 220, 60}, 22: {230, 100, 20}, 23: {0, 60, 200}, 24: {0, 180, 80},
	25: {120, 255, 170}, 26: {255, 0, 160}, 27: {255, 30, 30}, 28: {255, 90, 0},
	29: {255, 130, 30}, 30: {255, 190, 110}, 31: {220, 220, 255}, 32: {200, 120, 40},
	33: {255, 150, 0}, 34: {255, 220, 190}, 35: {255, 0, 0}, 36: {180, 220, 255},
}

func sceneItems() []item {
	items := make([]item, 0, len(wiz.Scenes))
	for _, scene := range wiz.Scenes {
		items = append(items, item{name: scene.Name, kind: kindScene, scene: scene, color: sceneMood[scene.ID]})
	}
	return items
}

// hsvToRGB converts hue (0-360) and saturation (0-1) at full value.
func hsvToRGB(hue, sat float64) rgb {
	hue = math.Mod(math.Mod(hue, 360)+360, 360)
	chroma := sat
	x := chroma * (1 - math.Abs(math.Mod(hue/60, 2)-1))
	low := 1 - chroma
	var r, g, b float64
	switch {
	case hue < 60:
		r, g, b = chroma, x, 0
	case hue < 120:
		r, g, b = x, chroma, 0
	case hue < 180:
		r, g, b = 0, chroma, x
	case hue < 240:
		r, g, b = 0, x, chroma
	case hue < 300:
		r, g, b = x, 0, chroma
	default:
		r, g, b = chroma, 0, x
	}
	return rgb{int((r + low) * 255), int((g + low) * 255), int((b + low) * 255)}
}

// rgbToHS returns hue (0-360) and saturation (0-1), ignoring value.
func rgbToHS(c rgb) (float64, float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	high, low := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	delta := high - low
	if high == 0 || delta == 0 {
		return 0, 0
	}
	var hue float64
	switch high {
	case r:
		hue = 60 * math.Mod((g-b)/delta, 6)
	case g:
		hue = 60 * ((b-r)/delta + 2)
	default:
		hue = 60 * ((r-g)/delta + 4)
	}
	return math.Mod(hue+360, 360), delta / high
}

func mix(a, b rgb, f float64) rgb {
	f = math.Max(0, math.Min(1, f))
	return rgb{
		a.R + int(float64(b.R-a.R)*f),
		a.G + int(float64(b.G-a.G)*f),
		a.B + int(float64(b.B-a.B)*f),
	}
}

func (c rgb) scale(f float64) rgb {
	return rgb{int(float64(c.R) * f), int(float64(c.G) * f), int(float64(c.B) * f)}
}

func (c rgb) luma() float64 { return 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B) }
