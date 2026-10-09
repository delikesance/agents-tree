package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

// agentStatus is the one-line state of an agent: what it is doing now, or how it ended.
func agentStatus(st *store.Store, n *model.AgentNode, now float64, frame int) (string, lipgloss.Style) {
	switch state := st.State(n, now); state {
	case "stale":
		ref := now
		if ref == 0 {
			ref = st.Clock
		}
		last := n.LastActive
		if last == 0 {
			last = n.Started
		}
		word := "◌ no activity "
		if n.ID == model.Main {
			word = "◌ idle "
		}
		return word + fmtAge(max(0, ref-last)), fg(colFaint)
	case "failed":
		return "✗ failed", bold(colRed)
	case "done":
		return "✓ done", fg(colGrey)
	}
	act := n.Activity
	if act == "" {
		act = "working"
		if n.ID == model.Main {
			act = "main session"
		}
	}
	return spin(frame) + " " + act, bold(colGreen)
}

// agentDiagram draws the agents as a vertical tree, one airy block per agent:
//
//	● main · Sonnet 5.5                  ⠋ Bash
//	│ 261 turns · ~$27.88 · cache 99%
//	│
//	├─ ● worker · Sonnet 5.5             ⠴ Edit
//	│  │ Implement 2.1 tunnel stats
//	│  │ 63 turns · ~$1.62 · cache 96%
//
// Only running agents (and the ancestors that connect them) unless showAll.
func agentDiagram(st *store.Store, now float64, frame int, showAll bool, width int) string {
	var out []string
	var walk func(id, prefix string, last, root bool)
	walk = func(id, prefix string, last, root bool) {
		n := st.Nodes[id]
		state := st.State(n, now)
		col := modelColor(n.Model, state != "running")
		if state == "failed" {
			col = colRed
		}
		branch, cont := "", ""
		if !root {
			branch, cont = "├─ ", "│  "
			if last {
				branch, cont = "└─ ", "   "
			}
		}
		lineFg := fg(colLine)
		name := bold(col).Render("● " + n.Kind)
		if m := shortModel(n.Model); m != "" {
			name += fg(colDim).Render(" · " + m)
		}
		status, stSt := agentStatus(st, n, now, frame)
		right := stSt.Render(status)
		left := prefix + lineFg.Render(branch) + name
		gap := max(width-lipgloss.Width(left)-lipgloss.Width(right), 2)
		out = append(out, clip(left+strings.Repeat(" ", gap)+right, width))
		body := prefix + lineFg.Render(cont+"│ ")
		if root {
			body = prefix + lineFg.Render("│ ")
		}
		if n.Desc != "" {
			out = append(out, clip(body+fg(colText).Render(n.Desc), width))
		}
		price := "$ n/a"
		if n.CostParts != nil {
			price = usd(n.Cost)
		}
		stats := fmt.Sprintf("%d turns · %s", n.Turns, price)
		if hr, ok := store.HitRate(n.Fresh, n.CacheWrite, n.CacheRead); ok {
			stats += fmt.Sprintf(" · cache %.0f%%", hr*100)
		}
		out = append(out, clip(body+fg(colFaint).Render(stats), width))
		kids := st.VisibleChildren(id, now, showAll)
		next := prefix
		if !root {
			next = prefix + lineFg.Render(cont)
		}
		out = append(out, strings.TrimRight(next+lineFg.Render("│"), " "))
		for i, c := range kids {
			walk(c, next, i == len(kids)-1, false)
		}
	}
	walk(model.Main, "", true, true)
	// the trailing connector line of the last block is just air
	for len(out) > 0 && strings.TrimSpace(ansiStrip(out[len(out)-1])) == "│" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// sidePanel is the right-hand column of wide terminals: the live agent diagram, the cost in one glance and the
// latest events. Calm on purpose: headings, air, no boxes.
func (m *Model) sidePanel(width, height int) string {
	now := m.now()
	running, total := m.st.RunningSubagents(now)
	head := func(t string) string { return bold(colFaint).Render(t) }
	var l []string
	title := "AGENTS"
	switch {
	case running > 0:
		title += fmt.Sprintf("  ·  %d working now", running)
	case total > 0:
		title += "  ·  none working"
	}
	l = append(l, head(title), "")
	l = append(l, agentDiagram(m.st, now, m.frame, m.showAll, width))
	if !m.showAll && total > running {
		l = append(l, "", fg(colFaint).Render(fmt.Sprintf("%d finished or idle hidden · d, then h", total-running)))
	}
	if sm := m.st.Summary(); sm.Turns > 0 {
		l = append(l, "", "", head("COST"), "")
		c := sm.Cost
		line := lipgloss.NewStyle().Bold(true).Foreground(colText).Render(usd(c.Total()))
		if hr, ok := sm.HitRate(); ok {
			line += fg(colDim).Render(fmt.Sprintf("   cache hit %.0f%%", hr*100))
		}
		l = append(l, line)
		if c.Saved() >= 0.005 {
			l = append(l, fg(colGreen).Render("saved "+usd(c.Saved()))+fg(colFaint).Render(" vs no cache"))
		}
	}
	if n := len(m.st.Log); n > 0 {
		l = append(l, "", "", head("ACTIVITY"), "")
		for _, e := range m.st.Log[max(0, n-8):] {
			col := colGrey
			if strings.HasPrefix(e.Text, "+") {
				col = colGreen
			}
			l = append(l, clip(fg(colFaint).Render(hhmm(e.TS)+"  ")+fg(col).Render(e.Text), width))
		}
	}
	return fitHeight(strings.Join(l, "\n"), height)
}
