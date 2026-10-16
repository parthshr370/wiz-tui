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

// SceneChange starts a built-in scene. speed and dimming are sent only to the
// scenes that use them; the light accepts the others and silently drops them.
func SceneChange(scene Scene, speed, dimming int) Change {
	change := Change{On: new(true), SceneID: scene.ID}
	if scene.Speed {
		change.Speed = speed
	}
	if scene.Dimming {
		change.Dimming = dimming
	}
	return change
}

// Dim changes brightness only.
func Dim(dimming int) Change { return Change{On: new(true), Dimming: dimming} }

// SceneKind is how the WiZ app groups a scene.
type SceneKind int

const (
	SceneWhite       SceneKind = iota // a still white
	SceneFunctional                   // a still light for a purpose: reading, TV, sleep
	SceneProgressive                  // changes over minutes at a pace set in the WiZ app
	SceneDynamic                      // a colour animation
)

// Scene is a built-in WiZ light mode.
type Scene struct {
	ID      int
	Name    string
	Kind    SceneKind
	Speed   bool   // takes a speed
	Dimming bool   // takes a brightness; false ones drive their own
	About   string // one line on what it does
}

// Animates reports whether the light moves on its own while the scene runs.
func (s Scene) Animates() bool { return s.Kind == SceneDynamic || s.Kind == SceneProgressive }

func white(id int, name, about string) Scene {
	return Scene{ID: id, Name: name, Kind: SceneWhite, Dimming: true, About: about}
}

func still(id int, name, about string) Scene {
	return Scene{ID: id, Name: name, Kind: SceneFunctional, Dimming: true, About: about}
}

func paced(id int, name, about string) Scene {
	return Scene{ID: id, Name: name, Kind: SceneDynamic, Speed: true, Dimming: true, About: about}
}

// Scenes lists the built-in scenes, ids as the firmware numbers them.
//
// Behaviour follows measurements on an ESP25_SHRGB_01 at firmware 1.38.0 (the
// wizlight crate's scene table), which corrects WiZ's published table: Cozy
// holds still, Candlelight and Alarm take no speed, Wake up, Night light and
// Alarm ignore brightness, and 38 to 40 exist although the app hides 38 and 39.
var Scenes = []Scene{
	paced(1, "Ocean", "blues and greens rolling"),
	paced(2, "Romance", "slow reds and pinks"),
	paced(3, "Sunset", "orange into red"),
	paced(4, "Party", "fast colour changes"),
	paced(5, "Fireplace", "flickering orange"),
	still(6, "Cozy", "warm amber, still"),
	paced(7, "Forest", "greens and yellows"),
	paced(8, "Pastel colours", "soft pastel drift"),
	{ID: 9, Name: "Wake up", Kind: SceneProgressive, About: "a slow sunrise"},
	{ID: 10, Name: "Bedtime", Kind: SceneProgressive, Dimming: true, About: "dims slowly into sleep"},
	white(11, "Warm white", "2700 K"),
	white(12, "Daylight", "4200 K"),
	white(13, "Cool white", "6500 K"),
	{ID: 14, Name: "Night light", Kind: SceneFunctional, About: "a very dim warm glow"},
	still(15, "Focus", "bright and crisp, for work"),
	still(16, "Relax", "soft and warm"),
	still(17, "True colours", "full-spectrum white"),
	still(18, "TV time", "low light for screens"),
	still(19, "Plant growth", "red and blue for plants"),
	paced(20, "Spring", "fresh greens"),
	paced(21, "Summer", "warm yellows"),
	paced(22, "Fall", "oranges and browns"),
	paced(23, "Deep dive", "deep blues"),
	paced(24, "Jungle", "lush greens"),
	paced(25, "Mojito", "lime and mint"),
	paced(26, "Club", "strobing colours"),
	paced(27, "Christmas", "red and green"),
	paced(28, "Halloween", "orange and purple"),
	{ID: 29, Name: "Candlelight", Kind: SceneDynamic, Dimming: true, About: "flickers like a candle"},
	paced(30, "Golden white", "a slow golden drift"),
	paced(31, "Pulse", "white breathing"),
	paced(32, "Steampunk", "copper and amber"),
	paced(33, "Diwali", "festive warm colours"),
	white(34, "White", "4000 K"),
	{ID: 35, Name: "Alarm", Kind: SceneDynamic, About: "flashing red"},
	paced(36, "Snowy sky", "icy blues"),
	white(38, "Soft white", "3500 K"),
	white(39, "Crisp white", "5000 K"),
	white(40, "Dim-to-warm", "gets warmer as it dims"),
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
	normalize := func(s string) string { return strings.ToLower(strings.NewReplacer(" ", "", "-", "").Replace(s)) }
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
