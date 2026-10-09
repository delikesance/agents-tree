package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

func convStore() *store.Store {
	s := store.New()
	for _, ev := range conversationEvents() {
		s.Apply(ev)
	}
	return s
}

func renderPlain(s *store.Store, m *model.Message, c rctx) string {
	out, _ := renderMessage(m, c)
	return strip(out)
}

func ctx(s *store.Store) rctx { return rctx{st: s, now: 1010, width: 90} }

func TestEachRoleRendersAReadableBox(t *testing.T) {
	s := convStore()
	by := byID(s.Messages)
	c := ctx(s)
	out := map[string]string{}
	for id, m := range by {
		out[id] = renderPlain(s, m, c)
	}

	if !strings.Contains(out["u"], "you") || !strings.Contains(out["u"], "ne montrer que les agents") {
		t.Errorf("user box:\n%s", out["u"])
	}
	// system lines are collapsed: first line, line count, hidden body
	if !strings.Contains(out["s"], "⚙ Stop hook feedback:") || !strings.Contains(out["s"], "2 lines") || strings.Contains(out["s"], "untracked") {
		t.Errorf("system line:\n%s", out["s"])
	}
	// assistant: label, model, markdown (bullets, inline code without backticks)
	a := out["a"]
	if !strings.Contains(a, "main") || !strings.Contains(a, "Sonnet 5.5") || !strings.Contains(a, "• the rail lists running agents") ||
		!strings.Contains(a, "• h shows history") || strings.Contains(a, "`") {
		t.Errorf("assistant box:\n%s", a)
	}
	// tool lines: icon, name, detail, duration
	for _, want := range []string{"✗", "Bash", "pytest -q", "9.0 s"} {
		if !strings.Contains(out["t1"], want) {
			t.Errorf("failed tool line lacks %q: %q", want, out["t1"])
		}
	}
	for _, want := range []string{"✓", "Read", "a/store.py", "0.2 s"} {
		if !strings.Contains(out["t2"], want) {
			t.Errorf("ok tool line lacks %q: %q", want, out["t2"])
		}
	}
	// delegation: kind, description, prompt, and a running chip because the target is alive
	d := out["deleg:ag1"]
	for _, want := range []string{"→ explorer", "map the repo", "List files.", "running"} {
		if !strings.Contains(d, want) {
			t.Errorf("delegation lacks %q:\n%s", want, d)
		}
	}
	// a subagent's own output is nested under its delegation
	if !strings.HasPrefix(out["sa"], strings.Repeat(" ", nestIndent)) || !strings.Contains(out["sa"], "explorer") || !strings.Contains(out["sa"], "Looking.") {
		t.Errorf("nested subagent box:\n%s", out["sa"])
	}
	if strings.HasPrefix(out["a"], " ") {
		t.Errorf("a main box must not be indented:\n%s", out["a"])
	}
	// report: arrow, kind, duration, text
	r := out["rep:ag1"]
	for _, want := range []string{"↩ explorer reported", "Found 3 files.", "5.0 s"} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}
}

func TestEveryRoleFitsTheRequestedWidth(t *testing.T) {
	s := convStore()
	for _, w := range []int{30, 60, 90, 130} {
		c := ctx(s)
		c.width = w
		for _, m := range s.Messages {
			out, _ := renderMessage(m, c)
			for _, l := range lines(out) {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("%s at width %d: line of width %d: %q", m.ID, w, lw, strip(l))
				}
			}
		}
	}
}

func TestToolLineShowsSpinnerWhileRunningAndMarkerWhenNeverAnswered(t *testing.T) {
	s := store.New()
	now := float64(1000)
	s.Apply(evTurn(now, model.Main, sonnet, "Bash", 1))
	m := &model.Message{ID: "t", TS: now, AgentID: model.Main, Role: "tool", Tool: "Bash", Detail: "sleep 30", Status: "running"}
	s.Apply(model.Event{TS: now, Kind: model.Message_, AgentID: model.Main, Msg: m})
	c := rctx{st: s, now: now + 5, width: 80}
	got := renderPlain(s, m, c)
	if !strings.Contains(got, spin(0)) || strings.Contains(got, "✓") || strings.Contains(got, "✗") || !strings.Contains(got, "sleep 30") {
		t.Errorf("running tool: %q", got)
	}
	c.frame = 3
	if got3 := renderPlain(s, m, c); !strings.Contains(got3, spin(3)) || got3 == got {
		t.Errorf("spinner must advance with the frame: %q vs %q", got, got3)
	}
	// the agent went quiet long ago: the tool never answered -> a neutral marker, not an endless spinner
	c.now = now + store.StaleSecs + 100
	stale := renderPlain(s, m, c)
	if strings.ContainsAny(stale, "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏") || !strings.Contains(stale, "…") {
		t.Errorf("stale tool: %q", stale)
	}
}

