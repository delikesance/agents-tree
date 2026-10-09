// Package baseline explains what the context already present at the very first API request of a
// session is made of (system prompt, tool schemas, skill/agent/MCP listings, instructions...), which
// of it is re-injected later, and which plugins/skills/MCP servers cost tokens there without having
// been used recently. It is read-only: it never modifies any configuration.
//
// Everything here is an estimate: tokens ~ characters / 4. Exact numbers exist only for the first
// request as a whole (message.usage); the split is derived from what the transcript records.
package baseline

import (
	"encoding/json"
	"hash/fnv"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/delikesance/agents-tree/internal/pricing"
	"github.com/delikesance/agents-tree/internal/sessions"
	"github.com/delikesance/agents-tree/internal/transcript"
)

// DefaultRecent is the number of recent sessions scanned for usage.
const DefaultRecent = 20

// Unprefixed is the owner of skills/agents that carry no "plugin:" prefix (built-in, user, project).
const Unprefixed = "(unprefixed: built-in, user, project)"

// Options are the inputs of Analyze. Everything that touches the disk is injectable.
type Options struct {
	Session     string // transcript path (required)
	ProjectsDir string // where recent sessions are listed (default ~/.claude/projects)
	Home        string // directory that contains .claude (default $HOME)
	Cwd         string // fallback project directory for ./CLAUDE.md (default: the session's cwd, else os.Getwd)
	Recent      int    // sessions scanned for usage (default 20)
}

type obj = map[string]any

// Source is one visible contributor to the first request.
type Source struct {
	Name   string
	Chars  int
	Tokens float64 // estimate: Chars / 4
	N      int     // items (tools, skills, attachments...)
}

// Repeat is an attachment/reminder kind injected more than once.
type Repeat struct {
	Type        string
	Occurrences int
	InBaseline  int
	Later       int
	Distinct    int
	AvgTokens   float64
	ExtraTokReq float64 // sum over later occurrences of tokens x requests that follow (token-requests)
}

// Owner aggregates what a plugin / MCP server / unprefixed group costs in the baseline.
type Owner struct {
	Name                          string
	Skills, Agents, MCPTools      int
	SkillChars, AgentChars        int
	MCPSchemaChars, MCPNameChars  int // tool schemas in the first request / deferred tool names
	MCPInstrChars                 int
	SkillUses, MCPUses, AgentUses int
	UnusedSkills                  []Skill // unprefixed owner only: listed skills never called
}

func (o *Owner) Chars() int {
	return o.SkillChars + o.AgentChars + o.MCPSchemaChars + o.MCPNameChars + o.MCPInstrChars
}
func (o *Owner) Tokens() float64 { return tok(o.Chars()) }
func (o *Owner) Uses() int       { return o.SkillUses + o.MCPUses + o.AgentUses }

// Skill is one entry of the skill listing.
type Skill struct {
	Name  string
	Chars int
}

// ToolSize is the size of one tool schema in the first request.
type ToolSize struct {
	Name   string
	Chars  int
	Tokens float64
}

// FileSize is a local file measured as a cross-check.
type FileSize struct {
	Path   string
	Exists bool
	Bytes  int
	Tokens float64
}

// Config is the local configuration cross-check (read-only, tolerant of missing files).
type Config struct {
	UserClaude, ProjectClaude FileSize
	SkillsDir                 string
	SkillFiles                int
	SkillBytes                int64
	SettingsFound             bool
	EnabledPlugins            []string
	DisabledPlugins           []string
	PluginsDir                string
	PluginEntries             []string
}

// Suggestion is one conditional cut, ranked by tokens x requests.
type Suggestion struct {
	Text        string
	TokPerReq   float64
	TokRequests float64
	Saved       float64 // USD over a session like this one (cache-read price); 0 if price unknown
	WriteOnce   float64 // USD, one-off cache write of the same tokens
}

// Usage counts calls across the scanned sessions.
type Usage struct {
	Sessions int
	Skill    map[string]int
	MCP      map[string]int
	Agent    map[string]int
}

// SubStats describes the first requests of the session's subagents.
type SubStats struct {
	Transcripts, Requests int
	FirstAvg              float64
	FirstMin, FirstMax    int
}

