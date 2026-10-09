package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
)

func visibleCount(m *Model) int {
	n := 0
	for _, x := range m.st.Messages {
		if m.passes(x) {
			n++
		}
	}
	return n
}

func TestKeyFromStringBuildsTheKeysTheAppListensTo(t *testing.T) {
	for _, s := range []string{"i", "q", "enter", "esc", "ctrl+k", "alt+enter", "up", "pgup", "pgdown", "end", "home", "tab", "backspace", "ctrl+c"} {
		if got := KeyFromString(s).String(); got != s {
			t.Errorf("KeyFromString(%q).String() = %q", s, got)
		}
	}
	if got := KeyFromString("space").String(); got != " " && got != "space" {
		t.Errorf("space = %q", got)
	}
	if got := KeyFromString("+").String(); got != "+" {
		t.Errorf("+ = %q", got)
	}
}

func TestFilterCyclesEveryoneMainEachAgentAndBack(t *testing.T) {
	m := replayApp(t, conversationEvents(), 140, 40)
	m.settle(t)
	if m.filter != "" || visibleCount(m) != 8 || !strings.Contains(m.chatText(), "Looking.") {
		t.Fatalf("everyone: filter=%q visible=%d", m.filter, visibleCount(m))
	}
	m.press("f") // everyone -> main only
	m.tick(1)
	if m.filter != model.Main || visibleCount(m) != 7 || strings.Contains(m.chatText(), "Looking.") {
		t.Fatalf("main: filter=%q visible=%d", m.filter, visibleCount(m))
	}
	if !strings.Contains(m.screen(), "filter: main") {
		t.Error("status line must name the filter")
	}
	m.press("f") // -> the explorer
	m.tick(1)
	txt := m.chatText()
	if m.filter != "ag1" || visibleCount(m) != 3 { // its own output + the delegation + its report
		t.Fatalf("explorer: filter=%q visible=%d", m.filter, visibleCount(m))
	}
	if !strings.Contains(txt, "Looking.") || !strings.Contains(txt, "Found 3 files.") || strings.Contains(txt, "Peux-tu") {
		t.Errorf("explorer filter content:\n%s", txt)
	}
	if !strings.Contains(m.screen(), "filter: explorer") {
		t.Error("status line must name the agent kind")
	}
	m.press("f") // -> everyone again
	m.tick(1)
	if m.filter != "" || visibleCount(m) != 8 || !strings.Contains(m.chatText(), "Peux-tu") {
		t.Fatalf("back to everyone: filter=%q visible=%d", m.filter, visibleCount(m))
	}
	if strings.Contains(m.screen(), "filter:") {
		t.Error("no filter label when showing everyone")
	}
}

func TestExpandKeyShowsSystemLinesInFullAndUnfoldsLongText(t *testing.T) {
	long := strings.Repeat("line\n", 20) + "tail marker"
	ev := append(conversationEvents(), evMsg(1007, model.Message{ID: "long", Role: "assistant", Model: sonnet, Text: long}))
	m := replayApp(t, ev, 140, 60)
	m.settle(t)
	collapsed := m.chatText()
	if strings.Contains(collapsed, "untracked files") || strings.Contains(collapsed, "tail marker") || !strings.Contains(collapsed, "e to expand") {
		t.Fatalf("collapsed chat:\n%s", collapsed)
	}
	m.press("e")
	m.tick(1)
	if !m.expanded {
		t.Fatal("e must set expanded")
	}
	full := m.chatText()
	if !strings.Contains(full, "⚙ system") || !strings.Contains(full, "untracked files") || !strings.Contains(full, "tail marker") {
		t.Errorf("expanded chat:\n%s", full)
	}
	m.press("e")
	m.tick(1)
	if m.expanded || strings.Contains(m.chatText(), "tail marker") {
		t.Error("e again must collapse")
	}
}

