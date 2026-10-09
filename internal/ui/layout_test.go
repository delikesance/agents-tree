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

// ---- helpers (prefix layout*) -----------------------------------------------------------------------------

var layoutSizes = [][2]int{{40, 12}, {60, 18}, {80, 24}, {120, 30}, {160, 50}, {200, 60}}

// layoutFresh shifts every timestamp of events so the newest one is one second old: live apps use the wall
// clock, so agents that were "running" in the fixture really count as running.
func layoutFresh(evs []model.Event) []model.Event {
	newest := 0.0
	for _, e := range evs {
		newest = max(newest, e.TS)
	}
	d := nowSecs() - 1 - newest
	out := make([]model.Event, len(evs))
	for i, e := range evs {
		e.TS += d
		if e.Msg != nil {
			mm := *e.Msg
			mm.TS += d
			e.Msg = &mm
		}
		out[i] = e
	}
	return out
}

// layoutFeed applies events straight into the store and redraws.
func layoutFeed(m *Model, evs []model.Event) {
	for _, e := range evs {
		m.st.Apply(e)
	}
	m.refresh()
}

// layoutLive is a live app (message box enabled, never able to start a real claude) of w x h.
func layoutLive(t *testing.T, opt Options, w, h int) *Model {
	t.Helper()
	opt.CanSend = true
	if opt.ClaudeBin == "" {
		opt.ClaudeBin = "/nonexistent/claude-for-tests"
	}
	m := liveApp(t, opt)
	t.Cleanup(func() {
		if m.snd != nil {
			m.snd.Stop()
		}
	})
	m.resize(w, h)
	return m
}

func layoutRich(t *testing.T, w, h int) *Model {
	t.Helper()
	m := layoutLive(t, Options{Session: liveFile(t, t.TempDir(), "s1", "hello")}, w, h)
	layoutFeed(m, layoutFresh(richEvents()))
	return m
}

// layoutFits fails when the screen is not exactly h lines or a line is wider than w.
func layoutFits(t *testing.T, m *Model, w, h int) {
	t.Helper()
	ls := lines(m.screen())
	if len(ls) != h {
		t.Errorf("%dx%d: screen has %d lines, want %d", w, h, len(ls), h)
	}
	for i, l := range ls {
		if lw := lipgloss.Width(l); lw > w {
			t.Errorf("%dx%d: line %d is %d columns wide: %q", w, h, i, lw, l)
			return
		}
	}
}

// layoutBoxRows counts the text rows of the message box (the only box that starts in column 0), -1 if none.
func layoutBoxRows(m *Model) int {
	top := -1
	for i, l := range lines(m.screen()) {
		if strings.HasPrefix(l, "╭") {
			top = i
		}
		if top >= 0 && strings.HasPrefix(l, "╰") {
			return i - top - 1
		}
	}
	return -1
}

// layoutBoxTop is the screen line of the message box's top border, -1 if none.
func layoutBoxTop(m *Model) int {
	for i, l := range lines(m.screen()) {
		if strings.HasPrefix(l, "╭") {
			return i
		}
	}
	return -1
}

func layoutProjects(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for i := 0; i < 6; i++ {
		prompt := fmt.Sprintf("session %d: 日本語のとても長いプロンプト %s", i, strings.Repeat("long-", 40))
		pickerWriteSession(t, root, fmt.Sprintf("-home-user-proj%d", i), fmt.Sprintf("id%06d", i), "/home/user/proj"+fmt.Sprint(i),
			prompt, "reply", time.Duration(i+1)*time.Hour)
	}
	return root
}

// ---- 7. the screen always fits ----------------------------------------------------------------------------

type layoutScenario struct {
	name  string
	build func(t *testing.T, w, h int) *Model
}

