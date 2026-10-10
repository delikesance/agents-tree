const READ_TOOL = 'Read'
export const DEDUPED_TOOLS = [READ_TOOL, 'Grep', 'Glob']
const DEDUPED_ARGS = ['file_path', 'offset', 'limit', 'pattern', 'path', 'glob', 'type', 'output_mode', 'head_limit', '-i', '-n', '-A', '-B', '-C', 'multiline', 'pages']
export const WRITING_TOOLS = ['Edit', 'Write', 'MultiEdit', 'NotebookEdit']
const DUPLICATE_READ_DENY = 'Appel identique déjà fait et sans modification depuis: son résultat est plus haut dans le contexte. Refais-le seulement si tu en as vraiment besoin.'
const readSeen = new Set<string>()
const readWarned = new Set<string>()

export const readKey = (agent: string, e: Record<string, unknown>) =>
  JSON.stringify([agent, e.tool, ...DEDUPED_ARGS.map(arg => e[arg] ?? '')])

export const pendingReads = new Map<string, string>()

export const settleRead = (toolUseId: string, complete: boolean) => {
  const key = pendingReads.get(toolUseId)
  pendingReads.delete(toolUseId)
  if (key && complete) readSeen.add(key)
}

export const duplicateReadDeny = (agent: string, e: Record<string, unknown>) => {
  const key = readKey(agent, e)
  if (!readSeen.has(key) || readWarned.has(key)) return undefined
  readWarned.add(key)
  return DUPLICATE_READ_DENY
}

export const forgetReads = () => {
  readSeen.clear()
  readWarned.clear()
  pendingReads.clear()
}
const MAX_IMAGE_READS = 6
const IMAGE_FILE = /\.(png|jpe?g|gif|webp)$/i
export const IMAGE_READ_DENY = `Limite de ${MAX_IMAGE_READS} images lues atteinte: décris ou compare via un script, ou demande à l'utilisateur.`

let imageReads = 0

export const imageLimitReached = () => imageReads >= MAX_IMAGE_READS
export const countImageRead = () => { imageReads++ }
export const resetImageReads = () => { imageReads = 0 }

export const isImageRead = (e: Record<string, unknown>) => e.tool === READ_TOOL && IMAGE_FILE.test(String(e.file_path ?? ''))