// Report is the full analysis.
type Report struct {
	Session   string
	Cwd       string
	Model     string
	Requests  int     // unique API requests of the main session
	PromptSum float64 // sum of prompt tokens over those requests
	NoRequest bool

	FirstTokens       int // input + cache_creation + cache_read of the first request
	FirstFresh        int
	FirstWrite        int
	FirstRead         int
	Visible           []Source // sorted by tokens desc
	VisibleTokens     float64
	Residual          float64 // FirstTokens - VisibleTokens (estimate)
	HasSnapshot       bool    // system prompt + tool schemas were recorded (prompt_snapshot)
	HasSystemPrompt   bool
	HasTools          bool
	KeptReminders     bool
	HasInstructions   bool // a CLAUDE.md-like "instructions" attachment was in the baseline
	TopTools          []ToolSize
	Repeats           []Repeat
	Owners            []*Owner // sorted by tokens desc
	Usage             Usage
	Recent            int
	Sub               SubStats
	Config            Config
	Suggestions       []Suggestion
	ReadPrice         float64 // $/MTok, 0 = unknown
	WritePrice        float64
	PriceKnown        bool
	UnusedTokensTotal float64
}

func tok(chars int) float64 { return float64(chars) / 4 }
func runes(s string) int    { return utf8.RuneCountInString(s) }
func hash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func str(m obj, k string) string { s, _ := m[k].(string); return s }
func sub(m obj, k string) obj    { o, _ := m[k].(obj); return o }
func list(m obj, k string) []any { l, _ := m[k].([]any); return l }
func intOf(m obj, k string) int  { f, _ := m[k].(float64); return int(f) }

// ---- attachment text --------------------------------------------------------------------------------

// skipKeys are bookkeeping fields that are not injected text.
var skipKeys = map[string]bool{
	"type": true, "names": true, "addedNames": true, "removedNames": true, "wireHiddenNames": true,
	"readdedNames": true, "pendingMcpServers": true, "needsAuthMcpServers": true, "failedMcpServers": true,
	"delivery_id": true, "from": true, "commandMode": true, "uuid": true, "organizationUuid": true,
	"skillCount": true, "isInitial": true,
}

func leaves(v any, out *[]string) {
	switch x := v.(type) {
	case string:
		*out = append(*out, x)
	case []any:
		for _, e := range x {
			leaves(e, out)
		}
	case obj:
		keys := make([]string, 0, len(x))
		for k := range x {
			if !skipKeys[k] {
				keys = append(keys, k)
			}
		}
		sort.Strings(keys)
		for _, k := range keys {
			leaves(x[k], out)
		}
	}
}

// attText is the text an attachment adds to the context (best effort, by known type, else every string).
func attText(typ string, a obj) string {
	var parts []string
	switch typ {
	case "skill_listing":
		if s := str(a, "content"); s != "" {
			return s
		}
		for _, n := range list(a, "names") {
			if s, ok := n.(string); ok {
				parts = append(parts, "- "+s)
			}
		}
		return strings.Join(parts, "\n")
	case "instructions":
		for _, f := range list(a, "files") {
			if fm, ok := f.(obj); ok {
				parts = append(parts, str(fm, "content"))
			}
		}
		return strings.Join(parts, "\n")
	case "deferred_tools_delta", "agent_listing_delta":
		leaves(a["addedLines"], &parts)
		return strings.Join(parts, "\n")
	case "mcp_instructions_delta":
		leaves(a["addedBlocks"], &parts)
		return strings.Join(parts, "\n")
	}
	leaves(a, &parts)
	return strings.Join(parts, "\n")
}

var reReminder = regexp.MustCompile(`(?s)<system-reminder>(.*?)</system-reminder>`)

// contentTexts returns the text strings of a message content (string or blocks, incl. tool results).
func contentTexts(content any) []string {
	switch c := content.(type) {
	case string:
		return []string{c}
	case []any:
		var out []string
		for _, b := range c {
			bm, _ := b.(obj)
			switch str(bm, "type") {
			case "text":
				out = append(out, str(bm, "text"))
			case "tool_result":
				out = append(out, contentTexts(bm["content"])...)
			}
		}
		return out
	}
	return nil
}

func reminderKey(body string) string {
	b := strings.TrimSpace(body)
	if i := strings.IndexByte(b, '\n'); i >= 0 {
		b = b[:i]
	}
	r := []rune(b)
	if len(r) > 48 {
		b = string(r[:48]) + "..."
	}
	return "user-text reminder: " + b
}

