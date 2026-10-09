package pricing

import (
	"math"
	"testing"
)

const M = 1_000_000

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func cost(t *testing.T, model string, fresh, w5, w1, read, out int) Cost {
	t.Helper()
	c, ok := TurnCost(model, fresh, w5, w1, read, out)
	if !ok {
		t.Fatalf("no price for %s", model)
	}
	return c
}

func TestOpus55RatesAndCacheMultipliers(t *testing.T) {
	if c := cost(t, "claude-opus-5-5", M, 0, 0, 0, 0); !near(c.Fresh, 4) {
		t.Errorf("fresh = %v", c.Fresh)
	}
	if c := cost(t, "claude-opus-5-5", 0, 0, 0, 0, M); !near(c.Out, 20) {
		t.Errorf("out = %v", c.Out)
	}
	if c := cost(t, "claude-opus-5-5", 0, 0, 0, M, 0); !near(c.Read, 0.20) {
		t.Errorf("read = %v (0.05x on this model)", c.Read)
	}
	if c := cost(t, "claude-opus-5-5", 0, M, 0, 0, 0); !near(c.Write, 5) {
		t.Errorf("5m write = %v", c.Write)
	}
	if c := cost(t, "claude-opus-5-5", 0, 0, M, 0, 0); !near(c.Write, 8) {
		t.Errorf("1h write = %v", c.Write)
	}
}

func TestModelSpecificCacheReadPrices(t *testing.T) {
	for model, want := range map[string]float64{
		"claude-sonnet-5-5": 0.20, "claude-fable-5-1": 0.25, "claude-fable-5": 1.0, "claude-opus-5": 0.5,
	} {
		if c := cost(t, model, 0, 0, 0, M, 0); !near(c.Read, want) {
			t.Errorf("%s read = %v, want %v", model, c.Read, want)
		}
	}
}

func TestLongestPrefixAndDatedIDs(t *testing.T) {
	if r, _ := RatesFor("claude-opus-5-5-20260101"); r.In != 4 {
		t.Errorf("dated id: %v", r)
	}
	if r, _ := RatesFor("claude-opus-5"); r.In != 5 {
		t.Errorf("opus 5: %v", r)
	}
}

func TestHaiku55LongPromptTierAppliesToTheWholeRequest(t *testing.T) {
	short := cost(t, "claude-haiku-5-5", 50_000, 0, 0, 0, 10_000)
	if !near(short.Total(), 50_000*0.10/M+10_000*0.50/M) {
		t.Errorf("short = %v", short.Total())
	}
	long := cost(t, "claude-haiku-5-5", 50_000, 0, 0, 150_000, 10_000)
	if !near(long.Fresh, 50_000*0.50/M) || !near(long.Out, 10_000*2.50/M) {
		t.Errorf("long = %+v", long)
	}
}

func TestUnknownModelIsNotPriced(t *testing.T) {
	if _, ok := TurnCost("some-other-model", M, 0, 0, 0, 0); ok {
		t.Error("unknown model must not be priced")
	}
}

func TestNoCacheBaselineAndSavings(t *testing.T) {
	c := cost(t, "claude-sonnet-5-5", 1_000, 0, 0, 99_000, 500)
	if !near(c.NoCache, (100_000*2.0+500*10.0)/M) || !(c.Saved() > 0) {
		t.Errorf("%+v saved=%v", c, c.Saved())
	}
}

func TestFamily(t *testing.T) {
	for in, want := range map[string]string{"claude-fable-5-1": "opus", "claude-haiku-5-5": "haiku", "x": ""} {
		if got := Family(in); got != want {
			t.Errorf("Family(%q) = %q", in, got)
		}
	}
}
