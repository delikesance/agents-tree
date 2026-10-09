// Package model holds the plain data types shared by every other package.
package model

import "github.com/delikesance/agents-tree/internal/pricing"

// Main is the node id of the followed session itself.
const Main = "main"

// Event kinds produced by sources and consumed by store.Store.Apply.
const (
	AgentStart = "agent_start"
	AgentEnd   = "agent_end"
	Alias      = "alias"
	Turn       = "turn"
	Advisor    = "advisor"
	Jev        = "jev"
	Log        = "log"
	Message_   = "message"
	MsgUpdate  = "msg_update"
)

// Usage is the token usage of one API request.
type Usage struct {
	Input         int
	CacheCreation int
	Cache1h       int // part of CacheCreation written with the 1-hour TTL
	CacheRead     int
	Output        int
}

// Event is a normalized fact about a session. Only the fields that belong to its Kind are set.
type Event struct {
	TS       float64
	Kind     string
	AgentID  string
	ParentID string

	// AgentStart
	AgentKind, Desc, Model, Effort, MatchKind string
	// AgentEnd / MsgUpdate
	Status string
	// Alias
	AliasID string
	// Turn
	Usage        *Usage
	Tool         string
	Dup          bool
	Sidechain    bool
	AdvisorModel string
	// Advisor
	Advice  string
	NoCount bool
	// Jev
	Decision   string
	Confidence *float64
	Escalate   bool
	// Log
	Text string
	// Message / MsgUpdate
	Msg         *Message
	MsgID       string
	MsgDuration *float64
}

// Message is one box in the chat. Roles: user, assistant, tool, delegation, report, system.
type Message struct {
	ID       string
	TS       float64
	AgentID  string
	Role     string
	Text     string
	Tool     string
	Detail   string
	Status   string
	Duration *float64
	Kind     string
	Desc     string
	Model    string
	Target   string
	Rev      int
}

// AgentNode is one agent in the tree.
type AgentNode struct {
	ID, Kind, Parent, Model, Effort, Status, Desc string
	Turns                                         int
	TokensIn, TokensOut                           int
	Fresh, CacheWrite, CacheRead                  int
	Cost                                          float64
	CostParts                                     *pricing.Cost // nil until a priced turn is seen
	UnpricedTurns                                 int
	Started                                       float64
	Ended                                         *float64
	LastActive                                    float64
	Activity                                      string
	Children                                      []string
	Aliases                                       map[string]bool
}

// AdvisorStats and JevStats summarise advisor calls and JEV forks.
type AdvisorStats struct {
	Model      string
	Calls      int
	LastAdvice string
}

type JevRow struct {
	Count         int
	ConfidenceSum float64
	Escalated     int
}

type JevStats struct {
	Forks     int
	Decisions map[string]*JevRow
	Order     []string
}

func (j *JevStats) AvgConfidence(decision string) (float64, bool) {
	r, ok := j.Decisions[decision]
	if !ok || r.Count == 0 {
		return 0, false
	}
	return r.ConfidenceSum / float64(r.Count), true
}
