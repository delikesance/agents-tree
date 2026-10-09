package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func hkLines(n int, prefix string) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "%s %d: compiling package with a fairly long descriptive line\n", prefix, i)
	}
	return sb.String()
}

func hkPayload(t *testing.T, v map[string]any) string {
	t.Helper()
	base := map[string]any{"session_id": "sess-1", "transcript_path": "/x/y.jsonl", "cwd": "/w", "hook_event_name": "PostToolUse", "tool_use_id": "toolu_1"}
	for k, x := range v {
		base[k] = x
	}
	b, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func hkEnv(t *testing.T) (outDir, logPath string) {
	t.Helper()
	d := t.TempDir()
	outDir, logPath = filepath.Join(d, "outputs"), filepath.Join(d, "log.jsonl")
	t.Setenv("AGENTS_TREE_OUTPUT_DIR", outDir)
	t.Setenv("AGENTS_TREE_COMPRESS_LOG", logPath)
	t.Setenv("AGENTS_TREE_COMPRESS_MIN", "2000")
	return
}

func hkCompress(t *testing.T, stdin string) (int, string) {
	t.Helper()
	var out bytes.Buffer
	code := runCompress(strings.NewReader(stdin), &out)
	return code, out.String()
}

type hkReply struct {
	HookSpecificOutput struct {
		HookEventName     string          `json:"hookEventName"`
		UpdatedToolOutput json.RawMessage `json:"updatedToolOutput"`
	} `json:"hookSpecificOutput"`
}

func TestCompressBashReplacesOutputKeepingShape(t *testing.T) {
	_, logPath := hkEnv(t)
	long := hkLines(400, "go build")
	in := hkPayload(t, map[string]any{
		"tool_name":     "Bash",
		"tool_input":    map[string]any{"command": "go build ./... # SECRET-TOKEN-123", "description": "build"},
		"tool_response": map[string]any{"stdout": long, "stderr": "", "interrupted": false, "isImage": false},
		"agent_id":      "a1b2", "agent_type": "worker",
	})
	code, out := hkCompress(t, in)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var r hkReply
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, out)
	}
	if r.HookSpecificOutput.HookEventName != "PostToolUse" {
		t.Errorf("event %q", r.HookSpecificOutput.HookEventName)
	}
	var tool map[string]any
	if err := json.Unmarshal(r.HookSpecificOutput.UpdatedToolOutput, &tool); err != nil {
		t.Fatal(err)
	}
	if len(tool) != 4 || tool["stderr"] != "" || tool["interrupted"] != false || tool["isImage"] != false {
		t.Errorf("shape changed: %v", tool)
	}
	so, _ := tool["stdout"].(string)
	if !strings.Contains(so, "lines omitted (full output: ") || len(so) >= len(long)/2 {
		t.Errorf("stdout not compressed (%d bytes)", len(so))
	}
	// log: sizes + tool name only
	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SECRET-TOKEN") || strings.Contains(string(raw), "compiling") {
		t.Errorf("log leaks content or command: %s", raw)
	}
	var e map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &e); err != nil {
		t.Fatal(err)
	}
	if e["tool"] != "Bash" || e["session"] != "sess-1" || e["agent"] != "worker" || e["bytes_in"].(float64) <= e["bytes_out"].(float64) || e["saved_path"] == nil {
		t.Errorf("log entry %v", e)
	}
}

func TestCompressMCPAndReadShapes(t *testing.T) {
	hkEnv(t)
	long := hkLines(300, "row")
	cases := map[string]map[string]any{
		"mcp content array": {"tool_name": "mcp__github__get_file_contents", "tool_input": map[string]any{},
			"tool_response": []any{map[string]any{"type": "text", "text": long}}},
		"mcp string": {"tool_name": "mcp__x__y", "tool_input": map[string]any{}, "tool_response": long},
		"grep":       {"tool_name": "Grep", "tool_input": map[string]any{"pattern": "x"}, "tool_response": map[string]any{"mode": "content", "numFiles": 2, "content": long}},
	}
	for name, p := range cases {
		code, out := hkCompress(t, hkPayload(t, p))
		if code != 0 || out == "" {
			t.Errorf("%s: code=%d out=%q", name, code, out)
			continue
		}
		var r hkReply
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		var orig, upd any
		b, _ := json.Marshal(p["tool_response"])
		_ = json.Unmarshal(b, &orig)
		_ = json.Unmarshal(r.HookSpecificOutput.UpdatedToolOutput, &upd)
		if reflect.TypeOf(orig) != reflect.TypeOf(upd) {
			t.Errorf("%s: type changed %T -> %T", name, orig, upd)
		}
	}
	// Read: cleaned only, content keeps every line
	p := map[string]any{"tool_name": "Read", "tool_input": map[string]any{"file_path": "/a.go"},
		"tool_response": map[string]any{"type": "text", "file": map[string]any{"filePath": "/a.go", "content": "\x1b[0m" + strings.Repeat("\x1b[1m", 400) + long, "numLines": 300}}}
	_, out := hkCompress(t, hkPayload(t, p))
	var r hkReply
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("Read: %v (%q)", err, out)
	}
	if strings.Contains(string(r.HookSpecificOutput.UpdatedToolOutput), "omitted") || strings.Contains(string(r.HookSpecificOutput.UpdatedToolOutput), "\\u001b") {
		t.Error("Read output truncated or still has ANSI")
	}
}

