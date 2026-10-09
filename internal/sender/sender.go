// Package sender sends messages to Claude Code by owning a `claude` subprocess.
//
// The process runs `claude -p --input-format stream-json --output-format stream-json`, resumed on the
// followed session (or started on a fresh --session-id). Messages go to its stdin; what Claude does
// lands in the normal transcript, so the chat and the tree pick it up through the usual tailers.
// Stdout is only read to learn when a turn ends (`result`) and to surface errors.
//
// This cannot write into an interactive `claude` already open in another terminal: do not drive the
// same session from both at once.
package sender

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Permissions maps a preset to CLI flags. "all" is the default chosen by the user: nothing can ask
// for confirmation in a headless run, so anything not allowed would simply be refused.
var Permissions = map[string][]string{
	"all":          {"--dangerously-skip-permissions"},
	"accept-edits": {"--permission-mode", "acceptEdits"},
	"plan":         {"--permission-mode", "plan"},
}

// Variables tying a Claude Code process to the session that launched agents-tree. Left in the
// child's environment they make it reuse that session's id and report into it.
var inherited = regexp.MustCompile(`^(CLAUDE_CODE_(SESSION|REMOTE|CHILD|MESSAGING|ENTRYPOINT|EXECPATH|DIAGNOSTICS)|CLAUDE_SESSION|SESSION_INGRESS|CLAUDECODE=|CLAUDE_PID=)`)

// CleanEnv drops those variables from a KEY=VALUE list.
func CleanEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !inherited.MatchString(kv) {
			out = append(out, kv)
		}
	}
	return out
}

// BuildCommand is the argv used to start Claude.
func BuildCommand(bin, sessionID string, resume bool, perms, model string) ([]string, error) {
	flags, ok := Permissions[perms]
	if !ok {
		return nil, fmt.Errorf("permissions must be one of all, accept-edits, plan (got %q)", perms)
	}
	cmd := []string{bin, "-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose"}
	cmd = append(cmd, flags...)
	if resume {
		cmd = append(cmd, "--resume", sessionID)
	} else {
		cmd = append(cmd, "--session-id", sessionID)
	}
	if model != "" {
		cmd = append(cmd, "--model", model)
	}
	return cmd, nil
}

// UserLine is one stream-json user message.
func UserLine(text string) []byte {
	b, _ := json.Marshal(map[string]any{"type": "user", "message": map[string]any{
		"role": "user", "content": []any{map[string]any{"type": "text", "text": text}}}})
	return append(b, '\n')
}

// NewUUID makes a random v4 UUID for a new session.
func NewUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

// Sender is one claude process bound to one session; restarted on demand after it exits.
type Sender struct {
	SessionID   string
	CWD         string
	Permissions string
	Bin         string
	Model       string

	mu        sync.Mutex
	resume    bool
	env       []string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	done      chan struct{}
	busy      bool
	lastError string
	sent      int
	errTail   []string
}

// New builds a sender. With an empty sessionID a fresh session id is generated (and not resumed).
func New(sessionID, cwd string, resume bool, perms, bin string) (*Sender, error) {
	if _, ok := Permissions[perms]; !ok {
		return nil, fmt.Errorf("permissions must be one of all, accept-edits, plan (got %q)", perms)
	}
	if sessionID == "" {
		sessionID, resume = NewUUID(), false
	}
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if bin == "" {
		bin = "claude"
	}
	return &Sender{SessionID: sessionID, CWD: cwd, Permissions: perms, Bin: bin, resume: resume,
		env: CleanEnv(os.Environ())}, nil
}

func (s *Sender) Available() bool {
	if _, err := exec.LookPath(s.Bin); err == nil {
		return true
	}
	st, err := os.Stat(s.Bin)
	return err == nil && !st.IsDir()
}

