package tui

import (
	"testing"
	"time"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

func TestSleepTimerFadesFromStartToLowestBrightness(t *testing.T) {
	start := time.Date(2026, 1, 1, 23, 0, 0, 0, time.UTC)
	timer := sleepTimer{}.next(start, 90)
	cases := []struct {
		after time.Duration
		want  int
	}{
		{0, 90},
		{timer.total / 2, 50},
		{timer.total, wiz.MinDimming},
		{timer.total + time.Minute, wiz.MinDimming},
	}
	for _, c := range cases {
		if got := timer.level(start.Add(c.after)); got != c.want {
			t.Errorf("level after %v = %d, want %d", c.after, got, c.want)
		}
	}
}

func TestSleepTimerCyclesThroughChoicesThenOff(t *testing.T) {
	now := time.Now()
	timer := sleepTimer{}
	for _, want := range sleepChoices {
		timer = timer.next(now, 80)
		if timer.total != want || !timer.at.Equal(now.Add(want)) {
			t.Fatalf("next = %v ending %v, want %v", timer.total, timer.at, want)
		}
	}
	if timer = timer.next(now, 80); !timer.at.IsZero() {
		t.Fatalf("after the longest choice the timer should be off, got %v", timer.total)
	}
}

// Every built-in scene is on exactly one tab, so none is unreachable.
func TestMoodsAndScenesSplitEveryScene(t *testing.T) {
	seen := map[int]int{}
	for _, it := range append(moodItems(), sceneItems()...) {
		seen[it.scene.ID]++
	}
	for _, scene := range wiz.Scenes {
		if seen[scene.ID] != 1 {
			t.Errorf("%s (%d) is on %d tabs", scene.Name, scene.ID, seen[scene.ID])
		}
	}
}
