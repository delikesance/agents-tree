package ui

import (
	"fmt"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/composition"
	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/pricing"
	"github.com/delikesance/agents-tree/internal/store"
)

func oneLine(s string, w int, st lipgloss.Style) string { return st.Render(clip(s, w)) }

// card is one agent in the rail.
func card(st *store.Store, id string, now float64, frame, width int) string {
	n := st.Nodes[id]
	state := st.State(n, now)
	fam := pricing.Family(n.Model)
	running := state == "running"
	border := famDim[fam]
	if running {
		border = famColor[fam]
	}
	if state == "failed" {
		border = colRed
	}
	inner := width - 4
	head := shortModel(n.Model)
	if head == "" {
		head = "model n/a"
	}
	if n.Effort != "" {
		head += " · " + n.Effort
	}
	headSt := fg(border)
	if running {
		headSt = bold(border)
	}
	var status string
	var statusSt lipgloss.Style
	switch state {
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
		if id == model.Main {
			word = "◌ idle "
		}
		status, statusSt = word+fmtAge(max(0, ref-last)), fg(colFaint)
	case "failed":
		status, statusSt = "✗ failed", bold(colRed)
	case "done":
		status, statusSt = "✓ done", fg(colGrey)
	default:
		act := n.Activity
		if act == "" {
			act = "working"
			if id == model.Main {
				act = "main session"
			}
		}
		status, statusSt = spin(frame)+" "+act, bold(colGreen)
	}
	price := "$ n/a"
	if n.CostParts != nil {
		price = usd(n.Cost)
	}
	stats := fmt.Sprintf("%d turns · %s", n.Turns, price)
	if hr, ok := store.HitRate(n.Fresh, n.CacheWrite, n.CacheRead); ok {
		stats += fmt.Sprintf(" · %.0f%%", hr*100)
	}
	rows := []string{oneLine(head, inner, headSt)}
	if n.Desc != "" {
		rows = append(rows, oneLine(n.Desc, inner, fg(colDim)))
	}
	rows = append(rows, oneLine(status, inner, statusSt), oneLine(stats, inner, fg(colFaint)))
	return box(bold(border).Render(n.Kind), strings.Join(rows, "\n"), width, border)
}

// railAgents lists main and its agents (running ones only unless showAll).
func railAgents(st *store.Store, now float64, frame int, showAll bool, width int) string {
	var cards []string
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		d := min(depth, 3) * 2
		cards = append(cards, indent(card(st, id, now, frame, width-d), d))
		for _, c := range st.VisibleChildren(id, now, showAll) {
			walk(c, depth+1)
		}
	}
	walk(model.Main, 0)
	running, total := st.RunningSubagents(now)
	out := []string{fg(colFaint).Bold(true).Render(fmt.Sprintf("AGENTS · %d RUNNING", running))}
	out = append(out, cards...)
	shown := 0
	for _, id := range st.Order {
		if id != model.Main && (showAll || st.State(st.Nodes[id], now) == "running") {
			shown++
		}
	}
	if hidden := total - shown; !showAll && hidden > 0 {
		out = append(out, fg(colFaint).Render(fmt.Sprintf("%d finished or idle hidden · h", hidden)))
	} else if total == 0 {
		out = append(out, fg(colFaint).Render("no agent running"))
	}
	return strings.Join(out, "\n")
}

func costPanel(st *store.Store, width int) string {
	sm := st.Summary()
	c := sm.Cost
	rows := []struct {
		label  string
		tokens int
		usd    float64
	}{{"input (uncached)", sm.Fresh, c.Fresh}, {"cache write", sm.Write, c.Write},
		{"cache read", sm.Read, c.Read}, {"output", sm.Out, c.Out}}
	inner := width - 4
	var l []string
	for _, r := range rows {
		left := fg(colGrey).Render(r.label)
		right := lipgloss.NewStyle().Foreground(colText).Render(padLeft(fmtTokens(r.tokens), 7)) + fg(colDim).Render(padLeft(usd(r.usd), 10))
		l = append(l, padRight(left, inner-17)+right)
	}
	l = append(l, fg(colLine).Render(strings.Repeat("─", inner)))
	l = append(l, padRight(lipgloss.NewStyle().Bold(true).Render("total"), inner-10)+lipgloss.NewStyle().Bold(true).Render(padLeft(usd(c.Total()), 10)))
	if hr, ok := sm.HitRate(); ok {
		hs := bold(colOrange)
		if hr >= 0.8 {
			hs = bold(colGreen)
		}
		line := fg(colDim).Render("cache hit ") + hs.Render(fmt.Sprintf("%.1f%%", hr*100))
		if c.NoCache > 0 {
			line += fg(colGreen).Render(fmt.Sprintf("  saved %s", usd(c.Saved()))) + fg(colFaint).Render(fmt.Sprintf(" (no cache %s)", usd(c.NoCache)))
		}
		l = append(l, line)
	}
	if sm.Unpriced > 0 {
		l = append(l, fg(colOrange).Render(fmt.Sprintf("%d turns on an unpriced model are excluded", sm.Unpriced)))
	}
	return box(fg(colDim).Render("cost (estimate)"), strings.Join(l, "\n"), width, colLine)
}

