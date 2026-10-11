export const MANUAL_MARKER = '<!-- manual -->'

const MANUAL_MARKER_LINE = /^<!-- manual -->[ \t]*$/m
const EMPTY_MANUAL = `${MANUAL_MARKER}\n## Pitfalls\n`

export const manualBlock = (existing?: string) => {
  const start = existing?.search(MANUAL_MARKER_LINE) ?? -1
  return existing && start >= 0 ? existing.slice(start) : EMPTY_MANUAL
}
