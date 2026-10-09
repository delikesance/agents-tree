package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

var sevenKinds = []string{"explorer", "worker", "researcher", "dispatcher", "reviewer", "planner", "tester"}

// sevenChildren is main with seven direct children of different kinds/models, one of them with a child.
func sevenChildren() *store.Store {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "", 5))
	for i, k := range sevenKinds {
		id := fmt.Sprintf("c%d", i)
		mdl := []string{haiku, sonnet, haiku, "claude-opus-5-5", sonnet, haiku, "claude-unknown-9"}[i]
		s.Apply(evStart(1001+float64(i), id, model.Main, k, "job "+k, mdl))
		s.Apply(evTurn(1002+float64(i), id, mdl, "Read", 7))
	}
	s.Apply(evStart(1020, "g0", "c1", "grandchild", "nested job", haiku))
	s.Apply(evEnd(1021, "c2", "done"))
	s.Apply(evEnd(1021, "c3", "failed"))
	return s
}

func TestTreeShowsEveryKindWithConnectorsAndNoLineTooWide(t *testing.T) {
	s := sevenChildren()
	for _, w := range []int{196, 106, 60} {
		out := treeView(s, w, 0, 1030, true)
		plain := strip(out)
		for _, k := range append([]string{"main", "grandchild"}, sevenKinds...) {
			if !strings.Contains(plain, "╭─ "+k+" ") {
				t.Errorf("width %d: no box for %s", w, k)
			}
		}
		for _, c := range []string{"─", "│", "┬"} {
			if !strings.Contains(plain, c) {
				t.Errorf("width %d: connector %q missing:\n%s", w, c, plain)
			}
		}
		if strings.Count(plain, "╭─") != 9 {
			t.Errorf("width %d: %d boxes, want 9", w, strings.Count(plain, "╭─"))
		}
		for _, want := range []string{"✓ done", "✗ failed", "⠋ Read", "Opus 5.5", "Haiku 5.5", "claude-unknown-9", "job explorer"} {
			if !strings.Contains(plain, want) {
				t.Errorf("width %d: tree lacks %q", w, want)
			}
		}
		for i, l := range lines(out) {
			if lw := lipgloss.Width(l); lw > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lw, strip(l))
			}
		}
	}
}

func TestTreeWrapsChildrenIntoRowsWhenTheyDoNotFit(t *testing.T) {
	s := sevenChildren()
	wide := strip(treeView(s, 400, 0, 1030, true))
	narrow := strip(treeView(s, 70, 0, 1030, true))
	if strings.Contains(wide, "└") || !strings.Contains(wide, "┌") || !strings.Contains(wide, "┐") {
		t.Errorf("seven boxes fit on one row at 400 columns: one bus line, no trunk:\n%s", wide)
	}
	// seven 32-wide boxes do not fit in 70 columns: they stack under a vertical trunk
	for _, c := range []string{"┌", "├", "└"} {
		if !strings.Contains(narrow, c) {
			t.Errorf("narrow tree lacks the trunk piece %q:\n%s", c, narrow)
		}
	}
	if len(lines(narrow)) <= len(lines(wide)) {
		t.Errorf("a narrow tree is taller (%d) than a wide one (%d)", len(lines(narrow)), len(lines(wide)))
	}
}

func TestTreeActiveViewDrawsOnlyRunningAgentsAndTheirAncestors(t *testing.T) {
	s := sevenChildren()
	active := strip(treeView(s, 196, 0, 1030, false))
	// c2 done, c3 failed; the others are running (last activity 1002..1008, now 1030); c1 has a running child
	for _, gone := range []string{"╭─ researcher ", "╭─ dispatcher "} {
		if strings.Contains(active, gone) {
			t.Errorf("finished agent drawn in the active view: %s", gone)
		}
	}
	for _, kept := range []string{"╭─ main ", "╭─ worker ", "╭─ grandchild ", "╭─ explorer "} {
		if !strings.Contains(active, kept) {
			t.Errorf("missing %s in the active view", kept)
		}
	}
	empty := store.New()
	empty.Apply(evTurn(1000, model.Main, sonnet, "", 1))
	out := strip(treeView(empty, 100, 0, 1010, false))
	if !strings.Contains(out, "no agent running") || !strings.Contains(out, "╭─ main ") {
		t.Errorf("empty active tree:\n%s", out)
	}
	if strings.Contains(strip(treeView(empty, 100, 0, 1010, true)), "no agent running") {
		t.Error("the history view never says no agent running")
	}
}

