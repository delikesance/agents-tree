package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/composition"
	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/replay"
	"github.com/delikesance/agents-tree/internal/sender"
	"github.com/delikesance/agents-tree/internal/sessions"
	"github.com/delikesance/agents-tree/internal/store"
	"github.com/delikesance/agents-tree/internal/tail"
	"github.com/delikesance/agents-tree/internal/transcript"
)

const (
	wideMin    = 150 // from this width the chat gets a side panel with the live agent diagram
	tickEvery  = 250 * time.Millisecond
	maxMounted = 400 // chat items drawn at once (older ones stay in the store)
	maxInput   = 6   // rows of the message box before it scrolls
)

// Options configures the app.
type Options struct {
	Session     string // transcript path to follow (live) or replay; empty = start a NEW session
	Replay      bool
	Speed       float64
	Hooks       bool
	HookFile    string
	ProjectsDir string
	Permissions string
	ClaudeBin   string
	CanSend     bool
	Pick        bool // open the session picker on start
}

type poller interface{ Poll() []model.Event }

type (
	tickMsg time.Time
	compMsg struct {
		path string
		comp *composition.Composition
	}
	sendDoneMsg struct{ err error }
)

type cached struct {
	sig     string
	out     string
	compact bool
}

// Model is the Bubble Tea model of the whole app.
type Model struct {
	laidStrip int // strip height the chat was last laid out with
	opt       Options
	st        *store.Store
	pollers   []poller
	rep       *replay.Replay
	session   string // transcript path; empty while a new session has not written anything yet

	w, h     int
	view     string // chat | tree
	filter   string // "" = everyone, else an agent node id
	showAll  bool   // details/tree: also list finished agents
	expanded bool   // unfold long messages, tool calls and system lines
	details  bool   // the details overlay (agents, cost, where tokens go)
	frame    int

	chat    viewport.Model
	treeVP  viewport.Model
	follow  bool
	cache   map[string]cached
	chatKey string
	hidden  int // system messages left out of the chat

	input      textarea.Model
	inputFocus bool
	picker     *picker

	snd          *sender.Sender
	newSessionID string
	sendErr      string

	comp     *composition.Composition
	compFor  string
	compAt   time.Time
	compBusy bool
}

// New builds the model. Without a session it starts a NEW one; replay mode loads the session up front.
func New(opt Options) (*Model, error) {
	if opt.Permissions == "" {
		opt.Permissions = "all"
	}
	if opt.ClaudeBin == "" {
		opt.ClaudeBin = "claude"
	}
	if opt.Speed == 0 {
		opt.Speed = 4
	}
	m := &Model{opt: opt, st: store.New(), view: "chat", follow: true, cache: map[string]cached{},
		showAll: opt.Replay, w: 120, h: 40}
	m.chat = viewport.New()
	m.treeVP = viewport.New()
	m.input = textarea.New()
	m.input.Placeholder = "Message Claude…"
	m.input.ShowLineNumbers = false
	m.input.Prompt = "❯ "
	m.input.SetHeight(1)
	m.input.SetVirtualCursor(true)
	m.input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j", "shift+enter"))
	m.input.SetStyles(textarea.DefaultStyles(true))
	if opt.Session != "" {
		if err := m.load(opt.Session); err != nil {
			return nil, err
		}
	}
	if m.canSend() {
		m.inputFocus = true // like a chat: type straight away; esc switches to commands
		m.input.Focus()
	}
	m.layout()
	return m, nil
}

func nowSecs() float64 { return float64(time.Now().UnixNano()) / 1e9 }

// now is wall-clock time when live, 0 (= newest event time) in a replay.
func (m *Model) now() float64 {
	if m.opt.Replay {
		return 0
	}
	return nowSecs()
}

func (m *Model) canSend() bool { return m.opt.CanSend && !m.opt.Replay }

func (m *Model) reset() {
	m.stopSender()
	m.st = store.New()
	m.filter = ""
	m.comp, m.compFor, m.compBusy = nil, "", false
	m.pollers, m.rep = nil, nil
	m.sendErr = ""
	m.resetChat()
}

