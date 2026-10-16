// Package tui is the interactive Bubble Tea screen for WiZ lights.
//
// The screen is a preset browser: moving through colours, whites and scenes
// only changes the on-screen preview. Enter sends the highlighted preset to
// the light; space switches it on or off straight away.
package tui

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/parthshr370/wiz-tui/internal/config"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const (
	pollEvery      = 3 * time.Second
	frameEvery     = 33 * time.Millisecond
	requestTimeout = 2 * time.Second
	tweenRate      = 0.22 // share of the remaining colour distance covered per frame
	dimStep        = 10
	speedStep      = 20
	hueStep        = 6
	satStep        = 0.05
)

type section int

const (
	sectionColors section = iota
	sectionWhites
	sectionScenes
	sectionCustom
	sectionCount
)

func (s section) String() string {
	return [...]string{"Colours", "Whites", "Scenes", "Custom"}[s]
}

type customRow int

const (
	rowHue customRow = iota
	rowSaturation
	customRows
)

type inputKind int

const (
	inputNone inputKind = iota
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
	appliedMsg struct {
		label string
		err   error
	}
	frameMsg struct{}
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
	synced  bool // the cursor has been placed on the light's live preset once
	loading bool
	busy    bool

	sections [sectionCount][]item
	section  section
	cursor   [sectionCount]int
	custom   customRow
	hue      float64
	sat      float64
	dim      int // brightness the next enter will send
	speed    int

	shown     [3]float64 // preview colour as drawn, easing towards the target
	animating bool
	frame     int

	toast    string
	toastBad bool

	input     textinput.Model
	inputKind inputKind
	help      help.Model
	keys      keyMap
	width     int
	height    int
	columns   int
}

// New builds the model. store holds remembered bulbs and names.
func New(client wiz.Client, store config.Store) Model {
	input := textinput.New()
	input.CharLimit = 32
	helpModel := help.New()
	helpModel.Styles = helpStyles()
	m := Model{
		client:  client,
		store:   store,
		loading: true,
		hue:     28,
		sat:     0.9,
		dim:     wiz.MaxDimming,
		speed:   wiz.DefaultSpeed,
		input:   input,
		help:    helpModel,
		keys:    defaultKeys(),
		toast:   "looking for your lights…",
		columns: 4,
	}
	m.sections = [sectionCount][]item{colorItems, whiteItems, sceneItems(), nil}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.loadCmd(false), tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} }))
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

// selected is the preset under the cursor; in Custom it is the hue picker.
func (m Model) selected() item {
	if m.section == sectionCustom {
		return item{name: "Custom", color: hsvToRGB(m.hue, m.sat)}
	}
	return m.sections[m.section][m.cursor[m.section]]
}

// pending is true when enter would change the light.
func (m Model) pending() bool {
	if !m.online {
		return false
	}
	return !m.state.On || !m.selected().matches(m.state) || m.dim != m.state.Dimming
}

// targetColor is what the preview orb eases towards.
func (m Model) targetColor() rgb {
	return m.selected().swatch().scale(0.3 + 0.7*float64(m.dim)/100)
}

func (m Model) lit() bool { return m.state.On || m.pending() }

func (m Model) breathing() bool {
	selected := m.selected()
	return selected.kind == kindScene && selected.scene.Dynamic && m.lit()
}

// glow is the halo size; animated scenes breathe.
func (m Model) glow() float64 {
	if !m.breathing() {
		return 1
	}
	return 1 + 0.18*math.Sin(float64(m.frame)*frameEvery.Seconds()*2*math.Pi/2.4)
}

func (m Model) shownColor() rgb {
	return rgb{int(m.shown[0]), int(m.shown[1]), int(m.shown[2])}
}

func (m Model) needsFrames() bool {
	target := m.targetColor()
	far := math.Abs(m.shown[0]-float64(target.R)) + math.Abs(m.shown[1]-float64(target.G)) + math.Abs(m.shown[2]-float64(target.B))
	return far > 2 || m.breathing()
}

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

