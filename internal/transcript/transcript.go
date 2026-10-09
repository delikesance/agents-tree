// Package transcript turns Claude Code transcript JSONL lines into normalized events.
package transcript

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/delikesance/agents-tree/internal/model"
)

const (
	jevPrefix = "mcp__jev__"
	maxText   = 6000
)

var (
	spawnTools  = map[string]bool{"Agent": true, "Task": true}
	hiddenTools = map[string]bool{"SubagentHandback": true} // shown as the delegation's report instead
)

type obj = map[string]any

// ---- small JSON accessors ---------------------------------------------------------------------

func str(m obj, k string) string {
	s, _ := m[k].(string)
	return s
}
func sub(m obj, k string) obj {
	o, _ := m[k].(obj)
	return o
}
func list(m obj, k string) []any {
	l, _ := m[k].([]any)
	return l
}
func num(m obj, k string) int {
	f, _ := m[k].(float64)
	return int(f)
}
func truthy(m obj, k string) bool {
	b, _ := m[k].(bool)
	return b
}

func tsOf(v string) float64 {
	if v == "" {
		return 0
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return 0
	}
	return float64(t.UnixNano()) / 1e9
}

// textOf joins the text blocks of a tool_result content (string or block list).
func textOf(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case []any:
		var parts []string
		for _, b := range c {
			if m, ok := b.(obj); ok {
				parts = append(parts, str(m, "text"))
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

var (
	reReminder = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	reTag      = regexp.MustCompile(`</?[a-zA-Z][\w-]*(?:\s[^>]*)?>`)
	reCmdName  = regexp.MustCompile(`<command-name>\s*(/?[^<\s]+)\s*</command-name>`)
	reCmdArgs  = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// CleanText strips injected reminders and markup tags, keeping the human text.
func CleanText(text string) string {
	text = reReminder.ReplaceAllString(text, "")
	if m := reCmdName.FindStringSubmatch(text); m != nil {
		out := m[1]
		if a := reCmdArgs.FindStringSubmatch(text); a != nil && strings.TrimSpace(a[1]) != "" {
			out += " " + strings.TrimSpace(a[1])
		}
		return strings.TrimSpace(out)
	}
	return strings.TrimSpace(reTag.ReplaceAllString(text, ""))
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return cut(strings.SplitN(s, "\n", 2)[0], 120)
}

// ToolDetail is a short line describing what a tool call does.
func ToolDetail(name string, in obj) string {
	switch name {
	case "Bash":
		cmd := strings.Split(strings.TrimSpace(str(in, "command")), "\n")
		if len(cmd) > 1 && strings.Contains(cmd[0], "<<") { // heredoc: the first line alone says nothing
			return cut(fmt.Sprintf("%s ⏎ %s", cut(cmd[0], 40), strings.TrimSpace(cmd[1])), 120)
		}
		return firstLine(str(in, "command"))
	case "Read", "Edit", "Write", "NotebookEdit", "MultiEdit":
		p := str(in, "file_path")
		if p == "" {
			p = str(in, "notebook_path")
		}
		parts := strings.Split(p, "/")
		if len(parts) > 3 {
			parts = parts[len(parts)-3:]
		}
		return strings.Join(parts, "/")
	case "Grep", "Glob":
		s := firstLine(str(in, "pattern"))
		if p := str(in, "path"); p != "" {
			s += "  in " + p
		}
		return s
	case "WebFetch", "WebSearch":
		if u := str(in, "url"); u != "" {
			return firstLine(u)
		}
		return firstLine(str(in, "query"))
	case "Agent", "Task":
		return firstLine(str(in, "description"))
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys { // MCP and other tools: first short string argument
		if s, ok := in[k].(string); ok && strings.TrimSpace(s) != "" {
			return firstLine(s)
		}
	}
	return ""
}

func lastSegment(name string) string {
	i := strings.LastIndex(name, "__")
	if i < 0 {
		return name
	}
	return name[i+2:]
}

func lastTool(content []any) string {
	last := ""
	for _, b := range content {
		if m, ok := b.(obj); ok {
			if t := str(m, "type"); t == "tool_use" || t == "server_tool_use" {
				last = lastSegment(str(m, "name"))
			}
		}
	}
	return last
}

// ---- parser ---------------------------------------------------------------------------------------

type pend struct{ kind, name string }

// Parser is a stateful per-file parser: it pairs tool_use with its tool_result.
type Parser struct {
	AgentID   string
	Sidechain bool

	pending    map[string]pend
	toolTS     map[string]float64
	seenPrompt bool
	n          int
	seenMsg    map[string]bool // API message ids whose usage was already counted
}

func NewParser(agentID string, sidechain bool) *Parser {
	return &Parser{AgentID: agentID, Sidechain: sidechain, pending: map[string]pend{},
		toolTS: map[string]float64{}, seenMsg: map[string]bool{}}
}

func (p *Parser) id(d obj, suffix string) string {
	p.n++
	if u := str(d, "uuid"); u != "" {
		return u + suffix
	}
	return fmt.Sprintf("%s-%d%s", p.AgentID, p.n, suffix)
}

func fp(v float64) *float64 { return &v }

// Parse converts one transcript line into zero or more events.
func (p *Parser) Parse(d obj) []model.Event {
	ts := tsOf(str(d, "timestamp"))
	msg := sub(d, "message")
	switch str(d, "type") {
	case "queue-operation": // background task notifications are queued here
		if str(d, "operation") == "enqueue" {
			return p.notification(ts, str(d, "content"))
		}
	case "assistant":
		if content, ok := msg["content"].([]any); ok {
			return p.assistant(d, msg, content, ts)
		}
	case "user":
		switch c := msg["content"].(type) {
		case string:
			return p.user(d, []any{obj{"type": "text", "text": c}}, ts)
		case []any:
			return p.user(d, c, ts)
		}
	}
	return nil
}

func (p *Parser) assistant(d, msg obj, content []any, ts float64) []model.Event {
	var out []model.Event
	mid := str(msg, "id")
	// Claude Code writes one line per content block, each repeating the request's usage:
	// count that usage once per message id.
	dup := mid != "" && p.seenMsg[mid]
	if mid != "" {
		p.seenMsg[mid] = true
	}
	if u := sub(msg, "usage"); u != nil {
		effort := str(d, "perTurnEffort")
		if effort == "" {
			effort = str(d, "effort")
		}
		cc := sub(u, "cache_creation")
		out = append(out, model.Event{TS: ts, Kind: model.Turn, AgentID: p.AgentID, Dup: dup,
			Model: str(msg, "model"), Effort: effort, Sidechain: p.Sidechain,
			AdvisorModel: str(d, "advisorModel"), Tool: lastTool(content),
			Usage: &model.Usage{Input: num(u, "input_tokens"), CacheCreation: num(u, "cache_creation_input_tokens"),
				Cache1h: num(cc, "ephemeral_1h_input_tokens"), CacheRead: num(u, "cache_read_input_tokens"),
				Output: num(u, "output_tokens")}})
	}
	modelName := str(msg, "model")
	for i, raw := range content {
		c, ok := raw.(obj)
		if !ok {
			continue
		}
		typ := str(c, "type")
		if typ == "text" && strings.TrimSpace(str(c, "text")) != "" {
			out = append(out, model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
				ID: p.id(d, fmt.Sprintf(":%d", i)), TS: ts, AgentID: p.AgentID, Role: "assistant",
				Text: cut(strings.TrimSpace(str(c, "text")), maxText), Model: modelName}})
		}
		if typ != "tool_use" && typ != "server_tool_use" {
			continue
		}
		name, tid, in := str(c, "name"), str(c, "id"), sub(c, "input")
		p.toolTS[tid] = ts
		switch {
		case spawnTools[name]:
			kind := str(in, "subagent_type")
			if kind == "" {
				kind = "agent"
			}
			p.pending[tid] = pend{"spawn", name}
			out = append(out,
				model.Event{TS: ts, Kind: model.AgentStart, AgentID: tid, ParentID: p.AgentID,
					AgentKind: kind, Desc: str(in, "description"), Model: str(in, "model")},
				model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
					ID: "deleg:" + tid, TS: ts, AgentID: p.AgentID, Role: "delegation",
					Text: cut(strings.TrimSpace(str(in, "prompt")), maxText), Kind: kind,
					Desc: str(in, "description"), Model: str(in, "model"), Target: tid}})
		case !hiddenTools[name]:
			out = append(out, model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
				ID: tid, TS: ts, AgentID: p.AgentID, Role: "tool", Tool: lastSegment(name),
				Detail: ToolDetail(name, in), Status: "running"}})
		}
		switch {
		case spawnTools[name]:
		case name == "advisor":
			p.pending[tid] = pend{"advisor", name}
			out = append(out, model.Event{TS: ts, Kind: model.Advisor, AgentID: p.AgentID, Model: str(d, "advisorModel")})
		case strings.HasPrefix(name, jevPrefix):
			p.pending[tid] = pend{"jev", strings.TrimPrefix(name, jevPrefix)}
		}
	}
	return out
}

