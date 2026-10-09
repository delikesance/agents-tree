// Package replay plays a recorded list of events against the clock, with speed control.
package replay

import (
	"sort"

	"github.com/delikesance/agents-tree/internal/model"
)

type Replay struct {
	Events []model.Event
	Speed  float64
	MaxGap float64
	Paused bool
	Pos    int
	clock  float64
}

func New(events []model.Event, speed float64) *Replay {
	ev := append([]model.Event(nil), events...)
	sort.SliceStable(ev, func(i, j int) bool { return ev[i].TS < ev[j].TS })
	r := &Replay{Events: ev, Speed: speed, MaxGap: 2.0}
	if len(ev) > 0 {
		r.clock = ev[0].TS
	}
	return r
}

func (r *Replay) Done() bool { return r.Pos >= len(r.Events) }

// Tick advances virtual time by dt real seconds and returns the events now due. Long idle gaps are
// collapsed to MaxGap so replays stay watchable.
func (r *Replay) Tick(dt float64) []model.Event {
	if r.Paused || r.Done() {
		return nil
	}
	r.clock += dt * r.Speed
	var out []model.Event
	for !r.Done() {
		ev := r.Events[r.Pos]
		if ev.TS > r.clock {
			if gap := ev.TS - r.clock; gap > r.MaxGap {
				r.clock = ev.TS - r.MaxGap
			}
			break
		}
		out = append(out, ev)
		r.Pos++
	}
	return out
}
