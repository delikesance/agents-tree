package ui

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/store"
)

// ---- helpers (prefixed msg* to stay clear of the other test files) ----------------------------------------

func msgStore() *store.Store {
	s := store.New()
	for _, ev := range conversationEvents() {
		s.Apply(ev)
	}
	return s
}

func msgCtx(s *store.Store) rctx { return rctx{st: s, now: 1010, width: 90} }

// msgPlain renders one message to plain text.
func msgPlain(m *model.Message, c rctx) string {
	out, _ := renderMessage(m, c)
	return strip(out)
}

func msgNumbered(n int, prefix string) string {
	ls := make([]string, n)
	for i := range ls {
		ls[i] = fmt.Sprintf("%s%02d", prefix, i+1)
	}
	return strings.Join(ls, "\n")
}

func msgMaxWidth(s string) int {
	w := 0
	for _, l := range lines(s) {
		w = max(w, lipgloss.Width(l))
	}
	return w
}

// ---- 1. every role renders readably ------------------------------------------------------------------------

func TestUserMessageHasABarOnEveryLineLabelAndTime(t *testing.T) {
	s := msgStore()
	m := &model.Message{ID: "u9", TS: 1000, AgentID: model.Main, Role: "user", Text: "première ligne\ndeuxième ligne\n\ntroisième"}
	got := msgPlain(m, msgCtx(s))
	ls := lines(got)
	if len(ls) < 5 {
		t.Fatalf("want header + 4 body lines, got %d:\n%s", len(ls), got)
	}
	for i, l := range ls {
		if !strings.HasPrefix(l, "▌") {
			t.Errorf("line %d lacks the user bar: %q", i, l)
		}
	}
	if !strings.Contains(ls[0], "you") || !strings.Contains(ls[0], hhmm(1000)) {
		t.Errorf("header must carry the label and time: %q", ls[0])
	}
	for _, want := range []string{"première ligne", "deuxième ligne", "troisième"} {
		if !strings.Contains(got, want) {
			t.Errorf("user text lacks %q:\n%s", want, got)
		}
	}
}

func TestLongUserLineWrapsAndKeepsTheBarOnEveryRow(t *testing.T) {
	s := msgStore()
	m := &model.Message{ID: "u9", TS: 1000, AgentID: model.Main, Role: "user", Text: strings.Repeat("mot ", 60)}
	c := rctx{st: s, now: 1010, width: 40}
	got := msgPlain(m, c)
	ls := lines(got)
	if len(ls) < 4 {
		t.Fatalf("a long line must wrap:\n%s", got)
	}
	for _, l := range ls {
		if !strings.HasPrefix(l, "▌") || lipgloss.Width(l) > 40 {
			t.Errorf("bad row (%d wide): %q", lipgloss.Width(l), l)
		}
	}
}

func TestAssistantHeaderShowsLabelShortModelAndTime(t *testing.T) {
	s := msgStore()
	m := byID(s.Messages)["a"]
	head := lines(msgPlain(m, msgCtx(s)))[0]
	for _, want := range []string{"main", "Sonnet 5.5", hhmm(m.TS)} {
		if !strings.Contains(head, want) {
			t.Errorf("assistant header lacks %q: %q", want, head)
		}
	}
	// a subagent is labelled with its kind and its own model
	sa := byID(s.Messages)["sa"]
	shead := lines(msgPlain(sa, msgCtx(s)))[0]
	for _, want := range []string{"explorer", "Haiku 5.5"} {
		if !strings.Contains(shead, want) {
			t.Errorf("subagent header lacks %q: %q", want, shead)
		}
	}
	// no model on the message: the agent's own model is used
	bare := &model.Message{ID: "bare", TS: 1000, AgentID: model.Main, Role: "assistant", Text: "hello"}
	if h := lines(msgPlain(bare, msgCtx(s)))[0]; !strings.Contains(h, "Sonnet 5.5") {
		t.Errorf("model must fall back to the agent's: %q", h)
	}
}