func (p *Parser) user(d obj, blocks []any, ts float64) []model.Event {
	hasResult := false
	for _, b := range blocks {
		if m, ok := b.(obj); ok && str(m, "type") == "tool_result" {
			hasResult = true
		}
	}
	if !hasResult {
		return p.userMessage(d, blocks, ts)
	}
	var out []model.Event
	for _, b := range blocks {
		if m, ok := b.(obj); ok && str(m, "type") == "tool_result" {
			out = append(out, p.toolResult(d, m, ts)...)
		}
	}
	return out
}

var (
	reNotif    = regexp.MustCompile(`(?s)<task-notification>(.*?)</task-notification>`)
	reAgentMsg = regexp.MustCompile(`(?s)<agent-message from="([^"]+)">\s*(.*?)</agent-message>`)
)

func xmlField(s, name string) string {
	m := regexp.MustCompile(`(?s)<` + name + `>(.*?)</` + name + `>`).FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// notification handles "<task-notification>" (a background agent or command finished): it ends the
// agent whose launch tool_result returned immediately, and updates the tool lines.
func (p *Parser) notification(ts float64, content string) []model.Event {
	m := reNotif.FindStringSubmatch(content)
	if m == nil {
		return nil
	}
	body := m[1]
	tid, status := xmlField(body, "tool-use-id"), xmlField(body, "status")
	if tid == "" {
		return nil
	}
	end, msgStatus := "done", "ok"
	switch status {
	case "completed", "":
	default: // failed, killed, cancelled...
		end, msgStatus = "failed", "error"
	}
	out := []model.Event{
		{TS: ts, Kind: model.AgentEnd, AgentID: tid, Status: end},
		{TS: ts, Kind: model.MsgUpdate, AgentID: p.AgentID, MsgID: tid, Status: msgStatus},
		{TS: ts, Kind: model.MsgUpdate, AgentID: p.AgentID, MsgID: "deleg:" + tid, Status: end},
	}
	if end == "failed" { // a failed agent sends no report: say so
		out = append(out, model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
			ID: "rep:" + tid, TS: ts, AgentID: p.AgentID, Role: "report", Status: "failed", Target: tid,
			Text: cut(xmlField(body, "summary"), maxText)}})
	}
	return out
}

