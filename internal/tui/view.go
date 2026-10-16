package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

// Palette from the dark-hearth omp theme.
var (
	colAccent = lipgloss.Color("#fe8019")
	colFg     = lipgloss.Color("#d5c4a1")
	colMuted  = lipgloss.Color("#928374")
	colDim    = lipgloss.Color("#6f655c")
	colLine   = lipgloss.Color("#3c3836")
	colTrack  = lipgloss.Color("#2c2826")
	colRed    = lipgloss.Color("#f2594b")
	colGreen  = lipgloss.Color("#5fc77f")
	colGold   = lipgloss.Color("#fabd2f")

	accentText = lipgloss.NewStyle().Foreground(colAccent)
	fgText     = lipgloss.NewStyle().Foreground(colFg)
	mutedText  = lipgloss.NewStyle().Foreground(colMuted)
	dimText    = lipgloss.NewStyle().Foreground(colDim)
	errText    = lipgloss.NewStyle().Foreground(colRed)
	okText     = lipgloss.NewStyle().Foreground(colGreen)
)

const (
	maxPanel   = 76
	minPanel   = 46
	labelWidth = 15
	valueWidth = 11
)

func helpStyles() help.Styles {
	styles := help.DefaultDarkStyles()
	styles.ShortKey = accentText
	styles.FullKey = accentText
	styles.ShortDesc = mutedText
	styles.FullDesc = mutedText
	styles.ShortSeparator = dimText
	styles.FullSeparator = dimText
	return styles
}

func panelWidth(termWidth int) int {
	return clamp(termWidth-4, minPanel, maxPanel)
}

func hex(r, g, b int) string { return fmt.Sprintf("#%02x%02x%02x", r, g, b) }

func scale(r, g, b int, factor float64) (int, int, int) {
	return int(float64(r) * factor), int(float64(g) * factor), int(float64(b) * factor)
}

// preview is the on-screen colour of the light as the model wants it.
func (m Model) preview() (r, g, b int) {
	switch m.mode {
	case wiz.ModeColor:
		r, g, b = m.red, m.green, m.blue
	case wiz.ModeScene:
		if m.state.R != nil && m.state.SceneID == wiz.Scenes[m.sceneIdx].ID {
			r, g, b = m.state.RGB()
		} else if m.state.Temp > 0 && m.state.SceneID == wiz.Scenes[m.sceneIdx].ID {
			r, g, b = wiz.KelvinRGB(m.state.Temp)
		} else {
			r, g, b = 255, 150, 60
		}
	default:
		r, g, b = wiz.KelvinRGB(m.temp)
	}
	return scale(r, g, b, 0.35+0.65*float64(m.dim)/100)
}

func (m Model) View() tea.View {
	width := panelWidth(m.width)
	var sections []string
	sections = append(sections, m.viewHeader(width))
	if _, ok := m.bulb(); ok {
		sections = append(sections, m.viewSwatch(width), m.viewModes(width), m.viewRows(width))
		if extra := m.viewModeExtra(width); extra != "" {
			sections = append(sections, extra)
		}
	}
	sections = append(sections, m.viewStatus(width))
	if m.inputKind != inputNone {
		sections = append(sections, m.input.View())
	}
	sections = append(sections, m.help.View(m.keys))

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colLine).
		Padding(0, 1).
		Render(lipgloss.JoinVertical(lipgloss.Left, sections...))

	content := panel
	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, panel)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "wiz"
	if bulb, ok := m.bulb(); ok {
		view.WindowTitle = "wiz · " + m.bulbName(bulb)
	}
	return view
}

func signalBars(rssi int) string {
	bars := 1
	switch {
	case rssi >= -55:
		bars = 4
	case rssi >= -65:
		bars = 3
	case rssi >= -75:
		bars = 2
	}
	glyphs := []string{"▂", "▄", "▆", "█"}
	var out strings.Builder
	for i, glyph := range glyphs {
		if i < bars {
			out.WriteString(okText.Render(glyph))
		} else {
			out.WriteString(dimText.Render(glyph))
		}
	}
	return out.String()
}

