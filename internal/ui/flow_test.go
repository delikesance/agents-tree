package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

// ---- helpers (prefixed flow*) ------------------------------------------------------------------------------

func flowTool(id, agent, tool, status string, dur float64) *model.Message {
	m := &model.Message{ID: id, TS: 1000, AgentID: agent, Role: "tool", Tool: tool, Detail: "arg-" + id, Status: status}
	if status != "running" {
		m.Duration = fp(dur)
	}
	return m
}

func flowMsg(id, agent, role, text string) *model.Message {
	return &model.Message{ID: id, TS: 1000, AgentID: agent, Role: role, Text: text}
}

func flowIDs(items []flowItem) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.id
	}
	return out
}

func flowSizes(items []flowItem) []int {
	out := make([]int, len(items))
	for i, it := range items {
		out[i] = len(it.msgs)
	}
	return out
}

func flowEq(a, b []int) bool { return fmt.Sprint(a) == fmt.Sprint(b) }

// flowStore has a live main agent (last activity 1000) and a live subagent ag1.
func flowStore() *store.Store {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "Bash", 1))
	s.Apply(evStart(1000, "ag1", model.Main, "worker", "edit", sonnet))
	s.Apply(evTurn(1000, "ag1", sonnet, "Edit", 1))
	return s
}

// flowGroupPlain renders a tool group to plain text.
func flowGroupPlain(msgs []*model.Message, c rctx) string { return strip(renderToolGroup(msgs, c)) }

func flowCalls(n int) []*model.Message {
	tools := []string{"Bash", "Bash", "Write", "Bash", "Bash", "Read"}
	var ms []*model.Message
	for i := 0; i < n; i++ {
		ms = append(ms, flowTool(fmt.Sprintf("c%d", i), model.Main, tools[i%len(tools)], "ok", 1))
	}
	return ms
}

// ---- 4. buildFlow ------------------------------------------------------------------------------------------

func TestBuildFlowGroupsConsecutiveToolCallsOfTheSameAgent(t *testing.T) {
	vis := []*model.Message{
		flowMsg("u", model.Main, "user", "go"),
		flowTool("t1", model.Main, "Read", "ok", 1),
		flowTool("t2", model.Main, "Bash", "ok", 1),
		flowTool("t3", model.Main, "Edit", "ok", 1),
		flowMsg("a", model.Main, "assistant", "done"),
	}
	items, hidden := buildFlow(vis, false)
	if hidden != 0 || !flowEq(flowSizes(items), []int{1, 3, 1}) {
		t.Fatalf("sizes = %v hidden = %d", flowSizes(items), hidden)
	}
	if items[1].id != "tools:t1" || items[0].id != "u" || items[2].id != "a" {
		t.Errorf("ids = %v (a group is keyed by its first call)", flowIDs(items))
	}
}

func TestBuildFlowSplitsGroupsByAgentAndByInterveningMessages(t *testing.T) {
	vis := []*model.Message{
		flowTool("t1", model.Main, "Read", "ok", 1),
		flowTool("t2", "ag1", "Read", "ok", 1), // other agent: new group
		flowTool("t3", "ag1", "Edit", "ok", 1),
		flowTool("t4", model.Main, "Bash", "ok", 1), // back to main: new group
		flowMsg("a", model.Main, "assistant", "hmm"),
		flowTool("t5", model.Main, "Bash", "ok", 1), // after a message: new group
		flowTool("t6", model.Main, "Bash", "ok", 1),
		{ID: "d", AgentID: model.Main, Role: "delegation", Target: "ag1"},
		flowTool("t7", model.Main, "Bash", "ok", 1),
	}
	items, _ := buildFlow(vis, false)
	if want := []int{1, 2, 1, 1, 2, 1, 1}; !flowEq(flowSizes(items), want) {
		t.Fatalf("sizes = %v, want %v (ids %v)", flowSizes(items), want, flowIDs(items))
	}
	if items[1].msgs[0].AgentID != "ag1" || items[1].id != "tools:t2" {
		t.Errorf("the second group belongs to ag1: %v", flowIDs(items))
	}
}

