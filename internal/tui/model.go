// Package tui is the interactive Bubble Tea screen for WiZ lights.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/parthshr370/wiz-tui/internal/config"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const (
	pollEvery      = 3 * time.Second
	debounce       = 90 * time.Millisecond
	editQuietSpell = 1500 * time.Millisecond // poll results do not overwrite fresh edits
	requestTimeout = 2 * time.Second
	defaultTemp    = 2700
)

type row int

const (
	rowPower row = iota
	rowBright
	rowTemp
	rowRed
	rowGreen
	rowBlue
	rowScene
	rowSpeed
)

var rowsByMode = map[wiz.Mode][]row{
	wiz.ModeWhite: {rowPower, rowBright, rowTemp},
	wiz.ModeColor: {rowPower, rowBright, rowRed, rowGreen, rowBlue},
	wiz.ModeScene: {rowPower, rowBright, rowScene, rowSpeed},
}

type inputKind int

const (
	inputNone inputKind = iota
	inputHex
	inputRename
)

type (
	loadedMsg struct {
		bulbs []wiz.Bulb
		err   error
		scan  bool
	}
	statusMsg struct {
		mac   string
		state wiz.State
		err   error
	}
	sentMsg  struct{ err error }
	flushMsg struct{ seq int }
	pollMsg  struct{}
)

// Model is the whole screen state.
type Model struct {
	client wiz.Client
	store  config.Store

	bulbs   []wiz.Bulb
	current int
	state   wiz.State
	online  bool
	loading bool

	// What the user is steering the light towards; synced from polls when idle.
	mode     wiz.Mode
	on       bool
	dim      int
	temp     int
	red      int
	green    int
	blue     int
	sceneIdx int
	speed    int

	focus    int
	seq      int
	lastEdit time.Time
	busy     int

	note string
	err  string

	input     textinput.Model
	inputKind inputKind
	spinner   spinner.Model
	help      help.Model
	keys      keyMap
	width     int
	height    int
}

// New builds the model. store holds remembered bulbs and names.
func New(client wiz.Client, store config.Store) Model {
	input := textinput.New()
	input.CharLimit = 32
	helpModel := help.New()
	helpModel.Styles = helpStyles()
	return Model{
		client:  client,
		store:   store,
		loading: true,
		dim:     wiz.MaxDimming,
		temp:    defaultTemp,
		red:     wiz.Presets[1].R, green: wiz.Presets[1].G, blue: wiz.Presets[1].B,
		speed:   wiz.DefaultSpeed,
		spinner: spinner.New(spinner.WithSpinner(spinner.MiniDot), spinner.WithStyle(accentText)),
		input:   input,
		help:    helpModel,
		keys:    defaultKeys(),
		note:    "looking for lights…",
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadCmd(false), tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} }))
}

func (m Model) bulb() (wiz.Bulb, bool) {
	if m.current < 0 || m.current >= len(m.bulbs) {
		return wiz.Bulb{}, false
	}
	return m.bulbs[m.current], true
}

func (m Model) bulbName(bulb wiz.Bulb) string {
	if name := m.store.Name(bulb.MAC); name != "" {
		return name
	}
	return "WiZ " + bulb.MAC[len(bulb.MAC)-4:]
}

// loadCmd finds bulbs: remembered ones first, a full network scan when
// forced or when none of them answer.
func (m Model) loadCmd(forceScan bool) tea.Cmd {
	client, store := m.client, m.store
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if !forceScan {
			if bulbs := config.KnownBulbs(ctx, client, store); len(bulbs) > 0 {
				return loadedMsg{bulbs: bulbs}
			}
		}
		bulbs, err := client.Discover(ctx, wiz.DefaultScanWindow)
		return loadedMsg{bulbs: bulbs, err: err, scan: true}
	}
}

func (m Model) statusCmd() tea.Cmd {
	bulb, ok := m.bulb()
	if !ok {
		return nil
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		state, err := client.Status(ctx, bulb.IP)
		return statusMsg{mac: bulb.MAC, state: state, err: err}
	}
}

func (m *Model) sendCmd(change wiz.Change) tea.Cmd {
	bulb, ok := m.bulb()
	if !ok {
		return nil
	}
	m.busy++
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return sentMsg{err: client.Set(ctx, bulb.IP, change)}
	}
}

// change is the setPilot request that makes the light match the model.
func (m Model) change() wiz.Change {
	if !m.on {
		return wiz.Power(false)
	}
	switch m.mode {
	case wiz.ModeColor:
		return wiz.Color(m.red, m.green, m.blue, m.dim)
	case wiz.ModeScene:
		return wiz.SceneChange(wiz.Scenes[m.sceneIdx], m.speed, m.dim)
	default:
		return wiz.White(m.temp, m.dim)
	}
}

