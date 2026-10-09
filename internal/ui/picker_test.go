package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/delikesance/agents-tree/internal/sessions"
)

// projectsWithTwoSessions builds two projects: A (older, the fixture session with two subagents and a prompt
// that carries a system reminder and a tag) and B (newer, a single low-effort Haiku turn).
func projectsWithTwoSessions(t *testing.T) (root, pathA, pathB string) {
	t.Helper()
	root = t.TempDir()
	a := filepath.Join(root, "-home-user-proj-a")
	b := filepath.Join(root, "-home-user-proj-b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	prompt, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{"content": []any{
		map[string]any{"type": "text", "text": "<system-reminder>ctx</system-reminder>"},
		map[string]any{"type": "text", "text": "Build the <b>login</b> page"}}}})
	fixture, err := os.ReadFile("../../tests/fixtures/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	pathA = filepath.Join(a, "aaaa1111.jsonl")
	if err := os.WriteFile(pathA, append(append(prompt, '\n'), fixture...), 0o644); err != nil {
		t.Fatal(err)
	}
	subs := filepath.Join(a, "aaaa1111", "subagents")
	if err := os.MkdirAll(subs, 0o755); err != nil {
		t.Fatal(err)
	}
	sub, err := os.ReadFile("../../tests/fixtures/session/subagents/agent-a1.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subs, "agent-a1.jsonl"), sub, 0o644); err != nil {
		t.Fatal(err)
	}
	pathB = filepath.Join(b, "bbbb2222.jsonl")
	turn := `{"type":"assistant","timestamp":"2026-10-09T11:00:00Z","effort":"low","message":{"model":"claude-haiku-5-5","usage":{"output_tokens":7},"content":[]}}`
	if err := os.WriteFile(pathB, []byte(`{"type":"user","message":{"content":"Fix the typo"}}`+"\n"+turn+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(pathA, old, old); err != nil {
		t.Fatal(err)
	}
	return root, pathA, pathB
}

func pickerApp(t *testing.T, root string) *Model {
	t.Helper()
	m := liveApp(t, Options{ProjectsDir: root})
	if m.picker == nil {
		t.Fatal("the picker opens when there is no session")
	}
	return m
}

func kindsOf(m *Model) []string {
	var ks []string
	for _, id := range m.st.Order {
		ks = append(ks, m.st.Nodes[id].Kind)
	}
	return ks
}

func TestPickerListsSessionsNewestFirstWithProjectAndTitle(t *testing.T) {
	root, _, _ := projectsWithTwoSessions(t)
	got := sessions.List(root, 0)
	if len(got) != 2 || got[0].ID() != "bbbb2222" || got[1].ID() != "aaaa1111" {
		t.Fatalf("list = %+v", got)
	}
	if got[0].Title != "Fix the typo" || got[1].Title != "Build the login page" || got[1].Subagents != 1 || got[0].Subagents != 0 {
		t.Errorf("titles/subagents: %+v", got)
	}
	m := pickerApp(t, root)
	scr := m.screen()
	for _, want := range []string{"select a session", "modified", "project", "first prompt", "Fix the typo", "Build the login page",
		"proj/b", "proj/a", "bbbb2222", "aaaa1111", "just now", "2h ago", "type to filter"} {
		if !strings.Contains(scr, want) {
			t.Errorf("picker lacks %q:\n%s", want, scr)
		}
	}
	if strings.Index(scr, "Fix the typo") > strings.Index(scr, "Build the login page") {
		t.Errorf("newest session first:\n%s", scr)
	}
	if strings.Contains(scr, "system-reminder") || strings.Contains(scr, "<b>") {
		t.Error("reminders and tags are stripped from titles")
	}
}

func TestPickerEnterLoadsTheNewestThenSwitchesAndRebuildsTheStore(t *testing.T) {
	root, pathA, pathB := projectsWithTwoSessions(t)
	m := pickerApp(t, root)
	m.press("enter") // first row = newest = B
	if m.picker != nil || m.session != pathB {
		t.Fatalf("session = %q picker=%v", m.session, m.picker != nil)
	}
	m.tick(2)
	if ks := kindsOf(m); len(ks) != 1 || ks[0] != "main" || m.st.Nodes["main"].Effort != "low" {
		t.Fatalf("B state: kinds=%v effort=%q", ks, m.st.Nodes["main"].Effort)
	}
	if !strings.Contains(m.chatText(), "Fix the typo") || !strings.Contains(m.screen(), "session bbbb2222") {
		t.Errorf("B chat/status:\n%s", m.screen())
	}

	m.press("s") // reopen: the current one is marked, then pick A
	if m.picker == nil || !strings.Contains(m.screen(), "● Fix the typo") {
		t.Fatalf("the current session is marked:\n%s", m.screen())
	}
	m.press("down", "enter")
	m.tick(2)
	if m.session != pathA {
		t.Fatalf("session = %q", m.session)
	}
	ks := kindsOf(m)
	if len(ks) != 3 || !contains(ks, "main") || !contains(ks, "explorer") || !contains(ks, "worker") {
		t.Errorf("A state, B's agents must be gone and A's present: kinds=%v", ks)
	}
	if m.st.Nodes["main"].Effort != "high" {
		t.Errorf("main effort = %q", m.st.Nodes["main"].Effort)
	}
	txt := m.chatText()
	if strings.Contains(txt, "Fix the typo") || !strings.Contains(txt, "Build the login page") {
		t.Errorf("the chat was rebuilt for A:\n%s", txt)
	}
	if m.filter != "" || !m.follow {
		t.Error("switching resets the filter and follows the end")
	}

	m.press("s")
	m.press("esc") // esc keeps the current session
	if m.picker != nil || m.session != pathA {
		t.Errorf("esc must keep %q, got %q", pathA, m.session)
	}
	if len(kindsOf(m)) != 3 {
		t.Error("esc must not reset the store")
	}
}

func TestPickerFilterTypingDoesNotTriggerShortcutsAndEnterOpensTheMatch(t *testing.T) {
	root, pathA, _ := projectsWithTwoSessions(t)
	m := pickerApp(t, root)
	m.typeText("sq") // s and q are app shortcuts
	if m.picker == nil || m.picker.filter.Value() != "sq" {
		t.Fatalf("typed into the filter, picker=%v", m.picker != nil)
	}
	if n := len(m.picker.visible()); n != 0 || !strings.Contains(m.screen(), "no session matches") {
		t.Errorf("nothing matches 'sq' (%d)", n)
	}
	m.press("enter") // nothing to open: the picker stays
	if m.picker == nil || m.session != "" {
		t.Error("enter with no match does nothing")
	}
	m.press("backspace", "backspace")
	m.typeText("login")
	if vis := m.picker.visible(); len(vis) != 1 || vis[0].Path != pathA {
		t.Fatalf("visible after 'login': %+v", vis)
	}
	scr := m.screen()
	if !strings.Contains(scr, "Build the login page") || strings.Contains(scr, "Fix the typo") {
		t.Errorf("filtered list:\n%s", scr)
	}
	m.press("enter")
	if m.picker != nil || m.session != pathA {
		t.Errorf("enter opens the matching session: %q", m.session)
	}
}

func TestPickerFiltersByProjectAndSessionIdToo(t *testing.T) {
	root, _, pathB := projectsWithTwoSessions(t)
	m := pickerApp(t, root)
	m.typeText("proj/b")
	if vis := m.picker.visible(); len(vis) != 1 || vis[0].Path != pathB {
		t.Errorf("by project: %+v", vis)
	}
	m.picker.filter.SetValue("AAAA")
	if vis := m.picker.visible(); len(vis) != 1 || vis[0].ID() != "aaaa1111" {
		t.Errorf("by id, case-insensitive: %+v", vis)
	}
}

func TestPickerCursorMovesAndStaysInRange(t *testing.T) {
	root, pathA, pathB := projectsWithTwoSessions(t)
	m := pickerApp(t, root)
	m.press("up")
	if m.picker.cursor != 0 {
		t.Error("up at the top stays at the top")
	}
	m.press("down", "down", "down")
	if m.picker.cursor != 1 {
		t.Errorf("down stops at the last row: %d", m.picker.cursor)
	}
	m.press("pgup")
	if m.picker.cursor != 0 {
		t.Error("pgup")
	}
	m.press("pgdown")
	if m.picker.cursor != 1 {
		t.Error("pgdown")
	}
	m.press("ctrl+p")
	m.press("ctrl+n")
	if m.picker.cursor != 1 {
		t.Error("ctrl+p / ctrl+n move like up/down")
	}
	// typing resets the cursor to the first match
	m.typeText("proj")
	if m.picker.cursor != 0 {
		t.Error("the cursor resets when the filter changes")
	}
	m.press("esc")
	m.press("s")
	m.press("down", "enter")
	if m.session != pathA {
		t.Errorf("down+enter opens the second row: %q (B is %q)", m.session, pathB)
	}
}

func TestPickerEscOnStartKeepsNoSessionAndQuitStillWorksAfterwards(t *testing.T) {
	root, _, _ := projectsWithTwoSessions(t)
	m := pickerApp(t, root)
	m.press("esc")
	if m.picker != nil || m.session != "" {
		t.Fatal("esc closes the picker without a session")
	}
	m.press("s")
	if m.picker == nil {
		t.Error("s reopens it")
	}
	_, cmd := m.Update(KeyFromString("ctrl+c"))
	if cmd == nil {
		t.Error("ctrl+c quits even from the picker")
	}
}

func TestPickerWithNoSessionsOnDiskSaysSo(t *testing.T) {
	m := pickerApp(t, t.TempDir())
	if !strings.Contains(m.screen(), "no session matches") {
		t.Errorf("empty picker:\n%s", m.screen())
	}
}

func TestPickerOverlayKeepsTheScreenTheSizeOfTheTerminal(t *testing.T) {
	root, _, _ := projectsWithTwoSessions(t)
	for _, sz := range [][2]int{{80, 24}, {120, 30}, {160, 50}} {
		m := pickerApp(t, root)
		m.resize(sz[0], sz[1])
		checkScreen(t, "picker", m, sz[0], sz[1])
	}
}

func TestLoadingAMissingSessionFails(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	if err := m.load(filepath.Join(root, "nope.jsonl")); err == nil {
		t.Error("loading a missing file is an error")
	}
	if _, err := New(Options{Session: filepath.Join(root, "nope.jsonl")}); err == nil {
		t.Error("New with a missing session fails")
	}
}
