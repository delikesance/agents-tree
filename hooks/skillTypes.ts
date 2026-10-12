export const SKILLS_STORE_KEY = 'skills'
export const SKILLS_DIR = '.claude/skills'
export const SYNTHESIS_ENV = 'HIVE_SKILL_SYNTHESIS'
export const SKILL_FILE = 'SKILL.md'
export const PROMOTE_MIN_SESSIONS = 3
export const ARCHIVE_AFTER_IDLE_SESSIONS = 10
export const MAX_ACTIVE_SKILLS = 20
export const MAX_SKILL_BODY_LINES = 40
export const MAX_DESCRIPTION_CHARS = 200

export type SkillSignalKind = 'correction' | 'retry' | 'exploration' | 'discovery'

export type SkillEvent =
  | { type: 'tool'; sessionId: string; at: number; tool: string; files: string[]; failed: boolean }
  | { type: 'prompt'; sessionId: string; at: number; text: string }
  | { type: 'skill-used'; sessionId: string; at: number; slug: string }
  | { type: 'turn-end'; sessionId: string; at: number; turns: number; corrected: boolean }

export type SkillSignal = {
  kind: SkillSignalKind
  sessionId: string
  at: number
  files: string[]
  tools: string[]
  note: string
}

export type SkillStatus = 'candidate' | 'active' | 'stale' | 'archived' | 'rejected'

export type SessionOutcome = { sessions: number; turns: number; corrections: number }

export type SkillRecord = {
  slug: string
  status: SkillStatus
  tags: string[]
  sessions: string[]
  signals: SkillSignal[]
  citedPaths: string[]
  headCommit: string
  summary?: string
  lastError?: string
  createdAt: number
  lastUsedAt?: number
  idleSessions: number
  uses: number
  cleanUses: number
  outcomes?: { with: SessionOutcome; without: SessionOutcome }
}

export type SkillLedger = Record<string, SkillRecord>

export type Outcome<T> = { ok: true; value: T } | { ok: false; reason: string }

export type SkillFs = {
  read: (path: string) => Promise<string | undefined>
  write: (path: string, text: string) => Promise<void>
  remove: (path: string) => Promise<void>
  run: (command: string[]) => Promise<string>
}
