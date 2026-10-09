import asyncio
from pathlib import Path

from agents_tree.sources.replay import Replay
from agents_tree.sources.transcript import read_session
from agents_tree.ui.app import AgentsTreeApp

FIX = Path(__file__).parent / "fixtures" / "session.jsonl"


def test_replay_app_renders_tree():
    async def run():
        app = AgentsTreeApp(replay=Replay(read_session(FIX), speed=64, max_gap=0.1))
        async with app.run_test(size=(120, 45)) as pilot:
            for _ in range(20):
                await pilot.pause(0.1)
            assert app.replay.done
            kinds = {n.kind for n in app.store.nodes.values()}
            assert {"main", "explorer", "worker"} <= kinds
            await pilot.press("space")
            return app.export_screenshot()
    svg = asyncio.run(run())
    assert "explorer" in svg and "worker" in svg
