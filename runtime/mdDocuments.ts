import * as fs from "node:fs/promises";
import * as path from "node:path";

const allowedSections = ["plans", "user-docs"] as const;
type MdSection = (typeof allowedSections)[number];

export interface MdFileInfo {
  section: MdSection;
  name: string;
  title?: string;
  path: string;
  modifiedAt?: string;
  sizeBytes?: number;
}

function isAllowedSection(section: string): section is MdSection {
  return (allowedSections as readonly string[]).includes(section);
}

function sectionDir(dataDir: string, section: MdSection): string {
  return path.join(dataDir, section);
}

function firstMarkdownHeading(content: string): string | undefined {
  for (const line of content.split("\n")) {
    const match = line.match(/^#\s+(.+)$/);
    if (match?.[1]) return match[1].trim();
  }
  return undefined;
}

function normalizeMdName(rawName: string): string {
  const withoutExtension = String(rawName || "").trim().replace(/\.md$/i, "");
  const normalized = withoutExtension
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (!normalized) throw new Error("Markdown filename is required");
  if (normalized.includes("/") || normalized.includes("\\") || normalized === "." || normalized === "..") {
    throw new Error("Markdown filename must be a simple .md basename");
  }
  return normalized;
}

function ensureMdExtension(name: string): string {
  return `${normalizeMdName(name)}.md`;
}

function allowedRoots(dataDir: string): string[] {
  return allowedSections.map((section) => path.resolve(sectionDir(dataDir, section)));
}

function isWithin(parent: string, child: string): boolean {
  const relative = path.relative(parent, child);
  return relative === "" || (!!relative && !relative.startsWith("..") && !path.isAbsolute(relative));
}

function resolveAllowedPath(dataDir: string, rawPath: string): { fsPath: string; section: MdSection; root: string } {
  const input = String(rawPath || "").trim();
  if (!input) throw new Error("Markdown path is required");
  const fsPath = path.resolve(input);
  if (path.extname(fsPath).toLowerCase() !== ".md") {
    throw new Error("Markdown editor can only access .md files");
  }

  for (const section of allowedSections) {
    const root = path.resolve(sectionDir(dataDir, section));
    if (isWithin(root, fsPath)) return { fsPath, section, root };
  }
  throw new Error("Markdown path must be inside .qubit/plans or .qubit/user-docs");
}

async function metadataForFile(section: MdSection, filePath: string): Promise<MdFileInfo> {
  const [stats, content] = await Promise.all([
    fs.stat(filePath),
    fs.readFile(filePath, "utf8").catch(() => ""),
  ]);
  const title = firstMarkdownHeading(content);
  return {
    section,
    name: path.basename(filePath, ".md"),
    ...(title ? { title } : {}),
    path: filePath,
    modifiedAt: stats.mtime.toISOString(),
    sizeBytes: stats.size,
  };
}

export async function listMdDocuments(dataDir: string): Promise<MdFileInfo[]> {
  const all = await Promise.all(allowedSections.map(async (section) => {
    const dir = sectionDir(dataDir, section);
    try {
      const entries = await fs.readdir(dir);
      const files = entries.filter((entry) => path.extname(entry).toLowerCase() === ".md");
      return await Promise.all(files.map((entry) => metadataForFile(section, path.join(dir, entry))));
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ENOENT") return [];
      throw error;
    }
  }));
  return all.flat().sort((a, b) => String(b.modifiedAt || "").localeCompare(String(a.modifiedAt || "")));
}

export async function readMdDocument(dataDir: string, request: { path?: string; section?: string; name?: string }) {
  const resolved = request.path
    ? resolveAllowedPath(dataDir, request.path)
    : (() => {
        const section = String(request.section || "");
        if (!isAllowedSection(section)) throw new Error("Markdown section must be plans or user-docs");
        const fsPath = path.join(sectionDir(dataDir, section), ensureMdExtension(String(request.name || "")));
        return resolveAllowedPath(dataDir, fsPath);
      })();
  const content = await fs.readFile(resolved.fsPath, "utf8");
  return { file: await metadataForFile(resolved.section, resolved.fsPath), content };
}

export async function createMdDocument(dataDir: string) {
  const dir = sectionDir(dataDir, "user-docs");
  await fs.mkdir(dir, { recursive: true });
  const stamp = new Date().toISOString().replace(/[-:]/g, "").replace(/\..+$/, "").replace("T", "-");
  for (let attempt = 0; attempt < 50; attempt += 1) {
    const suffix = attempt === 0 ? "" : `-${attempt + 1}`;
    const filePath = path.join(dir, `note-${stamp}${suffix}.md`);
    try {
      await fs.writeFile(filePath, "", { encoding: "utf8", flag: "wx" });
      return { file: await metadataForFile("user-docs", filePath), content: "" };
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "EEXIST") continue;
      throw error;
    }
  }
  throw new Error("Unable to create a unique user doc filename");
}

export async function saveMdDocument(dataDir: string, request: { path?: string; content?: string }) {
  const resolved = resolveAllowedPath(dataDir, String(request.path || ""));
  await fs.writeFile(resolved.fsPath, String(request.content ?? ""), "utf8");
  return { file: await metadataForFile(resolved.section, resolved.fsPath), content: String(request.content ?? "") };
}

export async function renameMdDocument(dataDir: string, request: { path?: string; name?: string; newName?: string }) {
  const resolved = resolveAllowedPath(dataDir, String(request.path || ""));
  const nextName = ensureMdExtension(String(request.newName || request.name || ""));
  const nextPath = path.join(resolved.root, nextName);
  const nextResolved = resolveAllowedPath(dataDir, nextPath);
  if (nextResolved.section !== resolved.section) throw new Error("Markdown files can only be renamed within their current section");
  if (path.resolve(nextResolved.fsPath) === path.resolve(resolved.fsPath)) {
    return { file: await metadataForFile(resolved.section, resolved.fsPath), previousPath: resolved.fsPath };
  }
  try {
    await fs.access(nextResolved.fsPath);
    throw new Error(`Markdown file already exists: ${nextName}`);
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code !== "ENOENT") throw error;
  }
  await fs.rename(resolved.fsPath, nextResolved.fsPath);
  return { file: await metadataForFile(resolved.section, nextResolved.fsPath), previousPath: resolved.fsPath };
}