func TestCompressStaysSilent(t *testing.T) {
	_, logPath := hkEnv(t)
	long := hkLines(400, "x")
	bash := func(cmd string) map[string]any {
		return map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": cmd}, "tool_response": map[string]any{"stdout": long, "stderr": "", "interrupted": false, "isImage": false}}
	}
	cases := map[string]string{
		"empty stdin":         "",
		"garbage":             "not json at all",
		"truncated json":      `{"hook_event_name":"PostToolUse","tool_name":"Bash"`,
		"json array":          `[1,2,3]`,
		"no tool_response":    hkPayload(t, map[string]any{"tool_name": "Grep", "tool_input": map[string]any{}}),
		"short output":        hkPayload(t, map[string]any{"tool_name": "Grep", "tool_input": map[string]any{}, "tool_response": map[string]any{"content": "small"}}),
		"pre tool use":        hkPayload(t, map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Grep", "tool_input": map[string]any{}, "tool_response": map[string]any{"content": long}}),
		"subagent start":      hkPayload(t, map[string]any{"hook_event_name": "SubagentStart", "tool_response": map[string]any{"content": long}}),
		"rtk rewritten":       hkPayload(t, bash("rtk go test ./...")),
		"rtk rewritten space": hkPayload(t, bash("  rtk git log")),
		"image":               hkPayload(t, map[string]any{"tool_name": "Bash", "tool_input": map[string]any{"command": "x"}, "tool_response": map[string]any{"stdout": long, "isImage": true}}),
	}
	for name, in := range cases {
		code, out := hkCompress(t, in)
		if code != 0 || out != "" {
			t.Errorf("%s: code=%d out=%.80q", name, code, out)
		}
	}
	if _, err := os.Stat(logPath); err == nil {
		t.Error("nothing was saved, nothing should be logged")
	}
	// but a Bash command that merely mentions rtk is still compressed
	if _, out := hkCompress(t, hkPayload(t, bash("echo rtk go test"))); out == "" {
		t.Error("non-rtk command must be compressed")
	}
}

type hkPanicReader struct{}

func (hkPanicReader) Read([]byte) (int, error) { panic("boom") }

type hkErrReader struct{}

func (hkErrReader) Read([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestCompressNeverFails(t *testing.T) {
	hkEnv(t)
	var out bytes.Buffer
	if code := runCompress(hkPanicReader{}, &out); code != 0 || out.Len() != 0 {
		t.Errorf("panic: code=%d out=%q", code, out.String())
	}
	if code := runCompress(hkErrReader{}, &out); code != 0 || out.Len() != 0 {
		t.Errorf("read error: code=%d out=%q", code, out.String())
	}
	// unwritable output dir and log: the replacement is still withheld safely or given, never an error
	t.Setenv("AGENTS_TREE_OUTPUT_DIR", "/proc/nope/x")
	t.Setenv("AGENTS_TREE_COMPRESS_LOG", "/proc/nope/log")
	in := hkPayload(t, map[string]any{"tool_name": "Grep", "tool_input": map[string]any{}, "tool_response": map[string]any{"content": hkLines(500, "q")}})
	if code, _ := hkCompress(t, in); code != 0 {
		t.Errorf("code %d", code)
	}
}

func TestMainDispatchesNewCommands(t *testing.T) {
	var o, e bytes.Buffer
	if code := run([]string{"hooks"}, &o, &e); code != 2 || !strings.Contains(e.String(), "hooks install|uninstall|status") {
		t.Errorf("hooks without subcommand: code=%d err=%q", code, e.String())
	}
	if !strings.Contains(usage, "agents-tree compress") || !strings.Contains(usage, "agents-tree hooks") {
		t.Error("usage does not mention compress/hooks")
	}
}

// ---- hooks install / uninstall / status ----

func hkHooks(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var o, e bytes.Buffer
	code := runHooks(args, &o, &e)
	return code, o.String(), e.String()
}

func hkSettings(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if content != "\x00missing" {
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

func hkBackups(t *testing.T, p string) []string {
	t.Helper()
	m, _ := filepath.Glob(p + ".bak-*")
	return m
}

const hkExisting = `{
  "model": "sonnet",
  "hooks": {
    "PostToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "other.sh"}]}
    ]
  }
}
`

func TestHooksDryRunByDefault(t *testing.T) {
	p := hkSettings(t, hkExisting)
	code, out, errOut := hkHooks(t, "install", "--settings", p)
	if code != 0 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	for _, want := range []string{"dry run", "--- " + p, "+++ " + p + " (proposed)", "@@", "+", "agents-tree compress", "agents-tree hook", "Exact JSON that would be written", "nothing was written", "--apply"} {
		if !strings.Contains(out, want) {
			t.Errorf("preview missing %q:\n%s", want, out)
		}
	}
	if b, _ := os.ReadFile(p); string(b) != hkExisting {
		t.Error("dry run modified the file")
	}
	if len(hkBackups(t, p)) != 0 {
		t.Error("dry run created a backup")
	}
}

func TestHooksApplyInstallThenNoopThenUninstall(t *testing.T) {
	p := hkSettings(t, hkExisting)
	fixed := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	hooksNow = func() time.Time { return fixed }
	t.Cleanup(func() { hooksNow = time.Now })

	code, out, errOut := hkHooks(t, "install", "--apply", "--settings", p)
	if code != 0 || errOut != "" || !strings.Contains(out, "backup: "+p+".bak-20261009-080000") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	bk := hkBackups(t, p)
	if len(bk) != 1 {
		t.Fatalf("backups %v", bk)
	}
	if b, _ := os.ReadFile(bk[0]); string(b) != hkExisting {
		t.Error("backup differs from original")
	}
	if st, _ := os.Stat(bk[0]); st.Mode().Perm() != 0o600 {
		t.Errorf("backup mode %v", st.Mode().Perm())
	}
	installed, _ := os.ReadFile(p)
	var m map[string]any
	if err := json.Unmarshal(installed, &m); err != nil {
		t.Fatalf("written file is not valid JSON: %v", err)
	}
	if m["model"] != "sonnet" || strings.Count(string(installed), "agents-tree compress") != 1 || !strings.Contains(string(installed), "other.sh") {
		t.Errorf("content wrong:\n%s", installed)
	}

	// second install: no-op, no new backup, file byte-identical
	code, out, _ = hkHooks(t, "install", "--apply", "--settings", p)
	if code != 0 || !strings.Contains(out, "already up to date") {
		t.Errorf("second install: code=%d out=%q", code, out)
	}
	again, _ := os.ReadFile(p)
	if string(again) != string(installed) || len(hkBackups(t, p)) != 1 {
		t.Error("second install changed something")
	}

	// uninstall restores an equivalent structure
	hooksNow = func() time.Time { return fixed.Add(time.Minute) }
	code, _, errOut = hkHooks(t, "uninstall", "--apply", "--settings", p)
	if code != 0 {
		t.Fatalf("uninstall code=%d err=%q", code, errOut)
	}
	var before, after any
	_ = json.Unmarshal([]byte(hkExisting), &before)
	restored, _ := os.ReadFile(p)
	if err := json.Unmarshal(restored, &after); err != nil || !reflect.DeepEqual(before, after) {
		t.Errorf("uninstall did not restore the original structure:\n%s", restored)
	}
	if len(hkBackups(t, p)) != 2 {
		t.Error("uninstall should also back up")
	}
	// uninstall again: nothing to do
	if code, out, _ := hkHooks(t, "uninstall", "--apply", "--settings", p); code != 0 || !strings.Contains(out, "nothing to do") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestHooksFlagsBeforeSubcommandArgsOrder(t *testing.T) {
	p := hkSettings(t, "{}")
	if code, _, e := hkHooks(t, "install", "--settings="+p, "--apply"); code != 0 {
		t.Fatalf("code=%d %s", code, e)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "agents-tree compress") {
		t.Error("not installed")
	}
}

func TestHooksMissingAndEmptyFile(t *testing.T) {
	for name, content := range map[string]string{"missing": "\x00missing", "empty": "", "whitespace": "\n  \n"} {
		p := hkSettings(t, content)
		if name == "missing" {
			p = filepath.Join(filepath.Dir(p), "sub", "settings.json")
		}
		code, out, _ := hkHooks(t, "install", "--settings", p)
		if code != 0 || !strings.Contains(out, "dry run") {
			t.Errorf("%s dry run: code=%d", name, code)
		}
		if name == "missing" && !strings.Contains(out, "--- /dev/null") {
			t.Errorf("missing: diff header\n%s", out)
		}
		if _, err := os.Stat(p); name == "missing" && err == nil {
			t.Error("dry run created the file")
		}
		if code, _, e := hkHooks(t, "install", "--apply", "--settings", p); code != 0 {
			t.Fatalf("%s apply: %d %s", name, code, e)
		}
		b, _ := os.ReadFile(p)
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil || m["hooks"] == nil {
			t.Errorf("%s: bad result %s", name, b)
		}
		wantBackups := 0
		if name != "missing" {
			wantBackups = 1
		}
		if n := len(hkBackups(t, p)); n != wantBackups {
			t.Errorf("%s: backups=%d want %d", name, n, wantBackups)
		}
	}
}

func TestHooksRefuseBadSettings(t *testing.T) {
	cases := map[string]string{
		"invalid json":     `{"hooks": {`,
		"array":            `[1]`,
		"hooks not object": `{"hooks": ["x"]}`,
		"event not array":  `{"hooks": {"SubagentStart": {}}}`,
	}
	for name, content := range cases {
		for _, sub := range []string{"install", "uninstall", "status"} {
			p := hkSettings(t, content)
			code, out, errOut := hkHooks(t, sub, "--apply", "--settings", p)
			if code == 0 || errOut == "" {
				t.Errorf("%s/%s: code=%d err=%q out=%q", name, sub, code, errOut, out)
			}
			if b, _ := os.ReadFile(p); string(b) != content {
				t.Errorf("%s/%s: file touched", name, sub)
			}
			if len(hkBackups(t, p)) != 0 {
				t.Errorf("%s/%s: backup created", name, sub)
			}
		}
	}
}

func TestHooksUsageErrors(t *testing.T) {
	for _, args := range [][]string{{}, {"nope"}, {"install", "stray"}, {"install", "--bogus"}} {
		if code, _, e := hkHooks(t, args...); code != 2 || !strings.Contains(e, "usage") && !strings.Contains(e, "unknown") {
			t.Errorf("%v: code=%d err=%q", args, code, e)
		}
	}
}

func TestHooksStatus(t *testing.T) {
	old := lookPath
	t.Cleanup(func() { lookPath = old })
	noRTK := func(string) (string, error) { return "", errors.New("not found") }
	hasRTK := func(string) (string, error) { return "/usr/local/bin/rtk", nil }

	p := hkSettings(t, hkExisting)
	lookPath = noRTK
	code, out, _ := hkHooks(t, "status", "--settings", p)
	if code != 0 {
		t.Fatal(code)
	}
	for _, want := range []string{"MISSING", "rtk on PATH: no", "RTK hook in settings: no", "brew install rtk", "cargo install --git https://github.com/rtk-ai/rtk", "rtk init -g --auto-patch", "rtk gain", "restart Claude Code"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "installed  PostToolUse") {
		t.Error("compress reported installed before install")
	}

	hkHooks(t, "install", "--apply", "--settings", p)
	lookPath = hasRTK
	_, out, _ = hkHooks(t, "status", "--settings", p)
	if strings.Contains(out, "MISSING") || strings.Count(out, "installed") < 3 || !strings.Contains(out, "rtk on PATH: yes (/usr/local/bin/rtk)") || !strings.Contains(out, "rtk init -g --auto-patch") {
		t.Errorf("status after install:\n%s", out)
	}
	if strings.Contains(out, "brew install") {
		t.Error("install hint shown although rtk is present")
	}

	// RTK hook present in settings
	rtkSettings := hkSettings(t, `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"rtk hook claude"}]}]}}`)
	_, out, _ = hkHooks(t, "status", "--settings", rtkSettings)
	if !strings.Contains(out, "RTK hook in settings: yes") || strings.Contains(out, "rtk is installed but its hook is not") {
		t.Errorf("rtk hook not detected:\n%s", out)
	}
	lookPath = noRTK
	_, out, _ = hkHooks(t, "status", "--settings", rtkSettings)
	if !strings.Contains(out, "warning") {
		t.Errorf("no warning when settings call rtk but it is missing:\n%s", out)
	}

	// status on a missing file works and writes nothing
	missing := filepath.Join(t.TempDir(), "settings.json")
	if code, out, _ := hkHooks(t, "status", "--settings", missing); code != 0 || !strings.Contains(out, "(missing)") {
		t.Errorf("code=%d out=%q", code, out)
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("status created the file")
	}
}
