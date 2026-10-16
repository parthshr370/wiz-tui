package tui

import (
	"testing"

	"github.com/parthshr370/wiz-tui/internal/config"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

// A typo in a default key would silently drop that favourite.
func TestEveryDefaultFavouriteResolves(t *testing.T) {
	m := New(wiz.Client{}, config.Store{})
	if got := len(m.sections[sectionFavourites]); got != len(defaultFavourites) {
		t.Fatalf("%d of %d default favourites resolved", got, len(defaultFavourites))
	}
	if first := m.sections[sectionFavourites][0].name; first != "Warm" {
		t.Errorf("first favourite is %q, want Warm", first)
	}
}

func TestPinnedFavouritesSurviveARestart(t *testing.T) {
	t.Setenv("WIZ_CONFIG_DIR", t.TempDir())
	m := New(wiz.Client{}, config.Store{})

	m.section = sectionCustom
	m.hue, m.sat = 200, 1
	custom := hsvToRGB(200, 1)
	m.toggleFavourite()

	m.section = sectionFavourites
	m.cursor[sectionFavourites] = 0 // Warm
	m.toggleFavourite()

	store, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	reopened := New(wiz.Client{}, store).sections[sectionFavourites]
	if len(reopened) != len(defaultFavourites) {
		t.Fatalf("got %d favourites after one pin and one unpin, want %d", len(reopened), len(defaultFavourites))
	}
	if reopened[0].name == "Warm" {
		t.Error("Warm was unpinned but came back")
	}
	last := reopened[len(reopened)-1]
	if last.color != custom || last.name != hex(custom) {
		t.Errorf("custom favourite came back as %q %v, want %s", last.name, last.color, hex(custom))
	}
}

func TestTheLastFavouriteCannotBeRemoved(t *testing.T) {
	t.Setenv("WIZ_CONFIG_DIR", t.TempDir())
	m := New(wiz.Client{}, config.Store{})
	m.section = sectionFavourites
	for range len(defaultFavourites) + 2 {
		m.toggleFavourite()
	}
	if got := len(m.sections[sectionFavourites]); got != 1 {
		t.Fatalf("%d favourites left, want 1", got)
	}
}
