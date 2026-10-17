package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/parthshr370/wiz-tui/internal/toys"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

// playing is a toy running on the light. A zero value means none is.
type playing struct {
	spec     toys.Spec
	toy      toys.Toy
	started  time.Time
	before   wiz.State // what the light showed when the toy started
	frame    toys.Frame
	sent     wiz.Change
	hasSent  bool
	inflight bool
	failures int
}

func (p playing) active() bool { return p.toy != nil }

type (
	toyTickMsg struct{ gen int }
	toySentMsg struct {
		gen    int
		change wiz.Change
		err    error
	}
)

// startToy plays the toy spec names, or rolls the dice.
func (m *Model) startToy(spec toys.Spec) tea.Cmd {
	if !m.online {
		m.say("the light is not answering yet", true)
		return nil
	}
	if spec.ID == toys.DiceID {
		name, change := toys.Dice(m.dim, m.speed)
		return m.applyCmd(change, "rolled "+name)
	}
	toy, err := toys.New(spec.ID)
	if err != nil {
		m.say(err.Error(), true)
		return nil
	}
	m.playGen++
	m.play = playing{spec: spec, toy: toy, started: time.Now(), before: m.state}
	m.say(spec.Name+" · any key stops it and puts the light back", false)
	return m.toyTick()
}

func (m Model) toyTick() tea.Cmd {
	gen := m.playGen
	return tea.Tick(toys.FrameEvery, func(time.Time) tea.Msg { return toyTickMsg{gen} })
}

// onToyTick takes the next frame and sends it when it differs from the last
// one the light accepted. One frame is in flight at a time, so a slow light
// gets fewer frames rather than a queue of stale ones.
func (m Model) onToyTick(msg toyTickMsg) (Model, tea.Cmd) {
	if msg.gen != m.playGen || !m.play.active() {
		return m, nil
	}
	frame, ok := m.play.toy.Frame(time.Since(m.play.started))
	if !ok {
		return m, m.stopToy(m.play.spec.Name + " finished, the light is back")
	}
	m.play.frame = frame
	if frame.Status == toys.NoSound {
		m.say("no sound: allow the mic for your terminal in System Settings, Privacy & Security, Microphone", true)
	}
	cmds := []tea.Cmd{m.toyTick()}
	if !m.play.inflight && (!m.play.hasSent || !toys.Same(m.play.sent, frame.Change)) {
		m.play.inflight = true
		cmds = append(cmds, m.sendFrameCmd(frame.Change))
	}
	return m, tea.Batch(cmds...)
}

func (m Model) sendFrameCmd(change wiz.Change) tea.Cmd {
	bulb, ok := m.bulb()
	if !ok {
		return nil
	}
	client, gen := m.client, m.playGen
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), toys.SendTimeout)
		defer cancel()
		return toySentMsg{gen: gen, change: change, err: client.Set(ctx, bulb.IP, change)}
	}
}

func (m Model) onToySent(msg toySentMsg) (Model, tea.Cmd) {
	if msg.gen != m.playGen || !m.play.active() {
		return m, nil
	}
	m.play.inflight = false
	if msg.err != nil {
		m.play.failures++
		if m.play.failures >= toys.MaxFailures {
			return m, m.stopToy(fmt.Sprintf("stopped %s, the light missed %d frames: %v", m.play.spec.Name, toys.MaxFailures, msg.err))
		}
		return m, nil
	}
	m.play.failures = 0
	m.play.sent, m.play.hasSent = msg.change, true
	return m, nil
}

// stopToy ends the toy and puts the light back the way it was.
func (m *Model) stopToy(label string) tea.Cmd {
	stopped := m.play
	m.play = playing{spec: stopped.spec}
	closeErr := toys.Close(stopped.toy)
	cmd := m.applyCmd(wiz.Restore(stopped.before), label)
	if closeErr != nil {
		m.say(errors.Join(errors.New("closing "+stopped.spec.Name), closeErr).Error(), true)
	}
	return cmd
}

// toyColor is the colour the running toy last asked for, as drawn.
func (m Model) toyColor() rgb {
	change := m.play.frame.Change
	if change.R == nil || change.G == nil || change.B == nil {
		return toyColour[m.play.spec.ID]
	}
	return rgb{*change.R, *change.G, *change.B}.scale(0.3 + 0.7*float64(change.Dimming)/100)
}