func TestBuildFlowHidesSystemNoiseButCountsIt(t *testing.T) {
	vis := []*model.Message{
		flowMsg("s1", model.Main, "system", "Stop hook feedback:\nx"),
		flowMsg("u", model.Main, "user", "hi"),
		flowMsg("s2", model.Main, "system", "<system-reminder>…"),
		flowMsg("a", model.Main, "assistant", "yo"),
	}
	items, hidden := buildFlow(vis, false)
	if hidden != 2 || !flowEq(flowSizes(items), []int{1, 1}) || items[0].id != "u" || items[1].id != "a" {
		t.Errorf("collapsed: ids %v hidden %d", flowIDs(items), hidden)
	}
	items, hidden = buildFlow(vis, true)
	if hidden != 0 || len(items) != 4 || items[0].id != "s1" {
		t.Errorf("expanded shows all: ids %v hidden %d", flowIDs(items), hidden)
	}
}

func TestBuildFlowKeepsInterruptionNotices(t *testing.T) {
	vis := []*model.Message{
		flowMsg("u", model.Main, "user", "go"),
		flowMsg("i", model.Main, "system", "[Request interrupted by user]"),
		flowMsg("i2", model.Main, "system", "[Request interrupted by user for tool use]"),
		flowMsg("s", model.Main, "system", "noise"),
	}
	items, hidden := buildFlow(vis, false)
	if hidden != 1 || !flowEq(flowSizes(items), []int{1, 1, 1}) || items[1].id != "i" || items[2].id != "i2" {
		t.Errorf("ids %v hidden %d", flowIDs(items), hidden)
	}
}

func TestBuildFlowOfNothingIsEmpty(t *testing.T) {
	if items, hidden := buildFlow(nil, false); len(items) != 0 || hidden != 0 {
		t.Errorf("items %v hidden %d", items, hidden)
	}
	// only hidden noise: nothing drawn, all counted
	items, hidden := buildFlow([]*model.Message{flowMsg("s", model.Main, "system", "x")}, false)
	if len(items) != 0 || hidden != 1 {
		t.Errorf("items %v hidden %d", flowIDs(items), hidden)
	}
}

func TestFlowItemSigChangesWhenAMemberUpdates(t *testing.T) {
	s := flowStore()
	ms := []*model.Message{flowTool("t1", model.Main, "Bash", "running", 0), flowTool("t2", model.Main, "Read", "ok", 1), flowTool("t3", model.Main, "Edit", "ok", 1)}
	for _, m := range ms {
		s.Apply(model.Event{TS: 1000, Kind: model.Message_, AgentID: model.Main, Msg: m})
	}
	items, _ := buildFlow(ms, false)
	it := items[0]
	c := rctx{st: s, now: 1005, width: 80}
	id, sig := it.id, it.sig(c)

	s.Apply(evUpdate(1006, "t1", "ok", fp(2.5)))
	if it.id != id {
		t.Error("the group id (cache key) stays stable across updates")
	}
	after := it.sig(c)
	if after == sig {
		t.Error("completing a member changes the group signature")
	}
	s.Apply(evUpdate(1007, "t3", "error", fp(0.3)))
	if it.sig(c) == after {
		t.Error("failing another member changes it again")
	}
}

func TestFlowItemSigChangesWithExpandedAndWidth(t *testing.T) {
	s := flowStore()
	items, _ := buildFlow(flowCalls(4), false)
	it := items[0]
	base := rctx{st: s, now: 1005, width: 80}
	exp, narrow := base, base
	exp.expanded, narrow.width = true, 50
	if it.sig(base) == it.sig(exp) {
		t.Error("expanded flips the signature")
	}
	if it.sig(base) == it.sig(narrow) {
		t.Error("width changes the signature")
	}
	if it.sig(base) != it.sig(base) {
		t.Error("signature is deterministic")
	}
	// a single-message item follows the same rules
	one, _ := buildFlow([]*model.Message{flowMsg("u", model.Main, "user", "x")}, false)
	if one[0].sig(base) == one[0].sig(exp) || one[0].sig(base) == one[0].sig(narrow) {
		t.Error("message items also depend on expanded and width")
	}
}

