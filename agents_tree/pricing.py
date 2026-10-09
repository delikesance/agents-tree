"""Per-request cost with prompt-cache accounting, in USD.

Rates are USD per million tokens, from the Claude API model table (cached 2026-10-06).
Override or extend them with a JSON file pointed to by $AGENTS_TREE_PRICING:
  {"claude-opus-5-5": {"in": 4, "out": 20, "cache_read": 0.2}}
Cache rules: a read costs `cache_read` (default 0.1x input); a write costs 1.25x input
for the 5-minute TTL and 2x for the 1-hour TTL. Costs remain estimates (flat API rates:
no batch/fast/priority discounts or premiums, no server-tool fees).
"""
from __future__ import annotations

import json
import os
from dataclasses import dataclass

WRITE_5M, WRITE_1H, READ_DEFAULT = 1.25, 2.0, 0.1


@dataclass(frozen=True)
class Rates:
    inp: float
    out: float
    cache_read: float | None = None          # explicit $/MTok, else READ_DEFAULT x inp
    long_over: int | None = None             # prompts above this many tokens use the long_* rates
    long_inp: float | None = None
    long_out: float | None = None

    def for_prompt(self, prompt_tokens: int) -> tuple[float, float]:
        if self.long_over and prompt_tokens > self.long_over and self.long_inp is not None:
            return self.long_inp, self.long_out if self.long_out is not None else self.out
        return self.inp, self.out

    def read_price(self, inp: float) -> float:
        return self.cache_read if self.cache_read is not None else inp * READ_DEFAULT


# Longest matching prefix wins (so "claude-opus-5-5" beats "claude-opus-5").
RATES: dict[str, Rates] = {
    "claude-fable-5-1": Rates(10, 50, 0.25), "claude-mythos-5-1": Rates(10, 50, 0.25),
    "claude-fable-5": Rates(10, 50, 1.0), "claude-mythos-5": Rates(10, 50, 1.0),
    "claude-opus-5-5": Rates(4, 20, 0.20),
    "claude-opus-5": Rates(5, 25), "claude-opus-4": Rates(5, 25),
    "claude-sonnet-5-5": Rates(2, 10, 0.20), "claude-sonnet-5": Rates(2, 10),
    "claude-sonnet-4": Rates(3, 15),
    "claude-haiku-5-5": Rates(0.10, 0.50, None, 100_000, 0.50, 2.50),
    "claude-haiku-4": Rates(1, 5),
}


def _table() -> dict[str, Rates]:
    table = dict(RATES)
    path = os.environ.get("AGENTS_TREE_PRICING")
    if path:
        try:
            with open(path) as f:
                for key, v in json.load(f).items():
                    table[key] = Rates(v["in"], v["out"], v.get("cache_read"))
        except (OSError, ValueError, KeyError, TypeError):
            pass
    return table


def family(model: str) -> str:
    m = model.lower()
    for fam in ("opus", "sonnet", "haiku", "fable", "mythos"):
        if fam in m:
            return {"fable": "opus", "mythos": "opus"}.get(fam, fam)   # colour group only
    return ""


def rates_for(model: str) -> Rates | None:
    table = _table()
    best = max((k for k in table if model.startswith(k)), key=len, default=None)
    return table[best] if best else None


@dataclass
class Cost:
    """USD split by token category, plus what the same request would cost with no cache."""
    fresh: float = 0.0
    write: float = 0.0
    read: float = 0.0
    out: float = 0.0
    nocache: float = 0.0

    @property
    def total(self) -> float:
        return self.fresh + self.write + self.read + self.out

    @property
    def saved(self) -> float:
        return self.nocache - self.total

    def add(self, o: "Cost") -> None:
        self.fresh += o.fresh
        self.write += o.write
        self.read += o.read
        self.out += o.out
        self.nocache += o.nocache


def turn_cost(model: str, fresh: int, write_5m: int, write_1h: int, read: int, out: int) -> Cost | None:
    """Cost of one request, or None when the model's price is unknown (never guess)."""
    r = rates_for(model)
    if r is None:
        return None
    prompt = fresh + write_5m + write_1h + read
    inp, outp = r.for_prompt(prompt)
    per = 1 / 1_000_000
    c = Cost(fresh=fresh * inp * per,
             write=(write_5m * WRITE_5M + write_1h * WRITE_1H) * inp * per,
             read=read * r.read_price(inp) * per,
             out=out * outp * per)
    c.nocache = (prompt * inp + out * outp) * per
    return c
