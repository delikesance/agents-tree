// Package store is the pure state reducer: events in, agent tree + chat + stats out. No I/O, no UI.
package store

import (
	"os"
	"strconv"

	"github.com/delikesance/agents-tree/internal/model"
	"github.com/delikesance/agents-tree/internal/pricing"
)

// MaxMessages is how many chat messages are kept (the oldest are dropped).
const MaxMessages = 2000

// StaleSecs: an agent with no event for this long is not shown as running (env AGENTS_TREE_STALE).
var StaleSecs = staleSecs()

func staleSecs() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("AGENTS_TREE_STALE"), 64); err == nil {
		return v
	}
	return 120
}

// LogLine is one entry of the event log.
type LogLine struct {
	TS   float64
	Text string
}

// Store holds the state of one followed session.
type Store struct {
	Nodes    map[string]*model.AgentNode
	Order    []string // node ids in order of appearance
	Advisor  model.AdvisorStats
	Jev      model.JevStats
	Log      []LogLine
	Messages []*model.Message
	Clock    float64 // newest event timestamp seen (virtual "now" for replays)

	msgByID      map[string]*model.Message
	alias        map[string]string
	pendingAlias map[string]string // alias seen before its node exists
}

func New() *Store {
	s := &Store{Nodes: map[string]*model.AgentNode{}, msgByID: map[string]*model.Message{},
		alias: map[string]string{}, pendingAlias: map[string]string{},
		Jev: model.JevStats{Decisions: map[string]*model.JevRow{}}}
	s.Nodes[model.Main] = &model.AgentNode{ID: model.Main, Kind: "main", Status: "running", Aliases: map[string]bool{}}
	s.Order = []string{model.Main}
	return s
}

// Resolve maps an alias (e.g. a subagent's own id) to its node id.
func (s *Store) Resolve(id string) string {
	if t, ok := s.alias[id]; ok {
		return t
	}
	return id
}

// Get returns the node an id (or alias) names, or nil.
func (s *Store) Get(id string) *model.AgentNode { return s.Nodes[s.Resolve(id)] }

func (s *Store) addAlias(n *model.AgentNode, a string) {
	n.Aliases[a] = true
	s.alias[a] = n.ID
}

// Apply folds one event into the state.
func (s *Store) Apply(ev model.Event) {
	if ev.TS != 0 {
		if ev.TS > s.Clock {
			s.Clock = ev.TS
		}
		if n := s.Get(ev.AgentID); n != nil && (ev.Kind == model.AgentStart || ev.Kind == model.Turn || ev.Kind == model.Alias) {
			if ev.TS > n.LastActive {
				n.LastActive = ev.TS
			}
		}
	}
	switch ev.Kind {
	case model.AgentStart:
		s.start(ev)
	case model.AgentEnd:
		s.end(ev)
	case model.Alias:
		s.onAlias(ev)
	case model.Turn:
		s.turn(ev)
	case model.Advisor:
		s.advisor(ev)
	case model.Jev:
		s.jev(ev)
	case model.Log:
		s.logf(ev.TS, ev.Text)
	case model.Message_:
		s.message(ev)
	case model.MsgUpdate:
		s.msgUpdate(ev)
	}
}

func (s *Store) start(ev model.Event) {
	existing := s.Get(ev.AgentID)
	if existing == nil && ev.MatchKind != "" {
		// A hook announced an agent the transcript may already know by its tool_use id.
		for _, id := range s.Order {
			n := s.Nodes[id]
			if n.Kind == ev.MatchKind && n.Status == "running" && n.ID != model.Main && len(n.Aliases) == 0 {
				existing = n
				s.addAlias(n, ev.AgentID)
				break
			}
		}
	}
	if existing != nil {
		if ev.Model != "" {
			existing.Model = ev.Model
		}
		if ev.Desc != "" {
			existing.Desc = ev.Desc
		}
		return
	}
	parent := model.Main
	if ev.ParentID != "" {
		parent = s.Resolve(ev.ParentID)
	}
	if _, ok := s.Nodes[parent]; !ok {
		parent = model.Main
	}
	kind := ev.AgentKind
	if kind == "" {
		kind = "agent"
	}
	n := &model.AgentNode{ID: ev.AgentID, Kind: kind, Parent: parent, Model: ev.Model, Effort: ev.Effort,
		Status: "running", Desc: ev.Desc, Started: ev.TS, Aliases: map[string]bool{}}
	s.Nodes[n.ID] = n
	s.Order = append(s.Order, n.ID)
	s.Nodes[parent].Children = append(s.Nodes[parent].Children, n.ID)
	if a, ok := s.pendingAlias[n.ID]; ok {
		delete(s.pendingAlias, n.ID)
		s.addAlias(n, a)
	}
	text := "+ " + n.Kind + " started"
	if n.Desc != "" {
		text += " · " + n.Desc
	}
	s.logf(ev.TS, text)
}

