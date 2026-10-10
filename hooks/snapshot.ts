const SNAPSHOTS_KEY = 'snapshots'
const MAX_SNAPSHOTS = 20

type Snapshot = Record<string, unknown>
type Snapshots = Record<string, Snapshot>
type Store = { get: (key: string) => Promise<unknown>; set: (key: string, value: unknown) => Promise<void> }

const loadAll = async (store: Store) => ((await store.get(SNAPSHOTS_KEY)) as Snapshots | undefined) ?? {}

export const saveSnapshot = async (store: Store, sessionId: string, snapshot: Snapshot) => {
  const { [sessionId]: _replaced, ...others } = await loadAll(store)
  await store.set(SNAPSHOTS_KEY, Object.fromEntries(Object.entries({ ...others, [sessionId]: snapshot }).slice(-MAX_SNAPSHOTS)))
}

export const loadSnapshot = async (store: Store, sessionId: string) => (await loadAll(store))[sessionId]
