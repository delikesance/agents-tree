"""Replay a recorded list of Events against the clock, with speed control."""
from __future__ import annotations

from ..model import Event


class Replay:
    def __init__(self, events: list[Event], speed: float = 1.0, max_gap: float = 2.0) -> None:
        self.events = sorted(events, key=lambda e: e.ts)
        self.speed, self.max_gap = speed, max_gap
        self.paused = False
        self.pos = 0
        self._clock = self.events[0].ts if self.events else 0.0

    @property
    def done(self) -> bool:
        return self.pos >= len(self.events)

    def tick(self, dt: float) -> list[Event]:
        """Advance virtual time by dt real seconds; return events now due.

        Long idle gaps are collapsed to max_gap so replays stay watchable.
        """
        if self.paused or self.done:
            return []
        self._clock += dt * self.speed
        out: list[Event] = []
        while not self.done:
            ev = self.events[self.pos]
            if ev.ts > self._clock:
                gap = ev.ts - self._clock
                if gap > self.max_gap:
                    self._clock = ev.ts - self.max_gap
                break
            out.append(ev)
            self.pos += 1
        return out
