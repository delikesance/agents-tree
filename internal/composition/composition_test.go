package composition

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type m = map[string]any

func line(v m) string { b, _ := json.Marshal(v); return string(b) }

func req(id, ts string, content []any, fresh, read, write int) string {
	return line(m{"type": "assistant", "timestamp": ts, "uuid": id, "message": m{"id": id, "model": "claude-sonnet-5-5",
		"content": content, "usage": m{"input_tokens": fresh, "cache_read_input_tokens": read,
			"cache_creation_input_tokens": write, "output_tokens": 1}}})
}
func user(content any) string {
	return line(m{"type": "user", "timestamp": "2026-10-09T10:00:00Z", "message": m{"role": "user", "content": content}})
}
func result(tid, text string) string {
	return user([]any{m{"type": "tool_result", "tool_use_id": tid, "content": text}})
}
func txt(s string) []any { return []any{m{"type": "text", "text": s}} }
func write(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "s.jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestVolumeIsSizeTimesTheRequestsThatComeAfter(t *testing.T) {
	// 4 requests. Request 1 calls Bash (compact input json = 40 chars = 10 tokens); its 400-char output
	// (100 tokens) is re-sent by requests 2, 3 and 4 -> volume 300.
	cmd := m{"command": strings.Repeat("x", 26)}
	p := write(t,
		req("m1", "2026-10-09T10:00:00Z", []any{m{"type": "tool_use", "id": "t1", "name": "Bash", "input": cmd}}, 1000, 0, 0),
		result("t1", strings.Repeat("y", 400)),
		req("m2", "2026-10-09T10:00:01Z", txt(strings.Repeat("a", 80)), 0, 1110, 0),
		user(strings.Repeat("u", 40)),
		req("m3", "2026-10-09T10:00:02Z", txt("done"), 0, 1250, 0),
		req("m4", "2026-10-09T10:00:03Z", txt("bye"), 0, 1260, 0),
	)
	c := Analyze(p, true)
	if c.Turns != 4 || c.PromptTokens != 1000+1110+1250+1260 {
		t.Fatalf("turns=%d prompt=%v", c.Turns, c.PromptTokens)
	}
	if c.BaselineTokens != 1000 || c.Baseline != 4000 {
		t.Errorf("baseline = %v / %v", c.BaselineTokens, c.Baseline)
	}
	if o := c.Outputs["Bash"]; o.N != 1 || o.Tokens != 100 || o.Volume != 300 {
		t.Errorf("outputs = %+v", o)
	}
	if in := c.Inputs["Bash"]; in.Tokens != 10 || in.Volume != 30 {
		t.Errorf("inputs = %+v", in)
	}
	if !near(c.AssistantText, 20*2+1*1) { // m2 (20 tok): 2 later; m3 ("done" = 1 tok): 1 later
		t.Errorf("assistant text = %v", c.AssistantText)
	}
	if c.UserText != 20 { // after m2: requests 3 and 4 re-send it
		t.Errorf("user text = %v", c.UserText)
	}
	if !near(c.Share(c.Baseline), 4000/c.PromptTokens) {
		t.Error("share")
	}
}

func TestRepeatedUsageLinesOfOneRequestAreOneRequest(t *testing.T) {
	p := write(t,
		req("m1", "2026-10-09T10:00:00Z", txt("a"), 100, 0, 0),
		req("m1", "2026-10-09T10:00:01Z", []any{m{"type": "tool_use", "id": "t", "name": "Read", "input": m{}}}, 100, 0, 0),
		result("t", strings.Repeat("z", 40)),
		req("m2", "2026-10-09T10:00:02Z", txt("b"), 0, 150, 0),
	)
	c := Analyze(p, true)
	if c.Turns != 2 || c.PromptTokens != 250 || c.Outputs["Read"].Volume != 10 {
		t.Errorf("turns=%d prompt=%v read volume=%v", c.Turns, c.PromptTokens, c.Outputs["Read"].Volume)
	}
}

func TestCostShareAndWhatIf(t *testing.T) {
	p := write(t,
		req("m1", "2026-10-09T10:00:00Z", []any{m{"type": "tool_use", "id": "t", "name": "Bash", "input": m{}}}, 1_000_000, 0, 0),
		result("t", strings.Repeat("q", 400_000)),
		req("m2", "2026-10-09T10:00:01Z", txt("ok"), 0, 1_100_000, 0),
	)
	c := Analyze(p, true)
	if !near(c.InputCost, 1_000_000*2/1e6+1_100_000*0.20/1e6) { // Sonnet 5.5 rates
		t.Errorf("input cost = %v", c.InputCost)
	}
	if !near(c.Share(c.OutputsVolume()), 100_000.0/2_100_000) {
		t.Errorf("output share = %v", c.Share(c.OutputsVolume()))
	}
	share, usd := c.WhatIfCompressOutputs(0.5)
	if !near(share, 0.5*100_000/2_100_000) || !near(usd, share*c.InputCost) {
		t.Errorf("what-if = %v %v", share, usd)
	}
}

func TestSubagentFilesAreSeparateConversations(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "s.jsonl")
	_ = os.WriteFile(main, []byte(req("m1", "2026-10-09T10:00:00Z", txt("x"), 500, 0, 0)+"\n"), 0o644)
	sub := filepath.Join(dir, "s", "subagents")
	_ = os.MkdirAll(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "agent-a1.jsonl"), []byte(req("s1", "2026-10-09T10:00:00Z", txt("y"), 300, 0, 0)+"\n"), 0o644)
	both, only := Analyze(main, true), Analyze(main, false)
	if both.Turns != 2 || both.PromptTokens != 800 || only.PromptTokens != 500 {
		t.Errorf("both=%+v only=%+v", both.PromptTokens, only.PromptTokens)
	}
}

func TestEmptyOrUnreadableSessionsDoNotCrash(t *testing.T) {
	p := write(t, "not json", "{}")
	c := Analyze(p, true)
	if s, u := c.WhatIfCompressOutputs(0.7); c.Turns != 0 || c.Share(10) != 0 || s != 0 || u != 0 {
		t.Error("empty session should be all zeros")
	}
	if len(New().Categories()) == 0 {
		t.Error("categories")
	}
	if Analyze(filepath.Join(t.TempDir(), "missing.jsonl"), true).Turns != 0 {
		t.Error("missing file")
	}
}
