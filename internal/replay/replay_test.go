package replay

import (
	"testing"

	"github.com/delikesance/agents-tree/internal/model"
)

func TestReplayCollapsesGapsAndPauses(t *testing.T) {
	evs := []model.Event{{TS: 0}, {TS: 1}, {TS: 500}, {TS: 501}}
	r := New(evs, 1)
	r.MaxGap = 1
	if got := r.Tick(0); len(got) != 1 || r.Done() {
		t.Fatalf("first tick = %d", len(got))
	}
	r.Paused = true
	if got := r.Tick(100); got != nil {
		t.Error("paused replay must not advance")
	}
	r.Paused = false
	for i := 0; i < 20 && !r.Done(); i++ {
		r.Tick(1)
	}
	if !r.Done() {
		t.Error("replay should finish despite the 500 s gap")
	}
}
