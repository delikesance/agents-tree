package sender

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeBin(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../tests/fake_claude.py")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func fakeEnv(t *testing.T, delay string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("FAKE_ARGV", filepath.Join(dir, "argv.jsonl"))
	t.Setenv("FAKE_PROJECTS", filepath.Join(dir, "projects"))
	t.Setenv("FAKE_DELAY", delay)
	t.Setenv("FAKE_MODE", "")
	return dir
}

type call struct {
	Argv   []string `json:"argv"`
	CWD    string   `json:"cwd"`
	Leaked []string `json:"leaked"`
}

func calls(t *testing.T, dir string) []call {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "argv.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []call
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var c call
		if err := json.Unmarshal([]byte(l), &c); err != nil {
			t.Fatal(err)
		}
		out = append(out, c)
	}
	return out
}

func until(t *testing.T, cond func() bool) {
	t.Helper()
	end := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(end) {
			t.Fatal("timed out")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func has(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestBuildCommandFlagsForEachPermissionPreset(t *testing.T) {
	all, _ := BuildCommand("claude", "S1", true, "all", "")
	if all[0] != "claude" || all[1] != "-p" || !has(all, "--dangerously-skip-permissions") || !has(all, "--verbose") {
		t.Errorf("all = %v", all)
	}
	if got := all[len(all)-2:]; got[0] != "--resume" || got[1] != "S1" {
		t.Errorf("resume = %v", got)
	}
	plan, _ := BuildCommand("claude", "S2", false, "plan", "haiku")
	if has(plan, "--dangerously-skip-permissions") || !has(plan, "plan") || !has(plan, "--session-id") || plan[len(plan)-1] != "haiku" {
		t.Errorf("plan = %v", plan)
	}
	ae, _ := BuildCommand("claude", "S", true, "accept-edits", "")
	if !has(ae, "acceptEdits") {
		t.Errorf("accept-edits = %v", ae)
	}
	if _, err := BuildCommand("claude", "S", true, "nope", ""); err == nil {
		t.Error("unknown preset must fail")
	}
	if _, err := New("S", "", true, "nope", ""); err == nil {
		t.Error("New must reject unknown preset")
	}
}

func TestUserLineIsOneStreamJSONMessage(t *testing.T) {
	var d struct {
		Type    string `json:"type"`
		Message struct {
			Role    string `json:"role"`
			Content []struct{ Type, Text string }
		} `json:"message"`
	}
	l := UserLine("héllo \"q\"\nline2")
	if err := json.Unmarshal(l, &d); err != nil {
		t.Fatal(err)
	}
	if d.Type != "user" || d.Message.Content[0].Text != "héllo \"q\"\nline2" || strings.Count(string(l), "\n") != 1 {
		t.Errorf("%s", l)
	}
}

func TestCleanEnvDropsVariablesThatTieTheChildToTheParentSession(t *testing.T) {
	in := []string{"PATH=/bin", "HOME=/h", "ANTHROPIC_API_KEY=k", "CLAUDE_CODE_SESSION_ID=x", "CLAUDE_CODE_REMOTE=true",
		"CLAUDE_CODE_REMOTE_SESSION_ID=y", "CLAUDECODE=1", "CLAUDE_CODE_CHILD_SESSION=1", "SESSION_INGRESS_URL=u",
		"CLAUDE_CODE_MESSAGING_TOKEN=t", "CLAUDE_EFFORT=high", "CLAUDE_PID=95"}
	got := strings.Join(CleanEnv(in), " ")
	if got != "PATH=/bin HOME=/h ANTHROPIC_API_KEY=k CLAUDE_EFFORT=high" {
		t.Errorf("got %q", got)
	}
}

func TestResumeSendBusyCycleAndStdinDelivery(t *testing.T) {
	dir := fakeEnv(t, "0.2")
	s, _ := New("sess-1", dir, true, "all", fakeBin(t))
	if !s.Available() || s.Alive() || s.Busy() {
		t.Fatal("initial state")
	}
	if err := s.Send("hello there"); err != nil {
		t.Fatal(err)
	}
	if !s.Busy() || !s.Alive() || s.Sent() != 1 {
		t.Fatal("after send")
	}
	until(t, func() bool { return !s.Busy() })
	if s.LastError() != "" {
		t.Errorf("error = %q", s.LastError())
	}
	tr, _ := filepath.Glob(filepath.Join(dir, "projects", "*", "sess-1.jsonl"))
	if len(tr) != 1 {
		t.Fatal("transcript not written")
	}
	raw, _ := os.ReadFile(tr[0])
	if !strings.Contains(string(raw), `"content": "hello there"`) && !strings.Contains(string(raw), `"content":"hello there"`) {
		t.Errorf("user line missing: %s", raw)
	}
	if !strings.Contains(string(raw), "echo: hello there") {
		t.Errorf("reply missing: %s", raw)
	}
	if err := s.Send("second"); err != nil { // same process, no restart
		t.Fatal(err)
	}
	until(t, func() bool { return !s.Busy() })
	cs := calls(t, dir)
	if len(cs) != 1 || cs[0].Argv[len(cs[0].Argv)-2] != "--resume" || !has(cs[0].Argv, "--dangerously-skip-permissions") {
		t.Errorf("calls = %+v", cs)
	}
	s.Stop()
	if s.Alive() {
		t.Error("still alive after Stop")
	}
}

func TestChildNeverSeesTheParentSessionVariables(t *testing.T) {
	dir := fakeEnv(t, "0.1")
	t.Setenv("CLAUDE_CODE_SESSION_ID", "parent-session")
	s, _ := New("sess-2", dir, true, "all", fakeBin(t))
	_ = s.Send("hi")
	until(t, func() bool { return !s.Busy() })
	s.Stop()
	if l := calls(t, dir)[0].Leaked; len(l) != 0 {
		t.Errorf("leaked %v", l)
	}
}

func TestNewSessionUsesSessionIDFlagAndCWD(t *testing.T) {
	dir := fakeEnv(t, "0.1")
	s, _ := New("", dir, false, "all", fakeBin(t))
	id := s.SessionID
	if len(id) != 36 || s.resume {
		t.Fatalf("id=%q resume=%v", id, s.resume)
	}
	_ = s.Send("start")
	until(t, func() bool { return !s.Busy() })
	s.Stop()
	c := calls(t, dir)[0]
	real, _ := filepath.EvalSymlinks(dir)
	cwd, _ := filepath.EvalSymlinks(c.CWD)
	if c.Argv[len(c.Argv)-2] != "--session-id" || c.Argv[len(c.Argv)-1] != id || cwd != real {
		t.Errorf("call = %+v", c)
	}
}

func TestErrorResultIsSurfacedAndProcessRestartsAfterExit(t *testing.T) {
	dir := fakeEnv(t, "0.1")
	t.Setenv("FAKE_MODE", "fail")
	s, _ := New("sess-3", dir, true, "all", fakeBin(t))
	_ = s.Send("x")
	until(t, func() bool { return !s.Busy() })
	if s.LastError() != "model overloaded" {
		t.Errorf("error = %q", s.LastError())
	}
	s.Stop()

	t.Setenv("FAKE_MODE", "exit") // answers once, then the process ends
	s2, _ := New("sess-4", dir, true, "all", fakeBin(t))
	_ = s2.Send("one")
	until(t, func() bool { return !s2.Alive() && !s2.Busy() })
	if err := s2.Send("two"); err != nil { // restarted, and resumes the session
		t.Fatal(err)
	}
	until(t, func() bool { return !s2.Alive() && !s2.Busy() })
	if s2.Sent() != 2 {
		t.Errorf("sent = %d", s2.Sent())
	}
	n := 0
	for _, c := range calls(t, dir) {
		if has(c.Argv, "sess-4") {
			n++
			if c.Argv[len(c.Argv)-2] != "--resume" {
				t.Errorf("not resumed: %v", c.Argv)
			}
		}
	}
	if n != 2 {
		t.Errorf("process started %d times", n)
	}
}

func TestMissingBinaryIsAClearErrorNotACrash(t *testing.T) {
	s, _ := New("x", t.TempDir(), true, "all", "definitely-not-claude")
	if s.Available() {
		t.Fatal("should not be available")
	}
	if err := s.Send("hi"); err == nil || !strings.Contains(s.LastError(), "not found") || s.Busy() {
		t.Errorf("err=%v lastError=%q busy=%v", err, s.LastError(), s.Busy())
	}
}

func TestInterruptStopsARunningTurn(t *testing.T) {
	dir := fakeEnv(t, "30")
	s, _ := New("sess-5", dir, true, "all", fakeBin(t))
	_ = s.Send("long task")
	until(t, func() bool { _, err := os.Stat(filepath.Join(dir, "argv.jsonl")); return err == nil })
	if !s.Busy() {
		t.Fatal("should be busy")
	}
	s.Interrupt()
	if s.Alive() || s.Busy() {
		t.Error("still running after Interrupt")
	}
}