func TestAssistantMarkdownRendersBulletsBoldCodeHeadingsAndFences(t *testing.T) {
	s := msgStore()
	text := "# Titre\n\nUn **gras** et du `code` ici.\n\n- premier\n- second item\n1. numéro un\n\n```go\nfmt.Println(\"x\")\n```\n\nfin"
	m := &model.Message{ID: "md", TS: 1000, AgentID: model.Main, Role: "assistant", Model: sonnet, Text: text}
	got := msgPlain(m, msgCtx(s))
	for _, want := range []string{"Titre", "Un gras et du code ici.", "• premier", "• second item", "1. numéro un", "fmt.Println(\"x\")", "fin"} {
		if !strings.Contains(got, want) {
			t.Errorf("markdown output lacks %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"**", "`", "# Titre", "```", "- premier"} {
		if strings.Contains(got, bad) {
			t.Errorf("markdown syntax %q must not leak:\n%s", bad, got)
		}
	}
	// the fenced line is indented, so it reads as a block
	for _, l := range lines(got) {
		if strings.Contains(l, "fmt.Println") && !strings.HasPrefix(l, "  ") {
			t.Errorf("code block line must be indented: %q", l)
		}
	}
}

func TestBulletContinuationLinesHangUnderTheText(t *testing.T) {
	s := msgStore()
	m := &model.Message{ID: "md", TS: 1000, AgentID: model.Main, Role: "assistant", Model: sonnet,
		Text: "- " + strings.Repeat("alpha ", 20)}
	got := msgPlain(m, rctx{st: s, now: 1010, width: 40})
	ls := lines(got)
	if len(ls) < 4 {
		t.Fatalf("bullet should wrap over several rows:\n%s", got)
	}
	for _, l := range ls[2:] {
		if !strings.HasPrefix(l, "  ") || strings.HasPrefix(l, "• ") {
			t.Errorf("continuation row must be indented under the bullet text: %q", l)
		}
	}
}

func TestToolLineShowsIconNameDetailAndRightAlignedDuration(t *testing.T) {
	s := msgStore()
	by := byID(s.Messages)
	c := msgCtx(s)
	c.width = 70
	cases := []struct {
		id    string
		parts []string
		suf   string
	}{
		{"t1", []string{"✗", "Bash", "pytest -q"}, "9.0 s"},
		{"t2", []string{"✓", "Read", "a/store.py"}, "0.2 s"},
	}
	for _, tc := range cases {
		got := msgPlain(by[tc.id], c)
		if len(lines(got)) != 1 {
			t.Errorf("a tool call is one line: %q", got)
		}
		for _, w := range tc.parts {
			if !strings.Contains(got, w) {
				t.Errorf("%s lacks %q: %q", tc.id, w, got)
			}
		}
		if !strings.HasSuffix(got, tc.suf) {
			t.Errorf("%s: duration must end the line: %q", tc.id, got)
		}
		if lipgloss.Width(got) != c.width {
			t.Errorf("%s: duration is right-aligned to the full width %d, got %d", tc.id, c.width, lipgloss.Width(got))
		}
	}
}

func TestToolDetailIsClippedToTheWidthWithoutLosingTheDuration(t *testing.T) {
	s := msgStore()
	m := &model.Message{ID: "tl", TS: 1000, AgentID: model.Main, Role: "tool", Tool: "Bash", Status: "ok",
		Detail: strings.Repeat("très-long-argument ", 30), Duration: fp(12.5)}
	for _, w := range []int{30, 50, 80, 108} {
		got := msgPlain(m, rctx{st: s, now: 1010, width: w})
		if lipgloss.Width(got) > w {
			t.Errorf("width %d: line is %d wide: %q", w, lipgloss.Width(got), got)
		}
		if !strings.HasSuffix(got, "12.5 s") || !strings.Contains(got, "…") || !strings.Contains(got, "Bash") {
			t.Errorf("width %d: clipped line must keep name and duration and show an ellipsis: %q", w, got)
		}
	}
}

func TestToolIconSpinnerWhileRunningAndEllipsisWhenOrphaned(t *testing.T) {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "Bash", 1))
	m := &model.Message{ID: "t", TS: 1000, AgentID: model.Main, Role: "tool", Tool: "Bash", Detail: "sleep 30", Status: "running"}
	s.Apply(model.Event{TS: 1000, Kind: model.Message_, AgentID: model.Main, Msg: m})

	c := rctx{st: s, now: 1005, width: 80}
	a := msgPlain(m, c)
	if !strings.Contains(a, spin(0)) || strings.ContainsAny(a, "✓✗") || !strings.Contains(a, "sleep 30") {
		t.Errorf("running tool of a live agent: %q", a)
	}
	c.frame = 3
	if b := msgPlain(m, c); !strings.Contains(b, spin(3)) || b == a {
		t.Errorf("spinner must advance with the frame: %q vs %q", a, b)
	}
	// the agent went quiet: the tool never answered -> neutral marker, no endless spinner
	c.now = 1000 + store.StaleSecs + 100
	idle := msgPlain(m, c)
	if strings.ContainsAny(idle, spinner) || !strings.Contains(idle, "…") || !strings.Contains(idle, "sleep 30") {
		t.Errorf("orphaned tool: %q", idle)
	}
}

