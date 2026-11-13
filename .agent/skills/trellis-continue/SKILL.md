---
name: trellis-continue
description: "Resume an active Trellis task ONLY when the user explicitly asks to continue/resume prior task work or when <workflow-state> is missing. Do NOT auto-load when Status: no_task."
---

# Continue Current Task (Antigravity Lean Mode)

1. If `<workflow-state>` is already present and shows `Status: no_task`, **STOP** — there is no active task to continue.
2. Otherwise inspect active task state and pull **only** the single phase step you need (do NOT dump the full `workflow.md`):
   ```bash
   python ./.trellis/scripts/get_context.py
   python ./.trellis/scripts/get_context.py --mode phase --step <step>
   ```
