package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

const (
	foldLines  = 6
	nestIndent = 4
)

// rctx is everything a message box depends on besides the message itself.
type rctx struct {
	st       *store.Store
	now      float64 // 0 = replay (use event time)
	frame    int
	expanded bool
	width    int
}

func agentOf(c rctx, m *model.Message) *model.AgentNode { return c.st.Get(m.AgentID) }

func agentLabel(c rctx, m *model.Message) string {
	if m.AgentID == model.Main {
		return "main"
	}
	if n := agentOf(c, m); n != nil {
		return n.Kind
	}
	return "agent"
}

// fold keeps the first `limit` lines unless expanded; hidden is how many lines were cut.
func fold(text string, expanded bool, limit int) (string, int) {
	lines := strings.Split(text, "\n")
	if expanded || len(lines) <= limit {
		return text, 0
	}
	return strings.Join(lines[:limit], "\n"), len(lines) - limit
}

func footer(hidden int) string {
	return fg(colFaint).Render(fmt.Sprintf("… +%d lines  (e to expand)", hidden))
}

func titled(label string, c colorStyle, modelName string, ts float64) string {
	t := c.Render(label)
	if modelName != "" {
		t += fg(colGrey).Render(" · " + shortModel(modelName))
	}
	return t + fg(colFaint).Render(" · "+hhmm(ts))
}

// colorStyle is a bold coloured style used for titles.
type colorStyle = lipgloss.Style

func targetState(c rctx, m *model.Message) string {
	if m.Target != "" {
		if n := c.st.Get(m.Target); n != nil {
			return c.st.State(n, c.now)
		}
	}
	if m.Status != "" {
		return m.Status
	}
	return "done"
}

// messageSig changes whenever the rendered box would change, so views re-render only then.
func messageSig(m *model.Message, c rctx) string {
	base := fmt.Sprintf("%d|%v|%d", m.Rev, c.expanded, c.width)
	switch m.Role {
	case "tool":
		if m.Status == "running" {
			live := false
			if n := agentOf(c, m); n != nil {
				live = c.st.State(n, c.now) == "running"
			}
			if live {
				return fmt.Sprintf("%s|spin%d", base, c.frame)
			}
			return base + "|idle"
		}
	case "delegation", "report":
		st := targetState(c, m)
		cost := 0.0
		if n := c.st.Get(m.Target); n != nil {
			cost = n.Cost
		}
		if st == "running" {
			return fmt.Sprintf("%s|%s|%.4f|spin%d", base, st, cost, c.frame)
		}
		return fmt.Sprintf("%s|%s|%.4f", base, st, cost)
	}
	return base
}

// renderMessage draws one chat message. compact messages (tool lines, system lines) sit tight.
func renderMessage(m *model.Message, c rctx) (out string, compact bool) {
	nested := m.AgentID != model.Main
	w := c.width
	if nested {
		w -= nestIndent
	}
	cc := c
	cc.width = w
	switch m.Role {
	case "user":
		out = renderUser(m, cc)
	case "assistant":
		out = renderAssistant(m, cc)
	case "tool":
		out, compact = renderTool(m, cc), true
	case "delegation":
		out = renderDelegation(m, cc)
	case "report":
		out = renderReport(m, cc)
	default:
		out, compact = renderSystem(m, cc), true
	}
	if nested {
		out = indent(out, nestIndent)
	}
	return out, compact
}

func renderUser(m *model.Message, c rctx) string {
	body, hidden := fold(m.Text, c.expanded, foldLines)
	if hidden > 0 {
		body += "\n" + footer(hidden)
	}
	return box(titled("you", bold(colUser), "", m.TS), body, c.width, colUser)
}

func renderAssistant(m *model.Message, c rctx) string {
	modelName := m.Model
	if n := agentOf(c, m); modelName == "" && n != nil {
		modelName = n.Model
	}
	col := modelColor(modelName, false)
	body, hidden := fold(m.Text, c.expanded, foldLines)
	text := renderMarkdown(body, c.width-4)
	if hidden > 0 {
		text += "\n" + footer(hidden)
	}
	return box(titled(agentLabel(c, m), bold(col), modelName, m.TS), text, c.width, col)
}

