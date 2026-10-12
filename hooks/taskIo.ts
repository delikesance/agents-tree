import { spawn } from 'node:child_process'
import { readFile, rename, writeFile } from 'node:fs/promises'
import type { TaskExec, TaskIo } from './taskStore'

const MISSING_FILE = 'ENOENT'

export const diskTaskIo: TaskIo = {
  read: path =>
    readFile(path, 'utf8').catch((error: NodeJS.ErrnoException) => {
      if (error.code === MISSING_FILE) return undefined
      throw error
    }),
  write: (path, content) => writeFile(path, content, 'utf8'),
  rename,
}

export const shellTaskExec: TaskExec = (cmd, cwd) =>
  new Promise(resolve => {
    const child = spawn('bash', ['-lc', cmd], { cwd })
    let output = ''
    child.stdout.on('data', chunk => (output += chunk))
    child.stderr.on('data', chunk => (output += chunk))
    child.on('error', error => resolve({ code: 1, output: error.message }))
    child.on('close', code => resolve({ code: code ?? 1, output }))
  })
