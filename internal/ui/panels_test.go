package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/composition"
	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

// panelsFlat drops box borders and collapses whitespace, so text wrapped inside a panel can be matched.
func panelsFlat(s string) string {
	s = strings.NewReplacer("│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ", "─", " ").Replace(strip(s))
	return strings.Join(strings.Fields(s), " ")
}

func panelsRichStore() *store.Store {
	s := store.New()
	for _, ev := range richEvents() {
		s.Apply(ev)
	}
	return s
}

func panelsMaxWidth(t *testing.T, what, s string, w int) {
	t.Helper()
	for i, l := range lines(s) {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("%s: line %d is %d wide (max %d): %q", what, i, lw, w, strip(l))
		}
	}
}

// ---- cost panel -------------------------------------------------------------------------------------------

func TestPanelsCostShowsTheFourCategoriesTotalHitRateAndSavings(t *testing.T) {
	s := panelsRichStore()
	sm := s.Summary()
	hr, ok := sm.HitRate()
	if !ok || sm.Cost.Saved() < 0.005 {
		t.Fatalf("test data should hit the cache and save money: hr=%v ok=%v saved=%v", hr, ok, sm.Cost.Saved())
	}
	out := strip(costPanel(s, 70))
	for _, want := range []string{"cost (estimate)", "input (uncached)", "cache write", "cache read", "output", "total",
		fmt.Sprintf("cache hit %.1f%%", hr*100),
		"saved " + usd(sm.Cost.Saved()) + " · without cache " + usd(sm.Cost.NoCache),
		usd(sm.Cost.Total()), usd(sm.Cost.Fresh), usd(sm.Cost.Write), usd(sm.Cost.Read), usd(sm.Cost.Out),
		fmtTokens(sm.Fresh), fmtTokens(sm.Write), fmtTokens(sm.Read), fmtTokens(sm.Out)} {
		if !strings.Contains(out, want) {
			t.Errorf("cost panel lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "unpriced") {
		t.Error("no unpriced turns here")
	}
	// every category sits on its own row with its token count
	for _, row := range []struct{ label, tokens string }{{"input (uncached)", fmtTokens(sm.Fresh)}, {"cache write", fmtTokens(sm.Write)},
		{"cache read", fmtTokens(sm.Read)}, {"output", fmtTokens(sm.Out)}} {
		found := false
		for _, l := range lines(out) {
			if strings.Contains(l, row.label) && strings.Contains(l, row.tokens) && strings.Contains(l, "~$") {
				found = true
			}
		}
		if !found {
			t.Errorf("row %q should show %s tokens and a price:\n%s", row.label, row.tokens, out)
		}
	}
}

func TestPanelsCostNeverExceedsItsWidthAndKeepsTheSavingsReadable(t *testing.T) {
	s := panelsRichStore()
	sm := s.Summary()
	for _, w := range []int{70, 52, 44, 40, 36} {
		out := costPanel(s, w)
		panelsMaxWidth(t, fmt.Sprintf("cost panel at %d", w), out, w)
		if !strings.Contains(panelsFlat(out), "saved "+usd(sm.Cost.Saved())) {
			t.Errorf("width %d: savings unreadable:\n%s", w, strip(out))
		}
	}
}

func TestPanelsCostHidesTheSavingsLineBelowHalfACent(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1, Kind: model.Turn, AgentID: model.Main, Model: sonnet, Usage: &model.Usage{Input: 1000, Output: 10}})
	out := panelsFlat(costPanel(s, 70))
	if !strings.Contains(out, "cache hit 0.0%") {
		t.Errorf("no cache reads is a 0%% hit rate: %s", out)
	}
	if strings.Contains(out, "saved") || strings.Contains(out, "without cache") {
		t.Errorf("nothing saved, no savings line: %s", out)
	}
	// a tiny cache read saves less than 0.005: still no line
	s.Apply(model.Event{TS: 2, Kind: model.Turn, AgentID: model.Main, Model: sonnet, Usage: &model.Usage{Input: 10, CacheRead: 200, Output: 1}})
	if sm := s.Summary(); sm.Cost.Saved() >= 0.005 {
		t.Fatalf("test data should save less than half a cent: %v", sm.Cost.Saved())
	}
	if out := panelsFlat(costPanel(s, 70)); strings.Contains(out, "saved") {
		t.Errorf("savings under half a cent are hidden: %s", out)
	}
}

