// Package ui is the Bubble Tea interface: chat, agent rail, tree view, session picker.
package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/delikesance/agents-tree/internal/pricing"
)

// Palette: the model family gives the colour (orange Sonnet, green Haiku, purple Opus), you are blue.
var (
	colOrange = lipgloss.Color("#ff8a1f")
	colGreen  = lipgloss.Color("#2fd27c")
	colPurple = lipgloss.Color("#a98bff")
	colUser   = lipgloss.Color("#7cb7ff")
	colRed    = lipgloss.Color("#ff6161")
	colText   = lipgloss.Color("#d9dce2")
	colDim    = lipgloss.Color("#8a909c")
	colFaint  = lipgloss.Color("#5b616c")
	colLine   = lipgloss.Color("#2a2d33")
	colCode   = lipgloss.Color("#ffd9a8")
	colGrey   = lipgloss.Color("#9aa1ad")

	famColor = map[string]color.Color{"opus": colPurple, "sonnet": colOrange, "haiku": colGreen, "": colGrey}
	famDim   = map[string]color.Color{"opus": lipgloss.Color("#6a5a9a"), "sonnet": lipgloss.Color("#8a5a1a"),
		"haiku": lipgloss.Color("#2f7a56"), "": lipgloss.Color("#6b707a")}
)

const spinner = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

func spin(frame int) string {
	r := []rune(spinner)
	return string(r[frame%len(r)])
}

func fg(c color.Color) lipgloss.Style   { return lipgloss.NewStyle().Foreground(c) }
func bold(c color.Color) lipgloss.Style { return lipgloss.NewStyle().Foreground(c).Bold(true) }

func modelColor(model string, dim bool) color.Color {
	if dim {
		return famDim[pricing.Family(model)]
	}
	return famColor[pricing.Family(model)]
}

// shortModel renders "claude-sonnet-5-5" as "Sonnet 5.5".
func shortModel(model string) string {
	fam := pricing.Family(model)
	if fam == "" {
		return model
	}
	key := fam
	if strings.Contains(strings.ToLower(model), "fable") {
		key = "fable"
	} else if strings.Contains(strings.ToLower(model), "mythos") {
		key = "mythos"
	}
	rest := model[strings.Index(strings.ToLower(model), key)+len(key):]
	var digits []string
	for _, p := range strings.FieldsFunc(rest, func(r rune) bool { return r < '0' || r > '9' }) {
		digits = append(digits, p)
		if len(digits) == 3 && len(digits[len(digits)-1]) >= 8 {
			digits = digits[:len(digits)-1] // dated suffix
		}
	}
	if len(digits) > 2 {
		digits = digits[:2]
	}
	name := strings.ToUpper(key[:1]) + key[1:]
	if len(digits) == 0 {
		return name
	}
	return name + " " + strings.Join(digits, ".")
}

func fmtTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

func fmtAge(secs float64) string {
	if secs >= 3600 {
		return fmt.Sprintf("%dh", int(secs/3600))
	}
	return fmt.Sprintf("%dm", int(secs/60))
}

func fmtSecs(d *float64) string {
	if d == nil {
		return ""
	}
	if *d < 60 {
		return fmt.Sprintf("%.1f s", *d)
	}
	return fmt.Sprintf("%d m %02d s", int(*d)/60, int(*d)%60)
}

func hhmm(ts float64) string {
	if ts == 0 {
		return "--:--"
	}
	return time.Unix(int64(ts), 0).Format("15:04")
}

func usd(v float64) string { return fmt.Sprintf("~$%s", comma(v)) }

func comma(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	dot := strings.Index(s, ".")
	intp, frac := s[:dot], s[dot:]
	var out []byte
	for i, c := range []byte(intp) {
		if i > 0 && (len(intp)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out) + frac
}

// ---- text helpers -------------------------------------------------------------------------------------

func clip(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return ansi.Truncate(s, w, "…")
}

func padRight(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func padLeft(s string, w int) string {
	if d := w - lipgloss.Width(s); d > 0 {
		return strings.Repeat(" ", d) + s
	}
	return s
}

func wrap(s string, w int) string {
	if w < 1 {
		w = 1
	}
	return lipgloss.NewStyle().Width(w).Render(s)
}

// box draws a rounded box with a title on its top border. width is the total width.
func box(title string, body string, width int, border color.Color) string {
	if width < 6 {
		width = 6
	}
	inner := width - 4
	b := fg(border)
	var top string
	if title != "" {
		t := clip(title, width-6)
		top = b.Render("╭─") + " " + t + " " + b.Render(strings.Repeat("─", max(0, width-6-lipgloss.Width(t)))+"╮")
	} else {
		top = b.Render("╭" + strings.Repeat("─", width-2) + "╮")
	}
	lines := strings.Split(wrap(body, inner), "\n")
	var sb strings.Builder
	sb.WriteString(top + "\n")
	for _, l := range lines {
		sb.WriteString(b.Render("│") + " " + padRight(clip(l, inner), inner) + " " + b.Render("│") + "\n")
	}
	sb.WriteString(b.Render("╰" + strings.Repeat("─", width-2) + "╯"))
	return sb.String()
}

// indent prefixes every line with n spaces.
func indent(s string, n int) string {
	if n <= 0 {
		return s
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}

func barOf(v float64, width int, ok bool) string {
	if !ok {
		return fg(colLine).Render(strings.Repeat("░", width))
	}
	if v > 1 {
		v = 1
	}
	n := int(v*float64(width) + 0.5)
	c := colGreen
	if v < 0.7 {
		c = colOrange
	}
	return fg(c).Render(strings.Repeat("█", n)) + fg(colLine).Render(strings.Repeat("░", width-n))
}