// ---- owners --------------------------------------------------------------------------------------------

func prefixOwner(name string) string {
	if i := strings.Index(name, ":"); i > 0 {
		return name[:i]
	}
	return Unprefixed
}

// mcpServer splits "mcp__server__tool" (ok=false for other names).
func mcpServer(name string) (server string, ok bool) {
	if !strings.HasPrefix(name, "mcp__") {
		return "", false
	}
	rest := name[len("mcp__"):]
	if i := strings.Index(rest, "__"); i > 0 {
		return rest[:i], true
	}
	return rest, true
}

// mcpOwner maps an MCP server to the plugin that ships it ("plugin_<plugin>_<server>") or "mcp:<server>".
func mcpOwner(server string) string {
	if strings.HasPrefix(server, "plugin_") {
		rest := server[len("plugin_"):]
		if i := strings.Index(rest, "_"); i > 0 {
			return rest[:i]
		}
		return rest
	}
	return "mcp:" + server
}

// ---- session scan ------------------------------------------------------------------------------------

type occ struct {
	typ   string
	hash  uint64
	chars int
	later int // requests that re-send it (n - requests seen before it)
	base  bool
}

// scanned is what one pass over the main transcript yields.
type scanned struct {
	rep         *Report
	sources     map[string]*Source
	occs        []occ
	skills      []Skill
	agents      []Skill
	deferred    map[string]int // mcp server (or "") -> chars of deferred tool names
	mcpInstr    map[string]int // server header -> chars
	toolsBy     map[string]*Source
	cwd         string
	models      map[string]bool
	firstTools  []ToolSize
	haveSkills  bool
	haveAgents  bool
	haveSnapSys bool
}

func (s *scanned) add(name string, chars, n int) {
	if chars <= 0 {
		return
	}
	v := s.sources[name]
	if v == nil {
		v = &Source{Name: name}
		s.sources[name] = v
	}
	v.Chars += chars
	v.N += n
}

func readRows(path string) []obj {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var rows []obj
	transcript.ReadLines(f, func(d map[string]any) { rows = append(rows, d) })
	return rows
}

type reqInfo struct {
	id                      string
	model                   string
	fresh, write, read, w1h int
}

// requests lists the unique API requests of a transcript in order.
func requests(rows []obj) (reqs []reqInfo, index map[string]int) {
	index = map[string]int{}
	for _, d := range rows {
		if str(d, "type") != "assistant" {
			continue
		}
		m := sub(d, "message")
		u := sub(m, "usage")
		if u == nil {
			continue
		}
		key := str(m, "id")
		if key == "" {
			key = str(d, "uuid")
		}
		if _, ok := index[key]; ok {
			continue
		}
		index[key] = len(reqs) + 1
		w := intOf(u, "cache_creation_input_tokens")
		w1h := intOf(sub(u, "cache_creation"), "ephemeral_1h_input_tokens")
		if w1h > w {
			w1h = w
		}
		reqs = append(reqs, reqInfo{id: key, model: str(m, "model"), fresh: intOf(u, "input_tokens"),
			write: w, read: intOf(u, "cache_read_input_tokens"), w1h: w1h})
	}
	return
}

func scanMain(rows []obj, rep *Report) *scanned {
	sc := &scanned{rep: rep, sources: map[string]*Source{}, deferred: map[string]int{}, mcpInstr: map[string]int{},
		toolsBy: map[string]*Source{}, models: map[string]bool{}}
	reqs, index := requests(rows)
	n := len(reqs)
	rep.Requests = n
	if n > 0 {
		r := reqs[0]
		rep.FirstFresh, rep.FirstWrite, rep.FirstRead = r.fresh, r.write, r.read
		rep.FirstTokens = r.fresh + r.write + r.read
		rep.Model = r.model
		for _, q := range reqs {
			rep.PromptSum += float64(q.fresh + q.write + q.read)
		}
	}
	t := 0 // requests seen so far
	for _, d := range rows {
		if sc.cwd == "" {
			sc.cwd = str(d, "cwd")
		}
		switch str(d, "type") {
		case "assistant":
			m := sub(d, "message")
			if sub(m, "usage") != nil {
				key := str(m, "id")
				if key == "" {
					key = str(d, "uuid")
				}
				if index[key] == t+1 {
					t++
				}
			}
		case "user":
			sc.user(d, t, n)
		case "attachment":
			a := sub(d, "attachment")
			sc.attachment(a, t, n)
		}
	}
	return sc
}