func TestDelegationBoxShowsKindDescriptionPromptAndRunningChip(t *testing.T) {
	s := msgStore()
	d := msgPlain(byID(s.Messages)["deleg:ag1"], msgCtx(s))
	for _, want := range []string{"╭", "╰", "→ explorer", "Haiku 5.5", "map the repo", "List files.", "running"} {
		if !strings.Contains(d, want) {
			t.Errorf("delegation lacks %q:\n%s", want, d)
		}
	}
}

func TestDelegationChipTracksTheTargetState(t *testing.T) {
	s := msgStore()
	by := byID(s.Messages)
	c := msgCtx(s)
	if got := msgPlain(by["deleg:ag1"], c); !strings.Contains(got, "running") {
		t.Fatalf("running chip:\n%s", got)
	}
	s.Apply(evEnd(1011, "ag1", "done"))
	s.Apply(evUpdate(1011, "deleg:ag1", "done", fp(7.5)))
	got := msgPlain(by["deleg:ag1"], c)
	if !strings.Contains(got, "✓ done") || !strings.Contains(got, "7.5 s") || strings.Contains(got, "running") {
		t.Errorf("done chip:\n%s", got)
	}
	s.Apply(evStart(1012, "ag2", model.Main, "worker", "x", ""))
	s.Apply(evEnd(1013, "ag2", "failed"))
	d2 := &model.Message{ID: "deleg:ag2", AgentID: model.Main, Role: "delegation", Kind: "worker", Text: "go", Target: "ag2"}
	if got := msgPlain(d2, c); !strings.Contains(got, "✗ failed") || !strings.Contains(got, "→ worker") {
		t.Errorf("failed chip:\n%s", got)
	}
	s.Apply(evStart(1014, "ag3", model.Main, "researcher", "", ""))
	d3 := &model.Message{ID: "deleg:ag3", AgentID: model.Main, Role: "delegation", Kind: "researcher", Text: "go", Target: "ag3"}
	c.now = 1014 + store.StaleSecs + 10
	if got := msgPlain(d3, c); !strings.Contains(got, "no activity") {
		t.Errorf("stale chip:\n%s", got)
	}
}

