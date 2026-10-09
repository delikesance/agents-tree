# agents-tree

TUI that visualises Claude Code behaviour as an agent tree: main session, subagents
(worker / explorer / researcher / dispatcher), advisor calls, JEV forks, tokens and cost.

```
pip install -e '.[dev]'
agents-tree live                     # follow the latest session of the current project
agents-tree replay session.jsonl     # replay a past session (space: pause, +/-: speed)
agents-tree live --permissions plan  # what Claude may do when you message it: all (default) | accept-edits | plan
agents-tree live --no-send           # read-only, no message box
```

Optional real-time hooks: add `python3 /path/to/hooks/emit.py` as a `SubagentStart` and
`SubagentStop` hook command in `~/.claude/settings.json`.

## Chat

The default view is a chat: one box per message (you, Claude, tool calls on one line, delegations to
subagents and their reports), with a rail of the agents running right now.
`i`/enter write a message, `esc` back to the chat, `f` filter by agent, `e` unfold long messages,
`t` agent tree view, `h` show finished agents in the rail, `s` switch session, `ctrl+k` stop Claude, `q` quit.

Messages you type are sent through a `claude -p` process that resumes the followed session. By default it runs
with `--dangerously-skip-permissions` (nothing can ask for confirmation in that mode); use
`--permissions accept-edits` or `plan` to restrict it. Do not use the same session from an interactive
`claude` at the same time.
