import { mkdir, rename, rm, writeFile } from "node:fs/promises";
import { dirname, join } from "node:path";
import { randomUUID } from "node:crypto";

export interface WriteJsonAtomicOptions {
  mode?: number;
}

export async function writeJsonAtomic(filePath: string, value: unknown, options: WriteJsonAtomicOptions = {}): Promise<void> {
  const dir = dirname(filePath);
  await mkdir(dir, { recursive: true });
  const tempPath = join(dir, `.${Date.now()}-${process.pid}-${randomUUID()}.tmp`);
  try {
    await writeFile(tempPath, `${JSON.stringify(value, null, 2)}\n`, options.mode === undefined ? undefined : { mode: options.mode });
    await rename(tempPath, filePath);
  } catch (error) {
    await rm(tempPath, { force: true }).catch(() => {});
    throw error;
  }
}
