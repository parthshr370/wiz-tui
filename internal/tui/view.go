package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/parthshr370/wiz-tui/internal/wiz"
)

// Palette: the dark-hearth omp theme.
var (
	colOrange   = lipgloss.Color("#fe8019") // input border, thinking accent
	colEmber    = lipgloss.Color("#e9973f")
	colSelBg    = lipgloss.Color("#3a1a1c") // sent-message background
	colSelFg    = lipgloss.Color("#fe9a4a") // sent-message text
	colPit      = lipgloss.Color("#121110")
	colHearth   = lipgloss.Color("#181615")
	colLine     = lipgloss.Color("#2c2826")
	colLineSoft = lipgloss.Color("#211e1c")
	colFg       = lipgloss.Color("#d5c4a1")
	colAsh      = lipgloss.Color("#a89984")
	colGray     = lipgloss.Color("#928374")
	colDim      = lipgloss.Color("#6f655c")
	colGold     = lipgloss.Color("#fabd2f")
	colGreen    = lipgloss.Color("#5fc77f")
	colRed      = lipgloss.Color("#f2594b")

	styleFg     = lipgloss.NewStyle().Foreground(colFg)
	styleAsh    = lipgloss.NewStyle().Foreground(colAsh)
	styleGray   = lipgloss.NewStyle().Foreground(colGray)
	styleDim    = lipgloss.NewStyle().Foreground(colDim)
	styleGreen  = lipgloss.NewStyle().Foreground(colGreen)
	styleRed    = lipgloss.NewStyle().Foreground(colRed)
	styleOrange = lipgloss.NewStyle().Foreground(colOrange)
	styleGold   = lipgloss.NewStyle().Foreground(colGold)
)

const (
	maxContent = 104
	minContent = 70
	cardWidth  = orbCols + 4
	chipWidth  = 19
	gutter     = 3
	// roomyHeight is the terminal height the layout needs with its spacing.
	roomyHeight = 34
)

func helpStyles() help.Styles {
	styles := help.DefaultDarkStyles()
	styles.ShortKey = styleOrange
	styles.FullKey = styleOrange
	styles.ShortDesc = styleGray
	styles.FullDesc = styleGray
	styles.ShortSeparator = styleDim
	styles.FullSeparator = styleDim
	return styles
}

func clamp(value, low, high int) int { return max(low, min(high, value)) }

func contentWidth(termWidth int) int { return clamp(termWidth-4, minContent, maxContent) }

func gridWidth(termWidth int) int { return contentWidth(termWidth) - cardWidth - gutter }

func gridColumns(termWidth int) int { return max(1, gridWidth(termWidth)/chipWidth) }

func hex(c rgb) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }

func colorOf(c rgb) lipgloss.Style { return lipgloss.NewStyle().Foreground(lipgloss.Color(hex(c))) }

// readable lifts very dark colours so text drawn in them stays legible.
func readable(c rgb) rgb {
	if c.luma() < 90 {
		return mix(c, rgb{0xd5, 0xc4, 0xa1}, 0.45)
	}
	return c
}

func pill(text string, fg, bg color.Color) string {
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Bold(true).Padding(0, 1).Render(text)
}

func (m Model) View() tea.View {
	width := contentWidth(m.width)
	parts := []string{m.viewTopBar(width), ""}
	if _, ok := m.bulb(); ok && m.online {
		parts = append(parts, m.viewTabs(width), "")
		body := lipgloss.JoinHorizontal(lipgloss.Top,
			m.viewCard(),
			strings.Repeat(" ", gutter),
			m.viewBrowser(gridWidth(m.width)))
		parts = append(parts, body)
	} else {
		parts = append(parts, m.viewWaiting(width))
	}
	parts = append(parts, "", m.viewToast(width))
	if m.inputKind != inputNone {
		parts = append(parts, m.input.View())
	}
	parts = append(parts, m.help.View(m.keys))

	content := lipgloss.NewStyle().Padding(1, 2).Render(lipgloss.JoinVertical(lipgloss.Left, parts...))
	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
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
	var out strings.Builder
	for i, glyph := range []string{"▂", "▄", "▆", "█"} {
		if i < bars {
			out.WriteString(styleGreen.Render(glyph))
		} else {
			out.WriteString(styleDim.Render(glyph))
		}
	}
	return out.String()
}