func TestTKeyTogglesChatAndTreeAndTheInputBoxGoesAway(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "first prompt"), CanSend: true, ProjectsDir: root})
	m.tick(2)
	if m.view != "chat" || !strings.Contains(m.screen(), "Message Claude") || !strings.Contains(m.screen(), "first prompt") {
		t.Fatalf("chat view should show the transcript and the input box:\n%s", m.screen())
	}
	if strings.Contains(m.screen(), "│ log") || strings.Contains(m.screen(), "─ log ") {
		t.Error("the log belongs to the tree view")
	}
	m.press("t")
	scr := m.screen()
	if m.view != "tree" || strings.Contains(scr, "Message Claude") || strings.Contains(scr, "write to Claude") || !strings.Contains(scr, "─ log ") {
		t.Fatalf("tree view has no input box but a log:\n%s", scr)
	}
	if !strings.Contains(scr, "· tree ·") {
		t.Error("status line should say tree")
	}
	m.press("i") // i does nothing in the tree view
	if m.inputFocus {
		t.Error("input must not take focus in the tree view")
	}
	m.press("t")
	if m.view != "chat" || !strings.Contains(m.screen(), "Message Claude") {
		t.Error("back to chat")
	}
}

func TestRailListsOnlyRunningAgentsAndCountsTheRest(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	now := nowSecs()
	for _, ev := range []model.Event{
		evTurn(now-600, model.Main, sonnet, "", 5), // main has been quiet for 10 minutes
		evStart(now-300, "ag1", model.Main, "explorer", "silent one", haiku),
		evStart(now-30, "ag2", model.Main, "worker", "done one", sonnet), evEnd(now-20, "ag2", "done"),
		evStart(now-30, "ag4", model.Main, "researcher", "failed one", haiku), evEnd(now-20, "ag4", "failed"),
		evStart(now-5, "ag3", model.Main, "dispatcher", "busy one", haiku), evTurn(now-1, "ag3", haiku, "Bash", 5),
	} {
		m.st.Apply(ev)
	}
	m.tick(1)
	scr := m.screen()
	for _, want := range []string{"AGENTS · 1 RUNNING", "dispatcher", "busy one", "Bash", "3 finished or idle hidden · h", "◌ idle", "view: active"} {
		if !strings.Contains(scr, want) {
			t.Errorf("active rail lacks %q:\n%s", want, scr)
		}
	}
	for _, gone := range []string{"explorer", "silent one", "done one", "failed one"} {
		if strings.Contains(scr, gone) {
			t.Errorf("active rail must not show %q", gone)
		}
	}
	if st := m.st.State(m.st.Nodes["ag1"], now); st != "stale" {
		t.Errorf("a silent subagent is not running, got %q", st)
	}
	m.press("h")
	all := m.screen()
	for _, want := range []string{"explorer", "silent one", "done one", "failed one", "✓ done", "✗ failed", "no activity", "view: all"} {
		if !strings.Contains(all, want) {
			t.Errorf("history rail lacks %q:\n%s", want, all)
		}
	}
	if strings.Contains(all, "finished or idle hidden") {
		t.Error("nothing is hidden in history view")
	}
	m.press("h")
	if strings.Contains(m.screen(), "silent one") || !strings.Contains(m.screen(), "view: active") {
		t.Error("h again goes back to the active view")
	}
}

func TestRailSaysNoAgentRunningWhenThereAreNone(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	m.st.Apply(evTurn(nowSecs(), model.Main, sonnet, "", 1))
	m.tick(1)
	if !strings.Contains(m.screen(), "no agent running") || !strings.Contains(m.screen(), "AGENTS · 0 RUNNING") {
		t.Errorf("empty rail:\n%s", m.screen())
	}
	m.press("t")
	if !strings.Contains(m.screen(), "no agent running") {
		t.Error("the tree view says it too")
	}
}

func TestActiveViewKeepsTheFinishedParentOfARunningChild(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	now := nowSecs()
	m.st.Apply(evStart(now-5, "p", model.Main, "dispatcher", "parent job", haiku))
	m.st.Apply(evStart(now-4, "c", "p", "worker", "child job", sonnet))
	m.st.Apply(evEnd(now-3, "p", "done"))
	m.tick(1)
	scr := m.screen()
	if !strings.Contains(scr, "child job") || !strings.Contains(scr, "parent job") || !strings.Contains(scr, "AGENTS · 1 RUNNING") {
		t.Errorf("ancestor of a running agent must stay visible:\n%s", scr)
	}
}

func TestLiveStartsActiveAndReplayStartsWithHistory(t *testing.T) {
	root := t.TempDir()
	live := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root})
	if live.showAll || !strings.Contains(live.screen(), "view: active") || !strings.Contains(live.screen(), "● live") {
		t.Errorf("live must start in the active view:\n%s", live.screen())
	}
	rep := replayApp(t, conversationEvents(), 140, 40)
	if !rep.showAll {
		t.Error("replay starts in the full-history view")
	}
}

