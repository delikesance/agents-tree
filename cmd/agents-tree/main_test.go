package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type obj = map[string]any

func jline(v obj) string { b, _ := json.Marshal(v); return string(b) }

func asst(id, ts string, content []any, fresh, read int) string {
	return jline(obj{"type": "assistant", "timestamp": ts, "uuid": id, "cwd": "/work/demo", "message": obj{
		"id": id, "model": "claude-sonnet-5-5", "content": content,
		"usage": obj{"input_tokens": fresh, "cache_read_input_tokens": read, "cache_creation_input_tokens": 0, "output_tokens": 5}}})
}

func userLine(ts string, content any) string {
	return jline(obj{"type": "user", "timestamp": ts, "uuid": "u" + ts, "cwd": "/work/demo",
		"message": obj{"role": "user", "content": content}})
}

// smallSession is a hand-written session: one prompt, one Bash call with a big output, one reply.
func smallSession() []string {
	return []string{
		userLine("2026-10-09T10:00:00Z", "list the files please"),
		asst("m1", "2026-10-09T10:00:01Z", []any{
			obj{"type": "tool_use", "id": "t1", "name": "Bash", "input": obj{"command": "ls -la"}}}, 1000, 0),
		userLine("2026-10-09T10:00:02Z", []any{
			obj{"type": "tool_result", "tool_use_id": "t1", "content": strings.Repeat("file.txt\n", 200)}}),
		asst("m2", "2026-10-09T10:00:03Z", []any{obj{"type": "text", "text": "Here are the files."}}, 0, 1500),
		asst("m3", "2026-10-09T10:00:04Z", []any{obj{"type": "text", "text": "Anything else?"}}, 0, 1600),
	}
}

func writeFile(t *testing.T, path string, lines []string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func exec(t *testing.T, args ...string) (code int, out, errOut string) {
	t.Helper()
	var so, se bytes.Buffer
	code = run(args, &so, &se)
	return code, so.String(), se.String()
}

func TestUsageAndUnknownCommand(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"-h"}, {"help"}} {
		code, out, errOut := exec(t, args...)
		if code != 2 || out != "" || !strings.Contains(errOut, "agents-tree live") || !strings.Contains(errOut, "--permissions") {
			t.Errorf("run(%v) = %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
	code, _, errOut := exec(t, "frobnicate")
	if code != 2 || !strings.Contains(errOut, `unknown command "frobnicate"`) || !strings.Contains(errOut, "agents-tree replay") {
		t.Errorf("unknown command: %d %q", code, errOut)
	}
}

func TestLiveErrors(t *testing.T) {
	code, _, errOut := exec(t, "live", "/nonexistent.jsonl")
	if code != 1 || !strings.Contains(errOut, "session transcript not found") {
		t.Errorf("missing file: %d %q", code, errOut)
	}
	code, _, errOut = exec(t, "live", "--permissions", "bogus")
	if code != 2 || !strings.Contains(errOut, "--permissions must be") {
		t.Errorf("bad permissions: %d %q", code, errOut)
	}
	// the bad value is rejected before the path is looked at
	if code, _, _ = exec(t, "live", "--permissions=bogus", "/nonexistent.jsonl"); code != 2 {
		t.Errorf("bad permissions with path: %d", code)
	}
}

func TestReplayErrors(t *testing.T) {
	code, _, errOut := exec(t, "replay")
	if code != 2 || !strings.Contains(errOut, "replay needs a session") {
		t.Errorf("no path: %d %q", code, errOut)
	}
	code, _, errOut = exec(t, "replay", "/nonexistent.jsonl")
	if code != 1 || !strings.Contains(errOut, "session transcript not found") {
		t.Errorf("missing file: %d %q", code, errOut)
	}
	// a directory is not a transcript either
	if code, _, _ = exec(t, "replay", t.TempDir()); code != 1 {
		t.Errorf("directory: %d", code)
	}
}

// fakeHome points HOME at a temp dir and returns its ~/.claude/projects.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return filepath.Join(home, ".claude", "projects")
}

func TestSessionsNewestFirstWithTitle(t *testing.T) {
	proj := fakeHome(t)
	old := writeFile(t, filepath.Join(proj, "-work-alpha", "aaaaaaaa-1111.jsonl"), []string{
		jline(obj{"type": "user", "cwd": "/work/alpha", "message": obj{"role": "user", "content": "old alpha task"}})})
	recent := writeFile(t, filepath.Join(proj, "-work-beta", "bbbbbbbb-2222.jsonl"), []string{
		jline(obj{"type": "user", "cwd": "/work/beta", "message": obj{"role": "user", "content": "fresh beta task"}})})
	now := time.Now()
	if err := os.Chtimes(old, now.Add(-3*time.Hour), now.Add(-3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(recent, now.Add(-5*time.Minute), now.Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := exec(t, "sessions")
	if code != 0 || errOut != "" {
		t.Fatalf("sessions: %d %q", code, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %q", out)
	}
	if !strings.Contains(lines[0], "beta") || !strings.Contains(lines[0], "bbbbbbbb") || !strings.Contains(lines[0], "fresh beta task") || !strings.Contains(lines[0], "5 min ago") {
		t.Errorf("first line should be the newest session: %q", lines[0])
	}
	if !strings.Contains(lines[1], "alpha") || !strings.Contains(lines[1], "aaaaaaaa") || !strings.Contains(lines[1], "old alpha task") || !strings.Contains(lines[1], "3h ago") {
		t.Errorf("second line should be the oldest session: %q", lines[1])
	}
}

func TestSessionsEmpty(t *testing.T) {
	fakeHome(t)
	if code, out, _ := exec(t, "sessions"); code != 0 || out != "" {
		t.Errorf("no sessions: %d %q", code, out)
	}
}

func TestContextReport(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "s.jsonl"), smallSession())
	code, out, errOut := exec(t, "context", p)
	if code != 0 || errOut != "" {
		t.Fatalf("context: %d %q", code, errOut)
	}
	for _, want := range []string{
		p, "3 requests", "input cost ~$", "(estimate)",
		"context before the first message", "tool outputs", "code and commands written", "your messages", "assistant replies",
		"tool outputs by tool:", "Bash (1 calls",
		"Shrinking every tool output by 70% would remove about",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("context output lacks %q:\n%s", want, out)
		}
	}
}

func TestContextMissingFile(t *testing.T) {
	code, _, errOut := exec(t, "context", "/nonexistent.jsonl")
	if code != 1 || !strings.Contains(errOut, "session transcript not found") {
		t.Errorf("missing file: %d %q", code, errOut)
	}
}

func TestContextNoRequests(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "s.jsonl"), []string{userLine("2026-10-09T10:00:00Z", "hi")})
	code, out, _ := exec(t, "context", p)
	if code != 0 || !strings.Contains(out, "no API requests in this session yet") {
		t.Errorf("empty session: %d %q", code, out)
	}
}

