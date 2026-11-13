---
name: trellis-start
description: "Manual fallback to inspect Trellis session context ONLY when explicitly requested by the user (e.g. /start) or when <trellis-session-context> hook output is missing. Do NOT auto-load on normal turns in Antigravity — .agent/hooks.json already injects session state and <workflow-state>."
---

# Start Session (Antigravity Hook-Managed Fallback)

In Antigravity, `.agent/hooks.json` (`session-start` and `pre-invocation`) automatically injects `<trellis-session-context>` and `<workflow-state>`.

1. **If `<trellis-session-context>` or `<workflow-state>` is already present in this conversation**: **STOP immediately.** Do NOT run `get_context.py` and do NOT read `.trellis/workflow.md`. Follow `<workflow-state>` directly.
2. **Only if hook output is missing or the user explicitly asked to reload session status**:
   ```bash
   python ./.trellis/scripts/get_context.py
   ```