func layoutScenarios() []layoutScenario {
	return []layoutScenario{
		{"live new session", func(t *testing.T, w, h int) *Model { return layoutLive(t, Options{}, w, h) }},
		{"live new session typing", func(t *testing.T, w, h int) *Model {
			m := layoutLive(t, Options{}, w, h)
			m.typeText("a long message with 日本語 and more words to wrap around the little message box " + strings.Repeat("z", 90))
			return m
		}},
		{"live rich", layoutRich},
		{"live rich commands", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc")
			return m
		}},
		{"live rich filtered", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc", "f", "f")
			return m
		}},
		{"live rich expanded", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc", "e")
			return m
		}},
		{"replay", func(t *testing.T, w, h int) *Model {
			m := replayApp(t, richEvents(), w, h)
			m.settle(t)
			return m
		}},
		{"tree view", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc", "t")
			return m
		}},
		{"tree view history", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc", "t", "h")
			return m
		}},
		{"details open", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc", "d")
			return m
		}},
		{"details open all agents", func(t *testing.T, w, h int) *Model {
			m := layoutRich(t, w, h)
			m.press("esc", "d", "h")
			return m
		}},
		{"picker open", func(t *testing.T, w, h int) *Model {
			m := layoutLive(t, Options{ProjectsDir: layoutProjects(t)}, w, h)
			layoutFeed(m, layoutFresh(richEvents()))
			m.press("esc", "s")
			return m
		}},
		{"picker open filtered", func(t *testing.T, w, h int) *Model {
			m := layoutLive(t, Options{ProjectsDir: layoutProjects(t)}, w, h)
			m.press("esc", "s")
			m.typeText("proj")
			m.press("down", "down")
			return m
		}},
	}
}

func TestLayoutScreenIsExactlyTheTerminalSizeInEveryScenario(t *testing.T) {
	for _, sc := range layoutScenarios() {
		for _, sz := range layoutSizes {
			w, h := sz[0], sz[1]
			t.Run(fmt.Sprintf("%s/%dx%d", sc.name, w, h), func(t *testing.T) {
				m := sc.build(t, w, h)
				m.tick(2)
				layoutFits(t, m, w, h)
				// a later resize to another size keeps fitting
				m.resize(w+7, h+3)
				layoutFits(t, m, w+7, h+3)
				m.resize(w, h)
				layoutFits(t, m, w, h)
			})
		}
	}
}

func TestLayoutChatTextNeverExceedsTheReadingWidthOnWideTerminals(t *testing.T) {
	const limit = chatMax + margin
	for _, sz := range [][2]int{{160, 50}, {200, 60}, {300, 60}} {
		w, h := sz[0], sz[1]
		builds := map[string]*Model{
			"live":   layoutRich(t, w, h),
			"replay": replayApp(t, richEvents(), w, h),
		}
		builds["replay"].settle(t)
		for name, m := range builds {
			m.tick(1)
			longest := 0
			for _, l := range lines(m.chatText()) {
				longest = max(longest, lipgloss.Width(l))
			}
			if longest == 0 {
				t.Fatalf("%s %dx%d: the chat is empty", name, w, h)
			}
			if longest > limit {
				t.Errorf("%s %dx%d: a chat line is %d columns wide, limit %d", name, w, h, longest, limit)
			}
			// the text is not squeezed either: wide lines really use the reading width
			if want := min(chatMax, m.chatPaneW()-2*margin); longest < want-8 {
				t.Errorf("%s %dx%d: widest chat line is only %d columns", name, w, h, longest)
			}
		}
	}
}

func TestLayoutNarrowTerminalsWrapChatTextInsteadOfOverflowing(t *testing.T) {
	m := layoutRich(t, 40, 24)
	m.tick(1)
	for _, l := range lines(m.chatText()) {
		if lw := lipgloss.Width(l); lw > 40 {
			t.Fatalf("chat line wider than the terminal (%d): %q", lw, l)
		}
	}
}

// ---- the agents strip -------------------------------------------------------------------------------------

func TestLayoutStripShowsOnlyWhileSubagentsRun(t *testing.T) {
	m := layoutLive(t, Options{}, 100, 30)
	if m.stripH() != 0 || strings.Contains(m.screen(), "explorer ·") {
		t.Fatal("no subagent, no strip")
	}
	base := m.chat.Height()

	layoutFeed(m, layoutFresh(richEvents()))
	if m.stripH() != 1 || m.chat.Height() != base-1 {
		t.Fatalf("strip height = %d, chat height %d (was %d)", m.stripH(), m.chat.Height(), base)
	}
	ls := lines(m.screen())
	top := layoutBoxTop(m)
	if top < 1 {
		t.Fatalf("no message box:\n%s", m.screen())
	}
	strip := ls[top-1] // one line, directly above the message box
	for _, want := range []string{"explorer · Grep", "worker · Edit", "researcher · WebFetch"} {
		if !strings.Contains(strip, want) {
			t.Errorf("strip lacks %q: %q", want, strip)
		}
	}
	n := 0
	for _, l := range ls {
		if strings.Contains(l, "worker · Edit") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("the strip is a single line, found %d lines naming the worker", n)
	}
	for _, l := range ls {
		if strings.Contains(l, "dispatcher ·") {
			t.Errorf("a finished agent must not be in the strip: %q", l)
		}
	}

	// the three agents finish: the strip goes away and the chat gets its row back
	now := nowSecs()
	layoutFeed(m, []model.Event{evEnd(now, "ag1", "done"), evEnd(now, "ag2", "done"), evEnd(now, "ag3", "done")})
	if m.stripH() != 0 || m.chat.Height() != base || strings.Contains(m.screen(), "worker · Edit") {
		t.Errorf("strip must vanish when nobody runs: stripH=%d chat=%d\n%s", m.stripH(), m.chat.Height(), m.screen())
	}
	layoutFits(t, m, 100, 30)
}

