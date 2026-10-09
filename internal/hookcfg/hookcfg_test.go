package hookcfg

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func mustInstall(t *testing.T, src string) Result {
	t.Helper()
	r, err := Install([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, s)
	}
	return m
}

func TestInstallEmptyAndMissing(t *testing.T) {
	for _, src := range []string{"", "  \n", "{}"} {
		r := mustInstall(t, src)
		if !r.Changed {
			t.Fatalf("%q: not changed", src)
		}
		m := decode(t, r.Out)
		hooks := m["hooks"].(map[string]any)
		post := hooks["PostToolUse"].([]any)[0].(map[string]any)
		if post["matcher"] != "Read|Grep|Glob|WebFetch|WebSearch|mcp__.*" {
			t.Errorf("matcher %v", post["matcher"])
		}
		h := post["hooks"].([]any)[0].(map[string]any)
		if h["type"] != "command" || h["command"] != "agents-tree compress" || h["timeout"] != float64(10) {
			t.Errorf("handler %v", h)
		}
		for _, ev := range []string{"SubagentStart", "SubagentStop"} {
			g := hooks[ev].([]any)[0].(map[string]any)
			if _, has := g["matcher"]; has {
				t.Errorf("%s must not have a matcher", ev)
			}
			hh := g["hooks"].([]any)[0].(map[string]any)
			if hh["command"] != "agents-tree hook" || hh["timeout"] != float64(5) {
				t.Errorf("%s handler %v", ev, hh)
			}
		}
		if !strings.HasSuffix(r.Out, "}\n") {
			t.Error("missing trailing newline")
		}
	}
}

const rich = `{
  "model": "opus",
  "env": {"B": "2", "A": "1"},
  "permissions": {"allow": ["Bash(ls)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "rtk hook claude"}]}
    ],
    "PostToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "/x/other.sh", "timeout": 3}]},
      {"matcher": "Write", "hooks": [{"type": "command", "command": "fmt.sh"}]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "notify.sh"}]}]
  },
  "zlast": null
}
`

func TestInstallPreservesEverythingElse(t *testing.T) {
	r := mustInstall(t, rich)
	before, after := decode(t, rich), decode(t, r.Out)
	// unknown keys identical
	for _, k := range []string{"model", "env", "permissions", "zlast"} {
		if !reflect.DeepEqual(before[k], after[k]) {
			t.Errorf("key %s changed", k)
		}
	}
	bh, ah := before["hooks"].(map[string]any), after["hooks"].(map[string]any)
	for _, ev := range []string{"PreToolUse", "Stop"} {
		if !reflect.DeepEqual(bh[ev], ah[ev]) {
			t.Errorf("%s changed", ev)
		}
	}
	post := ah["PostToolUse"].([]any)
	if len(post) != 3 {
		t.Fatalf("PostToolUse groups = %d", len(post))
	}
	if !reflect.DeepEqual(post[:2], bh["PostToolUse"]) {
		t.Error("existing PostToolUse groups modified or reordered")
	}
	// key order preserved in text
	idx := func(s string) int { return strings.Index(r.Out, s) }
	if !(idx(`"model"`) < idx(`"env"`) && idx(`"B"`) < idx(`"A"`) && idx(`"permissions"`) < idx(`"hooks"`) && idx(`"PreToolUse"`) < idx(`"PostToolUse"`) && idx(`"Stop"`) < idx(`"SubagentStart"`) && idx(`"hooks"`) < idx(`"zlast"`)) {
		t.Errorf("key order not preserved:\n%s", r.Out)
	}
}

func TestInstallIdempotent(t *testing.T) {
	for _, src := range []string{"", "{}", rich} {
		r1 := mustInstall(t, src)
		r2 := mustInstall(t, r1.Out)
		if r2.Changed || r2.Out != r1.Out {
			t.Errorf("second install changed the file")
		}
		if n := strings.Count(r2.Out, "agents-tree compress"); n != 1 {
			t.Errorf("compress count %d", n)
		}
		if n := strings.Count(r2.Out, "agents-tree hook"); n != 2 {
			t.Errorf("hook count %d", n)
		}
	}
}