func (s *Store) end(ev model.Event) {
	n := s.Get(ev.AgentID)
	if n == nil || n.ID == model.Main || n.Status != "running" {
		return
	}
	n.Status = ev.Status
	if n.Status == "" {
		n.Status = "done"
	}
	ts := ev.TS
	n.Ended = &ts
	s.logf(ev.TS, "- "+n.Kind+" "+n.Status)
}

func (s *Store) onAlias(ev model.Event) {
	if n := s.Get(ev.AgentID); n != nil && ev.AliasID != "" {
		s.addAlias(n, ev.AliasID)
	} else if ev.AliasID != "" {
		s.pendingAlias[ev.AgentID] = ev.AliasID
	}
}

// claim binds an unknown sidechain id to the oldest running subagent without an alias (best effort:
// subagent transcripts can be read before the tool_result that links their id).
func (s *Store) claim(id string) *model.AgentNode {
	for _, nid := range s.Order {
		n := s.Nodes[nid]
		if n.ID != model.Main && n.Status == "running" && len(n.Aliases) == 0 {
			s.addAlias(n, id)
			return n
		}
	}
	return nil
}

func (s *Store) turn(ev model.Event) {
	n := s.Get(ev.AgentID)
	if n == nil && ev.Sidechain {
		n = s.claim(ev.AgentID)
	}
	if n == nil {
		return
	}
	if ev.Dup { // another content block of an already counted request
		if ev.Tool != "" {
			n.Activity = ev.Tool
		}
		return
	}
	u := ev.Usage
	if u == nil {
		u = &model.Usage{}
	}
	w1h := u.Cache1h
	if w1h > u.CacheCreation {
		w1h = u.CacheCreation
	}
	w5m := u.CacheCreation - w1h
	n.Turns++
	n.Fresh += u.Input
	n.CacheWrite += u.CacheCreation
	n.CacheRead += u.CacheRead
	n.TokensIn += u.Input + u.CacheCreation + u.CacheRead
	n.TokensOut += u.Output
	n.Activity = ev.Tool
	if ev.Model != "" {
		n.Model = ev.Model
	}
	if ev.Effort != "" {
		n.Effort = ev.Effort
	}
	m := ev.Model
	if m == "" {
		m = n.Model
	}
	if c, ok := pricing.TurnCost(m, u.Input, w5m, w1h, u.CacheRead, u.Output); !ok {
		n.UnpricedTurns++
	} else {
		if n.CostParts == nil {
			n.CostParts = &pricing.Cost{}
		}
		n.CostParts.Add(c)
		n.Cost = n.CostParts.Total()
	}
	if ev.AdvisorModel != "" {
		s.Advisor.Model = ev.AdvisorModel
	}
}

func (s *Store) advisor(ev model.Event) {
	if !ev.NoCount {
		s.Advisor.Calls++
		s.logf(ev.TS, "advisor called")
	}
	if ev.Model != "" {
		s.Advisor.Model = ev.Model
	}
	if ev.Advice != "" {
		s.Advisor.LastAdvice = ev.Advice
	}
}

func (s *Store) jev(ev model.Event) {
	s.Jev.Forks++
	row, ok := s.Jev.Decisions[ev.Decision]
	if !ok {
		row = &model.JevRow{}
		s.Jev.Decisions[ev.Decision] = row
		s.Jev.Order = append(s.Jev.Order, ev.Decision)
	}
	row.Count++
	if ev.Confidence != nil {
		row.ConfidenceSum += *ev.Confidence
	}
	if ev.Escalate {
		row.Escalated++
	}
}