func (s *Sender) aliveLocked() bool {
	if s.done == nil {
		return false
	}
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *Sender) Alive() bool       { s.mu.Lock(); defer s.mu.Unlock(); return s.aliveLocked() }
func (s *Sender) Busy() bool        { s.mu.Lock(); defer s.mu.Unlock(); return s.busy }
func (s *Sender) LastError() string { s.mu.Lock(); defer s.mu.Unlock(); return s.lastError }
func (s *Sender) Sent() int         { s.mu.Lock(); defer s.mu.Unlock(); return s.sent }

func (s *Sender) startLocked() error {
	if s.aliveLocked() {
		return nil
	}
	argv, err := BuildCommand(s.Bin, s.SessionID, s.resume, s.Permissions, s.Model)
	if err != nil {
		return err
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir, cmd.Env = s.CWD, s.env
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()
	if err := cmd.Start(); err != nil {
		return err
	}
	s.cmd, s.stdin, s.errTail = cmd, stdin, nil
	s.done = make(chan struct{})
	s.resume = true // once started the session exists: later restarts resume it
	done := s.done
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { // stdout: turn results
		defer wg.Done()
		br := bufio.NewReaderSize(stdout, 1<<20)
		for {
			line, err := br.ReadBytes('\n')
			var d map[string]any
			if json.Unmarshal(line, &d) == nil && d["type"] == "result" {
				s.mu.Lock()
				s.busy = false
				if e, _ := d["is_error"].(bool); e {
					msg, _ := d["result"].(string)
					if msg == "" {
						msg, _ = d["subtype"].(string)
					}
					if msg == "" {
						msg = "error"
					}
					s.lastError = firstN(msg, 200)
				}
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { // stderr: keep the pipe empty (a full pipe blocks the child), remember the tail
		defer wg.Done()
		sc := bufio.NewScanner(stderr)
		sc.Buffer(make([]byte, 64*1024), 4<<20)
		for sc.Scan() {
			s.mu.Lock()
			s.errTail = append(s.errTail, sc.Text())
			if len(s.errTail) > 20 {
				s.errTail = s.errTail[1:]
			}
			s.mu.Unlock()
		}
	}()
	go func() {
		wg.Wait()
		err := cmd.Wait()
		s.mu.Lock()
		if s.busy {
			s.busy = false
			msg := ""
			for i := len(s.errTail) - 1; i >= 0; i-- {
				if strings.TrimSpace(s.errTail[i]) != "" {
					msg = s.errTail[i]
					break
				}
			}
			if msg == "" {
				code := -1
				if cmd.ProcessState != nil {
					code = cmd.ProcessState.ExitCode()
				}
				msg = fmt.Sprintf("claude exited with code %d", code)
				_ = err
			}
			s.lastError = firstN(msg, 200)
		}
		s.mu.Unlock()
		close(done)
	}()
	return nil
}

func firstN(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// Send writes one user message, starting (or restarting) the process when needed.
func (s *Sender) Send(text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if !s.Available() {
		s.mu.Lock()
		s.lastError = fmt.Sprintf("`%s` not found on PATH", s.Bin)
		err := fmt.Errorf("%s", s.lastError)
		s.mu.Unlock()
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.startLocked(); err != nil {
		s.lastError = firstN(err.Error(), 200)
		return err
	}
	s.lastError = ""
	s.busy = true
	s.sent++
	if _, err := s.stdin.Write(UserLine(text)); err != nil {
		s.busy = false
		s.lastError = firstN(err.Error(), 200)
		return err
	}
	return nil
}

// Interrupt stops the running turn: terminates the process (the next Send resumes the session).
func (s *Sender) Interrupt() {
	s.mu.Lock()
	cmd, done, alive := s.cmd, s.done, s.aliveLocked()
	s.mu.Unlock()
	if alive && cmd.Process != nil {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	s.mu.Lock()
	s.busy = false
	s.mu.Unlock()
}

// Stop ends the process (used when the app exits or switches session).
func (s *Sender) Stop() { s.Interrupt() }
