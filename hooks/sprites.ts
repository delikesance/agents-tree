const BLINK_EVERY_MS = 2400
type Pixel = `${number},${number}`
type Tone = 'hi' | 'base' | 'shade' | 'dark' | 'eye'
type Palette = Record<Tone, string>
type Segment = { char: string; fg?: string; bg?: string }
type Span = [row: number, from: number, to: number]
type Pose = { eyes: Pixel[]; armLift: number; legStep: number }
type Mood = { frames: Pose[]; frameMs: number }
type Species = { color: string; body: Span[]; legCols: number[]; arms: boolean }

const SUBPIXELS = 2
const BODY_CELLS_X = 8
const SLEEP_CELLS_X = 2
export const SPRITE_WIDTH = BODY_CELLS_X + SLEEP_CELLS_X
export const SPRITE_CELLS_Y = 3
const QUADRANTS = [' ', '▘', '▝', '▀', '▖', '▌', '▞', '▛', '▗', '▚', '▐', '▜', '▄', '▙', '▟', '█']
const LEG_ROW = 4
const ARM_ROW = 2
const ARM_COLS: [number, number][] = [[0, 1], [14, 15]]
const EYE_COLS = [5, 10]
const EYE_COLOR = '#1e1412'
const HI_MIX = { toward: 255, amount: 0.35 }
const SHADE_MIX = { toward: 0, amount: 0.3 }
const DARK_MIX = { toward: 0, amount: 0.58 }
const MISSING_COLOR_DISTANCE = 1e6

const pixel = (row: number, col: number): Pixel => `${row},${col}`
const span = (from: number, length: number) => Array.from({ length }, (_, i) => from + i)
const rows = (...spans: Span[]) => spans.flatMap(([row, from, to]) => span(from, to - from + 1).map(col => pixel(row, col)))
const eyesAt = (eyeRows: number[], shift = 0, width = 1): Pixel[] =>
  EYE_COLS.flatMap(col => eyeRows.flatMap(row => span(col + shift, width).map(c => pixel(row, c))))

const SPECIES = {
  sonnet: { color: '#d77757', body: [[0, 2, 13], [1, 2, 13], [2, 2, 13], [3, 2, 13]], legCols: [3, 6, 9, 12], arms: true },
  haiku: { color: '#4fb3a5', body: [[0, 3, 12], [1, 3, 12], [2, 3, 12], [3, 3, 12]], legCols: [4, 6, 9, 11], arms: false },
  opus: { color: '#c24a4a', body: [[0, 2, 3], [0, 12, 13], [1, 2, 13], [2, 2, 13], [3, 2, 13]], legCols: [3, 6, 9, 12], arms: true },
  fable: { color: '#8a63d2', body: [[0, 6, 9], [1, 2, 13], [2, 2, 13], [3, 2, 13]], legCols: [3, 6, 9, 12], arms: true },
} satisfies Record<string, Species>
const DEFAULT_SPECIES = 'sonnet'

