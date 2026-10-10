import { contextBar, rateBars } from '../hooks/format'
import { makePanel } from '../hooks/panel'
import type { Node } from '../hooks/tree'

const COLS = Number(process.argv[2] ?? 80)
const ESC = '\x1b['
const rgb = (hex: string, layer: 38 | 48) => `${ESC}${layer};2;${[1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16)).join(';')}m`
const ANSI_PATTERN = /\x1b\[[0-9;]*m/g
const width = (line: string) => [...line.replace(ANSI_PATTERN, '')].length
const flat = (children: unknown): unknown[] => [children].flat(Infinity).filter(c => c !== false && c !== undefined && c !== null && c !== '')

type El = { type: unknown; props: Record<string, any> }
const inline = (node: unknown, inherited = ''): string => {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  const { type, props } = node as El
  if (typeof type === 'function') return inline(type(props), inherited)
  const style = (props.bold ? `${ESC}1m` : '') + (props.color ? rgb(props.color, 38) : '') + (props.backgroundColor ? rgb(props.backgroundColor, 48) : '')
  const own = inherited + style
  return own + flat(props.children).map(c => inline(c, own)).join('') + `${ESC}0m` + inherited
}
const box = (node: El, avail: number): string[] => {
  const { props } = node
  const padX = props.paddingX ?? props.padding ?? 0
  const padY = props.paddingY ?? props.padding ?? 0
  const margin = props.marginLeft ?? 0
  const inner = avail - 2 * padX - margin
  const kids = flat(props.children).map(c => render(c, inner))
  const body = props.flexDirection === 'column' ? kids.flat() : [kids.map(k => k.join('')).join('')]
  const rowTwo = props.justifyContent === 'space-between' && kids.length === 2
  const content = rowTwo ? [kids[0].join('') + ' '.repeat(Math.max(1, inner - width(kids[0].join('')) - width(kids[1].join('')))) + kids[1].join('')] : body
  const bg = props.backgroundColor ? rgb(props.backgroundColor, 48) : ''
  const padded = [...Array(padY).fill(''), ...content, ...Array(padY).fill('')].map(l => `${' '.repeat(margin)}${bg}${' '.repeat(padX)}${l.replaceAll(`${ESC}0m`, `${ESC}0m${bg}`)}${' '.repeat(Math.max(0, inner - width(l)) + padX)}${ESC}0m`)
  return [...Array(props.marginTop ?? 0).fill(''), ...padded, ...Array(props.marginBottom ?? 0).fill('')]
}
const render = (node: unknown, avail: number): string[] => {
  if (typeof node === 'string' || typeof node === 'number') return [String(node)]
  const el = node as El
  if (typeof el.type === 'function') return render(el.type(el.props), avail)
  return el.type === 'Text' ? [inline(el)] : box(el, avail)
}

const Box = 'Box'
const Text = 'Text'
const { Section, Quota, AgentCard } = makePanel({ Box, Text, Button: 'Button' } as never)

const usage = (steps: number, cost: number) => ({ model: 'claude-sonnet-5-5', input: 40_000, output: 46_000, cacheRead: 9_100_000, cacheWrite: 0, steps, cost })
const agent = (over: Partial<Node>): Node => ({ id: 'a', label: 'main', status: 'running', reveal: 1, children: [], elapsed: 252_000, usage: usage(105, 0.412), goal: 'Refactorer le module de paiement', ...over })
const nodes = [
  { node: agent({ label: 'main', status: 'completed', finished: true }), depth: 0 },
  { node: agent({ label: 'explorer', model: 'claude-haiku-5-5', progress: { done: 3, total: 5 } }), depth: 1 },
  { node: agent({ label: 'worker', status: 'failed', goal: 'Mettre à jour les tests' }), depth: 1 },
]
const limits = [
  { kind: 'five_hour', percentUsed: 1, resetsAt: new Date(Date.now() + 130 * 60_000).toISOString() },
  { kind: 'seven_day', percentUsed: 91, resetsAt: new Date(Date.now() + 99 * 3600_000).toISOString() },
]
const quotas = [...rateBars(limits), contextBar({ percent: 6, tokens: 48_000, window: 800_000 })]

const tree = (
  <Box flexDirection="column" padding={1}>
    <Section title="QUOTAS" cols={COLS}>{quotas.map(q => <Quota {...q} cols={COLS} />)}</Section>
    <Section title="AGENTS" cols={COLS}>{nodes.map(({ node, depth }) => <AgentCard node={node} depth={depth} cols={COLS} onOpen={() => {}} onDismiss={() => {}} />)}</Section>
  </Box>
)
console.log(render(tree, COLS).join('\n'))