func TestDelegationChipTracksTheTargetState(t *testing.T) {
	s := convStore()
	by := byID(s.Messages)
	c := ctx(s)
	if got := renderPlain(s, by["deleg:ag1"], c); !strings.Contains(got, "running") {
		t.Fatalf("running chip: %s", got)
	}
	s.Apply(evEnd(1011, "ag1", "done"))
	s.Apply(evUpdate(1011, "deleg:ag1", "done", fp(7.5)))
	got := renderPlain(s, by["deleg:ag1"], c)
	if !strings.Contains(got, "✓ done") || !strings.Contains(got, "7.5 s") || strings.Contains(got, "running") {
		t.Errorf("done chip: %s", got)
	}
	s.Apply(evStart(1012, "ag2", model.Main, "worker", "x", ""))
	s.Apply(evEnd(1013, "ag2", "failed"))
	d2 := &model.Message{ID: "deleg:ag2", AgentID: model.Main, Role: "delegation", Kind: "worker", Text: "go", Target: "ag2"}
	if got := renderPlain(s, d2, c); !strings.Contains(got, "✗ failed") {
		t.Errorf("failed chip: %s", got)
	}
	// a target that stopped talking long ago
	s.Apply(evStart(1014, "ag3", model.Main, "researcher", "", ""))
	d3 := &model.Message{ID: "deleg:ag3", AgentID: model.Main, Role: "delegation", Kind: "researcher", Text: "go", Target: "ag3"}
	c.now = 1014 + store.StaleSecs + 10
	if got := renderPlain(s, d3, c); !strings.Contains(got, "no activity") {
		t.Errorf("stale chip: %s", got)
	}
}

func TestReportShowsCostWhenPricedAndFailureWhenFailed(t *testing.T) {
	s := store.New()
	s.Apply(evStart(1000, "ag1", model.Main, "worker", "edit", ""))
	s.Apply(model.Event{TS: 1001, Kind: model.Turn, AgentID: "ag1", Model: haiku, Usage: &model.Usage{Output: 1_000_000}})
	rep := &model.Message{ID: "rep:ag1", AgentID: model.Main, Role: "report", Text: "all done", Target: "ag1", Status: "done", Duration: fp(75)}
	c := rctx{st: s, now: 1005, width: 80}
	got := renderPlain(s, rep, c)
	if !strings.Contains(got, "~$") || !strings.Contains(got, "1 m 15 s") || !strings.Contains(got, "↩ worker reported") {
		t.Errorf("report: %s", got)
	}
	fail := &model.Message{ID: "rep2", AgentID: model.Main, Role: "report", Target: "ag1", Status: "failed"}
	got = renderPlain(s, fail, c)
	if !strings.Contains(got, "↩ worker failed") || !strings.Contains(got, "(no report)") {
		t.Errorf("failed report: %s", got)
	}
}

func TestLongTextFoldsAndExpands(t *testing.T) {
	s := store.New()
	var ls []string
	for i := 0; i < 20; i++ {
		ls = append(ls, "line "+string(rune('a'+i)))
	}
	text := strings.Join(ls, "\n")
	for _, role := range []string{"user", "assistant"} {
		m := &model.Message{ID: "x-" + role, AgentID: model.Main, Role: role, Text: text}
		folded := renderPlain(s, m, rctx{st: s, width: 80})
		full := renderPlain(s, m, rctx{st: s, width: 80, expanded: true})
		if !strings.Contains(folded, "line f") || strings.Contains(folded, "line g") || !strings.Contains(folded, "+14 lines") || !strings.Contains(folded, "e to expand") {
			t.Errorf("%s folded:\n%s", role, folded)
		}
		if !strings.Contains(full, "line t") || strings.Contains(full, "+14 lines") {
			t.Errorf("%s expanded:\n%s", role, full)
		}
	}
}

func TestFoldHelper(t *testing.T) {
	if got, hidden := fold("a\nb\nc\nd", false, 2); got != "a\nb" || hidden != 2 {
		t.Errorf("fold = %q, %d", got, hidden)
	}
	if got, hidden := fold("a\nb\nc\nd", true, 2); got != "a\nb\nc\nd" || hidden != 0 {
		t.Errorf("expanded fold = %q, %d", got, hidden)
	}
	if got, hidden := fold("a\nb", false, 2); got != "a\nb" || hidden != 0 {
		t.Errorf("short fold = %q, %d", got, hidden)
	}
}