// load (re)starts following an existing session.
func (m *Model) load(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	m.reset()
	m.session = path
	if m.opt.Replay {
		evs, err := transcript.ReadSession(path)
		if err != nil {
			return err
		}
		sp := m.opt.Speed
		m.rep = replay.New(evs, sp)
		return nil
	}
	m.pollers = []poller{tail.NewTranscriptTailer(path)}
	if m.opt.Hooks {
		hf := m.opt.HookFile
		if hf == "" {
			hf = os.Getenv("AGENTS_TREE_EVENTS")
		}
		if hf == "" {
			home, _ := os.UserHomeDir()
			hf = filepath.Join(home, ".claude", "agents-tree-events.jsonl")
		}
		m.pollers = append(m.pollers, tail.NewHookTailer(hf, strings.TrimSuffix(filepath.Base(path), ".jsonl")))
	}
	return nil
}

// newSession leaves the current session and starts a blank one (created on the first message).
func (m *Model) newSession() {
	m.reset()
	m.session, m.newSessionID = "", ""
}

func (m *Model) resetChat() {
	m.cache = map[string]cached{}
	m.chatKey = ""
	m.follow = true
	m.chat.SetContent("")
}

func (m *Model) stopSender() {
	if m.snd != nil {
		s := m.snd
		m.snd, m.newSessionID = nil, ""
		go s.Stop()
	}
}

// Init starts the tick loop (and the picker with --pick).
func (m *Model) Init() tea.Cmd {
	if m.opt.Pick && !m.opt.Replay {
		m.picker = newPicker(sessions.List(m.opt.ProjectsDir, 200), "", m.cwd())
	}
	cmds := []tea.Cmd{tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })}
	if m.inputFocus {
		cmds = append(cmds, m.input.Focus())
	}
	return tea.Batch(cmds...)
}

func (m *Model) cwd() string {
	if m.session != "" {
		if _, cwd := sessions.Scan(m.session); cwd != "" {
			return cwd
		}
	}
	d, _ := os.Getwd()
	return d
}

// ---- update -----------------------------------------------------------------------------------------------

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		m.refresh()
		return m, nil
	case tickMsg:
		return m, m.onTick()
	case compMsg:
		m.compBusy = false
		if msg.path == m.session && msg.comp != nil {
			m.comp, m.compFor, m.compAt = msg.comp, msg.path, time.Now()
		}
		return m, nil
	case sendDoneMsg:
		if msg.err != nil {
			m.sendErr = msg.err.Error()
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.onKey(msg)
	case tea.MouseWheelMsg:
		var cmd tea.Cmd
		if m.view == "chat" {
			m.chat, cmd = m.chat.Update(msg)
			m.follow = m.chat.AtBottom()
		} else {
			m.treeVP, cmd = m.treeVP.Update(msg)
		}
		return m, cmd
	}
	if m.picker != nil {
		cmd, _ := m.picker.update(msg)
		return m, cmd
	}
	if m.inputFocus {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *Model) onTick() tea.Cmd {
	next := tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
	var events []model.Event
	if m.rep != nil {
		events = m.rep.Tick(tickEvery.Seconds())
	} else {
		m.adoptNewSession()
		for _, p := range m.pollers {
			events = append(events, p.Poll()...)
		}
	}
	for _, ev := range events {
		m.st.Apply(ev)
	}
	m.frame++
	m.refresh()
	if cmd := m.compositionCmd(); cmd != nil {
		return tea.Batch(next, cmd)
	}
	return next
}

// compositionCmd recomputes "where tokens go" in the background (only while the details are open):
// every 15 s live, once for a replay.
func (m *Model) compositionCmd() tea.Cmd {
	if m.session == "" || m.compBusy || !m.details {
		return nil
	}
	due := m.compFor != m.session || (!m.opt.Replay && time.Since(m.compAt) > 15*time.Second)
	if !due {
		return nil
	}
	m.compBusy = true
	path := m.session
	return func() tea.Msg { return compMsg{path, composition.Analyze(path, true)} }
}