// edited marks a local change and sends it after a short pause, so holding
// an arrow key sends one request per pause instead of one per repeat.
func (m *Model) edited() tea.Cmd {
	m.lastEdit = time.Now()
	m.err = ""
	m.seq++
	seq := m.seq
	return tea.Tick(debounce, func(time.Time) tea.Msg { return flushMsg{seq: seq} })
}

func (m *Model) syncFromState(state wiz.State) {
	m.on = state.On
	if state.Dimming > 0 {
		m.dim = state.Dimming
	}
	m.mode = state.Mode()
	switch m.mode {
	case wiz.ModeWhite:
		if state.Temp > 0 {
			m.temp = state.Temp
		}
	case wiz.ModeColor:
		m.red, m.green, m.blue = state.RGB()
	case wiz.ModeScene:
		if i := wiz.SceneIndex(state.SceneID); i >= 0 {
			m.sceneIdx = i
		}
		if state.Speed > 0 {
			m.speed = state.Speed
		}
	}
	m.clampFocus()
}

func (m *Model) clampFocus() {
	if rows := rowsByMode[m.mode]; m.focus >= len(rows) {
		m.focus = len(rows) - 1
	}
}

func (m Model) focusedRow() row { return rowsByMode[m.mode][m.focus] }

func clamp(value, low, high int) int { return max(low, min(high, value)) }

