from __future__ import annotations

import time
from pathlib import Path

from textual.app import ComposeResult
from textual.containers import Vertical
from textual.screen import ModalScreen
from textual.widgets import DataTable, Input, Label

from ..sources.live import SessionInfo


def ago(ts: float, now: float | None = None) -> str:
    d = max(0, (now or time.time()) - ts)
    for unit, size in (("d", 86400), ("h", 3600), ("min", 60)):
        if d >= size:
            return f"{int(d // size)}{unit} ago" if unit != "min" else f"{int(d // 60)} min ago"
    return "just now"


def project_label(name: str, cwd: str = "") -> str:
    """Project directory name; falls back to a lossy decode of '-home-user-agents-tree'."""
    if cwd:
        return Path(cwd).name or cwd
    parts = [p for p in name.split("-") if p]
    return "/".join(parts[-2:]) if len(parts) > 1 else name


class SessionPicker(ModalScreen):
    """Choose a Claude Code session; dismisses with its Path, or None when cancelled."""

    BINDINGS = [("escape", "cancel", "Cancel")]
    DEFAULT_CSS = """
    SessionPicker { align: center middle; }
    #dialog { width: 90%; height: 80%; border: round $primary; background: $surface; padding: 0 1; }
    #dialog DataTable { height: 1fr; }
    """

    def __init__(self, sessions: list[SessionInfo], current: Path | None = None) -> None:
        super().__init__()
        self.sessions, self.current = sessions, current

    def compose(self) -> ComposeResult:
        with Vertical(id="dialog"):
            yield Label("Select a session  (type to filter · enter to open · esc to cancel)")
            yield Input(placeholder="filter by project, prompt or id…")
            yield DataTable(cursor_type="row", zebra_stripes=True)

    def on_mount(self) -> None:
        table = self.query_one(DataTable)
        table.add_columns("modified", "project", "subagents", "first prompt", "id")
        self.fill("")
        table.focus()

    def fill(self, needle: str) -> None:
        table = self.query_one(DataTable)
        table.clear()
        needle = needle.lower().strip()
        for s in self.sessions:
            hay = f"{project_label(s.project, s.cwd)} {s.title} {s.id}".lower()
            if needle in hay:
                mark = "● " if self.current and s.path == self.current else ""
                table.add_row(ago(s.mtime), project_label(s.project, s.cwd), str(s.subagents),
                              mark + (s.title or "(no prompt)"), s.id[:8], key=str(s.path))

    def on_input_changed(self, event: Input.Changed) -> None:
        self.fill(event.value)

    def on_input_submitted(self, event: Input.Submitted) -> None:
        table = self.query_one(DataTable)
        if table.row_count:
            table.focus()
            self.dismiss(Path(table.coordinate_to_cell_key(table.cursor_coordinate).row_key.value))

    def on_data_table_row_selected(self, event: DataTable.RowSelected) -> None:
        self.dismiss(Path(event.row_key.value))

    def action_cancel(self) -> None:
        self.dismiss(None)
