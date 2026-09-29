# Bootstrap Task: Fill Project Development Guidelines

**You (the AI) are running this task. The developer does not read this file.**

The developer just ran `trellis init` on this project for the first time.
`.trellis/` now exists with empty spec scaffolding, and this bootstrap task
exists under `.trellis/tasks/`. When they want to work on it, they should start
this task from a session that provides Trellis session identity.

**Your job**: help them populate `.trellis/spec/` with the team's real
coding conventions. Every future AI session — this project's
`trellis-implement` and `trellis-check` sub-agents — auto-loads spec files
listed in per-task jsonl manifests. Empty spec = sub-agents write generic
code. Real spec = sub-agents match the team's actual patterns.

Don't dump instructions. Open with a short greeting, figure out if the repo
has any existing convention docs (CLAUDE.md, .cursorrules, etc.), and drive
the rest conversationally.

---

## Session Scope (coding-leaf, 2026-09-29)

**来源需求**：按项目真实源码整理 `.trellis/` 下的 spec 文档，使 `trellis-implement`
/ `trellis-check` 子代理加载的规范与代码事实一致（消除占位符与 aspiration）。

**已确认决策**：

- 复用本 bootstrap 任务，不另建任务。
- 前端 spec 遵循 "Document reality"：明确记录当前无 store / composable 的事实，
  不写引导引入 Pinia Store 的理想化描述。
- 前端 spec 拆为多个子文档，`index.md` 作索引（与 backend 形态一致）。

**Deliverables**：

1. 补全 `.trellis/spec/backend/logging-guidelines.md`（当前为纯占位符）。
2. 扩展前端 spec：`frontend/index.md` + 子文档（目录结构、API 与数据流、组件与路由、质量门禁）。
3. 校正已填的 backend spec 与真实源码之间的偏差。
4. 为本任务配置 `implement.jsonl` / `check.jsonl` manifest。

**Acceptance criteria**：

- 所有 spec 结论可用 `file:line` 证据回溯到真实代码；无 `(To be filled by the team)` 类占位符。
- `logging-guidelines.md` 覆盖：日志库与构造、注入/共享方式、等级使用现状、标准字段、
  敏感信息处理、输出目标与配置项。
- 前端子文档覆盖：真实目录结构、无 Store 的现状、axios 数据流与 mock、路由与守卫、
  SFC 约定、`strict: false` 与无前端测试的现状。
- 不得存在与源码矛盾的描述（例如不得再写 "状态管理：Pinia（单例 Store）"）。
- `python ./.trellis/scripts/task.py validate 00-bootstrap-guidelines` 通过。

**Non-goals**：不重构源码；不改公共 API 契约、DB Schema；不引入前端测试框架；
不修改 `spec/guides/` 中的通用思考指南（除非发现明显不适配）。

---

## Status (update the checkboxes as you complete each item)

- [x] Fill backend guidelines（logging-guidelines.md 已补全；database/error-handling 偏差已校正）
- [x] Add code examples（全部结论附 `file:line` 证据）
- [x] Extend frontend guidelines（index + 4 个子文档，写实）
- [x] Curate implement.jsonl / check.jsonl（9 / 7 条，validate 通过）

**验收结果**：`go vet` / `go build` / `web npm run build` 全部通过；
`go test ./...` 存在**与本任务无关的既有 Windows 测试失败**，
详见 `implement.md` 的「门禁发现」。

---

## Spec files to populate


### Backend guidelines

| File | What to document |
|------|------------------|
| `.trellis/spec/backend/directory-structure.md` | Where different file types go (routes, services, utils) |
| `.trellis/spec/backend/database-guidelines.md` | ORM, migrations, query patterns, naming conventions |
| `.trellis/spec/backend/error-handling.md` | How errors are caught, logged, and returned |
| `.trellis/spec/backend/logging-guidelines.md` | Log levels, format, what to log |
| `.trellis/spec/backend/quality-guidelines.md` | Code review standards, testing requirements |


### Thinking guides (already populated)

`.trellis/spec/guides/` contains general thinking guides pre-filled with
best practices. Customize only if something clearly doesn't fit this project.

---

## How to fill the spec

### Step 1: Import from existing convention files first (preferred)

Search the repo for existing convention docs. If any exist, read them and
extract the relevant rules into the matching `.trellis/spec/` files —
usually much faster than documenting from scratch.

| File / Directory | Tool |
|------|------|
| `CLAUDE.md` / `CLAUDE.local.md` | Claude Code |
| `AGENTS.md` | Codex / Claude Code / agent-compatible tools |
| `.cursorrules` | Cursor |
| `.cursor/rules/*.mdc` | Cursor (rules directory) |
| `.windsurfrules` | Windsurf |
| `.clinerules` | Cline |
| `.roomodes` | Roo Code |
| `.github/copilot-instructions.md` | GitHub Copilot |
| `.vscode/settings.json` → `github.copilot.chat.codeGeneration.instructions` | VS Code Copilot |
| `CONVENTIONS.md` / `.aider.conf.yml` | aider |
| `CONTRIBUTING.md` | General project conventions |
| `.editorconfig` | Editor formatting rules |

### Step 2: Analyze the codebase for anything not covered by existing docs

Scan real code to discover patterns. Before writing each spec file:
- Find 2-3 real examples of each pattern in the codebase.
- Reference real file paths (not hypothetical ones).
- Document anti-patterns the team clearly avoids.

### Step 3: Document reality, not ideals

**Critical**: write what the code *actually does*, not what it should do.
Sub-agents match the spec, so aspirational patterns that don't exist in the
codebase will cause sub-agents to write code that looks out of place.

If the team has known tech debt, document the current state — improvement
is a separate conversation, not a bootstrap concern.

---

## Quick explainer of the runtime (share when they ask "why do we need spec at all")

- Every AI coding task spawns two sub-agents: `trellis-implement` (writes
  code) and `trellis-check` (verifies quality).
- Each task has `implement.jsonl` / `check.jsonl` manifests listing which
  spec files to load.
- The platform hook auto-injects those spec files + the task's `prd.md`
  into every sub-agent prompt, so the sub-agent codes/reviews per team
  conventions without anyone pasting them manually.
- Source of truth: `.trellis/spec/`. That's why filling it well now pays
  off forever.

---

## Completion

When the developer confirms the checklist items above are done with real
examples (not placeholders), guide them to run:

```bash
python ./.trellis/scripts/task.py finish
python ./.trellis/scripts/task.py archive 00-bootstrap-guidelines
```

After archive, every new developer who joins this project will get a
`00-join-<slug>` onboarding task instead of this bootstrap task.

---

## Suggested opening line

"Welcome to Trellis! Your init just set me up to help you fill the project
spec — a one-time setup so every future AI session follows the team's
conventions instead of writing generic code. Before we start, do you have
any existing convention docs (CLAUDE.md, .cursorrules, CONTRIBUTING.md,
etc.) I can pull from, or should I scan the codebase from scratch?"