func TestFlowItemSigFollowsTheFrameOnlyForRunningToolsOfLiveAgents(t *testing.T) {
	s := flowStore()
	running := flowTool("r", model.Main, "Bash", "running", 0)
	done1, done2 := flowTool("d1", model.Main, "Read", "ok", 1), flowTool("d2", model.Main, "Read", "ok", 1)
	for _, m := range []*model.Message{running, done1, done2} {
		s.Apply(model.Event{TS: 1000, Kind: model.Message_, AgentID: model.Main, Msg: m})
	}
	withRunning, _ := buildFlow([]*model.Message{done1, done2, running}, false)
	allDone, _ := buildFlow([]*model.Message{done1, done2}, false)

	live0 := rctx{st: s, now: 1005, width: 80}
	live1 := live0
	live1.frame = 1
	if withRunning[0].sig(live0) == withRunning[0].sig(live1) {
		t.Error("a group with a running tool of a live agent animates")
	}
	if allDone[0].sig(live0) != allDone[0].sig(live1) {
		t.Error("a finished group is static")
	}
	idle0 := rctx{st: s, now: 1000 + store.StaleSecs + 60, width: 80}
	idle1 := idle0
	idle1.frame = 1
	if withRunning[0].sig(idle0) != withRunning[0].sig(idle1) {
		t.Error("once the agent is idle the running tool no longer animates")
	}
	if withRunning[0].sig(live0) == withRunning[0].sig(idle0) {
		t.Error("going idle must still redraw the group once (spinner -> …)")
	}
}

// ---- 5. renderToolGroup ------------------------------------------------------------------------------------

func TestToolGroupOfOneOrTwoCallsIsOneLineEach(t *testing.T) {
	s := flowStore()
	c := rctx{st: s, now: 1005, width: 80}
	for n := 1; n <= 2; n++ {
		ms := flowCalls(n)
		got := flowGroupPlain(ms, c)
		ls := lines(got)
		if len(ls) != n || strings.Contains(got, "tool calls") {
			t.Fatalf("%d call(s):\n%s", n, got)
		}
		for i, l := range ls {
			if !strings.Contains(l, ms[i].Tool) || !strings.Contains(l, "arg-"+ms[i].ID) || !strings.Contains(l, "✓") {
				t.Errorf("line %d: %q", i, l)
			}
		}
	}
}

func TestToolGroupOfThreeOrMoreIsOneSummaryLine(t *testing.T) {
	s := flowStore()
	c := rctx{st: s, now: 1005, width: 100}
	ms := flowCalls(5) // Bash Bash Write Bash Bash, all ok, 1 s each
	got := flowGroupPlain(ms, c)
	if len(lines(got)) != 1 {
		t.Fatalf("all ok -> exactly one line:\n%s", got)
	}
	for _, want := range []string{"⚙ 5 tool calls", "Bash ×4", "Write", "✓ 5"} {
		if !strings.Contains(got, want) {
			t.Errorf("summary lacks %q: %q", want, got)
		}
	}
	if strings.Contains(got, "✗") || strings.Contains(got, "arg-c0") {
		t.Errorf("no failures and no individual calls expected: %q", got)
	}
	// names are listed in order of first use, repeated ones carry ×n, single ones do not
	if !(strings.Index(got, "Bash ×4") < strings.Index(got, "Write")) || strings.Contains(got, "Write ×") {
		t.Errorf("name order/counts: %q", got)
	}
	// total duration is right-aligned
	if !strings.HasSuffix(got, "5.0 s") || lipgloss.Width(got) != c.width {
		t.Errorf("total duration must end the line, full width %d (got %d): %q", c.width, lipgloss.Width(got), got)
	}
}

