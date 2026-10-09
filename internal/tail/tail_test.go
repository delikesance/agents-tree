package tail

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/delikesance/agents-tree/internal/model"
)

func TestTailIsIncrementalAndWaitsForPartialLines(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s.jsonl")
	line := `{"type":"assistant","timestamp":"2026-10-09T10:00:00Z","message":{"id":"m","model":"claude-sonnet-5-5","usage":{"output_tokens":1},"content":[]}}`
	_ = os.WriteFile(p, []byte(line+"\n"+line[:20]), 0o644)
	tt := NewTranscriptTailer(p)
	if n := len(tt.Poll()); n != 1 {
		t.Fatalf("first poll = %d", n)
	}
	if n := len(tt.Poll()); n != 0 {
		t.Fatalf("second poll = %d", n)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(line[20:] + "\n")
	f.Close()
	if n := len(tt.Poll()); n != 1 {
		t.Fatalf("after completing the partial line = %d", n)
	}
}

func TestHookTailerFiltersBySession(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ev.jsonl")
	var out []byte
	for _, e := range []HookEvent{
		{TS: 1, Kind: "agent_start", AgentID: "x", Session: "S1", AgentKind: "worker"},
		{TS: 2, Kind: "agent_start", AgentID: "y", Session: "S2", AgentKind: "worker"},
		{TS: 3, Kind: "agent_start", AgentID: "z", AgentKind: "worker"},
	} {
		b, _ := json.Marshal(e)
		out = append(append(out, b...), '\n')
	}
	_ = os.WriteFile(p, out, 0o644)
	got := NewHookTailer(p, "S1").Poll()
	if len(got) != 2 || got[0].AgentID != "x" || got[1].AgentID != "z" || got[0].MatchKind != "worker" {
		t.Errorf("got %+v", got)
	}
	var _ = model.Main
}