func TestContextRecent(t *testing.T) {
	proj := fakeHome(t)
	writeFile(t, filepath.Join(proj, "-work-demo", "cccccccc-1.jsonl"), smallSession())
	writeFile(t, filepath.Join(proj, "-work-demo", "dddddddd-2.jsonl"), smallSession())
	// a session without any API request is skipped
	writeFile(t, filepath.Join(proj, "-work-demo", "eeeeeeee-3.jsonl"), []string{userLine("2026-10-09T10:00:00Z", "hi")})
	code, out, errOut := exec(t, "context", "--recent", "5")
	if code != 0 || errOut != "" {
		t.Fatalf("context --recent: %d %q", code, errOut)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("want header + 2 sessions, got %d lines:\n%s", len(lines), out)
	}
	for _, col := range []string{"session", "project", "requests", "input $", "context", "tool in", "tool out", "other"} {
		if !strings.Contains(lines[0], col) {
			t.Errorf("header lacks %q: %q", col, lines[0])
		}
	}
	for _, l := range lines[1:] {
		if !strings.Contains(l, "demo") || !strings.Contains(l, "%") {
			t.Errorf("session line: %q", l)
		}
	}
	if strings.Contains(out, "eeeeeeee") {
		t.Error("a session without requests should be skipped")
	}
}

type hookEvent struct {
	TS        float64 `json:"ts"`
	Kind      string  `json:"kind"`
	AgentID   string  `json:"agent_id"`
	Session   string  `json:"session"`
	AgentKind string  `json:"agent_kind"`
	Status    string  `json:"status"`
}

