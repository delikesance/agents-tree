type Read = (path: string) => Promise<string>

const CONFIG_FILE = '.claudework/config.json'

const fromConfig = async (read: Read) => {
  const command = JSON.parse(await read(CONFIG_FILE)).test_command
  return typeof command === 'string' && command ? command : undefined
}

const fromPackage = async (read: Read) => (JSON.parse(await read('package.json')).scripts?.test ? 'npm test' : undefined)

const fromManifest = (file: string, command: string) => async (read: Read) => {
  await read(file)
  return command
}

const SOURCES = [fromConfig, fromPackage, fromManifest('Cargo.toml', 'cargo test'), fromManifest('pyproject.toml', 'python3 -m pytest -q')]

export const detectTestCommand = async (read: Read) => {
  for (const source of SOURCES) {
    const command = await source(read).catch(() => undefined)
    if (command) return command
  }
  return undefined
}
