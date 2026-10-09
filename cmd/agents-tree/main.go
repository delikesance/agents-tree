// agents-tree shows Claude Code agents as a live chat + agent rail (Bubble Tea).
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/delikesance/agents-tree/internal/composition"
	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/sessions"
	"github.com/delikesance/agents-tree/internal/tail"
	"github.com/delikesance/agents-tree/internal/ui"
)

const usage = `agents-tree: chat with Claude Code, see its agents, and where tokens go

  agents-tree                         start a NEW session (same as: agents-tree live)
  agents-tree live [session.jsonl]    open that session instead
        -c, --continue                continue the latest session of this directory
        --pick                        choose a session (or a new one) in the app
        --permissions all|accept-edits|plan
                                      what Claude may do when you message it (default: all)
        --claude-bin PATH             claude executable used to send messages
        --no-send                     read-only: no message box
        --no-hooks                    ignore the hook event file
  agents-tree replay <session.jsonl> [--speed N]
  agents-tree sessions                list sessions
  agents-tree context [session.jsonl] [--recent N]
                                      where a session's input tokens and money go
  agents-tree baseline [session.jsonl] [--recent N]
                                      what the first request contains and what could be cut (read-only)
  agents-tree hook                    Claude Code hook: reads the payload on stdin, appends an event
  agents-tree compress                Claude Code PostToolUse hook: shrinks long tool outputs
  agents-tree hooks install|uninstall|status [--settings PATH] [--apply]
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return runLive(nil, stderr) // no arguments: a new session
	}
	if args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "live":
		return runLive(args[1:], stderr)
	case "replay":
		return runReplay(args[1:], stderr)
	case "sessions":
		return runSessions(stdout)
	case "context":
		return runContext(args[1:], stdout, stderr)
	case "hook":
		return runHook(os.Stdin, stderr)
	case "baseline":
		return runBaseline(args[1:], stdout, stderr)
	case "compress":
		return runCompress(os.Stdin, stdout)
	case "hooks":
		return runHooks(args[1:], stdout, stderr)
	case "render": // developer aid: one frame of a session as ANSI text
		return runRender(args[1:], stdout, stderr)
	}
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", args[0], usage)
	return 2
}

func parse(name string, args []string, setup func(*flag.FlagSet)) (*flag.FlagSet, bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	setup(fs)
	// allow the session path before or after flags
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && a != "-" {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && !isBool(fs, strings.TrimLeft(a, "-")) && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, a)
		}
	}
	if err := fs.Parse(append(flags, pos...)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return fs, false
	}
	return fs, true
}

func isBool(fs *flag.FlagSet, name string) bool {
	f := fs.Lookup(name)
	if f == nil {
		return false
	}
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

func runLive(args []string, stderr io.Writer) int {
	var pick, noSend, noHooks, cont bool
	var perms, bin string
	fs, ok := parse("live", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&pick, "pick", false, "")
		fs.BoolVar(&cont, "continue", false, "")
		fs.BoolVar(&cont, "c", false, "")
		fs.BoolVar(&noSend, "no-send", false, "")
		fs.BoolVar(&noHooks, "no-hooks", false, "")
		fs.StringVar(&perms, "permissions", "all", "")
		fs.StringVar(&bin, "claude-bin", "claude", "")
	})
	if !ok {
		return 2
	}
	switch perms {
	case "all", "accept-edits", "plan":
	default:
		fmt.Fprintf(stderr, "--permissions must be all, accept-edits or plan (got %q)\n", perms)
		return 2
	}
	path := fs.Arg(0) // no path: a NEW session (unless --continue or --pick)
	if path == "" && cont {
		cwd, _ := os.Getwd()
		path = sessions.Latest("", cwd)
		if path == "" {
			fmt.Fprintln(stderr, "no earlier session found for this directory; start a new one with `agents-tree`")
			return 1
		}
	}
	if path != "" {
		if st, err := os.Stat(path); err != nil || st.IsDir() {
			fmt.Fprintf(stderr, "session transcript not found: %s\n", path)
			return 1
		}
	}
	err := ui.Run(ui.Options{Session: path, Pick: pick, Hooks: !noHooks, Permissions: perms, ClaudeBin: bin, CanSend: !noSend})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runReplay(args []string, stderr io.Writer) int {
	var speed float64
	fs, ok := parse("replay", args, func(fs *flag.FlagSet) { fs.Float64Var(&speed, "speed", 4, "") })
	if !ok {
		return 2
	}
	path := fs.Arg(0)
	if path == "" {
		fmt.Fprintln(stderr, "replay needs a session .jsonl")
		return 2
	}
	if st, err := os.Stat(path); err != nil || st.IsDir() {
		fmt.Fprintf(stderr, "session transcript not found: %s\n", path)
		return 1
	}
	if err := ui.Run(ui.Options{Session: path, Replay: true, Speed: speed}); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func runSessions(stdout io.Writer) int {
	for _, s := range sessions.List("", 50) {
		fmt.Fprintf(stdout, "%9s  %-24s %s  %s\n", sessions.Ago(s.MTime), sessions.Label(s.Project, s.CWD), short(s.ID()), s.Title)
	}
	return 0
}

func short(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func runContext(args []string, stdout, stderr io.Writer) int {
	var recent int
	fs, ok := parse("context", args, func(fs *flag.FlagSet) { fs.IntVar(&recent, "recent", 0, "") })
	if !ok {
		return 2
	}
	if recent > 0 {
		fmt.Fprintf(stdout, "%-10s%-20s%9s%9s%9s%9s%9s%8s\n", "session", "project", "requests", "input $", "context", "tool in", "tool out", "other")
		for _, s := range sessions.List("", recent) {
			c := composition.Analyze(s.Path, true)
			if c.Turns == 0 {
				continue
			}
			fmt.Fprintf(stdout, "%-10s%-20.19s%9d%9.2f%9.0f%%%8.0f%%%8.0f%%%7.0f%%\n", short(s.ID()), sessions.Label(s.Project, s.CWD),
				c.Turns, c.InputCost, 100*c.Share(c.Baseline), 100*c.Share(c.InputsVolume()), 100*c.Share(c.OutputsVolume()), 100*c.Share(c.Other()))
		}
		return 0
	}
	path := fs.Arg(0)
	if path == "" {
		cwd, _ := os.Getwd()
		path = sessions.Latest("", cwd)
	}
	if st, err := os.Stat(path); path == "" || err != nil || st.IsDir() {
		fmt.Fprintln(stderr, "session transcript not found; pass a .jsonl path")
		return 1
	}
	c := composition.Analyze(path, true)
	if c.Turns == 0 {
		fmt.Fprintln(stdout, "no API requests in this session yet")
		return 0
	}
	fmt.Fprintf(stdout, "%s\n%d requests · %.1fM input tokens re-sent · input cost ~$%.2f (estimate)\n\n", path, c.Turns, c.PromptTokens/1e6, c.InputCost)
	for _, r := range c.Categories() {
		fmt.Fprintf(stdout, "%5.1f%%  ~$%7.2f  %s\n", 100*c.Share(r.Volume), c.USD(r.Volume), r.Label)
	}
	fmt.Fprintln(stdout, "\ntool outputs by tool:")
	type kv struct {
		k string
		v *composition.ToolVol
	}
	var tools []kv
	for k, v := range c.Outputs {
		tools = append(tools, kv{k, v})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].v.Volume > tools[j].v.Volume })
	for i, t := range tools {
		if i == 8 {
			break
		}
		fmt.Fprintf(stdout, "%5.1f%%  ~$%7.2f  %s (%d calls, ~%d tokens)\n", 100*c.Share(t.v.Volume), c.USD(t.v.Volume), t.k, t.v.N, int(t.v.Tokens))
	}
	share, usd := c.WhatIfCompressOutputs(0.7)
	fmt.Fprintf(stdout, "\nShrinking every tool output by 70%% would remove about %.1f%% of input tokens (~$%.2f).\n", 100*share, usd)
	return 0
}

// runHook is a Claude Code hook: it turns SubagentStart/SubagentStop payloads into events that the
// live view reads from $AGENTS_TREE_EVENTS (default ~/.claude/agents-tree-events.jsonl). It never
// prints and always succeeds, so it cannot disturb the session.
func runHook(in io.Reader, stderr io.Writer) int {
	var p struct {
		Event     string `json:"hook_event_name"`
		AgentID   string `json:"agent_id"`
		AgentType string `json:"agent_type"`
		SessionID string `json:"session_id"`
	}
	raw, _ := io.ReadAll(in)
	if json.Unmarshal(raw, &p) != nil || p.AgentID == "" {
		return 0
	}
	ev := tail.HookEvent{TS: float64(time.Now().UnixNano()) / 1e9, AgentID: p.AgentID, Session: p.SessionID}
	switch p.Event {
	case "SubagentStart":
		ev.Kind, ev.AgentKind = model.AgentStart, p.AgentType
		if ev.AgentKind == "" {
			ev.AgentKind = "agent"
		}
	case "SubagentStop":
		ev.Kind, ev.Status = model.AgentEnd, "done"
	default:
		return 0
	}
	path := os.Getenv("AGENTS_TREE_EVENTS")
	if path == "" {
		home, _ := os.UserHomeDir()
		path = filepath.Join(home, ".claude", "agents-tree-events.jsonl")
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0
	}
	defer f.Close()
	b, _ := json.Marshal(ev)
	_, _ = f.Write(append(b, '\n'))
	return 0
}

func runRender(args []string, stdout, stderr io.Writer) int {
	var w, h int
	var keys string
	fs, ok := parse("render", args, func(fs *flag.FlagSet) {
		fs.IntVar(&w, "width", 140, "")
		fs.IntVar(&h, "height", 40, "")
		fs.StringVar(&keys, "keys", "", "")
	})
	if !ok || fs.Arg(0) == "" {
		fmt.Fprintln(stderr, "render needs a session .jsonl (or - for a new session)")
		return 2
	}
	path, replay := fs.Arg(0), true
	if path == "-" {
		path, replay = "", false
	}
	var ks []string
	if keys != "" {
		ks = strings.Split(keys, ",")
	}
	out, err := ui.Snapshot(ui.Options{Session: path, Replay: replay, CanSend: true, Permissions: "all"}, w, h, ks...)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintln(stdout, out)
	return 0
}
