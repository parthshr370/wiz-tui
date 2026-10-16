// Command wiz controls Philips WiZ lights on the local network.
//
// With no command it opens the interactive screen. With a command it does one
// thing and exits, so it works from scripts, shortcuts and cron.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/parthshr370/wiz-tui/internal/config"
	"github.com/parthshr370/wiz-tui/internal/tui"
	"github.com/parthshr370/wiz-tui/internal/wiz"
)

const commandTimeout = 10 * time.Second

const usage = `wiz controls Philips WiZ lights on your Wi-Fi.

usage:
  wiz                        open the interactive screen
  wiz [-b LIGHT] COMMAND     run one command and exit

commands:
  status [--json]            show the light's state
  on | off | toggle          power
  dim N                      brightness, 10-100
  warm | cool                warmest or coolest white the light supports
  temp K                     white at K kelvin (e.g. 2700, 4000, 6500)
  red | orange | amber | yellow | green | teal | blue | purple | pink
  color NAME|#RRGGBB         any colour
  scene NAME|ID [SPEED]      built-in scene, speed 10-200 for animated ones
  scenes                     list built-in scenes
  list                       scan the network and list every light
  name NEW NAME              name the selected light
  use LIGHT                  make LIGHT the default for commands
  help                       this text

LIGHT is a name, an IP address or a MAC. Without -b the default light is used.

setup: firmware 1.34 and later only accepts changes when the WiZ app allows it:
  Settings > Security (under Current Home) > Local communication > All controls.
  Or keep "Only verified controls" and save the home's udp_signing_key
  (Settings > Integrations > Local integrations) in ~/.config/wiz/key.

state lives in ~/.config/wiz (override with WIZ_CONFIG_DIR).
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "wiz:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("wiz", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	lightQuery := flags.String("b", "", "light name, IP or MAC")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	key, err := config.SigningKey()
	if err != nil {
		return err
	}
	client := wiz.Client{SigningKey: key}
	store, err := config.Load()
	if err != nil {
		return err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		_, err := tea.NewProgram(tui.New(client, store)).Run()
		return err
	}
	command, params := strings.ToLower(rest[0]), rest[1:]
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()

	switch command {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return nil
	case "scenes":
		kinds := [...]string{"white", "still", "slow", "animated"}
		for _, scene := range wiz.Scenes {
			fmt.Printf("%3d  %-15s %-9s %s\n", scene.ID, scene.Name, kinds[scene.Kind], scene.About)
		}
		return nil
	case "list":
		return listLights(ctx, client, &store)
	}

	bulb, err := config.Find(ctx, client, &store, *lightQuery)
	if err != nil {
		return err
	}
	switch command {
	case "status":
		return printStatus(ctx, client, store, bulb, len(params) > 0 && params[0] == "--json")
	case "name":
		if len(params) == 0 {
			return errors.New("usage: wiz name NEW NAME")
		}
		store.Rename(bulb.MAC, strings.Join(params, " "))
		return store.Save()
	case "use":
		store.Default = bulb.MAC
		if len(params) > 0 {
			mac, ok := store.Resolve(strings.Join(params, " "))
			if !ok {
				return fmt.Errorf("no light matches %q; run `wiz list`", strings.Join(params, " "))
			}
			store.Default = mac
		}
		return store.Save()
	}

	change, err := parseChange(ctx, client, bulb, command, params)
	if err != nil {
		return err
	}
	if err := client.Set(ctx, bulb.IP, change); err != nil {
		return err
	}
	return printStatus(ctx, client, store, bulb, false)
}

func intArg(params []string, what string, low, high int) (int, error) {
	if len(params) == 0 {
		return 0, fmt.Errorf("missing %s", what)
	}
	value, err := strconv.Atoi(params[0])
	if err != nil {
		return 0, fmt.Errorf("%s must be a number, got %q", what, params[0])
	}
	if value < low || value > high {
		return 0, fmt.Errorf("%s must be between %d and %d", what, low, high)
	}
	return value, nil
}

// parseChange maps a command to a setPilot request, keeping the light's
// current brightness when the command does not set one.
func parseChange(ctx context.Context, client wiz.Client, bulb wiz.Bulb, command string, params []string) (wiz.Change, error) {
	state, err := client.Status(ctx, bulb.IP)
	if err != nil {
		return wiz.Change{}, err
	}
	dimming := state.Dimming
	if dimming == 0 {
		dimming = wiz.MaxDimming
	}
	switch command {
	case "on", "off":
		return wiz.Power(command == "on"), nil
	case "toggle":
		return wiz.Power(!state.On), nil
	case "dim", "brightness":
		level, err := intArg(params, "brightness", bulb.MinDim, wiz.MaxDimming)
		return wiz.Dim(level), err
	case "warm":
		return wiz.White(bulb.MinTemp+500, dimming), nil
	case "cool":
		return wiz.White(bulb.MaxTemp, dimming), nil
	case "temp", "white":
		kelvin, err := intArg(params, "temperature", bulb.MinTemp, bulb.MaxTemp)
		return wiz.White(kelvin, dimming), err
	case "scene":
		if len(params) == 0 {
			return wiz.Change{}, errors.New("usage: wiz scene NAME|ID [SPEED]; see `wiz scenes`")
		}
		speed := wiz.DefaultSpeed
		name := strings.Join(params, " ")
		if len(params) > 1 {
			if value, err := strconv.Atoi(params[len(params)-1]); err == nil {
				speed = max(wiz.MinSpeed, min(wiz.MaxSpeed, value))
				name = strings.Join(params[:len(params)-1], " ")
			}
		}
		scene, ok := wiz.FindScene(name)
		if !ok {
			return wiz.Change{}, fmt.Errorf("no scene %q; see `wiz scenes`", name)
		}
		return wiz.SceneChange(scene, speed, dimming), nil
	case "color", "colour":
		if len(params) == 0 {
			return wiz.Change{}, errors.New("usage: wiz color NAME|#RRGGBB")
		}
		command = params[0]
	}
	red, green, blue, err := wiz.ParseColor(command)
	if err != nil {
		return wiz.Change{}, fmt.Errorf("unknown command %q; run `wiz help`", command)
	}
	return wiz.Color(red, green, blue, dimming), nil
}

func lightName(store config.Store, bulb wiz.Bulb) string {
	if name := store.Name(bulb.MAC); name != "" {
		return name
	}
	return "WiZ " + bulb.MAC[len(bulb.MAC)-4:]
}

func printStatus(ctx context.Context, client wiz.Client, store config.Store, bulb wiz.Bulb, asJSON bool) error {
	state, err := client.Status(ctx, bulb.IP)
	if err != nil {
		return err
	}
	if asJSON {
		out, err := json.MarshalIndent(struct {
			Name  string    `json:"name"`
			Bulb  wiz.Bulb  `json:"bulb"`
			State wiz.State `json:"state"`
		}{lightName(store, bulb), bulb, state}, "", "  ")
		if err != nil {
			return fmt.Errorf("encode status: %w", err)
		}
		fmt.Println(string(out))
		return nil
	}
	fmt.Printf("%s (%s): %s\n", lightName(store, bulb), bulb.IP, describe(state))
	return nil
}

func describe(state wiz.State) string {
	if !state.On {
		return "off"
	}
	var what string
	switch state.Mode() {
	case wiz.ModeColor:
		red, green, blue := state.RGB()
		what = fmt.Sprintf("colour #%02x%02x%02x", red, green, blue)
	case wiz.ModeScene:
		what = fmt.Sprintf("scene %d", state.SceneID)
		if i := wiz.SceneIndex(state.SceneID); i >= 0 {
			what = "scene " + wiz.Scenes[i].Name
		}
	default:
		what = fmt.Sprintf("white %d K", state.Temp)
	}
	return fmt.Sprintf("on, %s, %d%%", what, state.Dimming)
}

func listLights(ctx context.Context, client wiz.Client, store *config.Store) error {
	bulbs, err := config.Rediscover(ctx, client, store)
	if err != nil {
		return err
	}
	if len(bulbs) == 0 {
		return errors.New("no WiZ light answered on this network")
	}
	for _, bulb := range bulbs {
		marker := " "
		if bulb.MAC == store.Default {
			marker = "*"
		}
		fmt.Printf("%s %-14s %-15s %s  %s  fw %s  %d-%d K\n", marker, lightName(*store, bulb),
			bulb.IP, bulb.MAC, bulb.Module, bulb.Firmware, bulb.MinTemp, bulb.MaxTemp)
	}
	return nil
}