// adoptNewSession follows the transcript of a session we started ourselves, once Claude wrote it.
func (m *Model) adoptNewSession() {
	if m.newSessionID == "" || m.session != "" {
		return
	}
	if p := sessions.FindTranscript(m.newSessionID, m.opt.ProjectsDir); p != "" {
		snd := m.snd
		m.snd = nil // load() stops the sender: keep talking to the same process instead
		err := m.load(p)
		m.snd = snd
		if err == nil {
			m.newSessionID = ""
		}
	}
}

func (m *Model) quit() (tea.Model, tea.Cmd) {
	if m.snd != nil {
		m.snd.Stop()
	}
	return m, tea.Quit
}

func (m *Model) busy() bool { return m.snd != nil && m.snd.Busy() }

func (m *Model) scrollChat(k tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	if m.view == "chat" {
		m.chat, cmd = m.chat.Update(k)
		m.follow = m.chat.AtBottom()
	} else {
		m.treeVP, cmd = m.treeVP.Update(k)
	}
	return cmd
}

func (m *Model) onKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if m.picker != nil {
		if s == "ctrl+c" {
			return m.quit()
		}
		cmd, res := m.picker.update(k)
		if res.closed {
			m.picker = nil
			switch {
			case res.newSession:
				m.newSession()
			case res.path != "":
				if err := m.load(res.path); err != nil {
					m.sendErr = err.Error()
				}
			}
			m.refresh()
		}
		return m, cmd
	}
	switch s { // keys that work everywhere
	case "ctrl+c":
		if m.busy() {
			m.interrupt()
			return m, nil
		}
		return m.quit()
	case "ctrl+k":
		m.interrupt()
		return m, nil
	case "pgup", "pgdown":
		return m, m.scrollChat(k)
	}
	if m.details {
		switch s {
		case "d", "esc", "q":
			m.details = false
		case "h":
			m.showAll = !m.showAll
		}
		return m, nil
	}
	if m.inputFocus {
		switch s {
		case "esc":
			m.inputFocus = false
			m.input.Blur()
			return m, nil
		case "enter":
			text := strings.TrimSpace(m.input.Value())
			m.input.Reset()
			m.fitInput()
			if text == "" {
				return m, nil
			}
			return m, m.send(text)
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(k)
		m.fitInput()
		return m, cmd
	}
	switch s { // command mode
	case "q":
		return m.quit()
	case "s":
		m.picker = newPicker(sessions.List(m.opt.ProjectsDir, 200), m.session, m.cwd())
	case "n":
		if !m.opt.Replay {
			m.newSession()
		}
	case "i", "enter":
		if m.canSend() && m.view == "chat" {
			m.inputFocus = true
			return m, m.input.Focus()
		}
	case "d":
		m.details = true
	case "f":
		m.cycleFilter()
	case "e":
		m.expanded = !m.expanded
		m.resetChat()
	case "t":
		if m.view == "chat" {
			m.view = "tree"
		} else {
			m.view = "chat"
		}
		m.layout()
	case "h":
		m.showAll = !m.showAll
	case " ", "space":
		if m.rep != nil {
			m.rep.Paused = !m.rep.Paused
		}
	case "+", "=":
		if m.rep != nil {
			m.rep.Speed = min(m.rep.Speed*2, 64)
		}
	case "-":
		if m.rep != nil {
			m.rep.Speed = max(m.rep.Speed/2, 0.25)
		}
	case "end":
		if m.view == "chat" {
			m.follow = true
			m.chat.GotoBottom()
		} else {
			m.treeVP.GotoBottom()
		}
	case "home":
		if m.view == "chat" {
			m.follow = false
			m.chat.GotoTop()
		} else {
			m.treeVP.GotoTop()
		}
	case "up", "down":
		return m, m.scrollChat(k)
	}
	m.refresh()
	return m, nil
}

// fitInput grows the message box with its text (1 to maxInput rows).
func (m *Model) fitInput() {
	rows := min(max(m.input.LineCount(), 1), maxInput)
	if rows != m.input.Height() {
		m.input.SetHeight(rows)
		m.layout()
	}
}

func (m *Model) cycleFilter() {
	order := []string{"", model.Main}
	for _, id := range m.st.Order {
		if id != model.Main {
			order = append(order, id)
		}
	}
	i := 0
	for j, id := range order {
		if id == m.filter {
			i = j
		}
	}
	m.filter = order[(i+1)%len(order)]
	m.resetChat()
}