func TestPanelsCostCountsUnpricedTurnsInsteadOfPricingThemAtZero(t *testing.T) {
	s := store.New()
	for i := 1; i <= 2; i++ {
		s.Apply(model.Event{TS: float64(i), Kind: model.Turn, AgentID: model.Main, Model: "some-unknown-model-1",
			Usage: &model.Usage{Input: 10, Output: 10}})
	}
	out := panelsFlat(costPanel(s, 60))
	if !strings.Contains(out, "2 turns on an unpriced model are excluded") {
		t.Errorf("unpriced notice missing: %s", out)
	}
	if strings.Contains(out, "saved") {
		t.Error("no savings line without priced cache usage")
	}
	panelsMaxWidth(t, "unpriced cost panel", costPanel(s, 60), 60)
}

// ---- context panel ----------------------------------------------------------------------------------------

func panelsComposition() *composition.Composition {
	c := composition.New()
	c.Turns, c.PromptTokens, c.InputCost = 4, 1000, 10
	c.Baseline, c.UserText, c.AssistantText = 500, 50, 50
	c.Outputs["Bash"] = &composition.ToolVol{N: 1, Tokens: 100, Volume: 200}
	c.Outputs["Read"] = &composition.ToolVol{N: 1, Tokens: 50, Volume: 100}
	c.Outputs["Grep"] = &composition.ToolVol{N: 1, Tokens: 5, Volume: 30}
	c.Outputs["Glob"] = &composition.ToolVol{N: 1, Tokens: 1, Volume: 10}
	c.Inputs["Edit"] = &composition.ToolVol{N: 1, Tokens: 10, Volume: 20}
	return c
}

func TestPanelsContextSaysComputingWithoutAnalysis(t *testing.T) {
	for name, c := range map[string]*composition.Composition{"nil": nil, "empty": composition.New()} {
		out := strip(contextPanel(c, 44))
		if !strings.Contains(out, "where tokens go") || !strings.Contains(out, "computing…") {
			t.Errorf("%s analysis:\n%s", name, out)
		}
		if strings.Contains(out, "shrinking") || strings.Contains(out, "%") {
			t.Errorf("%s analysis shows numbers it does not have:\n%s", name, out)
		}
	}
}