func (s *scanned) user(d obj, t, n int) {
	if truthy(d, "isSidechain") {
		return
	}
	for _, txt := range contentTexts(sub(d, "message")["content"]) {
		rest := txt
		for _, m := range reMatches(txt) {
			body := m[1]
			if t == 0 {
				name := "system-reminder blocks in the first user message"
				if strings.Contains(body, "CLAUDE.md") {
					name = "CLAUDE.md contents in the first user message"
					s.rep.HasInstructions = true
				}
				s.add(name, runes(body), 1)
			}
			s.occs = append(s.occs, occ{typ: reminderKey(body), hash: hash(body), chars: runes(body), later: n - t, base: t == 0})
			rest = strings.Replace(rest, m[0], "", 1)
		}
		if t == 0 {
			s.add("first user message", runes(rest), 1)
		}
	}
}

func reMatches(s string) [][]string {
	if !strings.Contains(s, "<system-reminder>") {
		return nil
	}
	return reReminder.FindAllStringSubmatch(s, -1)
}

func truthy(m obj, k string) bool { b, _ := m[k].(bool); return b }

func (s *scanned) attachment(a obj, t, n int) {
	typ := str(a, "type")
	if typ == "" {
		return
	}
	if typ == "prompt_snapshot" {
		s.snapshot(a)
		return
	}
	text := attText(typ, a)
	chars := runes(text)
	s.occs = append(s.occs, occ{typ: typ, hash: hash(typ + "\x00" + text), chars: chars, later: n - t, base: t == 0})
	if t != 0 {
		return
	}
	switch typ {
	case "skill_listing":
		if !s.haveSkills {
			s.skills, s.haveSkills = parseSkills(text), true
		}
	case "agent_listing_delta":
		if !s.haveAgents {
			for _, l := range list(a, "addedLines") {
				if ls, ok := l.(string); ok {
					s.agents = append(s.agents, parseEntry(ls))
				}
			}
			s.haveAgents = true
		}
	case "deferred_tools_delta":
		for _, l := range list(a, "addedLines") {
			if name, ok := l.(string); ok {
				srv, _ := mcpServer(name)
				s.deferred[srv] += runes(name) + 1
			}
		}
	case "mcp_instructions_delta":
		for _, b := range list(a, "addedBlocks") {
			if bs, ok := b.(string); ok {
				s.mcpInstr[blockServer(bs)] += runes(bs) + 1
			}
		}
	case "instructions":
		s.rep.HasInstructions = true
	}
	label := "attachment " + typ
	if typ == "skill_listing" {
		label += " (skill descriptions)"
	}
	s.add(label, chars, 1)
}

func blockServer(b string) string {
	b = strings.TrimSpace(b)
	if strings.HasPrefix(b, "## ") {
		line := b[3:]
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		return strings.TrimSpace(line)
	}
	return ""
}

// snapshot records the system prompt and tool schemas of the first prompt_snapshot that carries them.
func (s *scanned) snapshot(a obj) {
	if truthy(a, "keptReminders") {
		s.rep.KeptReminders = true
	}
	if sp := a["systemPrompt"]; sp != nil && !s.haveSnapSys {
		var parts []string
		leaves(sp, &parts)
		s.haveSnapSys = true
		s.rep.HasSystemPrompt = true
		s.add("system prompt (prompt_snapshot)", runes(strings.Join(parts, "\n")), len(parts))
	}
	tools := list(a, "tools")
	if len(tools) > 0 && !s.rep.HasTools {
		s.rep.HasTools = true
		for _, tl := range tools {
			tm, _ := tl.(obj)
			raw, _ := json.Marshal(tl)
			name := str(tm, "name")
			ch := runes(string(raw))
			s.firstTools = append(s.firstTools, ToolSize{Name: name, Chars: ch, Tokens: tok(ch)})
			label := "tool schemas: built-in"
			if srv, ok := mcpServer(name); ok {
				label = "tool schemas: mcp " + srv
			}
			s.add(label, ch, 1)
		}
	}
}

