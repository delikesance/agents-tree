import { expect, test } from 'claude-code/testing'
import type { SkillDeps } from './skillHost'
import { createSkillLearning } from './skillLearning'
import { validateSkill } from './skillRender'
import { SKILLS_STORE_KEY } from './skillTypes'
import { skillPath } from './skillWriter'

const CWD = '/p'
const SYNTHESIS = JSON.stringify({ description: 'Use when editing the source module', steps: ['Read the module', 'Edit it'] })
const SUMMARY = 'Sert à modifier le module source'
const isSynthesis = (command: string[]) => command.at(-1)?.includes('JSON') ?? false

const harness = () => {
  const files: Record<string, string> = { '/p/src/a.ts': 'x' }
  const store = new Map<string, unknown>()
  const state = { session: 's1', diff: '', clock: 0 }
  const deps: SkillDeps = {
    store: { get: async key => store.get(key), set: async (key, value) => void store.set(key, value) },
    fs: {
      read: async path => files[path],
      write: async (path, text) => void (files[path] = text),
      remove: async path => void delete files[path],
      run: async command => (command.includes('claude') ? (isSynthesis(command) ? SYNTHESIS : SUMMARY) : command[1] === 'diff' ? state.diff : 'head1\n'),
    },
    cwd: CWD,
    now: () => ++state.clock,
    sessionId: async () => state.session,
  }
  return { files, store, state, deps, learning: createSkillLearning() }
}

type Harness = ReturnType<typeof harness>

const settle = async ({ learning, deps }: Harness) => {
  for (let round = 0; round < 3; round++) {
    await new Promise(resolve => setTimeout(resolve, 0))
    await learning.onSessionStart(deps)
  }
}

const runTurn = async ({ learning, deps }: Harness, prompt = 'please fix the source module') => {
  await learning.onPrompt(deps, prompt)
  await learning.onTool(deps, { tool: 'Read', file_path: '/p/src/a.ts' }, false)
  await learning.onTool(deps, { tool: 'Grep', path: '/p/src' }, false)
  await learning.onTool(deps, { tool: 'Edit', file_path: '/p/src/a.ts' }, false)
  await learning.onTurnComplete(deps)
}

const runSession = async (h: Harness, id: string) => {
  h.state.session = id
  await runTurn(h)
}

const learnSkill = async (h: Harness) => {
  for (const id of ['s1', 's2', 's3']) await runSession(h, id)
  await settle(h)
  return Object.keys(await h.learning.ledger(h.deps))[0] as string
}

test('three sessions of the same sequence write a valid SKILL.md', async () => {
  const h = harness()
  const slug = await learnSkill(h)
  const text = h.files[skillPath(CWD, slug)] as string
  expect(validateSkill(text)).toEqual([])
  expect(text).toContain('- src/a.ts')
  expect((await h.learning.ledger(h.deps))[slug]).toMatchObject({ status: 'active', headCommit: 'head1', citedPaths: ['src/a.ts'] })
  expect((h.store.get(SKILLS_STORE_KEY) as Record<string, unknown>)[slug]).toBeDefined()
})

test('a clean skill use is recorded after the next uncorrected prompt', async () => {
  const h = harness()
  const slug = await learnSkill(h)
  h.state.session = 's4'
  await h.learning.onSkillPrompt(h.deps, slug)
  await h.learning.onSkillPrompt(h.deps, 'hand-made')
  await h.learning.onTurnComplete(h.deps)
  await h.learning.onPrompt(h.deps, 'thanks, next task')
  const ledger = await h.learning.ledger(h.deps)
  expect(ledger[slug]).toMatchObject({ uses: 1, cleanUses: 1 })
  expect(ledger['hand-made']).toBeUndefined()
})

test('a correction after the turn makes the use unclean', async () => {
  const h = harness()
  const slug = await learnSkill(h)
  h.state.session = 's4'
  await h.learning.onSkillPrompt(h.deps, slug)
  await h.learning.onTurnComplete(h.deps)
  await h.learning.onPrompt(h.deps, 'non, plutôt autre chose')
  expect((await h.learning.ledger(h.deps))[slug]).toMatchObject({ uses: 1, cleanUses: 0 })
})

