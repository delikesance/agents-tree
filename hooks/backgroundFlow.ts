let pending: string | undefined

export const stashFlow = (text: string | undefined) => {
  pending = text
}

export const takeFlow = () => {
  const text = pending
  pending = undefined
  return text
}
