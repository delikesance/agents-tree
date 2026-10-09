package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/delikesance/agents-tree/internal/compress"
	"github.com/delikesance/agents-tree/internal/hookcfg"
)

// compressPayload is the part of a Claude Code hook payload that `agents-tree compress` reads.
type compressPayload struct {
	HookEventName string          `json:"hook_event_name"`
	SessionID     string          `json:"session_id"`
	ToolName      string          `json:"tool_name"`
	ToolInput     json.RawMessage `json:"tool_input"`
	ToolResponse  json.RawMessage `json:"tool_response"`
	AgentID       string          `json:"agent_id"`
	AgentType     string          `json:"agent_type"`
}

const maxHookPayload = 128 << 20

// runCompress is a Claude Code PostToolUse hook: it replaces long tool output with a shorter
// output of the same shape (see internal/compress). It ALWAYS returns 0 and prints nothing
// unless it has a replacement, so a problem here can never disturb the session.
func runCompress(in io.Reader, out io.Writer) (code int) {
	defer func() { _ = recover(); code = 0 }()
	data, err := io.ReadAll(io.LimitReader(in, maxHookPayload))
	if err != nil {
		return 0
	}
	var p compressPayload
	if json.Unmarshal(data, &p) != nil || p.HookEventName != "PostToolUse" || len(p.ToolResponse) == 0 {
		return 0
	}
	if p.ToolName == "Bash" && rtkRewritten(p.ToolInput) {
		return 0 // RTK already shortened this output
	}
	res, st, changed := compress.Compress(p.ToolName, p.ToolResponse, compress.OptionsFromEnv())
	if !changed {
		return 0
	}
	reply, err := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName":     "PostToolUse",
		"updatedToolOutput": res,
	}})
	if err != nil {
		return 0
	}
	agent := p.AgentType
	if agent == "" {
		agent = p.AgentID
	}
	// sizes and the tool name only: never the content or the command
	_ = compress.AppendLog("", compress.LogEntry{Session: p.SessionID, Tool: p.ToolName, Agent: agent,
		BytesIn: st.BytesIn, BytesOut: st.BytesOut, SavedPath: st.SavedPath})
	fmt.Fprintln(out, string(reply))
	return 0
}

func rtkRewritten(toolInput json.RawMessage) bool {
	var ti struct {
		Command string `json:"command"`
	}
	if json.Unmarshal(toolInput, &ti) != nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(ti.Command), "rtk ")
}

const hooksUsage = `usage: agents-tree hooks install|uninstall|status [--settings PATH] [--apply]

  install     add the agents-tree hooks to a Claude Code settings.json
  uninstall   remove only the agents-tree hooks
  status      show which hooks are installed and whether RTK (Bash) is set up

  --settings PATH   settings file (default ~/.claude/settings.json)
  --apply           write the change (default: dry run, only print what would change);
                    the original is first copied to <settings>.bak-<timestamp>
`

// test seams
var (
	lookPath = osexec.LookPath
	hooksNow = time.Now
)

