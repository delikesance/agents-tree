package baseline

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---- helpers ------------------------------------------------------------------------------------------------

func writeJSONL(t *testing.T, path string, rows ...any) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, r := range rows {
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func att(typ string, f obj) obj {
	a := obj{"type": typ}
	for k, v := range f {
		a[k] = v
	}
	return obj{"type": "attachment", "attachment": a}
}

func user(content any) obj {
	return obj{"type": "user", "cwd": "/proj", "message": obj{"role": "user", "content": content}}
}

func toolUse(id, name string, in obj) obj {
	return obj{"type": "tool_use", "id": id, "name": name, "input": in}
}

func asst(id string, fresh, write, read int, blocks ...any) obj {
	if blocks == nil {
		blocks = []any{obj{"type": "text", "text": "ok"}}
	}
	return obj{"type": "assistant", "uuid": "u-" + id, "message": obj{"id": id, "model": "claude-sonnet-5-5", "role": "assistant",
		"content": blocks,
		"usage": obj{"input_tokens": fresh, "cache_creation_input_tokens": write, "cache_read_input_tokens": read,
			"cache_creation": obj{"ephemeral_1h_input_tokens": write}}}}
}

func approx(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9+1e-6*math.Abs(want) {
		t.Errorf("%s = %.3f, want %.3f", name, got, want)
	}
}

func chars(s string) int { return len([]rune(s)) }

// session1 builds a synthetic main session plus one subagent transcript under projects/p/.
func session1(t *testing.T, root string) string {
	t.Helper()
	tools := []any{
		obj{"name": "Bash", "description": "run", "schema": obj{}},
		obj{"name": "mcp__plugin_pw_pw__click", "description": "click", "schema": obj{"a": 1}},
	}
	sys := []any{strings.Repeat("s", 400)}
	main := filepath.Join(root, "projects", "p", "s1.jsonl")
	writeJSONL(t, main,
		user("<system-reminder>CLAUDE.md says hi</system-reminder>hello"),
		att("skill_listing", obj{"content": "- plug:one: aaaa\n- plug:two: bbbb\n- plain: cc\n- unusedplain: dd", "skillCount": 4}),
		att("deferred_tools_delta", obj{"addedLines": []any{"mcp__srv__a", "mcp__srv__b", "WebFetch"}}),
		att("mcp_instructions_delta", obj{"addedBlocks": []any{"## srv\nuse it"}}),
		att("deferred_tools_record", obj{"entries": []any{}, "toolInputCopies": []any{obj{"copy": "wire", "id": "toolu_1"}}}),
		att("prompt_snapshot", obj{"systemPrompt": sys, "tools": tools, "keptReminders": true}),
		asst("r1", 2, 800, 200, toolUse("t1", "Skill", obj{"skill": "plug:one"}), toolUse("t2", "mcp__srv__a", obj{})),
		att("total_tokens_reminder", obj{"text": "<total_tokens>9 left</total_tokens>"}),
		asst("r2", 2, 10, 1000, toolUse("t3", "Agent", obj{"subagent_type": "Explore"})),
		att("total_tokens_reminder", obj{"text": "<total_tokens>9 left</total_tokens>"}),
		asst("r3", 2, 10, 1010, toolUse("t3", "Agent", obj{"subagent_type": "Explore"})), // t3 streamed twice
		asst("r3", 2, 10, 1010),
	)
	writeJSONL(t, filepath.Join(root, "projects", "p", "s1", "subagents", "agent-x.jsonl"),
		asst("a1", 5, 0, 395, toolUse("s1", "Skill", obj{"skill": "plain"}), toolUse("s2", "mcp__srv__b", obj{})),
		asst("a2", 5, 0, 400),
	)
	return main
}

// ---- tests ----------------------------------------------------------------------------------------------------

func TestAttText(t *testing.T) {
	tests := []struct {
		name string
		typ  string
		a    obj
		want string
	}{
		{"skill listing content", "skill_listing", obj{"content": "- a: b", "names": []any{"zzz"}}, "- a: b"},
		{"skill listing names only", "skill_listing", obj{"names": []any{"a", "b"}}, "- a\n- b"},
		{"instructions files", "instructions", obj{"files": []any{obj{"content": "X", "path": "/p"}, obj{"content": "Y"}}}, "X\nY"},
		{"deferred names", "deferred_tools_delta", obj{"addedLines": []any{"A", "B"}, "addedNames": []any{"A", "B"}}, "A\nB"},
		{"mcp blocks", "mcp_instructions_delta", obj{"addedBlocks": []any{"## s\nhi"}}, "## s\nhi"},
		{"bookkeeping is empty", "deferred_tools_record", obj{"toolInputCopies": []any{obj{"id": "toolu_1"}}}, ""},
		{"generic leaves skip ids", "silent_turn_reminder", obj{"text": "hey", "delivery_id": "d-1", "isMeta": true}, "hey"},
		{"empty task reminder", "task_reminder", obj{"content": []any{}, "itemCount": 0}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := attText(tc.typ, tc.a); got != tc.want {
				t.Errorf("attText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseSkills(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		names   []string
		owners  []string
		lastLen int
	}{
		{"plugin and plain", "- eng:one: desc\n- plain: d2", []string{"eng:one", "plain"}, []string{"eng", Unprefixed}, chars("- plain: d2") + 1},
		{"parenthetical", "- session-start-hook (startup-hook-skill): x", []string{"session-start-hook"}, []string{Unprefixed}, 0},
		{"continuation line counts", "- a:b: first\nmore text\n- c: d", []string{"a:b", "c"}, []string{"a", Unprefixed}, 0},
		{"empty", "", nil, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSkills(tc.in)
			if len(got) != len(tc.names) {
				t.Fatalf("got %d skills, want %d", len(got), len(tc.names))
			}
			for i, s := range got {
				if s.Name != tc.names[i] || prefixOwner(s.Name) != tc.owners[i] {
					t.Errorf("skill %d = %q owner %q, want %q owner %q", i, s.Name, prefixOwner(s.Name), tc.names[i], tc.owners[i])
				}
			}
			if tc.lastLen > 0 && got[len(got)-1].Chars != tc.lastLen {
				t.Errorf("last chars = %d, want %d", got[len(got)-1].Chars, tc.lastLen)
			}
		})
	}
	if got := parseSkills("- a:b: first\nmore text\n- c: d"); got[0].Chars != chars("- a:b: first")+1+chars("more text")+1 {
		t.Errorf("continuation chars = %d", got[0].Chars)
	}
}

func TestMCPOwners(t *testing.T) {
	tests := []struct{ tool, server, owner string }{
		{"mcp__github__get_me", "github", "mcp:github"},
		{"mcp__plugin_playwright_playwright__browser_click", "plugin_playwright_playwright", "playwright"},
		{"mcp__Claude_Docs__batch", "Claude_Docs", "mcp:Claude_Docs"},
	}
	for _, tc := range tests {
		srv, ok := mcpServer(tc.tool)
		if !ok || srv != tc.server || mcpOwner(srv) != tc.owner {
			t.Errorf("%s -> %q (%v) owner %q, want %q %q", tc.tool, srv, ok, mcpOwner(srv), tc.server, tc.owner)
		}
	}
	if _, ok := mcpServer("Bash"); ok {
		t.Error("Bash is not an MCP tool")
	}
}

func TestAnalyzeBaseline(t *testing.T) {
	root := t.TempDir()
	main := session1(t, root)
	rep, err := Analyze(Options{Session: main, ProjectsDir: filepath.Join(root, "projects"), Home: filepath.Join(root, "nohome"), Recent: 5})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Requests != 3 {
		t.Fatalf("requests = %d, want 3 (duplicate message ids count once)", rep.Requests)
	}
	if rep.FirstTokens != 1002 || rep.FirstWrite != 800 || rep.FirstRead != 200 {
		t.Errorf("first = %d (write %d read %d), want 1002", rep.FirstTokens, rep.FirstWrite, rep.FirstRead)
	}
	if !rep.HasSnapshot || !rep.KeptReminders || !rep.HasInstructions {
		t.Errorf("flags snapshot=%v kept=%v instructions=%v", rep.HasSnapshot, rep.KeptReminders, rep.HasInstructions)
	}
	bySrc := map[string]Source{}
	for _, s := range rep.Visible {
		bySrc[s.Name] = s
	}
	toolsJSON := func(v obj) int { raw, _ := json.Marshal(v); return chars(string(raw)) }
	want := map[string]int{
		"system prompt (prompt_snapshot)":               400,
		"tool schemas: built-in":                        toolsJSON(obj{"name": "Bash", "description": "run", "schema": obj{}}),
		"tool schemas: mcp plugin_pw_pw":                toolsJSON(obj{"name": "mcp__plugin_pw_pw__click", "description": "click", "schema": obj{"a": 1}}),
		"attachment skill_listing (skill descriptions)": chars("- plug:one: aaaa\n- plug:two: bbbb\n- plain: cc\n- unusedplain: dd"),
		"attachment deferred_tools_delta":               chars("mcp__srv__a\nmcp__srv__b\nWebFetch"),
		"attachment mcp_instructions_delta":             chars("## srv\nuse it"),
		"CLAUDE.md contents in the first user message":  chars("CLAUDE.md says hi"),
		"first user message":                            chars("hello"),
	}
	var sum float64
	for name, w := range want {
		got, ok := bySrc[name]
		if !ok || got.Chars != w {
			t.Errorf("source %q chars = %d (found %v), want %d", name, got.Chars, ok, w)
		}
		sum += float64(w) / 4
	}
	if _, ok := bySrc["attachment deferred_tools_record"]; ok {
		t.Error("bookkeeping attachment must not be a source")
	}
	approx(t, "visible tokens", rep.VisibleTokens, sum)
	approx(t, "residual", rep.Residual, 1002-sum)

	// owners
	owners := map[string]*Owner{}
	for _, o := range rep.Owners {
		owners[o.Name] = o
	}
	if o := owners["plug"]; o == nil || o.Skills != 2 || o.SkillChars != 2*(chars("- plug:one: aaaa")+1) || o.SkillUses != 1 {
		t.Errorf("owner plug = %+v", o)
	}
	if o := owners["pw"]; o == nil || o.MCPSchemaChars != want["tool schemas: mcp plugin_pw_pw"] || o.Uses() != 0 {
		t.Errorf("owner pw = %+v", o)
	}
	if o := owners["mcp:srv"]; o == nil || o.MCPNameChars != 2*(chars("mcp__srv__a")+1) || o.MCPInstrChars != chars("## srv\nuse it")+1 || o.MCPUses != 2 {
		t.Errorf("owner mcp:srv = %+v", o)
	}
	un := owners[Unprefixed]
	if un == nil || un.Skills != 2 || un.SkillUses != 1 || len(un.UnusedSkills) != 1 || un.UnusedSkills[0].Name != "unusedplain" || un.AgentUses != 1 {
		t.Errorf("unprefixed owner = %+v", un)
	}
	// usage: Skill t1 + plain from the subagent file; Agent t3 counted once despite two lines
	if rep.Usage.Skill["plug:one"] != 1 || rep.Usage.Skill["plain"] != 1 || rep.Usage.Agent["Explore"] != 1 || rep.Usage.MCP["srv"] != 2 {
		t.Errorf("usage = %+v", rep.Usage)
	}
	if rep.Sub.Transcripts != 1 || rep.Sub.Requests != 2 || rep.Sub.FirstMin != 400 {
		t.Errorf("sub = %+v", rep.Sub)
	}
	// suggestions: pw (never used) is suggested, plug (used) is not; $ uses the cache-read price (0.20/MTok)
	var found bool
	for _, s := range rep.Suggestions {
		if strings.Contains(s.Text, "plugin pw ") {
			found = true
			tokens := float64(want["tool schemas: mcp plugin_pw_pw"]) / 4
			approx(t, "tokens per request", s.TokPerReq, tokens)
			approx(t, "saved", s.Saved, tokens*3*0.20/1e6)
			approx(t, "write once", s.WriteOnce, tokens*4/1e6) // 1h cache write = 2 x $2/MTok
		}
		if strings.Contains(s.Text, "plugin plug ") {
			t.Error("a used plugin must not be suggested for removal")
		}
	}
	if !found {
		t.Errorf("no suggestion for the unused plugin pw: %+v", rep.Suggestions)
	}
	for i := 1; i < len(rep.Suggestions); i++ {
		if rep.Suggestions[i].TokRequests > rep.Suggestions[i-1].TokRequests {
			t.Error("suggestions not ordered by tokens x requests")
		}
	}
	var out bytes.Buffer
	rep.Write(&out)
	for _, s := range []string{"== 1. First request ==", "1,002 input tokens", "== 2.", "== 3. Residual", "== 4.", "== 5.", "== 6.", "== 7.", "not used in the last 1 sessions scanned", "only 1 session(s) found"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("report lacks %q:\n%s", s, out.String())
		}
	}
}

func TestRepeatedReminders(t *testing.T) {
	root := t.TempDir()
	same := att("total_tokens_reminder", obj{"text": "<total_tokens>9 left</total_tokens>"})
	other := att("total_tokens_reminder", obj{"text": "<total_tokens>8 left</total_tokens>"})
	listing := att("skill_listing", obj{"content": "- a: " + strings.Repeat("x", 95)}) // 100 chars
	tests := []struct {
		name                         string
		rows                         []any
		typ                          string
		times, base, later, distinct int
		extra                        float64
	}{
		{"same reminder each turn", []any{
			user("hi"), asst("r1", 1, 0, 0), same, asst("r2", 1, 0, 0), same, asst("r3", 1, 0, 0)},
			"total_tokens_reminder", 2, 0, 2, 1, 35.0 / 4 * (2 + 1)},
		{"changing reminder", []any{
			user("hi"), asst("r1", 1, 0, 0), same, asst("r2", 1, 0, 0), other, asst("r3", 1, 0, 0)},
			"total_tokens_reminder", 2, 0, 2, 2, 35.0 / 4 * (2 + 1)},
		{"baseline copy then re-injection", []any{
			user("hi"), listing, asst("r1", 1, 0, 0), listing, asst("r2", 1, 0, 0), asst("r3", 1, 0, 0)},
			"skill_listing", 2, 1, 1, 1, 100.0 / 4 * 2},
		{"reminder blocks inside user text", []any{
			user("hi"), asst("r1", 1, 0, 0),
			user([]any{obj{"type": "text", "text": "<system-reminder>be brief</system-reminder>go"}}), asst("r2", 1, 0, 0),
			user([]any{obj{"type": "text", "text": "<system-reminder>be brief</system-reminder>again"}}), asst("r3", 1, 0, 0)},
			"user-text reminder: be brief", 2, 0, 2, 1, float64(chars("be brief")) / 4 * (2 + 1)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeJSONL(t, filepath.Join(root, tc.name, "projects", "p", "s.jsonl"), tc.rows...)
			rep, err := Analyze(Options{Session: p, ProjectsDir: filepath.Join(root, tc.name, "projects"), Home: filepath.Join(root, "nohome")})
			if err != nil {
				t.Fatal(err)
			}
			var got *Repeat
			for i := range rep.Repeats {
				if rep.Repeats[i].Type == tc.typ {
					got = &rep.Repeats[i]
				}
			}
			if got == nil {
				t.Fatalf("no repeat for %q: %+v", tc.typ, rep.Repeats)
			}
			if got.Occurrences != tc.times || got.InBaseline != tc.base || got.Later != tc.later || got.Distinct != tc.distinct {
				t.Errorf("repeat = %+v", *got)
			}
			approx(t, "extra token-requests", got.ExtraTokReq, tc.extra)
		})
	}
}

func TestEmptyAndMinimalSessions(t *testing.T) {
	root := t.TempDir()
	tests := []struct {
		name string
		rows []any
	}{
		{"empty file", nil},
		{"only a user line", []any{user("hello")}},
		{"attachments but no request", []any{att("model", obj{"text": "You are X"}), user("hello")}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := writeJSONL(t, filepath.Join(root, tc.name, "projects", "p", "s.jsonl"), tc.rows...)
			rep, err := Analyze(Options{Session: p, ProjectsDir: filepath.Join(root, tc.name, "projects"), Home: filepath.Join(root, "nohome")})
			if err != nil {
				t.Fatal(err)
			}
			if !rep.NoRequest || rep.FirstTokens != 0 || rep.Requests != 0 {
				t.Errorf("report = %+v", rep)
			}
			var out bytes.Buffer
			rep.Write(&out) // must not panic or divide by zero
			if !strings.Contains(out.String(), "no API request in this session yet") || strings.Contains(out.String(), "NaN") || strings.Contains(out.String(), "Inf") {
				t.Errorf("bad report:\n%s", out.String())
			}
		})
	}
	if _, err := Analyze(Options{Session: filepath.Join(root, "missing.jsonl")}); err == nil {
		t.Error("a missing transcript must be an error")
	}
}