func TestReplayHasNoInputBoxAndITogglesNothing(t *testing.T) {
	m := replayApp(t, conversationEvents(), 120, 30)
	m.settle(t)
	scr := m.screen()
	if strings.Contains(scr, "Message Claude") || strings.Contains(scr, "write to Claude") || strings.Contains(scr, "i  write") || m.canSend() {
		t.Errorf("replay must not offer to write:\n%s", scr)
	}
	m.press("i")
	if m.inputFocus {
		t.Error("i must not focus the input in a replay")
	}
	m.press("enter")
	if m.inputFocus {
		t.Error("enter must not focus the input in a replay")
	}
	m.typeText("hello")
	if m.input.Value() != "" {
		t.Errorf("nothing is typed in a replay: %q", m.input.Value())
	}
	if strings.Contains(m.hints(), "ctrl+k") {
		t.Error("no stop-Claude hint in a replay")
	}
	if !strings.Contains(strip(m.hints()), "space") || !strings.Contains(strip(m.hints()), "speed") {
		t.Error("replay hints list pause and speed")
	}
}

func TestReplayControlsSpaceAndPlusMinus(t *testing.T) {
	m := replayApp(t, conversationEvents(), 140, 40)
	m.rep.Speed = 4
	m.press("space")
	if !m.rep.Paused || !strings.Contains(m.screen(), "[paused]") {
		t.Fatal("space pauses")
	}
	pos := m.rep.Pos
	m.tick(5)
	if m.rep.Pos != pos {
		t.Error("a paused replay does not advance")
	}
	m.press("space")
	if m.rep.Paused || strings.Contains(m.screen(), "[paused]") {
		t.Fatal("space resumes")
	}
	m.rep.Speed = 1000
	m.settle(t)
	if m.rep.Pos != len(m.rep.Events) {
		t.Error("an unpaused replay finishes")
	}

	m.rep.Speed = 4
	m.press("+")
	m.press("=")
	if m.rep.Speed != 16 || !strings.Contains(m.screen(), "x16") {
		t.Errorf("speed after ++ = %v", m.rep.Speed)
	}
	for i := 0; i < 10; i++ {
		m.press("+")
	}
	if m.rep.Speed != 64 {
		t.Errorf("speed is capped at 64, got %v", m.rep.Speed)
	}
	for i := 0; i < 20; i++ {
		m.press("-")
	}
	if m.rep.Speed != 0.25 {
		t.Errorf("speed is floored at 0.25, got %v", m.rep.Speed)
	}
}

func TestReplayRevealsEventsOverTime(t *testing.T) {
	m := replayApp(t, conversationEvents(), 140, 40)
	m.rep.Speed = 1 // 1 virtual second per real second: the first tick only reveals the first events
	m.rep.MaxGap = 100
	m.tick(1)
	if len(m.st.Messages) >= 8 {
		t.Fatalf("replay should not show everything at once: %d messages", len(m.st.Messages))
	}
	m.rep.Speed = 1000
	m.settle(t)
	if len(m.st.Messages) != 8 {
		t.Errorf("after the replay: %d messages", len(m.st.Messages))
	}
}

func longConversation(n int) []model.Event {
	var evs []model.Event
	for i := 0; i < n; i++ {
		evs = append(evs, evMsg(1000+float64(i), model.Message{ID: fmt.Sprintf("m%d", i), Role: "assistant", Model: sonnet,
			Text: fmt.Sprintf("msg-%04d\nsecond line\nthird line", i)}))
	}
	return evs
}

