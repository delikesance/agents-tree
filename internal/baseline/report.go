package baseline

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

func (r *Report) usd(tokens float64) (saved, write float64) {
	if !r.PriceKnown {
		return 0, 0
	}
	return tokens * float64(r.Requests) * r.ReadPrice / 1e6, tokens * r.WritePrice / 1e6
}

func (r *Report) money(tokens float64) string {
	if !r.PriceKnown {
		return "$? (model price unknown)"
	}
	s, w := r.usd(tokens)
	return fmt.Sprintf("~$%.2f over this session (+ one-off cache write ~$%.3f)", s, w)
}

func suggest(r *Report) []Suggestion {
	var out []Suggestion
	push := func(text string, tokPerReq float64) {
		s, w := r.usd(tokPerReq)
		out = append(out, Suggestion{Text: text, TokPerReq: tokPerReq,
			TokRequests: tokPerReq * float64(r.Requests), Saved: s, WriteOnce: w})
	}
	n := r.Recent
	for _, o := range r.Owners {
		if o.Name == Unprefixed {
			continue
		}
		if o.Uses() > 0 {
			continue
		}
		r.UnusedTokensTotal += o.Tokens()
		what := "the plugin " + o.Name
		if strings.HasPrefix(o.Name, "mcp:") {
			what = "the MCP server " + strings.TrimPrefix(o.Name, "mcp:")
		}
		push(fmt.Sprintf("If you do not use %s (not used in the last %d sessions: no Skill, MCP or Agent call), disabling it would remove ~%s tokens per request (%s) = %s.",
			what, n, num(o.Tokens()), parts(o), r.money(o.Tokens())), o.Tokens())
	}
	for _, o := range r.Owners {
		if o.Name != Unprefixed || len(o.UnusedSkills) == 0 {
			continue
		}
		var ch int
		var names []string
		for i, s := range o.UnusedSkills {
			ch += s.Chars
			if i < 5 {
				names = append(names, s.Name)
			}
		}
		push(fmt.Sprintf("%d of the %d unprefixed skills listed were not used in the last %d sessions (largest: %s). If they are your own skills (~/.claude/skills, .claude/skills), removing them would save ~%s tokens per request = %s. Built-in ones cannot be removed.",
			len(o.UnusedSkills), o.Skills, n, strings.Join(names, ", "), num(tok(ch)), r.money(tok(ch))), tok(ch))
	}
	for _, rp := range r.Repeats {
		if r.PromptSum <= 0 || rp.ExtraTokReq < 0.005*r.PromptSum {
			continue
		}
		s, _ := r.usd(rp.ExtraTokReq / float64(max(r.Requests, 1)))
		out = append(out, Suggestion{
			Text: fmt.Sprintf("'%s' was injected %d more times after the first request (~%s token-requests, %.1f%% of all input tokens, ~$%.2f at the cache-read price). It cannot be switched off from here; if one of your hooks or CLAUDE.md rules produces it, make it shorter or less frequent.",
				rp.Type, rp.Later, num(rp.ExtraTokReq), 100*rp.ExtraTokReq/r.PromptSum, s),
			TokPerReq: rp.ExtraTokReq / float64(max(r.Requests, 1)), TokRequests: rp.ExtraTokReq, Saved: s})
	}
	cl := r.Config.UserClaude.Tokens + r.Config.ProjectClaude.Tokens
	if cl >= 1500 {
		push(fmt.Sprintf("Your CLAUDE.md files weigh ~%s tokens and travel with every request: if parts are rarely needed, each 1,000 tokens moved out (e.g. to a skill or a docs file read on demand) would save %s.",
			num(cl), r.money(1000)), 1000)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TokRequests > out[j].TokRequests })
	return out
}

func parts(o *Owner) string {
	var p []string
	if o.Skills > 0 {
		p = append(p, fmt.Sprintf("%d skills ~%s", o.Skills, num(tok(o.SkillChars))))
	}
	if o.MCPSchemaChars+o.MCPNameChars+o.MCPInstrChars > 0 {
		p = append(p, fmt.Sprintf("MCP ~%s", num(tok(o.MCPSchemaChars+o.MCPNameChars+o.MCPInstrChars))))
	}
	if o.Agents > 0 {
		p = append(p, fmt.Sprintf("%d agents ~%s", o.Agents, num(tok(o.AgentChars))))
	}
	return strings.Join(p, ", ")
}

func num(f float64) string {
	i := int(f + 0.5)
	s := fmt.Sprintf("%d", i)
	for k := len(s) - 3; k > 0; k -= 3 {
		s = s[:k] + "," + s[k:]
	}
	return s
}

func pct(a, b float64) string {
	if b <= 0 {
		return "   -"
	}
	return fmt.Sprintf("%3.0f%%", 100*a/b)
}

