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
	tickEvery  = 250 * time.Millisecond
	railWidth  = 40
	maxMounted = 400 // chat messages drawn at once (older ones stay in the store)
	inputRows  = 3
)

// Options configures the app.
type Options struct {
	Session     string // transcript path to follow (live) or replay
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
	opt     Options
	st      *store.Store
	pollers []poller
	rep     *replay.Replay
	session string

	w, h     int
	view     string // chat | tree
	filter   string // "" = everyone, else an agent node id
	showAll  bool
	expanded bool
	frame    int

	chat    viewport.Model
	treeVP  viewport.Model
	follow  bool
	cache   map[string]cached
	chatKey string

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

// New builds the model. Replay mode loads the whole session up front.
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
	m.input.Placeholder = "Message Claude…   enter: send · alt+enter: new line · esc: back to chat"
	m.input.ShowLineNumbers = false
	m.input.Prompt = "│ "
	m.input.SetHeight(inputRows)
	m.input.SetVirtualCursor(true)
	m.input.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j", "shift+enter"))
	m.input.SetStyles(textarea.DefaultStyles(true))
	if opt.Session != "" {
		if err := m.load(opt.Session); err != nil {
			return nil, err
		}
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

// load (re)starts following a session.
func (m *Model) load(path string) error {
	if _, err := os.Stat(path); err != nil {
		return err
	}
	m.stopSender()
	m.session = path
	m.st = store.New()
	m.filter = ""
	m.comp, m.compFor, m.compBusy = nil, "", false
	m.resetChat()
	if m.opt.Replay {
		evs, err := transcript.ReadSession(path)
		if err != nil {
			return err
		}
		sp := m.opt.Speed
		if m.rep != nil {
			sp = m.rep.Speed
		}
		m.rep, m.pollers = replay.New(evs, sp), nil
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

// Init starts the tick loop (and the picker when no session was given).
func (m *Model) Init() tea.Cmd {
	cmds := []tea.Cmd{tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })}
	if m.session == "" && !m.opt.Replay {
		m.picker = newPicker(sessions.List(m.opt.ProjectsDir, 200), "")
	}
	return tea.Batch(cmds...)
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

// compositionCmd recomputes "where tokens go" in the background: every 15 s live, once for a replay.
func (m *Model) compositionCmd() tea.Cmd {
	if m.session == "" || m.compBusy {
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

func (m *Model) onKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		return m.quit()
	}
	if m.picker != nil {
		cmd, res := m.picker.update(k)
		if res.closed {
			m.picker = nil
			if res.path != "" {
				if err := m.load(res.path); err != nil {
					m.sendErr = err.Error()
				}
				m.refresh()
			}
		}
		return m, cmd
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
			if text == "" {
				return m, nil
			}
			return m, m.send(text)
		case "ctrl+k":
			m.interrupt()
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(k)
		return m, cmd
	}
	switch s {
	case "q":
		return m.quit()
	case "s":
		m.picker = newPicker(sessions.List(m.opt.ProjectsDir, 200), m.session)
	case "i", "enter":
		if m.canSend() && m.view == "chat" {
			m.inputFocus = true
			return m, m.input.Focus()
		}
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
	case "ctrl+k":
		m.interrupt()
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
	case "up", "down", "pgup", "pgdown":
		var cmd tea.Cmd
		if m.view == "chat" {
			m.chat, cmd = m.chat.Update(k)
			m.follow = m.chat.AtBottom()
		} else {
			m.treeVP, cmd = m.treeVP.Update(k)
		}
		return m, cmd
	}
	m.refresh()
	return m, nil
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
	s := m.ensureSender()
	return func() tea.Msg { return sendDoneMsg{err: s.Send(text)} }
}

func (m *Model) interrupt() {
	if m.snd != nil {
		go m.snd.Interrupt()
	}
}

// ---- layout & rendering -----------------------------------------------------------------------------------

func (m *Model) railW() int {
	if m.w >= 110 {
		return railWidth
	}
	return 0
}

func (m *Model) centerW() int {
	if r := m.railW(); r > 0 {
		return m.w - r - 1
	}
	return m.w
}

func (m *Model) bottomH() int {
	if m.view == "chat" && m.canSend() {
		return 1 + inputRows + 2
	}
	return 0
}

func (m *Model) bodyH() int {
	h := m.h - 3 - m.bottomH() // header, status, hints
	if m.railW() == 0 {
		h-- // pills bar replaces the rail
	}
	return max(h, 3)
}

func (m *Model) layout() {
	cw := m.centerW()
	m.chat.SetWidth(cw)
	m.chat.SetHeight(m.bodyH())
	m.treeVP.SetWidth(cw)
	m.treeVP.SetHeight(max(m.bodyH()-11, 3))
	m.input.SetWidth(max(cw-4, 10))
	m.resetChatKeepFollow()
}

func (m *Model) resetChatKeepFollow() {
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

// refresh rebuilds what changed: chat content (cached per message) and the tree.
func (m *Model) refresh() {
	if m.view == "chat" {
		m.refreshChat()
	} else {
		now := m.now()
		tv := treeView(m.st, m.centerW()-4, m.frame, now, m.showAll)
		lines := strings.Split(tv, "\n")
		wmax := 0
		for _, l := range lines {
			wmax = max(wmax, lipgloss.Width(l))
		}
		pad := max(0, (m.centerW()-wmax)/2)
		m.treeVP.SetContent(indent(tv, pad))
	}
}

func (m *Model) refreshChat() {
	now := m.now()
	c := rctx{st: m.st, now: now, frame: m.frame, expanded: m.expanded, width: max(m.centerW()-2, 20)}
	var vis []*model.Message
	for _, msg := range m.st.Messages {
		if m.passes(msg) {
			vis = append(vis, msg)
		}
	}
	if len(vis) > maxMounted {
		vis = vis[len(vis)-maxMounted:]
	}
	var sb strings.Builder
	prevCompact := true
	for i, msg := range vis {
		sig := messageSig(msg, c)
		ce, ok := m.cache[msg.ID]
		if !ok || ce.sig != sig {
			out, compact := renderMessage(msg, c)
			ce = cached{sig, out, compact}
			m.cache[msg.ID] = ce
		}
		if i > 0 && !(ce.compact && prevCompact) {
			sb.WriteString("\n")
		}
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(ce.out)
		prevCompact = ce.compact
	}
	content := sb.String()
	if content == "" {
		content = fg(colFaint).Render("\n  No messages yet. Press i to write to Claude, s to pick another session.")
	}
	key := fmt.Sprintf("%d|%s", len(content), content[max(0, len(content)-200):])
	if key != m.chatKey {
		m.chatKey = key
		m.chat.SetContent(content)
		if m.follow {
			m.chat.GotoBottom()
		}
	}
	// drop cache entries of messages that left the window
	if len(m.cache) > len(vis)+64 {
		keep := map[string]bool{}
		for _, v := range vis {
			keep[v.ID] = true
		}
		for id := range m.cache {
			if !keep[id] {
				delete(m.cache, id)
			}
		}
	}
}

func (m *Model) header() string {
	l := bold(colText).Render("AGENTS-TREE") + "  " +
		fg(colPurple).Render("■") + fg(colDim).Render(" opus advisor  ") +
		fg(colOrange).Render("■") + fg(colDim).Render(" sonnet main/worker  ") +
		fg(colGreen).Render("■") + fg(colDim).Render(" haiku swarm")
	if m.session != "" {
		_, cwd := sessions.Scan(m.session)
		right := fg(colDim).Render(sessions.Label("", cwd)) + fg(colFaint).Render(" · session "+short(m.sessionID()))
		if m.opt.Replay {
			right += "  " + fg(colOrange).Render("replay")
		} else {
			right += "  " + fg(colGreen).Render("● live")
		}
		gap := m.w - lipgloss.Width(l) - lipgloss.Width(right) - 2
		if gap > 0 {
			l += strings.Repeat(" ", gap) + right
		}
	}
	return " " + clip(l, m.w-1)
}

func (m *Model) sessionID() string { return strings.TrimSuffix(filepath.Base(m.session), ".jsonl") }

func short(s string) string { return s[:min(8, len(s))] }

func (m *Model) rail(h int) string {
	now := m.now()
	w := m.railW() - 1
	parts := []string{railAgents(m.st, now, m.frame, m.showAll, w)}
	sm := m.st.Summary()
	if sm.Turns > 0 {
		parts = append(parts, costPanel(m.st, w))
		if m.session != "" {
			parts = append(parts, contextPanel(m.comp, w))
		}
	}
	if m.st.Advisor.Calls > 0 {
		parts = append(parts, advisorPanel(m.st, w))
	}
	if m.st.Jev.Forks > 0 {
		parts = append(parts, jevPanel(m.st, w))
	}
	return fitHeight(strings.Join(parts, "\n"), h)
}

// pills is the one-line agent summary used when the terminal is too narrow for the rail.
func (m *Model) pills() string {
	now := m.now()
	var out []string
	var walk func(id string)
	walk = func(id string) {
		n := m.st.Nodes[id]
		if m.st.State(n, now) == "running" || (m.showAll && id != model.Main) {
			act := n.Activity
			if act == "" {
				act = n.Kind
			} else {
				act = n.Kind + " · " + act
			}
			col := modelColor(n.Model, false)
			out = append(out, fg(col).Render("● ")+fg(colText).Render(act))
		}
		for _, c := range m.st.VisibleChildren(id, now, m.showAll) {
			walk(c)
		}
	}
	walk(model.Main)
	sm := m.st.Summary()
	line := strings.Join(out, fg(colFaint).Render("  "))
	if sm.Turns > 0 {
		line += fg(colFaint).Render("   " + usd(sm.Cost.Total()))
		if hr, ok := sm.HitRate(); ok {
			line += fg(colFaint).Render(fmt.Sprintf(" · cache %.0f%%", hr*100))
		}
	}
	return " " + clip(line, m.w-1)
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

func (m *Model) sendStatus() string {
	var parts []string
	switch {
	case m.snd != nil && m.snd.Busy():
		parts = append(parts, bold(colGreen).Render(spin(m.frame)+" Claude is working…"), fg(colFaint).Render("ctrl+k stops it"))
	case m.snd != nil && m.snd.LastError() != "":
		parts = append(parts, bold(colRed).Render("✗ "+m.snd.LastError()))
	case m.sendErr != "":
		parts = append(parts, bold(colRed).Render("✗ "+m.sendErr))
	default:
		parts = append(parts, fg(colFaint).Render("i / enter: write to Claude"))
	}
	switch m.opt.Permissions {
	case "all":
		parts = append(parts, fg(colOrange).Render("⚠ all permissions (no confirmations)"))
	case "plan":
		parts = append(parts, fg(colFaint).Render("plan mode: read-only"))
	default:
		parts = append(parts, fg(colFaint).Render("accept-edits mode"))
	}
	return " " + clip(strings.Join(parts, "   "), m.w-1)
}

func (m *Model) statusLine() string {
	now := m.now()
	run, total := m.st.RunningSubagents(now)
	sm := m.st.Summary()
	mode := "live"
	if m.rep != nil {
		mode = fmt.Sprintf("replay %d/%d x%g", m.rep.Pos, len(m.rep.Events), m.rep.Speed)
		if m.rep.Paused {
			mode += " [paused]"
		}
	}
	mode += " · " + m.view
	if m.filter != "" {
		mode += " · filter: " + m.filterLabel()
	}
	if m.showAll {
		mode += " · view: all"
	} else {
		mode += " · view: active"
	}
	switch {
	case m.session != "":
		mode += " · session " + short(m.sessionID())
	case m.newSessionID != "":
		mode += " · new session"
	default:
		mode += " · no session (press s)"
	}
	line := fmt.Sprintf("%s · subagents [%d/%d running] · advisor [%d] · jev [%d forks] · %s", mode, run, total,
		m.st.Advisor.Calls, m.st.Jev.Forks, usd(sm.Cost.Total()))
	if hr, ok := sm.HitRate(); ok {
		line += fmt.Sprintf(" · cache %.0f%%", hr*100)
	}
	return fg(colGrey).Render(" " + clip(line, m.w-1))
}

func (m *Model) hints() string {
	type h struct{ k, d string }
	hs := []h{{"q", "quit"}, {"s", "session"}}
	if m.canSend() {
		hs = append(hs, h{"i", "write"})
	}
	hs = append(hs, h{"f", "filter"}, h{"e", "expand"}, h{"t", "tree"}, h{"h", "history"})
	if m.canSend() {
		hs = append(hs, h{"ctrl+k", "stop Claude"})
	}
	if m.rep != nil {
		hs = append(hs, h{"space", "pause"}, h{"+/-", "speed"})
	}
	var parts []string
	for _, x := range hs {
		parts = append(parts, bold(colOrange).Render(x.k)+" "+fg(colDim).Render(x.d))
	}
	line := padRight(clip(" "+strings.Join(parts, "  "), m.w), m.w) // never wrap: one line, whatever the width
	return lipgloss.NewStyle().Background(lipgloss.Color("#13151a")).Render(line)
}

func (m *Model) inputBox() string {
	col := colLine
	if m.inputFocus {
		col = colUser
	}
	st := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(col).Width(m.centerW())
	return st.Render(m.input.View())
}

// Render returns the whole screen as a string.
func (m *Model) Render() string {
	bh := m.bodyH()
	var center string
	if m.view == "chat" {
		center = m.chat.View()
	} else {
		logs := ""
		start := max(0, len(m.st.Log)-9)
		for _, l := range m.st.Log[start:] {
			logs += fg(colFaint).Render(hhmm(l.TS)+"  ") + fg(colGrey).Render(l.Text) + "\n"
		}
		center = m.treeVP.View() + "\n" + box(fg(colDim).Render("log"), strings.TrimRight(logs, "\n"), m.centerW(), colLine)
	}
	center = fitHeight(center, bh)
	body := center
	var lines []string
	lines = append(lines, m.header())
	if rw := m.railW(); rw > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, padBlock(m.rail(bh), rw), center)
	} else {
		lines = append(lines, m.pills())
	}
	lines = append(lines, body)
	if m.view == "chat" && m.canSend() {
		lines = append(lines, m.sendStatus(), m.inputBox())
	}
	lines = append(lines, m.statusLine(), m.hints())
	screen := strings.Join(lines, "\n")
	if m.picker != nil {
		screen = overlay(screen, m.picker.view(m.w, m.h), m.w, m.h)
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