func parseEntry(line string) Skill {
	head := strings.TrimPrefix(line, "- ")
	name := head
	if i := strings.Index(head, ": "); i > 0 {
		name = head[:i]
	}
	if j := strings.Index(name, " ("); j > 0 {
		name = name[:j]
	}
	return Skill{Name: strings.TrimSpace(name), Chars: runes(line) + 1}
}

// parseSkills splits a skill listing ("- name: description" entries, continuation lines allowed).
func parseSkills(content string) []Skill {
	var out []Skill
	for _, ln := range strings.Split(content, "\n") {
		switch {
		case strings.HasPrefix(ln, "- "):
			out = append(out, parseEntry(ln))
		case len(out) > 0 && ln != "":
			out[len(out)-1].Chars += runes(ln) + 1
		}
	}
	return out
}

// ---- usage -----------------------------------------------------------------------------------------------

var reCommand = regexp.MustCompile(`<command-name>/?([^<\s]+)</command-name>`)

// scanUsage counts Skill, MCP and Agent/Task calls in one transcript file.
func scanUsage(path string, u *Usage, seen map[string]bool) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	transcript.ReadLines(f, func(d map[string]any) {
		m := sub(d, "message")
		switch str(d, "type") {
		case "assistant":
			for _, raw := range list(m, "content") {
				b, _ := raw.(obj)
				if str(b, "type") != "tool_use" {
					continue
				}
				if id := str(b, "id"); id != "" {
					if seen[id] {
						continue
					}
					seen[id] = true
				}
				name := str(b, "name")
				in := sub(b, "input")
				switch {
				case name == "Skill":
					if sk := strings.TrimPrefix(strings.TrimSpace(str(in, "skill")), "/"); sk != "" {
						u.Skill[sk]++
					}
				case name == "Agent" || name == "Task":
					at := str(in, "subagent_type")
					if at == "" {
						at = "general-purpose"
					}
					u.Agent[at]++
				default:
					if srv, ok := mcpServer(name); ok {
						u.MCP[srv]++
					}
				}
			}
		case "user":
			if truthy(d, "isSidechain") {
				return
			}
			if s, ok := m["content"].(string); ok {
				for _, c := range reCommand.FindAllStringSubmatch(s, -1) {
					u.Skill[c[1]]++ // user-typed /skill or /plugin:skill
				}
			}
		}
	})
}

// CollectUsage counts calls over the `recent` most recent sessions (main + subagent transcripts), always
// including `include` when non-empty.
func CollectUsage(projectsDir string, recent int, include string) Usage {
	u := Usage{Skill: map[string]int{}, MCP: map[string]int{}, Agent: map[string]int{}}
	var paths []string
	for _, s := range sessions.List(projectsDir, recent) {
		paths = append(paths, s.Path)
	}
	if include != "" {
		found := false
		for _, p := range paths {
			if p == include {
				found = true
			}
		}
		if !found {
			paths = append(paths, include)
		}
	}
	for _, p := range paths {
		u.Sessions++
		seen := map[string]bool{}
		for _, f := range transcript.SessionFiles(p) {
			scanUsage(f.Path, &u, seen)
		}
	}
	return u
}

// subStats reads the first request of each subagent transcript of a session.
func subStats(session string) SubStats {
	var st SubStats
	var sum int
	for _, f := range transcript.SessionFiles(session) {
		if !f.Sidechain {
			continue
		}
		reqs, _ := requests(readRows(f.Path))
		if len(reqs) == 0 {
			continue
		}
		first := reqs[0].fresh + reqs[0].write + reqs[0].read
		st.Transcripts++
		st.Requests += len(reqs)
		sum += first
		if st.FirstMin == 0 || first < st.FirstMin {
			st.FirstMin = first
		}
		if first > st.FirstMax {
			st.FirstMax = first
		}
	}
	if st.Transcripts > 0 {
		st.FirstAvg = float64(sum) / float64(st.Transcripts)
	}
	return st
}

// ---- config --------------------------------------------------------------------------------------------------

func measure(path string) FileSize {
	fs := FileSize{Path: path}
	if st, err := os.Stat(path); err == nil && !st.IsDir() {
		fs.Exists, fs.Bytes, fs.Tokens = true, int(st.Size()), float64(st.Size())/4
	}
	return fs
}