func TestPanelsContextShowsCategoriesToolOutputsAndTheWhatIfLine(t *testing.T) {
	// Bash 200 + Read 100 + Grep 30 + Glob 10 = 340 of 1000 prompt tokens in tool outputs
	out := strip(contextPanel(panelsComposition(), 60))
	for _, want := range []string{"where tokens go (estimate)", "50.0%", "34.0%", "context before the first message", "tool outputs",
		"code and commands written", "your messages", "assistant replies", "other (thinking",
		"Bash", "Read", "Grep",
		"shrinking tool outputs 70% ≈ −23.8%", "(~$2.38)", "~$5.00", "~$3.40"} {
		if !strings.Contains(out, want) {
			t.Errorf("context panel lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "█") || !strings.Contains(out, "░") {
		t.Errorf("each category has a bar:\n%s", out)
	}
	if strings.Contains(out, "Glob") {
		t.Errorf("only the top 3 tools are listed under tool outputs:\n%s", out)
	}
	// largest category first; tools under "tool outputs", heaviest first
	if strings.Index(out, "context before the first message") > strings.Index(out, "tool outputs") {
		t.Errorf("categories are sorted by weight:\n%s", out)
	}
	b, r, g, o := strings.Index(out, "Bash"), strings.Index(out, "Read"), strings.Index(out, "Grep"), strings.Index(out, "tool outputs")
	if !(o < b && b < r && r < g) {
		t.Errorf("tools are listed under tool outputs, heaviest first:\n%s", out)
	}
	if strings.Index(out, "shrinking") < g {
		t.Errorf("the what-if line closes the panel:\n%s", out)
	}
	panelsMaxWidth(t, "context panel", contextPanel(panelsComposition(), 60), 60)
}

func TestPanelsContextStaysWithinNarrowWidths(t *testing.T) {
	for _, w := range []int{60, 44, 36} {
		panelsMaxWidth(t, fmt.Sprintf("context panel at %d", w), contextPanel(panelsComposition(), w), w)
	}
}

// ---- advisor & JEV ----------------------------------------------------------------------------------------

func TestPanelsAdvisorShowsModelCallsAndTheLastAdviceOnlyWhenPresent(t *testing.T) {
	s := panelsRichStore()
	out := strip(advisorPanel(s, 50))
	for _, want := range []string{"advisor", "Opus 5.5", "calls 1", "last advice", "verify the auth claims first"} {
		if !strings.Contains(out, want) {
			t.Errorf("advisor panel lacks %q:\n%s", want, out)
		}
	}
	bare := store.New()
	bare.Apply(model.Event{TS: 1, Kind: model.Advisor, AgentID: model.Main, Model: "claude-opus-5-5"})
	out = strip(advisorPanel(bare, 50))
	if !strings.Contains(out, "Opus 5.5") || !strings.Contains(out, "calls 1") || strings.Contains(out, "last advice") {
		t.Errorf("no advice yet, no 'last advice' block:\n%s", out)
	}
	anon := store.New()
	anon.Apply(model.Event{TS: 1, Kind: model.Advisor, AgentID: model.Main})
	if out := strip(advisorPanel(anon, 50)); !strings.Contains(out, "Advisor") || !strings.Contains(out, "calls 1") {
		t.Errorf("unknown advisor model reads 'Advisor':\n%s", out)
	}
	panelsMaxWidth(t, "advisor panel", advisorPanel(s, 30), 30)
}

func TestPanelsJevShowsForksBarsNotAvailableAndEscalations(t *testing.T) {
	s := panelsRichStore()
	out := strip(jevPanel(s, 44))
	for _, want := range []string{"JEV", "fork layer", "forks 2", "which file", "0.86", "retry or stop", "n/a", "↑1"} {
		if !strings.Contains(out, want) {
			t.Errorf("JEV panel lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "0.00") {
		t.Errorf("a decision without confidence is n/a, never 0.00:\n%s", out)
	}
	for _, l := range lines(out) {
		switch {
		case strings.Contains(l, "which file"):
			if !strings.Contains(l, "█") || strings.Contains(l, "n/a") || strings.Contains(l, "↑") {
				t.Errorf("which file has a bar, a confidence and no escalation: %q", l)
			}
		case strings.Contains(l, "retry or stop"):
			if strings.Contains(l, "█") || !strings.Contains(l, "░") || !strings.Contains(l, "n/a") || !strings.Contains(l, "↑1") {
				t.Errorf("retry or stop has an empty bar, n/a and one escalation: %q", l)
			}
		}
	}
	panelsMaxWidth(t, "JEV panel", jevPanel(s, 44), 44)
}

func TestPanelsAdvisorAndJevAppearInTheDetailsOverlayOnlyAfterACall(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	m.resize(150, 80)
	m.st.Apply(evTurn(nowSecs(), model.Main, sonnet, "", 1))
	m.press("d")
	if !m.details {
		t.Fatal("d opens the details")
	}
	scr := m.screen()
	for _, absent := range []string{"advisor", "fork layer", "JEV"} {
		if strings.Contains(scr, absent) {
			t.Errorf("no %s panel before a call:\n%s", absent, scr)
		}
	}
	for _, ev := range richEvents() {
		if ev.Kind == model.Advisor || ev.Kind == model.Jev {
			m.st.Apply(ev)
		}
	}
	m.tick(1)
	flat := panelsFlat(m.screen())
	for _, want := range []string{"details", "Opus 5.5", "calls 1", "verify the auth claims first", "fork layer", "forks 2", "which file", "↑1"} {
		if !strings.Contains(flat, want) {
			t.Errorf("details lack %q:\n%s", want, m.screen())
		}
	}
	m.press("d")
	if m.details || strings.Contains(m.screen(), "fork layer") {
		t.Error("d closes the details")
	}
}

func TestPanelsDetailsOverlayHoldsAgentsCostAndTheContextPanel(t *testing.T) {
	root := t.TempDir()
	path := liveFile(t, root, "s1", "hi")
	m := liveApp(t, Options{Session: path, ProjectsDir: root})
	m.resize(150, 70)
	m.press("d")
	if scr := m.screen(); !strings.Contains(scr, "No usage yet.") || strings.Contains(scr, "cost (estimate)") {
		t.Errorf("no turns, no cost panel:\n%s", scr)
	}
	m.press("d")
	m.st.Apply(model.Event{TS: nowSecs(), Kind: model.Turn, AgentID: model.Main, Model: sonnet,
		Usage: &model.Usage{Input: 100, CacheRead: 9000, Output: 20}})
	m.press("d")
	m.tick(1)
	scr := m.screen()
	if !strings.Contains(scr, "cost (estimate)") || !strings.Contains(scr, "computing…") || !strings.Contains(scr, "cache hit") ||
		!strings.Contains(scr, "AGENTS") {
		t.Errorf("agents, a cost panel and a 'computing' context panel expected:\n%s", scr)
	}
	if !m.compBusy {
		t.Error("the analysis runs in the background while the details are open")
	}
	c := panelsComposition()
	m.Update(compMsg{path: "/some/other/session.jsonl", comp: c}) // result of a session we left: ignored
	if m.comp != nil {
		t.Error("a stale analysis must be dropped")
	}
	m.Update(compMsg{path: path, comp: c})
	m.tick(1)
	scr = m.screen()
	if m.compBusy || !strings.Contains(scr, "where tokens go (estimate)") || !strings.Contains(scr, "50.0%") || strings.Contains(scr, "computing…") {
		t.Errorf("context panel with data expected:\n%s", scr)
	}
}

// ---- agent cards & the agents list ------------------------------------------------------------------------

// panelsCardStore has one agent per state at t=1005: run (running on Grep), dn (done), bad (failed), plus main.
func panelsCardStore() *store.Store {
	s := store.New()
	s.Apply(model.Event{TS: 1000, Kind: model.Turn, AgentID: model.Main, Model: sonnet, Effort: "high", Tool: "Bash",
		Usage: &model.Usage{Input: 1000, CacheRead: 9000, Output: 500}})
	s.Apply(evStart(1001, "run", model.Main, "explorer", "map the repo", haiku))
	s.Apply(model.Event{TS: 1002, Kind: model.Turn, AgentID: "run", Model: haiku, Tool: "Grep",
		Usage: &model.Usage{Input: 100, CacheRead: 900, Output: 10}})
	s.Apply(evStart(1001, "dn", model.Main, "researcher", "read the docs", haiku))
	s.Apply(evEnd(1003, "dn", "done"))
	s.Apply(evStart(1001, "bad", model.Main, "worker", "break things", sonnet))
	s.Apply(evEnd(1003, "bad", "failed"))
	return s
}

func TestPanelsCardShowsRunningDoneAndFailedAgents(t *testing.T) {
	s := panelsCardStore()
	run := strip(card(s, "run", 1005, 0, 40))
	for _, want := range []string{"╭─ explorer", "Haiku 5.5", "map the repo", "⠋ Grep", "1 turns", "~$", "90%"} {
		if !strings.Contains(run, want) {
			t.Errorf("running card lacks %q:\n%s", want, run)
		}
	}
	if next := strip(card(s, "run", 1005, 1, 40)); !strings.Contains(next, "⠙ Grep") {
		t.Errorf("the spinner advances with the frame:\n%s", next)
	}
	if dn := strip(card(s, "dn", 1005, 0, 40)); !strings.Contains(dn, "✓ done") || strings.Contains(dn, "⠋") || !strings.Contains(dn, "read the docs") {
		t.Errorf("done card:\n%s", dn)
	}
	if bad := strip(card(s, "bad", 1005, 0, 40)); !strings.Contains(bad, "✗ failed") || strings.Contains(bad, "⠋") || !strings.Contains(bad, "Sonnet 5.5") {
		t.Errorf("failed card:\n%s", bad)
	}
	mn := strip(card(s, model.Main, 1005, 0, 40))
	for _, want := range []string{"╭─ main", "Sonnet 5.5 · high", "⠋ Bash", "~$", "90%"} {
		if !strings.Contains(mn, want) {
			t.Errorf("main card lacks %q:\n%s", want, mn)
		}
	}
	s.Apply(evStart(1006, "fresh", model.Main, "worker", "", haiku))
	if fresh := strip(card(s, "fresh", 1006, 0, 40)); !strings.Contains(fresh, "⠋ working") || strings.Contains(fresh, "map the") {
		t.Errorf("an agent with no tool yet reads 'working':\n%s", fresh)
	}
}

func TestPanelsCardShowsNoActivityForSilentSubagentsAndIdleForMain(t *testing.T) {
	s := panelsCardStore()
	late := 1002 + store.StaleSecs + 600 // 12 minutes after the last event of "run" with the default settings
	run := strip(card(s, "run", late, 0, 40))
	want := "◌ no activity " + fmtAge(late-1002)
	if !strings.Contains(run, want) || strings.Contains(run, "⠋") {
		t.Errorf("a silent subagent reads %q:\n%s", want, run)
	}
	mn := strip(card(s, model.Main, late, 0, 40))
	if want := "◌ idle " + fmtAge(late-1000); !strings.Contains(mn, want) || strings.Contains(mn, "no activity") {
		t.Errorf("a silent main reads %q:\n%s", want, mn)
	}
	// a replay passes now=0: the age is relative to the newest event
	if rep := strip(card(s, "run", 0, 0, 40)); strings.Contains(rep, "◌") {
		t.Errorf("replay time 0 uses the newest event as the clock:\n%s", rep)
	}
}

func TestPanelsCardPriceIsNotAvailableForUnpricedModelsAndMissingUsage(t *testing.T) {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "", 1))
	s.Apply(evStart(1001, "u", model.Main, "worker", "mystery", "claude-unknown-9"))
	s.Apply(model.Event{TS: 1002, Kind: model.Turn, AgentID: "u", Model: "claude-unknown-9", Usage: &model.Usage{Input: 10, Output: 10}})
	s.Apply(evStart(1001, "nm", model.Main, "worker", "", ""))
	u := strip(card(s, "u", 1003, 0, 40))
	if !strings.Contains(u, "$ n/a") || strings.Contains(u, "~$0.00") || !strings.Contains(u, "claude-unknown-9") {
		t.Errorf("unpriced model:\n%s", u)
	}
	if nm := strip(card(s, "nm", 1003, 0, 40)); !strings.Contains(nm, "model n/a") || !strings.Contains(nm, "$ n/a") || !strings.Contains(nm, "0 turns") {
		t.Errorf("agent with no model and no usage:\n%s", nm)
	}
	if strings.Contains(strip(card(s, "nm", 1003, 0, 40)), "%") {
		t.Error("no cache percentage without a prompt")
	}
}

func TestPanelsCardTruncatesToItsWidth(t *testing.T) {
	s := panelsCardStore()
	long := strings.Repeat("a very long description ", 8)
	s.Apply(evStart(1001, "long", model.Main, "planner-with-a-very-long-kind-name", long, haiku))
	s.Apply(model.Event{TS: 1002, Kind: model.Turn, AgentID: "long", Model: haiku, Tool: "mcp__server__an_extremely_long_tool_name_here",
		Usage: &model.Usage{Input: 1, Output: 1}})
	for _, id := range []string{model.Main, "run", "dn", "bad", "long"} {
		for _, now := range []float64{1005, 1002 + store.StaleSecs + 600} {
			for w := 12; w <= 60; w += 4 {
				panelsMaxWidth(t, fmt.Sprintf("card %s width %d", id, w), card(s, id, now, 0, w), w)
			}
		}
	}
	c := strip(card(s, "long", 1005, 0, 30))
	if !strings.Contains(c, "…") || strings.Contains(c, "extremely_long_tool_name_here") {
		t.Errorf("long text is cut with an ellipsis:\n%s", c)
	}
}

// panelsRailStore: main, three running subagents (one has a running child), two done, one failed, one silent.
func panelsRailStore() *store.Store {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "", 5))
	s.Apply(evStart(1001, "r1", model.Main, "explorer", "scan", haiku))
	s.Apply(evStart(1001, "r2", model.Main, "worker", "edit", sonnet))
	s.Apply(evStart(1001, "r3", model.Main, "researcher", "docs", haiku))
	s.Apply(evStart(1001, "d1", model.Main, "dispatcher", "route", haiku))
	s.Apply(evStart(1001, "d2", model.Main, "reviewer", "review", haiku))
	s.Apply(evStart(1001, "f1", model.Main, "tester", "test", haiku))
	s.Apply(evStart(1001, "old", model.Main, "planner", "plan", haiku))
	s.Apply(evStart(1002, "n1", "r2", "nested", "child of worker", haiku))
	s.Apply(evEnd(1003, "d1", "done"))
	s.Apply(evEnd(1003, "d2", "done"))
	s.Apply(evEnd(1003, "f1", "failed"))
	for _, id := range []string{"r1", "r2", "r3", "n1"} {
		s.Apply(evTurn(1190, id, haiku, "Read", 3))
	}
	return s
}

