package toys

import (
	"math"
	"testing"
	"time"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

func dimAt(t *testing.T, toy Toy, at time.Duration) int {
	t.Helper()
	frame, ok := toy.Frame(at)
	if !ok {
		t.Fatalf("toy ended at %v", at)
	}
	return frame.Change.Dimming
}

func TestBoxBreathFollowsTheBreath(t *testing.T) {
	box, err := NewBreathe(BoxBreathing, 0)
	if err != nil {
		t.Fatal(err)
	}
	s := time.Second
	cases := []struct {
		at   time.Duration
		want int
	}{
		{0, wiz.MinDimming},            // empty lungs
		{2 * s, 56},                    // half way in
		{4 * s, wiz.MaxDimming},        // full, holding
		{7900 * time.Millisecond, 100}, // still holding
		{10 * s, 56},                   // half way out
		{12 * s, wiz.MinDimming},       // empty, holding
		{16 * s, wiz.MinDimming},       // next breath starts empty
	}
	for _, c := range cases {
		if got := dimAt(t, box, c.at); math.Abs(float64(got-c.want)) > breathStep {
			t.Errorf("at %v: brightness %d, want %d", c.at, got, c.want)
		}
	}
}

func TestSleepBreathHasNoHoldAtTheBottom(t *testing.T) {
	breath, err := NewBreathe(SleepBreathing, 0)
	if err != nil {
		t.Fatal(err)
	}
	if breath.Cycle() != 19*time.Second {
		t.Fatalf("cycle %v, want 19s", breath.Cycle())
	}
	if got := dimAt(t, breath, 10*time.Second); got != wiz.MaxDimming {
		t.Errorf("held breath at %d, want full", got)
	}
	if got := dimAt(t, breath, 18900*time.Millisecond); got > wiz.MinDimming+breathStep {
		t.Errorf("end of the out-breath at %d, want near empty", got)
	}
}

func TestBreathingEndsAfterItsRounds(t *testing.T) {
	breath, err := NewBreathe(BoxBreathing, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := breath.Frame(2*breath.Cycle() - time.Millisecond); !ok {
		t.Error("ended during the last breath")
	}
	if _, ok := breath.Frame(2 * breath.Cycle()); ok {
		t.Error("kept going after two breaths")
	}
}

func TestPartyFlashesANewColourOnEachBeat(t *testing.T) {
	party := Party{BPM: 120} // a beat every 500 ms
	onBeat, _ := party.Frame(0)
	late, _ := party.Frame(400 * time.Millisecond)
	next, _ := party.Frame(500 * time.Millisecond)
	if onBeat.Change.Dimming != wiz.MaxDimming || late.Change.Dimming != partyLow {
		t.Errorf("beat brightness %d then %d, want %d then %d", onBeat.Change.Dimming, late.Change.Dimming, wiz.MaxDimming, partyLow)
	}
	if !Same(wiz.Color(*onBeat.Change.R, *onBeat.Change.G, *onBeat.Change.B, late.Change.Dimming), late.Change) {
		t.Error("colour changed inside one beat")
	}
	if *next.Change.R == *onBeat.Change.R && *next.Change.G == *onBeat.Change.G && *next.Change.B == *onBeat.Change.B {
		t.Error("the next beat kept the same colour")
	}
}

// tone is a block of a sine at amplitude, one window long.
func tone(amplitude float64) []float32 {
	block := make([]float32, window)
	for i := range block {
		block[i] = float32(amplitude * math.Sin(2*math.Pi*440*float64(i)/SampleRate))
	}
	return block
}

func TestListenerCountsBeatsAboveTheRoom(t *testing.T) {
	var l Listener
	l.Feed(make([]float32, window*20))
	if s := l.Snapshot(); s.Heard || s.Beats != 0 {
		t.Fatalf("silence gave %+v", s)
	}
	const bursts = 6
	for range bursts {
		for range 20 { // ~460 ms of quiet room
			l.Feed(tone(0.01))
		}
		for range 3 { // ~70 ms kick
			l.Feed(tone(0.5))
		}
	}
	s := l.Snapshot()
	if s.Beats != bursts {
		t.Errorf("%d beats, want %d", s.Beats, bursts)
	}
	if s.Level < 0.8 {
		t.Errorf("level %.2f right after a kick, want near full", s.Level)
	}
	for range 40 {
		l.Feed(tone(0.01))
	}
	if after := l.Snapshot(); after.Level > 0.2 || after.Beats != bursts {
		t.Errorf("after a quiet second: level %.2f, %d beats", after.Level, after.Beats)
	}
}