func (m *Model) applyCmd(change wiz.Change, label string) tea.Cmd {
	bulb, ok := m.bulb()
	if !ok {
		return nil
	}
	m.busy = true
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		return appliedMsg{label: label, err: client.Set(ctx, bulb.IP, change)}
	}
}

// placeCursorOnLive moves every section's cursor to the preset the light is
// showing, and opens that section.
func (m *Model) placeCursorOnLive(state wiz.State) {
	if state.Dimming > 0 {
		m.dim = state.Dimming
	}
	if state.Speed > 0 {
		m.speed = state.Speed
	}
	for s := range sectionCustom {
		for i, each := range m.sections[s] {
			if each.matches(state) {
				m.cursor[s], m.section = i, s
			}
		}
	}
	if state.Mode() == wiz.ModeColor {
		r, g, b := state.RGB()
		m.hue, m.sat = rgbToHS(rgb{r, g, b})
		if !m.selected().matches(state) {
			m.section = sectionCustom
		}
	}
}

func (m *Model) say(text string, bad bool) { m.toast, m.toastBad = text, bad }

func (m *Model) move(step int) {
	if m.section == sectionCustom {
		switch m.custom {
		case rowHue:
			m.hue = math.Mod(m.hue+float64(step*hueStep)+360, 360)
		case rowSaturation:
			m.sat = math.Max(0, math.Min(1, m.sat+float64(step)*satStep))
		}
		return
	}
	items := len(m.sections[m.section])
	m.cursor[m.section] = (m.cursor[m.section] + step + items*4) % items
}

func (m *Model) moveRow(step int) {
	if m.section == sectionCustom {
		m.custom = customRow((int(m.custom) + step + int(customRows)) % int(customRows))
		return
	}
	m.move(step * max(1, m.columns))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.update(msg)
	if !next.animating && next.needsFrames() {
		next.animating = true
		cmd = tea.Batch(cmd, tea.Tick(frameEvery, func(time.Time) tea.Msg { return frameMsg{} }))
	}
	return next, cmd
}

func (m Model) update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.columns = gridColumns(m.width)
		m.help.SetWidth(contentWidth(m.width))
		return m, nil

	case frameMsg:
		m.animating = false
		m.frame++
		target := m.targetColor()
		for i, goal := range []int{target.R, target.G, target.B} {
			m.shown[i] += (float64(goal) - m.shown[i]) * tweenRate
		}
		return m, nil

	case loadedMsg:
		return m.onLoaded(msg)

	case statusMsg:
		bulb, ok := m.bulb()
		if !ok || bulb.MAC != msg.mac {
			return m, nil // reply for a light we switched away from
		}
		if msg.err != nil {
			m.online = false
			m.say(msg.err.Error(), true)
			return m, nil
		}
		if !m.online && m.toastBad {
			m.say("", false)
		}
		m.online, m.state = true, msg.state
		if !m.synced {
			m.synced = true
			m.placeCursorOnLive(msg.state)
		}
		return m, nil

	case pollMsg:
		next := tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
		if m.busy || m.loading {
			return m, next
		}
		return m, tea.Batch(next, m.statusCmd())

	case appliedMsg:
		m.busy = false
		if msg.err != nil {
			m.say(msg.err.Error(), true)
			return m, nil
		}
		m.say(msg.label, false)
		return m, m.statusCmd()

	case tea.MouseWheelMsg:
		switch msg.Mouse().Button {
		case tea.MouseWheelUp:
			m.move(-1)
		case tea.MouseWheelDown:
			m.move(1)
		}
		return m, nil

	case tea.KeyPressMsg:
		if m.inputKind != inputNone {
			return m.updateInput(msg)
		}
		return m.updateKeys(msg)
	}
	return m, nil
}

