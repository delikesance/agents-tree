package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/pricing"
	"github.com/delikesance/agents-tree/internal/store"
)

const (
	treeBoxW = 32
	treeGap  = 2
)

func center(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return strings.Repeat(" ", d/2) + s + strings.Repeat(" ", d-d/2)
}

// nodeBox draws one agent of the tree view: a fixed-size rounded box.
func nodeBox(st *store.Store, id string, frame int, now float64) string {
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
	inner := treeBoxW - 4
	head := shortModel(n.Model)
	if head == "" {
		head = "model n/a"
	}
	if n.Effort != "" {
		head += " · " + n.Effort
	}
	headSt, dim := fg(border), fg(colFaint)
	if running {
		headSt, dim = bold(border), fg(colGrey)
	}
	var status string
	var stSt lipgloss.Style
	switch {
	case id == model.Main && state == "stale":
		status, stSt = "◌ idle "+fmtAge(max(0, st.Clock-max(n.LastActive, n.Started))), fg(colFaint)
	case id == model.Main:
		status, stSt = "main session", lipgloss.NewStyle().Bold(true)
		if n.Activity != "" {
			status, stSt = spin(frame)+" "+n.Activity, lipgloss.NewStyle().Bold(true)
		}
	case state == "running":
		act := n.Activity
		if act == "" {
			act = "running"
		}
		status, stSt = spin(frame)+" "+act, bold(colGreen)
	case state == "failed":
		status, stSt = "✗ failed", bold(colRed)
	case state == "stale":
		status, stSt = "◌ no activity "+fmtAge(max(0, st.Clock-max(n.LastActive, n.Started))), fg(colFaint)
	default:
		status, stSt = "✓ done", fg(colGrey)
	}
	price := "$ n/a"
	if n.CostParts != nil {
		price = usd(n.Cost)
		if n.UnpricedTurns > 0 {
			price += "+"
		}
	}
	stats := fmt.Sprintf("%d turns · %s", n.Turns, price)
	cache := "out " + fmtTokens(n.TokensOut)
	if hr, ok := store.HitRate(n.Fresh, n.CacheWrite, n.CacheRead); ok {
		cache = fmt.Sprintf("cache %.0f%% · out %s", hr*100, fmtTokens(n.TokensOut))
	}
	rows := []string{
		center(headSt.Render(clip(head, inner)), inner),
		center(dim.Render(clip(n.Desc, inner)), inner),
		center(stSt.Render(clip(status, inner)), inner),
		center(dim.Render(clip(stats, inner)), inner),
		center(dim.Render(clip(cache, inner)), inner),
	}
	return box(bold(border).Render(n.Kind), strings.Join(rows, "\n"), treeBoxW, border)
}

type block struct {
	rows   []string
	width  int
	anchor int // column of the box's top centre
}

func padTo(s string, w int) string { return padRight(s, w) }

// hjoin places blocks side by side; returns rows, the anchor columns and the total width.
func hjoin(blocks []block) ([]string, []int, int) {
	height := 0
	for _, b := range blocks {
		height = max(height, len(b.rows))
	}
	rows := make([]string, height)
	var anchors []int
	x := 0
	for i, b := range blocks {
		for r := 0; r < height; r++ {
			line := ""
			if r < len(b.rows) {
				line = b.rows[r]
			}
			rows[r] += padTo(line, b.width)
			if i < len(blocks)-1 {
				rows[r] += strings.Repeat(" ", treeGap)
			}
		}
		anchors = append(anchors, x+b.anchor)
		x += b.width + treeGap
	}
	return rows, anchors, x - treeGap
}

// bus is the horizontal connector line above a row of children.
func bus(width int, anchors []int, parent int, trunk string) string {
	chars := make([]rune, width)
	for i := range chars {
		chars[i] = ' '
	}
	lo, hi := anchors[0], anchors[len(anchors)-1]
	for _, a := range anchors {
		lo, hi = min(lo, a), max(hi, a)
	}
	if parent >= 0 {
		lo = min(lo, parent)
	}
	if trunk != "" {
		lo = 0
	}
	for x := lo; x <= hi && x < width; x++ {
		chars[x] = '─'
	}
	for _, a := range anchors {
		chars[a] = '┬'
	}
	if trunk != "" {
		chars[0] = []rune(trunk)[0]
	} else if lo == anchors[0] {
		if anchors[0] != parent {
			chars[lo] = '┌'
		} else {
			chars[lo] = '├'
		}
	}
	if trunk == "" && hi == anchors[len(anchors)-1] && len(anchors) > 1 {
		chars[hi] = '┐'
	}
	if parent >= 0 && trunk == "" {
		in := false
		for _, a := range anchors {
			in = in || a == parent
		}
		switch {
		case in && len(anchors) == 1:
			chars[parent] = '│'
		case in:
			chars[parent] = '┼'
		default:
			chars[parent] = '┴'
		}
	}
	return fg(colFaint).Render(string(chars))
}

