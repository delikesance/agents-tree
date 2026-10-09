package compress

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// bytesBuf wraps bytes.Buffer.
type bytesBuf struct{ bytes.Buffer }

// LogEntry is one line of compress-log.jsonl. It never holds content or commands.
type LogEntry struct {
	TS        string `json:"ts"`
	Session   string `json:"session,omitempty"`
	Tool      string `json:"tool"`
	Agent     string `json:"agent,omitempty"`
	BytesIn   int    `json:"bytes_in"`
	BytesOut  int    `json:"bytes_out"`
	SavedPath string `json:"saved_path,omitempty"`
}

// DefaultLogPath honors AGENTS_TREE_COMPRESS_LOG, else ~/.claude/agents-tree/compress-log.jsonl.
func DefaultLogPath() string {
	if p := os.Getenv("AGENTS_TREE_COMPRESS_LOG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "agents-tree", "compress-log.jsonl")
}

// AppendLog appends one JSON line to path (empty path: DefaultLogPath). TS is filled if empty.
func AppendLog(path string, e LogEntry) error {
	if path == "" {
		path = DefaultLogPath()
	}
	if path == "" {
		return os.ErrNotExist
	}
	if e.TS == "" {
		e.TS = time.Now().UTC().Format(time.RFC3339)
	}
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}
