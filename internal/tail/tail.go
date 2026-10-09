// Package tail reads transcripts and the hook event file incrementally.
package tail

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"sort"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/transcript"
)

// Tail returns the complete new lines of a growing file (a partial last line waits for its end).
type Tail struct {
	Path   string
	offset int64
	buf    []byte
}

func (t *Tail) Lines() [][]byte {
	f, err := os.Open(t.Path)
	if err != nil {
		return nil
	}
	defer f.Close()
	if st, err := f.Stat(); err == nil && st.Size() < t.offset { // truncated or replaced
		t.offset, t.buf = 0, nil
	}
	if _, err := f.Seek(t.offset, io.SeekStart); err != nil {
		return nil
	}
	chunk, _ := io.ReadAll(f)
	t.offset += int64(len(chunk))
	t.buf = append(t.buf, chunk...)
	i := bytes.LastIndexByte(t.buf, '\n')
	if i < 0 {
		return nil
	}
	complete := t.buf[:i]
	t.buf = append([]byte(nil), t.buf[i+1:]...)
	var out [][]byte
	for _, l := range bytes.Split(complete, []byte("\n")) {
		if len(bytes.TrimSpace(l)) > 0 {
			out = append(out, l)
		}
	}
	return out
}

type entry struct {
	tail   *Tail
	parser *transcript.Parser
}

// TranscriptTailer polls the main transcript and its subagent transcripts for new events.
type TranscriptTailer struct {
	Session string
	tails   map[string]*entry
}

func NewTranscriptTailer(session string) *TranscriptTailer {
	return &TranscriptTailer{Session: session, tails: map[string]*entry{}}
}

func (t *TranscriptTailer) Poll() []model.Event {
	var events []model.Event
	for _, f := range transcript.SessionFiles(t.Session) {
		if _, ok := t.tails[f.Path]; !ok {
			t.tails[f.Path] = &entry{&Tail{Path: f.Path}, transcript.NewParser(f.AgentID, f.Sidechain)}
			if f.Sidechain {
				if a, ok := transcript.MetaAlias(f); ok {
					events = append(events, a)
				}
			}
		}
	}
	for _, e := range t.tails {
		for _, l := range e.tail.Lines() {
			var d map[string]any
			if json.Unmarshal(l, &d) == nil {
				events = append(events, e.parser.Parse(d)...)
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].TS < events[j].TS })
	return events
}

// HookTailer reads events appended by `agents-tree hook` (already normalized, one JSON per line).
type HookTailer struct {
	tail      *Tail
	SessionID string // keep only this session's events (events without one are kept)
}

func NewHookTailer(path, sessionID string) *HookTailer {
	return &HookTailer{tail: &Tail{Path: path}, SessionID: sessionID}
}

// HookEvent is the on-disk form of a hook event.
type HookEvent struct {
	TS        float64 `json:"ts"`
	Kind      string  `json:"kind"`
	AgentID   string  `json:"agent_id"`
	Session   string  `json:"session,omitempty"`
	AgentKind string  `json:"agent_kind,omitempty"`
	Status    string  `json:"status,omitempty"`
}

func (h *HookTailer) Poll() []model.Event {
	var out []model.Event
	for _, l := range h.tail.Lines() {
		var d HookEvent
		if json.Unmarshal(l, &d) != nil || d.Kind == "" {
			continue
		}
		if h.SessionID != "" && d.Session != "" && d.Session != h.SessionID {
			continue
		}
		ev := model.Event{TS: d.TS, Kind: d.Kind, AgentID: d.AgentID}
		if d.AgentID == "" {
			ev.AgentID = model.Main
		}
		switch d.Kind {
		case model.AgentStart:
			ev.AgentKind, ev.MatchKind = d.AgentKind, d.AgentKind
		case model.AgentEnd:
			ev.Status = d.Status
		}
		out = append(out, ev)
	}
	return out
}
