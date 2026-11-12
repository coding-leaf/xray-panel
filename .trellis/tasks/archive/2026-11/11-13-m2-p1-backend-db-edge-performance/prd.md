# M2: P1 批量操作、SQLite 裁剪、订阅与服务性能提速

## Goal

消除批量用户操作的 `O(N)` 重编译与 `xray -test` 进程 Fork 风暴，补齐删除用户时的孤儿数据清理与 SQLite 历史表自动裁剪，将公开订阅查询 SQL 缩减 85% 并按需单次编码，同时解决入站探测串行阻塞/UDP 误判、HTTP 零超时、Telegram 客户端并发竞态及 Cloudflare 边缘网关连接未释放问题。

## Confirmed Facts

- 前置依赖：M1 已完成 `ConfigService` 互斥串行化与 `SubService` 无状态指纹重构（固定于引用 `refs/swarm-opt/m1-xray` 与 `refs/swarm-opt/m1-net`）。
- `BatchRenew`、`BatchResetTraffic`、`BatchSetStatus`（`internal/service/user_service.go:695-769`）在循环内逐个调用 `SaveConfigQuietly`，触发 `N` 次快照落库与 `xray -test` 子进程 Fork；前端 `useUserList.ts:220-233` 串行发送 `DELETE /users/:id`。
- `DeleteUser`（`internal/service/user_service.go:491-557`）未清理 `tickets`（`ticket_repo.go:58-60` 已有 `DeleteByUserID`）与 `traffic_logs`，同名邮箱重建会继承旧用户历史流量。
- `config_snapshots`（`config_snapshot_repo.go:20-38`）与 `traffic_logs`（`traffic_log_repo.go:20-87`）无容量或时间裁剪机制（对比 `audit_log_repo.go:27-42` 已有原子计数器裁剪）。
- `SubService`（`internal/service/sub_service.go:105-166, 216-257`）单次请求串行调用 11~12 次 `settingRepo.Get`，且 `ExportUserSubscription` 无条件预先编码 `Raw` 与 `Base64` 再丢弃。
- `ConfigService.ListInbounds`（`internal/service/config_service.go:350-381`）对所有入站（含停用与纯 UDP 的 `hysteria2`）串行执行 1s TCP 拨号，阻塞接口并误判纯 UDP 节点离线。
- `http.Server`（`internal/delivery/http/server.go:40-45`）未配置读写与空闲超时；Telegram `BotAdapter`（`internal/adapter/telegram/bot.go:137-141, 182-204`, `notifier.go:35-95`）共享 `bot.Client` 存在轮询与发消息并发覆写竞态、无 HTTP 超时、无消息处理并发上限。
- Cloudflare 边缘脚本（`deploy/cloudflare-pages/_worker.js:298-340`、`deploy/cloudflare-worker-sub-proxy.js:281-323`）在多源站漫游遇到非 `HIT` 响应时未调用 `response.body?.cancel()`。

## Requirements