func advisorPanel(st *store.Store, width int) string {
	a := st.Advisor
	name := shortModel(a.Model)
	if name == "" {
		name = "Advisor"
	}
	body := bold(colPurple).Render(name) + "\n" + fg(colDim).Render("calls ") + lipgloss.NewStyle().Bold(true).Render(fmt.Sprint(a.Calls))
	if a.LastAdvice != "" {
		body += "\n\n" + fg(colFaint).Render("last advice") + "\n" + a.LastAdvice
	}
	return box(fg(colPurple).Render("advisor"), body, width, colPurple)
}

func jevPanel(st *store.Store, width int) string {
	j := st.Jev
	var l []string
	l = append(l, fg(colDim).Render("fork layer  ")+bold(colGreen).Render(fmt.Sprintf("forks %d", j.Forks)), "")
	names := append([]string(nil), j.Order...)
	sort.Strings(names)
	for _, name := range names {
		conf, ok := j.AvgConfidence(name)
		label := strings.ReplaceAll(strings.TrimPrefix(name, "jev_"), "_", " ")
		line := padRight(fg(colGrey).Render(label), 15) + barOf(conf, 10, ok)
		if ok {
			line += fg(colGrey).Render(fmt.Sprintf(" %.2f", conf))
		} else {
			line += fg(colFaint).Render("  n/a")
		}
		if e := j.Decisions[name].Escalated; e > 0 {
			line += fg(colOrange).Render(fmt.Sprintf(" ↑%d", e))
		}
		l = append(l, line)
	}
	return box(fg(colGreen).Render("JEV"), strings.Join(l, "\n"), width, colGreen)
}

func contextPanel(c *composition.Composition, width int) string {
	if c == nil || c.Turns == 0 {
		return box(fg(colDim).Render("where tokens go"), fg(colFaint).Render("computing…"), width, colLine)
	}
	colors := map[string]lipgloss.Style{
		"context before the first message":       fg(colGrey),
		"code and commands written (tool calls)": fg(colOrange),
		"tool outputs":                           fg(colGreen),
		"your messages":                          fg(colUser),
		"assistant replies":                      fg(colPurple),
	}
	var l []string
	for _, r := range c.Categories() {
		share := c.Share(r.Volume)
		l = append(l, lipgloss.NewStyle().Bold(true).Render(padLeft(fmt.Sprintf("%.1f%%", share*100), 6))+" "+
			barOf(min(share/0.5, 1), 8, true)+fg(colFaint).Render(" "+usd(c.USD(r.Volume))))
		st, ok := colors[r.Label]
		if !ok {
			st = fg(colFaint)
		}
		l = append(l, "       "+st.Render(clip(r.Label, width-11)))
		if r.Label == "tool outputs" {
			type kv struct {
				k string
				v *composition.ToolVol
			}
			var tools []kv
			for k, v := range c.Outputs {
				tools = append(tools, kv{k, v})
			}
			sort.Slice(tools, func(i, j int) bool { return tools[i].v.Volume > tools[j].v.Volume })
			for i, t := range tools {
				if i == 3 {
					break
				}
				l = append(l, fg(colFaint).Render(fmt.Sprintf("         %-10s%6.1f%%", clip(t.k, 10), c.Share(t.v.Volume)*100)))
			}
		}
	}
	share, usdv := c.WhatIfCompressOutputs(0.7)
	l = append(l, "", fg(colGrey).Render(fmt.Sprintf("shrinking tool outputs 70%% ≈ −%.1f%%", share*100))+" "+fg(colGreen).Render("("+usd(usdv)+")"))
	return box(fg(colDim).Render("where tokens go (estimate)"), strings.Join(l, "\n"), width, colLine)
}
