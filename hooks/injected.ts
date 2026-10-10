const lastInjected = new Map<string, string>()

export const onlyIfChanged = (session: string, kind: string, text: string | undefined) => {
  if (!text) return undefined
  const key = `${session}:${kind}`
  if (lastInjected.get(key) === text) return undefined
  lastInjected.set(key, text)
  return text
}