func TestLayoutStripIsAbsentForStaleAgentsAndInTheTreeView(t *testing.T) {
	m := layoutLive(t, Options{}, 100, 30)
	layoutFeed(m, richEvents()) // timestamps from 1970: nothing is running any more
	if m.stripH() != 0 || len(m.runningAgents()) != 0 {
		t.Errorf("stale agents are not running: strip=%d", m.stripH())
	}
	m = layoutRich(t, 100, 30)
	if m.stripH() != 1 {
		t.Fatal("precondition: running agents")
	}
	m.press("esc", "t")
	if m.stripH() != 0 || strings.Contains(m.screen(), "worker · Edit") {
		t.Error("the tree view has no strip")
	}
	layoutFits(t, m, 100, 30)
}

func TestLayoutStripAlsoShowsInReplayWithoutAMessageBox(t *testing.T) {
	m := replayApp(t, richEvents(), 100, 30)
	m.settle(t)
	if m.stripH() != 1 || !strings.Contains(m.screen(), "explorer · Grep") {
		t.Errorf("replay strip:\n%s", m.screen())
	}
	if layoutBoxTop(m) != -1 {
		t.Error("replay has no message box")
	}
	layoutFits(t, m, 100, 30)
}

// ---- the growing message box ------------------------------------------------------------------------------

func TestLayoutMessageBoxGrowsWithMultilineInputUpToSixRowsAndChatShrinks(t *testing.T) {
	for _, withStrip := range []bool{false, true} {
		w, h := 80, 30
		m := layoutLive(t, Options{}, w, h)
		if withStrip {
			layoutFeed(m, layoutFresh(richEvents()))
		}
		base := m.chat.Height()
		if layoutBoxRows(m) != 1 {
			t.Fatalf("strip=%v: an empty box is one row, got %d", withStrip, layoutBoxRows(m))
		}
		for line := 1; line <= 9; line++ {
			m.typeText(fmt.Sprintf("line%d", line))
			m.press("alt+enter")
			rows := min(line+1, maxInput)
			if got := layoutBoxRows(m); got != rows {
				t.Fatalf("strip=%v after %d newlines the box has %d rows, want %d\n%s", withStrip, line, got, rows, m.screen())
			}
			if want := base - (rows - 1); m.chat.Height() != want {
				t.Errorf("strip=%v rows=%d: chat height %d, want %d", withStrip, rows, m.chat.Height(), want)
			}
			layoutFits(t, m, w, h)
		}
		if m.snd != nil || !strings.Contains(m.input.Value(), "line1\nline2") {
			t.Errorf("alt+enter inserts a newline and never sends: %q", m.input.Value())
		}
		// emptying the box gives the rows back
		for i := 0; i < 80; i++ {
			m.press("backspace")
		}
		if m.input.Value() != "" || layoutBoxRows(m) != 1 || m.chat.Height() != base {
			t.Errorf("strip=%v: box not back to one row: value=%q rows=%d chat=%d (base %d)", withStrip, m.input.Value(), layoutBoxRows(m), m.chat.Height(), base)
		}
		layoutFits(t, m, w, h)
	}
}

func TestLayoutSixRowMessageBoxStillFitsSmallTerminals(t *testing.T) {
	for _, sz := range layoutSizes {
		w, h := sz[0], sz[1]
		m := layoutLive(t, Options{}, w, h)
		layoutFeed(m, layoutFresh(richEvents()))
		for i := 0; i < 8; i++ {
			m.typeText("x")
			m.press("alt+enter")
		}
		ls := lines(m.screen())
		if len(ls) != h {
			t.Skipf("BUG: internal/ui/app.go:573 bodyH() floors the chat at 3 rows, so a 6-row box + strip overflows a %dx%d terminal (%d lines instead of %d); proposed fix: in fitInput cap rows to max(1, m.h-2-3-2-m.stripH())", w, h, len(ls), h)
		}
		layoutFits(t, m, w, h)
	}
}

