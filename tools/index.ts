import type { AnyToolDefinition } from '@hyper-labs/hyper-router'
import { bashTool } from './bash.js'
import { createFileTool } from './createFile.js'
import { deleteFileTool } from './deleteFile.js'
import { editFileTool, multiEditTool } from './editFile.js'
import { globTool } from './glob.js'
import { powershellTool } from './powershell.js'
import { readFileContinuationTool, readFileTool } from './readFile.js'
import { readFilesTool } from './readFiles.js'
import { ripgrepTool } from './ripgrep.js'
import { todoMdTool } from './todoMd.js'

export const qubitTools: AnyToolDefinition[] = [
  readFileTool,
  readFileContinuationTool,
  readFilesTool,
  globTool,
  ripgrepTool,
  bashTool,
  powershellTool,
  createFileTool,
  editFileTool,
  multiEditTool,
  deleteFileTool,
  todoMdTool,
]