func TestPanelsRailAgentsListsOnlyRunningAgentsAndCountsTheHiddenOnes(t *testing.T) {
	s := panelsRailStore()
	const now = 1200 // "old" has been silent since 1001: stale
	out := strip(railAgents(s, now, 0, false, 42))
	for _, kept := range []string{"AGENTS · 4 RUNNING", "╭─ main", "╭─ explorer", "╭─ worker", "╭─ researcher", "╭─ nested"} {
		if !strings.Contains(out, kept) {
			t.Errorf("rail lacks %q:\n%s", kept, out)
		}
	}
	for _, gone := range []string{"╭─ dispatcher", "╭─ reviewer", "╭─ tester", "╭─ planner"} {
		if strings.Contains(out, gone) {
			t.Errorf("rail shows a finished/idle agent %q:\n%s", gone, out)
		}
	}
	if !strings.Contains(out, "4 finished or idle hidden · h") {
		t.Errorf("hidden count line missing:\n%s", out)
	}
	panelsMaxWidth(t, "rail agents", railAgents(s, now, 0, false, 42), 42)

	all := strip(railAgents(s, now, 0, true, 42))
	for _, k := range []string{"main", "explorer", "worker", "researcher", "nested", "dispatcher", "reviewer", "tester", "planner"} {
		if !strings.Contains(all, "╭─ "+k) {
			t.Errorf("showAll lacks %q:\n%s", k, all)
		}
	}
	if strings.Contains(all, "hidden") || !strings.Contains(all, "✓ done") || !strings.Contains(all, "✗ failed") || !strings.Contains(all, "◌ no activity") {
		t.Errorf("showAll lists every state and has nothing hidden:\n%s", all)
	}
	for _, w := range []int{42, 30, 20} {
		panelsMaxWidth(t, fmt.Sprintf("rail agents (all) at %d", w), railAgents(s, now, 0, true, w), w)
	}
	// children are nested (indented) under their parent
	cardCol := func(plain, kind string) int {
		for _, l := range lines(plain) {
			if i := strings.Index(l, "╭─ "+kind); i >= 0 {
				return len([]rune(l[:i]))
			}
		}
		return -1
	}
	if cardCol(all, "nested") <= cardCol(all, "worker") || cardCol(all, "worker") <= cardCol(all, "main") {
		t.Errorf("cards are indented by depth:\n%s", all)
	}
}

