// Package toys drives a light frame by frame: breathing guides, a party
// beat, music from the mic, and a dice roll.
//
// A toy is a pure function of time (music also reads the mic), so the CLI and
// the interactive screen play the same toy the same way. Only changed frames
// are sent, so a held breath or a quiet room sends nothing.
package toys

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"reflect"
	"time"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const (
	// FrameEvery is how often a toy is asked for a frame: at most ten
	// messages a second reach the light.
	FrameEvery = 100 * time.Millisecond
	// SendTimeout bounds one frame's round trip to the light.
	SendTimeout = time.Second
	// MaxFailures is how many frames in a row may go unanswered before a toy
	// stops. A single lost UDP frame is normal and the next one replaces it.
	MaxFailures = 5

	goldenAngle = 137.508 // degrees; consecutive hues never sit close together
)

// Frame is what the light should show now, and a short line for the screen.
type Frame struct {
	Change wiz.Change
	Status string
}

// Toy produces frames. Frame returns false once the toy has finished.
type Toy interface {
	Frame(elapsed time.Duration) (Frame, bool)
}

// Spec names a toy in the catalogue.
type Spec struct {
	ID    string
	Name  string
	About string
}

// Catalogue IDs.
const (
	BoxBreathing   = "box"
	SleepBreathing = "478"
	PartyID        = "party"
	MusicID        = "music"
	DiceID         = "dice"
)

// Catalogue is every toy, in the order the screen lists them. About fits the
// screen's card, 26 columns.
var Catalogue = []Spec{
	{BoxBreathing, "Box breathing", "in, hold, out, hold · 4 s"},
	{SleepBreathing, "4-7-8 sleep", "in 4, hold 7, out 8 s"},
	{PartyID, "Party", fmt.Sprintf("a colour per beat, %d bpm", DefaultBPM)},
	{MusicID, "Music", "pulses with the mic"},
	{DiceID, "Dice", "a random colour or scene"},
}

// New starts the toy id with its defaults. Music opens the mic, so a toy
// that is an io.Closer must be closed when done. Dice is a single roll, not
// a toy; see Dice.
func New(id string) (Toy, error) {
	switch id {
	case BoxBreathing, SleepBreathing:
		return NewBreathe(id, 0)
	case PartyID:
		return Party{BPM: DefaultBPM}, nil
	case MusicID:
		music, err := NewMusic()
		if err != nil {
			return nil, err
		}
		return music, nil
	}
	return nil, fmt.Errorf("no toy %q", id)
}

// Close releases what a toy holds open (the mic, for music).
func Close(toy Toy) error {
	if closer, ok := toy.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// Same reports whether two frames would send the same message.
func Same(a, b wiz.Change) bool { return reflect.DeepEqual(a, b) }

// Play sends toy's frames to the light at ip until the toy ends or ctx is
// cancelled, passing each frame's status to show. It does not restore the
// light afterwards; the caller knows what to restore.
func Play(ctx context.Context, client wiz.Client, ip string, toy Toy, show func(string)) error {
	ticker := time.NewTicker(FrameEvery)
	defer ticker.Stop()
	start := time.Now()
	var last *wiz.Change
	failures := 0
	for {
		frame, ok := toy.Frame(time.Since(start))
		if !ok {
			return nil
		}
		show(frame.Status)
		if last == nil || !Same(*last, frame.Change) {
			sendCtx, cancel := context.WithTimeout(ctx, SendTimeout)
			err := client.Set(sendCtx, ip, frame.Change)
			cancel()
			switch {
			case ctx.Err() != nil:
				return nil
			case err != nil:
				failures++
				if failures >= MaxFailures {
					return fmt.Errorf("the light stopped answering: %w", err)
				}
			default:
				failures = 0
				last = &frame.Change
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// Dice picks a random still or animated scene, or a random vivid colour, and
// names it. Slow scenes and plain whites are left out: a roll should show.
func Dice(dimming, speed int) (string, wiz.Change) {
	var scenes []wiz.Scene
	for _, scene := range wiz.Scenes {
		if scene.Kind == wiz.SceneFunctional || scene.Kind == wiz.SceneDynamic {
			scenes = append(scenes, scene)
		}
	}
	if rand.IntN(2) == 0 {
		scene := scenes[rand.IntN(len(scenes))]
		return scene.Name, wiz.SceneChange(scene, speed, dimming)
	}
	r, g, b := HueRGB(rand.Float64() * 360)
	return fmt.Sprintf("#%02x%02x%02x", r, g, b), wiz.Color(r, g, b, dimming)
}

// HueRGB is the fully saturated colour at hue degrees.
func HueRGB(hue float64) (r, g, b int) {
	hue = math.Mod(math.Mod(hue, 360)+360, 360)
	x := 1 - math.Abs(math.Mod(hue/60, 2)-1)
	var rf, gf, bf float64
	switch {
	case hue < 60:
		rf, gf = 1, x
	case hue < 120:
		rf, gf = x, 1
	case hue < 180:
		gf, bf = 1, x
	case hue < 240:
		gf, bf = x, 1
	case hue < 300:
		rf, bf = x, 1
	default:
		rf, bf = 1, x
	}
	return int(math.Round(rf * 255)), int(math.Round(gf * 255)), int(math.Round(bf * 255))
}

// quantize rounds a brightness to step so small wobbles send nothing.
func quantize(dimming, step int) int {
	q := int(math.Round(float64(dimming)/float64(step))) * step
	return max(wiz.MinDimming, min(wiz.MaxDimming, q))
}