func renderTool(m *model.Message, c rctx) string {
	live := false
	if n := agentOf(c, m); n != nil {
		live = c.st.State(n, c.now) == "running"
	}
	var icon string
	switch {
	case m.Status == "error":
		icon = bold(colRed).Render("✗")
	case m.Status == "ok":
		icon = fg(colGreen).Render("✓")
	case live:
		icon = bold(colGreen).Render(spin(c.frame))
	default:
		icon = fg(colFaint).Render("…") // never answered: interrupted or still unknown
	}
	dur := fmtSecs(m.Duration)
	left := "  " + fg(colLine).Render("┃") + " " + icon + " " + lipgloss.NewStyle().Bold(true).Render(m.Tool) + " "
	room := c.width - lipgloss.Width(left) - lipgloss.Width(dur) - 1
	detail := fg(colGrey).Render(clip(m.Detail, max(room, 0)))
	gap := c.width - lipgloss.Width(left) - lipgloss.Width(detail) - lipgloss.Width(dur)
	if gap < 1 {
		gap = 1
	}
	return left + detail + strings.Repeat(" ", gap) + fg(colFaint).Render(dur)
}

func chip(state string, dur *float64, frame int) string {
	var s string
	switch state {
	case "running":
		s = bold(colGreen).Render(spin(frame) + " running")
	case "failed":
		s = bold(colRed).Render("✗ failed")
	case "stale":
		s = fg(colFaint).Render("◌ no activity")
	default:
		s = fg(colGrey).Render("✓ done")
	}
	if dur != nil {
		s += fg(colFaint).Render(" · " + fmtSecs(dur))
	}
	return s
}

func renderDelegation(m *model.Message, c rctx) string {
	modelName := m.Model
	if n := c.st.Get(m.Target); n != nil && n.Model != "" {
		modelName = n.Model
	}
	col := modelColor(modelName, false)
	body, hidden := fold(m.Text, c.expanded, 3)
	var parts []string
	if m.Desc != "" {
		parts = append(parts, fg(colGrey).Render(m.Desc))
	}
	parts = append(parts, body)
	if hidden > 0 {
		parts = append(parts, footer(hidden))
	}
	parts = append(parts, chip(targetState(c, m), m.Duration, c.frame))
	kind := m.Kind
	if kind == "" {
		kind = "agent"
	}
	return box(titled("→ "+kind, bold(col), modelName, m.TS), strings.Join(parts, "\n"), c.width, col)
}

func renderReport(m *model.Message, c rctx) string {
	n := c.st.Get(m.Target)
	failed := m.Status == "failed"
	col := modelColor("", false)
	kind := "agent"
	if n != nil {
		col, kind = modelColor(n.Model, false), n.Kind
	}
	if failed {
		col = colRed
	}
	text := m.Text
	if text == "" {
		text = "(no report)"
	}
	body, hidden := fold(text, c.expanded, foldLines)
	out := renderMarkdown(body, c.width-4)
	if hidden > 0 {
		out += "\n" + footer(hidden)
	}
	verb := "reported"
	if failed {
		verb = "failed"
	}
	t := bold(col).Render("↩ " + kind + " " + verb)
	if m.Duration != nil {
		t += fg(colGrey).Render(" · " + fmtSecs(m.Duration))
	}
	if n != nil && n.CostParts != nil {
		t += fg(colGrey).Render(" · " + usd(n.Cost))
	}
	t += fg(colFaint).Render(" · " + hhmm(m.TS))
	return box(t, out, c.width, col)
}

func renderSystem(m *model.Message, c rctx) string {
	if c.expanded {
		body, _ := fold(m.Text, true, 0)
		return box(fg(colFaint).Render("⚙ system · "+hhmm(m.TS)), fg(colGrey).Render(body), c.width, colLine)
	}
	lines := strings.Split(m.Text, "\n")
	first := clip(lines[0], 110)
	s := fg(colFaint).Render(" ⚙ " + first)
	if len(lines) > 1 {
		s += fg(colFaint).Faint(true).Render(fmt.Sprintf("  · %d lines ▸", len(lines)))
	}
	return clip(s, c.width)
}
