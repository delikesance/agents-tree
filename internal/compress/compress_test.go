package compress

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func opts(t *testing.T, min int) Options {
	t.Helper()
	return Options{MinBytes: min, OutputDir: filepath.Join(t.TempDir(), "out")}
}

func js(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func numbered(n int, prefix string) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "%s line %d with some filler text\n", prefix, i)
	}
	return sb.String()
}

// field decodes the response and returns the string at the given top-level key.
func field(t *testing.T, raw json.RawMessage, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, raw)
	}
	s, _ := m[key].(string)
	return s
}

func TestCleanSteps(t *testing.T) {
	pad := strings.Repeat("x", 100)
	bloat := strings.Repeat("\x1b[0m", 300) // pushes every input over the threshold; cleaned text stays small
	cases := []struct {
		name, in string
		want     []string // substrings that must appear
		not      []string // substrings that must not
	}{
		{"ansi", "\x1b[31mred\x1b[0m \x1b[1;32mgreen\x1b[0m\x1b]0;title\x07 ok\n" + pad, []string{"red green ok"}, []string{"\x1b"}},
		{"cr progress", "downloading 10%\rdownloading 50%\rdownloading 100%\ndone\n" + pad, []string{"downloading 100%\ndone"}, []string{"10%", "50%"}},
		{"crlf", "a\r\nb\r\n" + pad, []string{"a\nb\n"}, []string{"\r"}},
		{"dup", "start\n" + strings.Repeat("retrying...\n", 40) + "end\n" + pad, []string{"retrying...  (×40)", "start", "end"}, nil},
		{"two dups stay", "a\na\nb\n" + pad, []string{"a\na\nb"}, []string{"(×"}},
		{"blank runs", "a\n\n\n\n\nb\n" + pad, []string{"a\n\nb"}, []string{"\n\n\n"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, st, changed := Compress("Bash", js(t, map[string]any{"stdout": bloat + c.in}), opts(t, 1000))
			if !changed {
				t.Fatalf("expected change")
			}
			got := field(t, out, "stdout")
			for _, w := range c.want {
				if !strings.Contains(got, w) {
					t.Errorf("missing %q in %q", w, got)
				}
			}
			for _, w := range c.not {
				if strings.Contains(got, w) {
					t.Errorf("unexpected %q in %q", w, got)
				}
			}
			if st.BytesOut >= st.BytesIn || st.Tool != "Bash" || st.SavedPath != "" {
				t.Errorf("stats %+v", st)
			}
		})
	}
}

func TestTruncateHeadTailAndSavedFile(t *testing.T) {
	o := opts(t, 1000)
	orig := numbered(500, "build")
	in := js(t, map[string]any{"stdout": orig, "stderr": "", "interrupted": false, "isImage": false})
	out, st, changed := Compress("Bash", in, o)
	if !changed || st.SavedPath == "" {
		t.Fatalf("changed=%v stats=%+v", changed, st)
	}
	got := field(t, out, "stdout")
	lines := strings.Split(got, "\n")
	// 60 head + marker + 40 tail (+ the empty element after the final newline)
	if len(lines) != 60+1+40+1 {
		t.Fatalf("got %d lines", len(lines))
	}
	if lines[0] != "build line 1 with some filler text" || lines[59] != "build line 60 with some filler text" {
		t.Errorf("head wrong: %q %q", lines[0], lines[59])
	}
	wantMarker := fmt.Sprintf("… 400 lines omitted (full output: %s) …", st.SavedPath)
	if lines[60] != wantMarker {
		t.Errorf("marker %q want %q", lines[60], wantMarker)
	}
	if lines[61] != "build line 461 with some filler text" {
		t.Errorf("tail wrong: %q", lines[61])
	}
	saved, err := os.ReadFile(st.SavedPath)
	if err != nil || string(saved) != orig {
		t.Fatalf("saved file differs from original (err=%v)", err)
	}
	fi, _ := os.Stat(st.SavedPath)
	di, _ := os.Stat(filepath.Dir(st.SavedPath))
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Errorf("perms file=%v dir=%v", fi.Mode().Perm(), di.Mode().Perm())
	}
	if !regexp.MustCompile(`^[0-9a-f]{16}\.txt$`).MatchString(filepath.Base(st.SavedPath)) {
		t.Errorf("name %s", st.SavedPath)
	}
	// other keys untouched
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	if m["stderr"] != "" || m["interrupted"] != false || m["isImage"] != false {
		t.Errorf("shape changed: %v", m)
	}
}