// ReadConfig measures the local configuration. Missing or unknown layouts are tolerated.
func ReadConfig(home, cwd string) Config {
	c := Config{}
	claude := filepath.Join(home, ".claude")
	c.UserClaude = measure(filepath.Join(claude, "CLAUDE.md"))
	if cwd != "" {
		c.ProjectClaude = measure(filepath.Join(cwd, "CLAUDE.md"))
	}
	c.SkillsDir = filepath.Join(claude, "skills")
	filepath.WalkDir(c.SkillsDir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			c.SkillFiles++
			if info, e := d.Info(); e == nil {
				c.SkillBytes += info.Size()
			}
		}
		return nil
	})
	if raw, err := os.ReadFile(filepath.Join(claude, "settings.json")); err == nil {
		var st struct {
			Enabled map[string]any `json:"enabledPlugins"`
		}
		if json.Unmarshal(raw, &st) == nil {
			c.SettingsFound = true
			for k, v := range st.Enabled {
				if b, ok := v.(bool); ok && !b {
					c.DisabledPlugins = append(c.DisabledPlugins, k)
				} else {
					c.EnabledPlugins = append(c.EnabledPlugins, k)
				}
			}
			sort.Strings(c.EnabledPlugins)
			sort.Strings(c.DisabledPlugins)
		}
	}
	c.PluginsDir = filepath.Join(claude, "plugins")
	if raw, err := os.ReadFile(filepath.Join(c.PluginsDir, "installed_plugins.json")); err == nil {
		var ip struct {
			Plugins map[string]any `json:"plugins"`
		}
		if json.Unmarshal(raw, &ip) == nil {
			for k := range ip.Plugins {
				c.PluginEntries = append(c.PluginEntries, k)
			}
		}
	}
	if len(c.PluginEntries) == 0 { // unknown layout: list directories one and two levels down
		top, _ := os.ReadDir(c.PluginsDir)
		for _, e := range top {
			if !e.IsDir() {
				continue
			}
			c.PluginEntries = append(c.PluginEntries, e.Name()+"/")
			subs, _ := os.ReadDir(filepath.Join(c.PluginsDir, e.Name()))
			for _, s := range subs {
				if s.IsDir() {
					c.PluginEntries = append(c.PluginEntries, e.Name()+"/"+s.Name())
				}
			}
		}
	}
	sort.Strings(c.PluginEntries)
	return c
}

// ---- analysis --------------------------------------------------------------------------------------------------

// Analyze builds the report. It only reads files.
func Analyze(o Options) (*Report, error) {
	if o.Recent <= 0 {
		o.Recent = DefaultRecent
	}
	if o.Home == "" {
		o.Home, _ = os.UserHomeDir()
	}
	if _, err := os.Stat(o.Session); err != nil {
		return nil, err
	}
	rep := &Report{Session: o.Session, Recent: o.Recent}
	rows := readRows(o.Session)
	sc := scanMain(rows, rep)
	rep.Cwd = sc.cwd
	if rep.Cwd == "" {
		rep.Cwd = o.Cwd
	}
	if rep.Cwd == "" {
		rep.Cwd, _ = os.Getwd()
	}
	rep.NoRequest = rep.Requests == 0
	if r, ok := pricing.RatesFor(rep.Model); ok && rep.Model != "" {
		rep.PriceKnown = true
		rep.ReadPrice = r.CacheRead
		if rep.ReadPrice <= 0 {
			rep.ReadPrice = r.In * pricing.ReadDefault
		}
		rep.WritePrice = r.In * pricing.Write5m
		if first := firstRequest(rows); first.w1h > 0 {
			rep.WritePrice = r.In * pricing.Write1h
		}
	}
	rep.HasSnapshot = rep.HasSystemPrompt && rep.HasTools

	for _, s := range sc.sources {
		s.Tokens = tok(s.Chars)
		rep.Visible = append(rep.Visible, *s)
		rep.VisibleTokens += s.Tokens
	}
	sort.Slice(rep.Visible, func(i, j int) bool { return rep.Visible[i].Tokens > rep.Visible[j].Tokens })
	if rep.FirstTokens > 0 {
		rep.Residual = float64(rep.FirstTokens) - rep.VisibleTokens
	}
	sort.Slice(sc.firstTools, func(i, j int) bool { return sc.firstTools[i].Chars > sc.firstTools[j].Chars })
	if len(sc.firstTools) > 8 {
		sc.firstTools = sc.firstTools[:8]
	}
	rep.TopTools = sc.firstTools

	rep.Repeats = repeats(sc.occs, rep.Requests)
	rep.Usage = CollectUsage(o.ProjectsDir, o.Recent, o.Session)
	rep.Owners = owners(sc, rep)
	rep.Sub = subStats(o.Session)
	rep.Config = ReadConfig(o.Home, rep.Cwd)
	rep.Suggestions = suggest(rep)
	return rep, nil
}