func (m *Model) filterLabel() string {
	if m.filter == model.Main {
		return "main"
	}
	if n := m.st.Get(m.filter); n != nil {
		return n.Kind
	}
	return m.filter
}

func (m *Model) ensureSender() *sender.Sender {
	if m.snd != nil {
		return m.snd
	}
	var s *sender.Sender
	if m.session != "" {
		_, cwd := sessions.Scan(m.session)
		s, _ = sender.New(strings.TrimSuffix(filepath.Base(m.session), ".jsonl"), cwd, true, m.opt.Permissions, m.opt.ClaudeBin)
	} else {
		s, _ = sender.New("", "", false, m.opt.Permissions, m.opt.ClaudeBin)
		m.newSessionID = s.SessionID
	}
	m.snd = s
	return s
}

func (m *Model) send(text string) tea.Cmd {
	if !m.canSend() {
		return nil
	}
	m.sendErr = ""
	m.follow = true
	s := m.ensureSender()
	return func() tea.Msg { return sendDoneMsg{err: s.Send(text)} }
}

func (m *Model) interrupt() {
	if m.snd != nil {
		go m.snd.Interrupt()
	}
}

// ---- layout & rendering -----------------------------------------------------------------------------------

func (m *Model) inputBoxH() int {
	if m.view == "chat" && m.canSend() {
		return m.input.Height() + 2
	}
	return 0
}

// runningAgents are the subagents working right now (shown as a one-line strip above the message box).
func (m *Model) runningAgents() []*model.AgentNode {
	var out []*model.AgentNode
	now := m.now()
	for _, id := range m.st.Order {
		if id != model.Main && m.st.State(m.st.Nodes[id], now) == "running" {
			out = append(out, m.st.Nodes[id])
		}
	}
	return out
}

// wide: big terminals show the live agent diagram next to the chat instead of a strip above the message box.
func (m *Model) wide() bool { return m.w >= wideMin && m.view == "chat" }

func (m *Model) sideW() int { return min(max(m.w*36/100, 56), 84) }

// chatPaneW is the width of the conversation (the whole screen unless the side panel is shown).
func (m *Model) chatPaneW() int {
	if m.wide() {
		return m.w - m.sideW() - 3
	}
	return m.w
}

func (m *Model) stripH() int {
	if m.view == "chat" && !m.wide() && len(m.runningAgents()) > 0 {
		return 1
	}
	return 0
}

func (m *Model) bodyH() int { return max(m.h-2-m.inputBoxH()-m.stripH(), 3) } // header + status

func (m *Model) textW() int { return min(m.chatPaneW()-2*margin, chatMax) }

func (m *Model) layout() {
	m.laidStrip = m.stripH()
	m.chat.SetWidth(m.chatPaneW())
	m.chat.SetHeight(m.bodyH())
	m.treeVP.SetWidth(m.w)
	m.treeVP.SetHeight(max(m.bodyH()-11, 3))
	m.input.SetWidth(max(m.chatPaneW()-6, 10))
	f := m.follow
	m.cache = map[string]cached{}
	m.chatKey = ""
	m.follow = f
}

func (m *Model) passes(msg *model.Message) bool {
	if m.filter == "" {
		return true
	}
	return m.st.Resolve(msg.AgentID) == m.filter || (msg.Target != "" && m.st.Resolve(msg.Target) == m.filter)
}

// refresh rebuilds what changed: chat content (cached per item) and the tree.
func (m *Model) refresh() {
	if m.stripH() != m.laidStrip { // agents started or stopped: the strip appears or goes
		m.layout()
	}
	if m.view == "chat" {
		m.refreshChat()
		return
	}
	tv := treeView(m.st, m.w-4, m.frame, m.now(), m.showAll)
	wmax := 0
	for _, l := range strings.Split(tv, "\n") {
		wmax = max(wmax, lipgloss.Width(l))
	}
	m.treeVP.SetContent(indent(tv, max(0, (m.w-wmax)/2)))
}

