# Start Session

Initialize or refresh a Trellis-managed development session. Note: `.agent/hooks.json` now auto-injects `SessionStart`, per-turn `PreInvocation`, and `PreToolUse` Swarm/subagent spec manifests; run the steps below manually only when refreshing mid-session or if hooks are disabled.

---

## Step 1: Current state
Identity, git status, current task, active tasks, journal location.

```bash
python ./.trellis/scripts/get_context.py
```

If this output includes a line beginning `Trellis update available:`, copy the full line verbatim when summarizing session context. Do not shorten operational command hints.

## Step 2: Workflow overview
Compact Phase Index, request triage rules, planning artifact contract, and the step-detail command.

```bash
python ./.trellis/scripts/get_context.py --mode phase
```

Full guide in `.trellis/workflow.md` (read on demand).

## Step 3: Guideline indexes
Discover packages + spec layers, then read each relevant index file.

```bash
python ./.trellis/scripts/get_context.py --mode packages
cat .trellis/spec/guides/index.md
cat .trellis/spec/<package>/<layer>/index.md   # for each relevant layer
```

Index files list the specific guideline docs to read when you actually start coding.

## Step 4: Decide next action
From Step 1 you know the current task and status. Check the task directory:

- **Active task status `planning` + no `prd.md`** → Phase 1.1. Load the `trellis-brainstorm` skill.
- **Active task status `planning` + `prd.md` exists** → stay in Phase 1. Lightweight tasks can be PRD-only; complex tasks need `design.md` + `implement.md`. Load the relevant Phase 1 step detail before `task.py start`.
- **Active task status `in_progress`** → Phase 2 step 2.1. Load the step detail:
  ```bash
  python ./.trellis/scripts/get_context.py --mode phase --step 2.1 --platform antigravity
  ```
- **No active task** → classify first. For simple conversation / small task, ask only whether this turn should create a Trellis task. For complex work, ask in ONE prompt whether to create a Trellis task and which `model` child agents should use (`flash`/`pro`/`inherit` tier or exact model ID; never silently inherit or override via env/hooks). At Phase 1.4 (`task.py start`), the model autonomously recommends ONE Phase 2 formation (`Subagent 三把斧` vs `Swarm` vs `Inline`) without requiring `/swarm`.

---

## Skill & Agent routing (quick reference)

| User intent / Phase | Tool / Skill / Agent |
|---|---|
| New feature / unclear requirements | `trellis-brainstorm` |
| Heavy research (>3 slices / `trellis mem` / SSH) | `invoke_subagent` `trellis-research` (tier model) or read-only `swarm` `researcher` (exact model ID) |
| About to write code (Inline mode) | `trellis-before-dev` |
| Single-track Phase 2 execution (tier model) | `Subagent 三把斧`: `trellis-implement` -> `trellis-check` via `.agent/plugins/trellis-agy` (specs auto-injected via `.agent/hooks.json`) |
| Multi-slice parallel / adversarial / exact-model Phase 2 | `swarm` (`Worker(s) + verifier`, explicit user-approved `model`, specs auto-injected via `.agent/hooks.json`) |
| Done coding / quality check | `trellis-check` (subagent or skill) |
| Stuck / fixed same bug multiple times | `trellis-break-loop` |
| Learned something worth capturing | `trellis-update-spec` |

Full rules + anti-rationalization table in `.trellis/workflow.md`.
