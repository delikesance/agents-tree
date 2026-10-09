package store_test

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/pricing"
	"github.com/delikesance/agents-tree/internal/store"
	"github.com/delikesance/agents-tree/internal/transcript"
)

type m = map[string]any

func line(kv m) string {
	if _, ok := kv["timestamp"]; !ok {
		kv["timestamp"] = "2026-10-09T10:00:00Z"
	}
	b, _ := json.Marshal(kv)
	return string(b)
}

// session writes a main transcript (and optional subagent files) and returns its path.
func session(t *testing.T, lines []string, sub map[string][2]any) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sess.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, v := range sub {
		d := filepath.Join(dir, "sess", "subagents")
		_ = os.MkdirAll(d, 0o755)
		meta, _ := json.Marshal(v[0])
		_ = os.WriteFile(filepath.Join(d, "agent-"+name+".meta.json"), meta, 0o644)
		_ = os.WriteFile(filepath.Join(d, "agent-"+name+".jsonl"), []byte(strings.Join(v[1].([]string), "\n")+"\n"), 0o644)
	}
	return p
}

func build(t *testing.T, path string) *store.Store {
	t.Helper()
	evs, err := transcript.ReadSession(path)
	if err != nil {
		t.Fatal(err)
	}
	s := store.New()
	for _, e := range evs {
		s.Apply(e)
	}
	return s
}

func usage(out int) m { return m{"output_tokens": out} }

func assistant(id, ts string, content ...m) string {
	c := make([]any, len(content))
	for i, b := range content {
		c[i] = b
	}
	return line(m{"type": "assistant", "uuid": id, "timestamp": ts, "message": m{
		"id": id, "model": "claude-sonnet-5-5", "usage": usage(1), "content": c}})
}