func (m *Model) welcome() string {
	var l []string
	where := sessions.Label("", m.cwd())
	if m.session == "" && !m.opt.Replay {
		l = append(l, bold(colText).Render("New session")+fg(colDim).Render(" in "+where), "",
			fg(colDim).Render("Type a message below to start."),
			fg(colFaint).Render("esc  commands · s  open an existing session · d  details"))
	} else {
		l = append(l, fg(colDim).Render("No messages in this session yet."))
	}
	return "\n" + indent(strings.Join(l, "\n"), margin+2)
}

func (m *Model) refreshChat() {
	c := rctx{st: m.st, now: m.now(), frame: m.frame, expanded: m.expanded, width: m.textW()}
	var vis []*model.Message
	for _, msg := range m.st.Messages {
		if m.passes(msg) {
			vis = append(vis, msg)
		}
	}
	items, hidden := buildFlow(vis, m.expanded)
	m.hidden = hidden
	if len(items) > maxMounted {
		items = items[len(items)-maxMounted:]
	}
	var sb strings.Builder
	prevCompact := true
	live := map[string]bool{}
	for i, it := range items {
		live[it.id] = true
		sig := it.sig(c)
		ce, ok := m.cache[it.id]
		if !ok || ce.sig != sig {
			out, compact := it.render(c)
			ce = cached{sig, out, compact}
			m.cache[it.id] = ce
		}
		if i > 0 {
			sb.WriteString("\n")
			if !(ce.compact && prevCompact) { // air between messages; tool lines of one run stay together
				sb.WriteString("\n")
			}
		}
		sb.WriteString(indent(ce.out, margin))
		prevCompact = ce.compact
	}
	content := sb.String()
	if content == "" {
		content = m.welcome()
	} else {
		content = "\n" + content // a blank line under the header
	}
	key := fmt.Sprintf("%d|%s", len(content), content[max(0, len(content)-200):])
	if key != m.chatKey {
		m.chatKey = key
		m.chat.SetContent(content)
		if m.follow {
			m.chat.GotoBottom()
		}
	}
	if len(m.cache) > len(items)+64 { // drop entries of items that left the window
		for id := range m.cache {
			if !live[id] {
				delete(m.cache, id)
			}
		}
	}
}

func short(s string) string { return s[:min(8, len(s))] }

func (m *Model) sessionID() string { return strings.TrimSuffix(filepath.Base(m.session), ".jsonl") }

// header is one quiet line: where you are, and whether it is live.
func (m *Model) header() string {
	l := " " + bold(colText).Render("agents-tree") + fg(colFaint).Render("  ·  ") + fg(colDim).Render(sessions.Label("", m.cwd()))
	switch {
	case m.session != "":
		l += fg(colFaint).Render("  ·  session " + short(m.sessionID()))
	case m.opt.Replay:
	default:
		l += fg(colFaint).Render("  ·  new session")
	}
	if m.filter != "" {
		l += "  " + fg(colOrange).Render("filter: "+m.filterLabel())
	}
	right := fg(colGreen).Render("● live") + " "
	if m.opt.Replay {
		right = fg(colOrange).Render("replay") + " "
	}
	if gap := m.w - lipgloss.Width(l) - lipgloss.Width(right); gap > 0 {
		l += strings.Repeat(" ", gap) + right
	}
	return clip(l, m.w)
}

// strip shows the agents working right now, one line, only when there are some.
func (m *Model) strip() string {
	var parts []string
	for _, n := range m.runningAgents() {
		act := n.Kind
		if n.Activity != "" {
			act += " · " + n.Activity
		}
		parts = append(parts, fg(modelColor(n.Model, false)).Render(spin(m.frame))+" "+fg(colText).Render(act))
	}
	return clip(" "+strings.Join(parts, fg(colFaint).Render("   ")), m.w)
}

func (m *Model) inputBox() string {
	col := colLine
	if m.inputFocus {
		col = colUser
	}
	w := m.chatPaneW()
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(col).Width(w - 2).MaxWidth(w)
	return st.Render(m.input.View())
}

