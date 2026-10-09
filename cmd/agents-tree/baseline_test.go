package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBaseline(t *testing.T) {
	dir := t.TempDir()
	sess := filepath.Join(dir, "projects", "p", "s.jsonl")
	os.MkdirAll(filepath.Dir(sess), 0o755)
	line := `{"type":"assistant","message":{"id":"m1","model":"claude-sonnet-5-5","content":[],"usage":{"input_tokens":1,"cache_creation_input_tokens":10,"cache_read_input_tokens":0}}}` + "\n"
	os.WriteFile(sess, []byte(line), 0o644)
	common := []string{"--projects", filepath.Join(dir, "projects"), "--home", filepath.Join(dir, "home")}
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{"ok", append([]string{sess}, common...), 0, "first request: 11 input tokens", ""},
		{"ok with flags first and --recent=", append(append([]string{"--recent=3"}, common...), sess), 0, "(1 scanned)", ""},
		{"missing file", []string{filepath.Join(dir, "none.jsonl")}, 1, "", "not found"},
		{"bad recent", []string{"--recent", "x", sess}, 2, "", "positive number"},
		{"unknown flag", []string{"--bogus"}, 2, "", "unknown flag"},
		{"two paths", []string{sess, sess}, 2, "", "usage"},
		{"flag without value", []string{"--recent"}, 2, "", "needs a value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if code := runBaseline(tc.args, &out, &errb); code != tc.code {
				t.Fatalf("exit %d, want %d (stderr %q)", code, tc.code, errb.String())
			}
			if !strings.Contains(out.String(), tc.stdout) || !strings.Contains(errb.String(), tc.stderr) {
				t.Errorf("stdout %q stderr %q", out.String(), errb.String())
			}
		})
	}
}