func TestTreeNodeBoxShowsModelStatusCostAndCache(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1000, Kind: model.Turn, AgentID: model.Main, Model: sonnet, Effort: "high", Tool: "Bash",
		Usage: &model.Usage{Input: 1000, CacheRead: 9000, Output: 500}})
	b := strip(nodeBox(s, model.Main, 0, 1005))
	for _, want := range []string{"╭─ main", "Sonnet 5.5 · high", "⠋ Bash", "1 turns", "~$", "cache 90% · out 500"} {
		if !strings.Contains(b, want) {
			t.Errorf("main box lacks %q:\n%s", want, b)
		}
	}
	idle := strip(nodeBox(s, model.Main, 0, 1000+store.StaleSecs+600))
	if !strings.Contains(idle, "◌ idle") {
		t.Errorf("an old main reads idle:\n%s", idle)
	}
	s.Apply(evStart(1010, "x", model.Main, "worker", "", "claude-unknown-9"))
	s.Apply(evStart(1010, "y", model.Main, "worker", "", ""))
	if y := strip(nodeBox(s, "y", 0, 1011)); !strings.Contains(y, "model n/a") {
		t.Errorf("an agent whose model is not known yet:\n%s", y)
	}
	x := strip(nodeBox(s, "x", 0, 1011))
	if !strings.Contains(x, "$ n/a") || !strings.Contains(x, "claude-unknown-9") {
		t.Errorf("unpriced / unknown model:\n%s", x)
	}
	for _, l := range lines(nodeBox(s, "x", 0, 1011)) {
		if lw := lipgloss.Width(l); lw > treeBoxW {
			t.Errorf("node box line wider than %d: %d", treeBoxW, lw)
		}
	}
}

func treeApp(t *testing.T, w, h int) *Model {
	t.Helper()
	var evs []model.Event
	evs = append(evs, evTurn(1000, model.Main, sonnet, "", 5))
	for i, k := range sevenKinds {
		id := fmt.Sprintf("c%d", i)
		evs = append(evs, evStart(1001+float64(i), id, model.Main, k, "job "+k, haiku), evTurn(1002+float64(i), id, haiku, "Read", 7))
	}
	m := replayApp(t, evs, w, h)
	m.settle(t)
	m.press("t")
	m.tick(1)
	return m
}

func TestTreeViewInTheAppAtWidth200And110(t *testing.T) {
	for _, w := range []int{200, 110} {
		m := treeApp(t, w, 70)
		scr := m.screen()
		for _, k := range append([]string{"main"}, sevenKinds...) {
			if !strings.Contains(scr, "╭─ "+k+" ") {
				t.Errorf("width %d: no %s box on screen", w, k)
			}
		}
		if !strings.Contains(scr, "┬") || !strings.Contains(scr, "─ log ") {
			t.Errorf("width %d: connectors and the log expected", w)
		}
		for i, l := range lines(m.Render()) {
			if lw := lipgloss.Width(l); lw > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lw, strip(l))
			}
		}
		if n := len(lines(m.Render())); n != 70 {
			t.Errorf("width %d: %d lines, want 70", w, n)
		}
	}
}

func TestTreeViewScrollsWhenTallerThanTheWindow(t *testing.T) {
	m := treeApp(t, 110, 30) // seven stacked boxes do not fit in 30 rows
	if m.treeVP.TotalLineCount() <= m.treeVP.Height() {
		t.Fatalf("the test needs a tree taller than its viewport: %d vs %d", m.treeVP.TotalLineCount(), m.treeVP.Height())
	}
	if !m.treeVP.AtTop() || !strings.Contains(strip(m.treeVP.View()), "╭─ main ") {
		t.Fatal("the tree starts at the top")
	}
	m.press("pgdown")
	if m.treeVP.AtTop() {
		t.Error("pgdown scrolls the tree")
	}
	if strings.Contains(strip(m.treeVP.View()), "╭─ main ") {
		t.Error("main scrolled out of the tree viewport")
	}
	m.press("up", "up", "up", "up", "pgup", "pgup")
	if !m.treeVP.AtTop() {
		t.Error("scrolling back up reaches the top")
	}
	if m.follow != true {
		t.Error("tree scrolling must not touch the chat's follow flag")
	}
}