func TestInstallRecognisesExistingByCommandNotMatcher(t *testing.T) {
	src := `{"hooks":{"SubagentStart":[{"hooks":[{"type":"command","command":"/usr/local/bin/agents-tree hook"}]}],
	"PostToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"agents-tree compress"}]}]}}`
	r := mustInstall(t, src)
	if strings.Count(r.Out, "agents-tree") != 3 { // 2 existing + SubagentStop
		t.Errorf("duplicated entries:\n%s", r.Out)
	}
	if !strings.Contains(r.Out, "SubagentStop") {
		t.Error("SubagentStop not added")
	}
}

func TestInstallKeepsLookalikeCommands(t *testing.T) {
	src := `{"hooks":{"PostToolUse":[{"hooks":[{"type":"command","command":"agents-tree compress --dry"},{"type":"command","command":"my-agents-tree compress"}]}]}}`
	r := mustInstall(t, src)
	if n := strings.Count(r.Out, `"command": "agents-tree compress"`); n != 1 {
		t.Errorf("lookalikes must not count as ours:\n%s", r.Out)
	}
}

func TestInstallErrors(t *testing.T) {
	cases := map[string]string{
		"invalid json":        `{"hooks": {`,
		"trailing":            `{} {}`,
		"array root":          `[]`,
		"string root":         `"x"`,
		"null root":           `null`,
		"hooks not object":    `{"hooks": []}`,
		"hooks string":        `{"hooks": "nope"}`,
		"event not array":     `{"hooks": {"PostToolUse": {"matcher": "x"}}}`,
		"substart not array":  `{"hooks": {"SubagentStart": "x"}}`,
		"hooks null":          `{"hooks": null}`,
		"event null":          `{"hooks": {"SubagentStop": null}}`,
		"dangling comma form": `{"a": 1,}`,
	}
	for name, src := range cases {
		for opName, op := range map[string]func([]byte) (Result, error){"install": Install, "uninstall": Uninstall} {
			r, err := op([]byte(src))
			if err == nil {
				t.Errorf("%s/%s: expected error, got %q", name, opName, r.Out)
			}
			if r.Changed || r.Out != "" {
				t.Errorf("%s/%s: result must be empty on error", name, opName)
			}
		}
		if _, err := Status([]byte(src)); err == nil {
			t.Errorf("%s: status should fail", name)
		}
	}
	if _, err := Install([]byte(`{"hooks":[]}`)); !errors.Is(err, ErrShape) {
		t.Errorf("want ErrShape, got %v", err)
	}
}

func TestUninstallRemovesOnlyOurs(t *testing.T) {
	r := mustInstall(t, rich)
	u, err := Uninstall([]byte(r.Out))
	if err != nil || !u.Changed {
		t.Fatalf("err=%v changed=%v", err, u.Changed)
	}
	if !reflect.DeepEqual(decode(t, u.Out), decode(t, rich)) {
		t.Errorf("uninstall did not restore an equivalent structure:\n%s", u.Out)
	}
	u2, _ := Uninstall([]byte(u.Out))
	if u2.Changed {
		t.Error("second uninstall changed the file")
	}
}

func TestUninstallPrunesEmptyContainers(t *testing.T) {
	for _, src := range []string{"{}", `{"model":"x"}`} {
		r := mustInstall(t, src)
		u, err := Uninstall([]byte(r.Out))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decode(t, u.Out), decode(t, src)) {
			t.Errorf("not restored: %s", u.Out)
		}
		if strings.Contains(u.Out, "hooks") {
			t.Errorf("empty hooks container left behind:\n%s", u.Out)
		}
	}
}

func TestUninstallSharedGroupKeepsOthers(t *testing.T) {
	src := `{"hooks":{"PostToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"keep.sh"},{"type":"command","command":"agents-tree compress","timeout":10}]}]}}`
	u, err := Uninstall([]byte(src))
	if err != nil || !u.Changed {
		t.Fatal(err)
	}
	want := `{"hooks":{"PostToolUse":[{"matcher":"Read","hooks":[{"type":"command","command":"keep.sh"}]}]}}`
	if !reflect.DeepEqual(decode(t, u.Out), decode(t, want)) {
		t.Errorf("got %s", u.Out)
	}
}

func TestUninstallLeavesUnrelatedEmptyContainers(t *testing.T) {
	src := `{"hooks":{"Stop":[]}}`
	u, _ := Uninstall([]byte(src))
	if u.Changed || u.Out != src {
		t.Errorf("must not touch unrelated containers: %q", u.Out)
	}
	src = `{"hooks":{}}`
	u, _ = Uninstall([]byte(src))
	if u.Changed {
		t.Error("changed")
	}
}

