package ui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/delikesance/agents-tree/internal/sessions"
)

// picker is the session chooser: a filter box and a list.
type picker struct {
	all     []sessions.Info
	current string
	filter  textinput.Model
	cursor  int
	top     int
}

func newPicker(all []sessions.Info, current string) *picker {
	f := textinput.New()
	f.Placeholder = "filter by project, prompt or id…"
	f.Prompt = "› "
	f.Focus()
	return &picker{all: all, current: current, filter: f}
}

func (p *picker) visible() []sessions.Info {
	needle := strings.ToLower(strings.TrimSpace(p.filter.Value()))
	if needle == "" {
		return p.all
	}
	var out []sessions.Info
	for _, s := range p.all {
		hay := strings.ToLower(sessions.Label(s.Project, s.CWD) + " " + s.Title + " " + s.ID())
		if strings.Contains(hay, needle) {
			out = append(out, s)
		}
	}
	return out
}

type pickResult struct {
	path   string // chosen session; empty when cancelled
	closed bool
}

// update handles a key; closed is true when the picker should go away.
func (p *picker) update(msg tea.Msg) (cmd tea.Cmd, res pickResult) {
	if k, ok := msg.(tea.KeyPressMsg); ok {
		vis := p.visible()
		switch k.String() {
		case "esc":
			return nil, pickResult{closed: true}
		case "enter":
			if len(vis) > 0 {
				return nil, pickResult{path: vis[min(p.cursor, len(vis)-1)].Path, closed: true}
			}
			return nil, pickResult{}
		case "up", "ctrl+p":
			p.cursor = max(0, p.cursor-1)
			return nil, pickResult{}
		case "down", "ctrl+n":
			p.cursor = min(max(len(vis)-1, 0), p.cursor+1)
			return nil, pickResult{}
		case "pgup":
			p.cursor = max(0, p.cursor-10)
			return nil, pickResult{}
		case "pgdown":
			p.cursor = min(max(len(vis)-1, 0), p.cursor+10)
			return nil, pickResult{}
		}
	}
	before := p.filter.Value()
	p.filter, cmd = p.filter.Update(msg)
	if p.filter.Value() != before {
		p.cursor, p.top = 0, 0
	}
	return cmd, pickResult{}
}

func (p *picker) view(width, height int) string {
	w := min(width-4, 130)
	h := min(height-4, 30)
	inner := w - 4
	vis := p.visible()
	rows := h - 6
	if p.cursor < p.top {
		p.top = p.cursor
	}
	if p.cursor >= p.top+rows {
		p.top = p.cursor - rows + 1
	}
	var l []string
	l = append(l, fg(colDim).Render("type to filter · ↑↓ choose · enter open · esc cancel"), p.filter.View(), "")
	head := fmt.Sprintf("%-10s %-16s %-4s %s", "modified", "project", "sub", "first prompt")
	l = append(l, fg(colFaint).Bold(true).Render(clip(head, inner)))
	for i := p.top; i < min(len(vis), p.top+rows); i++ {
		s := vis[i]
		mark := "  "
		if s.Path == p.current {
			mark = "● "
		}
		title := s.Title
		if title == "" {
			title = "(no prompt)"
		}
		line := fmt.Sprintf("%-10s %-16s %-4d %s%s  %s", sessions.Ago(s.MTime), clip(sessions.Label(s.Project, s.CWD), 16),
			s.Subagents, mark, title, fg(colFaint).Render(s.ID()[:min(8, len(s.ID()))]))
		line = clip(line, inner)
		if i == p.cursor {
			line = lipgloss.NewStyle().Reverse(true).Render(padRight(line, inner))
		}
		l = append(l, line)
	}
	if len(vis) == 0 {
		l = append(l, fg(colFaint).Render("no session matches"))
	}
	body := strings.Join(l, "\n")
	return box(bold(colUser).Render("select a session"), body, w, colUser)
}