// statusLine: what matters (state, cost) on the left, the keys that apply now on the right.
func (m *Model) statusLine() string {
	var left []string
	switch {
	case m.busy():
		left = append(left, bold(colGreen).Render(spin(m.frame)+" Claude is working…"))
	case m.snd != nil && m.snd.LastError() != "":
		left = append(left, bold(colRed).Render("✗ "+m.snd.LastError()))
	case m.sendErr != "":
		left = append(left, bold(colRed).Render("✗ "+m.sendErr))
	case m.rep != nil:
		s := fmt.Sprintf("replay %d/%d x%g", m.rep.Pos, len(m.rep.Events), m.rep.Speed)
		if m.rep.Paused {
			s += " [paused]"
		}
		left = append(left, fg(colOrange).Render(s))
	}
	sm := m.st.Summary()
	if sm.Turns > 0 {
		s := usd(sm.Cost.Total())
		if hr, ok := sm.HitRate(); ok {
			s += fmt.Sprintf(" · cache %.0f%%", hr*100)
		}
		left = append(left, fg(colDim).Render(s))
	}
	if m.canSend() {
		switch m.opt.Permissions {
		case "all":
			left = append(left, fg(colOrange).Render("⚠ all permissions"))
		case "plan":
			left = append(left, fg(colDim).Render("plan mode"))
		default:
			left = append(left, fg(colDim).Render("accept-edits"))
		}
	}
	if m.hidden > 0 && !m.expanded {
		left = append(left, fg(colFaint).Render(fmt.Sprintf("%d hidden", m.hidden)))
	}
	hint := func(k, d string) string { return bold(colOrange).Render(k) + " " + fg(colDim).Render(d) }
	var hs []string
	switch {
	case m.details:
		hs = []string{hint("d", "close"), hint("h", "finished agents")}
	case m.inputFocus:
		hs = []string{hint("enter", "send"), hint("alt+enter", "new line"), hint("esc", "commands")}
	default:
		if m.canSend() {
			hs = append(hs, hint("i", "write"))
		}
		hs = append(hs, hint("d", "details"), hint("s", "sessions"), hint("e", "expand"), hint("f", "filter"), hint("t", "tree"), hint("q", "quit"))
		if m.rep != nil {
			hs = append(hs, hint("space", "pause"))
		}
	}
	l := " " + strings.Join(left, fg(colFaint).Render("  ·  "))
	r := strings.Join(hs, fg(colFaint).Render("  ")) + " "
	if m.w-lipgloss.Width(l)-lipgloss.Width(r) < 2 {
		r = clip(r, max(m.w-lipgloss.Width(l)-2, 0))
	}
	gap := max(m.w-lipgloss.Width(l)-lipgloss.Width(r), 0)
	return clip(l+strings.Repeat(" ", gap)+r, m.w)
}

