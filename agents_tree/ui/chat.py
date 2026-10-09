"""Scrolling chat made of one box per message."""
from __future__ import annotations

from textual.containers import VerticalScroll
from textual.widgets import Static

from ..model import MAIN, Message
from ..store import Store
from .messages import message_renderable, message_signature

MAX_MOUNTED = 400      # widgets kept alive; older messages stay in the Store but are not drawn


class ChatMessage(Static):
    DEFAULT_CSS = """
    ChatMessage { height: auto; width: 1fr; margin: 0 0 1 0; }
    ChatMessage.compact { margin: 0; }
    """

    def __init__(self, renderable, msg_id: str, sig, role: str = "") -> None:
        super().__init__(renderable, classes="compact" if role in ("tool", "system") else "")
        self.msg_id, self.sig = msg_id, sig


class ChatView(VerticalScroll):
    can_focus = True

    def __init__(self, **kw) -> None:
        super().__init__(**kw)
        self._widgets: dict[str, ChatMessage] = {}
        self.filter: str | None = None      # None = everyone, else an agent node id (or "main")
        self.expanded = False
        self.follow = True                  # stick to the newest message

    # -- state ------------------------------------------------------------
    def reset(self) -> None:
        """Drop every drawn message; the next sync redraws from the Store."""
        self.remove_children()
        self._widgets.clear()
        self.follow = True

    def passes(self, store: Store, m: Message) -> bool:
        if self.filter is None:
            return True
        return store.resolve(m.agent_id) == self.filter or (bool(m.target) and store.resolve(m.target) == self.filter)

    def visible_messages(self, store: Store) -> list[Message]:
        return [m for m in store.messages if self.passes(store, m)][-MAX_MOUNTED:]

    # -- rendering --------------------------------------------------------
    def sync(self, store: Store, now: float | None, frame: int) -> None:
        """Mount new messages, re-render changed ones, unmount those that scrolled out of the window."""
        at_end = self.scroll_y >= self.max_scroll_y - 2
        if at_end:
            self.follow = True
        wanted = self.visible_messages(store)
        wanted_ids = {m.id for m in wanted}
        for mid in [i for i in self._widgets if i not in wanted_ids]:
            self._widgets.pop(mid).remove()
        new: list[ChatMessage] = []
        for m in wanted:
            sig = message_signature(m, store, now, self.expanded, frame)
            w = self._widgets.get(m.id)
            if w is None:
                w = ChatMessage(message_renderable(m, store, now, self.expanded, frame), m.id, sig, m.role)
                self._widgets[m.id] = w
                new.append(w)
            elif w.sig != sig:
                w.sig = sig
                w.update(message_renderable(m, store, now, self.expanded, frame))
        if new:
            self.mount(*new)
        if (new or self.follow) and self.follow:
            self.call_after_refresh(self.scroll_end, animate=False)

    def on_mouse_scroll_up(self) -> None:
        self.follow = False

    def key_pageup(self) -> None:
        self.follow = False
        self.scroll_page_up()

    def key_up(self) -> None:
        self.follow = False
        self.scroll_up()

    def key_home(self) -> None:
        self.follow = False
        self.scroll_home()

    def key_end(self) -> None:
        self.follow = True
        self.scroll_end(animate=False)
