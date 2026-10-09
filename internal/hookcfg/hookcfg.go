// Package hookcfg installs and removes agents-tree's Claude Code hooks in a settings.json,
// preserving every unknown key and the order of existing hooks.
package hookcfg

import (
	"errors"
	"fmt"
	"path"
	"strings"
)

// Entry is one hook handler that agents-tree manages.
type Entry struct {
	Event   string
	Matcher string // empty: no matcher key
	Sub     string // agents-tree subcommand ("compress" or "hook")
	Timeout int    // seconds
}

// Command is the command string written for a new install.
func (e Entry) Command() string { return "agents-tree " + e.Sub }

// Managed lists the entries install adds.
func Managed() []Entry {
	return []Entry{
		{Event: "PostToolUse", Matcher: "Read|Grep|Glob|WebFetch|WebSearch|mcp__.*", Sub: "compress", Timeout: 10},
		{Event: "SubagentStart", Sub: "hook", Timeout: 5},
		{Event: "SubagentStop", Sub: "hook", Timeout: 5},
	}
}

// IsOurs reports whether a hook command is `agents-tree <sub>` (optionally with a path to the
// binary), which is how our entries are recognised.
func IsOurs(command, sub string) bool {
	f := strings.Fields(command)
	return len(f) == 2 && path.Base(f[0]) == "agents-tree" && f[1] == sub
}

// IsRTK reports whether a hook command invokes `rtk hook`.
func IsRTK(command string) bool { return strings.Contains(command, "rtk hook") }

// Result is the outcome of Install or Uninstall.
type Result struct {
	Out     string // new file content (equal to the input when nothing changed)
	Changed bool
}

// ErrShape is returned when the settings have an unexpected structure; nothing is changed.
var ErrShape = errors.New("unexpected settings structure")

func shapeErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrShape, fmt.Sprintf(format, a...))
}

// hooksObj returns the "hooks" object, optionally creating it.
func hooksObj(root *obj, create bool) (*obj, error) {
	v, ok := root.get("hooks")
	if !ok {
		if !create {
			return nil, nil
		}
		h := newObj()
		root.set("hooks", h)
		return h, nil
	}
	h, ok := v.(*obj)
	if !ok {
		return nil, shapeErr(`"hooks" is not an object`)
	}
	return h, nil
}

// handlers walks event -> groups -> handlers and calls fn for each handler object.
// A malformed event (not an array) is an error.
func eventGroups(hooks *obj, event string) ([]any, bool, error) {
	v, ok := hooks.get(event)
	if !ok {
		return nil, false, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, true, shapeErr(`hooks.%s is not an array`, event)
	}
	return arr, true, nil
}

func groupHandlers(g any) (*obj, []any) {
	go_, ok := g.(*obj)
	if !ok {
		return nil, nil
	}
	v, _ := go_.get("hooks")
	hs, _ := v.([]any)
	return go_, hs
}

func handlerCommand(h any) string {
	o, ok := h.(*obj)
	if !ok {
		return ""
	}
	c, _ := o.get("command")
	s, _ := c.(string)
	return s
}

func hasOurs(groups []any, sub string) bool {
	for _, g := range groups {
		_, hs := groupHandlers(g)
		for _, h := range hs {
			if IsOurs(handlerCommand(h), sub) {
				return true
			}
		}
	}
	return false
}

