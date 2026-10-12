import { clip } from './skillSynthesis'
import { coChangeClusters, readHistory } from './skillMined'
import type { SkillDeps } from './skillHost'
import type { SkillFs, SkillSignal } from './skillTypes'
import { readSymbols, symbolEntry } from './mapSymbols'
import { detectTestCommand } from './testCommand'

export const MAX_DIGEST_CHARS = 6000

const HISTORY_COMMITS = 300
const HOT_FILES = 10
const FIX_COMMITS = 15
const RECURRING_SIGNALS = 15
const FIX_PATTERN = '--grep=fix|revert|regress'
const RECURRING_KINDS = ['correction', 'retry']

type Read = (path: string) => Promise<string>

const readRepoFile = (deps: SkillDeps, root: string): Read => async path => {
  const text = await deps.fs.read(`${root}/${path}`)
  if (text === undefined) throw new Error(`missing ${path}`)
  return text
}

const section = (title: string, lines: string[]) => (lines.length ? ['', `## ${title}`, ...lines] : [])

const gitLines = (fs: SkillFs, argv: string[]) =>
  fs
    .run(['git', ...argv])
    .then(out => out.split('\n').filter(Boolean))
    .catch((): string[] => [])

const mostChanged = (files: string[]) => {
  const counts = new Map<string, number>()
  for (const file of files) counts.set(file, (counts.get(file) ?? 0) + 1)
  return [...counts].sort(([, a], [, b]) => b - a).slice(0, HOT_FILES)
}

const commandLines = async (read: Read) => {
  const command = await detectTestCommand(read)
  return section('Commands', command ? [`- Test: \`${command}\``] : [])
}

const hotLines = async (fs: SkillFs, read: Read) => {
  const hot = mostChanged(await gitLines(fs, ['log', `-n${HISTORY_COMMITS}`, '--no-merges', '--name-only', '--pretty=format:']))
  const symbols = await readSymbols(read, hot.map(([file]) => file))
  return section(
    'Hot files',
    hot.map(([file, count]) => `- ${count} ${symbolEntry(file, symbols) ?? file}`),
  )
}

const cochangeLines = async (fs: SkillFs) =>
  section(
    'Co-changed groups',
    coChangeClusters(await readHistory(fs)).map(({ files }) => `- ${files.join(', ')}`),
  )

const fixLines = async (fs: SkillFs) =>
  section(
    'Fix and revert commits',
    (await gitLines(fs, ['log', '--no-merges', '-i', '-E', FIX_PATTERN, `-n${FIX_COMMITS}`, '--pretty=format:%s'])).map(subject => `- ${subject}`),
  )

const signalLines = (signals: SkillSignal[]) =>
  section(
    'Recurring signals',
    [...new Set(signals.filter(({ kind }) => RECURRING_KINDS.includes(kind)).map(({ note }) => clip(note)))]
      .slice(-RECURRING_SIGNALS)
      .map(note => `- ${note}`),
  )

export const buildDigest = async (deps: SkillDeps, root: string, signals: SkillSignal[]) => {
  const read = readRepoFile(deps, root)
  const sections = await Promise.all([commandLines(read), hotLines(deps.fs, read), cochangeLines(deps.fs), fixLines(deps.fs)])
  return [...sections.flat(), ...signalLines(signals)].join('\n').trim().slice(0, MAX_DIGEST_CHARS)
}