func TestNoSnapshotMeansResidualHoldsSystemPrompt(t *testing.T) {
	root := t.TempDir()
	p := writeJSONL(t, filepath.Join(root, "projects", "p", "s.jsonl"),
		user("hi"), att("skill_listing", obj{"content": "- a: b"}), asst("r1", 1, 5000, 0))
	home := filepath.Join(root, "home")
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte(strings.Repeat("c", 4000)), 0o644)
	rep, err := Analyze(Options{Session: p, ProjectsDir: filepath.Join(root, "projects"), Home: home})
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	rep.Write(&out)
	if rep.HasSnapshot || rep.Residual < 4900 {
		t.Errorf("snapshot=%v residual=%.0f", rep.HasSnapshot, rep.Residual)
	}
	for _, s := range []string{"NOT visible", "local CLAUDE.md files (~1,000 tokens"} {
		if !strings.Contains(out.String(), s) {
			t.Errorf("report lacks %q:\n%s", s, out.String())
		}
	}
}

func TestReadConfig(t *testing.T) {
	t.Run("missing everything", func(t *testing.T) {
		c := ReadConfig(filepath.Join(t.TempDir(), "nope"), "")
		if c.UserClaude.Exists || c.ProjectClaude.Exists || c.SkillFiles != 0 || c.SettingsFound || len(c.PluginEntries) != 0 {
			t.Errorf("config = %+v", c)
		}
	})
	t.Run("populated", func(t *testing.T) {
		home, proj := t.TempDir(), t.TempDir()
		cl := filepath.Join(home, ".claude")
		os.MkdirAll(filepath.Join(cl, "skills", "a"), 0o755)
		os.MkdirAll(filepath.Join(cl, "plugins", "synced", "x"), 0o755)
		os.WriteFile(filepath.Join(cl, "CLAUDE.md"), []byte(strings.Repeat("u", 400)), 0o644)
		os.WriteFile(filepath.Join(proj, "CLAUDE.md"), []byte(strings.Repeat("p", 800)), 0o644)
		os.WriteFile(filepath.Join(cl, "skills", "a", "SKILL.md"), []byte("12345"), 0o644)
		os.WriteFile(filepath.Join(cl, "skills", "a", "ref.md"), []byte("123"), 0o644)
		os.WriteFile(filepath.Join(cl, "settings.json"), []byte(`{"enabledPlugins":{"b@m":true,"a@m":true,"off@m":false}}`), 0o644)
		c := ReadConfig(home, proj)
		approx(t, "user tokens", c.UserClaude.Tokens, 100)
		approx(t, "project tokens", c.ProjectClaude.Tokens, 200)
		if c.SkillFiles != 2 || c.SkillBytes != 8 {
			t.Errorf("skills = %d files %d bytes", c.SkillFiles, c.SkillBytes)
		}
		if strings.Join(c.EnabledPlugins, ",") != "a@m,b@m" || strings.Join(c.DisabledPlugins, ",") != "off@m" {
			t.Errorf("plugins = %v / %v", c.EnabledPlugins, c.DisabledPlugins)
		}
		if strings.Join(c.PluginEntries, ",") != "synced/,synced/x" {
			t.Errorf("plugin entries = %v", c.PluginEntries)
		}
	})
	t.Run("broken settings and installed_plugins", func(t *testing.T) {
		home := t.TempDir()
		cl := filepath.Join(home, ".claude")
		os.MkdirAll(filepath.Join(cl, "plugins"), 0o755)
		os.WriteFile(filepath.Join(cl, "settings.json"), []byte(`{not json`), 0o644)
		os.WriteFile(filepath.Join(cl, "plugins", "installed_plugins.json"), []byte(`{"plugins":{"p@m":[]}}`), 0o644)
		c := ReadConfig(home, "")
		if c.SettingsFound || strings.Join(c.PluginEntries, ",") != "p@m" {
			t.Errorf("config = %+v", c)
		}
	})
}