const SLEEP_FRAME_MS = 900
const TALL = eyesAt([2, 3])
const MOODS = {
  idle: { frames: [{ eyes: TALL, armLift: 0, legStep: -1 }], frameMs: 1000 },
  blink: { frames: [{ eyes: eyesAt([3]), armLift: 0, legStep: -1 }], frameMs: 1000 },
  read: {
    frames: [-1, 0, 1, 0].map(shift => ({ eyes: eyesAt([2, 3], shift), armLift: 0, legStep: -1 })),
    frameMs: 300,
  },
  write: {
    frames: [0, 1].map(armLift => ({ eyes: eyesAt([3]), armLift, legStep: -1 })),
    frameMs: 200,
  },
  run: { frames: [0, 1].map(legStep => ({ eyes: TALL, armLift: legStep, legStep })), frameMs: 200 },
  delegate: {
    frames: [{ eyes: eyesAt([2]), armLift: 0, legStep: -1 }, { eyes: eyesAt([2]), armLift: 1, legStep: -1 }],
    frameMs: 350,
  },
  web: { frames: [{ eyes: eyesAt([2, 3], 0, 2), armLift: 0, legStep: -1 }, { eyes: eyesAt([2, 3], -1, 2), armLift: 0, legStep: -1 }], frameMs: 400 },
  sleep: {
    frames: [0, 1].map(armLift => ({ eyes: eyesAt([3], 0, 2), armLift, legStep: -1 })),
    frameMs: SLEEP_FRAME_MS,
  },
  think: {
    frames: [{ eyes: eyesAt([2], -1), armLift: 0, legStep: -1 }, { eyes: eyesAt([2], 1), armLift: 0, legStep: -1 }, { eyes: TALL, armLift: 0, legStep: -1 }],
    frameMs: 450,
  },
} satisfies Record<string, Mood>
type MoodName = keyof typeof MOODS
const MOOD_BY_TOOL: Record<string, MoodName> = {
  Read: 'read', Grep: 'read', Glob: 'read', LS: 'read',
  Edit: 'write', MultiEdit: 'write', Write: 'write', NotebookEdit: 'write',
  Bash: 'run',
  Agent: 'delegate', Task: 'delegate',
  WebFetch: 'web', WebSearch: 'web',
}
const DEFAULT_MOOD: MoodName = 'think'
const BLINK_MS = 160
const SLEEP_MARKS = [{ row: 2, col: 0, char: 'z' }, { row: 1, col: 1, char: 'z' }, { row: 0, col: 1, char: 'Z' }]
const SLEEP_STEPS = SLEEP_MARKS.length + 1
const SLEEP_MARK_COLOR = '#9b978c'

const hexToRgb = (hex: string) => [1, 3, 5].map(i => parseInt(hex.slice(i, i + 2), 16))
const mix = (hex: string, { toward, amount }: { toward: number; amount: number }) =>
  `#${hexToRgb(hex).map(v => Math.round(v + (toward - v) * amount).toString(16).padStart(2, '0')).join('')}`
const paletteFor = (color: string): Palette => ({
  hi: mix(color, HI_MIX), base: color, shade: mix(color, SHADE_MIX), dark: mix(color, DARK_MIX), eye: EYE_COLOR,
})

const poseParts = (species: Species, { eyes, armLift, legStep }: Pose) => {
  const legs = species.legCols.flatMap((col, i) => rows([LEG_ROW, col, col], [LEG_ROW + 1, col, col]).slice(0, legStep < 0 ? 2 : 2 - ((i + legStep) % 2)))
  const arms = species.arms
    ? ARM_COLS.flatMap(([from, to]) => rows(...span(ARM_ROW + armLift, 2).map((row): Span => [row, from, to])))
    : []
  return { parts: { body: rows(...species.body), arms, legs }, eyes: new Set(eyes) }
}

const shadePixels = (parts: Record<string, Pixel[]>, eyes: Set<Pixel>) => {
  const owner = new Map<Pixel, string>()
  Object.entries(parts).forEach(([name, pixels]) => pixels.forEach(p => owner.set(p, name)))
  const tones = new Map<Pixel, Tone>()
  owner.forEach((name, key) => {
    const [r = 0, c = 0] = key.split(',').map(Number)
    const same = (dr: number, dc: number) => owner.get(pixel(r + dr, c + dc)) === name
    if (eyes.has(key)) return tones.set(key, 'eye')
    if (name !== 'body') return tones.set(key, 'shade')
    if (!same(-1, 0) || !same(0, -1)) return tones.set(key, 'hi')
    if (!same(0, 1)) return tones.set(key, 'shade')
    tones.set(key, 'base')
  })
  return tones
}