func fitHeight(s string, h int) string {
	lines := strings.Split(s, "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

// detailsView is the overlay with everything that is not the conversation: agents, cost, where tokens go.
func (m *Model) detailsView() string {
	w := min(m.w-4, 124)
	left := min(46, w/2)
	right := w - left - 6
	now := m.now()
	l := railAgents(m.st, now, m.frame, m.showAll, left-4)
	var r []string
	if m.st.Summary().Turns > 0 {
		r = append(r, costPanel(m.st, right))
		if m.session != "" {
			r = append(r, contextPanel(m.comp, right))
		}
	} else {
		r = append(r, fg(colFaint).Render("No usage yet."))
	}
	if m.st.Advisor.Calls > 0 {
		r = append(r, advisorPanel(m.st, right))
	}
	if m.st.Jev.Forks > 0 {
		r = append(r, jevPanel(m.st, right))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, padBlock(l, left), "  ", strings.Join(r, "\n"))
	return box(bold(colText).Render("details")+fg(colFaint).Render("  ·  d to close"), fitHeight(body, min(strings.Count(body, "\n")+1, m.h-4)), w, colLine)
}

// Render returns the whole screen as a string.
func (m *Model) Render() string {
	bh := m.bodyH()
	var body string
	if m.view == "chat" {
		body = m.chat.View()
	} else {
		var logs []string
		start := max(0, len(m.st.Log)-9)
		for _, l := range m.st.Log[start:] {
			logs = append(logs, fg(colFaint).Render(hhmm(l.TS)+"  ")+fg(colGrey).Render(l.Text))
		}
		body = m.treeVP.View() + "\n" + box(fg(colDim).Render("log"), strings.Join(logs, "\n"), m.w-2, colLine)
	}
	main := []string{fitHeight(body, bh)}
	if m.stripH() > 0 {
		main = append(main, m.strip())
	}
	if m.view == "chat" && m.canSend() {
		main = append(main, m.inputBox())
	}
	block := strings.Join(main, "\n")
	if m.wide() { // conversation | live agents, the same height down to the status line
		left := padBlock(fitHeight(block, m.h-2), m.chatPaneW())
		sep := strings.TrimRight(strings.Repeat(fg(colLine).Render("│")+"\n", m.h-2), "\n")
		right := indent(m.sidePanel(m.sideW()-2, m.h-2), 1)
		block = lipgloss.JoinHorizontal(lipgloss.Top, left, " ", sep, " ", right)
	}
	lines := []string{m.header(), block}
	lines = append(lines, m.statusLine())
	screen := strings.Join(lines, "\n")
	switch {
	case m.picker != nil:
		screen = overlay(screen, m.picker.view(m.w, m.h), m.w, m.h)
	case m.details:
		screen = overlay(screen, m.detailsView(), m.w, m.h)
	}
	return screen
}

func padBlock(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = padRight(l, w)
	}
	return strings.Join(lines, "\n")
}

// overlay draws popup centred over the screen (replacing the lines it covers).
func overlay(screen, popup string, w, h int) string {
	base := strings.Split(screen, "\n")
	pop := strings.Split(popup, "\n")
	top := max(0, (h-len(pop))/2)
	left := max(0, (w-lipgloss.Width(popup))/2)
	for i, pl := range pop {
		y := top + i
		if y >= len(base) {
			break
		}
		base[y] = strings.Repeat(" ", left) + pl
	}
	return strings.Join(base, "\n")
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	v := tea.NewView(m.Render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.WindowTitle = "agents-tree"
	return v
}

// Run starts the program.
func Run(opt Options) error {
	m, err := New(opt)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}

// Snapshot renders one frame of a session as an ANSI string (used by `agents-tree render` and tests):
// all of its events are applied first, then the given keys are pressed (e.g. "f", "t").
func Snapshot(opt Options, width, height int, keys ...string) (string, error) {
	opt.Replay = opt.Replay || opt.Session != ""
	m, err := New(opt)
	if err != nil {
		return "", err
	}
	if m.rep != nil {
		for _, ev := range m.rep.Events {
			m.st.Apply(ev)
		}
		m.rep.Pos = len(m.rep.Events)
	}
	m.opt.Replay = opt.Replay
	m.w, m.h = width, height
	m.layout()
	m.refresh()
	for _, k := range keys {
		m.onKey(KeyFromString(k))
	}
	m.refresh()
	return m.Render(), nil
}

// KeyFromString builds a key press from "a", "enter", "esc", "ctrl+k", "alt+enter", "up"...
func KeyFromString(s string) tea.KeyPressMsg {
	k := tea.KeyPressMsg{}
	for {
		switch {
		case strings.HasPrefix(s, "ctrl+") && len(s) > 5:
			k.Mod |= tea.ModCtrl
			s = s[5:]
			continue
		case strings.HasPrefix(s, "alt+") && len(s) > 4:
			k.Mod |= tea.ModAlt
			s = s[4:]
			continue
		case strings.HasPrefix(s, "shift+") && len(s) > 6:
			k.Mod |= tea.ModShift
			s = s[6:]
			continue
		}
		break
	}
	special := map[string]rune{"enter": tea.KeyEnter, "esc": tea.KeyEscape, "up": tea.KeyUp, "down": tea.KeyDown,
		"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd, "backspace": tea.KeyBackspace,
		"tab": tea.KeyTab, "space": ' '}
	if r, ok := special[s]; ok {
		k.Code = r
		if s == "space" {
			k.Text = " "
		}
		return k
	}
	r := []rune(s)
	k.Code = r[0]
	if k.Mod == 0 || k.Mod == tea.ModShift {
		k.Text = s
	}
	return k
}
