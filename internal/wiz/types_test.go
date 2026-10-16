package wiz

import "testing"

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