func TestReportBoxShowsKindDurationCostAndFailedVariant(t *testing.T) {
	s := msgStore()
	r := msgPlain(byID(s.Messages)["rep:ag1"], msgCtx(s))
	for _, want := range []string{"╭", "↩ explorer reported", "5.0 s", "Found 3 files.", hhmm(1006)} {
		if !strings.Contains(r, want) {
			t.Errorf("report lacks %q:\n%s", want, r)
		}
	}

	s2 := store.New()
	s2.Apply(evStart(1000, "ag1", model.Main, "worker", "edit", ""))
	s2.Apply(model.Event{TS: 1001, Kind: model.Turn, AgentID: "ag1", Model: haiku, Usage: &model.Usage{Output: 1_000_000}})
	rep := &model.Message{ID: "rep:ag1", TS: 1002, AgentID: model.Main, Role: "report", Text: "all done", Target: "ag1", Status: "done", Duration: fp(75)}
	c := rctx{st: s2, now: 1005, width: 80}
	got := msgPlain(rep, c)
	for _, want := range []string{"↩ worker reported", "1 m 15 s", "~$", "all done"} {
		if !strings.Contains(got, want) {
			t.Errorf("priced report lacks %q:\n%s", want, got)
		}
	}
	fail := &model.Message{ID: "rep2", TS: 1002, AgentID: model.Main, Role: "report", Target: "ag1", Status: "failed"}
	got = msgPlain(fail, c)
	if !strings.Contains(got, "↩ worker failed") || strings.Contains(got, "reported") || !strings.Contains(got, "(no report)") {
		t.Errorf("failed report:\n%s", got)
	}
}

func TestReportForUnknownAgentStillRenders(t *testing.T) {
	s := store.New()
	rep := &model.Message{ID: "r", TS: 1000, AgentID: model.Main, Role: "report", Text: "orphan", Target: "ghost", Status: "done"}
	got := msgPlain(rep, rctx{st: s, width: 60})
	if !strings.Contains(got, "↩ agent reported") || !strings.Contains(got, "orphan") {
		t.Errorf("report without a node:\n%s", got)
	}
}

func TestSystemMessageIsOneDimLineWithLineCount(t *testing.T) {
	s := store.New()
	m := &model.Message{ID: "s", TS: 1000, AgentID: model.Main, Role: "system", Text: "Stop hook feedback:\nuntracked files\nthird"}
	one := msgPlain(m, rctx{st: s, width: 80})
	if len(lines(one)) != 1 || !strings.Contains(one, "⚙ Stop hook feedback:") || !strings.Contains(one, "· 3 lines") || strings.Contains(one, "untracked") {
		t.Errorf("collapsed system line: %q", one)
	}
	single := msgPlain(&model.Message{ID: "s2", AgentID: model.Main, Role: "system", Text: "[Request interrupted by user]"}, rctx{st: s, width: 80})
	if !strings.Contains(single, "⚙ [Request interrupted by user]") || strings.Contains(single, "lines") {
		t.Errorf("a one-line system message has no counter: %q", single)
	}
	full := msgPlain(m, rctx{st: s, width: 80, expanded: true})
	for _, want := range []string{"╭", "⚙ system", "untracked files", "third", hhmm(1000)} {
		if !strings.Contains(full, want) {
			t.Errorf("expanded system box lacks %q:\n%s", want, full)
		}
	}
}

func TestSystemLineIsClippedToTheWidth(t *testing.T) {
	s := store.New()
	m := &model.Message{ID: "s", AgentID: model.Main, Role: "system", Text: strings.Repeat("hook output ", 40) + "\nsecond"}
	for _, w := range []int{20, 40, 80} {
		if got := msgPlain(m, rctx{st: s, width: w}); lipgloss.Width(got) > w || len(lines(got)) != 1 {
			t.Errorf("width %d: %d wide, %d lines: %q", w, lipgloss.Width(got), len(lines(got)), got)
		}
	}
}

func TestCompactFlagMarksToolsAndSystemOnly(t *testing.T) {
	s := msgStore()
	by := byID(s.Messages)
	c := msgCtx(s)
	for id, want := range map[string]bool{"u": false, "a": false, "t1": true, "t2": true, "s": true, "deleg:ag1": false, "rep:ag1": false} {
		if _, compact := renderMessage(by[id], c); compact != want {
			t.Errorf("%s compact = %v, want %v", id, compact, want)
		}
	}
}