func (s *Store) logf(ts float64, text string) {
	s.Log = append(s.Log, LogLine{ts, text})
	if len(s.Log) > 500 {
		s.Log = s.Log[len(s.Log)-500:]
	}
}

func (s *Store) message(ev model.Event) {
	m := ev.Msg
	if m == nil {
		return
	}
	if _, dup := s.msgByID[m.ID]; dup { // re-reading a line must not duplicate it
		return
	}
	s.Messages = append(s.Messages, m)
	s.msgByID[m.ID] = m
	if len(s.Messages) > MaxMessages {
		drop := len(s.Messages) - MaxMessages
		for _, old := range s.Messages[:drop] {
			delete(s.msgByID, old.ID)
		}
		s.Messages = append([]*model.Message(nil), s.Messages[drop:]...)
	}
}

func (s *Store) msgUpdate(ev model.Event) {
	m, ok := s.msgByID[ev.MsgID]
	if !ok {
		return
	}
	if ev.Status != "" {
		m.Status = ev.Status
	}
	if ev.MsgDuration != nil {
		m.Duration = ev.MsgDuration
	}
	m.Rev++
}

// ---- derived ----------------------------------------------------------------------------------------

// State is running | done | failed | stale. Only truly active agents read as running.
// now is wall-clock seconds when following a live session; 0 uses the newest event time (replays).
func (s *Store) State(n *model.AgentNode, now float64) string {
	if n.Status != "running" {
		return n.Status
	}
	ref := now
	if ref == 0 {
		ref = s.Clock
	}
	last := n.LastActive
	if last == 0 {
		last = n.Started
	}
	if last != 0 && ref-last > StaleSecs {
		return "stale"
	}
	return "running"
}

func (s *Store) alive(id string, now float64) bool {
	n := s.Nodes[id]
	if s.State(n, now) == "running" {
		return true
	}
	for _, c := range n.Children {
		if s.alive(c, now) {
			return true
		}
	}
	return false
}

// VisibleChildren are the children to draw: all of them, or only running agents plus the ancestors
// needed to keep a running agent connected to the tree.
func (s *Store) VisibleChildren(id string, now float64, showAll bool) []string {
	kids := s.Nodes[id].Children
	if showAll {
		return append([]string(nil), kids...)
	}
	var out []string
	for _, c := range kids {
		if s.alive(c, now) {
			out = append(out, c)
		}
	}
	return out
}

// RunningSubagents counts running subagents and all subagents.
func (s *Store) RunningSubagents(now float64) (running, total int) {
	for _, id := range s.Order {
		if id == model.Main {
			continue
		}
		total++
		if s.State(s.Nodes[id], now) == "running" {
			running++
		}
	}
	return
}

// Summary is the session-wide token and cost totals, split by category.
type Summary struct {
	Fresh, Write, Read, Out, Turns, Unpriced int
	Cost                                     pricing.Cost
}

func (m Summary) Prompt() int { return m.Fresh + m.Write + m.Read }

// HitRate is the share of prompt tokens served from cache (ok=false before any prompt).
func (m Summary) HitRate() (float64, bool) {
	if m.Prompt() == 0 {
		return 0, false
	}
	return float64(m.Read) / float64(m.Prompt()), true
}

func (s *Store) Summary() Summary {
	var sm Summary
	for _, id := range s.Order {
		n := s.Nodes[id]
		sm.Fresh += n.Fresh
		sm.Write += n.CacheWrite
		sm.Read += n.CacheRead
		sm.Out += n.TokensOut
		sm.Turns += n.Turns
		sm.Unpriced += n.UnpricedTurns
		if n.CostParts != nil {
			sm.Cost.Add(*n.CostParts)
		}
	}
	return sm
}

// HitRate of one agent.
func HitRate(fresh, write, read int) (float64, bool) {
	t := fresh + write + read
	if t == 0 {
		return 0, false
	}
	return float64(read) / float64(t), true
}
