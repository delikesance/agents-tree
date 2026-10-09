// Package composition answers: where do the input tokens of a session come from?
//
// Every API request re-sends the whole conversation, so a piece of content costs roughly its size
// times the number of requests that come after it. This attributes that volume to categories: the
// context already there on the first request (system prompt, tools, skills, CLAUDE.md), what the
// assistant wrote into tool calls (code, commands), what tools returned (per tool), and plain text.
// What cannot be attributed (thinking, images, attachments, estimation error) stays in Other.
//
// Sizes are estimated as characters / 4; money is the session's input cost split in proportion to
// volume. Context compaction shrinks the real context and is not modelled.
package composition

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/delikesance/agents-tree/internal/pricing"
	"github.com/delikesance/agents-tree/internal/transcript"
)

// ToolVol is the weight of one tool's calls or outputs.
type ToolVol struct {
	N      int
	Tokens float64 // estimated tokens produced once
	Volume float64 // tokens x later requests = input tokens they cause
}

// Row is one category line.
type Row struct {
	Label  string
	Volume float64
}

// Composition is the breakdown of a session (or of one conversation context).
type Composition struct {
	Turns          int
	PromptTokens   float64
	InputCost      float64
	Baseline       float64
	BaselineTokens float64
	UserText       float64
	AssistantText  float64
	Outputs        map[string]*ToolVol
	Inputs         map[string]*ToolVol
}

func New() *Composition {
	return &Composition{Outputs: map[string]*ToolVol{}, Inputs: map[string]*ToolVol{}}
}

func sum(m map[string]*ToolVol) (v float64) {
	for _, t := range m {
		v += t.Volume
	}
	return
}

func (c *Composition) OutputsVolume() float64 { return sum(c.Outputs) }
func (c *Composition) InputsVolume() float64  { return sum(c.Inputs) }

// Other is the volume that could not be attributed.
func (c *Composition) Other() float64 {
	known := c.Baseline + c.UserText + c.AssistantText + c.OutputsVolume() + c.InputsVolume()
	if o := c.PromptTokens - known; o > 0 {
		return o
	}
	return 0
}

func (c *Composition) Share(v float64) float64 {
	if c.PromptTokens == 0 {
		return 0
	}
	return v / c.PromptTokens
}

func (c *Composition) USD(v float64) float64 { return c.Share(v) * c.InputCost }

// Categories are the rows, largest first.
func (c *Composition) Categories() []Row {
	rows := []Row{
		{"context before the first message", c.Baseline},
		{"code and commands written (tool calls)", c.InputsVolume()},
		{"tool outputs", c.OutputsVolume()},
		{"your messages", c.UserText},
		{"assistant replies", c.AssistantText},
		{"other (thinking, images, files…)", c.Other()},
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Volume > rows[j].Volume })
	return rows
}

// WhatIfCompressOutputs is the share of input tokens (and USD) saved if tool outputs shrank by ratio.
func (c *Composition) WhatIfCompressOutputs(ratio float64) (share, usd float64) {
	saved := c.OutputsVolume() * ratio
	return c.Share(saved), c.USD(saved)
}

func (c *Composition) merge(o *Composition) {
	c.Turns += o.Turns
	c.PromptTokens += o.PromptTokens
	c.InputCost += o.InputCost
	c.Baseline += o.Baseline
	c.BaselineTokens += o.BaselineTokens
	c.UserText += o.UserText
	c.AssistantText += o.AssistantText
	for _, p := range [][2]map[string]*ToolVol{{c.Outputs, o.Outputs}, {c.Inputs, o.Inputs}} {
		for k, v := range p[1] {
			t := p[0][k]
			if t == nil {
				t = &ToolVol{}
				p[0][k] = t
			}
			t.N += v.N
			t.Tokens += v.Tokens
			t.Volume += v.Volume
		}
	}
}

type obj = map[string]any

func strOf(m obj, k string) string { s, _ := m[k].(string); return s }
func intOf(m obj, k string) int    { f, _ := m[k].(float64); return int(f) }

func textLen(content any) int {
	switch c := content.(type) {
	case string:
		return len([]rune(c))
	case []any:
		n := 0
		for _, b := range c {
			if m, ok := b.(obj); ok && m["type"] == "text" {
				n += len([]rune(strOf(m, "text")))
			}
		}
		return n
	}
	return 0
}