// adjust moves the focused control by steps (negative = left).
func (m *Model) adjust(steps int, big bool) {
	bulb, _ := m.bulb()
	pick := func(small, large int) int {
		if big {
			return large * steps
		}
		return small * steps
	}
	switch m.focusedRow() {
	case rowPower:
		m.on = steps > 0
		return
	case rowBright:
		m.dim = clamp(m.dim+pick(5, 20), max(bulb.MinDim, 1), wiz.MaxDimming)
	case rowTemp:
		m.temp = clamp(m.temp+pick(100, 500), bulb.MinTemp, bulb.MaxTemp)
	case rowRed:
		m.red = clamp(m.red+pick(5, 32), 0, 255)
	case rowGreen:
		m.green = clamp(m.green+pick(5, 32), 0, 255)
	case rowBlue:
		m.blue = clamp(m.blue+pick(5, 32), 0, 255)
	case rowScene:
		m.sceneIdx = (m.sceneIdx + pick(1, 6) + len(wiz.Scenes)*8) % len(wiz.Scenes)
	case rowSpeed:
		m.speed = clamp(m.speed+pick(10, 40), wiz.MinSpeed, wiz.MaxSpeed)
	}
	m.on = true
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(panelWidth(m.width))
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case loadedMsg:
		return m.onLoaded(msg)

	case statusMsg:
		bulb, ok := m.bulb()
		if !ok || bulb.MAC != msg.mac {
			return m, nil // a reply for a bulb we have since switched away from
		}
		if msg.err != nil {
			m.online = false
			m.err = msg.err.Error()
			return m, nil
		}
		if !m.online && strings.HasPrefix(m.err, "getPilot") {
			m.err = ""
		}
		m.online, m.state = true, msg.state
		if time.Since(m.lastEdit) > editQuietSpell {
			m.syncFromState(msg.state)
		}
		return m, nil

	case pollMsg:
		next := tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
		if m.busy > 0 || m.loading {
			return m, next
		}
		return m, tea.Batch(next, m.statusCmd())

	case flushMsg:
		if msg.seq != m.seq {
			return m, nil
		}
		return m, m.sendCmd(m.change())

	case sentMsg:
		m.busy--
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		return m, m.statusCmd()

	case tea.KeyPressMsg:
		if m.inputKind != inputNone {
			return m.updateInput(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m Model) onLoaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	m.loading = false
	if msg.err != nil {
		m.err = msg.err.Error()
		m.note = ""
		return m, nil
	}
	if len(msg.bulbs) == 0 {
		if !msg.scan {
			m.loading = true
			return m, m.loadCmd(true)
		}
		m.note = ""
		m.err = "no WiZ light answered on this network. Is it powered, and is this Mac on the same Wi-Fi? Press d to scan again."
		return m, nil
	}
	previous, hadPrevious := m.bulb()
	m.bulbs = msg.bulbs
	for _, bulb := range msg.bulbs {
		m.store.Remember(bulb.MAC, bulb.IP)
	}
	if err := m.store.Save(); err != nil {
		m.err = err.Error()
	}
	m.current = 0
	target := m.store.Default
	if hadPrevious {
		target = previous.MAC
	}
	for i, bulb := range msg.bulbs {
		if bulb.MAC == target {
			m.current = i
		}
	}
	m.note = fmt.Sprintf("found %d light%s", len(msg.bulbs), plural(len(msg.bulbs)))
	m.lastEdit = time.Time{}
	return m, m.statusCmd()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputKind = inputNone
		m.input.Blur()
		return m, nil
	case "enter":
		kind, value := m.inputKind, m.input.Value()
		m.inputKind = inputNone
		m.input.Blur()
		return m.commitInput(kind, value)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) commitInput(kind inputKind, value string) (tea.Model, tea.Cmd) {
	switch kind {
	case inputHex:
		red, green, blue, err := wiz.ParseColor(value)
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.mode, m.red, m.green, m.blue, m.on = wiz.ModeColor, red, green, blue, true
		m.clampFocus()
		return m, m.edited()
	case inputRename:
		bulb, ok := m.bulb()
		if !ok {
			return m, nil
		}
		m.store.Rename(bulb.MAC, value)
		if err := m.store.Save(); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.note = "renamed to " + m.bulbName(bulb)
	}
	return m, nil
}

func (m Model) startInput(kind inputKind, prompt, value string) (tea.Model, tea.Cmd) {
	m.inputKind = kind
	m.input.Prompt = prompt
	m.input.SetValue(value)
	m.input.CursorEnd()
	return m, m.input.Focus()
}

func (m Model) updateKeys(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := m.keys
	switch {
	case matches(msg, k.Quit):
		return m, tea.Quit
	case matches(msg, k.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	case matches(msg, k.Rescan):
		m.loading, m.note, m.err = true, "scanning the network…", ""
		return m, m.loadCmd(true)
	}
	if _, ok := m.bulb(); !ok {
		return m, nil
	}
	switch {
	case matches(msg, k.Power):
		m.on = !m.on
		m.lastEdit = time.Now()
		return m, m.sendCmd(m.change())
	case matches(msg, k.NextMode), matches(msg, k.PrevMode):
		step := 1
		if matches(msg, k.PrevMode) {
			step = 2
		}
		m.mode = wiz.Mode((int(m.mode) + step) % 3)
		m.on = true
		m.clampFocus()
		return m, m.edited()
	case matches(msg, k.Up):
		m.focus = (m.focus + len(rowsByMode[m.mode]) - 1) % len(rowsByMode[m.mode])
	case matches(msg, k.Down):
		m.focus = (m.focus + 1) % len(rowsByMode[m.mode])
	case matches(msg, k.Left), matches(msg, k.Right), matches(msg, k.BigLeft), matches(msg, k.BigRight):
		steps := 1
		if matches(msg, k.Left) || matches(msg, k.BigLeft) {
			steps = -1
		}
		m.adjust(steps, matches(msg, k.BigLeft, k.BigRight))
		return m, m.edited()
	case matches(msg, k.Warm), matches(msg, k.Cool):
		bulb, _ := m.bulb()
		m.mode, m.on, m.temp = wiz.ModeWhite, true, bulb.MinTemp+500
		if matches(msg, k.Cool) {
			m.temp = bulb.MaxTemp
		}
		m.clampFocus()
		return m, m.edited()
	case matches(msg, k.Preset):
		preset := wiz.Presets[int(msg.String()[0]-'1')]
		m.mode, m.on = wiz.ModeColor, true
		m.red, m.green, m.blue = preset.R, preset.G, preset.B
		m.clampFocus()
		m.note = preset.Name
		return m, m.edited()
	case matches(msg, k.Hex):
		return m.startInput(inputHex, "colour # ", fmt.Sprintf("%02x%02x%02x", m.red, m.green, m.blue))
	case matches(msg, k.Rename):
		bulb, _ := m.bulb()
		return m.startInput(inputRename, "name › ", m.store.Name(bulb.MAC))
	case matches(msg, k.NextBulb), matches(msg, k.PrevBulb):
		step := 1
		if matches(msg, k.PrevBulb) {
			step = len(m.bulbs) - 1
		}
		m.current = (m.current + step) % len(m.bulbs)
		m.online, m.lastEdit = false, time.Time{}
		m.store.Default = m.bulbs[m.current].MAC
		if err := m.store.Save(); err != nil {
			m.err = err.Error()
		}
		return m, m.statusCmd()
	case matches(msg, k.Refresh):
		m.lastEdit = time.Time{}
		return m, m.statusCmd()
	}
	return m, nil
}