// Install adds the managed entries that are missing.
func Install(src []byte) (Result, error) {
	root, err := parseRoot(src)
	if err != nil {
		return Result{}, err
	}
	hooks, err := hooksObj(root, false)
	if err != nil {
		return Result{}, err
	}
	// validate the events we touch before changing anything
	for _, e := range Managed() {
		if hooks != nil {
			if _, _, err := eventGroups(hooks, e.Event); err != nil {
				return Result{}, err
			}
		}
	}
	changed := false
	for _, e := range Managed() {
		if hooks == nil {
			hooks, _ = hooksObj(root, true)
		}
		groups, _, _ := eventGroups(hooks, e.Event)
		if hasOurs(groups, e.Sub) {
			continue
		}
		handler := newObj()
		handler.set("type", "command")
		handler.set("command", e.Command())
		handler.set("timeout", e.Timeout)
		group := newObj()
		if e.Matcher != "" {
			group.set("matcher", e.Matcher)
		}
		group.set("hooks", []any{handler})
		hooks.set(e.Event, append(groups, group))
		changed = true
	}
	if !changed {
		return Result{Out: string(src)}, nil
	}
	return Result{Out: render(root, detectIndent(src)), Changed: true}, nil
}

// Uninstall removes only our handlers and prunes containers that this empties.
func Uninstall(src []byte) (Result, error) {
	root, err := parseRoot(src)
	if err != nil {
		return Result{}, err
	}
	hooks, err := hooksObj(root, false)
	if err != nil {
		return Result{}, err
	}
	if hooks == nil {
		return Result{Out: string(src)}, nil
	}
	for _, e := range Managed() {
		if _, _, err := eventGroups(hooks, e.Event); err != nil {
			return Result{}, err
		}
	}
	changed := false
	seen := map[string]bool{}
	for _, e := range Managed() {
		if seen[e.Event] {
			continue
		}
		seen[e.Event] = true
		groups, _, _ := eventGroups(hooks, e.Event)
		var keptGroups []any
		removedAny := false
		for _, g := range groups {
			go_, hs := groupHandlers(g)
			if go_ == nil || hs == nil {
				keptGroups = append(keptGroups, g)
				continue
			}
			var kept []any
			removed := false
			for _, h := range hs {
				if isManaged(handlerCommand(h), e.Event) {
					removed = true
					continue
				}
				kept = append(kept, h)
			}
			if !removed {
				keptGroups = append(keptGroups, g)
				continue
			}
			removedAny = true
			if len(kept) > 0 {
				go_.set("hooks", kept)
				keptGroups = append(keptGroups, go_)
			}
		}
		if !removedAny {
			continue
		}
		changed = true
		if len(keptGroups) == 0 {
			hooks.del(e.Event)
		} else {
			hooks.set(e.Event, keptGroups)
		}
	}
	if !changed {
		return Result{Out: string(src)}, nil
	}
	if len(hooks.keys) == 0 {
		root.del("hooks")
	}
	return Result{Out: render(root, detectIndent(src)), Changed: true}, nil
}

func isManaged(command, event string) bool {
	for _, e := range Managed() {
		if e.Event == event && IsOurs(command, e.Sub) {
			return true
		}
	}
	return false
}

// EntryStatus is the state of one managed entry.
type EntryStatus struct {
	Entry
	Installed bool
}

// StatusReport is what `hooks status` shows about a settings document.
type StatusReport struct {
	Entries []EntryStatus
	RTKHook bool // some handler runs `rtk hook`
}

// Status inspects a settings document without changing it.
func Status(src []byte) (StatusReport, error) {
	var rep StatusReport
	root, err := parseRoot(src)
	if err != nil {
		return rep, err
	}
	hooks, err := hooksObj(root, false)
	if err != nil {
		return rep, err
	}
	for _, e := range Managed() {
		st := EntryStatus{Entry: e}
		if hooks != nil {
			groups, _, err := eventGroups(hooks, e.Event)
			if err != nil {
				return rep, err
			}
			st.Installed = hasOurs(groups, e.Sub)
		}
		rep.Entries = append(rep.Entries, st)
	}
	if hooks != nil {
		for _, ev := range hooks.keys {
			groups, _ := hooks.m[ev].([]any)
			for _, g := range groups {
				_, hs := groupHandlers(g)
				for _, h := range hs {
					if IsRTK(handlerCommand(h)) {
						rep.RTKHook = true
					}
				}
			}
		}
	}
	return rep, nil
}