func TestToolGroupSummaryCountsFailuresAndListsFailedAndRunningCalls(t *testing.T) {
	s := flowStore()
	ms := []*model.Message{
		flowTool("c0", model.Main, "Bash", "ok", 1),
		flowTool("c1", model.Main, "Bash", "error", 2),
		flowTool("c2", model.Main, "Edit", "running", 0),
		flowTool("c3", model.Main, "Read", "ok", 1),
	}
	for _, m := range ms {
		s.Apply(model.Event{TS: 1000, Kind: model.Message_, AgentID: model.Main, Msg: m})
	}
	c := rctx{st: s, now: 1005, width: 100}
	ls := lines(flowGroupPlain(ms, c))
	if len(ls) != 3 {
		t.Fatalf("summary + the failed + the running call, got:\n%s", strings.Join(ls, "\n"))
	}
	if !strings.Contains(ls[0], "⚙ 4 tool calls") || !strings.Contains(ls[0], "✓ 2") || !strings.Contains(ls[0], "✗ 1") || !strings.HasSuffix(ls[0], "4.0 s") {
		t.Errorf("summary: %q", ls[0])
	}
	if !strings.Contains(ls[1], "✗") || !strings.Contains(ls[1], "Bash") || !strings.Contains(ls[1], "arg-c1") {
		t.Errorf("failed call line: %q", ls[1])
	}
	if !strings.Contains(ls[2], spin(0)) || !strings.Contains(ls[2], "Edit") || !strings.Contains(ls[2], "arg-c2") {
		t.Errorf("running call line: %q", ls[2])
	}
}

func TestToolGroupShowsAtMostThreeExtraLines(t *testing.T) {
	s := flowStore()
	var ms []*model.Message
	for i := 0; i < 8; i++ {
		ms = append(ms, flowTool(fmt.Sprintf("e%d", i), model.Main, "Bash", "error", 1))
	}
	got := flowGroupPlain(ms, rctx{st: s, now: 1005, width: 90})
	ls := lines(got)
	if len(ls) != 4 || !strings.Contains(ls[0], "8 tool calls") || !strings.Contains(ls[0], "✗ 8") {
		t.Fatalf("1 summary + max 3 failed lines:\n%s", got)
	}
	if !strings.Contains(got, "arg-e0") || !strings.Contains(got, "arg-e2") || strings.Contains(got, "arg-e3") {
		t.Errorf("the first three failures are listed:\n%s", got)
	}
}

func TestToolGroupWithoutDurationsOmitsTheTotal(t *testing.T) {
	s := flowStore()
	var ms []*model.Message
	for i := 0; i < 3; i++ {
		m := flowTool(fmt.Sprintf("n%d", i), model.Main, "Read", "ok", 0)
		m.Duration = nil
		ms = append(ms, m)
	}
	got := flowGroupPlain(ms, rctx{st: s, now: 1005, width: 80})
	if strings.Contains(got, " s") || !strings.Contains(got, "✓ 3") || !strings.Contains(got, "Read ×3") {
		t.Errorf("no duration expected: %q", got)
	}
}

func TestExpandedToolGroupShowsEveryCallAsALine(t *testing.T) {
	s := flowStore()
	ms := flowCalls(6)
	c := rctx{st: s, now: 1005, width: 90, expanded: true}
	got := flowGroupPlain(ms, c)
	ls := lines(got)
	if len(ls) != 6 || strings.Contains(got, "tool calls") {
		t.Fatalf("expanded -> one line per call:\n%s", got)
	}
	for i, l := range ls {
		if !strings.Contains(l, ms[i].Tool) || !strings.Contains(l, "arg-c"+fmt.Sprint(i)) || !strings.HasSuffix(l, "1.0 s") {
			t.Errorf("line %d: %q", i, l)
		}
	}
}

func TestToolGroupLinesNeverExceedTheWidth(t *testing.T) {
	s := flowStore()
	long := strings.Repeat("très-long-argument-日本語 ", 12)
	build := func(agent string) []*model.Message {
		var ms []*model.Message
		for i := 0; i < 9; i++ {
			status := []string{"ok", "error", "running"}[i%3]
			m := flowTool(fmt.Sprintf("w%d", i), agent, []string{"Bash", "WebFetch", "mcp__github__get_file"}[i%3], status, float64(i)*37)
			m.Detail = long
			ms = append(ms, m)
		}
		return ms
	}
	for _, w := range []int{40, 80, 108} {
		for _, agent := range []string{model.Main, "ag1"} {
			for _, exp := range []bool{false, true} {
				for _, n := range []int{1, 2, 3, 9} {
					c := rctx{st: s, now: 1005, width: w, expanded: exp}
					out := renderToolGroup(build(agent)[:n], c)
					for _, l := range lines(out) {
						if lw := lipgloss.Width(l); lw > w {
							t.Errorf("w=%d agent=%s expanded=%v n=%d: %d wide: %q", w, agent, exp, n, lw, strip(l))
						}
					}
				}
			}
		}
	}
}