func (m Model) viewTopBar(width int) string {
	left := pill("◉ wiz", colPit, colOrange)
	bulb, ok := m.bulb()
	if ok {
		for i, each := range m.bulbs {
			name := m.bulbName(each)
			if i == m.current {
				left += "  " + styleFg.Bold(true).Render(name)
			} else {
				left += "  " + styleDim.Render(name)
			}
		}
	}
	var right string
	switch {
	case !ok:
		right = styleDim.Render("no light yet")
	case !m.online:
		right = styleRed.Render("○ offline") + styleDim.Render("  "+bulb.IP)
	default:
		right = signalBars(m.state.RSSI) + styleDim.Render(fmt.Sprintf(" %d dBm · %s · fw %s", m.state.RSSI, bulb.IP, bulb.Firmware))
		if len(m.client.SigningKey) > 0 {
			right += styleGreen.Render(" · signed")
		}
	}
	gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(right))
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) viewWaiting(width int) string {
	orb := renderOrb(rgb{}, false, 1)
	caption := "looking for your lights…"
	if !m.loading {
		caption = "press d to scan the network again"
	}
	card := lipgloss.NewStyle().Background(colHearth).Padding(1, 2).Render(orb + "\n\n" + styleGray.Background(colHearth).Width(orbCols).Align(lipgloss.Center).Render(caption))
	return lipgloss.PlaceHorizontal(width, lipgloss.Center, card)
}

func (m Model) describe(it item) string {
	switch it.kind {
	case kindWhite:
		return fmt.Sprintf("white · %d K", it.kelvin)
	case kindScene:
		if it.scene.ID == dimToWarmScene {
			return fmt.Sprintf("warmer as it dims · %d K", dimToWarmKelvin(m.dim))
		}
		return it.scene.About
	case kindToy:
		return it.toy.About
	default:
		return "colour · " + hex(it.color)
	}
}

// meter is a small bar drawn on background bg.
func meter(value, low, high, width int, fill rgb, bg color.Color) string {
	filled := int(float64(value-low) / float64(high-low) * float64(width))
	return colorOf(fill).Background(bg).Render(strings.Repeat("━", filled)) +
		styleDim.Background(bg).Render(strings.Repeat("─", width-filled))
}

// liveSummary describes what the light is doing right now, for the card.
func (m Model) liveSummary() string {
	dot := func(c rgb, text string) string {
		return colorOf(c).Background(colHearth).Render("●") + styleAsh.Background(colHearth).Render(" "+text)
	}
	if m.play.active() {
		return dot(m.toyColor(), m.play.spec.Name)
	}
	if !m.state.On {
		return styleDim.Background(colHearth).Render("○ off")
	}
	for s := range sectionCustom {
		for _, each := range m.sections[s] {
			if each.matches(m.state) {
				return dot(each.swatch(), fmt.Sprintf("%s %d%%", each.name, m.state.Dimming))
			}
		}
	}
	r, g, b := m.state.RGB()
	live := rgb{r, g, b}
	if m.state.Mode() == wiz.ModeWhite {
		r, g, b = wiz.KelvinRGB(m.state.Temp)
		live = rgb{r, g, b}
	}
	return dot(live, fmt.Sprintf("%s %d%%", hex(live), m.state.Dimming))
}

// viewBrightness is the card's brightness row, or a note for the scenes that
// set their own.
func (m Model) viewBrightness(selected item, swatch rgb, inner int) string {
	bg := lipgloss.NewStyle().Background(colHearth).Width(inner)
	dim := m.dim
	switch {
	case m.play.active():
		dim = max(wiz.MinDimming, m.play.frame.Change.Dimming)
	case selected.kind == kindToy:
		return bg.Render(styleDim.Background(colHearth).Render("◉ the toy sets brightness"))
	case selected.kind == kindScene && !selected.scene.Dimming:
		return bg.Render(styleDim.Background(colHearth).Render("◉ fixed brightness"))
	}
	return bg.Render(styleOrange.Background(colHearth).Render("◉ ") +
		meter(dim, 0, wiz.MaxDimming, inner-8, swatch, colHearth) +
		styleFg.Background(colHearth).Render(fmt.Sprintf(" %3d%%", dim)))
}

