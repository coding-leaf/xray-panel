<!-- TRELLIS:START -->
# Trellis Instructions

These instructions are for AI assistants working in this project.

This project is managed by Trellis. The working knowledge you need lives under `.trellis/`:

- `.trellis/workflow.md` — development phases, when to create tasks, skill routing
- `.trellis/spec/` — package- and layer-scoped coding guidelines (read before writing code in a given layer)
- `.trellis/workspace/` — per-developer journals and session traces
- `.trellis/tasks/` — active and archived tasks (PRDs, research, jsonl context)

If a Trellis command is available on your platform (e.g. `/trellis:finish-work`, `/trellis:continue`), prefer it over manual steps. Not every platform exposes every command.

If you're using Codex, OpenCode, Claude Code, or another agent-capable tool, additional project-scoped helpers live in:
- `.agents/skills/` — reusable Trellis skills (`trellis-brainstorm`, `trellis-check`, `trellis-before-dev`, etc.)
- `.opencode/agents/` / `.codex/agents/` / `.claude/agents/` — Trellis subagents (`trellis-implement`, `trellis-check`, `trellis-research`)

Managed by Trellis. Edits outside this block are preserved; edits inside may be overwritten by a future `trellis update`.

<!-- TRELLIS:END -->

# xray-panel 全局工程协议与门禁

终端实际运行结果与源码是唯一事实源。本项目运行环境已由 `mise` 统一接管。

---

## 1. 核心质量门禁 (Quality Gates)

在汇报任何任务完成前，必须在终端执行并通过对应门禁：

```bash
# 后端测试与构建
mise x -- go test ./...             # 单元测试全套 (必须全绿)
mise x -- go vet ./...              # 静态检查 (必须无 warning)
mise x -- go build .                # 后端完整二进制构建

# 前端构建与检查
cd web && mise x -- npm run build   # 前端 Vite 生产构建与静态类型验证
```

注：Windows 下如需执行带竞态检查的测试，请使用 `CGO_ENABLED=1 mise x -- go test -race ./...`。

---

## 2. 架构拓扑与事实源 (Architecture & Truth)

- **事实源唯一性**：终端测试运行结果与有效源码 >> 架构契约与 Spec >> 历史记录。
- **架构分层边界**：
  - `internal/domain`：纯业务逻辑与领域模型，保持无外部副作用；
  - `internal/service`：跨领域用例编排与核心业务流；
  - `internal/adapter`：外部系统（Xray-core gRPC、SQLite/GORM 仓储、Telegram Bot、HTTP 交付层）；
  - `web/`：Vue 3 + Tailwind CSS + Vite 交互面板。

---

## 3. 核心红线 (Must-Not)

- **契约优先**：严禁在技术方案获确认前擅自修改公共 API 契约、数据库 Schema 或核心持久化格式。
- **并发与资源安全**：严禁在并发路径中遗漏互斥保护，严禁产生 goroutine 泄漏与未关闭的 Response/连接。
- **生产敏感防线**：严禁将真实公网 IP、生产域名、Token、私钥或敏感凭证写入代码、测试或配置。
- **杜绝静默吞错**：严禁静默吞掉 error 或忽略未处理的异常返回值。
- **KISS 极简规范**：严禁过度设计，严禁单实现 interface、空转包装类与无痛点工厂层。

---

## 4. 典型错误防御 (Things AI Gets Wrong)

- **并发竞态**：并发读写共享状态时遗漏互斥锁或原子操作。
- **伪造测试**：自称测试已通过但未在终端实际执行，或通过削弱断言掩盖回归。
- **巨石任务**：面对复杂诉求试图立项“大爆炸重构”，必须按 Trellis 规范拆解为清晰的 Milestone 实施。
- **跨平台脚本执行**：Windows 宿主下运行 Python 脚本统一使用 `python .trellis/scripts/...`，不要调用缺失的 Linux 专属命令。

---

## 5. Trellis 协同规范 (Workflow & Subagents)

- **新功能与复杂重构**：按 Trellis 三阶段流转：
  - Phase 1 (Plan): 激活 `trellis-brainstorm` / `trellis-before-dev`，讨论需求并建立 task 工件 (`prd.md`, `design.md`, `implement.md`)；多里程碑任务首批仅细化 `M1` 全套工件（`M2..Mn` 先留轻量 stub，避免主会话长时间静默）。任何阶段派发 Swarm / 子代理（含只读调研/审计代理）前，**必须显式请示用户确认执行阵型与子代理模型**（除非用户当轮已明确指定；严禁静默继承高配主模型或通过环境变量/Hook 暗改）；
  - Phase 2 (Execute): 在 Claude/Codex 下委派 `trellis-implement`；在 Antigravity 下按用户授权显式传入 `model`，走 **Swarm (`Worker + Verifier`，钩子自动注入绝对路径规范；单批次用 `git merge --squash` 归拢，未提交多里程碑串行批次用 `refs/swarm-opt/*` + `git show -p --format= <commit> | git apply` 增量归拢)** 或轻量 **Inline** 直改（Swarm 子代理严禁修改 `.trellis/**`）；
  - Phase 3 (Finish): 激活 `trellis-check` 执行最终全量门禁与 spec 对照，`trellis-update-spec` 沉淀经验后确认提交并归档（主分支直改未走 PR 分支时归档加 `--skip-branch-validation`）。
- **缺陷排查与修复**：严格遵循 Fail-repro First（失败测试先行复现，红灯后再编写修复代码，最后绿灯确认）。