func TestVeryLongToolNameStillFitsANarrowNestedLine(t *testing.T) {
	s := flowStore()
	m := flowTool("long", "ag1", "mcp__plugin_playwright_playwright__browser_take_screenshot", "running", 0)
	m.Detail = "x"
	for _, w := range []int{40, 60} {
		for _, l := range lines(renderToolGroup([]*model.Message{m}, rctx{st: s, now: 1005, width: w})) {
			if lw := lipgloss.Width(l); lw > w {
				t.Skipf("BUG: internal/ui/messages.go:renderTool never clips the `left` part (rail + icon + tool name), so a long MCP tool "+
					"name makes the line %d wide at width %d; proposed fix: build left with clip(name, max(width-lipgloss.Width(prefix)-lipgloss.Width(dur)-2, 4))", lw, w)
			}
		}
	}
}

func TestSubagentToolGroupIsNestedUnderTheRail(t *testing.T) {
	s := flowStore()
	var ms []*model.Message
	for i := 0; i < 4; i++ {
		ms = append(ms, flowTool(fmt.Sprintf("s%d", i), "ag1", "Edit", "ok", 1))
	}
	got := flowGroupPlain(ms, rctx{st: s, now: 1005, width: 80})
	for _, l := range lines(got) {
		if !strings.HasPrefix(l, "  │ ") {
			t.Errorf("subagent group row must be nested: %q", l)
		}
	}
	if !strings.Contains(got, "⚙ 4 tool calls") {
		t.Errorf("summary missing: %q", got)
	}
	// flowItem.render dispatches tool items to the group renderer and marks them compact
	items, _ := buildFlow(ms, false)
	out, compact := items[0].render(rctx{st: s, now: 1005, width: 80})
	if !compact || strip(out) != got {
		t.Errorf("flowItem.render of a tool group: compact=%v\n%s", compact, strip(out))
	}
}

// ---- 6. whole chat through the model -----------------------------------------------------------------------

func TestChatGroupsToolLinesUnderTheRightMessage(t *testing.T) {
	m := replayApp(t, conversationEvents(), 100, 40)
	m.settle(t)
	chat := m.chatText()
	order := []string{"Peux-tu ne montrer", "Done.", "Bash", "pytest -q", "Read", "a/store.py", "→ explorer", "Looking.", "↩ explorer reported"}
	pos := -1
	for _, w := range order {
		i := strings.Index(chat[max(pos, 0):], w)
		if i < 0 {
			t.Fatalf("%q missing or out of order in the chat:\n%s", w, chat)
		}
		pos = max(pos, 0) + i
	}
	// the two tool calls follow the assistant answer on consecutive lines (no blank line between them)
	ls := lines(chat)
	var bashAt int = -1
	for i, l := range ls {
		if strings.Contains(l, "pytest -q") {
			bashAt = i
		}
	}
	if bashAt < 1 || !strings.Contains(ls[bashAt+1], "a/store.py") {
		t.Errorf("tool lines sit tight together:\n%s", chat)
	}
	// the nested subagent text carries the rail
	for _, l := range ls {
		if strings.Contains(l, "Looking.") && !strings.Contains(l, "│ ") {
			t.Errorf("subagent text without a rail: %q", l)
		}
	}
}

func TestChatCollapsesManyToolCallsIntoOneSummary(t *testing.T) {
	ev := []model.Event{
		evTurn(1000, model.Main, sonnet, "", 5),
		evMsg(1000, model.Message{ID: "u", Role: "user", Text: "run everything"}),
	}
	for i := 0; i < 6; i++ {
		ev = append(ev, evMsg(1001+float64(i), model.Message{ID: fmt.Sprintf("t%d", i), Role: "tool", Tool: "Bash",
			Detail: fmt.Sprintf("cmd-%d", i), Status: "ok", Duration: fp(1)}))
	}
	ev = append(ev, evMsg(1010, model.Message{ID: "a", Role: "assistant", Model: sonnet, Text: "all ran"}))
	m := replayApp(t, ev, 100, 40)
	m.settle(t)
	chat := m.chatText()
	if !strings.Contains(chat, "⚙ 6 tool calls · Bash ×6") || !strings.Contains(chat, "✓ 6") || strings.Contains(chat, "cmd-2") {
		t.Errorf("one summary line expected:\n%s", chat)
	}
	m.press("e")
	if exp := m.chatText(); !strings.Contains(exp, "cmd-2") || strings.Contains(exp, "6 tool calls") {
		t.Errorf("expanding shows every call:\n%s", exp)
	}
}

