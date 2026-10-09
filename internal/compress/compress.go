// Package compress shrinks long tool outputs without changing the JSON shape of a tool response.
//
// It is meant for a Claude Code PostToolUse hook: every long string value is cleaned (ANSI
// escapes, carriage-return progress, repeated and blank lines) and, if it is still too long,
// reduced to its first and last lines. The full original text is saved to a file whose path is
// written into the omission marker, so nothing is lost for good. Failure-looking outputs keep
// more. The transformation is idempotent.
package compress

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	DefaultMinBytes = 6000
	pruneAfter      = 7 * 24 * time.Hour
)

// Options tunes the compressor. The zero value is usable (defaults apply).
type Options struct {
	MinBytes  int      // strings longer than this are processed (default 6000)
	OutputDir string   // where full originals are saved (default ~/.claude/agents-tree/tool-outputs)
	KeepWhole []string // tools whose output is cleaned but never truncated (default: Read)
	Now       func() time.Time
}

// OptionsFromEnv builds Options from AGENTS_TREE_COMPRESS_MIN and AGENTS_TREE_OUTPUT_DIR.
func OptionsFromEnv() Options {
	o := Options{OutputDir: os.Getenv("AGENTS_TREE_OUTPUT_DIR")}
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("AGENTS_TREE_COMPRESS_MIN"))); err == nil && v > 0 {
		o.MinBytes = v
	}
	return o
}

// Stats describes one Compress call. Sizes are the JSON sizes of the response.
type Stats struct {
	Tool      string
	BytesIn   int
	BytesOut  int
	SavedPath string // full original of the first truncated string, if any
}

func (o Options) min() int {
	if o.MinBytes > 0 {
		return o.MinBytes
	}
	return DefaultMinBytes
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

func (o Options) dir() string {
	if o.OutputDir != "" {
		return o.OutputDir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "agents-tree", "tool-outputs")
}

func (o Options) keepWhole(tool string) bool {
	if o.KeepWhole == nil {
		return tool == "Read"
	}
	for _, t := range o.KeepWhole {
		if t == tool {
			return true
		}
	}
	return false
}

var (
	ansiRe   = regexp.MustCompile("\x1b\\[[0-9;?<=>]*[ -/]*[@-~]|\x1b\\][^\x07\x1b]*(?:\x07|\x1b\\\\)|\x1b[@-Z\\\\-_]")
	markerRe = regexp.MustCompile(`(?m)^… \d+ (?:lines|bytes) omitted \(full output: `)
	b64Re    = regexp.MustCompile(`^[A-Za-z0-9+/=_-]+$`)
	failRe   = regexp.MustCompile(`(?im)^\s*(?:error\b|fatal\b|panic:|traceback \(most recent|exception\b|fail(?:ed|ure)?\b|--- fail|npm err!|build failed|✗|✘|e\d{3,}\b)|` +
		`\b(?:exit (?:status|code) [1-9]|command failed|segmentation fault|permission denied|assertionerror|undefined reference|cannot find|no such file)\b`)
)

type run struct {
	opt     Options
	tool    string
	failed  bool
	saved   string
	changed bool
}

// Compress rewrites the long string values of a tool response (any JSON value). It returns the
// new response, size stats, and whether anything was saved. On invalid JSON, images, or when
// nothing shrinks, the input is returned unchanged with changed=false.
func Compress(toolName string, response json.RawMessage, opt Options) (out json.RawMessage, stats Stats, changed bool) {
	stats = Stats{Tool: toolName, BytesIn: len(response), BytesOut: len(response)}
	root, err := parseTree(response)
	if err != nil {
		return response, stats, false
	}
	image, failed := scan(root)
	if image {
		return response, stats, false
	}
	r := &run{opt: opt, tool: toolName, failed: failed}
	r.walk(root, "")
	if !r.changed {
		return response, stats, false
	}
	var b bytesBuf
	root.write(&b.Buffer)
	if b.Len() >= len(response) {
		return response, stats, false
	}
	stats.BytesOut, stats.SavedPath = b.Len(), r.saved
	return b.Bytes(), stats, true
}

func (r *run) walk(n *node, key string) {
	switch n.kind {
	case 's':
		if ns := r.str(key, n.s); ns != n.s {
			n.s, r.changed = ns, true
		}
	case 'o':
		if isImageBlock(n) {
			return
		}
		for i, k := range n.keys {
			r.walk(n.kids[i], k)
		}
	case 'a':
		for _, kid := range n.kids {
			r.walk(kid, key)
		}
	}
}

func looksBinary(s string) bool {
	if strings.IndexByte(s, 0) >= 0 || strings.HasPrefix(s, "data:") {
		return true
	}
	return len(s) > 1024 && !strings.ContainsAny(s, " \n\t") && b64Re.MatchString(s)
}

func (r *run) str(key, s string) string {
	min := r.opt.min()
	if len(s) <= min || looksBinary(s) {
		return s
	}
	cleaned := clean(s)
	failed := r.failed || key == "stderr" || looksFailed(cleaned)
	if len(cleaned) <= min || markerRe.MatchString(cleaned) || r.opt.keepWhole(r.tool) {
		return cleaned
	}
	if t, ok := r.truncate(s, cleaned, min, failed); ok {
		return t
	}
	return cleaned
}

func looksFailed(s string) bool {
	lines := strings.Split(s, "\n")
	const n = 15
	var sample string
	if len(lines) <= 2*n {
		sample = s
	} else {
		sample = strings.Join(lines[:n], "\n") + "\n" + strings.Join(lines[len(lines)-n:], "\n")
	}
	return failRe.MatchString(sample)
}

// clean strips ANSI escapes and carriage-return progress, drops blank-line runs and collapses
// runs (3+) of identical lines into `line  (×N)`.
func clean(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.IndexByte(l, '\r') >= 0 {
			segs := strings.Split(l, "\r")
			l = ""
			for i := len(segs) - 1; i >= 0; i-- {
				if segs[i] != "" {
					l = segs[i]
					break
				}
			}
		}
		if strings.TrimSpace(l) == "" && len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
			continue
		}
		out = append(out, l)
	}
	res := make([]string, 0, len(out))
	for i := 0; i < len(out); {
		j := i + 1
		for j < len(out) && out[j] == out[i] {
			j++
		}
		if j-i >= 3 && strings.TrimSpace(out[i]) != "" {
			res = append(res, fmt.Sprintf("%s  (×%d)", out[i], j-i))
		} else {
			res = append(res, out[i:j]...)
		}
		i = j
	}
	return strings.Join(res, "\n")
}

