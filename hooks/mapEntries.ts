const ENTRY_MAX = 8
const ENTRY_MAX_DEPTH = 3
const ENTRY_POINT = /(^|\/)(main|index|app|cli|register|lib|server|__main__|manage)\.([cm]?[jt]sx?|rs|py)$/
const PRIMARY_ENTRY = /(^|\/)(main|index|cli|__main__|manage)\./
const WORKSPACE_ENTRY = /^[^/]+\/[^/]+\/src\/[^/]+$/

const depthOf = (file: string) => file.split('/').length
const rankOf = (file: string) => (PRIMARY_ENTRY.test(file) ? 0 : 1)

export const entryPoints = (files: string[]) =>
  files
    .filter(file => ENTRY_POINT.test(file) && (depthOf(file) <= ENTRY_MAX_DEPTH || WORKSPACE_ENTRY.test(file)))
    .sort((a, b) => rankOf(a) - rankOf(b) || depthOf(a) - depthOf(b))
    .slice(0, ENTRY_MAX)
