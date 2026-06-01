import * as fs from 'node:fs'
import * as os from 'node:os'
import * as path from 'node:path'

/**
 * Create a temporary directory with the given prefix.
 * The directory is created under the OS temp directory.
 */
export async function createTempDir(prefix: string = 'qubit-test-'): Promise<string> {
  return fs.promises.mkdtemp(path.join(os.tmpdir(), prefix))
}

/**
 * Recursively remove a temporary directory and all its contents.
 * Silently succeeds if the directory does not exist.
 */
export async function cleanupTempDir(dir: string): Promise<void> {
  await fs.promises.rm(dir, { recursive: true, force: true })
}

/**
 * Write a file with the given content inside the specified directory.
 * Returns the full path to the created file.
 * Optionally creates parent directories.
 */
export async function writeFileInDir(
  dir: string,
  filename: string,
  content: string,
  createParentDirs: boolean = true
): Promise<string> {
  const fullPath = path.join(dir, filename)
  if (createParentDirs) {
    await fs.promises.mkdir(path.dirname(fullPath), { recursive: true })
  }
  await fs.promises.writeFile(fullPath, content, 'utf8')
  return fullPath
}

/**
 * Read a file's content from inside the specified directory.
 */
export async function readFileInDir(dir: string, filename: string): Promise<string> {
  const fullPath = path.join(dir, filename)
  return fs.promises.readFile(fullPath, 'utf8')
}

/**
 * Check whether a file exists at the given path.
 */
export async function fileExists(filePath: string): Promise<boolean> {
  try {
    const stats = await fs.promises.stat(filePath)
    return stats.isFile()
  } catch {
    return false
  }
}

/**
 * List files matching a glob-like pattern (simple extension filter) in a directory.
 */
export async function listFilesInDir(dir: string, extension?: string): Promise<string[]> {
  const entries = await fs.promises.readdir(dir, { withFileTypes: true })
  return entries
    .filter(e => e.isFile() && (!extension || e.name.endsWith(extension)))
    .map(e => e.name)
}