func (m Model) onLoaded(msg loadedMsg) (Model, tea.Cmd) {
	m.loading = false
	if msg.err != nil {
		m.say(msg.err.Error(), true)
		return m, nil
	}
	if len(msg.bulbs) == 0 {
		if !msg.scan {
			m.loading = true
			return m, m.loadCmd(true)
		}
		m.say("no WiZ light answered. Is it powered, and is this Mac on the same Wi-Fi? Press d to scan again.", true)
		return m, nil
	}
	previous, hadPrevious := m.bulb()
	m.bulbs = msg.bulbs
	for _, bulb := range msg.bulbs {
		m.store.Remember(bulb.MAC, bulb.IP)
	}
	if err := m.store.Save(); err != nil {
		m.say(err.Error(), true)
	}
	target := m.store.Default
	if hadPrevious {
		target = previous.MAC
	}
	m.current = 0
	for i, bulb := range msg.bulbs {
		if bulb.MAC == target {
			m.current = i
		}
	}
	m.synced = false
	m.say(fmt.Sprintf("found %d light%s", len(msg.bulbs), plural(len(msg.bulbs))), false)
	return m, m.statusCmd()
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func (m Model) updateInput(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.inputKind = inputNone
		m.input.Blur()
		return m, nil
	case "enter":
		m.inputKind = inputNone
		m.input.Blur()
		bulb, ok := m.bulb()
		if !ok {
			return m, nil
		}
		m.store.Rename(bulb.MAC, m.input.Value())
		if err := m.store.Save(); err != nil {
			m.say(err.Error(), true)
			return m, nil
		}
		m.say("this light is now called "+m.bulbName(bulb), false)
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m Model) updateKeys(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	k := m.keys
	switch {
	case matches(msg, k.Quit):
		return m, tea.Quit
	case matches(msg, k.Help):
		m.help.ShowAll = !m.help.ShowAll
		return m, nil
	case matches(msg, k.Rescan):
		m.loading = true
		m.say("scanning the network…", false)
		return m, m.loadCmd(true)
	case matches(msg, k.NextSection):
		m.section = (m.section + 1) % sectionCount
	case matches(msg, k.PrevSection):
		m.section = (m.section + sectionCount - 1) % sectionCount
	case matches(msg, k.Left):
		m.move(-1)
	case matches(msg, k.Right):
		m.move(1)
	case matches(msg, k.Up):
		m.moveRow(-1)
	case matches(msg, k.Down):
		m.moveRow(1)
	case matches(msg, k.Brighter):
		m.dim = min(wiz.MaxDimming, m.dim+dimStep)
	case matches(msg, k.Dimmer):
		bulb, _ := m.bulb()
		m.dim = max(max(bulb.MinDim, wiz.MinDimming), m.dim-dimStep)
	case matches(msg, k.Faster):
		m.speed = min(wiz.MaxSpeed, m.speed+speedStep)
	case matches(msg, k.Slower):
		m.speed = max(wiz.MinSpeed, m.speed-speedStep)
	case matches(msg, k.Surprise):
		if m.section == sectionCustom {
			m.hue, m.sat = rand.Float64()*360, 0.6+rand.Float64()*0.4
		} else {
			m.cursor[m.section] = rand.IntN(len(m.sections[m.section]))
		}
	case matches(msg, k.Revert):
		if m.online {
			m.placeCursorOnLive(m.state)
		}
	}
	if _, ok := m.bulb(); !ok {
		return m, nil
	}
	switch {
	case matches(msg, k.Apply):
		if m.busy {
			return m, nil
		}
		selected := m.selected()
		label := fmt.Sprintf("%s at %d%%", selected.name, m.dim)
		return m, m.applyCmd(selected.change(m.dim, m.speed), label)
	case matches(msg, k.Power):
		on := !m.state.On
		label := "light off"
		if on {
			label = "light on"
		}
		m.state.On = on
		return m, m.applyCmd(wiz.Power(on), label)
	case matches(msg, k.Rename):
		bulb, _ := m.bulb()
		m.inputKind = inputRename
		m.input.Prompt = "name › "
		m.input.SetValue(m.store.Name(bulb.MAC))
		m.input.CursorEnd()
		return m, m.input.Focus()
	case matches(msg, k.NextBulb):
		m.current = (m.current + 1) % len(m.bulbs)
		m.online, m.synced = false, false
		m.store.Default = m.bulbs[m.current].MAC
		if err := m.store.Save(); err != nil {
			m.say(err.Error(), true)
		}
		return m, m.statusCmd()
	}
	return m, nil
}
