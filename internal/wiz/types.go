package wiz

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Ranges used when a light does not report its own.
const (
	DefaultMinTemp = 2200
	DefaultMaxTemp = 6500
	MinDimming     = 10
	MaxDimming     = 100
	MinSpeed       = 10
	MaxSpeed       = 200
	DefaultSpeed   = 100
)

// Bulb is one light's identity and capabilities.
type Bulb struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Module   string `json:"module"`
	Firmware string `json:"firmware"`
	MinTemp  int    `json:"minTemp"`
	MaxTemp  int    `json:"maxTemp"`
	MinDim   int    `json:"minDimming"`
}

// State is what getPilot reports. Colour fields are pointers because the
// light omits them unless it is in colour mode.
type State struct {
	MAC     string `json:"mac"`
	RSSI    int    `json:"rssi"`
	On      bool   `json:"state"`
	SceneID int    `json:"sceneId"`
	Speed   int    `json:"speed"`
	Temp    int    `json:"temp"`
	Dimming int    `json:"dimming"`
	R       *int   `json:"r"`
	G       *int   `json:"g"`
	B       *int   `json:"b"`
}

// Mode is how the light makes its colour.
type Mode int

const (
	ModeWhite Mode = iota
	ModeColor
	ModeScene
)

func (m Mode) String() string {
	return [...]string{"White", "Colour", "Scene"}[m]
}

// Mode infers the light's mode from a status reply.
func (s State) Mode() Mode {
	switch {
	case s.SceneID != 0:
		return ModeScene
	case s.R != nil && s.Temp == 0:
		return ModeColor
	default:
		return ModeWhite
	}
}

// RGB returns the colour channels, zero when the light is not in colour mode.
func (s State) RGB() (r, g, b int) {
	if s.R == nil || s.G == nil || s.B == nil {
		return 0, 0, 0
	}
	return *s.R, *s.G, *s.B
}

// Change is a setPilot request. Zero fields are left out of the message,
// except the pointer ones, where 0 is a real value.
type Change struct {
	On      *bool `json:"state,omitempty"`
	Dimming int   `json:"dimming,omitempty"`
	Temp    int   `json:"temp,omitempty"`
	R       *int  `json:"r,omitempty"`
	G       *int  `json:"g,omitempty"`
	B       *int  `json:"b,omitempty"`
	C       *int  `json:"c,omitempty"`
	W       *int  `json:"w,omitempty"`
	SceneID int   `json:"sceneId,omitempty"`
	Speed   int   `json:"speed,omitempty"`
}

// Power turns the light on or off without touching anything else.
func Power(on bool) Change { return Change{On: new(on)} }

// White sets tunable white at a colour temperature in kelvin.
func White(kelvin, dimming int) Change {
	return Change{On: new(true), Temp: kelvin, Dimming: dimming}
}

// Color sets an RGB colour with the white channels off.
func Color(r, g, b, dimming int) Change {
	return Change{On: new(true), R: new(r), G: new(g), B: new(b), C: new(0), W: new(0), Dimming: dimming}
}

// SceneChange starts a built-in scene. speed is sent only for dynamic scenes.
func SceneChange(scene Scene, speed, dimming int) Change {
	change := Change{On: new(true), SceneID: scene.ID, Dimming: dimming}
	if scene.Dynamic {
		change.Speed = speed
	}
	return change
}

// Dim changes brightness only.
func Dim(dimming int) Change { return Change{On: new(true), Dimming: dimming} }

// Scene is a built-in WiZ light mode. Dynamic scenes animate and take a speed.
type Scene struct {
	ID      int
	Name    string
	Dynamic bool
}

// Scenes lists the built-in scenes, ids as the firmware numbers them.
var Scenes = []Scene{
	{1, "Ocean", true}, {2, "Romance", true}, {3, "Sunset", true}, {4, "Party", true},
	{5, "Fireplace", true}, {6, "Cozy", true}, {7, "Forest", true}, {8, "Pastel colours", true},
	{9, "Wake up", true}, {10, "Bedtime", true}, {11, "Warm white", false}, {12, "Daylight", false},
	{13, "Cool white", false}, {14, "Night light", false}, {15, "Focus", false}, {16, "Relax", false},
	{17, "True colours", false}, {18, "TV time", false}, {19, "Plant growth", false}, {20, "Spring", true},
	{21, "Summer", true}, {22, "Fall", true}, {23, "Deep dive", true}, {24, "Jungle", true},
	{25, "Mojito", true}, {26, "Club", true}, {27, "Christmas", true}, {28, "Halloween", true},
	{29, "Candlelight", true}, {30, "Golden white", false}, {31, "Pulse", true}, {32, "Steampunk", true},
	{33, "Diwali", true}, {34, "White", false}, {35, "Alarm", true}, {36, "Snowy sky", true},
}

// SceneIndex returns the position of a scene id in Scenes, or -1.
func SceneIndex(id int) int {
	for i, scene := range Scenes {
		if scene.ID == id {
			return i
		}
	}
	return -1
}

// FindScene matches a scene by id or by name, ignoring case and spaces.
func FindScene(query string) (Scene, bool) {
	if id, err := strconv.Atoi(query); err == nil {
		if i := SceneIndex(id); i >= 0 {
			return Scenes[i], true
		}
		return Scene{}, false
	}
	normalize := func(s string) string { return strings.ToLower(strings.ReplaceAll(s, " ", "")) }
	for _, scene := range Scenes {
		if normalize(scene.Name) == normalize(query) {
			return scene, true
		}
	}
	return Scene{}, false
}

// Preset is a named colour for quick keys and the CLI.
type Preset struct {
	Name    string
	R, G, B int
}

// Presets are the colours on keys 1 to 9, in order.
var Presets = []Preset{
	{"red", 255, 0, 0}, {"orange", 255, 110, 0}, {"amber", 255, 160, 20},
	{"yellow", 255, 220, 0}, {"green", 0, 255, 0}, {"teal", 0, 200, 160},
	{"blue", 0, 0, 255}, {"purple", 150, 0, 255}, {"pink", 255, 60, 150},
}

// ParseColor reads a preset name or #rrggbb.
func ParseColor(text string) (r, g, b int, err error) {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, preset := range Presets {
		if preset.Name == text {
			return preset.R, preset.G, preset.B, nil
		}
	}
	hex := strings.TrimPrefix(text, "#")
	if len(hex) != 6 {
		return 0, 0, 0, fmt.Errorf("%q is not a colour name or #rrggbb", text)
	}
	value, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("%q is not a colour name or #rrggbb", text)
	}
	return int(value >> 16 & 0xff), int(value >> 8 & 0xff), int(value & 0xff), nil
}

// KelvinRGB approximates the colour of white light at a temperature, for
// on-screen previews (Tanner Helland's fit of the blackbody curve).
func KelvinRGB(kelvin int) (r, g, b int) {
	t := float64(kelvin) / 100
	clamp := func(v float64) int { return int(math.Max(0, math.Min(255, v))) }
	if t <= 66 {
		r = 255
		g = clamp(99.4708025861*math.Log(t) - 161.1195681661)
	} else {
		r = clamp(329.698727446 * math.Pow(t-60, -0.1332047592))
		g = clamp(288.1221695283 * math.Pow(t-60, -0.0755148492))
	}
	switch {
	case t >= 66:
		b = 255
	case t <= 19:
		b = 0
	default:
		b = clamp(138.5177312231*math.Log(t-10) - 305.0447927307)
	}
	return r, g, b
}
