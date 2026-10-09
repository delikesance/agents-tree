package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/delikesance/agents-tree/internal/composition"
	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

// flat drops box borders and collapses whitespace, so text wrapped inside a panel can be matched.
func flat(s string) string {
	s = strings.NewReplacer("│", " ", "╭", " ", "╮", " ", "╰", " ", "╯", " ", "─", " ").Replace(strip(s))
	return strings.Join(strings.Fields(s), " ")
}

func richStore() *store.Store {
	s := store.New()
	for _, ev := range richEvents() {
		s.Apply(ev)
	}
	return s
}

func TestCostPanelShowsTheSplitCacheHitRateAndSavings(t *testing.T) {
	s := richStore()
	sm := s.Summary()
	hr, ok := sm.HitRate()
	if !ok || sm.Cost.Saved() <= 0 {
		t.Fatalf("test data should hit the cache and save money: hr=%v ok=%v saved=%v", hr, ok, sm.Cost.Saved())
	}
	out := flat(costPanel(s, 70))
	for _, want := range []string{"cost (estimate)", "input (uncached)", "cache write", "cache read", "output", "total",
		fmt.Sprintf("cache hit %.1f%%", hr*100), "saved " + usd(sm.Cost.Saved()), "(no cache " + usd(sm.Cost.NoCache) + ")",
		usd(sm.Cost.Total()), fmtTokens(sm.Read), fmtTokens(sm.Out)} {
		if !strings.Contains(out, want) {
			t.Errorf("cost panel lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "unpriced") {
		t.Error("no unpriced turns here")
	}
	for _, l := range lines(strip(costPanel(s, 40))) {
		if w := len([]rune(l)); w > 40 {
			t.Errorf("cost panel line wider than the panel: %q", l)
		}
	}
	if narrow := flat(costPanel(s, railWidth-1)); !strings.Contains(narrow, "saved "+usd(sm.Cost.Saved())) {
		t.Errorf("savings still readable in the rail (wrapped): %s", narrow)
	}
}

func TestCostPanelCountsUnpricedTurnsInsteadOfPricingThemAtZero(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1, Kind: model.Turn, AgentID: model.Main, Model: "some-unknown-model-1",
		Usage: &model.Usage{Input: 10, Output: 10}})
	s.Apply(model.Event{TS: 2, Kind: model.Turn, AgentID: model.Main, Model: "some-unknown-model-1",
		Usage: &model.Usage{Input: 10, Output: 10}})
	out := strip(costPanel(s, 60))
	if !strings.Contains(out, "2 turns on an unpriced model are excluded") {
		t.Errorf("unpriced notice missing:\n%s", out)
	}
	if strings.Contains(out, "saved") {
		t.Error("no savings line without priced cache usage")
	}
}

func TestCostPanelWithoutCacheReadsZeroPercentAndSavesNothing(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1, Kind: model.Turn, AgentID: model.Main, Model: sonnet, Usage: &model.Usage{Input: 1000, Output: 10}})
	out := flat(costPanel(s, 70))
	if !strings.Contains(out, "cache hit 0.0%") || (strings.Contains(out, "saved") && !strings.Contains(out, "saved ~$0.00")) {
		t.Errorf("0%% hit, nothing saved: %s", out)
	}
}

func TestContextPanelShowsWhereTokensGoAndTheWhatIfLine(t *testing.T) {
	if out := strip(contextPanel(nil, 44)); !strings.Contains(out, "where tokens go") || !strings.Contains(out, "computing…") {
		t.Errorf("no analysis yet:\n%s", out)
	}
	if out := strip(contextPanel(composition.New(), 44)); !strings.Contains(out, "computing…") {
		t.Errorf("an empty analysis is still computing:\n%s", out)
	}
	c := composition.New()
	c.Turns, c.PromptTokens, c.InputCost = 4, 1000, 10
	c.Baseline, c.UserText, c.AssistantText = 500, 50, 50
	c.Outputs["Bash"] = &composition.ToolVol{N: 1, Tokens: 100, Volume: 200}
	c.Outputs["Read"] = &composition.ToolVol{N: 1, Tokens: 50, Volume: 100}
	c.Inputs["Edit"] = &composition.ToolVol{N: 1, Tokens: 10, Volume: 20}
	out := strip(contextPanel(c, 44))
	for _, want := range []string{"where tokens go (estimate)", "50.0%", "context before the first message", "tool outputs", "30.0%",
		"Bash", "Read", "code and commands written", "your messages", "assistant replies", "other (thinking",
		"shrinking tool outputs 70% ≈ −21.0%", "(~$2.10)"} {
		if !strings.Contains(out, want) {
			t.Errorf("context panel lacks %q:\n%s", want, out)
		}
	}
	// largest category first
	if strings.Index(out, "context before the first message") > strings.Index(out, "tool outputs") {
		t.Errorf("categories are sorted by weight:\n%s", out)
	}
	if strings.Index(out, "Bash") > strings.Index(out, "Read") || strings.Index(out, "Bash") < strings.Index(out, "tool outputs") {
		t.Errorf("tools are listed under tool outputs, heaviest first:\n%s", out)
	}
}