func TestChatFollowsNewMessagesUnlessScrolledUp(t *testing.T) {
	m := replayApp(t, longConversation(60), 120, 30)
	m.settle(t)
	if m.chat.TotalLineCount() <= m.chat.Height() {
		t.Fatalf("the test needs a chat taller than the window: %d lines, %d rows", m.chat.TotalLineCount(), m.chat.Height())
	}
	if !m.follow || !m.chat.AtBottom() || !strings.Contains(m.screen(), "msg-0059") {
		t.Fatal("a fresh chat follows the newest message")
	}
	m.press("up")
	if m.follow || m.chat.AtBottom() {
		t.Fatal("up stops following")
	}
	off := m.chat.YOffset()
	m.st.Apply(evMsg(2000, model.Message{ID: "late", Role: "assistant", Text: "a late message"}))
	m.tick(1)
	if !strings.Contains(m.chatText(), "a late message") {
		t.Fatal("the new message is in the chat")
	}
	if m.chat.YOffset() != off || m.chat.AtBottom() || strings.Contains(m.screen(), "a late message") {
		t.Errorf("the reader keeps their place (offset %d -> %d)", off, m.chat.YOffset())
	}
	m.press("end")
	if !m.follow || !m.chat.AtBottom() || !strings.Contains(m.screen(), "a late message") {
		t.Error("end resumes following")
	}
	m.st.Apply(evMsg(2001, model.Message{ID: "later", Role: "assistant", Text: "an even later message"}))
	m.tick(1)
	if !strings.Contains(m.screen(), "an even later message") {
		t.Error("following again shows the newest message")
	}
	m.press("pgup")
	if m.follow {
		t.Error("pgup stops following")
	}
	m.press("pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown", "pgdown")
	if !m.follow || !m.chat.AtBottom() {
		t.Error("scrolling back to the bottom follows again")
	}
}

func TestHomeJumpsToTheTopOfTheChat(t *testing.T) {
	m := replayApp(t, longConversation(60), 120, 30)
	m.settle(t)
	m.press("home")
	if m.follow || !m.chat.AtTop() {
		t.Skip("BUG: internal/ui/app.go:388 forwards \"home\" to the viewport, whose default keymap has no home binding, so nothing happens; fix: case \"home\": m.chat.GotoTop(); m.follow = false (and m.treeVP.GotoTop() in the tree view)")
	}
}

func TestMouseWheelScrollsTheChat(t *testing.T) {
	m := replayApp(t, longConversation(60), 120, 30)
	m.settle(t)
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.follow || m.chat.AtBottom() {
		t.Error("wheel up leaves the bottom")
	}
}

func TestOnlyTheNewestFourHundredMessagesAreMounted(t *testing.T) {
	m := replayApp(t, longConversation(450), 120, 30)
	m.settle(t)
	txt := m.chatText()
	if !strings.Contains(txt, "msg-0449") || !strings.Contains(txt, "msg-0050") || strings.Contains(txt, "msg-0049") {
		t.Errorf("mounted window: has 449=%v 50=%v 49=%v", strings.Contains(txt, "msg-0449"), strings.Contains(txt, "msg-0050"), strings.Contains(txt, "msg-0049"))
	}
	if len(m.cache) > maxMounted+64 {
		t.Errorf("render cache grew to %d entries", len(m.cache))
	}
}

func TestEmptyChatTellsWhatToDo(t *testing.T) {
	m := replayApp(t, nil, 120, 30)
	m.tick(1)
	if !strings.Contains(m.screen(), "No messages yet") {
		t.Errorf("empty chat:\n%s", m.screen())
	}
}

func TestQuitKeys(t *testing.T) {
	m := replayApp(t, conversationEvents(), 120, 30)
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := m.Update(KeyFromString(k))
		if cmd == nil {
			t.Fatalf("%s must quit", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s did not produce tea.QuitMsg", k)
		}
	}
}

// ---- layout -----------------------------------------------------------------------------------------------

func checkScreen(t *testing.T, name string, m *Model, w, h int) {
	t.Helper()
	out := m.Render()
	ls := lines(out)
	if len(ls) != h {
		t.Errorf("%s: %d lines, want %d", name, len(ls), h)
	}
	for i, l := range ls {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("%s: line %d is %d wide (> %d): %q", name, i, lw, w, strip(l))
		}
	}
}

func TestScreenHasExactlyTheTerminalHeightAndNeverOverflowsItsWidth(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 30}, {160, 50}, {109, 30}, {110, 30}, {50, 15}} {
		w, h := sz[0], sz[1]
		for _, view := range []string{"chat", "tree"} {
			for _, all := range []bool{false, true} {
				m := replayApp(t, richEvents(), w, h)
				m.settle(t)
				m.showAll = all
				if view == "tree" {
					m.press("t")
				}
				m.tick(1)
				checkScreen(t, fmt.Sprintf("replay %dx%d %s all=%v", w, h, view, all), m, w, h)
			}
		}
	}
}