// viewSleep is the card's sleep-timer row: empty until t arms it.
func (m Model) viewSleep() string {
	if m.sleep.at.IsZero() {
		return ""
	}
	left := time.Until(m.sleep.at).Round(time.Minute)
	text := fmt.Sprintf("zZ off in %d min", int(left.Minutes()))
	if left < time.Minute {
		text = "zZ off in under a minute"
	}
	return styleGold.Background(colHearth).Render(text) + styleDim.Background(colHearth).Render(" · t")
}

func (m Model) viewCard() string {
	selected := m.selected()
	swatch := selected.swatch()
	name, about := selected.name, m.describe(selected)
	if m.play.active() {
		swatch, name, about = toyColour[m.play.spec.ID], m.play.spec.Name, m.play.frame.Status
	}
	inner := orbCols
	bg := lipgloss.NewStyle().Background(colHearth)
	line := func(text string) string { return bg.Width(inner).Render(text) }

	var status string
	switch {
	case m.play.active():
		status = pill("any key stops it", colPit, colGold)
	case m.busy:
		status = pill("sending…", colPit, colEmber)
	case m.pending():
		status = pill("⏎ enter to apply", colSelFg, colSelBg)
	default:
		status = pill("✓ live", colPit, colGreen)
	}
	spacer := line("")
	if m.compact() {
		spacer = ""
	}
	rows := []string{
		renderOrb(m.shownColor(), m.lit(), m.glow()),
		spacer,
		line(colorOf(readable(swatch)).Background(colHearth).Bold(true).Render(name)),
		line(styleGray.Background(colHearth).Render(ansi.Truncate(about, inner, "…"))),
		spacer,
		m.viewBrightness(selected, swatch, inner),
		line(styleDim.Background(colHearth).Render("now ") + m.liveSummary()),
	}
	if sleep := m.viewSleep(); sleep != "" || !m.compact() {
		rows = append(rows, line(sleep))
	}
	rows = append(rows, spacer, line(status))
	rows = slices.DeleteFunc(rows, func(row string) bool { return row == "" })
	return lipgloss.NewStyle().Background(colHearth).Padding(1, 2).Render(strings.Join(rows, "\n"))
}

// compact is true on a short terminal: the card drops its spacer rows so the
// whole screen still fits.
func (m Model) compact() bool { return m.height > 0 && m.height < roomyHeight }

// viewTabs lists the tabs, with their sizes when they all fit on one line.
func (m Model) viewTabs(width int) string {
	if row := m.tabRow(true); lipgloss.Width(row) <= width {
		return row
	}
	return m.tabRow(false)
}

func (m Model) tabRow(counts bool) string {
	var tabs []string
	for s := range sectionCount {
		label := s.String()
		if count := len(m.sections[s]); counts && count > 0 {
			label += fmt.Sprintf(" %d", count)
		}
		if s == m.section {
			tabs = append(tabs, pill(label, colSelFg, colSelBg))
		} else {
			tabs = append(tabs, styleGray.Padding(0, 1).Render(label))
		}
	}
	return strings.Join(tabs, " ")
}

func (m Model) viewBrowser(width int) string {
	var parts []string
	if m.section == sectionCustom {
		parts = append(parts, m.viewCustom(width))
	} else {
		parts = append(parts, m.viewGrid())
	}
	if selected := m.selected(); selected.kind == kindScene {
		switch {
		case selected.scene.Speed:
			parts = append(parts, "", styleGray.Render("speed ")+
				meter(m.speed, wiz.MinSpeed, wiz.MaxSpeed, 20, selected.color, lipgloss.NoColor{})+
				styleFg.Render(fmt.Sprintf(" %d%%", m.speed))+styleDim.Render("   , slower  . faster"))
		case selected.scene.Kind == wiz.SceneProgressive:
			parts = append(parts, "", styleDim.Render("changes over many minutes · its pace is set in the WiZ app"))
		}
	}
	var hints []string
	if m.section == sectionFavourites {
		hints = append(hints, "f  on a preset in any tab pins it here; f here unpins it")
	}
	if m.section == sectionToys {
		hints = append(hints, "enter starts a toy; any key stops it and puts the light back")
	}
	if (m.section == sectionFavourites || m.section == sectionMoods) && m.sleep.at.IsZero() {
		hints = append(hints, "t  sleep timer: dims the light bit by bit, then turns it off")
	}
	if len(hints) > 0 {
		parts = append(parts, "", styleDim.Render(strings.Join(hints, "\n")))
	}
	return lipgloss.NewStyle().Width(width).Render(strings.Join(parts, "\n"))
}

