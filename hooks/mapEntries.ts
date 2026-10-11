const ENTRY_MAX = 8
const ENTRY_MAX_DEPTH = 3
const ENTRY_POINT = /(^|\/)(main|index|app|cli|register|lib|server|__main__|manage)\.([cm]?[jt]sx?|rs|py)$/
const WORKSPACE_ENTRY = /^[^/]+\/[^/]+\/src\/[^/]+$/

const depthOf = (file: string) => file.split('/').length

export const entryPoints = (files: string[]) =>
  files
    .filter(file => ENTRY_POINT.test(file) && (depthOf(file) <= ENTRY_MAX_DEPTH || WORKSPACE_ENTRY.test(file)))
    .sort((a, b) => depthOf(a) - depthOf(b))
    .slice(0, ENTRY_MAX)