func firstRequest(rows []obj) reqInfo {
	reqs, _ := requests(rows)
	if len(reqs) == 0 {
		return reqInfo{}
	}
	return reqs[0]
}

func repeats(occs []occ, n int) []Repeat {
	by := map[string]*Repeat{}
	dist := map[string]map[uint64]bool{}
	chars := map[string]int{}
	for _, o := range occs {
		r := by[o.typ]
		if r == nil {
			r = &Repeat{Type: o.typ}
			by[o.typ] = r
			dist[o.typ] = map[uint64]bool{}
		}
		r.Occurrences++
		dist[o.typ][o.hash] = true
		chars[o.typ] += o.chars
		if o.base {
			r.InBaseline++
		} else {
			r.Later++
			r.ExtraTokReq += tok(o.chars) * float64(o.later)
		}
	}
	var out []Repeat
	for k, r := range by {
		r.Distinct = len(dist[k])
		if r.Occurrences > 0 {
			r.AvgTokens = tok(chars[k]) / float64(r.Occurrences)
		}
		if r.Occurrences >= 2 && chars[k] > 0 && r.Later > 0 {
			out = append(out, *r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExtraTokReq > out[j].ExtraTokReq })
	return out
}

// owners attributes skill/agent listings, MCP schemas, deferred names and MCP instructions to plugins.
func owners(sc *scanned, rep *Report) []*Owner {
	m := map[string]*Owner{}
	get := func(n string) *Owner {
		o := m[n]
		if o == nil {
			o = &Owner{Name: n}
			m[n] = o
		}
		return o
	}
	listed := map[string]string{} // short name -> full skill name, for unprefixed usage resolution
	for _, s := range sc.skills {
		o := get(prefixOwner(s.Name))
		o.Skills++
		o.SkillChars += s.Chars
		if i := strings.Index(s.Name, ":"); i > 0 {
			listed[s.Name[i+1:]] = s.Name
		}
	}
	for _, a := range sc.agents {
		o := get(prefixOwner(a.Name))
		o.Agents++
		o.AgentChars += a.Chars
	}
	// MCP schemas in the first request, per server (recorded in the tool snapshot)
	for _, src := range rep.Visible {
		if srv := strings.TrimPrefix(src.Name, "tool schemas: mcp "); srv != src.Name {
			o := get(mcpOwner(srv))
			o.MCPSchemaChars += src.Chars
			o.MCPTools += src.N
		}
	}
	for srv, ch := range sc.deferred {
		if srv == "" {
			continue // deferred built-in tool names are not attributable to a plugin
		}
		get(mcpOwner(srv)).MCPNameChars += ch
	}
	for srv, ch := range sc.mcpInstr {
		if srv == "" {
			continue
		}
		get(mcpOwner(srv)).MCPInstrChars += ch
	}
	// usage
	used := map[string]bool{}
	for sk, c := range rep.Usage.Skill {
		if full, ok := listed[sk]; ok && !strings.Contains(sk, ":") {
			sk = full
		}
		used[sk] = true
		o := m[prefixOwner(sk)]
		if o != nil {
			o.SkillUses += c
		}
	}
	for srv, c := range rep.Usage.MCP {
		if o := m[mcpOwner(srv)]; o != nil {
			o.MCPUses += c
		}
	}
	for at, c := range rep.Usage.Agent {
		if o := m[prefixOwner(at)]; o != nil {
			o.AgentUses += c
		}
	}
	if un := m[Unprefixed]; un != nil {
		for _, s := range sc.skills {
			if prefixOwner(s.Name) == Unprefixed && !used[s.Name] {
				un.UnusedSkills = append(un.UnusedSkills, s)
			}
		}
		sort.Slice(un.UnusedSkills, func(i, j int) bool { return un.UnusedSkills[i].Chars > un.UnusedSkills[j].Chars })
	}
	var out []*Owner
	for _, o := range m {
		if o.Chars() > 0 {
			out = append(out, o)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Chars() > out[j].Chars() })
	return out
}
