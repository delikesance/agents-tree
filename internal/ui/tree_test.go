package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

var treeKinds = []string{"explorer", "worker", "researcher", "dispatcher", "reviewer", "planner", "tester"}

// treeSeven is main with seven direct children of different kinds/models, one of them (c1) with a running
// child g0 and c2 (done) with a running child g2. c3 failed.
func treeSeven() *store.Store {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "", 5))
	for i, k := range treeKinds {
		id := fmt.Sprintf("c%d", i)
		mdl := []string{haiku, sonnet, haiku, "claude-opus-5-5", sonnet, haiku, "claude-unknown-9"}[i]
		s.Apply(evStart(1001+float64(i), id, model.Main, k, "job "+k, mdl))
		s.Apply(evTurn(1002+float64(i), id, mdl, "Read", 7))
	}
	s.Apply(evStart(1020, "g0", "c1", "grandchild", "nested job", haiku))
	s.Apply(evStart(1021, "g2", "c2", "inheritor", "child of a done agent", haiku))
	s.Apply(evEnd(1022, "c2", "done"))
	s.Apply(evEnd(1022, "c3", "failed"))
	return s
}

// treeNKids is main with n running children of kind "kid".
func treeNKids(n int) *store.Store {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "", 5))
	for i := 0; i < n; i++ {
		s.Apply(evStart(1001+float64(i), fmt.Sprintf("k%d", i), model.Main, "kid", "job", haiku))
	}
	return s
}

func treeMaxWidth(s string) int {
	w := 0
	for _, l := range lines(s) {
		w = max(w, lipgloss.Width(l))
	}
	return w
}

func treeBoxes(plain string) int { return strings.Count(plain, "╭─ ") }