func toolUse(id, name string, in m) m {
	return m{"type": "tool_use", "id": id, "name": name, "input": in}
}
func text(s string) m { return m{"type": "text", "text": s} }
func result(ts, tid string, content any, isErr bool) string {
	return line(m{"type": "user", "timestamp": ts, "message": m{"content": []any{
		m{"type": "tool_result", "tool_use_id": tid, "content": content, "is_error": isErr}}}})
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func msgByID(s *store.Store) map[string]*model.Message {
	out := map[string]*model.Message{}
	for _, x := range s.Messages {
		out[x.ID] = x
	}
	return out
}

func TestCleanTextRemovesRemindersTagsAndFormatsCommands(t *testing.T) {
	cases := map[string]string{
		"hello <system-reminder>secret ctx</system-reminder> world":                   "hello  world",
		"<command-message>init</command-message>\n<command-name>/init</command-name>": "/init",
		"<command-name>/design</command-name><command-args>make it</command-args>":    "/design make it",
		"a <b>bold</b> b": "a bold b",
	}
	for in, want := range cases {
		if got := transcript.CleanText(in); got != want {
			t.Errorf("CleanText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToolDetailIsShortAndSpecific(t *testing.T) {
	cases := []struct {
		name string
		in   m
		want string
	}{
		{"Bash", m{"command": "pytest -q\nsecond line"}, "pytest -q"},
		{"Bash", m{"command": "python3 - <<'E'\nimport json\nprint(1)\nE"}, "python3 - <<'E' ⏎ import json"},
		{"Read", m{"file_path": "/home/user/proj/pkg/mod.py"}, "proj/pkg/mod.py"},
		{"Grep", m{"pattern": "TODO", "path": "src"}, "TODO  in src"},
		{"mcp__x__do", m{"a": 1, "b": "first string"}, "first string"},
	}
	for _, c := range cases {
		if got := transcript.ToolDetail(c.name, c.in); got != c.want {
			t.Errorf("ToolDetail(%s) = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestUserSystemAndInterruptRoles(t *testing.T) {
	p := session(t, []string{
		line(m{"type": "user", "uuid": "u1", "origin": m{"kind": "human"}, "turnOrigin": "human",
			"message": m{"role": "user", "content": "<command-name>/init</command-name>"}}),
		line(m{"type": "user", "uuid": "u2", "isMeta": true, "message": m{"role": "user", "content": []any{text("Please analyze this codebase")}}}),
		line(m{"type": "user", "uuid": "u3", "origin": m{"kind": "human"}, "message": m{"role": "user", "content": []any{
			text("Look at this <system-reminder>noise</system-reminder>"), m{"type": "image", "source": m{}}}}}),
		line(m{"type": "user", "uuid": "u4", "message": m{"role": "user", "content": "[Request interrupted by user]"}}),
		line(m{"type": "user", "uuid": "u5", "isMeta": true, "message": m{"role": "user", "content": "Stop hook feedback: x"}}),
	}, nil)
	s := build(t, p)
	want := [][2]string{{"user", "/init"}, {"system", "Please analyze this codebase"}, {"user", "Look at this  ▣ image"},
		{"system", "[Request interrupted by user]"}, {"system", "Stop hook feedback: x"}}
	if len(s.Messages) != len(want) {
		t.Fatalf("got %d messages", len(s.Messages))
	}
	for i, w := range want {
		if s.Messages[i].Role != w[0] || s.Messages[i].Text != w[1] {
			t.Errorf("message %d = (%s, %q), want %v", i, s.Messages[i].Role, s.Messages[i].Text, w)
		}
	}
}

func TestAssistantTextAndToolPairingWithDurationAndStatus(t *testing.T) {
	p := session(t, []string{
		assistant("a1", "2026-10-09T10:00:00Z", m{"type": "thinking", "thinking": ""}, text("Running the tests."),
			toolUse("t1", "Bash", m{"command": "pytest -q"}), toolUse("t2", "Read", m{"file_path": "/a/b/c.py"})),
		result("2026-10-09T10:00:03Z", "t1", "1 failed", true),
		result("2026-10-09T10:00:01Z", "t2", "ok", false),
	}, nil)
	msgs := msgByID(build(t, p))
	if a := msgs["a1:1"]; a == nil || a.Role != "assistant" || a.Model != "claude-sonnet-5-5" {
		t.Fatalf("assistant message = %+v", a)
	}
	t1, t2 := msgs["t1"], msgs["t2"]
	if t1.Tool != "Bash" || t1.Detail != "pytest -q" || t1.Status != "error" || t1.Duration == nil || *t1.Duration != 3 {
		t.Errorf("t1 = %+v", t1)
	}
	if t2.Status != "ok" || *t2.Duration != 1 || t2.Detail != "a/b/c.py" || t1.Rev != 1 {
		t.Errorf("t2 = %+v rev(t1)=%d", t2, t1.Rev)
	}
}

func TestUnansweredToolStaysRunning(t *testing.T) {
	p := session(t, []string{assistant("a", "2026-10-09T10:00:00Z", toolUse("t", "Bash", m{"command": "sleep 99"}))}, nil)
	if got := build(t, p).Messages[0].Status; got != "running" {
		t.Errorf("status = %s", got)
	}
}

func TestDelegationReportAndSubagentPromptNotDuplicated(t *testing.T) {
	main := []string{
		assistant("a", "2026-10-09T10:00:00Z", toolUse("ag1", "Agent", m{"subagent_type": "explorer", "description": "map the repo", "prompt": "List the files."})),
		line(m{"type": "user", "timestamp": "2026-10-09T10:00:08Z", "toolUseResult": m{"agentId": "a99"}, "message": m{"content": []any{
			m{"type": "tool_result", "tool_use_id": "ag1", "content": []any{text("Found 3 files.\nagentId: a99")}}}}}),
	}
	sub := map[string][2]any{"a99": {m{"toolUseId": "ag1"}, []string{
		line(m{"type": "user", "isSidechain": true, "uuid": "s0", "message": m{"role": "user", "content": "List the files."}}),
		line(m{"type": "assistant", "isSidechain": true, "uuid": "s1", "timestamp": "2026-10-09T10:00:02Z", "message": m{
			"id": "s1", "model": "claude-haiku-5-5", "usage": usage(1), "content": []any{text("Looking."),
				toolUse("st1", "Bash", m{"command": "ls"}), toolUse("hb", "SubagentHandback", m{})}}}),
	}}}
	s := build(t, session(t, main, sub))
	msgs := msgByID(s)
	if msgs["deleg:ag1"] == nil || msgs["rep:ag1"] == nil {
		t.Fatal("missing delegation or report")
	}
	if msgs["s0"] != nil || msgs["hb"] != nil {
		t.Error("the delegated prompt and the hand-back must not appear as messages")
	}
	d := msgs["deleg:ag1"]
	if d.Kind != "explorer" || d.Desc != "map the repo" || d.Text != "List the files." || d.Target != "ag1" || d.Status != "done" || *d.Duration != 8 {
		t.Errorf("delegation = %+v", d)
	}
	if r := msgs["rep:ag1"]; r.Text != "Found 3 files." || r.Status != "done" {
		t.Errorf("report = %+v", r)
	}
	if n := s.Get(msgs["st1"].AgentID); n == nil || n.ID != "ag1" {
		t.Errorf("subagent message resolves to %v", n)
	}
}

func TestFailedAgentReport(t *testing.T) {
	p := session(t, []string{
		assistant("a", "2026-10-09T10:00:00Z", toolUse("ag", "Task", m{"subagent_type": "worker", "prompt": "x"})),
		result("2026-10-09T10:00:05Z", "ag", "boom", true),
	}, nil)
	r := msgByID(build(t, p))["rep:ag"]
	if r == nil || r.Status != "failed" || r.Text != "boom" {
		t.Errorf("report = %+v", r)
	}
}

func TestStoreDedupesAndCapsMessages(t *testing.T) {
	s := store.New()
	add := func(id, txt string) {
		s.Apply(model.Event{Kind: model.Message_, AgentID: model.Main, Msg: &model.Message{ID: id, AgentID: model.Main, Role: "assistant", Text: txt}})
	}
	add("same", "x")
	add("same", "dup")
	if len(s.Messages) != 1 || s.Messages[0].Text != "x" {
		t.Fatalf("dedupe failed: %+v", s.Messages)
	}
	s.Apply(model.Event{Kind: model.MsgUpdate, MsgID: "nope", Status: "ok"}) // unknown id: ignored
	for i := 0; i < store.MaxMessages+50; i++ {
		add("m"+itoa(i), "t")
	}
	if len(s.Messages) != store.MaxMessages || s.Messages[len(s.Messages)-1].ID != "m"+itoa(store.MaxMessages+49) {
		t.Errorf("cap failed: %d", len(s.Messages))
	}
	s.Apply(model.Event{Kind: model.MsgUpdate, MsgID: "m0", Status: "ok"}) // trimmed id: ignored
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func turn(agent string, ts float64, mod string, u model.Usage, tool string, sidechain bool) model.Event {
	return model.Event{TS: ts, Kind: model.Turn, AgentID: agent, Model: mod, Usage: &u, Tool: tool, Sidechain: sidechain}
}

func TestTreeShapeStatusAndSidechainAliases(t *testing.T) {
	main := []string{
		assistant("a", "2026-10-09T10:00:04Z", toolUse("t_exp", "Agent", m{"subagent_type": "explorer", "description": "map"}),
			toolUse("t_wrk", "Agent", m{"subagent_type": "worker", "description": "edit"})),
		line(m{"type": "user", "timestamp": "2026-10-09T10:00:09Z", "toolUseResult": m{"agentId": "a1"}, "message": m{"content": []any{
			m{"type": "tool_result", "tool_use_id": "t_exp", "content": "ok"}}}}),
		result("2026-10-09T10:00:12Z", "t_wrk", "boom", true),
	}
	sub := map[string][2]any{"a1": {m{"toolUseId": "t_exp"}, []string{
		line(m{"type": "assistant", "isSidechain": true, "timestamp": "2026-10-09T10:00:05Z", "message": m{"id": "x1", "model": "claude-haiku-5-5",
			"usage": m{"input_tokens": 100, "output_tokens": 200}, "content": []any{}}}),
		line(m{"type": "assistant", "isSidechain": true, "timestamp": "2026-10-09T10:00:06Z", "message": m{"id": "x2", "model": "claude-haiku-5-5",
			"usage": m{"input_tokens": 100, "output_tokens": 200}, "content": []any{}}}),
	}}}
	s := build(t, session(t, main, sub))
	kids := s.Nodes[model.Main].Children
	if len(kids) != 2 || s.Nodes[kids[0]].Kind != "explorer" || s.Nodes[kids[0]].Status != "done" ||
		s.Nodes[kids[1]].Kind != "worker" || s.Nodes[kids[1]].Status != "failed" {
		t.Fatalf("tree = %v", kids)
	}
	if r, tot := s.RunningSubagents(0); r != 0 || tot != 2 {
		t.Errorf("running = %d/%d", r, tot)
	}
	e := s.Get("a1")
	if e.Kind != "explorer" || e.Turns != 2 || e.TokensIn != 200 || e.TokensOut != 400 || e.Cost <= 0 {
		t.Errorf("explorer = %+v", e)
	}
}

func TestMetaAliasBindsBeforeNodeExists(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 0, Kind: model.Alias, AgentID: "tool1", AliasID: "agentX"})
	s.Apply(model.Event{TS: 1, Kind: model.AgentStart, AgentID: "tool1", ParentID: model.Main, AgentKind: "worker"})
	s.Apply(turn("agentX", 2, "claude-haiku-5-5", model.Usage{Output: 5}, "", true))
	if s.Nodes["tool1"].Turns != 1 {
		t.Error("turn did not reach the aliased node")
	}
}

func TestHookStartMergesWithTranscriptNode(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1, Kind: model.AgentStart, AgentID: "tool1", ParentID: model.Main, AgentKind: "worker"})
	s.Apply(model.Event{TS: 2, Kind: model.AgentStart, AgentID: "hook-agent", AgentKind: "worker", MatchKind: "worker"})
	if len(s.Nodes) != 2 {
		t.Fatalf("nodes = %d, want main + one worker", len(s.Nodes))
	}
	s.Apply(model.Event{TS: 3, Kind: model.AgentEnd, AgentID: "hook-agent", Status: "done"})
	if s.Nodes["tool1"].Status != "done" {
		t.Error("hook end did not finish the merged node")
	}
}

func TestSilentAgentIsNotShownAsRunning(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{TS: 1000, Kind: model.AgentStart, AgentID: "w", ParentID: model.Main, AgentKind: "worker"})
	w := s.Nodes["w"]
	if s.State(w, 0) != "running" || s.State(w, 1000+store.StaleSecs+1) != "stale" {
		t.Error("stale detection failed")
	}
	s.Apply(turn("w", 1000+store.StaleSecs+5, "", model.Usage{Output: 1}, "", false))
	if s.State(w, 0) != "running" {
		t.Error("activity must revive the agent")
	}
	if r, tot := s.RunningSubagents(1000 + 10*store.StaleSecs); r != 0 || tot != 1 {
		t.Errorf("running = %d/%d", r, tot)
	}
	s.Apply(model.Event{TS: 1000 + 11*store.StaleSecs, Kind: model.AgentEnd, AgentID: "w", Status: "done"})
	if s.State(w, 1e10) != "done" {
		t.Error("finished agents never go stale")
	}
}

func TestActiveViewListsOnlyRunningAndKeepsAncestors(t *testing.T) {
	s := store.New()
	t0 := 10_000.0
	for _, k := range []string{"done_one", "failed_one", "silent_one", "busy_one"} {
		s.Apply(model.Event{TS: t0, Kind: model.AgentStart, AgentID: k, ParentID: model.Main, AgentKind: k})
	}
	s.Apply(model.Event{TS: t0 + 5, Kind: model.AgentEnd, AgentID: "done_one", Status: "done"})
	s.Apply(model.Event{TS: t0 + 5, Kind: model.AgentEnd, AgentID: "failed_one", Status: "failed"})
	s.Apply(turn("busy_one", t0+store.StaleSecs+50, "", model.Usage{Output: 1}, "Bash", false))
	now := t0 + store.StaleSecs + 60
	if got := s.VisibleChildren(model.Main, now, false); len(got) != 1 || got[0] != "busy_one" {
		t.Errorf("active = %v", got)
	}
	if got := s.VisibleChildren(model.Main, now, true); len(got) != 4 {
		t.Errorf("all = %v", got)
	}
	// a running child keeps its finished parent visible
	s2 := store.New()
	s2.Apply(model.Event{TS: 1, Kind: model.AgentStart, AgentID: "parent", ParentID: model.Main, AgentKind: "dispatcher"})
	s2.Apply(model.Event{TS: 2, Kind: model.AgentStart, AgentID: "child", ParentID: "parent", AgentKind: "worker"})
	s2.Apply(model.Event{TS: 3, Kind: model.AgentEnd, AgentID: "parent", Status: "done"})
	if got := s2.VisibleChildren(model.Main, 4, false); len(got) != 1 || got[0] != "parent" {
		t.Errorf("parent hidden: %v", got)
	}
	s2.Apply(model.Event{TS: 5, Kind: model.AgentEnd, AgentID: "child", Status: "done"})
	if got := s2.VisibleChildren(model.Main, 6, false); len(got) != 0 {
		t.Errorf("nothing should be visible: %v", got)
	}
}

func TestUsageRepeatedOnEveryBlockLineIsCountedOnce(t *testing.T) {
	u := m{"input_tokens": 10, "cache_read_input_tokens": 1000, "output_tokens": 50}
	mk := func(id, ts string, c m) string {
		return line(m{"type": "assistant", "timestamp": ts, "message": m{"id": id, "model": "claude-sonnet-5-5", "usage": u, "content": []any{c}}})
	}
	p := transcript.NewParser(model.Main, false)
	s := store.New()
	for _, l := range []string{
		mk("msg_1", "2026-10-09T10:00:00Z", m{"type": "thinking", "thinking": ""}),
		mk("msg_1", "2026-10-09T10:00:01Z", text("ok")),
		mk("msg_1", "2026-10-09T10:00:02Z", toolUse("t1", "Bash", m{"command": "ls"})),
		mk("msg_2", "2026-10-09T10:00:09Z", text("next")),
	} {
		var d map[string]any
		_ = json.Unmarshal([]byte(l), &d)
		for _, e := range p.Parse(d) {
			s.Apply(e)
		}
	}
	main := s.Nodes[model.Main]
	if main.Turns != 2 || main.TokensOut != 100 || main.CacheRead != 2000 || main.Activity != "" {
		t.Errorf("main = turns %d out %d read %d activity %q", main.Turns, main.TokensOut, main.CacheRead, main.Activity)
	}
	one, _ := pricing.TurnCost("claude-sonnet-5-5", 10, 0, 0, 1000, 50)
	if !near(main.Cost, 2*one.Total()) {
		t.Errorf("cost = %v", main.Cost)
	}
}

func TestStoreSplitsCacheTTLsAndHitRate(t *testing.T) {
	s := store.New()
	s.Apply(turn(model.Main, 1, "claude-opus-5-5", model.Usage{Input: 1000, Output: 200, CacheRead: 90_000, CacheCreation: 9_000, Cache1h: 6_000}, "", false))
	sm := s.Summary()
	if sm.Fresh != 1000 || sm.Write != 9000 || sm.Read != 90_000 || sm.Out != 200 {
		t.Fatalf("summary = %+v", sm)
	}
	if hr, _ := sm.HitRate(); !near(hr, 0.9) {
		t.Errorf("hit rate = %v", hr)
	}
	want := (1000*4 + 3000*4*1.25 + 6000*4*2 + 90_000*0.20 + 200*20) / 1e6
	if !near(sm.Cost.Total(), want) {
		t.Errorf("cost = %v, want %v", sm.Cost.Total(), want)
	}
}

func TestUnpricedTurnsAreExcludedAndCounted(t *testing.T) {
	s := store.New()
	s.Apply(turn(model.Main, 1, "mystery-model", model.Usage{Input: 500}, "", false))
	sm := s.Summary()
	if sm.Unpriced != 1 || sm.Cost.Total() != 0 || sm.Prompt() != 500 || s.Nodes[model.Main].CostParts != nil {
		t.Errorf("summary = %+v", sm)
	}
}

func TestAdvisorAndJev(t *testing.T) {
	s := store.New()
	s.Apply(model.Event{Kind: model.Advisor, Model: "claude-opus-5-5"})
	s.Apply(model.Event{Kind: model.Advisor, Advice: "verify auth", NoCount: true})
	c := 0.86
	s.Apply(model.Event{Kind: model.Jev, Decision: "jev_which_file", Confidence: &c})
	if s.Advisor.Calls != 1 || s.Advisor.LastAdvice != "verify auth" || s.Advisor.Model != "claude-opus-5-5" {
		t.Errorf("advisor = %+v", s.Advisor)
	}
	if v, ok := s.Jev.AvgConfidence("jev_which_file"); !ok || !near(v, 0.86) || s.Jev.Forks != 1 {
		t.Errorf("jev = %+v", s.Jev)
	}
}
