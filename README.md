# wiz-tui

Control Philips WiZ lights from the terminal, over your own Wi-Fi. No cloud,
no account, no phone. Run `wiz` for an interactive screen, or `wiz red` for
one command that scripts, shortcuts and cron can call.

Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea),
[Lip Gloss](https://github.com/charmbracelet/lipgloss) and
[Bubbles](https://github.com/charmbracelet/bubbles) v2.

## Install

```sh
go install github.com/parthshr370/wiz-tui/cmd/wiz@latest
```

Or from a checkout: `go build -o ~/.local/bin/wiz ./cmd/wiz`.

## One-time setup in the WiZ app

WiZ firmware 1.34 and later accepts local changes only when the app allows it.
Reading the light always works; changing it fails with `Invalid params` until
you pick one of these:

1. **All controls** (simplest). WiZ app: Settings, Security (under Current
   Home), Local communication, **All controls**. If it already says All
   controls and changes still fail, switch to "Only verified controls", save,
   switch back, save. The app can show the wrong value on 1.38.0
   ([home-assistant/core#177463](https://github.com/home-assistant/core/issues/177463)).
2. **Only verified controls** (stricter). Settings, Integrations, Local
   integrations, Get the link. Open the link within 15 minutes, copy the
   32-character `udp_signing_key`, and save it:

   ```sh
   mkdir -p ~/.config/wiz && printf '%s' 'KEY' > ~/.config/wiz/key && chmod 600 ~/.config/wiz/key
   ```

   `wiz` then signs every change (HMAC-SHA256 over the params, as in
   [this reference](https://gist.github.com/rmcolber/ccb5d804c36270efd1e1dfd239efe740)).
   The header shows `signed` when a key is loaded.

## The interactive screen

`wiz` opens a panel with a live colour preview, the light's IP, firmware and
Wi-Fi signal, and three modes:

- **White**: brightness and colour temperature, across the range the bulb
  reports (2200 to 6500 K on the colour bulbs).
- **Colour**: brightness and red, green and blue sliders, nine presets on keys
  1 to 9, and `#` to type any hex colour.
- **Scene**: all 36 built-in scenes in a grid, with speed for the animated ones.

Changes are sent as you move a slider, batched so holding a key sends one
request per pause rather than one per key repeat. The screen re-reads the
light every 3 seconds, so a change from the phone app shows up too.

| Key | Does |
|---|---|
| `space` / `p` | on or off |
| `tab` / `shift+tab` | next or previous mode |
| `↑` `↓` / `k` `j` | choose a control |
| `←` `→` / `h` `l` | adjust it; `shift` or `H` `L` for bigger steps |
| `w` / `c` | warm or cool white |
| `1`…`9` | colour presets |
| `#` | type a hex colour |
| `b` / `B` | next or previous light |
| `n` | name this light |
| `d` | scan the network again |
| `r` | re-read the light now |
| `?` | all keys |
| `q` / `esc` | quit |

## Commands

```text
wiz status [--json]          show the light's state
wiz on | off | toggle
wiz dim 40                   brightness 10-100
wiz warm | cool
wiz temp 4000                white at 4000 K
wiz red                      also orange amber yellow green teal blue purple pink
wiz color '#fe8019'
wiz scene fireplace 150      scene by name or id, optional speed 10-200
wiz scenes                   list scenes
wiz list                     scan and list every light
wiz name Desk lamp           name the selected light
wiz use desk                 make it the default
wiz -b 192.168.1.40 off      target one light by name, IP or MAC
```

Every change prints the light's state afterwards, for example
`Room (192.168.68.104): on, colour #ff0000, 100%`.

## How it works

WiZ lights listen for JSON on UDP port 38899. `getPilot` reads the state,
`setPilot` changes it, `getSystemConfig` and `getModelConfig` describe the
module, firmware, colour temperature range and minimum brightness.

Discovery sends `getPilot` to every address on each local /22 or smaller
network (many routers drop broadcast UDP) and keeps the ones that answer.
Lights are remembered by MAC in `~/.config/wiz/bulbs.json`, so a light that
gets a new IP from the router is found again by a rescan. Set
`WIZ_CONFIG_DIR` to keep state elsewhere.

Local control only works on the same network as the light. Away from home,
use the WiZ app.

## Layout

```text
cmd/wiz            CLI entry point; no command opens the TUI
internal/wiz       UDP protocol: requests, signing, discovery, scenes, colours
internal/config    ~/.config/wiz: signing key, remembered lights and names
internal/tui       Bubble Tea model, keys and view
```
