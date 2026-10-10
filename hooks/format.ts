import type { AgentUsage, GoalProgress } from '../types'

import { ICONS } from './icons'
import { PALETTE } from './palette'
import { segmentColors } from './segmentColors'

const TOKENS_PER_K = 1_000
const TOKENS_PER_M = 1_000_000
const compact = (value: number) => (value >= 10 ? String(Math.round(value)) : value.toFixed(1).replace(/\.0$/, ''))
export const formatTokens = (n: number) => (n >= TOKENS_PER_M ? `${compact(n / TOKENS_PER_M)}M` : n >= TOKENS_PER_K ? `${compact(n / TOKENS_PER_K)}k` : String(n))
export const RATE_LABEL_WIDTH = 8
const RATE_WINDOWS = [
  { kind: 'five_hour', label: 'session' },
  { kind: 'seven_day', label: 'semaine' },
]
const LEVELS = [
  { from: 85, color: PALETTE.red },
  { from: 60, color: PALETTE.yellow },
]
export const DANGER_FROM = LEVELS[0].from
export const levelColor = (pct: number) => LEVELS.find(l => pct >= l.from)?.color ?? PALETTE.green
const LINE_CELL = '━'
const HALF_CELL = '╸'
export const bar = (pct: number, width: number) => {
  const halves = Math.min(width * 2, pct > 0 ? Math.max(1, Math.round((pct / 100) * width * 2)) : 0)
  const full = Math.floor(halves / 2)
  const half = halves % 2
  return { filled: LINE_CELL.repeat(full) + (half ? HALF_CELL : ''), empty: LINE_CELL.repeat(width - full - half) }
}
const MS_PER_MINUTE = 60_000
const MINUTES_PER_HOUR = 60
const HOURS_PER_DAY = 24
export const formatReset = (resetsAt: string, now = Date.now()) => {
  const minutes = Math.max(0, Math.round((Date.parse(resetsAt) - now) / MS_PER_MINUTE))
  const hours = Math.floor(minutes / MINUTES_PER_HOUR)
  if (hours < 1) return `${minutes}m`
  if (hours < HOURS_PER_DAY) return `${hours}h${String(minutes % MINUTES_PER_HOUR).padStart(2, '0')}`
  return `${Math.floor(hours / HOURS_PER_DAY)}j ${hours % HOURS_PER_DAY}h`
}
const RESET_WIDTH = 6
const MAX_PERCENT = 100
const clampPercent = (value: number) => (Number.isFinite(value) ? Math.min(MAX_PERCENT, Math.max(0, Math.round(value))) : 0)
export const rateBars = (limits: readonly { kind: string; percentUsed: number; resetsAt: string }[]) =>
  RATE_WINDOWS.flatMap(({ kind, label }) => {
    const limit = limits.find(l => l.kind === kind)
    return limit ? [{ label, percent: clampPercent(limit.percentUsed), reset: `${formatReset(limit.resetsAt).padStart(RESET_WIDTH)} ${ICONS.reset}` }] : []
  })
export const contextSegments = (categories: readonly { name: string; tokens: number; kind: string }[], window?: number) => {
  const used = categories.filter(({ kind, tokens }) => kind === 'used' && tokens > 0).sort((a, b) => b.tokens - a.tokens)
  const total = window || used.reduce((sum, { tokens }) => sum + tokens, 0)
  const colors = segmentColors(used.length)
  return used.map(({ name, tokens }, index) => ({ name, tokens: formatTokens(tokens), percent: clampPercent((tokens / total) * MAX_PERCENT), color: colors[index] }))
}
const LEGEND_MAX = 3
const OTHERS_LABEL = 'autres'
export const legendSegments = <T extends { name: string; tokens: string; percent: number; color: string }>(segments: readonly T[]) => {
  if (segments.length <= LEGEND_MAX + 1) return segments
  const rest = segments.slice(LEGEND_MAX)
  const percent = rest.reduce((sum, segment) => sum + segment.percent, 0)
  return [...segments.slice(0, LEGEND_MAX), { name: OTHERS_LABEL, tokens: '', percent, color: PALETTE.muted }]
}
export const mostUrgent = <T extends { percent: number }>(quotas: readonly T[]) => [...quotas].sort((a, b) => b.percent - a.percent)
export const contextSummary = ({ tokens, window }: { tokens?: number; window?: number }) =>
  tokens !== undefined && window !== undefined ? `${formatTokens(tokens)} / ${formatTokens(window)}` : ''
export const clampedPercent = clampPercent
export const compactionLimit = ({ categories, maxTokens }: { categories: readonly { tokens: number; kind: string }[]; maxTokens: number }) =>
  maxTokens - categories.filter(({ kind }) => kind === 'buffer').reduce((sum, { tokens }) => sum + tokens, 0)
const MS_PER_SECOND = 1_000
const SECONDS_PER_MINUTE = 60
export const formatDuration = (ms: number) => {
  const seconds = Math.floor(ms / MS_PER_SECOND)
  return seconds < SECONDS_PER_MINUTE ? `${seconds}s` : `${Math.floor(seconds / SECONDS_PER_MINUTE)}m${String(seconds % SECONDS_PER_MINUTE).padStart(2, '0')}`
}
export const formatCost = (usd: number) => `$${usd.toFixed(usd < 1 ? 3 : 2)}`
export const cacheHitRate = (u: AgentUsage) => u.cacheRead / Math.max(1, u.input + u.cacheRead + u.cacheWrite)
export const percent = (ratio: number) => `${Math.round(ratio * 100)}%`
export const shortModel = (model: string) => model.replace(/^claude-/, '').replace(/-\d{8}$/, '')
export const barLine = ({ done, total }: GoalProgress) => ({ label: `${done}/${total}` })
export const clipLines = (text: string, width: number, lines: number) => (text.length > width * lines ? `${text.slice(0, width * lines - 1)}…` : text)
export const usageLines = (u: AgentUsage) => [
  `${ICONS.download} ${formatTokens(u.input)} ${ICONS.upload} ${formatTokens(u.output)} · ${ICONS.cost} ${formatCost(u.cost).slice(1)} · ${ICONS.steps} ${u.steps} étapes`,
  `${ICONS.cache} ${formatTokens(u.cacheRead)} lu · ${formatTokens(u.cacheWrite)} écrit (${percent(cacheHitRate(u))})`,
]
