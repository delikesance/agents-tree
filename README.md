# agents-tree

TUI that visualises Claude Code behaviour as an agent tree: main session, subagents
(worker / explorer / researcher / dispatcher), advisor calls, JEV forks, tokens and cost.

```
pip install -e '.[dev]'
agents-tree live                     # follow the latest session of the current project
agents-tree replay session.jsonl     # replay a past session (space: pause, +/-: speed)
```

Optional real-time hooks: add `python3 /path/to/hooks/emit.py` as a `SubagentStart` and
`SubagentStop` hook command in `~/.claude/settings.json`.
