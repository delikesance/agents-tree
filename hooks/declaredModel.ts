export type Reader = (path: string) => Promise<string>

const AGENTS_DIR = '.claude/agents'
const INHERIT = 'inherit'
const MODEL_LINE = /^model:\s*['"]?([^'"\s#]+)/m
const FRONTMATTER = /^---\r?\n([\s\S]*?)\r?\n---/

const isPathSafe = (agentType: string) => !!agentType && !agentType.includes('/') && !agentType.includes('..')

const modelOfDefinition = (text: string) => {
  const model = FRONTMATTER.exec(text)?.[1]?.match(MODEL_LINE)?.[1]
  return model === INHERIT ? undefined : model
}

export const declaredModel = async (read: Reader, dirs: (string | undefined)[], agentType: string | undefined) => {
  if (!agentType || !isPathSafe(agentType)) return undefined
  for (const dir of dirs) {
    if (!dir) continue
    const text = await read(`${dir}/${AGENTS_DIR}/${agentType}.md`).catch(() => undefined)
    if (text !== undefined) return modelOfDefinition(text)
  }
  return undefined
}

export const modelToInject = (callerModel: string | undefined, declared: string | undefined, routeModel: string) =>
  callerModel || declared ? undefined : routeModel
