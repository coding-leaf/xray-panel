/* global process */
/**
 * Trellis Session Start Plugin
 *
 * Injects compact SessionStart context into the copy of the latest user
 * message that OpenCode sends to the model. Uses V2 `ctx.session.hook("context")`
 * and V1 `server()` with `experimental.chat.messages.transform` so TUI / Web / SQLite
 * history stay untouched.
 */

import { TrellisContext, debugLog, isTrellisSubagent } from "../lib/trellis-context.js"
import {
  MESSAGES_TRANSFORM_HOOK,
  platformInputFromMessages,
  prependEphemeralText,
  transcriptHasAssistantMessage,
} from "../lib/context-visibility.js"
import { buildSessionContext } from "../lib/session-utils.js"

const FIRST_REPLY_NOTICE_RE = /<first-reply-notice>[\s\S]*?<\/first-reply-notice>\s*/g

function stripFirstReplyNotice(context) {
  return context.replace(FIRST_REPLY_NOTICE_RE, "")
}

function handleContextInjection(ctx, messages, platformInput) {
  const agent = platformInput?.agent || "unknown"
  debugLog("session", "handleContextInjection called, agent:", agent)

  if (isTrellisSubagent(platformInput)) {
    debugLog("session", "Skipping trellis subagent turn:", agent)
    return
  }

  if (process.env.TRELLIS_HOOKS === "0" || process.env.TRELLIS_DISABLE_HOOKS === "1") {
    debugLog("session", "Skipping - TRELLIS_HOOKS disabled")
    return
  }

  if (process.env.OPENCODE_NON_INTERACTIVE === "1") {
    debugLog("session", "Skipping - non-interactive mode")
    return
  }

  let context = buildSessionContext(ctx, platformInput)
  if (transcriptHasAssistantMessage(messages)) {
    context = stripFirstReplyNotice(context)
  }
  debugLog("session", "Built context, length:", context.length)
  prependEphemeralText(messages, context)
}

export default {
  id: "session-start",
  async setup(opencodeCtx) {
    const directory = opencodeCtx?.directory ?? opencodeCtx?.location?.directory ?? process.cwd()
    const ctx = new TrellisContext(directory)
    debugLog("session", "Plugin loaded (V2), directory:", directory)

    if (opencodeCtx?.session?.hook) {
      await opencodeCtx.session.hook("context", async (event) => {
        try {
          const messages = event.messages
          const platformInput = {
            agent: event.agent,
            sessionID: event.sessionID,
            ...platformInputFromMessages(messages),
          }
          handleContextInjection(ctx, messages, platformInput)
        } catch (error) {
          debugLog("session", "Error in context hook:", error?.message, error?.stack)
        }
      })
    }
  },
  async server({ directory }) {
    const ctx = new TrellisContext(directory)
    debugLog("session", "Plugin loaded (V1), directory:", directory)

    return {
      [MESSAGES_TRANSFORM_HOOK]: async (_input, output) => {
        try {
          const messages = output?.messages
          const platformInput = platformInputFromMessages(messages)
          handleContextInjection(ctx, messages, platformInput)
        } catch (error) {
          debugLog("session", "Error in messages.transform:", error?.message, error?.stack)
        }
      },
    }
  },
}