func subtree(st *store.Store, id string, maxWidth, frame int, now float64, showAll bool) block {
	boxRows := strings.Split(nodeBox(st, id, frame, now), "\n")
	childIDs := st.VisibleChildren(id, now, showAll)
	if len(childIDs) == 0 {
		return block{boxRows, treeBoxW, treeBoxW / 2}
	}
	kids := make([]block, len(childIDs))
	for i, c := range childIDs {
		kids[i] = subtree(st, c, maxWidth, frame, now, showAll)
	}
	// Group children into rows that fit the available width.
	groups := [][]block{{}}
	used := 0
	for _, k := range kids {
		g := &groups[len(groups)-1]
		if len(*g) > 0 && used+treeGap+k.width > maxWidth-4 {
			groups = append(groups, []block{})
			used = 0
			g = &groups[len(groups)-1]
		}
		if len(*g) > 0 {
			used += treeGap
		}
		used += k.width
		*g = append(*g, k)
	}
	wrapped := len(groups) > 1
	indentW := 0
	if wrapped {
		indentW = 2
	}
	type laid struct {
		rows    []string
		anchors []int
		w       int
	}
	ls := make([]laid, len(groups))
	maxW := 0
	for i, g := range groups {
		r, a, w := hjoin(g)
		ls[i] = laid{r, a, w}
		maxW = max(maxW, w)
	}
	width := maxW + indentW
	var rows []string
	parent := 0
	var firstAnchors []int
	for gi, l := range ls {
		shift := indentW
		if !wrapped {
			shift += (width - l.w) / 2
		}
		anchors := make([]int, len(l.anchors))
		for i, a := range l.anchors {
			anchors[i] = a + shift
		}
		if gi == 0 {
			firstAnchors = anchors
			parent = (anchors[0] + anchors[len(anchors)-1]) / 2
		}
		trunk, p := "", -1
		if wrapped {
			switch {
			case gi == 0:
				trunk = "┌"
			case gi == len(ls)-1:
				trunk = "└"
			default:
				trunk = "├"
			}
		}
		if gi == 0 {
			p = parent
		}
		rows = append(rows, padTo(bus(width, anchors, p, trunk), width))
		for _, r := range l.rows {
			prefix := strings.Repeat(" ", shift)
			if wrapped && gi < len(ls)-1 {
				prefix = fg(colFaint).Render("│") + strings.Repeat(" ", shift-1)
			}
			rows = append(rows, padTo(prefix+r, width))
		}
	}
	parentCol := parent
	if wrapped {
		parentCol = (firstAnchors[0] + firstAnchors[len(firstAnchors)-1]) / 2
	}
	// Parent box centred over its children, with a stem down to the bus line.
	pw := max(width, treeBoxW)
	off := max(0, min(pw-treeBoxW, parentCol-treeBoxW/2))
	var out []string
	for _, r := range boxRows {
		out = append(out, padTo(strings.Repeat(" ", off)+r, pw))
	}
	out = append(out, padTo(strings.Repeat(" ", parentCol)+fg(colFaint).Render("│"), pw))
	for _, r := range rows {
		out = append(out, padTo(r, pw))
	}
	return block{out, pw, off + treeBoxW/2}
}

// treeView draws the whole agent tree.
func treeView(st *store.Store, maxWidth, frame int, now float64, showAll bool) string {
	b := subtree(st, model.Main, max(maxWidth, treeBoxW+4), frame, now, showAll)
	rows := b.rows
	if !showAll && len(st.VisibleChildren(model.Main, now, false)) == 0 {
		msg := "no agent running"
		rows = append(rows, strings.Repeat(" ", max(0, b.anchor-len(msg)/2))+fg(colFaint).Render(msg))
	}
	return strings.Join(rows, "\n")
}
