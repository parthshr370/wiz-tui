// Package tui is the interactive Bubble Tea screen for WiZ lights.
//
// The screen is a preset browser: moving through colours, moods, whites and
// scenes only changes the on-screen preview. Enter sends the highlighted
// preset to the light; space switches it on or off straight away.
package tui

import (
	"context"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/parthshr370/wiz-tui/internal/config"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const (
	pollEvery      = 3 * time.Second
	frameEvery     = 33 * time.Millisecond  // while the preview eases to a new colour
	sceneFrame     = 100 * time.Millisecond // while it only breathes for an animated scene
	requestTimeout = 2 * time.Second
	tweenRate      = 0.22 // share of the remaining colour distance covered per frame
	dimStep        = 10
	speedStep      = 20
	hueStep        = 6
	satStep        = 0.05
	sleepFadeStep  = 5 // smallest brightness drop the sleep timer sends
)

// sleepChoices are what t cycles through; past the last one the timer is off.
var sleepChoices = []time.Duration{15 * time.Minute, 30 * time.Minute, 60 * time.Minute}

type section int

const (
	sectionFavourites section = iota
	sectionColors
	sectionMoods
	sectionWhites
	sectionScenes
	sectionToys
	sectionCustom
	sectionCount
)

func (s section) String() string {
	return [...]string{"Favourites", "Colours", "Moods", "Whites", "Scenes", "Toys", "Custom"}[s]
}

// sleepTimer fades the light down to the lowest brightness, then turns it
// off at `at`. It runs in this screen, so it stops if wiz is closed.
type sleepTimer struct {
	at    time.Time
	total time.Duration
	from  int // brightness when the timer started
}

// level is the brightness the fade should have reached by now.
func (s sleepTimer) level(now time.Time) int {
	share := math.Max(0, math.Min(1, float64(s.at.Sub(now))/float64(s.total)))
	return wiz.MinDimming + int(math.Round(float64(s.from-wiz.MinDimming)*share))
}

// next is the timer t moves to: the next longer choice, then off.
func (s sleepTimer) next(now time.Time, from int) sleepTimer {
	for _, choice := range sleepChoices {
		if choice > s.total {
			return sleepTimer{at: now.Add(choice), total: choice, from: from}
		}
	}
	return sleepTimer{}
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
	frameMsg time.Time
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

	sections  [sectionCount][]item
	section   section
	cursor    [sectionCount]int
	custom    customRow
	hue       float64
	sat       float64
	dim       int // brightness the next enter will send
	speed     int
	sleep     sleepTimer
	shown     [3]float64 // preview colour as drawn, easing towards the target
	animating bool
	now       time.Time // time of the last preview frame, for the breathing glow
	play      playing
	playGen   int // bumped per toy, so ticks from a stopped toy are dropped

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
	m.sections = [sectionCount][]item{nil, colorItems, moodItems(), whiteItems, sceneItems(), toyItems(), nil}
	m.sections[sectionFavourites] = m.loadFavourites()
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
	if !m.online || m.play.active() {
		return false
	}
	selected := m.selected()
	dimChanged := m.dim != m.state.Dimming && (selected.kind != kindScene || selected.scene.Dimming)
	return !m.state.On || !selected.matches(m.state) || dimChanged
}

// targetColor is what the preview orb eases towards.
func (m Model) targetColor() rgb {
	if m.play.active() {
		return m.toyColor()
	}
	selected := m.selected()
	if selected.kind == kindScene {
		if selected.scene.ID == dimToWarmScene {
			r, g, b := wiz.KelvinRGB(dimToWarmKelvin(m.dim))
			return rgb{r, g, b}.scale(0.3 + 0.7*float64(m.dim)/100)
		}
		if !selected.scene.Dimming {
			return selected.swatch()
		}
	}
	return selected.swatch().scale(0.3 + 0.7*float64(m.dim)/100)
}

func (m Model) lit() bool { return m.state.On || m.pending() || m.play.active() }

func (m Model) breathing() bool {
	selected := m.selected()
	return selected.kind == kindScene && selected.scene.Animates() && m.lit()
}

// liveTakesDimming is false while the light runs a scene that sets its own
// brightness, or one this program does not know.
func (m Model) liveTakesDimming() bool {
	if m.state.SceneID == 0 {
		return true
	}
	i := wiz.SceneIndex(m.state.SceneID)
	return i >= 0 && wiz.Scenes[i].Dimming
}

// tickSleep runs the sleep timer on each poll: off at the end, otherwise one
// brightness step down once the fade has fallen sleepFadeStep below the light.
func (m *Model) tickSleep(now time.Time) tea.Cmd {
	if m.sleep.at.IsZero() || !m.online || m.busy {
		return nil
	}
	if !m.state.On {
		m.sleep = sleepTimer{}
		m.say("the light is off, so the sleep timer is cleared", false)
		return nil
	}
	if !now.Before(m.sleep.at) {
		m.sleep = sleepTimer{}
		m.state.On = false
		return m.applyCmd(wiz.Power(false), "sleep timer done, good night")
	}
	if level := m.sleep.level(now); m.liveTakesDimming() && m.state.Dimming-level >= sleepFadeStep {
		return m.applyCmd(wiz.Dim(level), fmt.Sprintf("sleep timer: dimmed to %d%%", level))
	}
	return nil
}

// glow is the halo size; animated scenes breathe.
func (m Model) glow() float64 {
	if !m.breathing() {
		return 1
	}
	return 1 + 0.18*math.Sin(float64(m.now.UnixMilli())/1000*2*math.Pi/2.4)
}

