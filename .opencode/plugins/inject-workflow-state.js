/* global process */
/**
 * Trellis Workflow State Injection Plugin
 *
 * Per-turn UserPromptSubmit equivalent for OpenCode.
 *
 * Injects a short <workflow-state> breadcrumb into the in-memory copy
 * of the latest user message. Supports both V2 `ctx.session.hook("context")`
 * and V1 `server()` with `experimental.chat.messages.transform`.
 */

import { existsSync, readFileSync } from "fs"
import { join } from "path"
import {
  MESSAGES_TRANSFORM_HOOK,
  latestUserPromptText,
  platformInputFromMessages,
  prependEphemeralText,
} from "../lib/context-visibility.js"
import { TrellisContext, debugLog, isTrellisSubagent } from "../lib/trellis-context.js"

// Supports STATUS values with letters, digits, underscores, hyphens
const TAG_RE = /\[workflow-state:([A-Za-z0-9_-]+)\]\s*\n([\s\S]*?)\n\s*\[\/workflow-state:\1\]/g

const DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD = "no-trellis"

function stripInlineComment(value) {
  let inQuote = null
  for (let idx = 0; idx < value.length; idx++) {
    const ch = value[idx]
    if (inQuote) {
      if (ch === inQuote) inQuote = null
      continue
    }
    if (ch === '"' || ch === "'") {
      inQuote = ch
      continue
    }
    if (ch === "#" && (idx === 0 || /\s/.test(value[idx - 1]))) return value.slice(0, idx)
  }
  return value
}

function unquoteYaml(s) {
  if (s.length >= 2 && s[0] === s[s.length - 1] && (s[0] === '"' || s[0] === "'")) return s.slice(1, -1)
  return s
}

function readSkipKeyword(directory) {
  const path = join(directory, ".trellis", "config.yaml")
  if (!existsSync(path)) return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD
  let text
  try {
    text = readFileSync(path, "utf-8")
  } catch {
    return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD
  }

  let inSection = false
  let sectionIndent = -1
  for (const rawLine of text.split(/\r?\n/)) {
    const trimmed = rawLine.trim()
    if (!inSection) {
      if (/^prompt_injection\s*:\s*(#.*)?$/.test(trimmed)) {
        inSection = true
        sectionIndent = rawLine.length - rawLine.trimStart().length
      }
      continue
    }
    if (!trimmed || trimmed.startsWith("#")) continue
    const indent = rawLine.length - rawLine.trimStart().length
    if (indent <= sectionIndent) break
    const m = trimmed.match(/^skip_keyword\s*:\s*(.*)$/)
    if (!m) continue
    return unquoteYaml(stripInlineComment(m[1]).trim())
  }
  return DEFAULT_PROMPT_INJECTION_SKIP_KEYWORD
}

function promptHasSkipKeyword(text, keyword) {
  if (!keyword || typeof text !== "string") return false
  const escaped = keyword.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")
  const pattern = new RegExp(`(?<![\\w-])${escaped}(?![\\w-])`, "i")
  return pattern.test(text)
}

function loadBreadcrumbs(directory) {
  const workflowPath = join(directory, ".trellis", "workflow.md")
  if (!existsSync(workflowPath)) return {}
  let content
  try {
    content = readFileSync(workflowPath, "utf-8")
  } catch {
    return {}
  }
  const result = {}
  for (const match of content.matchAll(TAG_RE)) {
    const status = match[1]
    const body = match[2].trim()
    if (body) result[status] = body
  }
  return result
}

function getActiveTask(ctx, platformInput = null) {
  const active = ctx.getActiveTask(platformInput)
  const taskRef = active.taskPath
  if (!taskRef) return null
  const taskDir = ctx.resolveTaskDir(taskRef)
  if (active.stale || !taskDir || !existsSync(taskDir)) {
    return { id: taskRef.split("/").pop(), status: "stale", source: active.source }
  }
  const taskJsonPath = join(taskDir, "task.json")
  if (!existsSync(taskJsonPath)) return null
  try {
    const data = JSON.parse(readFileSync(taskJsonPath, "utf-8"))
    const status = typeof data.status === "string" ? data.status : ""
    if (!status) return null
    const id = data.id || taskRef.split("/").pop()
    return { id, status, source: active.source }
  } catch {
    return null
  }
}

function buildBreadcrumb(id, status, templates) {
  let body = templates[status]
  if (body === undefined) {
    body = "Refer to workflow.md for current step."
  }
  let header = id === null ? `Status: ${status}` : `Task: ${id} (${status})`
  return `<workflow-state>\n${header}\n${body}\n</workflow-state>`
}

function handleWorkflowStateInjection(ctx, directory, messages, platformInput) {
  if (isTrellisSubagent(platformInput)) {
    debugLog("workflow-state", "Skipping trellis subagent turn:", platformInput?.agent)
    return
  }
  if (process.env.TRELLIS_HOOKS === "0" || process.env.TRELLIS_DISABLE_HOOKS === "1") {
    return
  }
  if (process.env.OPENCODE_NON_INTERACTIVE === "1") {
    return
  }
  if (!ctx.isTrellisProject()) {
    return
  }

  const originalText = latestUserPromptText(messages)
  if (promptHasSkipKeyword(originalText, readSkipKeyword(directory))) {
    debugLog("workflow-state", "Skipping turn: skip keyword present in prompt")
    return
  }

  const templates = loadBreadcrumbs(directory)
  const task = getActiveTask(ctx, platformInput)
  const breadcrumb = task
    ? buildBreadcrumb(task.id, task.status, templates, task.source)
    : buildBreadcrumb(null, "no_task", templates)

  prependEphemeralText(messages, breadcrumb)
  debugLog(
    "workflow-state",
    "Injected breadcrumb for task",
    task ? task.id : "none",
    "status",
    task ? task.status : "no_task",
  )
}

export default {
  id: "inject-workflow-state",
  async setup(opencodeCtx) {
    const directory = opencodeCtx?.directory ?? opencodeCtx?.location?.directory ?? process.cwd()
    const ctx = new TrellisContext(directory)
    debugLog("workflow-state", "Plugin loaded (V2), directory:", directory)

    if (opencodeCtx?.session?.hook) {
      await opencodeCtx.session.hook("context", async (event) => {
        try {
          const messages = event.messages
          const platformInput = {
            agent: event.agent,
            sessionID: event.sessionID,
            ...platformInputFromMessages(messages),
          }
          handleWorkflowStateInjection(ctx, directory, messages, platformInput)
        } catch (error) {
          debugLog(
            "workflow-state",
            "Error in context hook:",
            error instanceof Error ? error.message : String(error),
          )
        }
      })
    }
  },
  async server({ directory }) {
    const ctx = new TrellisContext(directory)
    debugLog("workflow-state", "Plugin loaded (V1), directory:", directory)

    return {
      [MESSAGES_TRANSFORM_HOOK]: async (_input, output) => {
        try {
          const messages = output?.messages
          const platformInput = platformInputFromMessages(messages)
          handleWorkflowStateInjection(ctx, directory, messages, platformInput)
        } catch (error) {
          debugLog(
            "workflow-state",
            "Error in messages.transform:",
            error instanceof Error ? error.message : String(error),
          )
        }
      },
    }
  },
}