func TestThresholdBoundary(t *testing.T) {
	o := opts(t, 500)
	// a string that needs cleaning (ANSI) so any processing would be visible
	mk := func(n int) string {
		s := "\x1b[0m" + strings.Repeat("a", n-4)
		if len(s) != n {
			t.Fatal("bad fixture")
		}
		return s
	}
	if _, _, changed := Compress("Bash", js(t, map[string]string{"stdout": mk(500)}), o); changed {
		t.Error("string of exactly MinBytes must be left alone")
	}
	if _, _, changed := Compress("Bash", js(t, map[string]string{"stdout": mk(501)}), o); !changed {
		t.Error("string above MinBytes must be processed")
	}
	// cleaned text within the limit: cleaned but not truncated, no file
	out, st, changed := Compress("Bash", js(t, map[string]string{"stdout": strings.Repeat("\x1b[0m", 100) + strings.Repeat("a", 400)}), o)
	if !changed || st.SavedPath != "" || len(field(t, out, "stdout")) != 400 {
		t.Errorf("changed=%v st=%+v", changed, st)
	}
}

func TestShapePreserved(t *testing.T) {
	big := numbered(300, "x")
	in := json.RawMessage(`{"z":1,"a":{"n":[1,2.50,-3e2,null,true,false,{"deep":"` + strings.ReplaceAll(big, "\n", `\n`) + `","k":null}],"s":"short"},"m":"` + strings.ReplaceAll(big, "\n", `\n`) + `","b":false,"big":12345678901234567890}`)
	out, _, changed := Compress("mcp__x__y", in, opts(t, 1000))
	if !changed {
		t.Fatal("expected change")
	}
	var a, b any
	dec := func(r json.RawMessage, v *any) {
		d := json.NewDecoder(strings.NewReader(string(r)))
		d.UseNumber()
		if err := d.Decode(v); err != nil {
			t.Fatal(err)
		}
	}
	dec(in, &a)
	dec(out, &b)
	if !sameShape(a, b) {
		t.Fatalf("shape changed:\n%s\n%s", in[:80], out)
	}
	if !strings.HasPrefix(string(out), `{"z":1,"a":{"n":[1,2.50,-3e2,null,true,false,{"deep":`) {
		t.Errorf("key order or number literals not preserved: %.120s", out)
	}
	if !strings.Contains(string(out), `"big":12345678901234567890}`) {
		t.Error("large number literal altered")
	}
}

// sameShape compares two decoded values: identical except that strings may differ.
func sameShape(a, b any) bool {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !sameShape(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !sameShape(x[i], y[i]) {
				return false
			}
		}
		return true
	case string:
		_, ok := b.(string)
		return ok
	}
	return reflect.DeepEqual(a, b)
}

func TestErrorOutputsKeepMore(t *testing.T) {
	body := numbered(500, "log")
	cases := []struct {
		name string
		resp map[string]any
	}{
		{"is_error", map[string]any{"is_error": true, "stdout": body}},
		{"interrupted", map[string]any{"interrupted": true, "stdout": body}},
		{"exitCode", map[string]any{"exitCode": 2, "stdout": body}},
		{"stderr key", map[string]any{"stdout": "ok", "stderr": body}},
		{"text marker tail", map[string]any{"stdout": body + "FAIL github.com/x/y 0.3s\n"}},
		{"text marker head", map[string]any{"stdout": "Traceback (most recent call last):\n" + body}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, st, changed := Compress("Bash", js(t, c.resp), opts(t, 1000))
			if !changed {
				t.Fatal("expected change")
			}
			var key string
			for _, k := range []string{"stdout", "stderr"} {
				if s := field(t, out, k); strings.Contains(s, "lines omitted") {
					key = k
				}
			}
			s := field(t, out, key)
			ls := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
			if len(ls) != 201 {
				t.Errorf("want 100+marker+100 lines, got %d", len(ls))
			}
			if st.SavedPath == "" {
				t.Error("no saved file")
			}
		})
	}
	// a clean success truncates harder
	out, _, _ := Compress("Bash", js(t, map[string]any{"stdout": body}), opts(t, 1000))
	if n := len(strings.Split(field(t, out, "stdout"), "\n")); n != 102 {
		t.Errorf("non-error lines %d", n)
	}
}

func TestErrorFewLinesKeptWhole(t *testing.T) {
	// error mode keeps 100+100 lines: 150 ordinary lines must survive intact even above the threshold
	body := numbered(150, "e")
	in := js(t, map[string]any{"is_error": true, "stdout": body})
	out, st, changed := Compress("Bash", in, opts(t, 1000))
	if changed || st.SavedPath != "" || string(out) != string(in) {
		t.Errorf("changed=%v st=%+v", changed, st)
	}
}