func TestTreeShowsEveryKindWithConnectorsAndNoLineTooWideAt200_110_60(t *testing.T) {
	s := treeSeven()
	for _, w := range []int{200, 110, 60} {
		out := treeView(s, w, 0, 1030, true)
		plain := strip(out)
		for _, k := range append([]string{"main", "grandchild", "inheritor"}, treeKinds...) {
			if !strings.Contains(plain, "╭─ "+k+" ") {
				t.Errorf("width %d: no box for %s", w, k)
			}
		}
		if n := treeBoxes(plain); n != 10 {
			t.Errorf("width %d: %d boxes, want 10", w, n)
		}
		for _, c := range []string{"┌", "┬", "└", "│"} {
			if !strings.Contains(plain, c) {
				t.Errorf("width %d: connector %q missing:\n%s", w, c, plain)
			}
		}
		for _, want := range []string{"✓ done", "✗ failed", "⠋ Read", "Opus 5.5", "Haiku 5.5", "Sonnet 5.5", "claude-unknown-9", "job explorer"} {
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
	s := treeSeven()
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
	if strings.Contains(narrow, "┐") {
		t.Errorf("a wrapped tree has no closing corner:\n%s", narrow)
	}
	if len(lines(narrow)) <= len(lines(wide)) {
		t.Errorf("a narrow tree is taller (%d) than a wide one (%d)", len(lines(narrow)), len(lines(wide)))
	}
	// the three real widths: the narrower, the more rows of children
	h := func(w int) int { return len(lines(treeView(s, w, 0, 1030, true))) }
	if !(h(200) <= h(110) && h(110) <= h(60)) {
		t.Errorf("tree heights should grow as the width shrinks: 200=%d 110=%d 60=%d", h(200), h(110), h(60))
	}
}

func TestTreeConnectorsForOneTwoAndThreeChildren(t *testing.T) {
	junction := "┌┐┬┴┼├└"
	one := strip(treeView(treeNKids(1), 200, 0, 1030, true))
	if strings.ContainsAny(one, junction) {
		t.Errorf("a single child hangs on a plain stem, no junction:\n%s", one)
	}
	stems := 0
	for _, l := range lines(one) {
		if strings.TrimSpace(l) == "│" {
			stems++
		}
	}
	if stems < 2 || treeBoxes(one) != 2 {
		t.Errorf("single child: %d stem lines, %d boxes:\n%s", stems, treeBoxes(one), one)
	}

	two := strip(treeView(treeNKids(2), 200, 0, 1030, true))
	for _, c := range []string{"┌", "┐", "┴"} { // the parent sits between its two children
		if !strings.Contains(two, c) {
			t.Errorf("two children lack %q:\n%s", c, two)
		}
	}
	three := strip(treeView(treeNKids(3), 200, 0, 1030, true))
	for _, c := range []string{"┌", "┐", "┼"} { // the parent stem meets the middle child
		if !strings.Contains(three, c) {
			t.Errorf("three children lack %q:\n%s", c, three)
		}
	}
	if strings.Contains(three, "┴") || strings.Contains(three, "└") {
		t.Errorf("three children fit on one row:\n%s", three)
	}
	four := strip(treeView(treeNKids(4), 200, 0, 1030, true)) // inner children hang on ┬
	for _, c := range []string{"┌", "┬", "┐", "┴"} {
		if !strings.Contains(four, c) {
			t.Errorf("four children lack %q:\n%s", c, four)
		}
	}
}

func TestTreeActiveViewDrawsOnlyRunningAgentsAndTheirAncestors(t *testing.T) {
	s := treeSeven()
	active := strip(treeView(s, 400, 0, 1030, false))
	// c3 failed (gone); c2 is done but its child g2 runs (kept as an ancestor); the others run
	if strings.Contains(active, "╭─ dispatcher ") {
		t.Error("a failed agent is drawn in the active view")
	}
	for _, kept := range []string{"╭─ main ", "╭─ worker ", "╭─ grandchild ", "╭─ explorer ", "╭─ researcher ", "╭─ inheritor "} {
		if !strings.Contains(active, kept) {
			t.Errorf("missing %s in the active view:\n%s", kept, active)
		}
	}
	all := strip(treeView(s, 400, 0, 1030, true))
	if !strings.Contains(all, "╭─ dispatcher ") || treeBoxes(all) <= treeBoxes(active) {
		t.Error("the full history draws every agent")
	}
	if strings.Contains(active, "no agent running") {
		t.Error("agents are running: no empty message")
	}

	// the same tree a long time later: nothing is running any more
	late := strip(treeView(s, 400, 0, 1030+store.StaleSecs+600, false))
	if treeBoxes(late) != 1 || !strings.Contains(late, "no agent running") {
		t.Errorf("only main and the empty message once everything went silent:\n%s", late)
	}
}

func TestTreeSaysNoAgentRunningOnlyInTheActiveView(t *testing.T) {
	empty := store.New()
	empty.Apply(evTurn(1000, model.Main, sonnet, "", 1))
	out := strip(treeView(empty, 100, 0, 1010, false))
	if !strings.Contains(out, "no agent running") || !strings.Contains(out, "╭─ main ") {
		t.Errorf("empty active tree:\n%s", out)
	}
	if strings.Contains(strip(treeView(empty, 100, 0, 1010, true)), "no agent running") {
		t.Error("the history view never says no agent running")
	}
	for _, l := range lines(treeView(empty, 100, 0, 1010, false)) {
		if lw := lipgloss.Width(l); lw > 100 {
			t.Errorf("empty tree line is %d wide", lw)
		}
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

// ---- the tree inside the app --------------------------------------------------------------------------------

func treeAppEvents() []model.Event {
	evs := []model.Event{evTurn(1000, model.Main, sonnet, "", 5),
		{TS: 1000.5, Kind: model.Advisor, AgentID: model.Main, Model: "claude-opus-5-5"}}
	for i, k := range treeKinds {
		id := fmt.Sprintf("c%d", i)
		evs = append(evs, evStart(1001+float64(i), id, model.Main, k, "job "+k, haiku), evTurn(1002+float64(i), id, haiku, "Read", 7))
	}
	return evs
}

// treeApp is a replay of treeAppEvents played to the end, in tree view.
func treeApp(t *testing.T, w, h int) *Model {
	t.Helper()
	m := replayApp(t, treeAppEvents(), w, h)
	m.settle(t)
	m.press("t")
	m.tick(1)
	if m.view != "tree" {
		t.Fatal("t opens the tree view")
	}
	return m
}

func TestTreeViewInTheAppFitsTheScreenAt200_110_60(t *testing.T) {
	for _, w := range []int{200, 110, 60} {
		m := treeApp(t, w, 100)
		scr := m.screen()
		for _, k := range append([]string{"main"}, treeKinds...) {
			if !strings.Contains(scr, "╭─ "+k+" ") {
				t.Errorf("width %d: no %s box on screen", w, k)
			}
		}
		if !strings.Contains(scr, "┬") || !strings.Contains(scr, "─ log ") {
			t.Errorf("width %d: connectors and the log box expected", w)
		}
		if strings.Contains(scr, "Message Claude") {
			t.Errorf("width %d: no message box in the tree view", w)
		}
		for i, l := range lines(m.Render()) {
			if lw := lipgloss.Width(l); lw > w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lw, strip(l))
			}
		}
		if n := len(lines(m.Render())); n != 100 {
			t.Errorf("width %d: %d lines, want 100", w, n)
		}
	}
}

func TestTreeViewHasALogBoxUnderTheTreeThatKeepsTheLastLines(t *testing.T) {
	m := treeApp(t, 140, 60)
	scr := m.screen()
	logAt := strings.Index(scr, "─ log ")
	if logAt < 0 || logAt < strings.Index(scr, "╭─ main ") || !strings.Contains(scr[logAt:], "advisor called") {
		t.Errorf("the log box sits under the tree and lists events:\n%s", scr)
	}
	for i := 0; i < 15; i++ {
		m.st.Apply(model.Event{TS: 1100 + float64(i), Kind: model.Advisor, AgentID: model.Main})
	}
	m.tick(1)
	if n := strings.Count(m.screen(), "advisor called"); n != 9 {
		t.Errorf("the log box shows the 9 most recent lines, got %d", n)
	}
}

func TestTreeViewScrollsWithDownEndAndHome(t *testing.T) {
	m := treeApp(t, 110, 30) // stacked rows of boxes do not fit in 30 rows
	if m.treeVP.TotalLineCount() <= m.treeVP.Height() {
		t.Fatalf("the test needs a tree taller than its viewport: %d vs %d", m.treeVP.TotalLineCount(), m.treeVP.Height())
	}
	if !m.treeVP.AtTop() || !strings.Contains(strip(m.treeVP.View()), "╭─ main ") {
		t.Fatal("the tree starts at the top")
	}
	if strings.Contains(strip(m.treeVP.View()), "╭─ tester ") {
		t.Fatal("the last child starts below the fold")
	}
	m.press("down")
	if m.treeVP.AtTop() {
		t.Error("down scrolls the tree")
	}
	m.press("end")
	if !m.treeVP.AtBottom() || !strings.Contains(strip(m.treeVP.View()), "╭─ tester ") {
		t.Errorf("end reaches the last row of boxes:\n%s", strip(m.treeVP.View()))
	}
	if strings.Contains(strip(m.treeVP.View()), "╭─ main ") {
		t.Error("main scrolled out of the viewport")
	}
	if !strings.Contains(m.screen(), "─ log ") {
		t.Error("the log box stays on screen while the tree scrolls")
	}
	m.press("home")
	if !m.treeVP.AtTop() || !strings.Contains(strip(m.treeVP.View()), "╭─ main ") {
		t.Error("home goes back to the top")
	}
	m.press("pgdown")
	if m.treeVP.AtTop() {
		t.Error("pgdown scrolls the tree")
	}
	if !m.follow {
		t.Error("tree scrolling must not touch the chat's follow flag")
	}
}

func TestTreeViewActiveFilterSaysNoAgentRunningAndHShowsHistoryAgain(t *testing.T) {
	evs := []model.Event{evTurn(1000, model.Main, sonnet, "", 1),
		evStart(1001, "a", model.Main, "explorer", "old job", haiku), evEnd(1002, "a", "done")}
	m := replayApp(t, evs, 120, 40)
	m.settle(t)
	m.press("t")
	m.tick(1)
	if scr := m.screen(); !strings.Contains(scr, "╭─ explorer ") || strings.Contains(scr, "no agent running") {
		t.Errorf("a replay starts with the full history:\n%s", scr)
	}
	m.press("h")
	m.tick(1)
	scr := m.screen()
	if strings.Contains(scr, "╭─ explorer ") || !strings.Contains(scr, "no agent running") || !strings.Contains(scr, "╭─ main ") {
		t.Errorf("active view: main only and the empty message:\n%s", scr)
	}
	m.press("h")
	m.tick(1)
	if !strings.Contains(m.screen(), "╭─ explorer ") {
		t.Error("h again brings the history back")
	}
}

func TestTreeViewInALiveSessionLeavesTheMessageBoxAndComesBack(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hello"), ProjectsDir: root, CanSend: true})
	if !m.inputFocus {
		t.Fatal("the live box starts focused")
	}
	m.press("esc", "t")
	m.tick(1)
	if m.view != "tree" || strings.Contains(m.screen(), "Message Claude") || !strings.Contains(m.screen(), "╭─ main ") {
		t.Errorf("tree view of a live session:\n%s", m.screen())
	}
	m.press("i") // writing is a chat feature
	if m.inputFocus {
		t.Error("i does nothing in the tree view")
	}
	m.press("t")
	m.tick(1)
	if m.view != "chat" || !strings.Contains(m.screen(), "Message Claude") {
		t.Errorf("t returns to the chat with the message box:\n%s", m.screen())
	}
}