func TestIndentDetected(t *testing.T) {
	r := mustInstall(t, "{\n\t\"model\": \"x\"\n}\n")
	if !strings.Contains(r.Out, "\n\t\"model\"") || strings.Contains(r.Out, "\n  \"") {
		t.Errorf("tab indent lost:\n%s", r.Out)
	}
	r = mustInstall(t, "{\n    \"model\": \"x\"\n}\n")
	if !strings.Contains(r.Out, "\n    \"model\"") {
		t.Errorf("4-space indent lost:\n%s", r.Out)
	}
}

func TestStatus(t *testing.T) {
	rep, err := Status([]byte(rich))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range rep.Entries {
		if e.Installed {
			t.Errorf("%s reported installed", e.Event)
		}
	}
	if !rep.RTKHook {
		t.Error("rtk hook not detected")
	}
	r := mustInstall(t, rich)
	rep, _ = Status([]byte(r.Out))
	for _, e := range rep.Entries {
		if !e.Installed {
			t.Errorf("%s not installed", e.Event)
		}
	}
	rep, _ = Status(nil)
	if rep.RTKHook || len(rep.Entries) != 3 {
		t.Errorf("empty: %+v", rep)
	}
}

func TestDiff(t *testing.T) {
	old := "{\n  \"a\": 1\n}\n"
	r := mustInstall(t, old)
	d := Diff("a", "b", old, r.Out)
	if !strings.HasPrefix(d, "--- a\n+++ b\n@@ ") || !strings.Contains(d, "+  \"hooks\": {") || !strings.Contains(d, "-  \"a\": 1\n") && !strings.Contains(d, "   \"a\": 1") {
		t.Errorf("diff:\n%s", d)
	}
	if Diff("a", "b", old, old) != "" {
		t.Error("equal inputs must give empty diff")
	}
	if d := Diff("a", "b", "", "x\n"); !strings.Contains(d, "@@ -0,0 +1,1 @@") && !strings.Contains(d, "+x") {
		t.Errorf("new file diff:\n%s", d)
	}
}

func TestApplyCreatesBackupAndIsAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	orig := "{\n  \"model\": \"x\"\n}\n"
	if err := os.WriteFile(p, []byte(orig), 0o640); err != nil {
		t.Fatal(err)
	}
	r := mustInstall(t, orig)
	now := time.Date(2026, 10, 9, 12, 30, 5, 0, time.UTC)
	bak, err := Apply(p, r.Out, now)
	if err != nil {
		t.Fatal(err)
	}
	if bak != p+".bak-20261009-123005" {
		t.Errorf("backup name %s", bak)
	}
	if b, _ := os.ReadFile(bak); string(b) != orig {
		t.Error("backup content differs")
	}
	if st, _ := os.Stat(bak); st.Mode().Perm() != 0o600 {
		t.Errorf("backup mode %v", st.Mode().Perm())
	}
	if b, _ := os.ReadFile(p); string(b) != r.Out {
		t.Error("settings not written")
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o640 {
		t.Errorf("settings mode changed: %v", st.Mode().Perm())
	}
	// same-second second apply must not overwrite the first backup
	bak2, err := Apply(p, orig, now)
	if err != nil || bak2 == bak {
		t.Errorf("backup collision: %s %v", bak2, err)
	}
	// no temp files left behind
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left: %s", e.Name())
		}
	}
}

func TestApplyNewFileAndSymlink(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "newdir", "settings.json")
	bak, err := Apply(p, "{}\n", time.Now())
	if err != nil || bak != "" {
		t.Fatalf("bak=%q err=%v", bak, err)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode().Perm())
	}
	if st, _ := os.Stat(filepath.Dir(p)); st.Mode().Perm() != 0o700 {
		t.Errorf("dir mode %v", st.Mode().Perm())
	}
	link := filepath.Join(dir, "link.json")
	if err := os.Symlink(p, link); err != nil {
		t.Skip("symlinks unavailable")
	}
	if _, err := Apply(link, "{\"a\":1}\n", time.Now()); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Error("symlink replaced by a regular file")
	}
	if b, _ := os.ReadFile(p); string(b) != "{\"a\":1}\n" {
		t.Error("write did not go through the symlink")
	}
}
