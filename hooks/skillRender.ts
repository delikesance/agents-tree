import { MAX_DESCRIPTION_CHARS, MAX_SKILL_BODY_LINES } from './skillTypes'

export type SkillDraft = { slug: string; description: string; steps: string[]; citedPaths: string[] }

const SECRET = /\b(sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|xox[bap]-[A-Za-z0-9-]{10,}|eyJ[A-Za-z0-9_-]{15,}\.[A-Za-z0-9_-]{10,})|(api[_-]?key|token|secret|password)\s*[:=]\s*\S{8,}/i
const HOME_PATH = /(\/home\/|\/Users\/|~\/)\S/
const TRIGGER_PREFIX = /^Use when\b/
const FRONTMATTER = /^---\n([\s\S]*?)\n---\n?([\s\S]*)$/

export function renderSkill({ slug, description, steps, citedPaths }: SkillDraft): string {
  const lines = ['---', `name: ${slug}`, `description: ${description}`, '---', '', '## Steps']
  steps.forEach((step, index) => lines.push(`${index + 1}. ${step}`))
  if (citedPaths.length) lines.push('', '## Relevant files', ...citedPaths.map(path => `- ${path}`))
  return lines.join('\n') + '\n'
}

const field = (header: string, name: string) => header.match(new RegExp(`^${name}:[ \\t]*(.*)$`, 'm'))?.[1].trim()

function descriptionErrors(description: string | undefined): string[] {
  if (!description) return ['description is empty']
  const errors: string[] = []
  if (description.length > MAX_DESCRIPTION_CHARS) errors.push(`description exceeds ${MAX_DESCRIPTION_CHARS} chars`)
  if (!TRIGGER_PREFIX.test(description)) errors.push("description must start with 'Use when'")
  return errors
}

export function validateSkill(text: string): string[] {
  const match = text.match(FRONTMATTER)
  if (!match) return ['missing frontmatter']
  const [, header, body] = match
  const errors = [...descriptionErrors(field(header, 'description'))]
  if (!field(header, 'name')) errors.push('name is empty')
  if (header.split('\n').some(line => !/^[a-z-]+:/.test(line))) errors.push('description spans multiple lines')
  if (body.trimEnd().split('\n').length > MAX_SKILL_BODY_LINES) errors.push(`body exceeds ${MAX_SKILL_BODY_LINES} lines`)
  if (SECRET.test(text)) errors.push('contains a secret-looking string')
  if (HOME_PATH.test(text)) errors.push('contains an absolute home path')
  return errors
}
