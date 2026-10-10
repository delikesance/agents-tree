import { ensureProjectMap, projectMapExists } from './mapHost'
import { withMapPointer } from './routing'

type MapUpkeepHost = Parameters<typeof ensureProjectMap>[0]

export const startMapUpkeep = ($: MapUpkeepHost, cwd: string | undefined, isCommand: boolean) => {
  if (!isCommand) void ensureProjectMap($, cwd).catch(() => undefined)
}

export const pointAtMap = async ($: Pick<MapUpkeepHost, 'fs'>, cwd: string | undefined, prompt: string) =>
  withMapPointer(prompt, await projectMapExists($, cwd))