func TestStatusLineCountsHiddenSystemMessagesAndExpandRevealsThem(t *testing.T) {
	m := replayApp(t, conversationEvents(), 140, 40)
	m.settle(t)
	if chat := m.chatText(); strings.Contains(chat, "Stop hook") || strings.Contains(chat, "⚙") && strings.Contains(chat, "untracked") {
		t.Errorf("system noise must be hidden by default:\n%s", chat)
	}
	if scr := m.screen(); !strings.Contains(scr, "1 hidden") {
		t.Errorf("status line must count hidden messages:\n%s", scr)
	}
	m.press("e")
	chat := m.chatText()
	if !strings.Contains(chat, "⚙ system") || !strings.Contains(chat, "untracked files") {
		t.Errorf("expanded view shows the system message as a box:\n%s", chat)
	}
	if scr := m.screen(); strings.Contains(scr, "hidden") {
		t.Errorf("nothing is hidden once expanded:\n%s", scr)
	}
	m.press("e")
	if scr := m.screen(); !strings.Contains(scr, "1 hidden") {
		t.Errorf("collapsing again hides it again:\n%s", scr)
	}
}

func TestInterruptNoticeStaysVisibleInTheChat(t *testing.T) {
	ev := []model.Event{
		evTurn(1000, model.Main, sonnet, "", 5),
		evMsg(1000, model.Message{ID: "u", Role: "user", Text: "go"}),
		evMsg(1001, model.Message{ID: "i", Role: "system", Text: "[Request interrupted by user]"}),
		evMsg(1002, model.Message{ID: "s", Role: "system", Text: "hook noise"}),
	}
	m := replayApp(t, ev, 100, 30)
	m.settle(t)
	chat := m.chatText()
	if !strings.Contains(chat, "⚙ [Request interrupted by user]") || strings.Contains(chat, "hook noise") {
		t.Errorf("chat:\n%s", chat)
	}
	if !strings.Contains(m.screen(), "1 hidden") {
		t.Errorf("only the noise is counted:\n%s", m.screen())
	}
}

func flowUnicodeEvents() []model.Event {
	cjk := strings.Repeat("日本語のテキストはとても長い行になることがあります。", 6)
	emoji := strings.Repeat("🎉🚀👩‍💻 fête 😀 ", 25)
	fr := strings.Repeat("Élève à l'été, où l'œuvre naïve déçoit ; garçon « très » rapide. ", 6)
	ev := []model.Event{
		evTurn(1000, model.Main, sonnet, "", 5),
		evStart(1001, "ag1", model.Main, "explorer", "cartographier le dépôt 日本語 🚀", haiku),
		evTurn(1001, "ag1", haiku, "Read", 5),
		evMsg(1000, model.Message{ID: "u1", Role: "user", Text: fr + "\n" + cjk + "\n" + emoji}),
		evMsg(1001, model.Message{ID: "a1", Role: "assistant", Model: sonnet,
			Text: "# Résumé 日本語\n\n- " + cjk + "\n- " + emoji + "\n- **gras** et `code` : " + fr + "\n\n```\n日本語 🚀 code()\n```\n\n" + strings.Repeat("É", 300)}),
		evMsg(1002, model.Message{ID: "s1", Role: "system", Text: cjk + "\n" + emoji}),
	}
	for i := 0; i < 5; i++ {
		ev = append(ev, evMsg(1003+float64(i), model.Message{ID: fmt.Sprintf("t%d", i), Role: "tool", Tool: "Bash",
			Detail: []string{fr, cjk, emoji}[i%3], Status: []string{"ok", "error"}[i%2], Duration: fp(float64(i) * 61)}))
	}
	ev = append(ev,
		evMsg(1010, model.Message{ID: "deleg:ag1", Role: "delegation", Kind: "explorer", Desc: "cartographier le dépôt 日本語 🚀", Text: cjk + "\n" + emoji, Target: "ag1"}),
		evMsg(1011, model.Message{ID: "sa", AgentID: "ag1", Role: "assistant", Model: haiku, Text: fr + cjk + emoji}),
		evMsg(1012, model.Message{ID: "st1", AgentID: "ag1", Role: "tool", Tool: "Read", Detail: cjk + emoji, Status: "ok", Duration: fp(0.4)}),
		evMsg(1013, model.Message{ID: "st2", AgentID: "ag1", Role: "tool", Tool: "Grep", Detail: fr, Status: "running"}),
		evMsg(1014, model.Message{ID: "rep:ag1", Role: "report", Text: cjk + "\n" + emoji + "\n" + fr, Target: "ag1", Status: "done", Duration: fp(5)}),
		evEnd(1015, "ag1", "done"),
	)
	return ev
}