// handbacks extracts reports that background agents deliver as "<agent-message from=...>" reminders.
func (p *Parser) handbacks(text string, ts float64) []model.Event {
	var out []model.Event
	for _, m := range reAgentMsg.FindAllStringSubmatch(text, -1) {
		body := m[2]
		if i := strings.Index(body, "The report follows:"); i >= 0 {
			body = body[i+len("The report follows:"):]
		}
		lines := strings.Split(body, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimPrefix(l, "  ")
		}
		report := strings.TrimSpace(strings.Join(lines, "\n"))
		if report == "" {
			continue
		}
		out = append(out, model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
			ID: "rep:" + m[1], TS: ts, AgentID: p.AgentID, Role: "report", Status: "done", Target: m[1],
			Kind: "handback", Text: cut(report, maxText)}})
	}
	return out
}

func (p *Parser) userMessage(d obj, blocks []any, ts float64) []model.Event {
	if p.Sidechain && !p.seenPrompt {
		p.seenPrompt = true // the delegated prompt is already shown as the delegation
		return nil
	}
	var texts []string
	images := 0
	for _, b := range blocks {
		m, ok := b.(obj)
		if !ok {
			continue
		}
		switch str(m, "type") {
		case "text":
			texts = append(texts, str(m, "text"))
		case "image":
			images++
		}
	}
	joined := strings.Join(texts, "\n")
	hand := p.handbacks(joined, ts)
	cleaned := CleanText(joined)
	if images > 0 {
		label := "image"
		if images > 1 {
			label = fmt.Sprintf("%d images", images)
		}
		if cleaned != "" {
			cleaned += "  "
		}
		cleaned += "▣ " + label
	}
	if cleaned == "" {
		return hand
	}
	origin := sub(d, "origin")
	human := str(origin, "kind") == "human" || str(d, "turnOrigin") == "human" || (origin == nil && !truthy(d, "isMeta"))
	role := "system"
	if human && !truthy(d, "isMeta") {
		role = "user"
	}
	if strings.HasPrefix(cleaned, "[Request interrupted") {
		role = "system"
	}
	return append(hand, model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
		ID: p.id(d, ""), TS: ts, AgentID: p.AgentID, Role: role, Text: cut(cleaned, maxText)}})
}

