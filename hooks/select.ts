let selectedId: string | undefined

export const selectedAgent = () => selectedId
export const selectAgent = (id: string | undefined) => {
  selectedId = id
}

const dismissed = new Set<string>()
export const dismissAgent = (id: string) => void dismissed.add(id)
export const isDismissed = (id: string) => dismissed.has(id)
