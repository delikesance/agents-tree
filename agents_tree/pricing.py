"""Rough $ estimation per model, in USD per million tokens.

These are estimates: override them with a JSON file pointed to by
$AGENTS_TREE_PRICING, e.g. {"haiku": [1.0, 5.0]} ([input, output] per family key).
"""
from __future__ import annotations

import json
import os

DEFAULT = {"opus": (5.0, 25.0), "sonnet": (3.0, 15.0), "haiku": (1.0, 5.0)}


def _table() -> dict[str, tuple[float, float]]:
    table = dict(DEFAULT)
    path = os.environ.get("AGENTS_TREE_PRICING")
    if path:
        try:
            with open(path) as f:
                table.update({k: tuple(v) for k, v in json.load(f).items()})
        except (OSError, ValueError):
            pass
    return table


def family(model: str) -> str:
    m = model.lower()
    for fam in ("opus", "sonnet", "haiku"):
        if fam in m:
            return fam
    return ""


# Prompt-cache multipliers on the input price (estimates: write 1.25x, read 0.1x).
CACHE_WRITE, CACHE_READ = 1.25, 0.1


def cost(model: str, tokens_in: int, tokens_out: int, cache_write: int = 0, cache_read: int = 0) -> float:
    price = _table().get(family(model))
    if not price:
        return 0.0
    inp = tokens_in + cache_write * CACHE_WRITE + cache_read * CACHE_READ
    return (inp * price[0] + tokens_out * price[1]) / 1_000_000