// truncate keeps head/tail lines (or bytes when there are too few lines) and saves orig.
func (r *run) truncate(orig, cleaned string, min int, failed bool) (string, bool) {
	head, tail := 60, 40
	if failed {
		head, tail = 100, 100
	}
	dir := r.opt.dir()
	if dir == "" {
		return "", false
	}
	var build func(path string) string
	lines := strings.Split(cleaned, "\n")
	if omitted := len(lines) - head - tail; omitted >= 2 {
		build = func(path string) string {
			marker := fmt.Sprintf("… %d lines omitted (full output: %s) …", omitted, path)
			return strings.Join(lines[:head], "\n") + "\n" + marker + "\n" + strings.Join(lines[len(lines)-tail:], "\n")
		}
	}
	hb, tb := min*6/10, min*4/10
	if failed {
		hb, tb = min, min
	}
	byteMode := func(path string) string {
		h, t := cutRunes(cleaned, hb, tb)
		return h + "\n… " + strconv.Itoa(len(cleaned)-len(h)-len(t)) + " bytes omitted (full output: " + path + ") …\n" + t
	}
	// size check with a placeholder path of realistic length
	probe := filepath.Join(dir, strings.Repeat("0", 20)+".txt")
	if build == nil || len(build(probe)) >= len(cleaned) {
		build = byteMode
	}
	if len(build(probe)) >= len(cleaned) {
		return "", false
	}
	path, err := r.save(dir, orig)
	if err != nil {
		return "", false
	}
	if r.saved == "" {
		r.saved = path
	}
	return build(path), true
}

// cutRunes returns the first headB and last tailB bytes of s, moved inward to rune boundaries.
func cutRunes(s string, headB, tailB int) (string, string) {
	if headB+tailB >= len(s) {
		return s, ""
	}
	h := headB
	for h > 0 && !utf8.RuneStart(s[h]) {
		h--
	}
	t := len(s) - tailB
	for t < len(s) && !utf8.RuneStart(s[t]) {
		t++
	}
	return s[:h], s[t:]
}

func (r *run) save(dir, text string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(text))
	path := filepath.Join(dir, hex.EncodeToString(sum[:])[:16]+".txt")
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(text)) {
		_ = os.Chtimes(path, r.opt.now(), r.opt.now())
	} else if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return "", err
	}
	prune(dir, r.opt.now())
	return path, nil
}

// prune removes saved outputs older than seven days (best effort).
func prune(dir string, now time.Time) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		if info, err := e.Info(); err == nil && now.Sub(info.ModTime()) > pruneAfter {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