func shortName(n string) string {
	if i := strings.LastIndex(n, "__"); i >= 0 {
		return n[i+2:]
	}
	return n
}

// analyzeFile is one transcript = one conversation context (the main session, or one subagent).
func analyzeFile(path string) *Composition {
	comp := New()
	f, err := os.Open(path)
	if err != nil {
		return comp
	}
	defer f.Close()
	var rows []obj
	transcript.ReadLines(f, func(d map[string]any) { rows = append(rows, d) })

	// Pass 1: unique API requests in order, with their input size.
	seen := map[string]int{}
	var prompts []float64
	for _, d := range rows {
		m, _ := d["message"].(obj)
		u, _ := m["usage"].(obj)
		if strOf(d, "type") != "assistant" || u == nil {
			continue
		}
		key := strOf(m, "id")
		if key == "" {
			key = strOf(d, "uuid")
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = len(prompts) + 1
		fresh, write, read := intOf(u, "input_tokens"), intOf(u, "cache_creation_input_tokens"), intOf(u, "cache_read_input_tokens")
		cc, _ := u["cache_creation"].(obj)
		w1h := intOf(cc, "ephemeral_1h_input_tokens")
		if w1h > write {
			w1h = write
		}
		prompts = append(prompts, float64(fresh+write+read))
		if c, ok := pricing.TurnCost(strOf(m, "model"), fresh, write-w1h, w1h, read, 0); ok {
			comp.InputCost += c.Fresh + c.Write + c.Read
		}
	}
	comp.Turns = len(prompts)
	if comp.Turns == 0 {
		return comp
	}
	for _, p := range prompts {
		comp.PromptTokens += p
	}
	n := float64(len(prompts))
	comp.BaselineTokens = prompts[0]
	comp.Baseline = prompts[0] * n

	// Pass 2: attribute content to the requests that re-send it.
	toolName := map[string]string{}
	t := 0 // requests seen so far
	add := func(m map[string]*ToolVol, name string, tokens, later float64) {
		v := m[name]
		if v == nil {
			v = &ToolVol{}
			m[name] = v
		}
		v.N++
		v.Tokens += tokens
		v.Volume += tokens * later
	}
	for _, d := range rows {
		m, _ := d["message"].(obj)
		content := m["content"]
		switch strOf(d, "type") {
		case "assistant":
			key := strOf(m, "id")
			if key == "" {
				key = strOf(d, "uuid")
			}
			if m["usage"] != nil && seen[key] == t+1 {
				t++
			}
			later := n - float64(t)
			blocks, _ := content.([]any)
			for _, raw := range blocks {
				b, _ := raw.(obj)
				switch strOf(b, "type") {
				case "text":
					comp.AssistantText += float64(len([]rune(strOf(b, "text")))) / 4 * later
				case "tool_use", "server_tool_use":
					toolName[strOf(b, "id")] = strOf(b, "name")
					raw, _ := json.Marshal(b["input"])
					s := string(raw)
					if s == "null" {
						s = "{}"
					}
					add(comp.Inputs, shortName(strOf(b, "name")), float64(len([]rune(s)))/4, later)
				}
			}
		case "user":
			later := n - float64(t)
			var blocks []any
			if s, ok := content.(string); ok {
				blocks = []any{obj{"type": "text", "text": s}}
			} else {
				blocks, _ = content.([]any)
			}
			for _, raw := range blocks {
				b, _ := raw.(obj)
				switch strOf(b, "type") {
				case "text":
					comp.UserText += float64(len([]rune(strOf(b, "text")))) / 4 * later
				case "tool_result":
					name := toolName[strOf(b, "tool_use_id")]
					if name == "" {
						name = "?"
					}
					add(comp.Outputs, shortName(name), float64(textLen(b["content"]))/4, later)
				}
			}
		}
	}
	return comp
}

// Analyze is the composition of a whole session: the main conversation plus each subagent's own.
func Analyze(sessionJSONL string, subagents bool) *Composition {
	total := New()
	for _, f := range transcript.SessionFiles(sessionJSONL) {
		if f.Sidechain && !subagents {
			continue
		}
		if _, err := os.Stat(f.Path); err == nil {
			total.merge(analyzeFile(f.Path))
		}
	}
	return total
}
