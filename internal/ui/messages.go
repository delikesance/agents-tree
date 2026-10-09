package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

const (
	foldLines = 14 // lines of a long message shown before "… +N lines"
	chatMax   = 108
	margin    = 2
)

// rctx is everything a message depends on besides the message itself.
type rctx struct {
	st       *store.Store
	now      float64 // 0 = replay (use event time)
	frame    int
	expanded bool
	width    int // text width of the chat column
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

func agentLive(c rctx, m *model.Message) bool {
	n := agentOf(c, m)
	return n != nil && c.st.State(n, c.now) == "running"
}

// messageSig changes whenever the rendered message would change, so views re-render only then.
func messageSig(m *model.Message, c rctx) string {
	base := fmt.Sprintf("%d|%v|%d", m.Rev, c.expanded, c.width)
	switch m.Role {
	case "tool":
		if m.Status == "running" {
			if agentLive(c, m) {
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

// nest marks text produced by a subagent: indented under a thin rail in its colour.
func nest(c rctx, m *model.Message, s string) string {
	if m.AgentID == model.Main {
		return s
	}
	col := colLine
	if n := agentOf(c, m); n != nil {
		col = modelColor(n.Model, true)
	}
	bar := fg(col).Render("│ ")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = "  " + bar + l
	}
	return strings.Join(lines, "\n")
}

func textWidth(c rctx, m *model.Message) int {
	if m.AgentID != model.Main {
		return c.width - 4
	}
	return c.width
}

// renderMessage draws one chat message. compact messages (tool lines, system lines) sit tight.
func renderMessage(m *model.Message, c rctx) (out string, compact bool) {
	switch m.Role {
	case "user":
		out = renderUser(m, c)
	case "assistant":
		out = renderAssistant(m, c)
	case "tool":
		out, compact = nest(c, m, renderTool(m, c, textWidth(c, m))), true
	case "delegation":
		out = renderDelegation(m, c)
	case "report":
		out = renderReport(m, c)
	default:
		out, compact = renderSystem(m, c), true
	}
	return out, compact
}

func header(left string, ts float64) string {
	return left + fg(colFaint).Render(" · "+hhmm(ts))
}

func renderUser(m *model.Message, c rctx) string {
	body, hidden := fold(m.Text, c.expanded, foldLines)
	bar := fg(colUser).Render("▌ ")
	lines := []string{bar + header(bold(colUser).Render("you"), m.TS)}
	for _, l := range strings.Split(wrap(body, c.width-2), "\n") {
		lines = append(lines, bar+lipgloss.NewStyle().Foreground(colText).Render(l))
	}
	if hidden > 0 {
		lines = append(lines, bar+footer(hidden))
	}
	return strings.Join(lines, "\n")
}

func renderAssistant(m *model.Message, c rctx) string {
	modelName := m.Model
	if n := agentOf(c, m); modelName == "" && n != nil {
		modelName = n.Model
	}
	col := modelColor(modelName, false)
	body, hidden := fold(m.Text, c.expanded, foldLines)
	w := textWidth(c, m)
	head := bold(col).Render(agentLabel(c, m))
	if modelName != "" {
		head += fg(colDim).Render(" · " + shortModel(modelName))
	}
	text := renderMarkdown(body, w)
	if hidden > 0 {
		text += "\n" + footer(hidden)
	}
	return nest(c, m, clip(header(head, m.TS), w)+"\n"+text)
}

func toolIcon(m *model.Message, live bool, frame int) string {
	switch {
	case m.Status == "error":
		return bold(colRed).Render("✗")
	case m.Status == "ok":
		return fg(colGreen).Render("✓")
	case live:
		return bold(colGreen).Render(spin(frame))
	}
	return fg(colFaint).Render("…") // never answered: interrupted or still unknown
}

func renderTool(m *model.Message, c rctx, width int) string {
	dur := fmtSecs(m.Duration)
	prefix := fg(colFaint).Render("┃ ") + toolIcon(m, agentLive(c, m), c.frame) + " "
	name := clip(m.Tool, max(width-lipgloss.Width(prefix)-lipgloss.Width(dur)-2, 4))
	left := prefix + lipgloss.NewStyle().Bold(true).Foreground(colText).Render(name) + " "
	room := width - lipgloss.Width(left) - lipgloss.Width(dur) - 1
	detail := fg(colGrey).Render(clip(m.Detail, max(room, 0)))
	gap := max(width-lipgloss.Width(left)-lipgloss.Width(detail)-lipgloss.Width(dur), 1)
	return left + detail + strings.Repeat(" ", gap) + fg(colFaint).Render(dur)
}

// renderToolGroup draws consecutive tool calls: up to two as lines, more as one summary line (with the
// failed and the running ones still visible). Expanded shows every call.
func renderToolGroup(msgs []*model.Message, c rctx) string {
	first := msgs[0]
	w := textWidth(c, first)
	if c.expanded || len(msgs) <= 2 {
		lines := make([]string, len(msgs))
		for i, m := range msgs {
			lines[i] = renderTool(m, c, w)
		}
		return nest(c, first, strings.Join(lines, "\n"))
	}
	var order []string
	counts := map[string]int{}
	ok, bad, running := 0, 0, 0
	total := 0.0
	var lines []string
	for _, m := range msgs {
		if counts[m.Tool] == 0 {
			order = append(order, m.Tool)
		}
		counts[m.Tool]++
		switch m.Status {
		case "ok":
			ok++
		case "error":
			bad++
		default:
			running++
		}
		if m.Duration != nil {
			total += *m.Duration
		}
	}
	var names []string
	for _, n := range order {
		if counts[n] > 1 {
			names = append(names, fmt.Sprintf("%s ×%d", n, counts[n]))
		} else {
			names = append(names, n)
		}
	}
	left := fg(colFaint).Render("┃ ") + fg(colDim).Render(fmt.Sprintf("⚙ %d tool calls", len(msgs))) + fg(colFaint).Render(" · "+strings.Join(names, " · "))
	right := ""
	if ok > 0 {
		right += fg(colGreen).Render(fmt.Sprintf("✓ %d", ok)) + " "
	}
	if bad > 0 {
		right += bold(colRed).Render(fmt.Sprintf("✗ %d", bad)) + " "
	}
	if total > 0 {
		t := total
		right += fg(colFaint).Render(fmtSecs(&t))
	}
	right = strings.TrimRight(right, " ")
	left = clip(left, max(w-lipgloss.Width(right)-2, 10))
	lines = append(lines, left+strings.Repeat(" ", max(w-lipgloss.Width(left)-lipgloss.Width(right), 1))+right)
	shown := 0
	for _, m := range msgs {
		if (m.Status == "error" || m.Status == "running") && shown < 3 {
			lines = append(lines, renderTool(m, c, w))
			shown++
		}
	}
	_ = running
	return nest(c, first, strings.Join(lines, "\n"))
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

func boxTitle(label string, col lipgloss.Style, modelName string, ts float64) string {
	t := col.Render(label)
	if modelName != "" {
		t += fg(colGrey).Render(" · " + shortModel(modelName))
	}
	return t + fg(colFaint).Render(" · "+hhmm(ts))
}

func renderDelegation(m *model.Message, c rctx) string {
	modelName := m.Model
	if n := c.st.Get(m.Target); n != nil && n.Model != "" {
		modelName = n.Model
	}
	col := modelColor(modelName, false)
	body, hidden := fold(m.Text, c.expanded, 4)
	var parts []string
	if m.Desc != "" {
		parts = append(parts, lipgloss.NewStyle().Foreground(colText).Bold(true).Render(m.Desc))
	}
	parts = append(parts, fg(colGrey).Render(body))
	if hidden > 0 {
		parts = append(parts, footer(hidden))
	}
	parts = append(parts, chip(targetState(c, m), m.Duration, c.frame))
	kind := m.Kind
	if kind == "" {
		kind = "agent"
	}
	return box(boxTitle("→ "+kind, bold(col), modelName, m.TS), strings.Join(parts, "\n"), c.width, col)
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
	s := fg(colFaint).Render("⚙ " + clip(lines[0], max(c.width-12, 20)))
	if len(lines) > 1 {
		s += fg(colFaint).Faint(true).Render(fmt.Sprintf("  · %d lines", len(lines)))
	}
	return clip(s, c.width)
}

// ---- flow: grouping of messages into what is drawn -----------------------------------------------------

// flowItem is one drawn unit: a message, or a run of tool calls by the same agent.
type flowItem struct {
	id   string
	msgs []*model.Message
}

func isInterrupt(m *model.Message) bool { return strings.HasPrefix(m.Text, "[Request interrupted") }

// buildFlow groups consecutive tool calls of one agent, and hides system noise (hook feedback,
// injected prompts) unless expanded; hidden counts the system messages left out.
func buildFlow(vis []*model.Message, expanded bool) (items []flowItem, hidden int) {
	for _, m := range vis {
		if m.Role == "system" && !expanded && !isInterrupt(m) {
			hidden++
			continue
		}
		if m.Role == "tool" && len(items) > 0 {
			last := &items[len(items)-1]
			if last.msgs[0].Role == "tool" && last.msgs[0].AgentID == m.AgentID {
				last.msgs = append(last.msgs, m)
				continue
			}
		}
		id := m.ID
		if m.Role == "tool" {
			id = "tools:" + m.ID
		}
		items = append(items, flowItem{id: id, msgs: []*model.Message{m}})
	}
	return
}

func (it flowItem) sig(c rctx) string {
	parts := make([]string, len(it.msgs))
	for i, m := range it.msgs {
		parts[i] = messageSig(m, c)
	}
	return strings.Join(parts, ";")
}

func (it flowItem) render(c rctx) (string, bool) {
	if it.msgs[0].Role == "tool" {
		return renderToolGroup(it.msgs, c), true
	}
	return renderMessage(it.msgs[0], c)
}