func (m Model) shownColor() rgb {
	return rgb{int(m.shown[0]), int(m.shown[1]), int(m.shown[2])}
}

// frameInterval returns the preview's frame interval and whether it needs one.
func (m Model) frameInterval() (time.Duration, bool) {
	target := m.targetColor()
	far := math.Abs(m.shown[0]-float64(target.R)) + math.Abs(m.shown[1]-float64(target.G)) + math.Abs(m.shown[2]-float64(target.B))
	switch {
	case far > 2:
		return frameEvery, true
	case m.breathing():
		return sceneFrame, true
	}
	return 0, false
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
// showing. With open it also switches to the first tab holding it, so
// Favourites wins; a colour in no tab opens Custom.
func (m *Model) placeCursorOnLive(state wiz.State, open bool) {
	if state.Dimming > 0 {
		m.dim = state.Dimming
	}
	if state.Speed > 0 {
		m.speed = state.Speed
	}
	liveIn := sectionCount
	for s := range sectionCustom {
		for i, each := range m.sections[s] {
			if each.matches(state) {
				m.cursor[s] = i
				liveIn = min(liveIn, s)
			}
		}
	}
	if state.Mode() == wiz.ModeColor {
		r, g, b := state.RGB()
		m.hue, m.sat = rgbToHS(rgb{r, g, b})
	}
	switch {
	case !open:
	case liveIn != sectionCount:
		m.section = liveIn
	case state.Mode() == wiz.ModeColor:
		m.section = sectionCustom
	}
}

// loadFavourites resolves the stored favourites, falling back to the
// built-in picks when none are stored or none still resolve.
func (m Model) loadFavourites() []item {
	others := m.sections[sectionColors:sectionCustom]
	if items := favouriteItems(m.store.Favourites, others...); len(items) > 0 {
		return items
	}
	return favouriteItems(defaultFavourites, others...)
}

func (m Model) isFavourite(it item) bool {
	return slices.ContainsFunc(m.sections[sectionFavourites], func(each item) bool { return each.key() == it.key() })
}

// toggleFavourite pins the selected preset to Favourites, or unpins it, and
// saves. The last favourite stays, so the first tab is never empty.
func (m *Model) toggleFavourite() {
	selected := m.selected()
	favourites := m.sections[sectionFavourites]
	at := slices.IndexFunc(favourites, func(it item) bool { return it.key() == selected.key() })
	switch {
	case at < 0:
		favourites = append(slices.Clip(favourites), selected)
		if m.section == sectionCustom {
			favourites[len(favourites)-1].name = hex(selected.color)
		}
		m.say("added "+favourites[len(favourites)-1].name+" to favourites", false)
	case len(favourites) == 1:
		m.say("keep at least one favourite", true)
		return
	default:
		m.say("removed "+selected.name+" from favourites", false)
		favourites = slices.Delete(slices.Clone(favourites), at, at+1)
		m.cursor[sectionFavourites] = min(m.cursor[sectionFavourites], len(favourites)-1)
	}
	m.sections[sectionFavourites] = favourites
	m.store.Favourites = make([]string, 0, len(favourites))
	for _, it := range favourites {
		m.store.Favourites = append(m.store.Favourites, it.key())
	}
	if err := m.store.Save(); err != nil {
		m.say(err.Error(), true)
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
	if every, needed := next.frameInterval(); !next.animating && needed {
		next.animating = true
		cmd = tea.Batch(cmd, tea.Tick(every, func(t time.Time) tea.Msg { return frameMsg(t) }))
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
		m.now = time.Time(msg)
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
			m.placeCursorOnLive(msg.state, false)
		}
		return m, nil

	case pollMsg:
		next := tea.Tick(pollEvery, func(time.Time) tea.Msg { return pollMsg{} })
		if m.busy || m.loading || m.play.active() {
			return m, next
		}
		return m, tea.Batch(next, m.statusCmd(), m.tickSleep(time.Now()))

	case appliedMsg:
		m.busy = false
		if msg.err != nil {
			m.say(msg.err.Error(), true)
			return m, nil
		}
		m.say(msg.label, false)
		return m, m.statusCmd()

	case toyTickMsg:
		return m.onToyTick(msg)

	case toySentMsg:
		return m.onToySent(msg)

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
	if m.play.active() {
		cmd := m.stopToy("stopped " + m.play.spec.Name + ", the light is back")
		if matches(msg, k.Quit) {
			return m, tea.Sequence(cmd, tea.Quit)
		}
		return m, cmd
	}
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
			m.placeCursorOnLive(m.state, true)
		}
	case matches(msg, k.Favourite):
		m.toggleFavourite()
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
		if selected.kind == kindToy {
			return m, m.startToy(selected.toy)
		}
		label := fmt.Sprintf("%s at %d%%", selected.name, m.dim)
		return m, m.applyCmd(selected.change(m.dim, m.speed), label)
	case matches(msg, k.Power):
		m.sleep = sleepTimer{}
		on := !m.state.On
		label := "light off"
		if on {
			label = "light on"
		}
		m.state.On = on
		return m, m.applyCmd(wiz.Power(on), label)
	case matches(msg, k.Sleep):
		if !m.online || !m.state.On {
			m.say("turn the light on first, then set the sleep timer", true)
			return m, nil
		}
		m.sleep = m.sleep.next(time.Now(), m.state.Dimming)
		if m.sleep.at.IsZero() {
			m.say("sleep timer off", false)
			return m, nil
		}
		m.say(fmt.Sprintf("sleep timer: dimming to off over %d min, keep wiz open", int(m.sleep.total.Minutes())), false)
		return m, nil
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
