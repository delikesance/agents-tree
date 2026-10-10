import { SECRET_MASK, SECRET_PATTERN } from './knowledge'
import type { Entry } from './knowledge'

type RememberInput = { key?: string; text?: string; tags?: string[] }

const mask = (value: string) => value.trim().replace(SECRET_PATTERN, SECRET_MASK)

export const parseRemember = ({ key, text, tags }: RememberInput): { entry: Pick<Entry, 'key' | 'text' | 'tags'> } | { error: string } => {
  const cleanTags = (tags ?? []).map(tag => tag.trim()).filter(Boolean)
  if (!key?.trim() || !text?.trim() || !cleanTags.length) return { error: 'remember: key, text et tags doivent être non vides.' }
  return { entry: { key: mask(key), text: mask(text), tags: cleanTags } }
}
