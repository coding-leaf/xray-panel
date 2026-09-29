# Implement — Spec 文档整理（00-bootstrap-guidelines）

> 有序执行清单。每步完成后运行对应校验；失败则回退该步再继续。

---

## 前置

- [x] 0. 读取 `prd.md` / `design.md`，确认范围与证据锚点。
  校验：`python ./.trellis/scripts/task.py current` 输出 `.trellis/tasks/00-bootstrap-guidelines` ✓

---

## Step 1 — 补全 backend/logging-guidelines.md

- [x] 1.1 按 `design.md` §3 证据重写 `.trellis/spec/backend/logging-guidelines.md`，覆盖：
  日志库与构造、注入/共享方式、等级使用现状、标准字段、敏感信息处理、输出与配置。
- [x] 1.2 删除所有 `(To be filled by the team)` 与 HTML 注释占位符。
- 校验：`grep -rn "To be filled" .trellis/spec/backend/` 无输出 ✓

---

## Step 2 — 校正既有 backend spec

- [x] 2.1 逐条核对 `index.md` / `directory-structure.md` / `database-guidelines.md`
  / `error-handling.md` / `quality-guidelines.md` 中的可验证声明。
- [x] 2.2 修正与源码不符处，并给关键结论补 `file:line`。**实际发现并修正 2 处偏差**：
  - `database-guidelines.md`：DSN 补 `&_pragma=synchronous(NORMAL)`、
    连接池补 `SetMaxIdleConns(1)`（`sqlite_db.go:27,37-38`）；
    `Omit` 示例字段改为真实的 `up_bytes`/`down_bytes` + `.Save()`（`user_repo.go:30`）。
  - `error-handling.md`：删除与代码不符的 `{"code":40001,"message":...}` 臆造格式，
    改为真实的 `gin.H{"error": ...}` + HTTP 状态码（`handler_auth.go:41`），
    并说明 Reality 的 `{code,msg,data}` 信封是特例（`handler_inbound.go:114-158`）。
  - `index.md` 补 `internal/config/` 目录（`internal/config/config.go:39-40`）。
- [x] 2.3 `index.md` 索引表中 `logging-guidelines.md` 状态为 Active。
- 校验：核对的包路径全部存在；无与源码矛盾的表述 ✓

---

## Step 3 — 扩展前端 spec

- [x] 3.1 改写 `frontend/index.md`：技术栈 + 子文档索引；
  删除 "状态管理：Pinia（单例 Store…）"，改为写实说明。
- [x] 3.2 新增 `frontend/directory-structure.md`（真实 6 个子目录、状态存放三类方式、命名）。
- [x] 3.3 新增 `frontend/api-and-dataflow.md`（axios 单例、拦截器、Bearer、401、mock 分流、toast、类型现状）。
- [x] 3.4 新增 `frontend/components-and-routing.md`（`<script setup lang="ts">`、无 props/emit、相对路径导入、懒加载路由、守卫、Tailwind 设计类、lucide）。
- [x] 3.5 新增 `frontend/quality-guidelines.md`（`typecheck`+`build` 门禁、无测试、`strict:false`、无组件库/i18n）。
- 校验：五个文件均存在 ✓；无 "Pinia（单例 Store" 残留 ✓

---

## Step 4 — 配置 context manifest

- [x] 4.1 写入 `implement.jsonl`（9 条）。
- [x] 4.2 写入 `check.jsonl`（7 条）。
- [x] 4.3 未写入 seed `_example` 行（直接写真实条目）。
- 校验：`python ./.trellis/scripts/task.py validate 00-bootstrap-guidelines` → 全部通过 ✓

---

## Step 5 — 全量门禁与收尾

- [x] 5.1 `mise x -- go build .` → exit 0 ✓
- [x] 5.2 `cd web && mise x -- npm run build` → exit 0（built in 4.20s）✓
- [x] 5.3 `mise x -- go vet ./...` → exit 0 ✓
- [ ] 5.4 `go test ./...` — **未通过，但与本次变更无关（预先存在）**，详见下方「门禁发现」。
- [ ] 5.5 回填 `prd.md` 的 Status 勾选。
- [ ] 5.6 `python ./.trellis/scripts/task.py finish` 后 `archive 00-bootstrap-guidelines`（待用户确认）。

---

## 门禁发现（如实记录）

`go test ./...` 红，失败包：`panel/internal/service`、`panel/internal/delivery/http`。

- `internal/service`：多个 `TestTicketService_*` 在 `TempDir RemoveAll cleanup` 阶段报
  `The process cannot access the file ... test_ticket_service.db` — Windows 下 SQLite
  测试库句柄未在清理前释放导致的 teardown 失败。
- `internal/delivery/http`：整包 124s 超时（`Server.Start` 起了真实 `ListenAndServe`，`server.go:82-84`）。

**判定为预先存在且与本次变更无关**，依据：本次改动仅涉及 `.trellis/**`（untracked 文档）与
构建产物，`git status` 中**无任何 `internal/**` 源文件被修改**；Go 测试不读取 `.trellis/`。
如需修复，应作为独立任务（先复现红灯，再改测试/连接关闭逻辑）。

---

## 回滚点

- 每 Step 完成后为一个回滚点：`git checkout -- .trellis/spec/backend` 等；
  新增文件直接删除。Step 2 若发现既有文档大面积失准，先记录差异清单再决定是否拆分新任务。