- **R1 (批量用户操作合并编译与 `batch-delete`)**：重构 `internal/service/user_service.go:695-769` 的 `BatchRenew`、`BatchResetTraffic`、`BatchSetStatus`，提取仅执行单用户 DB + gRPC 同步的内部方法，循环结束后统一执行 **1 次** `SaveConfigQuietly`；新增 `BatchDelete(ctx, ids)` 与路由 `POST /api/users/batch-delete`（复用 `BatchIDsRequest`），并在 `web/src/views/users/composables/useUserList.ts` 接入该批量删除接口。
- **R2 (`DeleteUser` 孤儿数据级联清理)**：在 `DeleteUser`（及 `BatchDelete`）确认删除用户后，清理该用户的 `tickets`（通过可选注入 `WithTicketRepo` 调用 `DeleteByUserID`）与 `traffic_logs`（按 `user_id` 或 `user_email` 删除历史记录），并在 `main.go` 装配。
- **R3 (`config_snapshots` 与 `traffic_logs` 自动裁剪)**：仿照 `audit_log_repo.go:27-42` 在 `ConfigSnapshotRepository` 引入原子写计数器自动裁剪（保留最新 100 条快照）；在 `TrafficLogRepository` 新增 `PruneBefore(ctx, cutoffDate)` 并在 `TrafficSyncJob`（`internal/delivery/cron/traffic_sync.go`）中低频周期触发（保留最近 180 天）。
- **R4 (`/sub` 单次批量读设置与按需编码)**：在 `internal/service/sub_service.go` 改用单次 `settingRepo.GetAll(ctx)` 获取配置映射（若测试桩返回空或未实现则兼容回退）；`ExportUserSubscription` 按目标 `format` 按需单次编码，保留 `GetSubscriptionByToken` 返回 `NodesRaw` + `Base64Data` 的既有契约。
- **R5 (`ListInbounds` 并发探测与纯 UDP 协议修正)**：优化 `internal/service/config_service.go` 的 `ListInbounds`，跳过 `Enabled == false` 入站；对启用入站采用 `sync.WaitGroup` 并发探测、超时收紧为 `200ms`、支持 `"::" -> "::1"` 地址映射；对纯 UDP 协议（`hysteria2` 及 `network == "udp"` 的 `dokodemo-door`）跳过无效 TCP 拨号（依据核心运行状态判定而非误标离线）。
- **R6 (`http.Server` 安全超时基线)**：在 `internal/delivery/http/server.go:40-45` 的 `NewServer` 设置默认 `ReadHeaderTimeout: 10s`、`ReadTimeout: 30s`、`WriteTimeout: 60s`、`IdleTimeout: 120s`、`MaxHeaderBytes: 1 << 20`，保留 `ServerOption` 覆盖能力。
- **R7 (Telegram 客户端竞态消除、超时与信号量)**：在 `internal/adapter/telegram/` 为 `BotAPI` 配置 `http.Client{Timeout: 45 * time.Second}`；`Start()` 对 `*bot` 浅拷贝生成轮询专用副本挂载 `contextAwareHTTPClient`，消除与 `SendMessage` 的 `bot.Client` 竞态；为消息/回调处理 goroutine 增加容量 8 的信号量上限。
- **R8 (Cloudflare 边缘网关非命中流释放)**：在 `deploy/cloudflare-pages/_worker.js` 与 `deploy/cloudflare-worker-sub-proxy.js` 的多源站漫游循环中，当 `evaluation.action !== "HIT"` 时立即执行 `await response.body?.cancel().catch(() => {})`，并在 `deploy/gateway.test.js` 增加断言验证。

## Acceptance Criteria

- [ ] [R1] 批量续期/重置流量/改状态/批量删除 N 名用户时，`SaveConfigQuietly` 仅在循环末尾调用 1 次；`POST /api/users/batch-delete` 前后端联调类型检查通过。
- [ ] [R2] 删除用户后，其关联 `tickets` 与同邮箱/ID 的 `traffic_logs` 被同步清理，重建同邮箱新用户时历史流量为空。
- [ ] [R3] 连续写入超过阈值后 `config_snapshots` 自动裁剪至 ≤ 100 条；`TrafficLogRepository.PruneBefore` 准确清理早于截止日期的记录且不影响近期数据。
- [ ] [R4] 单次订阅导出仅触发 1 次 `settingRepo.GetAll`，且请求 `clash`/`sing-box` 时不再执行多余的 Base64 节点拼接。
- [ ] [R5, R6, R7] `ListInbounds` 对 `hysteria2` 不再因 TCP 拨号失败误标离线，并发探测耗时受控于单跳 200ms；`http.Server` 具备默认超时；Telegram 轮询与并发 `SendMessage` 在 `-race` 下零告警。
- [ ] [R8] `mise x -- node deploy/gateway.test.js` 全绿，验证非命中上游响应流已调用 `cancel()`。
- [ ] 全局门禁 `CGO_ENABLED=1 mise x -- go test -race ./...`、`mise x -- go vet ./...`、`cd web && mise x -- npm run typecheck` 全绿。

## Out of Scope

- 不在 M2 修改 `TrafficLog` 批量 `ON CONFLICT` UPSERT 或 SQLite DSN Pragma（归属 M3）。
- 不把 `POST /api/portal/claim` 改为多源站并发请求（保持顺序漫游防止多源站 CAS 竞态核销）。
