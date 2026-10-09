package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeSession(t *testing.T, root, project, id string, lines []map[string]any, mtime time.Time) string {
	t.Helper()
	dir := filepath.Join(root, project)
	_ = os.MkdirAll(dir, 0o755)
	p := filepath.Join(dir, id+".jsonl")
	var out []byte
	for _, l := range lines {
		b, _ := json.Marshal(l)
		out = append(append(out, b...), '\n')
	}
	if err := os.WriteFile(p, out, 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(p, mtime, mtime)
	return p
}

func TestListSortedWithTitlesSubagentsAndCWD(t *testing.T) {
	root := t.TempDir()
	a := writeSession(t, root, "-home-user-proj-a", "aaaa1111", []map[string]any{
		{"type": "user", "cwd": "/home/user/proj-a", "message": map[string]any{"content": []any{
			map[string]any{"type": "text", "text": "<system-reminder>ctx</system-reminder>"},
			map[string]any{"type": "text", "text": "Build the <b>login</b> page"}}}}}, time.Unix(1000, 0))
	_ = os.MkdirAll(filepath.Join(a[:len(a)-6], "subagents"), 0o755)
	_ = os.WriteFile(filepath.Join(a[:len(a)-6], "subagents", "agent-x.jsonl"), []byte("{}\n"), 0o644)
	writeSession(t, root, "-home-user-proj-b", "bbbb2222", []map[string]any{
		{"type": "user", "message": map[string]any{"content": "Fix the typo"}}}, time.Unix(2000, 0))
	got := List(root, 0)
	if len(got) != 2 || got[0].ID() != "bbbb2222" || got[1].ID() != "aaaa1111" {
		t.Fatalf("order = %+v", got)
	}
	if got[0].Title != "Fix the typo" || got[1].Title != "Build the login page" {
		t.Errorf("titles = %q / %q", got[0].Title, got[1].Title)
	}
	if got[1].Subagents != 1 || got[0].Subagents != 0 || got[1].CWD != "/home/user/proj-a" {
		t.Errorf("a = %+v", got[1])
	}
	if Label(got[1].Project, got[1].CWD) != "proj-a" || Label("-home-user-agents-tree", "") != "agents/tree" {
		t.Error("labels")
	}
	if id := FindTranscript("aaaa1111", root); id != a {
		t.Errorf("find = %q", id)
	}
	if Latest(root, "") != got[0].Path {
		t.Error("latest")
	}
	if Latest(root, "/home/user/proj/a") != filepath.Join(root, "-home-user-proj-a", "aaaa1111.jsonl") {
		t.Error("latest should prefer the project of cwd")
	}
}

func TestScanMissingFileAndLongTitle(t *testing.T) {
	if ti, cw := Scan(filepath.Join(t.TempDir(), "nope.jsonl")); ti != "" || cw != "" {
		t.Error("missing file")
	}
	root := t.TempDir()
	long := ""
	for i := 0; i < 200; i++ {
		long += "word "
	}
	p := writeSession(t, root, "-p", "s", []map[string]any{{"type": "user", "message": map[string]any{"content": long}}}, time.Now())
	if ti, _ := Scan(p); len([]rune(ti)) != 80 {
		t.Errorf("title length = %d", len([]rune(ti)))
	}
}