func (m Model) viewHeader(width int) string {
	logo := lipgloss.NewStyle().Background(colAccent).Foreground(lipgloss.Color("#1d1a18")).Bold(true).Padding(0, 1).Render("wiz")
	bulb, ok := m.bulb()
	if !ok {
		return logo + "  " + mutedText.Render("no light selected")
	}
	var tabs []string
	for i, each := range m.bulbs {
		name := m.bulbName(each)
		if i == m.current {
			tabs = append(tabs, accentText.Bold(true).Render(name))
		} else {
			tabs = append(tabs, mutedText.Render(name))
		}
	}
	title := logo + "  " + strings.Join(tabs, dimText.Render("  ·  "))
	if len(m.bulbs) > 1 {
		title += dimText.Render(fmt.Sprintf("   %d/%d", m.current+1, len(m.bulbs)))
	}

	link := okText.Render("● online")
	if !m.online {
		link = errText.Render("○ offline")
	}
	details := []string{
		link,
		fgText.Render(bulb.IP),
		mutedText.Render("fw " + bulb.Firmware),
	}
	if m.online {
		details = append(details, signalBars(m.state.RSSI)+mutedText.Render(fmt.Sprintf(" %d dBm", m.state.RSSI)))
	}
	if len(m.client.SigningKey) > 0 {
		details = append(details, okText.Render("signed"))
	}
	info := lipgloss.NewStyle().Width(width).Render(strings.Join(details, dimText.Render(" · ")))
	return title + "\n" + info
}

func (m Model) describeLight() string {
	if !m.on {
		return "OFF"
	}
	var what string
	switch m.mode {
	case wiz.ModeColor:
		what = "Colour " + hex(m.red, m.green, m.blue)
	case wiz.ModeScene:
		what = "Scene · " + wiz.Scenes[m.sceneIdx].Name
	default:
		what = fmt.Sprintf("White · %d K", m.temp)
	}
	return fmt.Sprintf("ON  ·  %s  ·  %d%%", what, m.dim)
}

func (m Model) viewSwatch(width int) string {
	r, g, b := 24, 22, 21
	if m.on {
		r, g, b = m.preview()
	}
	text := lipgloss.Color("#f2e5bc")
	if 0.299*float64(r)+0.587*float64(g)+0.114*float64(b) > 140 {
		text = lipgloss.Color("#1d1a18")
	}
	return lipgloss.NewStyle().
		Background(lipgloss.Color(hex(r, g, b))).
		Foreground(text).
		Bold(true).
		Width(width).
		Height(3).
		Align(lipgloss.Center, lipgloss.Center).
		MarginTop(1).
		Render(m.describeLight())
}

func (m Model) viewModes(width int) string {
	var tabs []string
	for _, mode := range []wiz.Mode{wiz.ModeWhite, wiz.ModeColor, wiz.ModeScene} {
		style := lipgloss.NewStyle().Padding(0, 2)
		if mode == m.mode {
			style = style.Background(colTrack).Foreground(colAccent).Bold(true)
		} else {
			style = style.Foreground(colMuted)
		}
		tabs = append(tabs, style.Render(mode.String()))
	}
	return lipgloss.NewStyle().MarginTop(1).Width(width).Render(lipgloss.JoinHorizontal(lipgloss.Top, tabs...) + dimText.Render("   tab ⇄"))
}

// slider draws a bar whose filled cells take their colour from paint(f),
// f running 0..1 across the bar, with a knob at value.
func slider(value, low, high, width int, focused bool, paint func(f float64) (int, int, int)) string {
	if high <= low || width < 3 {
		return ""
	}
	knob := int(float64(value-low) / float64(high-low) * float64(width-1))
	var out strings.Builder
	for cell := range width {
		f := float64(cell) / float64(width-1)
		r, g, b := paint(f)
		switch {
		case cell == knob:
			glyph := "●"
			if !focused {
				glyph = "◆"
			}
			out.WriteString(lipgloss.NewStyle().Foreground(colFg).Bold(true).Render(glyph))
		case cell < knob:
			out.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(hex(r, g, b))).Render("━"))
		default:
			out.WriteString(lipgloss.NewStyle().Foreground(colTrack).Render("━"))
		}
	}
	return out.String()
}

func lerp(a, b int, f float64) int { return a + int(float64(b-a)*f) }