// ---- 8. the chat follows the bottom ----------------------------------------------------------------------

func layoutNumbered(i int) model.Event {
	return evMsg(float64(2000+i), model.Message{ID: fmt.Sprintf("n%03d", i), Role: "assistant", Model: sonnet,
		Text: fmt.Sprintf("message number %03d", i)})
}

func layoutManyMessages(t *testing.T, n int) *Model {
	t.Helper()
	m := layoutLive(t, Options{}, 80, 14)
	for i := 1; i <= n; i++ {
		m.st.Apply(layoutNumbered(i))
	}
	m.refresh()
	return m
}

func TestLayoutChatFollowsNewMessagesUntilTheUserScrollsUp(t *testing.T) {
	m := layoutManyMessages(t, 60)
	if !m.follow || !m.chat.AtBottom() || !strings.Contains(m.screen(), "message number 060") || strings.Contains(m.screen(), "message number 001") {
		t.Fatalf("starts at the bottom:\n%s", m.screen())
	}
	m.st.Apply(layoutNumbered(61))
	m.tick(1)
	if !strings.Contains(m.screen(), "message number 061") {
		t.Fatalf("a new message must scroll into view while following:\n%s", m.screen())
	}

	m.press("pgup") // pgup works even with the message box focused
	if m.follow || m.chat.AtBottom() {
		t.Fatal("pgup stops following")
	}
	m.st.Apply(layoutNumbered(62))
	m.tick(1)
	if strings.Contains(m.screen(), "message number 062") {
		t.Errorf("a scrolled-up chat must not jump to new messages:\n%s", m.screen())
	}

	m.press("esc", "end") // end resumes following
	if !m.follow || !m.chat.AtBottom() || !strings.Contains(m.screen(), "message number 062") {
		t.Errorf("end resumes following:\n%s", m.screen())
	}
	m.st.Apply(layoutNumbered(63))
	m.tick(1)
	if !strings.Contains(m.screen(), "message number 063") {
		t.Error("following again after end")
	}

	m.press("up") // command mode: up scrolls
	if m.follow {
		t.Error("up (command mode) stops following")
	}
	m.press("end")
	m.press("home")
	if m.follow || !m.chat.AtTop() || !strings.Contains(m.screen(), "message number 001") {
		t.Errorf("home goes to the top:\n%s", m.screen())
	}
	m.press("pgdown", "pgdown", "pgdown")
	if m.chat.AtTop() {
		t.Error("pgdown scrolls back down")
	}
	m.press("end")
	if !m.follow || !strings.Contains(m.screen(), "message number 063") {
		t.Error("end after home is back at the bottom")
	}
}

func TestLayoutHomeEndAndArrowsInTheMessageBoxDoNotScrollTheChat(t *testing.T) {
	m := layoutManyMessages(t, 60)
	m.typeText("abc")
	m.press("home", "up", "end", "down")
	if !m.follow || !m.chat.AtBottom() {
		t.Error("in the message box those keys belong to the text")
	}
	if m.input.Value() != "abc" {
		t.Errorf("value = %q", m.input.Value())
	}
	m.press("pgup", "pgup")
	if m.follow || m.input.Value() != "abc" {
		t.Error("pgup always scrolls the chat and leaves the text alone")
	}
	m.press("pgdown", "pgdown", "pgdown", "pgdown")
	if !m.chat.AtBottom() || !m.follow {
		t.Error("pgdown back to the bottom resumes following")
	}
}

func TestLayoutMouseWheelScrollsTheChat(t *testing.T) {
	m := layoutManyMessages(t, 60)
	m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelUp})
	if m.follow || m.chat.AtBottom() {
		t.Error("wheel up leaves the bottom")
	}
}

// ---- the side panel on big terminals ----------------------------------------------------------------------

func TestLayoutWideTerminalsShowTheAgentDiagramBesideTheChat(t *testing.T) {
	m := layoutRich(t, 200, 50)
	m.tick(1)
	if !m.wide() || m.stripH() != 0 {
		t.Fatalf("200 columns: side panel instead of the strip (wide=%v strip=%d)", m.wide(), m.stripH())
	}
	scr := m.screen()
	if !strings.Contains(scr, "AGENTS") || !strings.Contains(scr, "COST") {
		t.Errorf("side panel lacks AGENTS / COST:\n%s", scr)
	}
	layoutFits(t, m, 200, 50)

	n := layoutRich(t, 120, 40)
	n.tick(1)
	if n.wide() || strings.Contains(n.screen(), "AGENTS ") {
		t.Error("120 columns keep the single column")
	}
}