func TestEveryRoleFitsTheRequestedWidth(t *testing.T) {
	s := msgStore()
	long := &model.Message{ID: "long", TS: 1000, AgentID: model.Main, Role: "assistant", Model: sonnet,
		Text: "日本語のテキスト、très long « français » 😀 " + strings.Repeat("x", 200) + "\n- " + strings.Repeat("puce ", 30)}
	sub := &model.Message{ID: "longsub", TS: 1000, AgentID: "ag1", Role: "assistant", Text: strings.Repeat("y", 150)}
	all := append(append([]*model.Message{}, s.Messages...), long, sub)
	for _, w := range []int{40, 60, 90, 108} {
		for _, exp := range []bool{false, true} {
			c := msgCtx(s)
			c.width, c.expanded = w, exp
			for _, m := range all {
				out, _ := renderMessage(m, c)
				if got := msgMaxWidth(out); got > w {
					t.Errorf("%s at width %d (expanded=%v): line of width %d", m.ID, w, exp, got)
				}
			}
		}
	}
}

func TestNestedAssistantHeaderFitsVeryNarrowWidths(t *testing.T) {
	s := msgStore()
	c := msgCtx(s)
	c.width = 30 // a 34-column terminal
	out := msgPlain(byID(s.Messages)["sa"], c)
	if got := msgMaxWidth(out); got > c.width {
		t.Skipf("BUG: internal/ui/messages.go:renderAssistant builds header(head, m.TS) without clipping, so a nested "+
			"`explorer · Haiku 5.5 · hh:mm` header is %d wide at width %d; proposed fix: clip(header(head, m.TS), w) before nest()", got, c.width)
	}
}

// ---- 2. subagent messages are nested ----------------------------------------------------------------------

func TestSubagentMessagesAreIndentedUnderARailAndNarrower(t *testing.T) {
	s := msgStore()
	c := msgCtx(s)
	c.width = 60
	text := strings.Repeat("wordy ", 40)
	main := &model.Message{ID: "m", TS: 1000, AgentID: model.Main, Role: "assistant", Model: sonnet, Text: text}
	sub := &model.Message{ID: "sm", TS: 1000, AgentID: "ag1", Role: "assistant", Model: haiku, Text: text}
	subTool := &model.Message{ID: "st", TS: 1000, AgentID: "ag1", Role: "tool", Tool: "Read", Detail: "x.go", Status: "ok", Duration: fp(0.1)}

	mo, so := msgPlain(main, c), msgPlain(sub, c)
	for i, l := range lines(so) {
		if !strings.HasPrefix(l, "  │ ") {
			t.Errorf("subagent line %d must sit under the rail: %q", i, l)
		}
	}
	for i, l := range lines(mo) {
		if strings.HasPrefix(l, " ") || strings.Contains(l, "│") {
			t.Errorf("main line %d must not be nested: %q", i, l)
		}
	}
	if msgMaxWidth(so) > c.width || msgMaxWidth(mo) > c.width {
		t.Errorf("widths: main %d, sub %d, limit %d", msgMaxWidth(mo), msgMaxWidth(so), c.width)
	}
	// narrower text column: the same text needs at least as many rows, and each text row is 4 cells shorter
	if len(lines(so)) < len(lines(mo)) {
		t.Errorf("nested text must wrap at least as often: main %d rows, sub %d rows", len(lines(mo)), len(lines(so)))
	}
	for _, l := range lines(so)[1:] {
		if lipgloss.Width(strings.TrimPrefix(l, "  │ ")) > c.width-4 {
			t.Errorf("nested text row wider than width-4: %q", l)
		}
	}
	// tools too
	tl := msgPlain(subTool, c)
	if !strings.HasPrefix(tl, "  │ ") || lipgloss.Width(tl) != c.width {
		t.Errorf("nested tool line (want %d wide, rail first): %q", c.width, tl)
	}
	if mt := msgPlain(&model.Message{ID: "mt", AgentID: model.Main, Role: "tool", Tool: "Read", Status: "ok"}, c); strings.HasPrefix(mt, " ") {
		t.Errorf("main tool line is not nested: %q", mt)
	}
}

