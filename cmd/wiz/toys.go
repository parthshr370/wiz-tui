package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/parthshr370/wiz-tui/internal/config"
	"github.com/parthshr370/wiz-tui/internal/toys"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const (
	maxBreatheMinutes = 120
	micHint           = "macOS gives the mic only to apps you allow: System Settings, Privacy & Security, Microphone, then turn on your terminal."
)

// newToy builds the toy a command names. Music holds the mic open and must be
// closed; the others close to nothing.
func newToy(command string, params []string) (toys.Toy, string, error) {
	switch command {
	case "breathe":
		pattern := toys.BoxBreathing
		if len(params) > 0 {
			pattern, params = params[0], params[1:]
		}
		breathe, err := toys.NewBreathe(pattern, 0)
		if err != nil {
			return nil, "", err
		}
		if len(params) > 0 {
			minutes, err := intArg(params, "minutes", 1, maxBreatheMinutes)
			if err != nil {
				return nil, "", err
			}
			breathe.Rounds = int(math.Ceil(float64(time.Duration(minutes)*time.Minute) / float64(breathe.Cycle())))
		}
		return breathe, toyName(pattern), nil
	case "party":
		bpm := toys.DefaultBPM
		if len(params) > 0 {
			var err error
			if bpm, err = intArg(params, "bpm", toys.MinBPM, toys.MaxBPM); err != nil {
				return nil, "", err
			}
		}
		return toys.Party{BPM: bpm}, toyName(toys.PartyID), nil
	default:
		toy, err := toys.New(toys.MusicID)
		return toy, toyName(toys.MusicID), err
	}
}

func toyName(id string) string {
	for _, spec := range toys.Catalogue {
		if spec.ID == id {
			return spec.Name
		}
	}
	return id
}

// playToy runs a toy on the light until it ends or ctrl-c, then puts the
// light back the way it was.
func playToy(client wiz.Client, bulb wiz.Bulb, command string, params []string) error {
	toy, name, err := newToy(command, params)
	if err != nil {
		return err
	}
	readCtx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	before, err := client.Status(readCtx, bulb.IP)
	cancel()
	if err != nil {
		return errors.Join(err, toys.Close(toy))
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	fmt.Fprintf(os.Stderr, "%s. ctrl-c stops and puts the light back.\n", name)
	var last string
	playErr := toys.Play(ctx, client, bulb.IP, toy, func(status string) {
		if status != last {
			last = status
			fmt.Printf("\r\033[K%s", status)
		}
	})
	stop()
	fmt.Println()
	if last == toys.NoSound {
		fmt.Fprintln(os.Stderr, micHint)
	}

	restoreCtx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	return errors.Join(playErr, toys.Close(toy), client.Set(restoreCtx, bulb.IP, wiz.Restore(before)))
}

// rollDice sends a random scene or colour at the light's brightness.
func rollDice(ctx context.Context, client wiz.Client, store config.Store, bulb wiz.Bulb) error {
	state, err := client.Status(ctx, bulb.IP)
	if err != nil {
		return err
	}
	dimming := state.Dimming
	if dimming == 0 {
		dimming = wiz.MaxDimming
	}
	name, change := toys.Dice(dimming, wiz.DefaultSpeed)
	if err := client.Set(ctx, bulb.IP, change); err != nil {
		return err
	}
	fmt.Println("rolled", name)
	return printStatus(ctx, client, store, bulb, false)
}
