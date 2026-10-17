package toys

import (
	"fmt"
	"math"
	"time"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

// Breathing colour: Ember, deep at the bottom of a breath and bright at the top.
var breathColor = [3]int{254, 128, 25}

const breathStep = 2 // brightness resolution of a breath

type phase struct {
	name   string
	length time.Duration
	rise   int // +1 breathing in, -1 breathing out, 0 holding
}

// Breathe guides a breathing pattern: the light grows on the in-breath, holds,
// and shrinks on the out-breath. Rounds of 0 breathe until stopped.
type Breathe struct {
	phases []phase
	Rounds int
}

// NewBreathe returns the pattern for id (BoxBreathing or SleepBreathing).
func NewBreathe(id string, rounds int) (Breathe, error) {
	in, out := phase{"breathe in", 4 * time.Second, 1}, phase{"breathe out", 0, -1}
	switch id {
	case BoxBreathing:
		out.length = 4 * time.Second
		return Breathe{phases: []phase{in, {"hold", 4 * time.Second, 0}, out, {"hold", 4 * time.Second, 0}}, Rounds: rounds}, nil
	case SleepBreathing:
		out.length = 8 * time.Second
		return Breathe{phases: []phase{in, {"hold", 7 * time.Second, 0}, out}, Rounds: rounds}, nil
	}
	return Breathe{}, fmt.Errorf("no breathing pattern %q; use box or 478", id)
}

// Cycle is one full breath.
func (b Breathe) Cycle() time.Duration {
	var total time.Duration
	for _, p := range b.phases {
		total += p.length
	}
	return total
}

func (b Breathe) Frame(elapsed time.Duration) (Frame, bool) {
	cycle := b.Cycle()
	round := int(elapsed / cycle)
	if b.Rounds > 0 && round >= b.Rounds {
		return Frame{}, false
	}
	into := elapsed % cycle
	level := 0.0 // where the last phase left the light: 0 empty, 1 full
	for i, p := range b.phases {
		if into >= p.length && i < len(b.phases)-1 {
			into -= p.length
			level = math.Max(0, math.Min(1, level+float64(p.rise)))
			continue
		}
		f := math.Min(1, float64(into)/float64(p.length))
		eased := (1 - math.Cos(math.Pi*f)) / 2
		switch p.rise {
		case 1:
			level = eased
		case -1:
			level = 1 - eased
		}
		dim := wiz.MinDimming + int(math.Round(level*float64(wiz.MaxDimming-wiz.MinDimming)))
		left := max(1, int(math.Ceil((p.length - into).Seconds())))
		status := fmt.Sprintf("%s · %d", p.name, left)
		if b.Rounds > 0 {
			status += fmt.Sprintf("   breath %d of %d", round+1, b.Rounds)
		}
		return Frame{Change: wiz.Color(breathColor[0], breathColor[1], breathColor[2], quantize(dim, breathStep)), Status: status}, true
	}
	return Frame{}, false // no phases
}

// Party bounds and default.
const (
	DefaultBPM = 120
	MinBPM     = 60
	MaxBPM     = 200

	partyLow  = 30 // brightness a beat decays to
	partyStep = 10
)

// Party flashes a new colour on every beat, then lets it fall back until the
// next one. It runs until stopped.
type Party struct{ BPM int }

func (p Party) Frame(elapsed time.Duration) (Frame, bool) {
	period := time.Minute / time.Duration(p.BPM)
	beat := int(elapsed / period)
	into := float64(elapsed%period) / float64(period)
	dim := wiz.MaxDimming - int(float64(wiz.MaxDimming-partyLow)*math.Min(1, into*2))
	r, g, b := HueRGB(float64(beat) * goldenAngle)
	return Frame{
		Change: wiz.Color(r, g, b, quantize(dim, partyStep)),
		Status: fmt.Sprintf("beat %d · %d bpm", beat+1, p.BPM),
	}, true
}