func readHookEvents(t *testing.T, path string) []hookEvent {
	t.Helper()
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var evs []hookEvent
	for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if l == "" {
			continue
		}
		var e hookEvent
		if err := json.Unmarshal([]byte(l), &e); err != nil {
			t.Fatalf("bad event line %q: %v", l, err)
		}
		evs = append(evs, e)
	}
	return evs
}

func TestHookAppendsEvents(t *testing.T) {
	file := filepath.Join(t.TempDir(), "events.jsonl")
	t.Setenv("AGENTS_TREE_EVENTS", file)
	var errOut bytes.Buffer
	send := func(payload string) {
		t.Helper()
		if code := runHook(strings.NewReader(payload), &errOut); code != 0 {
			t.Fatalf("hook returned %d for %q", code, payload)
		}
	}
	send(`{"hook_event_name":"SubagentStart","agent_id":"a1","agent_type":"explorer","session_id":"sess-1"}`)
	send(`{"hook_event_name":"SubagentStart","agent_id":"a2","session_id":"sess-1"}`) // no agent_type
	send(`{"hook_event_name":"SubagentStop","agent_id":"a1","session_id":"sess-1"}`)
	// all of these are ignored, silently
	send(`not json at all`)
	send(``)
	send(`{"hook_event_name":"PreToolUse","agent_id":"a3","session_id":"sess-1"}`)
	send(`{"hook_event_name":"SubagentStart","session_id":"sess-1"}`) // missing agent_id
	send(`{"hook_event_name":"SubagentStop","agent_id":""}`)

	evs := readHookEvents(t, file)
	if len(evs) != 3 {
		t.Fatalf("want 3 events, got %+v", evs)
	}
	if e := evs[0]; e.Kind != "agent_start" || e.AgentID != "a1" || e.AgentKind != "explorer" || e.Session != "sess-1" || e.TS <= 0 {
		t.Errorf("start: %+v", e)
	}
	if e := evs[1]; e.Kind != "agent_start" || e.AgentID != "a2" || e.AgentKind != "agent" {
		t.Errorf("start without agent_type should default to 'agent': %+v", e)
	}
	if e := evs[2]; e.Kind != "agent_end" || e.AgentID != "a1" || e.Status != "done" || e.Session != "sess-1" {
		t.Errorf("stop: %+v", e)
	}
	if errOut.Len() != 0 {
		t.Errorf("hook must stay silent, wrote %q", errOut.String())
	}
}

func TestHookDefaultFileUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("AGENTS_TREE_EVENTS", "")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	runHook(strings.NewReader(`{"hook_event_name":"SubagentStart","agent_id":"x","agent_type":"worker"}`), &bytes.Buffer{})
	evs := readHookEvents(t, filepath.Join(home, ".claude", "agents-tree-events.jsonl"))
	if len(evs) != 1 || evs[0].AgentID != "x" || evs[0].AgentKind != "worker" {
		t.Errorf("default hook file: %+v", evs)
	}
}

func TestHookUnwritableFileStillSucceeds(t *testing.T) {
	t.Setenv("AGENTS_TREE_EVENTS", filepath.Join(t.TempDir(), "missing-dir", "events.jsonl"))
	code := runHook(strings.NewReader(`{"hook_event_name":"SubagentStart","agent_id":"a1"}`), &bytes.Buffer{})
	if code != 0 {
		t.Errorf("hook must never fail the session, got %d", code)
	}
}

func TestRenderScreenSize(t *testing.T) {
	p := writeFile(t, filepath.Join(t.TempDir(), "s.jsonl"), smallSession())
	code, out, errOut := exec(t, "render", p, "--width", "100", "--height", "30")
	if code != 0 || errOut != "" {
		t.Fatalf("render: %d %q", code, errOut)
	}
	lines := strings.Split(strings.TrimSuffix(out, "\n"), "\n")
	if len(lines) != 30 {
		t.Errorf("want exactly 30 lines, got %d", len(lines))
	}
	if !strings.Contains(out, "list the files please") {
		t.Errorf("the prompt should be on screen:\n%s", out)
	}
}

func TestRenderNeedsAPath(t *testing.T) {
	if code, _, errOut := exec(t, "render"); code != 2 || !strings.Contains(errOut, "render needs a session") {
		t.Errorf("render without path: %d %q", code, errOut)
	}
	if code, _, _ := exec(t, "render", "/nonexistent.jsonl"); code != 1 {
		t.Errorf("render with missing file: %d", code)
	}
}
