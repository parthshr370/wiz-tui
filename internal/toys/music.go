package toys

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/gen2brain/malgo"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const (
	// SampleRate is what the mic is opened at.
	SampleRate = 44100
	window     = 1024 // samples per analysis step, about 23 ms

	averageWeight = 1.0 / 64 // running loudness covers about 1.5 s
	beatRatio     = 1.6      // a beat is this much louder than the running loudness
	beatGap       = 250 * time.Millisecond
	silence       = 1e-4  // RMS below this is no sound at all
	quiet         = 0.01  // RMS below this is the room (a laptop mic hears ~0.003), not music
	peakFall      = 0.998 // per window; the loudest recent sound fades over ~10 s
	levelFall     = 0.85  // per window; brightness falls slower than it rises
	musicStep     = 5
	deafAfter     = 3 * time.Second
)

// NoSound is the music status once the mic has been silent for a while,
// usually because macOS has not let the terminal use it.
const NoSound = "no sound: check mic access"

// Listener turns mic samples into a loudness level and a beat count. Feed
// runs on the audio thread; the getters run on the toy's.
type Listener struct {
	mu      sync.Mutex
	pending []float32
	clock   time.Duration // audio time fed so far
	average float64       // running energy
	peak    float64       // loudest recent RMS
	level   float64       // 0..1, brightness to show
	beats   int
	last    time.Duration // when the last beat landed
	heard   bool
}

// Feed takes mono samples at SampleRate.
func (l *Listener) Feed(samples []float32) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, samples...)
	for len(l.pending) >= window {
		l.step(l.pending[:window])
		l.pending = l.pending[window:]
	}
	l.pending = append(l.pending[:0:0], l.pending...)
}

func (l *Listener) step(block []float32) {
	l.clock += time.Second * window / SampleRate
	var sum float64
	for _, s := range block {
		sum += float64(s) * float64(s)
	}
	energy := sum / float64(len(block))
	rms := math.Sqrt(energy)
	if rms < silence {
		l.level *= levelFall
		return
	}
	if !l.heard {
		l.heard, l.average = true, energy // start the room at the first sound, not at silence
	}
	loud := rms >= quiet
	if loud && energy > beatRatio*l.average && l.clock-l.last >= beatGap {
		l.beats++
		l.last = l.clock
	}
	l.average += (energy - l.average) * averageWeight
	now := 0.0
	if loud {
		l.peak = math.Max(rms, l.peak*peakFall)
		now = rms / l.peak
	}
	if now > l.level {
		l.level = now
	} else {
		l.level = l.level*levelFall + now*(1-levelFall)
	}
}

// Snapshot is the listener's state at one moment.
type Snapshot struct {
	Level float64
	Beats int
	Heard bool
}

func (l *Listener) Snapshot() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	return Snapshot{Level: l.level, Beats: l.beats, Heard: l.heard}
}

// Music shows the room's sound: each beat moves to a new colour and the
// loudness sets the brightness. It runs until stopped.
type Music struct {
	listener *Listener
	mic      *malgo.AllocatedContext
	device   *malgo.Device
}

// NewMusic opens the default mic. Close it when done.
func NewMusic() (*Music, error) {
	m := &Music{listener: &Listener{}}
	mic, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("open audio: %w", err)
	}
	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatF32
	config.Capture.Channels = 1
	config.SampleRate = SampleRate
	device, err := malgo.InitDevice(mic.Context, config, malgo.DeviceCallbacks{
		Data: func(_, input []byte, frames uint32) {
			if frames > 0 {
				m.listener.Feed(unsafe.Slice((*float32)(unsafe.Pointer(&input[0])), frames))
			}
		},
	})
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open the mic: %w", err), freeContext(mic))
	}
	if err := device.Start(); err != nil {
		device.Uninit()
		return nil, errors.Join(fmt.Errorf("start the mic: %w", err), freeContext(mic))
	}
	m.mic, m.device = mic, device
	return m, nil
}

func freeContext(mic *malgo.AllocatedContext) error {
	err := mic.Uninit()
	mic.Free()
	return err
}

// Close stops the mic.
func (m *Music) Close() error {
	m.device.Uninit()
	return freeContext(m.mic)
}

func (m *Music) Frame(elapsed time.Duration) (Frame, bool) {
	return musicFrame(m.listener.Snapshot(), elapsed), true
}

func musicFrame(s Snapshot, elapsed time.Duration) Frame {
	r, g, b := HueRGB(float64(s.Beats) * goldenAngle)
	dim := wiz.MinDimming + int(math.Round(s.Level*float64(wiz.MaxDimming-wiz.MinDimming)))
	status := fmt.Sprintf("%-10s %d beats", strings.Repeat("▮", int(math.Round(s.Level*10))), s.Beats)
	if !s.Heard {
		status = "listening…"
		if elapsed > deafAfter {
			status = NoSound
		}
	}
	return Frame{Change: wiz.Color(r, g, b, quantize(dim, musicStep)), Status: status}
}