func (p *Parser) toolResult(d, c obj, ts float64) []model.Event {
	var out []model.Event
	tid := str(c, "tool_use_id")
	var duration *float64
	if started, ok := p.toolTS[tid]; ok {
		delete(p.toolTS, tid)
		if ts >= started {
			v := float64(int((ts-started)*100+0.5)) / 100
			duration = &v
		}
	}
	failed := truthy(c, "is_error")
	status := "ok"
	if failed {
		status = "error"
	}
	pd, hasPending := p.pending[tid]
	delete(p.pending, tid)
	if !hasPending || pd.kind != "spawn" {
		out = append(out, model.Event{TS: ts, Kind: model.MsgUpdate, AgentID: p.AgentID, MsgID: tid, Status: status, MsgDuration: duration})
	}
	if !hasPending {
		return out
	}
	switch pd.kind {
	case "spawn":
		end := "done"
		if failed {
			end = "failed"
		}
		res := sub(d, "toolUseResult")
		if res != nil {
			if aid := str(res, "agentId"); aid != "" {
				out = append(out, model.Event{TS: ts, Kind: model.Alias, AgentID: tid, AliasID: aid})
			}
		}
		if res != nil && (truthy(res, "isAsync") || str(res, "status") == "async_launched") {
			// Launched in the background: this result only says "started". The agent keeps running until
			// its <task-notification> (or until it goes silent); its report arrives as a hand-back message.
			return out
		}
		report := strings.TrimSpace(strings.SplitN(textOf(c["content"]), "agentId:", 2)[0])
		out = append(out,
			model.Event{TS: ts, Kind: model.AgentEnd, AgentID: tid, Status: end},
			model.Event{TS: ts, Kind: model.Message_, AgentID: p.AgentID, Msg: &model.Message{
				ID: "rep:" + tid, TS: ts, AgentID: p.AgentID, Role: "report", Text: cut(report, maxText),
				Status: end, Duration: duration, Target: tid}},
			model.Event{TS: ts, Kind: model.MsgUpdate, AgentID: p.AgentID, MsgID: "deleg:" + tid, Status: end, MsgDuration: duration})
	case "jev":
		var parsed obj
		_ = json.Unmarshal([]byte(textOf(c["content"])), &parsed)
		ev := model.Event{TS: ts, Kind: model.Jev, AgentID: p.AgentID, Decision: pd.name, Escalate: truthy(parsed, "escalate")}
		if v, ok := parsed["confidence"].(float64); ok {
			ev.Confidence = &v
		}
		out = append(out, ev)
	case "advisor":
		out = append(out, model.Event{TS: ts, Kind: model.Advisor, AgentID: p.AgentID,
			Advice: cut(strings.TrimSpace(textOf(c["content"])), 200), NoCount: true})
	}
	return out
}

// ---- files ----------------------------------------------------------------------------------------

// File is one transcript of a session: the main one or a subagent's.
type File struct {
	Path      string
	AgentID   string
	Sidechain bool
}

// SessionFiles lists the main transcript plus any subagent transcripts stored next to it.
func SessionFiles(sessionJSONL string) []File {
	files := []File{{Path: sessionJSONL, AgentID: model.Main}}
	dir := strings.TrimSuffix(sessionJSONL, filepath.Ext(sessionJSONL))
	matches, _ := filepath.Glob(filepath.Join(dir, "subagents", "agent-*.jsonl"))
	sort.Strings(matches)
	for _, m := range matches {
		id := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "agent-"), ".jsonl")
		files = append(files, File{Path: m, AgentID: id, Sidechain: true})
	}
	return files
}

// MetaAlias reads agent-<id>.meta.json ({"toolUseId": ...}): a deterministic agent-id link.
func MetaAlias(f File) (model.Event, bool) {
	raw, err := os.ReadFile(strings.TrimSuffix(f.Path, ".jsonl") + ".meta.json")
	if err != nil {
		return model.Event{}, false
	}
	var meta obj
	if json.Unmarshal(raw, &meta) != nil || str(meta, "toolUseId") == "" {
		return model.Event{}, false
	}
	return model.Event{Kind: model.Alias, AgentID: str(meta, "toolUseId"), AliasID: f.AgentID}, true
}

// ReadLines calls fn for each JSON object line of r (lines may be very long).
func ReadLines(r io.Reader, fn func(obj)) {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var d obj
			if json.Unmarshal(line, &d) == nil {
				fn(d)
			}
		}
		if err != nil {
			return
		}
	}
}

// ReadSession reads every transcript of a session into time-ordered events.
func ReadSession(sessionJSONL string) ([]model.Event, error) {
	var events []model.Event
	for _, f := range SessionFiles(sessionJSONL) {
		fh, err := os.Open(f.Path)
		if err != nil {
			if f.AgentID == model.Main {
				return nil, err
			}
			continue
		}
		p := NewParser(f.AgentID, f.Sidechain)
		ReadLines(fh, func(d obj) { events = append(events, p.Parse(d)...) })
		fh.Close()
		if f.Sidechain {
			if a, ok := MetaAlias(f); ok {
				events = append(events, a)
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS < events[j].TS })
	return events, nil
}
