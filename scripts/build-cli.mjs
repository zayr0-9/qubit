import { mkdirSync } from 'node:fs'
import { spawnSync } from 'node:child_process'

mkdirSync('bin', { recursive: true })

const requestedTarget = process.argv[2]
if (requestedTarget && !['linux', 'windows'].includes(requestedTarget)) {
  console.error(`unsupported CLI build target: ${requestedTarget}`)
  process.exit(1)
}

const isWindowsTarget = requestedTarget === 'windows' || (!requestedTarget && process.platform === 'win32')
const output = isWindowsTarget ? 'bin/qubit.exe' : 'bin/qubit'
const result = spawnSync('go', ['build', '-o', output, '.'], { stdio: 'inherit' })

if (result.error) {
  console.error(result.error.message)
  process.exit(1)
}

process.exit(result.status ?? 1)
