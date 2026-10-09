package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/delikesance/agents-tree/internal/baseline"
	"github.com/delikesance/agents-tree/internal/sessions"
)

const baselineUsage = `usage: agents-tree baseline [session.jsonl] [--recent N]
  explains what the first request's context is made of and what could be cut (read-only)
        --recent N       sessions scanned for skill/MCP/agent usage (default 20)
        --projects DIR   transcripts directory (default ~/.claude/projects)
        --home DIR       directory containing .claude (default $HOME)
`

// runBaseline implements `agents-tree baseline`: the composition of the context that is already there
// at the first API request, per source, with usage-based suggestions. It never modifies any file.
func runBaseline(args []string, stdout, stderr io.Writer) int {
	opts := baseline.Options{Recent: baseline.DefaultRecent}
	var path string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, val, hasVal := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") {
			if path != "" {
				fmt.Fprint(stderr, baselineUsage)
				return 2
			}
			path = a
			continue
		}
		switch name {
		case "h", "help":
			fmt.Fprint(stderr, baselineUsage)
			return 2
		case "recent", "projects", "home":
			if !hasVal {
				if i+1 >= len(args) {
					fmt.Fprintf(stderr, "--%s needs a value\n\n%s", name, baselineUsage)
					return 2
				}
				i++
				val = args[i]
			}
			switch name {
			case "recent":
				n, err := strconv.Atoi(val)
				if err != nil || n <= 0 {
					fmt.Fprintf(stderr, "--recent needs a positive number, got %q\n", val)
					return 2
				}
				opts.Recent = n
			case "projects":
				opts.ProjectsDir = val
			case "home":
				opts.Home = val
			}
		default:
			fmt.Fprintf(stderr, "unknown flag %q\n\n%s", a, baselineUsage)
			return 2
		}
	}
	if path == "" {
		cwd, _ := os.Getwd()
		path = sessions.Latest(opts.ProjectsDir, cwd)
		opts.Cwd = cwd
	}
	if st, err := os.Stat(path); path == "" || err != nil || st.IsDir() {
		fmt.Fprintln(stderr, "session transcript not found; pass a .jsonl path")
		return 1
	}
	opts.Session = path
	rep, err := baseline.Analyze(opts)
	if err != nil {
		fmt.Fprintln(stderr, "cannot analyse session:", err)
		return 1
	}
	rep.Write(stdout)
	return 0
}