test('a change to a cited path marks the skill stale on the next session', async () => {
  const h = harness()
  const slug = await learnSkill(h)
  h.state.diff = 'src/a.ts\n'
  h.state.session = 's4'
  await h.learning.onSessionStart(h.deps)
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('stale')
  h.state.diff = ''
  await h.learning.revalidate(h.deps, slug)
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('active')
})

test('idle sessions archive the skill and delete its file', async () => {
  const h = harness()
  const slug = await learnSkill(h)
  for (let n = 4; n <= 15; n++) {
    h.state.session = `s${n}`
    await h.learning.onSessionStart(h.deps)
  }
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('archived')
  expect(h.files[skillPath(CWD, slug)]).toBeUndefined()
})

test('reject deletes the file and the same pattern never returns', async () => {
  const h = harness()
  const slug = await learnSkill(h)
  await h.learning.reject(h.deps, slug)
  expect(h.files[skillPath(CWD, slug)]).toBeUndefined()
  for (const id of ['s4', 's5', 's6']) await runSession(h, id)
  await settle(h)
  const ledger = await h.learning.ledger(h.deps)
  expect(Object.keys(ledger)).toEqual([slug])
  expect(ledger[slug]?.status).toBe('rejected')
  expect(Object.keys(h.files)).toEqual(['/p/src/a.ts'])
})

test('accept promotes a candidate before the session threshold', async () => {
  const h = harness()
  await runSession(h, 's1')
  const slug = Object.keys(await h.learning.ledger(h.deps))[0] as string
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('candidate')
  await h.learning.accept(h.deps, slug)
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('active')
})

test('a failed synthesis keeps the candidate and retries later', async () => {
  const h = harness()
  const run = h.deps.fs.run
  h.deps.fs.run = async command => (command.includes('claude') ? 'not json' : run(command))
  const slug = await learnSkill(h)
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('candidate')
  h.deps.fs.run = run
  await runSession(h, 's4')
  await settle(h)
  expect((await h.learning.ledger(h.deps))[slug]?.status).toBe('active')
})

test('a failed synthesis stores the reason on the candidate', async () => {
  const h = harness()
  const run = h.deps.fs.run
  h.deps.fs.run = async command => (command.includes('claude') ? 'not json' : run(command))
  const slug = await learnSkill(h)
  expect((await h.learning.ledger(h.deps))[slug]).toMatchObject({ status: 'candidate', lastError: 'réponse de synthèse invalide' })
  h.deps.fs.run = run
  await h.learning.accept(h.deps, slug)
  expect((await h.learning.ledger(h.deps))[slug]).toMatchObject({ status: 'active' })
  expect((await h.learning.ledger(h.deps))[slug]?.lastError).toBeUndefined()
})

test('accept returns the failure reason when the write is refused', async () => {
  const h = harness()
  await runSession(h, 's1')
  const slug = Object.keys(await h.learning.ledger(h.deps))[0] as string
  const run = h.deps.fs.run
  h.deps.fs.run = async command => (command.includes('claude') ? '{"description":"Fix tests","steps":["a"]}' : run(command))
  expect(await h.learning.accept(h.deps, slug)).toBe("écriture refusée : description must start with 'Use when'")
  expect((await h.learning.ledger(h.deps))[slug]).toMatchObject({ status: 'candidate', lastError: "écriture refusée : description must start with 'Use when'" })
})

test('accept explains a missing project and a promotion already running', async () => {
  const h = harness()
  await runSession(h, 's1')
  const slug = Object.keys(await h.learning.ledger(h.deps))[0] as string
  expect(await h.learning.accept({ ...h.deps, cwd: undefined }, slug)).toBe('Aucun projet ouvert.')
  const pending = h.learning.accept(h.deps, slug)
  expect(await h.learning.accept(h.deps, slug)).toBe('Rédaction déjà en cours pour ce skill.')
  await pending
})

test('a candidate gets a stored one-line summary', async () => {
  const h = harness()
  await runSession(h, 's1')
  await settle(h)
  const slug = Object.keys(await h.learning.ledger(h.deps))[0] as string
  expect((await h.learning.ledger(h.deps))[slug]).toMatchObject({ status: 'candidate', summary: SUMMARY })
})
