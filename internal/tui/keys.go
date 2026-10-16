package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type keyMap struct {
	Left, Right, Up, Down          key.Binding
	NextSection, PrevSection       key.Binding
	Apply, Power, Revert, Surprise key.Binding
	Brighter, Dimmer               key.Binding
	Faster, Slower, Sleep          key.Binding
	NextBulb, Rename, Rescan       key.Binding
	Help, Quit                     key.Binding
}

func defaultKeys() keyMap {
	bind := func(help, desc string, keys ...string) key.Binding {
		return key.NewBinding(key.WithKeys(keys...), key.WithHelp(help, desc))
	}
	return keyMap{
		Left:        bind("←/h", "previous", "left", "h"),
		Right:       bind("→/l", "next", "right", "l"),
		Up:          bind("↑/k", "row up", "up", "k"),
		Down:        bind("↓/j", "row down", "down", "j"),
		NextSection: bind("tab", "next tab", "tab"),
		PrevSection: bind("⇧tab", "previous tab", "shift+tab"),
		Apply:       bind("enter", "apply to the light", "enter"),
		Power:       bind("space", "on / off now", "space"),
		Revert:      bind("esc", "back to what is on", "esc"),
		Surprise:    bind("z", "surprise me", "z"),
		Brighter:    bind("+", "brighter", "+", "=", "]"),
		Dimmer:      bind("-", "dimmer", "-", "_", "["),
		Faster:      bind(".", "scene faster", ".", ">"),
		Slower:      bind(",", "scene slower", ",", "<"),
		Sleep:       bind("t", "sleep timer", "t"),
		NextBulb:    bind("b", "next light", "b"),
		Rename:      bind("n", "name this light", "n"),
		Rescan:      bind("d", "scan network", "d"),
		Help:        bind("?", "all keys", "?"),
		Quit:        bind("q", "quit", "q", "ctrl+c"),
	}
}

// ShortHelp is one line, so the arrows are summarised.
func (k keyMap) ShortHelp() []key.Binding {
	short := func(keys, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
	}
	return []key.Binding{
		short("←→↑↓", "browse"), short("enter", "apply"), short("tab", "tabs"),
		short("+/-", "bright"), short("space", "power"), short("?", "more"), short("q", "quit"),
	}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Left, k.Right, k.Up, k.Down, k.NextSection, k.PrevSection, k.Help},
		{k.Apply, k.Power, k.Revert, k.Surprise, k.Brighter, k.Dimmer, k.Sleep},
		{k.Faster, k.Slower, k.NextBulb, k.Rename, k.Rescan, k.Quit},
	}
}

func matches(msg tea.KeyPressMsg, bindings ...key.Binding) bool {
	return key.Matches(msg, bindings...)
}
