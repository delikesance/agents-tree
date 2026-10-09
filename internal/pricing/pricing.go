// Package pricing estimates API cost with prompt-cache accounting (USD per million tokens).
//
// Rates come from the Claude API model table (cached 2026-10-06). Override or extend them with a JSON
// file named by $AGENTS_TREE_PRICING: {"claude-opus-5-5": {"in": 4, "out": 20, "cache_read": 0.2}}.
// A cache read costs `cache_read` (default 0.1x input); a write costs 1.25x input (5-minute TTL) or
// 2x (1-hour TTL). Flat API rates only: no batch/fast/priority adjustments, no server-tool fees.
package pricing

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
)

const (
	Write5m     = 1.25
	Write1h     = 2.0
	ReadDefault = 0.1
)

// Rates is the price list of one model.
type Rates struct {
	In, Out   float64
	CacheRead float64 // explicit $/MTok; 0 = ReadDefault x In
	LongOver  int     // prompts above this many tokens use LongIn/LongOut (0 = never)
	LongIn    float64
	LongOut   float64
}

func (r Rates) forPrompt(prompt int) (in, out float64) {
	if r.LongOver > 0 && prompt > r.LongOver && r.LongIn > 0 {
		out = r.LongOut
		if out == 0 {
			out = r.Out
		}
		return r.LongIn, out
	}
	return r.In, r.Out
}

func (r Rates) readPrice(in float64) float64 {
	if r.CacheRead > 0 {
		return r.CacheRead
	}
	return in * ReadDefault
}

// table maps model-id prefixes to rates; the longest matching prefix wins.
var table = map[string]Rates{
	"claude-fable-5-1":  {In: 10, Out: 50, CacheRead: 0.25},
	"claude-mythos-5-1": {In: 10, Out: 50, CacheRead: 0.25},
	"claude-fable-5":    {In: 10, Out: 50, CacheRead: 1.0},
	"claude-mythos-5":   {In: 10, Out: 50, CacheRead: 1.0},
	"claude-opus-5-5":   {In: 4, Out: 20, CacheRead: 0.20},
	"claude-opus-5":     {In: 5, Out: 25},
	"claude-opus-4":     {In: 5, Out: 25},
	"claude-sonnet-5-5": {In: 2, Out: 10, CacheRead: 0.20},
	"claude-sonnet-5":   {In: 2, Out: 10},
	"claude-sonnet-4":   {In: 3, Out: 15},
	"claude-haiku-5-5":  {In: 0.10, Out: 0.50, LongOver: 100_000, LongIn: 0.50, LongOut: 2.50},
	"claude-haiku-4":    {In: 1, Out: 5},
}

func merged() map[string]Rates {
	t := make(map[string]Rates, len(table))
	for k, v := range table {
		t[k] = v
	}
	path := os.Getenv("AGENTS_TREE_PRICING")
	if path == "" {
		return t
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return t
	}
	var over map[string]struct {
		In        float64 `json:"in"`
		Out       float64 `json:"out"`
		CacheRead float64 `json:"cache_read"`
	}
	if json.Unmarshal(raw, &over) != nil {
		return t
	}
	for k, v := range over {
		t[k] = Rates{In: v.In, Out: v.Out, CacheRead: v.CacheRead}
	}
	return t
}

// Family groups a model id into opus / sonnet / haiku for colouring ("" when unknown).
func Family(model string) string {
	m := strings.ToLower(model)
	for _, f := range []string{"opus", "sonnet", "haiku", "fable", "mythos"} {
		if strings.Contains(m, f) {
			if f == "fable" || f == "mythos" {
				return "opus"
			}
			return f
		}
	}
	return ""
}

// RatesFor returns the rates of a model id (longest known prefix), if any.
func RatesFor(model string) (Rates, bool) {
	t := merged()
	keys := make([]string, 0, len(t))
	for k := range t {
		if strings.HasPrefix(model, k) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return Rates{}, false
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	return t[keys[0]], true
}

// Cost is the USD split by token category, plus what the same request would cost with no cache.
type Cost struct {
	Fresh, Write, Read, Out, NoCache float64
}

func (c Cost) Total() float64 { return c.Fresh + c.Write + c.Read + c.Out }
func (c Cost) Saved() float64 { return c.NoCache - c.Total() }

func (c *Cost) Add(o Cost) {
	c.Fresh += o.Fresh
	c.Write += o.Write
	c.Read += o.Read
	c.Out += o.Out
	c.NoCache += o.NoCache
}

// TurnCost prices one request. ok is false when the model's price is unknown: never guess.
func TurnCost(model string, fresh, write5m, write1h, read, out int) (c Cost, ok bool) {
	r, ok := RatesFor(model)
	if !ok {
		return Cost{}, false
	}
	prompt := fresh + write5m + write1h + read
	in, outp := r.forPrompt(prompt)
	const per = 1.0 / 1_000_000
	c.Fresh = float64(fresh) * in * per
	c.Write = (float64(write5m)*Write5m + float64(write1h)*Write1h) * in * per
	c.Read = float64(read) * r.readPrice(in) * per
	c.Out = float64(out) * outp * per
	c.NoCache = (float64(prompt)*in + float64(out)*outp) * per
	return c, true
}
