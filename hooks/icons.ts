const TOOL_ICONS: Record<string, string> = {
  Bash: '',
  Read: '',
  Edit: '',
  Write: '',
  NotebookEdit: '',
  Grep: '',
  Glob: '',
  WebFetch: '',
  WebSearch: '',
  Agent: '',
  Task: '',
  TodoWrite: '',
  SubagentHandback: '',
  SendMessage: '',
  Monitor: '',
  Skill: '',
  ToolSearch: '',
}
const DEFAULT_ICON = ''
const ERROR_ICON = ''

export const toolIcon = (tool: string) => TOOL_ICONS[tool] ?? DEFAULT_ICON
export const errorIcon = () => ERROR_ICON

export const ICONS = {
  context: '',
  quotas: '',
  agents: '',
  reset: '',
  cost: '',
  goal: '',
  tokens: '',
  requests: '',
  download: '',
  upload: '',
  cache: '',
  saved: '',
  warning: '',
  done: '',
  running: '',
  failed: '',
  pending: '',
  prompt: '',
  prompt_user: '',
  assistant: '',
  steps: '',
  back: '',
} as const