func TestPanelsRailAgentsWithoutSubagentsSaysSo(t *testing.T) {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "", 5))
	out := strip(railAgents(s, 1001, 0, false, 40))
	if !strings.Contains(out, "AGENTS · 0 RUNNING") || !strings.Contains(out, "no agent running") || strings.Contains(out, "hidden") {
		t.Errorf("empty rail:\n%s", out)
	}
}

func TestPanelsDetailsOverlayListsRunningAgentsAndHTogglesTheHistory(t *testing.T) {
	m := replayApp(t, richEvents(), 150, 80)
	m.settle(t)
	m.press("h") // replay starts in "all": go to the active view
	m.press("d")
	scr := m.screen()
	if !strings.Contains(scr, "AGENTS · 3 RUNNING") || !strings.Contains(scr, "4 finished or idle hidden · h") {
		t.Errorf("details list only the running agents:\n%s", scr)
	}
	if !strings.Contains(scr, "cost (estimate)") || !strings.Contains(scr, "cache hit") {
		t.Errorf("details hold the cost panel:\n%s", scr)
	}
	m.press("h")
	if scr = m.screen(); strings.Contains(scr, "finished or idle hidden") || !strings.Contains(scr, "✓ done") {
		t.Errorf("h inside the details lists the finished agents too:\n%s", scr)
	}
	for _, k := range []string{"esc"} {
		m.press(k)
		if m.details {
			t.Errorf("%s closes the details", k)
		}
	}
	for i, l := range lines(m.Render()) {
		if lw := lipgloss.Width(l); lw > 150 {
			t.Errorf("line %d is %d wide", i, lw)
		}
	}
}