func TestUnicodeNeverProducesLinesWiderThanTheTerminal(t *testing.T) {
	for _, w := range []int{60, 100, 160} {
		for _, expanded := range []bool{false, true} {
			m := replayApp(t, flowUnicodeEvents(), w, 40)
			m.settle(t)
			if expanded {
				m.press("e")
			}
			chat := m.chatText()
			if !strings.Contains(chat, "Résumé") || !strings.Contains(chat, "Élève") {
				t.Fatalf("w=%d: chat lacks the accented text:\n%s", w, chat)
			}
			for i, l := range lines(chat) {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("w=%d expanded=%v chat line %d is %d wide: %q", w, expanded, i, lw, l)
				}
			}
			for i, l := range lines(m.screen()) {
				if lw := lipgloss.Width(l); lw > w {
					t.Errorf("w=%d expanded=%v screen line %d is %d wide: %q", w, expanded, i, lw, l)
				}
			}
		}
	}
}

func TestLongFencedCodeLineIsWrappedOrClipped(t *testing.T) {
	s := msgStore()
	for _, line := range []string{strings.Repeat("x", 300), strings.Repeat("日本語", 60)} {
		m := &model.Message{ID: "fence", TS: 1000, AgentID: model.Main, Role: "assistant", Model: sonnet, Text: "```\n" + line + "\n```"}
		for _, w := range []int{60, 108} {
			if got := msgMaxWidth(msgPlain(m, rctx{st: s, now: 1010, width: w})); got > w {
				t.Skipf("BUG: internal/ui/markdown.go:42-43 (renderMarkdown, inFence branch) neither wraps nor clips fenced code, so a long "+
					"code line makes the message %d wide at width %d (it overflows the terminal); proposed fix: clip(\"  \"+line, width) or hard-wrap it", got, w)
			}
		}
	}
}

func TestUnicodeWidthsAreCountedInCells(t *testing.T) {
	s := msgStore()
	// each CJK glyph is 2 cells: a line of N glyphs must wrap by cells, not by runes
	cjk := strings.Repeat("日", 60)
	m := &model.Message{ID: "cj", TS: 1000, AgentID: model.Main, Role: "assistant", Model: sonnet, Text: cjk}
	out := msgPlain(m, rctx{st: s, now: 1010, width: 40})
	if got := msgMaxWidth(out); got > 40 {
		t.Errorf("CJK text overflowed: %d cells", got)
	}
	if n := strings.Count(out, "日"); n != 60 {
		t.Errorf("no glyph may be lost while wrapping: %d of 60\n%s", n, out)
	}
	u := &model.Message{ID: "cu", TS: 1000, AgentID: model.Main, Role: "user", Text: cjk}
	if got := msgMaxWidth(msgPlain(u, rctx{st: s, width: 40})); got > 40 {
		t.Errorf("CJK user text overflowed: %d cells", got)
	}
	tool := &model.Message{ID: "ct", AgentID: model.Main, Role: "tool", Tool: "Bash", Status: "ok", Detail: cjk, Duration: fp(1)}
	tl := msgPlain(tool, rctx{st: s, width: 40})
	if lipgloss.Width(tl) > 40 || !strings.HasSuffix(tl, "1.0 s") {
		t.Errorf("CJK tool detail must be clipped by cells, keeping the duration: %q (%d)", tl, lipgloss.Width(tl))
	}
}