func (m Model) viewGrid() string {
	items := m.sections[m.section]
	cursor := m.cursor[m.section]
	columns := max(1, m.columns)
	var rows []string
	for start := 0; start < len(items); start += columns {
		var cells []string
		for i := start; i < min(start+columns, len(items)); i++ {
			cells = append(cells, m.viewChip(items[i], i == cursor))
		}
		rows = append(rows, strings.Join(cells, ""))
	}
	return strings.Join(rows, "\n")
}

func (m Model) viewChip(it item, focused bool) string {
	isLive := m.state.On && it.matches(m.state)
	name := ansi.Truncate(it.name, chipWidth-6, "…")
	nameWidth := chipWidth - 6
	marker := " "
	switch {
	case isLive:
		marker = "◉"
	case m.section != sectionFavourites && m.isFavourite(it):
		marker = "◆"
	}
	base := lipgloss.NewStyle()
	nameStyle := styleAsh
	edge := " "
	if focused {
		base = base.Background(colSelBg)
		nameStyle = lipgloss.NewStyle().Foreground(colSelFg).Bold(true)
		edge = "▌"
	}
	if isLive && !focused {
		nameStyle = lipgloss.NewStyle().Foreground(colGold)
	}
	edgeStyle := base.Foreground(colOrange)
	markStyle := base.Foreground(colGold)
	chip := edgeStyle.Render(edge) +
		colorOf(it.swatch()).Inherit(base).Render("██") +
		base.Render(" ") +
		nameStyle.Inherit(base).Width(nameWidth).Render(name) +
		markStyle.Render(marker)
	return lipgloss.NewStyle().Width(chipWidth).Render(chip)
}

// slider draws a track painted by paint(f) for f in 0..1 with a knob at f0.
func slider(position float64, width int, focused bool, paint func(f float64) rgb) string {
	knob := int(position * float64(width-1))
	var out strings.Builder
	for cell := range width {
		f := float64(cell) / float64(width-1)
		if cell == knob {
			glyph, style := "◆", styleFg
			if focused {
				glyph, style = "●", lipgloss.NewStyle().Foreground(colSelFg).Bold(true)
			}
			out.WriteString(style.Render(glyph))
			continue
		}
		out.WriteString(colorOf(paint(f)).Render("━"))
	}
	return out.String()
}

func (m Model) viewCustom(width int) string {
	barWidth := max(10, width-18)
	current := hsvToRGB(m.hue, m.sat)
	row := func(r customRow, label, value string, bar string) string {
		edge, labelStyle := "  ", styleGray
		if m.custom == r {
			edge, labelStyle = styleOrange.Render("▌ "), lipgloss.NewStyle().Foreground(colSelFg).Bold(true)
		}
		return edge + labelStyle.Width(6).Render(label) + bar + styleFg.Width(8).Align(lipgloss.Right).Render(value)
	}
	hue := row(rowHue, "hue", fmt.Sprintf("%.0f°", m.hue), slider(m.hue/360, barWidth, m.custom == rowHue, func(f float64) rgb {
		return hsvToRGB(f*360, 1)
	}))
	sat := row(rowSaturation, "sat", fmt.Sprintf("%.0f%%", m.sat*100), slider(m.sat, barWidth, m.custom == rowSaturation, func(f float64) rgb {
		return hsvToRGB(m.hue, f)
	}))
	preview := colorOf(current).Render(strings.Repeat("█", 6)) + "  " + styleFg.Render(hex(current)) +
		styleDim.Render("   ↑↓ pick a slider · ←→ move it · z random")
	return strings.Join([]string{hue, "", sat, "", preview}, "\n")
}

func (m Model) viewToast(width int) string {
	style := lipgloss.NewStyle().Width(width)
	switch {
	case m.toast == "":
		return style.Render(" ")
	case m.toastBad:
		return style.Render(styleRed.Render("✗ " + m.toast))
	case m.busy || m.loading:
		return style.Render(styleOrange.Render("◌ ") + styleGray.Render(m.toast))
	default:
		return style.Render(styleGreen.Render("✓ ") + styleGray.Render(m.toast))
	}
}