func trunc(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n-1]) + "…"
}

// Write renders the text report.
func (r *Report) Write(w io.Writer) {
	p := func(f string, a ...any) { fmt.Fprintf(w, f, a...) }
	p("Baseline context report (read-only; tokens are estimates = characters / 4)\n")
	p("session: %s\n", r.Session)
	if r.Cwd != "" {
		p("project: %s\n", r.Cwd)
	}
	p("\n== 1. First request ==\n")
	if r.NoRequest {
		p("no API request in this session yet: only the recorded context can be listed below.\n")
	} else {
		p("model %s, %d requests in the main session\n", r.Model, r.Requests)
		p("first request: %s input tokens (fresh %s + cache write %s + cache read %s), from message.usage\n",
			num(float64(r.FirstTokens)), num(float64(r.FirstFresh)), num(float64(r.FirstWrite)), num(float64(r.FirstRead)))
		if r.PromptSum > 0 {
			p("the same context is re-sent on every request: baseline = %s of %s input tokens of the session (%.0f%%)\n",
				num(float64(r.FirstTokens)*float64(r.Requests)), num(r.PromptSum), 100*float64(r.FirstTokens)*float64(r.Requests)/r.PromptSum)
		}
	}
	if r.Sub.Transcripts > 0 {
		p("subagents: %d transcripts, %d requests; first request ~%s tokens on average (min %s, max %s) - not included in the numbers below\n",
			r.Sub.Transcripts, r.Sub.Requests, num(r.Sub.FirstAvg), num(float64(r.Sub.FirstMin)), num(float64(r.Sub.FirstMax)))
	}

	p("\n== 2. What the transcript lets us see ==\n")
	if len(r.Visible) == 0 {
		p("nothing recorded before the first request.\n")
	} else {
		p("%-58s %9s %6s\n", "source", "tokens", "share")
		for _, s := range r.Visible {
			name := s.Name
			if s.N > 1 {
				name = fmt.Sprintf("%s [%d]", name, s.N)
			}
			p("%-58s %9s %6s\n", trunc(name, 58), num(s.Tokens), pct(s.Tokens, float64(r.FirstTokens)))
		}
		p("%-58s %9s %6s\n", "visible total (estimate)", num(r.VisibleTokens), pct(r.VisibleTokens, float64(r.FirstTokens)))
	}
	p("\n== 3. Residual (estimate) ==\n")
	if r.FirstTokens > 0 {
		p("first request %s - visible %s = %s tokens (%s of the request)\n",
			num(float64(r.FirstTokens)), num(r.VisibleTokens), num(r.Residual), strings.TrimSpace(pct(r.Residual, float64(r.FirstTokens))))
		switch {
		case r.HasSnapshot:
			p("system prompt and tool schemas are recorded in this transcript (prompt_snapshot), so the residual is mostly tokenizer error of chars/4,\nformatting the API adds around tools, and context that is not recorded.\n")
		default:
			p("this transcript has no prompt_snapshot: the system prompt and tool schemas are NOT visible, so the residual is mostly them (not attributable here).\n")
		}
		if !r.HasInstructions && (r.Config.UserClaude.Exists || r.Config.ProjectClaude.Exists) {
			p("no CLAUDE.md contents were recorded before the first request; your local CLAUDE.md files (~%s tokens, see section 6) are probably part of the residual.\n",
				num(r.Config.UserClaude.Tokens+r.Config.ProjectClaude.Tokens))
		}
	} else {
		p("not computable (no first request).\n")
	}
	if len(r.TopTools) > 0 {
		p("\nlargest tool schemas in the first request:\n")
		for _, t := range r.TopTools {
			p("  %-52s %9s tokens\n", trunc(t.Name, 52), num(t.Tokens))
		}
	}

	p("\n== 4. Reminders injected again after the first request ==\n")
	if len(r.Repeats) == 0 {
		p("none found.\n")
	} else {
		if r.KeptReminders {
			p("(prompt_snapshot says keptReminders=true: injected reminders stay in the history and are re-sent on later requests.)\n")
		} else {
			p("(cannot tell from this transcript whether each injection is re-sent on later requests; the volume assumes it is, as history.)\n")
		}
		p("%-44s %6s %6s %6s %8s %12s %7s\n", "kind", "times", "base", "later", "distinct", "extra tok-req", "$")
		for i, rp := range r.Repeats {
			if i == 10 {
				break
			}
			usd := ""
			if r.PriceKnown && r.Requests > 0 {
				usd = fmt.Sprintf("%.2f", rp.ExtraTokReq*r.ReadPrice/1e6)
			}
			p("%-44s %6d %6d %6d %8d %12s %7s\n", trunc(rp.Type, 44), rp.Occurrences, rp.InBaseline, rp.Later, rp.Distinct, num(rp.ExtraTokReq), usd)
		}
		p("extra tok-req = size x number of later requests that carry it, summed over the injections after the first request.\n")
	}

	p("\n== 5. Plugins, skills and MCP servers: size in the baseline vs. use in the last %d sessions (%d scanned) ==\n", r.Recent, r.Usage.Sessions)
	if len(r.Owners) == 0 {
		p("no skill/agent/MCP listing recorded before the first request.\n")
	} else {
		p("%-30s %8s %-28s %6s %6s %6s\n", "owner", "tokens", "content", "skill", "mcp", "agent")
		for i, o := range r.Owners {
			if i == 15 {
				p("  ... %d more\n", len(r.Owners)-15)
				break
			}
			p("%-30s %8s %-28s %6d %6d %6d\n", trunc(o.Name, 30), num(o.Tokens()), trunc(parts(o), 28), o.SkillUses, o.MCPUses, o.AgentUses)
		}
		p("(skill/mcp/agent = calls counted over the scanned sessions incl. subagent transcripts; skills typed as /name count too.)\n")
		var unused, most []string
		for _, o := range r.Owners {
			if o.Name != Unprefixed && o.Uses() == 0 {
				unused = append(unused, fmt.Sprintf("%s (~%s)", o.Name, num(o.Tokens())))
			}
		}
		used := append([]*Owner(nil), r.Owners...)
		sort.SliceStable(used, func(i, j int) bool { return used[i].Uses() > used[j].Uses() })
		for _, o := range used {
			if o.Uses() == 0 || len(most) == 5 {
				break
			}
			most = append(most, fmt.Sprintf("%s (%d)", o.Name, o.Uses()))
		}
		p("\ncost tokens in the baseline but not used in the last %d sessions (candidates to disable, if you do not need them):\n  %s\n", r.Recent, orNone(unused))
		p("most used: %s\n", orNone(most))
	}
	if len(r.Usage.Skill)+len(r.Usage.MCP)+len(r.Usage.Agent) > 0 {
		p("calls seen: skills %s | mcp %s | agents %s\n", top(r.Usage.Skill), top(r.Usage.MCP), top(r.Usage.Agent))
	}

	p("\n== 6. Local configuration (cross-check) ==\n")
	c := r.Config
	fl := func(label string, f FileSize) {
		if f.Exists {
			p("%-22s %s bytes ~%s tokens  %s\n", label, num(float64(f.Bytes)), num(f.Tokens), f.Path)
		} else if f.Path != "" {
			p("%-22s not found  %s\n", label, f.Path)
		}
	}
	fl("~/.claude/CLAUDE.md", c.UserClaude)
	fl("./CLAUDE.md", c.ProjectClaude)
	p("%-22s %d files, %s bytes under %s\n", "~/.claude/skills", c.SkillFiles, num(float64(c.SkillBytes)), c.SkillsDir)
	if c.SettingsFound {
		p("%-22s enabled: %s; disabled: %s\n", "enabledPlugins", orNone(c.EnabledPlugins), orNone(c.DisabledPlugins))
	} else {
		p("%-22s settings.json not found or without enabledPlugins\n", "enabledPlugins")
	}
	p("%-22s %s\n", "~/.claude/plugins", orNone(c.PluginEntries))

	p("\n== 7. Suggestions (ordered by tokens x requests; conditional, nothing was changed) ==\n")
	if len(r.Suggestions) == 0 {
		p("none: nothing sizeable that went unused in the scanned sessions.\n")
	}
	for i, s := range r.Suggestions {
		if i == 8 {
			break
		}
		p("%d. %s\n", i+1, s.Text)
	}
	if r.UnusedTokensTotal > 0 {
		p("\ntotal of the unused groups above: ~%s tokens per request = %s.\n", num(r.UnusedTokensTotal), r.money(r.UnusedTokensTotal))
	}
	p("The saving counts cache-read cost for the requests of this session; the first request of a new session pays the cache write once. Estimates only; the main session only (subagent baselines come on top).\n")
}

func orNone(s []string) string {
	if len(s) == 0 {
		return "(none)"
	}
	return strings.Join(s, ", ")
}

func top(m map[string]int) string {
	type kv struct {
		k string
		v int
	}
	var l []kv
	for k, v := range m {
		l = append(l, kv{k, v})
	}
	if len(l) == 0 {
		return "-"
	}
	sort.Slice(l, func(i, j int) bool {
		if l[i].v != l[j].v {
			return l[i].v > l[j].v
		}
		return l[i].k < l[j].k
	})
	var out []string
	for i, e := range l {
		if i == 6 {
			out = append(out, fmt.Sprintf("+%d more", len(l)-6))
			break
		}
		out = append(out, fmt.Sprintf("%s=%d", trunc(e.k, 28), e.v))
	}
	return strings.Join(out, " ")
}