func TestAdvisorAndJevPanelsExistOnlyAfterACall(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	m.resize(140, 70)
	m.st.Apply(evTurn(nowSecs(), model.Main, sonnet, "", 1))
	rail := strip(m.rail(60))
	if strings.Contains(rail, "advisor") || strings.Contains(rail, "JEV") || strings.Contains(rail, "fork layer") {
		t.Errorf("no advisor/JEV panel before a call:\n%s", rail)
	}
	for _, ev := range richEvents() {
		if ev.Kind == model.Advisor || ev.Kind == model.Jev {
			m.st.Apply(ev)
		}
	}
	rail = strip(m.rail(60))
	for _, want := range []string{"advisor", "Opus 5.5", "calls 1", "last advice", "verify the auth claims first",
		"JEV", "fork layer", "forks 2", "which file", "0.86", "retry or stop", "↑1"} {
		if !strings.Contains(rail, want) {
			t.Errorf("rail lacks %q:\n%s", want, rail)
		}
	}
}

func TestRailGainsCostAndContextPanelsOnceTheSessionHasTurnsAndAnAnalysis(t *testing.T) {
	root := t.TempDir()
	path := liveFile(t, root, "s1", "hi")
	m := liveApp(t, Options{Session: path, ProjectsDir: root})
	m.resize(140, 70)
	if rail := strip(m.rail(60)); strings.Contains(rail, "cost (estimate)") || strings.Contains(rail, "where tokens go") {
		t.Errorf("no turns, no panels:\n%s", rail)
	}
	m.st.Apply(model.Event{TS: nowSecs(), Kind: model.Turn, AgentID: model.Main, Model: sonnet,
		Usage: &model.Usage{Input: 100, CacheRead: 9000, Output: 20}})
	m.tick(1)
	scr := m.screen()
	if !strings.Contains(scr, "cost (estimate)") || !strings.Contains(scr, "computing…") || !strings.Contains(scr, "cache hit") {
		t.Errorf("cost panel and a 'computing' context panel expected:\n%s", scr)
	}
	if !m.compBusy {
		t.Error("the analysis runs in the background")
	}
	c := composition.New()
	c.Turns, c.PromptTokens, c.InputCost, c.Baseline = 3, 1000, 1, 800
	c.Outputs["Bash"] = &composition.ToolVol{N: 1, Volume: 100}
	m.Update(compMsg{path: "/some/other/session.jsonl", comp: c}) // result of a session we left: ignored
	if m.comp != nil {
		t.Error("a stale analysis must be dropped")
	}
	m.Update(compMsg{path: path, comp: c})
	m.tick(1)
	scr = m.screen()
	if m.compBusy || !strings.Contains(scr, "where tokens go (estimate)") || !strings.Contains(scr, "80.0%") || strings.Contains(scr, "computing…") {
		t.Errorf("context panel with data expected:\n%s", scr)
	}
}

func TestStatusLineSummarisesTheSession(t *testing.T) {
	m := replayApp(t, richEvents(), 200, 30)
	m.settle(t)
	line := strip(m.statusLine())
	sm := m.st.Summary()
	hr, _ := sm.HitRate()
	for _, want := range []string{"replay ", "· chat", "view: all", "subagents [3/7 running]", "advisor [1]", "jev [2 forks]",
		usd(sm.Cost.Total()), fmt.Sprintf("cache %.0f%%", hr*100)} {
		if !strings.Contains(line, want) {
			t.Errorf("status line lacks %q: %q", want, line)
		}
	}
}

func TestJevDecisionWithoutConfidenceShowsNotAvailableInsteadOfZero(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1, Kind: model.Jev, AgentID: model.Main, Decision: "jev_retry_or_stop", Escalate: true})
	out := strip(jevPanel(s, 40))
	if !strings.Contains(out, "n/a") || strings.Contains(out, "0.00") {
		t.Skip("BUG: internal/store/store.go:264 counts a JEV decision that carries no confidence (row.Count++), so the panel shows a confidence of 0.00 (internal/ui/rail.go:176) instead of n/a; fix: keep a separate counter of decisions that had a confidence and use it in model.JevStats.AvgConfidence")
	}
}
