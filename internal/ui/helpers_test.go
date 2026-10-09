package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/replay"
)

// ---- generic helpers --------------------------------------------------------------------------------------

func strip(s string) string { return ansi.Strip(s) }

// until polls cond until it is true or the deadline passes.
func until(t *testing.T, what string, cond func() bool) {
	t.Helper()
	end := time.Now().Add(8 * time.Second)
	for !cond() {
		if time.Now().After(end) {
			t.Fatalf("timed out waiting for: %s", what)
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func fp(v float64) *float64 { return &v }

// ---- driving a Model without a terminal -------------------------------------------------------------------

// run feeds a command's message back into the model. Batches are expanded; tick commands (which
// would sleep) and nil commands are skipped.
func (m *Model) run(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		switch v := msg.(type) {
		case tea.BatchMsg:
			for _, c := range v {
				m.run(c)
			}
		case sendDoneMsg, compMsg:
			m.Update(v)
		}
	case <-time.After(150 * time.Millisecond):
		// a sleeping tick (or anything slow): not needed by the tests
	}
}

// press sends key presses ("i", "enter", "ctrl+k", ...) and runs the commands they return.
func (m *Model) press(keys ...string) {
	for _, k := range keys {
		_, cmd := m.Update(KeyFromString(k))
		m.run(cmd)
	}
}

// typeText types text rune by rune, as a user would.
func (m *Model) typeText(text string) {
	for _, r := range text {
		if r == ' ' {
			m.press("space")
			continue
		}
		m.press(string(r))
	}
}

func (m *Model) resize(w, h int) { m.Update(tea.WindowSizeMsg{Width: w, Height: h}) }

// tick advances the app clock n ticks (poll sources, refresh).
func (m *Model) tick(n int) {
	for i := 0; i < n; i++ {
		m.Update(tickMsg(time.Now()))
	}
}

// screen is the plain-text screen.
func (m *Model) screen() string { return strip(m.Render()) }

// chatText is the whole chat content (not only the visible window), plain.
func (m *Model) chatText() string { return strip(m.chat.GetContent()) }

// ---- building apps ----------------------------------------------------------------------------------------

// replayApp is a replay model fed with in-memory events, sized w x h.
func replayApp(t *testing.T, events []model.Event, w, h int) *Model {
	t.Helper()
	m, err := New(Options{Replay: true, ProjectsDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	m.rep = replay.New(events, 1000)
	m.rep.MaxGap = 0.01
	m.resize(w, h)
	return m
}

// settle plays a replay to its end.
func (m *Model) settle(t *testing.T) {
	t.Helper()
	for i := 0; i < 400 && !m.rep.Done(); i++ {
		m.tick(1)
	}
	m.tick(2)
	if !m.rep.Done() {
		t.Fatal("replay did not finish")
	}
}

// liveFile writes a one-prompt transcript under <root>/projects/-fake-proj/<id>.jsonl.
func liveFile(t *testing.T, root, id, prompt string) string {
	t.Helper()
	proj := filepath.Join(root, "projects", "-fake-proj")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(proj, id+".jsonl")
	line := fmt.Sprintf(`{"type":"user","uuid":"u0-%s","timestamp":"2026-10-09T10:00:00Z","cwd":%q,"origin":{"kind":"human"},"message":{"role":"user","content":%q}}`+"\n",
		id, root, prompt)
	if err := os.WriteFile(p, []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// liveApp follows a transcript live (no hooks), 140x40.
func liveApp(t *testing.T, opt Options) *Model {
	t.Helper()
	opt.Hooks = false
	if opt.ProjectsDir == "" {
		opt.ProjectsDir = t.TempDir()
	}
	m, err := New(opt)
	if err != nil {
		t.Fatal(err)
	}
	m.resize(140, 40)
	m.Init()
	return m
}

// ---- events & messages ------------------------------------------------------------------------------------

func evTurn(ts float64, id, modelName, tool string, out int) model.Event {
	return model.Event{TS: ts, Kind: model.Turn, AgentID: id, Model: modelName, Tool: tool,
		Usage: &model.Usage{Output: out}}
}

func evStart(ts float64, id, parent, kind, desc, modelName string) model.Event {
	return model.Event{TS: ts, Kind: model.AgentStart, AgentID: id, ParentID: parent, AgentKind: kind, Desc: desc, Model: modelName}
}

func evEnd(ts float64, id, status string) model.Event {
	return model.Event{TS: ts, Kind: model.AgentEnd, AgentID: id, Status: status}
}

func evMsg(ts float64, m model.Message) model.Event {
	m.TS = ts
	if m.AgentID == "" {
		m.AgentID = model.Main
	}
	return model.Event{TS: ts, Kind: model.Message_, AgentID: m.AgentID, Msg: &m}
}

func evUpdate(ts float64, id, status string, dur *float64) model.Event {
	return model.Event{TS: ts, Kind: model.MsgUpdate, MsgID: id, Status: status, MsgDuration: dur}
}

const (
	sonnet = "claude-sonnet-5-5"
	haiku  = "claude-haiku-5-5"
)

// conversationEvents is the port of the Python `conversation_store` fixture: every role once, one explorer
// subagent (ag1) that is still running, timestamps around 1000.
func conversationEvents() []model.Event {
	return []model.Event{
		evTurn(1000, model.Main, sonnet, "", 5),
		evStart(1001, "ag1", model.Main, "explorer", "map the repo", ""),
		evTurn(1001, "ag1", haiku, "Read", 5),
		evMsg(1000, model.Message{ID: "u", Role: "user", Text: "Peux-tu ne montrer que les agents en cours ?"}),
		evMsg(1001, model.Message{ID: "s", Role: "system", Text: "Stop hook feedback:\nuntracked files"}),
		evMsg(1002, model.Message{ID: "a", Role: "assistant", Model: sonnet,
			Text: "Done.\n\n- the rail lists running agents\n- `h` shows history"}),
		evMsg(1003, model.Message{ID: "t1", Role: "tool", Tool: "Bash", Detail: "pytest -q", Status: "error", Duration: fp(9.0)}),
		evMsg(1003, model.Message{ID: "t2", Role: "tool", Tool: "Read", Detail: "a/store.py", Status: "ok", Duration: fp(0.2)}),
		evMsg(1004, model.Message{ID: "deleg:ag1", Role: "delegation", Kind: "explorer", Desc: "map the repo",
			Text: "List files.", Target: "ag1"}),
		evMsg(1005, model.Message{ID: "sa", AgentID: "ag1", Role: "assistant", Model: haiku, Text: "Looking."}),
		evMsg(1006, model.Message{ID: "rep:ag1", Role: "report", Text: "Found 3 files.", Target: "ag1", Status: "done", Duration: fp(5.0)}),
	}
}

// byID indexes the store's messages.
func byID(msgs []*model.Message) map[string]*model.Message {
	out := map[string]*model.Message{}
	for _, m := range msgs {
		out[m.ID] = m
	}
	return out
}

func lines(s string) []string { return strings.Split(s, "\n") }