func (m Model) viewRows(width int) string {
	bulb, _ := m.bulb()
	barWidth := width - labelWidth - valueWidth - 2
	var lines []string
	for i, id := range rowsByMode[m.mode] {
		focused := i == m.focus
		var label, bar, value string
		switch id {
		case rowPower:
			label = "Power"
			on, off := dimText.Render("on"), dimText.Render("off")
			if m.on {
				on = okText.Bold(true).Render("● on")
			} else {
				off = errText.Bold(true).Render("● off")
			}
			bar = on + "   " + off + dimText.Render("      space toggles")
		case rowBright:
			label = "Brightness"
			r, g, b := m.preview()
			bar = slider(m.dim, max(bulb.MinDim, 1), wiz.MaxDimming, barWidth, focused, func(f float64) (int, int, int) {
				return lerp(40, r, f), lerp(36, g, f), lerp(34, b, f)
			})
			value = fmt.Sprintf("%d%%", m.dim)
		case rowTemp:
			label = "Temperature"
			bar = slider(m.temp, bulb.MinTemp, bulb.MaxTemp, barWidth, focused, func(f float64) (int, int, int) {
				return wiz.KelvinRGB(lerp(bulb.MinTemp, bulb.MaxTemp, f))
			})
			value = fmt.Sprintf("%d K", m.temp)
		case rowRed, rowGreen, rowBlue:
			channel := map[row]struct {
				name  string
				value int
				r, g  int
				b     int
			}{
				rowRed:   {"Red", m.red, 255, 0, 0},
				rowGreen: {"Green", m.green, 0, 255, 0},
				rowBlue:  {"Blue", m.blue, 0, 0, 255},
			}[id]
			label = channel.name
			bar = slider(channel.value, 0, 255, barWidth, focused, func(f float64) (int, int, int) {
				return scale(channel.r, channel.g, channel.b, 0.25+0.75*f)
			})
			value = fmt.Sprint(channel.value)
		case rowScene:
			label = "Scene"
			scene := wiz.Scenes[m.sceneIdx]
			kind := "static"
			if scene.Dynamic {
				kind = "animated"
			}
			bar = accentText.Render("‹ ") + fgText.Bold(true).Render(scene.Name) + accentText.Render(" ›") + dimText.Render("  "+kind)
			value = fmt.Sprintf("%d/%d", m.sceneIdx+1, len(wiz.Scenes))
		case rowSpeed:
			label = "Speed"
			if !wiz.Scenes[m.sceneIdx].Dynamic {
				bar = dimText.Render("only for animated scenes")
				break
			}
			bar = slider(m.speed, wiz.MinSpeed, wiz.MaxSpeed, barWidth, focused, func(f float64) (int, int, int) {
				return lerp(90, 254, f), lerp(60, 128, f), lerp(30, 25, f)
			})
			value = fmt.Sprintf("%d%%", m.speed)
		}
		labelStyle := mutedText
		marker := "  "
		if focused {
			labelStyle = accentText.Bold(true)
			marker = accentText.Render("› ")
		}
		lines = append(lines, marker+
			labelStyle.Width(labelWidth-2).Render(label)+
			lipgloss.NewStyle().Width(barWidth).MaxWidth(barWidth).Render(bar)+
			lipgloss.NewStyle().Width(valueWidth).Align(lipgloss.Right).Foreground(colFg).Render(value))
	}
	return lipgloss.NewStyle().MarginTop(1).Render(strings.Join(lines, "\n"))
}

func (m Model) viewModeExtra(width int) string {
	switch m.mode {
	case wiz.ModeColor:
		var chips []string
		for i, preset := range wiz.Presets {
			swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(hex(preset.R, preset.G, preset.B))).Render("■")
			chips = append(chips, dimText.Render(fmt.Sprint(i+1))+" "+swatch)
		}
		line := strings.Join(chips, "  ") + dimText.Render("   # hex")
		return lipgloss.NewStyle().MarginTop(1).Width(width).Render(line)
	case wiz.ModeScene:
		return m.viewSceneGrid(width)
	}
	return lipgloss.NewStyle().MarginTop(1).Render(dimText.Render("w warm   c cool"))
}

func (m Model) viewSceneGrid(width int) string {
	const columns = 4
	cellWidth := width / columns
	var rows []string
	for start := 0; start < len(wiz.Scenes); start += columns {
		var cells []string
		for i := start; i < min(start+columns, len(wiz.Scenes)); i++ {
			scene := wiz.Scenes[i]
			name := scene.Name
			if scene.Dynamic {
				name += " ∿"
			}
			style := lipgloss.NewStyle().Width(cellWidth).MaxWidth(cellWidth).Foreground(colMuted)
			switch {
			case i == m.sceneIdx:
				style = style.Foreground(colAccent).Bold(true)
				name = "› " + name
			case m.online && m.state.SceneID == scene.ID:
				style = style.Foreground(colGold)
				name = "• " + name
			default:
				name = "  " + name
			}
			cells = append(cells, style.Render(name))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, cells...))
	}
	return lipgloss.NewStyle().MarginTop(1).Render(strings.Join(rows, "\n"))
}

func (m Model) viewStatus(width int) string {
	style := lipgloss.NewStyle().Width(width).MarginTop(1)
	switch {
	case m.err != "":
		return style.Render(errText.Render("✗ " + m.err))
	case m.loading || m.busy > 0:
		note := m.note
		if m.busy > 0 {
			note = "sending…"
		}
		return style.Render(m.spinner.View() + " " + mutedText.Render(note))
	case m.note != "":
		return style.Render(okText.Render("✓ ") + mutedText.Render(m.note))
	}
	return style.Render(" ")
}