func TestCollectUsageSessionsAndLimit(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "projects")
	for i, name := range []string{"a", "b", "c"} {
		writeJSONL(t, filepath.Join(proj, "p", name+".jsonl"),
			asst("m"+name, 1, 0, 0, toolUse("id"+name, "Skill", obj{"skill": "s" + name})),
			user("<command-name>/slash"+name+"</command-name>"))
		mt := int64(1_700_000_000 + i*100)
		os.Chtimes(filepath.Join(proj, "p", name+".jsonl"), timeAt(mt), timeAt(mt))
	}
	u := CollectUsage(proj, 2, "")
	if u.Sessions != 2 || u.Skill["sc"] != 1 || u.Skill["sb"] != 1 || u.Skill["sa"] != 0 || u.Skill["slashc"] != 1 {
		t.Errorf("recent=2 usage = %+v", u)
	}
	// the analysed session is always included, even if older than the window
	u = CollectUsage(proj, 1, filepath.Join(proj, "p", "a.jsonl"))
	if u.Sessions != 2 || u.Skill["sa"] != 1 || u.Skill["sc"] != 1 {
		t.Errorf("include usage = %+v", u)
	}
	if u := CollectUsage(filepath.Join(root, "none"), 5, ""); u.Sessions != 0 {
		t.Errorf("missing dir usage = %+v", u)
	}
}

func timeAt(sec int64) time.Time { return time.Unix(sec, 0) }
