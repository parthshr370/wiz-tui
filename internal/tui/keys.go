package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type keyMap struct {
	Up, Down, Left, Right, BigLeft, BigRight key.Binding
	Power, NextMode, PrevMode                key.Binding
	Warm, Cool, Preset, Hex                  key.Binding
	NextBulb, PrevBulb, Rename, Rescan       key.Binding
	Refresh, Help, Quit                      key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "previous control")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "next control")),
		Left:     key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "decrease")),
		Right:    key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "increase")),
		BigLeft:  key.NewBinding(key.WithKeys("shift+left", "H"), key.WithHelp("⇧←/H", "decrease more")),
		BigRight: key.NewBinding(key.WithKeys("shift+right", "L"), key.WithHelp("⇧→/L", "increase more")),
		Power:    key.NewBinding(key.WithKeys("space", "p"), key.WithHelp("space", "on/off")),
		NextMode: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "white/colour/scene")),
		PrevMode: key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("⇧tab", "previous mode")),
		Warm:     key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "warm white")),
		Cool:     key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cool white")),
		Preset:   key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1-9", "colour presets")),
		Hex:      key.NewBinding(key.WithKeys("#", "x"), key.WithHelp("#", "type a hex colour")),
		NextBulb: key.NewBinding(key.WithKeys("b", "]"), key.WithHelp("b", "next light")),
		PrevBulb: key.NewBinding(key.WithKeys("B", "["), key.WithHelp("B", "previous light")),
		Rename:   key.NewBinding(key.WithKeys("n"), key.WithHelp("n", "name this light")),
		Rescan:   key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "scan network")),
		Refresh:  key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "re-read light")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "all keys")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c", "esc"), key.WithHelp("q", "quit")),
	}
}

// ShortHelp shows one line; the arrows are summarised so it fits the panel.
func (k keyMap) ShortHelp() []key.Binding {
	short := func(keys, desc string) key.Binding {
		return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, desc))
	}
	return []key.Binding{
		short("space", "on/off"), short("tab", "mode"), short("↑↓", "choose"),
		short("←→", "adjust"), short("1-9", "colours"), short("?", "more"), short("q", "quit"),
	}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Left, k.Right, k.BigLeft, k.BigRight},
		{k.Power, k.NextMode, k.PrevMode, k.Warm, k.Cool, k.Preset, k.Hex},
		{k.NextBulb, k.PrevBulb, k.Rename, k.Rescan, k.Refresh, k.Help, k.Quit},
	}
}

func matches(msg tea.KeyPressMsg, bindings ...key.Binding) bool {
	return key.Matches(msg, bindings...)
}
