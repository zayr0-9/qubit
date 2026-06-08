import { readFile } from 'node:fs/promises'
import { join } from 'node:path'

export type PromptMode = 'plan' | 'edit'

export const baseInstructions = 'You are Qubit, a concise terminal coding assistant MVP. Be helpful, direct, and practical. Keep answers brief unless the user asks for detail.'

export const defaultModePrompts: Record<PromptMode, string> = {
  plan: 'You are in plan mode: reason carefully and propose changes before editing.',
  edit: 'You are in edit mode: implement changes directly and validate them.',
}

export const defaultSubagentPrompt = 'You are a delegated Qubit subagent running inside a hidden child session. You are a small, fast helper model: do focused exploration or web research for the parent agent, return concise evidence and sources, and do not formulate deep plans or make final decisions.'

export interface PromptBuilder {
  instructionsForMode(mode: unknown): string
  instructionsForSubagent(): string
}

export function normalizePromptMode(mode: unknown): PromptMode {
  const normalized = String(mode || '').trim().toLowerCase()
  if (['edit', 'always', 'always_allow', 'always-allow', 'allow'].includes(normalized)) return 'edit'
  return 'plan'
}

export async function loadPromptFile(promptsDir: string, name: string, fallback: string): Promise<string> {
  try {
    const content = (await readFile(join(promptsDir, `${name}.md`), 'utf8')).trim()
    return content || fallback
  } catch {
    return fallback
  }
}

export async function loadModePrompts(promptsDir: string): Promise<Record<PromptMode, string>> {
  const entries = await Promise.all(
    (Object.keys(defaultModePrompts) as PromptMode[]).map(async (mode) => [
      mode,
      await loadPromptFile(promptsDir, mode, defaultModePrompts[mode]),
    ] as const),
  )
  return Object.fromEntries(entries) as Record<PromptMode, string>
}

export async function createPromptBuilder(promptsDir: string): Promise<PromptBuilder> {
  const modePrompts = await loadModePrompts(promptsDir)
  const subagentPrompt = await loadPromptFile(promptsDir, 'subagent', defaultSubagentPrompt)

  return {
    instructionsForMode(mode: unknown): string {
      const normalized = normalizePromptMode(mode)
      const modePrompt = modePrompts[normalized] || defaultModePrompts[normalized]
      return [baseInstructions, modePrompt].filter(Boolean).join('\n\n')
    },

    instructionsForSubagent(): string {
      return subagentPrompt
    },
  }
}
