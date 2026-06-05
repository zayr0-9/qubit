import { createHash } from 'node:crypto'
import { createWriteStream, existsSync } from 'node:fs'
import { cp, mkdir, readdir, rm, stat, writeFile } from 'node:fs/promises'
import { basename, join } from 'node:path'
import { spawnSync } from 'node:child_process'

const targets = {
  linux: { goos: 'linux', goarch: 'amd64', exe: 'qubit', archiveExt: 'tar.gz' },
  windows: { goos: 'windows', goarch: 'amd64', exe: 'qubit.exe', archiveExt: 'zip' },
}

const requestedTarget = process.argv[2] || hostTarget()
if (!targets[requestedTarget]) {
  console.error(`unsupported release target: ${requestedTarget}`)
  console.error(`supported targets: ${Object.keys(targets).join(', ')}`)
  process.exit(1)
}

const target = targets[requestedTarget]
const packageJson = JSON.parse(await readText('package.json'))
const version = process.env.QUBIT_VERSION || packageJson.version || '0.0.0'
const name = `qubit-v${version}-${requestedTarget}-x64`
const outDir = 'release'
const stageDir = join(outDir, 'stage', name)
const archiveName = `${name}.${target.archiveExt}`
const archivePath = join(outDir, archiveName)

await assertExists('pnpm-lock.yaml')
await assertExists('node_modules')
await assertExists('prompts')

run('pnpm', ['run', 'build:runtime'])
await rm(join(outDir, 'stage'), { recursive: true, force: true })
await mkdir(join(stageDir, 'bin'), { recursive: true })

run('go', ['build', '-o', join(stageDir, 'bin', target.exe), '.'], {
  env: { ...process.env, GOOS: target.goos, GOARCH: target.goarch },
})

await copyRuntimeTree(stageDir)
await writeFile(join(stageDir, 'VERSION'), `${version}\n`, 'utf8')
await writeFile(join(stageDir, 'README.install.txt'), installReadme(requestedTarget), 'utf8')

await rm(archivePath, { force: true })
if (requestedTarget === 'windows') {
  if (process.platform === 'win32') {
    run('powershell', ['-NoProfile', '-Command', `Compress-Archive -Path ${psQuote(stageDir + '/*')} -DestinationPath ${psQuote(archivePath)} -Force`])
  } else {
    run('zip', ['-qr', join('..', archiveName), name], { cwd: join(outDir, 'stage') })
  }
} else {
  run('tar', ['-czf', archivePath, '-C', join(outDir, 'stage'), name])
}

const sha256 = await fileSha256(archivePath)
await writeFile(`${archivePath}.sha256`, `${sha256}  ${archiveName}\n`, 'utf8')
console.log(`created ${archivePath}`)
console.log(`sha256 ${sha256}`)

function hostTarget() {
  if (process.platform === 'win32') return 'windows'
  return 'linux'
}

async function copyRuntimeTree(root) {
  for (const path of ['dist', 'prompts', 'node_modules']) {
    await cp(path, join(root, path), { recursive: true, verbatimSymlinks: true })
  }
  for (const path of ['package.json', 'pnpm-lock.yaml']) {
    await cp(path, join(root, basename(path)))
  }
}

async function assertExists(path) {
  if (!existsSync(path)) {
    console.error(`missing ${path}`)
    if (path === 'node_modules') console.error('run pnpm install before packaging')
    process.exit(1)
  }
}

async function readText(path) {
  return await import('node:fs/promises').then((fs) => fs.readFile(path, 'utf8'))
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, { stdio: 'inherit', ...options })
  if (result.error) {
    console.error(result.error.message)
    process.exit(1)
  }
  if ((result.status ?? 1) !== 0) process.exit(result.status ?? 1)
}

async function fileSha256(path) {
  const hash = createHash('sha256')
  const stream = (await import('node:fs')).createReadStream(path)
  await new Promise((resolve, reject) => {
    stream.on('data', (chunk) => hash.update(chunk))
    stream.on('end', resolve)
    stream.on('error', reject)
  })
  return hash.digest('hex')
}

function psQuote(value) {
  return `'${String(value).replaceAll("'", "''")}'`
}

function installReadme(targetName) {
  const command = targetName === 'windows' ? '.\\bin\\qubit.exe' : './bin/qubit'
  return `Qubit ${version}\n\nThis archive contains the Qubit Go CLI plus its Node runtime sidecar.\nNode.js must be installed and available on PATH.\n\nRun from this directory with:\n\n  ${command}\n\nInstall scripts place the CLI on PATH while keeping dist/, prompts/, and node_modules/ beside it for the runtime.\n`
}