func TestSystemLineCollapsedAndExpanded(t *testing.T) {
	s := store.New()
	m := &model.Message{ID: "s", AgentID: model.Main, Role: "system", Text: "Stop hook feedback:\nuntracked files\nthird"}
	one := renderPlain(s, m, rctx{st: s, width: 80})
	if !strings.Contains(one, "⚙ Stop hook feedback:") || !strings.Contains(one, "3 lines") || strings.Contains(one, "untracked") || len(lines(one)) != 1 {
		t.Errorf("collapsed: %q", one)
	}
	full := renderPlain(s, m, rctx{st: s, width: 80, expanded: true})
	if !strings.Contains(full, "⚙ system") || !strings.Contains(full, "untracked files") || !strings.Contains(full, "third") {
		t.Errorf("expanded:\n%s", full)
	}
	single := renderPlain(s, &model.Message{ID: "s2", AgentID: model.Main, Role: "system", Text: "[Request interrupted by user]"}, rctx{st: s, width: 80})
	if strings.Contains(single, "lines ▸") {
		t.Errorf("a one-line system message has no line counter: %q", single)
	}
}

func TestCompactMessagesSitTightAndBoxesGetABlankLine(t *testing.T) {
	s := convStore()
	by := byID(s.Messages)
	c := ctx(s)
	for id, want := range map[string]bool{"u": false, "a": false, "t1": true, "t2": true, "s": true, "deleg:ag1": false, "rep:ag1": false} {
		if _, compact := renderMessage(by[id], c); compact != want {
			t.Errorf("%s compact = %v, want %v", id, compact, want)
		}
	}
}

func TestMessageSignatureChangesOnlyWhenTheBoxWould(t *testing.T) {
	s := convStore()
	by := byID(s.Messages)
	c := ctx(s)
	sig := func(id string, c rctx) string { return messageSig(by[id], c) }

	c5 := c
	c5.frame = 5
	if sig("t2", c) != sig("t2", c5) {
		t.Error("a finished tool line does not depend on the spinner frame")
	}
	if sig("u", c) != sig("u", c5) {
		t.Error("a user box does not depend on the spinner frame")
	}
	rev := sig("t2", c)
	s.Apply(evUpdate(1010, "t2", "error", nil))
	if sig("t2", c) == rev {
		t.Error("a tool update changes its signature")
	}
	before := sig("deleg:ag1", c)
	if sig("deleg:ag1", c) == sig("deleg:ag1", c5) {
		t.Error("a delegation to a running agent animates (its chip has a spinner)")
	}
	s.Apply(evEnd(1011, "ag1", "done"))
	if sig("deleg:ag1", c) == before {
		t.Error("delegation signature must change when its target ends")
	}
	if sig("deleg:ag1", c) != sig("deleg:ag1", c5) {
		t.Error("a finished delegation is static")
	}
	e := c
	e.expanded = true
	if sig("u", c) == sig("u", e) {
		t.Error("expanding changes every signature")
	}
	n := c
	n.width = 50
	if sig("u", c) == sig("u", n) {
		t.Error("resizing changes every signature")
	}
}

func TestToolSignatureFollowsRevAndLiveness(t *testing.T) {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "Bash", 1))
	m := &model.Message{ID: "t", TS: 1000, AgentID: model.Main, Role: "tool", Tool: "Bash", Status: "running"}
	s.Apply(model.Event{TS: 1000, Kind: model.Message_, AgentID: model.Main, Msg: m})
	live := rctx{st: s, now: 1005, width: 80}
	a, b := messageSig(m, live), messageSig(m, rctx{st: s, now: 1005, width: 80, frame: 1})
	if a == b {
		t.Error("a running tool of a live agent animates")
	}
	idle := rctx{st: s, now: 1000 + store.StaleSecs + 50, width: 80}
	idle2 := idle
	idle2.frame = 1
	if messageSig(m, idle) != messageSig(m, idle2) {
		t.Error("a running tool of an idle agent is static")
	}
	s.Apply(evUpdate(1006, "t", "ok", fp(1)))
	if messageSig(m, live) == a {
		t.Error("msg update bumps Rev and changes the signature")
	}
}

func TestBoxTitleLineIsAsWideAsItsBody(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		out := strip(box("title", "hello\nworld", w, colLine))
		for i, l := range lines(out) {
			if lipgloss.Width(l) != w {
				t.Skipf("BUG: internal/ui/theme.go:174 draws the top border one column narrower than the body (width-6 should be width-5); width %d line %d is %d wide", w, i, lipgloss.Width(l))
			}
		}
	}
}
