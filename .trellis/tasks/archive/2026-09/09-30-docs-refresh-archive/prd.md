# 文档刷新与归档：README / 架构总览 / SDLC 封存

## Goal

让仓库入口文档（README、架构总览）与当前源码事实一致，并建立"过时文档原路径出新版、旧版封存到 `docs/archive/`"的归档惯例。同时停用并封存已过时的 `docs/sdlc/` 机制。

## Background / Evidence

已核实的漂移点（证据来自源码与终端）：

| # | 漂移 | 证据 |
|---|---|---|
| E1 | 版本号自相矛盾：代码 `v2.5.0` vs README/CHANGELOG `v2.6.0-beta.1` | `main.go:32` vs `README.md:4` / `README.md:72` |
| E2 | Go 版本过时：README 写 `Go 1.22+`，实际 `go 1.26` | `README.md:5,223` / `go.mod:3` / `mise.toml` |
| E3 | 启动参数表缺 4 个 flag | `README.md:208-216` vs `internal/config/config.go:30-42` |
| E4 | 架构总览仍为 v2.0.0，缺 v2.1~v2.6 全部演进 | `docs/ARCHITECTURE_AND_LOGIC.md:1,101` |
| E5 | `docs/sdlc/` 机制已停用：仅 7 目录/7 行，8 个 v2.6 前端任务未归档 | `docs/sdlc/` vs `.trellis/tasks/archive/2026-09` |

## Requirements

- **R1 版本单一事实源**：以 README / CHANGELOG 的 `v2.6.0-beta.1` 为准，同步更新 `main.go` 的 `Version`；文档中不得再出现与代码冲突的版本数字。
- **R2 README 校正**：
  - R2.1 徽章 Go 版本改为与实际工具链一致（`go 1.26`）；
  - R2.2 启动参数表补齐 `internal/config` 实际支持的全部对外 flag（`-log-level`、`-log-json`、`-public-url`、`-v`/`-version`），默认值与环境变量名逐项核对；
  - R2.3 架构分层树与 `internal/` 实际目录一致（补齐 `internal/config`、`internal/pkg/*`、`internal/adapter/reality`、`internal/adapter/xray/proto`、`internal/delivery/cron` 等，删除幽灵条目）。
- **R3 架构总览重写**：`docs/ARCHITECTURE_AND_LOGIC.md` 按 v2.6.0-beta.1 重写，覆盖 v2.1~v2.6 核心子系统（SubRoute 用户隔离、Reality 域名巡检、Telegram Bot、审计日志、GeoData、Alert、多协议 hysteria2/socks/vmess/shadowsocks、CF 多机漫游网关、前端控制台体系）；旧 v2.0.0 版本**原样**移入 `docs/archive/` 留档。
- **R4 SDLC 封存**：整个 `docs/sdlc/`（含 `_template/`）原样移动到 `docs/archive/sdlc/`，不再维护、不再回填新任务。
- **R5 归档惯例**：建立 `docs/archive/`，并在其中提供简短说明（封存规则、只读约定）。

## Acceptance Criteria

- [ ] AC1 `main.go` 的 `Version` 与 README 徽章、CHANGELOG 一致（`v2.6.0-beta.1`）。
- [ ] AC2 README 参数表覆盖 `internal/config/config.go` 全部对外 flag，默认值/环境变量无错漏。
- [ ] AC3 README 分层树与 `internal/` 实际目录一致，无缺失、无幽灵条目。
- [ ] AC4 新版 `docs/ARCHITECTURE_AND_LOGIC.md` 覆盖 R3 列出的子系统，无 v2.0.0 残留表述。
- [ ] AC5 旧架构文档完整留存于 `docs/archive/`（内容未被删改）。
- [ ] AC6 `docs/sdlc/` 不再存在于原路径，完整封存于 `docs/archive/sdlc/`。
- [ ] AC7 全仓不存在指向 `docs/sdlc/` 旧路径的有效引用。
- [ ] AC8 质量门禁全绿：`go build .`、`go vet ./...`、`go test ./...`、`cd web && npm run build`（因改动 `main.go`）。

## Constraints

- 不改动公共 API 契约、数据库 Schema、持久化格式。
- 封存=原样移动，不编辑旧文档内容，保留历史原貌。
- 不新增脚本/工具，KISS；归档用 `git mv` 保留历史。

## Out of Scope

- 不回填 8 个未归档的 v2.6 前端任务到 `docs/sdlc/`（该机制已停用）。
- 不重写 CHANGELOG 历史条目（仅确保版本号口径一致）。

## Notes

- 若发现 `docs/archive/` 与其他工具链/构建存在路径耦合，需在 design 中说明处理方式。
