package wiz

import (
	"reflect"
	"testing"
)

// A scene is sent only the parameters it uses; the light would drop the rest.
func TestSceneChangeSendsOnlyWhatTheSceneTakes(t *testing.T) {
	cases := []struct {
		name               string
		wantSpeed, wantDim bool
	}{
		{"Party", true, true},
		{"Candlelight", false, true},
		{"Bedtime", false, true},
		{"Night light", false, false},
		{"Wake up", false, false},
		{"Warm white", false, true},
	}
	for _, c := range cases {
		scene, ok := FindScene(c.name)
		if !ok {
			t.Fatalf("no scene %q", c.name)
		}
		change := SceneChange(scene, 150, 40)
		if (change.Speed != 0) != c.wantSpeed || (change.Dimming != 0) != c.wantDim {
			t.Errorf("%s: speed %d dimming %d, want speed %v dimming %v", c.name, change.Speed, change.Dimming, c.wantSpeed, c.wantDim)
		}
	}
	if _, ok := FindScene("dim-to-warm"); !ok {
		t.Error("dim-to-warm should resolve with its hyphens")
	}
}

// A toy ends by sending Restore of the state it started from.
func TestRestorePutsBackWhatWasShowing(t *testing.T) {
	fireplace, _ := FindScene("Fireplace")
	cases := []struct {
		name  string
		state State
		want  Change
	}{
		{"off", State{On: false, Dimming: 40}, Power(false)},
		{"colour", State{On: true, R: new(254), G: new(128), B: new(25), Dimming: 70}, Color(254, 128, 25, 70)},
		{"white", State{On: true, Temp: 2700, Dimming: 50}, White(2700, 50)},
		{"scene", State{On: true, SceneID: fireplace.ID, Speed: 80, Dimming: 60}, SceneChange(fireplace, 80, 60)},
	}
	for _, c := range cases {
		if got := Restore(c.state); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}