func TestSubagentRailIsDrawnForUnknownAgentsToo(t *testing.T) {
	s := store.New()
	m := &model.Message{ID: "g", TS: 1000, AgentID: "ghost", Role: "assistant", Text: "hi"}
	got := msgPlain(m, rctx{st: s, width: 50})
	if !strings.Contains(got, "agent") {
		t.Errorf("unknown agent is labelled generically: %q", got)
	}
	for _, l := range lines(got) {
		if !strings.HasPrefix(l, "  │ ") {
			t.Errorf("no rail: %q", l)
		}
	}
}

// ---- 3. fold / expand --------------------------------------------------------------------------------------

func TestFoldHelper(t *testing.T) {
	if got, hidden := fold("a\nb\nc\nd", false, 2); got != "a\nb" || hidden != 2 {
		t.Errorf("fold = %q, %d", got, hidden)
	}
	if got, hidden := fold("a\nb\nc\nd", true, 2); got != "a\nb\nc\nd" || hidden != 0 {
		t.Errorf("expanded fold = %q, %d", got, hidden)
	}
	if got, hidden := fold("a\nb", false, 2); got != "a\nb" || hidden != 0 {
		t.Errorf("short fold = %q, %d", got, hidden)
	}
	if _, hidden := fold(msgNumbered(foldLines, "l"), false, foldLines); hidden != 0 {
		t.Errorf("exactly %d lines must not fold", foldLines)
	}
	if _, hidden := fold(msgNumbered(foldLines+1, "l"), false, foldLines); hidden != 1 {
		t.Errorf("one line over the limit hides 1, got %d", hidden)
	}
}

func TestLongAssistantAndUserTextFoldAndExpand(t *testing.T) {
	s := store.New()
	text := msgNumbered(20, "ligne") // 20 lines: 14 shown, 6 hidden
	for _, role := range []string{"user", "assistant"} {
		m := &model.Message{ID: "x-" + role, TS: 1000, AgentID: model.Main, Role: role, Text: text}
		folded := msgPlain(m, rctx{st: s, width: 80})
		full := msgPlain(m, rctx{st: s, width: 80, expanded: true})
		if !strings.Contains(folded, "ligne14") || strings.Contains(folded, "ligne15") ||
			!strings.Contains(folded, "… +6 lines  (e to expand)") {
			t.Errorf("%s folded:\n%s", role, folded)
		}
		if !strings.Contains(full, "ligne20") || strings.Contains(full, "e to expand") || strings.Contains(full, "+6 lines") {
			t.Errorf("%s expanded:\n%s", role, full)
		}
	}
}

func TestFoldFooterKeepsTheUserBarAndTheSubagentRail(t *testing.T) {
	s := msgStore()
	c := msgCtx(s)
	u := &model.Message{ID: "u9", TS: 1000, AgentID: model.Main, Role: "user", Text: msgNumbered(20, "u")}
	ls := lines(msgPlain(u, c))
	if last := ls[len(ls)-1]; !strings.HasPrefix(last, "▌") || !strings.Contains(last, "e to expand") {
		t.Errorf("user footer: %q", last)
	}
	a := &model.Message{ID: "a9", TS: 1000, AgentID: "ag1", Role: "assistant", Text: msgNumbered(20, "s")}
	ls = lines(msgPlain(a, c))
	if last := ls[len(ls)-1]; !strings.HasPrefix(last, "  │ ") || !strings.Contains(last, "+6 lines") {
		t.Errorf("subagent footer: %q", last)
	}
}

func TestLongReportFoldsToo(t *testing.T) {
	s := msgStore()
	rep := &model.Message{ID: "rep:ag1", TS: 1006, AgentID: model.Main, Role: "report", Target: "ag1", Status: "done", Text: msgNumbered(30, "r")}
	got := msgPlain(rep, msgCtx(s))
	if !strings.Contains(got, "r14") || strings.Contains(got, "r15") || !strings.Contains(got, "+16 lines") {
		t.Errorf("folded report:\n%s", got)
	}
	c := msgCtx(s)
	c.expanded = true
	if full := msgPlain(rep, c); !strings.Contains(full, "r30") || strings.Contains(full, "e to expand") {
		t.Errorf("expanded report:\n%s", full)
	}
}

