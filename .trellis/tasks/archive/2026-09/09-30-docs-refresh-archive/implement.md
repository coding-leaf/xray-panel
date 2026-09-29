# Implement: 文档刷新与归档

> 规则：按序执行；每个 Milestone 末尾跑一次验证命令；未通过不得进入下一段。

## M0 准备与基线

- [ ] `git status` 确认工作区基线（仅 `.trellis` 有脏文件属正常）。
- [ ] 建目录：`docs/archive/`。
- [ ] 记录基线哈希：`git rev-parse HEAD`。

## M1 版本口径统一（R1 / AC1）

- [ ] 编辑 `main.go:32`：`Version = "v2.5.0"` → `"v2.6.0-beta.1"`。
- [ ] 校验三处一致：`main.go`、`README.md:4` 徽章、`README.md:72` CHANGELOG 首条。
- [ ] 门禁（因改 Go 源码）：
  ```bash
  mise x -- go build .
  mise x -- go vet ./...
  mise x -- go test ./...
  ```

## M2 README 定点校正（R2 / AC2 / AC3）

- [ ] L5 Go 徽章 `Go 1.22+` → `Go 1.26+`。
- [ ] 对照 `internal/config/config.go:33-42` 逐条补齐参数表：
  - `-log-level` / `LOG_LEVEL` / `info`
  - `-log-json` / （无环境变量）/ `false`
  - `-public-url` / `PUBLIC_URL` / `http://127.0.0.1:9000`
  - `-v` / `-version` / — / 打印版本
- [ ] 核对已有 6 行的默认值/环境变量与源码一致（`-port`、`-xray-config`、`-xray-grpc`、`-xray-bin`、`-service`、`-db`、`-jwt-secret`）。
- [ ] 重写 L158-173 分层树，与 `internal/` 实目录对齐：
  ```bash
  Get-ChildItem -Recurse -Directory -Path internal -Name | Sort-Object
  ```
  重点补：`config/`、`pkg/{cache,jsonc,jwt,logger,totp}/`、`adapter/reality/`、`adapter/xray/proto/`、`delivery/cron/`；确认无幽灵条目。
- [ ] 验证：参数表 flag 名集合 == `config.go` 中 `flag.*Var` 集合（人工逐条比对）。

## M3 架构文档重写 + 旧版封存（R3 / AC4 / AC5）

- [ ] `git mv docs/ARCHITECTURE_AND_LOGIC.md docs/archive/ARCHITECTURE_AND_LOGIC.v2.0.0.md`（先封存旧版）。
- [ ] 在 `docs/ARCHITECTURE_AND_LOGIC.md` 新建 v2.6.0-beta.1 版，覆盖：
  - [ ] 领域模型与单端口多出口 RouteID（保留 v2.0 骨架）
  - [ ] SubRoute 用户隔离（`AllowedUsers` + Layer 3 `User` 注入物理阻断）
  - [ ] 路由分层编排（Layer 1-4）
  - [ ] 运行时解耦（gRPC 热重载 + 冷启动落盘）
  - [ ] Ticket 阅后即焚与 CF 反探测网关
  - [ ] Reality 域名合规巡检
  - [ ] 多协议注册表（vless/vmess/trojan/shadowsocks/hysteria2/socks）
  - [ ] 服务编排（`main.go:171-180` 全量服务 + 两阶段释放）
  - [ ] 持久化（WAL、BatchSyncTraffic 单事务）
  - [ ] CF 多机漫游网关与提取门户
  - [ ] 前端控制台体系与分包
  - [ ] 标题/正文版本号统一为 `v2.6.0-beta.1`
- [ ] 验证旧版完整：`git show HEAD:docs/ARCHITECTURE_AND_LOGIC.md` 与新封存文件 diff 应为空。
- [ ] 验证无残留：全文搜索 `v2.0.0` 只应出现在封存文件名中。

## M4 SDLC 封存（R4 / AC6 / AC7）

- [ ] `git mv docs/sdlc docs/archive/sdlc`。
- [ ] `Test-Path docs/sdlc` 应为 `False`；`Get-ChildItem docs/archive/sdlc` 应含原 8 个子目录 + ARCHIVE.md。
- [ ] 全仓引用检查（应 0 命中有效引用）：
  ```bash
  Select-String -Path (Get-ChildItem -Recurse -File -Include *.md,*.go,*.yml,*.yaml,*.json | Where-Object { $_.FullName -notmatch '\.trellis|node_modules|archive' }).FullName -Pattern 'docs/sdlc'
  ```

## M5 归档说明（R5）

- [ ] 新建 `docs/archive/README.md`：说明封存规则、只读约定、命名规范、旧 SDLC 停用声明。
- [ ] README 主文档链接（`README.md:156`）仍指向 `docs/ARCHITECTURE_AND_LOGIC.md`，确认有效（AC7）。

## M6 全量门禁（AC8）

- [ ] `mise x -- go build .`
- [ ] `mise x -- go vet ./...`（0 warning）
- [ ] `mise x -- go test ./...`（全绿）
- [ ] `cd web && mise x -- npm run build`
- [ ] `git status` 复核：应为 rename（旧文档/SDLC）+ modify（README、main.go）+ add（新架构文档、archive README）。（因改 `main.go` 需重跑全部门禁）

## Review Gates

- **G1（M1 后）**：版本三处一致。
- **G2（M3 后）**：旧架构文档完整性 diff 为空。
- **G3（M6 前）**：AC1-AC7 逐条勾选，再跑 AC8 门禁。

## Rollback Points

- M1 前：`git restore .`
- M3/M4 移动后如需回退：`git mv docs/archive/... <原路径>`
