package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// pickerWriteSession writes ~/.claude/projects-like <root>/<project>/<id>.jsonl with a first prompt and a reply,
// `age` old.
func pickerWriteSession(t *testing.T, root, project, id, cwd, prompt, reply string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(root, project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+".jsonl")
	var out []byte
	for _, l := range []map[string]any{
		{"type": "user", "cwd": cwd, "timestamp": "2026-10-09T10:00:00Z", "message": map[string]any{"content": prompt}},
		{"type": "assistant", "timestamp": "2026-10-09T10:00:05Z", "message": map[string]any{"id": "m-" + id, "model": "claude-sonnet-5-5",
			"content": []any{map[string]any{"type": "text", "text": reply}}}},
	} {
		b, _ := json.Marshal(l)
		out = append(append(out, b...), '\n')
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
	mt := time.Now().Add(-age)
	_ = os.Chtimes(p, mt, mt)
	return p
}