func TestIdempotent(t *testing.T) {
	long := numbered(400, "p")
	inputs := map[string]any{
		"truncated": map[string]any{"stdout": long, "stderr": ""},
		"error":     map[string]any{"is_error": true, "stdout": long},
		"cleaned":   map[string]any{"stdout": "\x1b[1mhi\x1b[0m\n" + strings.Repeat("same\n", 500) + strings.Repeat("\n", 10)},
		"oneline":   map[string]any{"stdout": strings.Repeat("é€漢", 4000)},
		"wide":      map[string]any{"stdout": strings.Repeat(strings.Repeat("w", 200)+"\n", 150)},
		"mcp":       map[string]any{"content": []any{map[string]any{"type": "text", "text": long}}},
	}
	for name, v := range inputs {
		t.Run(name, func(t *testing.T) {
			o := opts(t, 1000)
			out1, _, changed := Compress("mcp__a__b", js(t, v), o)
			if !changed {
				t.Fatal("first pass should change")
			}
			out2, st2, changed2 := Compress("mcp__a__b", out1, o)
			if changed2 || string(out2) != string(out1) || st2.BytesOut != st2.BytesIn {
				t.Errorf("second pass changed output (stats %+v)", st2)
			}
		})
	}
}

func TestUnicodeSafety(t *testing.T) {
	for _, unit := range []string{"é", "€", "漢", "😀", "a😀"} {
		orig := strings.Repeat(unit, 3000) // a single very long line: forces byte-level cut
		for _, min := range []int{999, 1000, 1001, 1002, 1003} {
			out, st, changed := Compress("WebFetch", js(t, map[string]string{"result": orig}), opts(t, min))
			if !changed {
				t.Fatalf("%q min=%d: expected change", unit, min)
			}
			got := field(t, out, "result")
			if !utf8.ValidString(got) || strings.ContainsRune(got, utf8.RuneError) {
				t.Fatalf("%q min=%d: invalid UTF-8 in output", unit, min)
			}
			if !strings.Contains(got, "bytes omitted (full output: "+st.SavedPath+")") {
				t.Fatalf("no byte marker: %.200s", got)
			}
			if b, _ := os.ReadFile(st.SavedPath); string(b) != orig {
				t.Fatal("saved file differs")
			}
		}
	}
}

func TestToolShapes(t *testing.T) {
	long := numbered(300, "src")
	t.Run("Read is never truncated", func(t *testing.T) {
		resp := map[string]any{"type": "text", "file": map[string]any{"filePath": "/a/b.go", "content": "\x1b[0m" + long, "numLines": 300, "startLine": 1, "totalLines": 300}}
		out, st, changed := Compress("Read", js(t, resp), opts(t, 1000))
		if !changed || st.SavedPath != "" {
			t.Fatalf("changed=%v st=%+v", changed, st)
		}
		var m struct {
			File struct {
				Content  string
				NumLines int
			}
		}
		_ = json.Unmarshal(out, &m)
		if m.File.Content != long || m.File.NumLines != 300 {
			t.Errorf("Read content altered beyond cleaning")
		}
	})
	t.Run("Read truncates if KeepWhole cleared", func(t *testing.T) {
		o := opts(t, 1000)
		o.KeepWhole = []string{}
		resp := map[string]any{"type": "text", "file": map[string]any{"content": long}}
		_, st, changed := Compress("Read", js(t, resp), o)
		if !changed || st.SavedPath == "" {
			t.Error("expected truncation")
		}
	})
	t.Run("Grep and Glob", func(t *testing.T) {
		resp := map[string]any{"mode": "content", "numFiles": 3, "filenames": []string{"a", "b"}, "content": long, "numLines": 300}
		out, _, changed := Compress("Grep", js(t, resp), opts(t, 1000))
		if !changed || !strings.Contains(field(t, out, "content"), "lines omitted") {
			t.Fatal("grep content not truncated")
		}
		var m map[string]any
		_ = json.Unmarshal(out, &m)
		if m["numFiles"] != float64(3) || len(m["filenames"].([]any)) != 2 {
			t.Errorf("non-string fields changed: %v", m)
		}
	})
	t.Run("MCP content array", func(t *testing.T) {
		img := strings.Repeat("QUJD", 5000)
		resp := []any{
			map[string]any{"type": "text", "text": long},
			map[string]any{"type": "image", "data": img, "mimeType": "image/png"},
			map[string]any{"type": "text", "text": "short"},
		}
		out, _, changed := Compress("mcp__x__shot", js(t, resp), opts(t, 1000))
		if !changed {
			t.Fatal("expected change")
		}
		var got []map[string]string
		if err := json.Unmarshal(out, &got); err != nil || len(got) != 3 {
			t.Fatalf("array shape lost: %v %s", err, out[:60])
		}
		if got[1]["data"] != img || got[2]["text"] != "short" || !strings.Contains(got[0]["text"], "lines omitted") {
			t.Error("array elements wrong")
		}
	})
	t.Run("bare string response", func(t *testing.T) {
		out, _, changed := Compress("mcp__x__y", js(t, long), opts(t, 1000))
		var s string
		if !changed || json.Unmarshal(out, &s) != nil || !strings.Contains(s, "lines omitted") {
			t.Errorf("changed=%v out=%.60s", changed, out)
		}
	})
	t.Run("isImage untouched", func(t *testing.T) {
		resp := map[string]any{"stdout": long, "stderr": "", "interrupted": false, "isImage": true}
		in := js(t, resp)
		out, _, changed := Compress("Bash", in, opts(t, 1000))
		if changed || string(out) != string(in) {
			t.Error("image response modified")
		}
	})
	t.Run("base64 blob untouched", func(t *testing.T) {
		in := js(t, map[string]string{"blob": strings.Repeat("QUJD", 2000)})
		if _, _, changed := Compress("mcp__x__y", in, opts(t, 1000)); changed {
			t.Error("binary-looking string modified")
		}
	})
}