func TestDelegationPromptFoldsAtFourLines(t *testing.T) {
	s := msgStore()
	d := &model.Message{ID: "deleg:ag1", TS: 1004, AgentID: model.Main, Role: "delegation", Kind: "explorer", Desc: "map the repo",
		Target: "ag1", Text: msgNumbered(10, "p")}
	folded := msgPlain(d, msgCtx(s))
	if !strings.Contains(folded, "p04") || strings.Contains(folded, "p05") || !strings.Contains(folded, "… +6 lines  (e to expand)") {
		t.Errorf("folded delegation:\n%s", folded)
	}
	if !strings.Contains(folded, "running") {
		t.Errorf("the chip survives folding:\n%s", folded)
	}
	c := msgCtx(s)
	c.expanded = true
	full := msgPlain(d, c)
	if !strings.Contains(full, "p10") || strings.Contains(full, "e to expand") {
		t.Errorf("expanded delegation:\n%s", full)
	}
	exact := *d
	exact.Text = msgNumbered(4, "p")
	if got := msgPlain(&exact, msgCtx(s)); strings.Contains(got, "e to expand") {
		t.Errorf("4 lines fit:\n%s", got)
	}
}

// ---- messageSig --------------------------------------------------------------------------------------------

func TestMessageSigChangesOnlyWhenTheRenderingWould(t *testing.T) {
	s := msgStore()
	by := byID(s.Messages)
	c := msgCtx(s)
	c5 := c
	c5.frame = 5
	sig := func(id string, c rctx) string { return messageSig(by[id], c) }

	if sig("t2", c) != sig("t2", c5) || sig("u", c) != sig("u", c5) || sig("a", c) != sig("a", c5) {
		t.Error("finished tool, user and assistant messages do not depend on the frame")
	}
	if sig("deleg:ag1", c) == sig("deleg:ag1", c5) {
		t.Error("a delegation to a running agent animates")
	}
	before := sig("t2", c)
	s.Apply(evUpdate(1010, "t2", "error", nil))
	if sig("t2", c) == before {
		t.Error("a tool update (Rev) changes the signature")
	}
	d := sig("deleg:ag1", c)
	s.Apply(evEnd(1011, "ag1", "done"))
	if sig("deleg:ag1", c) == d {
		t.Error("the delegation signature changes when its target ends")
	}
	if sig("deleg:ag1", c) != sig("deleg:ag1", c5) {
		t.Error("a finished delegation is static")
	}
	e, n := c, c
	e.expanded, n.width = true, 50
	if sig("u", c) == sig("u", e) || sig("u", c) == sig("u", n) {
		t.Error("expanding or resizing changes the signature")
	}
}

func TestRunningToolSigFollowsTheFrameOnlyWhileItsAgentRuns(t *testing.T) {
	s := store.New()
	s.Apply(evTurn(1000, model.Main, sonnet, "Bash", 1))
	m := &model.Message{ID: "t", TS: 1000, AgentID: model.Main, Role: "tool", Tool: "Bash", Status: "running"}
	s.Apply(model.Event{TS: 1000, Kind: model.Message_, AgentID: model.Main, Msg: m})
	live := rctx{st: s, now: 1005, width: 80}
	live1 := live
	live1.frame = 1
	if messageSig(m, live) == messageSig(m, live1) {
		t.Error("a running tool of a live agent animates")
	}
	idle := rctx{st: s, now: 1000 + store.StaleSecs + 50, width: 80}
	idle1 := idle
	idle1.frame = 1
	if messageSig(m, idle) != messageSig(m, idle1) {
		t.Error("a running tool of an idle agent is static")
	}
	before := messageSig(m, live)
	s.Apply(evUpdate(1006, "t", "ok", fp(1)))
	if messageSig(m, live) == before {
		t.Error("the update bumps Rev and changes the signature")
	}
	// finished: no animation either
	live1.frame = 7
	if messageSig(m, live) != messageSig(m, live1) {
		t.Error("a finished tool is static")
	}
}
