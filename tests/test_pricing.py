import pytest

from agents_tree import pricing
from agents_tree.model import MAIN, TURN, Event
from agents_tree.store import Store

M = 1_000_000


def cost(model, fresh=0, w5=0, w1=0, read=0, out=0):
    return pricing.turn_cost(model, fresh, w5, w1, read, out)


def test_opus_55_rates_and_cache_multipliers():
    assert cost("claude-opus-5-5", fresh=M).fresh == pytest.approx(4.0)
    assert cost("claude-opus-5-5", out=M).out == pytest.approx(20.0)
    assert cost("claude-opus-5-5", read=M).read == pytest.approx(0.20)       # 0.05x on this model
    assert cost("claude-opus-5-5", w5=M).write == pytest.approx(5.0)         # 1.25x
    assert cost("claude-opus-5-5", w1=M).write == pytest.approx(8.0)         # 2x


def test_model_specific_cache_read_prices():
    assert cost("claude-sonnet-5-5", read=M).read == pytest.approx(0.20)
    assert cost("claude-fable-5-1", read=M).read == pytest.approx(0.25)
    assert cost("claude-fable-5", read=M).read == pytest.approx(1.0)
    assert cost("claude-opus-5", read=M).read == pytest.approx(0.5)          # default 0.1x of $5


def test_longest_prefix_and_dated_ids():
    assert pricing.rates_for("claude-opus-5-5-20260101").inp == 4
    assert pricing.rates_for("claude-opus-5").inp == 5


def test_haiku_55_long_prompt_tier_applies_to_the_whole_request():
    short = cost("claude-haiku-5-5", fresh=50_000, out=10_000)
    assert short.total == pytest.approx(50_000 * 0.10 / M + 10_000 * 0.50 / M)
    long = cost("claude-haiku-5-5", fresh=50_000, read=150_000, out=10_000)   # 200K prompt
    assert long.fresh == pytest.approx(50_000 * 0.50 / M)
    assert long.out == pytest.approx(10_000 * 2.50 / M)


def test_unknown_model_is_not_priced():
    assert cost("some-other-model", fresh=M) is None


def test_nocache_baseline_and_savings():
    c = cost("claude-sonnet-5-5", fresh=1_000, read=99_000, out=500)
    assert c.nocache == pytest.approx((100_000 * 2 + 500 * 10) / M)
    assert c.saved == pytest.approx(c.nocache - c.total) and c.saved > 0


def usage(**kw):
    return Event(1, TURN, MAIN, data={"model": kw.pop("model", "claude-opus-5-5"), "usage": kw})


def test_store_splits_cache_ttls_and_hit_rate():
    s = Store()
    s.apply(usage(input_tokens=1_000, output_tokens=200, cache_read_input_tokens=90_000,
                  cache_creation_input_tokens=9_000,
                  cache_creation={"ephemeral_1h_input_tokens": 6_000, "ephemeral_5m_input_tokens": 3_000}))
    sm = s.summary()
    assert (sm.fresh, sm.write, sm.read, sm.out) == (1_000, 9_000, 90_000, 200)
    assert sm.hit_rate == pytest.approx(0.9)
    expect = (1_000 * 4 + 3_000 * 4 * 1.25 + 6_000 * 4 * 2 + 90_000 * 0.20 + 200 * 20) / M
    assert sm.cost.total == pytest.approx(expect)
    assert s.nodes[MAIN].cost == pytest.approx(expect)


def test_unsplit_cache_write_counts_as_5m():
    s = Store()
    s.apply(usage(cache_creation_input_tokens=1_000))
    assert s.summary().cost.write == pytest.approx(1_000 * 4 * 1.25 / M)


def test_unpriced_turns_are_excluded_and_counted():
    s = Store()
    s.apply(usage(model="mystery-model", input_tokens=500))
    sm = s.summary()
    assert sm.unpriced_turns == 1 and sm.cost.total == 0 and sm.prompt == 500
    assert s.nodes[MAIN].cost_parts is None


def test_cost_panel_and_boxes_show_cache_hit():
    from agents_tree.ui import render
    s = Store()
    s.apply(usage(input_tokens=100, output_tokens=50, cache_read_input_tokens=900))
    panel = render.cost_view(s)
    text = panel.renderable.plain
    assert "cache hit" in text and "90.0%" in text and "saved" in text
    box = render.tree_view(s, 100, 0, None, True).plain
    assert "cache 90%" in box