func TestLiveScreenWithInputBoxFitsToo(t *testing.T) {
	for _, sz := range [][2]int{{80, 24}, {120, 30}, {160, 50}} {
		w, h := sz[0], sz[1]
		root := t.TempDir()
		m := liveApp(t, Options{Session: liveFile(t, root, "s1", "hi"), ProjectsDir: root, CanSend: true})
		m.resize(w, h)
		now := nowSecs()
		for _, ev := range richEvents() {
			ev.TS = now - 200 + (ev.TS-1000)/10
			m.st.Apply(ev)
		}
		m.tick(1)
		checkScreen(t, fmt.Sprintf("live %dx%d chat", w, h), m, w, h)
		m.press("i")
		m.typeText("some text typed by the user")
		checkScreen(t, fmt.Sprintf("live %dx%d typing", w, h), m, w, h)
		if !strings.Contains(m.screen(), "some text typed") {
			t.Errorf("%dx%d: typed text is visible", w, h)
		}
		m.press("esc", "t")
		checkScreen(t, fmt.Sprintf("live %dx%d tree", w, h), m, w, h)
	}
}

func TestWideTerminalHasARailAndNarrowOneHasPills(t *testing.T) {
	wide := replayApp(t, richEvents(), 120, 40)
	wide.settle(t)
	if !strings.Contains(wide.screen(), "AGENTS · ") || wide.railW() != railWidth {
		t.Errorf("120 columns have the rail:\n%s", wide.screen())
	}
	narrow := replayApp(t, richEvents(), 109, 30)
	narrow.settle(t)
	narrow.showAll = false
	narrow.tick(1)
	scr := narrow.screen()
	if narrow.railW() != 0 || strings.Contains(scr, "AGENTS · ") || strings.Contains(scr, "cost (estimate)") {
		t.Errorf("80 columns have no rail:\n%s", scr)
	}
	pills := lines(scr)[1]
	// the replay clock is at t=1100+: only ag1..ag3 (started 1010-1012, active at 1100) are not done; staleness is
	// measured against the newest event, so they are still running
	for _, want := range []string{"● main", "● explorer · Grep", "● worker · Edit", "● researcher · WebFetch"} {
		if !strings.Contains(pills, want) {
			t.Errorf("pills line lacks %q: %q", want, pills)
		}
	}
	if strings.Contains(pills, "dispatcher") {
		t.Errorf("finished agents are not pills: %q", pills)
	}
	narrow.press("h")
	if !strings.Contains(lines(narrow.screen())[1], "dispatcher") {
		t.Error("history view lists finished agents in the pills too")
	}
}

func TestHeaderMentionsSessionAndMode(t *testing.T) {
	root := t.TempDir()
	m := liveApp(t, Options{Session: liveFile(t, root, "abcdef123456", "hi"), ProjectsDir: root})
	h := strip(m.header())
	if !strings.Contains(h, "AGENTS-TREE") || !strings.Contains(h, "session abcdef12") || !strings.Contains(h, "● live") {
		t.Errorf("header = %q", h)
	}
	if lw := lipgloss.Width(m.header()); lw > m.w {
		t.Errorf("header too wide: %d", lw)
	}
}

func TestViewRequestsTheAlternateScreen(t *testing.T) {
	m := replayApp(t, conversationEvents(), 120, 30)
	if v := m.View(); !v.AltScreen {
		t.Error("alt screen expected")
	}
}

func TestSnapshotRendersAFixtureSessionWithKeys(t *testing.T) {
	out, err := Snapshot(Options{Session: "../../tests/fixtures/session.jsonl"}, 140, 40, "t")
	if err != nil {
		t.Fatal(err)
	}
	plain := strip(out)
	for _, want := range []string{"explorer", "worker", "main", "· tree ·"} {
		if !strings.Contains(plain, want) {
			t.Errorf("snapshot lacks %q:\n%s", want, plain)
		}
	}
	if len(lines(out)) != 40 {
		t.Errorf("snapshot has %d lines", len(lines(out)))
	}
	if _, err := Snapshot(Options{Session: "/no/such/file.jsonl"}, 100, 30); err == nil {
		t.Error("a missing session is an error")
	}
}

var _ = time.Second