func TestGarbageInput(t *testing.T) {
	for _, in := range []string{"", "   ", "not json", `{"a":`, `{"a":"b"} trailing`, `{"a":"b"}{"c":1}`, `null`, `123`, `[1,2`, `{"a" "b"}`, "\x00\x01\x02", strings.Repeat("[", 10000)} {
		out, st, changed := Compress("Bash", json.RawMessage(in), opts(t, 10))
		if changed || string(out) != in || st.BytesIn != len(in) || st.BytesOut != len(in) {
			t.Errorf("input %.20q: changed=%v out=%.20q", in, changed, out)
		}
	}
	// nil must not panic either
	if _, _, changed := Compress("Bash", nil, Options{}); changed {
		t.Error("nil changed")
	}
}

func TestSaveFailureKeepsOriginalInformation(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dir, []byte("x"), 0o600); err != nil { // OutputDir is a regular file: MkdirAll fails
		t.Fatal(err)
	}
	out, st, _ := Compress("Bash", js(t, map[string]string{"stdout": numbered(500, "n")}), Options{MinBytes: 1000, OutputDir: dir})
	if strings.Contains(string(out), "omitted") || st.SavedPath != "" {
		t.Error("must not omit lines when the original cannot be saved")
	}
}

func TestPrune(t *testing.T) {
	o := opts(t, 1000)
	if err := os.MkdirAll(o.OutputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(o.OutputDir, "old.txt")
	fresh := filepath.Join(o.OutputDir, "fresh.txt")
	other := filepath.Join(o.OutputDir, "keep.json")
	for _, p := range []string{old, fresh, other} {
		_ = os.WriteFile(p, []byte("x"), 0o600)
	}
	past := time.Now().Add(-8 * 24 * time.Hour)
	_ = os.Chtimes(old, past, past)
	_ = os.Chtimes(other, past, past)
	Compress("Bash", js(t, map[string]string{"stdout": numbered(500, "n")}), o)
	if _, err := os.Stat(old); err == nil {
		t.Error("old file not pruned")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Error("fresh file pruned")
	}
	if _, err := os.Stat(other); err != nil {
		t.Error("non-.txt file pruned")
	}
}

func TestOptionsFromEnv(t *testing.T) {
	t.Setenv("AGENTS_TREE_COMPRESS_MIN", "1234")
	t.Setenv("AGENTS_TREE_OUTPUT_DIR", "/x/y")
	o := OptionsFromEnv()
	if o.min() != 1234 || o.dir() != "/x/y" {
		t.Errorf("%+v", o)
	}
	t.Setenv("AGENTS_TREE_COMPRESS_MIN", "junk")
	if OptionsFromEnv().min() != DefaultMinBytes {
		t.Error("bad env must fall back to default")
	}
}

func TestAppendLog(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "log.jsonl")
	for i := 0; i < 2; i++ {
		if err := AppendLog(p, LogEntry{Session: "s1", Tool: "Grep", Agent: "worker", BytesIn: 100, BytesOut: 40, SavedPath: "/x.txt"}); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(p)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines %d", len(lines))
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"ts", "session", "tool", "agent", "bytes_in", "bytes_out", "saved_path"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %s", k)
		}
	}
	if len(m) != 7 {
		t.Errorf("unexpected keys: %v", m)
	}
	if st, _ := os.Stat(p); st.Mode().Perm() != 0o600 {
		t.Errorf("perm %v", st.Mode().Perm())
	}
	// default path honors the env var
	t.Setenv("AGENTS_TREE_COMPRESS_LOG", p)
	if DefaultLogPath() != p {
		t.Error("env ignored")
	}
}
