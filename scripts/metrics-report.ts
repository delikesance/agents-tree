import { existsSync, readFileSync } from 'node:fs'
import { metricsPath, summarize } from '../hooks/metrics'

const path = metricsPath(process.argv[2] ?? process.cwd())
const lines = existsSync(path) ? readFileSync(path, 'utf8').split('\n') : []
console.log(JSON.stringify(summarize(lines), null, 2))