func runHooks(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(stderr, hooksUsage)
		return 2
	}
	sub := args[0]
	if sub != "install" && sub != "uninstall" && sub != "status" {
		fmt.Fprintf(stderr, "unknown hooks command %q\n\n%s", sub, hooksUsage)
		return 2
	}
	var settings string
	var apply bool
	fs := flag.NewFlagSet("hooks "+sub, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&settings, "settings", "", "")
	fs.BoolVar(&apply, "apply", false, "")
	if err := fs.Parse(args[1:]); err != nil || fs.NArg() > 0 {
		fmt.Fprint(stderr, hooksUsage)
		return 2
	}
	if settings == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(stderr, "cannot find the home directory; pass --settings PATH")
			return 1
		}
		settings = filepath.Join(home, ".claude", "settings.json")
	}
	data, exists, err := hookcfg.Read(settings)
	if err != nil {
		fmt.Fprintf(stderr, "cannot read %s: %v\n", settings, err)
		return 1
	}
	if sub == "status" {
		return hooksStatus(settings, data, exists, stdout, stderr)
	}

	var res hookcfg.Result
	if sub == "install" {
		res, err = hookcfg.Install(data)
	} else {
		res, err = hookcfg.Uninstall(data)
	}
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\nnothing was changed\n", settings, err)
		return 1
	}
	if !res.Changed {
		if sub == "install" {
			fmt.Fprintf(stdout, "%s: already up to date, nothing to do\n", settings)
		} else {
			fmt.Fprintf(stdout, "%s: no agents-tree hooks found, nothing to do\n", settings)
		}
		return 0
	}
	if !apply {
		old := string(data)
		oldName := settings
		if !exists {
			oldName = "/dev/null"
		}
		fmt.Fprintf(stdout, "agents-tree hooks %s: dry run for %s\n\n", sub, settings)
		fmt.Fprintln(stdout, hookcfg.Diff(oldName, settings+" (proposed)", old, res.Out))
		fmt.Fprintf(stdout, "Exact JSON that would be written:\n%s\n", res.Out)
		fmt.Fprintln(stdout, "Dry run: nothing was written. Re-run with --apply to write it.")
		return 0
	}
	backup, err := hookcfg.Apply(settings, res.Out, hooksNow())
	if err != nil {
		fmt.Fprintf(stderr, "write failed: %v\n", err)
		return 1
	}
	if backup != "" {
		fmt.Fprintf(stdout, "backup: %s\n", backup)
	}
	fmt.Fprintf(stdout, "updated: %s\n", settings)
	if sub == "install" {
		fmt.Fprintln(stdout, "Restart Claude Code (hooks are read at startup). Run `agents-tree hooks status` to check, and for the RTK part (Bash).")
	}
	return 0
}

const rtkHint = `RTK (compresses Bash output; the other tools are covered by agents-tree compress) is not installed. Install one of:
  brew install rtk
  cargo install --git https://github.com/rtk-ai/rtk
  or the install script from https://github.com/rtk-ai/rtk
then run: rtk init -g --auto-patch
and restart Claude Code. RTK only handles Bash and is lossy (on failure the full output is kept: rtk recall).
Check what it saves with: rtk gain`

func hooksStatus(settings string, data []byte, exists bool, stdout, stderr io.Writer) int {
	rep, err := hookcfg.Status(data)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", settings, err)
		return 1
	}
	state := "exists"
	if !exists {
		state = "missing"
	}
	fmt.Fprintf(stdout, "settings: %s (%s)\n", settings, state)
	for _, e := range rep.Entries {
		mark := "MISSING  "
		if e.Installed {
			mark = "installed"
		}
		matcher := e.Matcher
		if matcher == "" {
			matcher = "-"
		}
		fmt.Fprintf(stdout, "  %s  %-13s %-45s %s\n", mark, e.Event, matcher, e.Command())
	}
	rtkPath, lerr := lookPath("rtk")
	if lerr == nil {
		fmt.Fprintf(stdout, "  rtk on PATH: yes (%s)\n", rtkPath)
	} else {
		fmt.Fprintln(stdout, "  rtk on PATH: no")
	}
	if rep.RTKHook {
		fmt.Fprintln(stdout, "  RTK hook in settings: yes")
	} else {
		fmt.Fprintln(stdout, "  RTK hook in settings: no")
	}
	switch {
	case lerr != nil && rep.RTKHook:
		fmt.Fprintln(stdout, "\nwarning: settings call `rtk hook` but rtk is not on PATH.")
		fmt.Fprintln(stdout, rtkHint)
	case lerr != nil:
		fmt.Fprintln(stdout, "\n"+rtkHint)
	case !rep.RTKHook:
		fmt.Fprintln(stdout, "\nrtk is installed but its hook is not in this settings file. Run: rtk init -g --auto-patch (then restart Claude Code).")
	}
	return 0
}
