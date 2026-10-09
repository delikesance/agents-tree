// Package sessions finds Claude Code session transcripts on disk.
package sessions

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ProjectsDir is ~/.claude/projects.
func ProjectsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// Info describes one session transcript.
type Info struct {
	Path      string
	Project   string // directory name under the projects dir
	MTime     float64
	Title     string // first real user prompt
	Subagents int
	CWD       string
}

// ID is the session id (the file name without extension).
func (i Info) ID() string { return strings.TrimSuffix(filepath.Base(i.Path), ".jsonl") }

var (
	reReminder = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	reTag      = regexp.MustCompile(`<[^>]+>`)
)

func cleanPrompt(s string) string {
	s = reReminder.ReplaceAllString(s, "")
	s = reTag.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// Scan reads the head of a transcript: the first user prompt (as a title) and the cwd.
func Scan(path string) (title, cwd string) {
	f, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, 1<<20)
	read := 0
	for read < 256_000 {
		line, err := br.ReadBytes('\n')
		read += len(line)
		var d map[string]any
		if json.Unmarshal(line, &d) == nil {
			if cwd == "" {
				cwd, _ = d["cwd"].(string)
			}
			if title == "" && d["type"] == "user" && d["isSidechain"] != true {
				title = firstPrompt(d)
			}
		}
		if (title != "" && cwd != "") || err != nil {
			break
		}
	}
	return truncate(title, 80), cwd
}

func firstPrompt(d map[string]any) string {
	msg, _ := d["message"].(map[string]any)
	switch c := msg["content"].(type) {
	case string:
		return cleanPrompt(c)
	case []any:
		for _, b := range c {
			if m, ok := b.(map[string]any); ok && m["type"] == "text" {
				if t := cleanPrompt(fmt.Sprint(m["text"])); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

// List returns every session, most recently modified first (limit <= 0: all).
func List(projectsDir string, limit int) []Info {
	if projectsDir == "" {
		projectsDir = ProjectsDir()
	}
	files, _ := filepath.Glob(filepath.Join(projectsDir, "*", "*.jsonl"))
	type fm struct {
		p string
		t float64
	}
	var all []fm
	for _, p := range files {
		if st, err := os.Stat(p); err == nil {
			all = append(all, fm{p, float64(st.ModTime().UnixNano()) / 1e9})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].t > all[j].t })
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	out := make([]Info, 0, len(all))
	for _, f := range all {
		subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(f.p, ".jsonl"), "subagents", "agent-*.jsonl"))
		title, cwd := Scan(f.p)
		out = append(out, Info{Path: f.p, Project: filepath.Base(filepath.Dir(f.p)), MTime: f.t,
			Title: title, Subagents: len(subs), CWD: cwd})
	}
	return out
}

// Latest is the most recently modified transcript, preferring the project of cwd.
func Latest(projectsDir, cwd string) string {
	if projectsDir == "" {
		projectsDir = ProjectsDir()
	}
	pick := func(pattern string) string {
		files, _ := filepath.Glob(pattern)
		best, bt := "", time.Time{}
		for _, p := range files {
			if st, err := os.Stat(p); err == nil && st.ModTime().After(bt) {
				best, bt = p, st.ModTime()
			}
		}
		return best
	}
	if cwd != "" {
		enc := strings.ReplaceAll(cwd, "/", "-")
		if p := pick(filepath.Join(projectsDir, enc, "*.jsonl")); p != "" {
			return p
		}
	}
	return pick(filepath.Join(projectsDir, "*", "*.jsonl"))
}

// FindTranscript locates a session's transcript by id.
func FindTranscript(id, projectsDir string) string {
	if projectsDir == "" {
		projectsDir = ProjectsDir()
	}
	m, _ := filepath.Glob(filepath.Join(projectsDir, "*", id+".jsonl"))
	if len(m) > 0 {
		return m[0]
	}
	return ""
}

// Label is a readable project name: the cwd's base, else a lossy decode of "-home-user-agents-tree".
func Label(project, cwd string) string {
	if cwd != "" {
		if b := filepath.Base(cwd); b != "." && b != "/" {
			return b
		}
		return cwd
	}
	var parts []string
	for _, p := range strings.Split(project, "-") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) > 1 {
		return strings.Join(parts[len(parts)-2:], "/")
	}
	return project
}

// Ago renders a timestamp as "5 min ago".
func Ago(ts float64) string {
	d := float64(time.Now().UnixNano())/1e9 - ts
	if d < 0 {
		d = 0
	}
	switch {
	case d >= 86400:
		return fmt.Sprintf("%dd ago", int(d/86400))
	case d >= 3600:
		return fmt.Sprintf("%dh ago", int(d/3600))
	case d >= 60:
		return fmt.Sprintf("%d min ago", int(d/60))
	}
	return "just now"
}