const colorDistance = (palette: Palette, a?: Tone, b?: Tone) => {
  if (a === b) return 0
  if (!a || !b) return MISSING_COLOR_DISTANCE
  const [ra, rb] = [hexToRgb(palette[a]), hexToRgb(palette[b])]
  return ra.reduce((sum, v, i) => sum + (v - (rb[i] ?? 0)) ** 2, 0)
}

const dominantTones = (cell: (Tone | undefined)[]) => {
  const counts = new Map<Tone | undefined, number>()
  cell.forEach(t => counts.set(t, (counts.get(t) ?? 0) + 1))
  const priority = (tone?: Tone) => (tone === 'eye' ? Infinity : counts.get(tone) ?? 0)
  return [...counts.keys()].sort((a, b) => priority(b) - priority(a)).slice(0, 2)
}

const renderCell = (palette: Palette, cell: (Tone | undefined)[]): Segment => {
  const tones = dominantTones(cell)
  const foreground = tones[0] === undefined && tones.length > 1 ? tones[1] : tones[0]
  if (foreground === undefined) return { char: ' ' }
  const background = tones.find(t => t !== foreground)
  const mask = cell.reduce((bits, tone, i) => {
    const nearer = colorDistance(palette, tone, foreground) <= colorDistance(palette, tone, background)
    return nearer ? bits | (1 << i) : bits
  }, 0)
  return { char: QUADRANTS[mask] ?? ' ', fg: palette[foreground], bg: background && palette[background] }
}

const speciesFor = (model: string): Species =>
  Object.entries(SPECIES).find(([name]) => model.includes(name))?.[1] ?? SPECIES[DEFAULT_SPECIES]

const frameAt = ({ frames, frameMs }: Mood, now: number) => frames[Math.floor(now / frameMs) % frames.length]!

const moodOf = (activity: string | undefined, now: number, asleep: boolean) => {
  if (asleep) return frameAt(MOODS.sleep, now)
  if (!activity) return frameAt(now % BLINK_EVERY_MS < BLINK_MS ? MOODS.blink : MOODS.idle, now)
  const [tool = ''] = activity.split(' ')
  return frameAt(MOODS[MOOD_BY_TOOL[tool] ?? DEFAULT_MOOD], now)
}

export const speciesColor = (model: string) => speciesFor(model).color

const toHex = (rgb: number[]) => `#${rgb.map(v => Math.round(v).toString(16).padStart(2, '0')).join('')}`

export const gradient = (from: string, to: string, steps: number) => {
  const [a, b] = [hexToRgb(from), hexToRgb(to)]
  return Array.from({ length: steps }, (_, i) => toHex(a.map((v, k) => v + ((b[k] ?? 0) - v) * (steps > 1 ? i / (steps - 1) : 0))))
}

const withSleepMarks = (lines: Segment[][], now: number): Segment[][] => {
  const visible = Math.floor(now / SLEEP_FRAME_MS) % SLEEP_STEPS
  const blank = (): Segment[] => span(0, SLEEP_CELLS_X).map(() => ({ char: ' ' }))
  return lines.map((line, row) => {
    const tail = blank()
    SLEEP_MARKS.slice(0, visible).filter(mark => mark.row === row).forEach(({ col, char }) => (tail[col] = { char, fg: SLEEP_MARK_COLOR }))
    return [...line, ...tail]
  })
}

export const spriteLines = (model: string, activity: string | undefined, now: number, asleep = false): Segment[][] => {
  const species = speciesFor(model)
  const palette = paletteFor(species.color)
  const { parts, eyes } = poseParts(species, moodOf(activity, now, asleep))
  const tones = shadePixels(parts, eyes)
  const lines = span(0, SPRITE_CELLS_Y).map(cy =>
    span(0, BODY_CELLS_X).map(cx =>
      renderCell(palette, span(0, SUBPIXELS).flatMap(sy => span(0, SUBPIXELS).map(sx => tones.get(pixel(cy * SUBPIXELS + sy, cx * SUBPIXELS + sx))))),
    ),
  )
  return asleep ? withSleepMarks(lines, now) : lines
}
