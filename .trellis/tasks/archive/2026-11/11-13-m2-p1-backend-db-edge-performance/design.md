# M2 技术设计：P1 批量操作、SQLite 裁剪、订阅与服务性能提速

## 1. 架构边界与跨层契约

- **新增接口契约（向后兼容增量）**：
  - HTTP：`POST /api/users/batch-delete`，请求体复用 `BatchIDsRequest` (`{"ids": [1, 2, ...]}`)，响应 `200 {"message": "成功删除 N 位用户"}`，记录审计动作 `domain.ActionUserDelete`；前端 `web/src/api/` 新增 `batchDeleteUsers(ids: number[])` 并由 `useUserList.ts` 调用。
  - Repository：`domain.TrafficLogRepository` 新增 `DeleteByUser(ctx context.Context, userID uint, email string) error` 与 `PruneBefore(ctx context.Context, beforeDate string) error`（同步更新测试中的 mock 实现）。
  - Service：`UserService` 新增可选装配方法 `WithTicketRepo(repo domain.TicketRepository) *UserService`（保持 `NewUserService` 原 5 参数签名不变，零破坏既有测试），在 `main.go` 链式注入 `ticketRepo`。

---

## 2. 模块设计细节

### 2.1 批量用户操作单次编译与孤儿清理 (`user_service.go`, `handler_user.go`, `router.go`, `main.go`, `useUserList.ts`)
- 抽取不触发 `SaveConfigQuietly` 的单用户内部方法（如 `renewUserNoSave`、`resetTrafficNoSave`、`setUserStatusNoSave`、`deleteUserNoSave`）：
  - 循环内逐个完成 DB 更新与 Xray gRPC 动态增删用户；
  - 循环结束后若存在至少 1 名用户变更成功，统一调用 **1 次** `s.configSvc.SaveConfigQuietly(ctx, reason)`。
- `DeleteUser` 与 `deleteUserNoSave` 在 DB 删除成功后：
  - 若 `s.ticketRepo != nil`，调用 `s.ticketRepo.DeleteByUserID(ctx, user.ID)`（记录日志但不回滚已删用户）；
  - 若 `s.trafficLogRepo != nil`，调用 `s.trafficLogRepo.DeleteByUser(ctx, user.ID, user.Email)` 清理 `user_id = ? OR user_email = ?` 的流量日志。

### 2.2 SQLite 历史表自动裁剪 (`config_snapshot_repo.go`, `traffic_log_repo.go`, `traffic_sync.go`)
- `GormConfigSnapshotRepository`：新增 `writeCount atomic.Uint64`，设定 `snapshotMaxKeep = 100`、`snapshotPruneEvery = 20`；每次写入达到间隔（或第 1 次达到间隔）时，按 `id DESC` 保留最新 100 条并删除其余旧快照（同步提供显式 `Prune(ctx, maxKeep)` 方法便于单测直接验证）。
- `GormTrafficLogRepository`：实现 `PruneBefore(ctx, beforeDate)` 执行 `WHERE date < ?` 删除；在 `TrafficSyncJob` 中记录上次裁剪日期（如每日首次同步或每 N 轮同步），自动清理早于 `time.Now().AddDate(0, 0, -180).Format("2006-01-02")` 的过期日志。

### 2.3 `/sub` 单次查询设置与按需编码 (`sub_service.go`)
- 提取辅助方法 `loadSettingsMap(ctx)`：优先一次调用 `s.settingRepo.GetAll(ctx)` 返回 `map[string]string`（若返回 `nil` map 则安全兜底为空 map，注意检查 `sub_service_test.go` 中 mock 的 `GetAll` 实现，确保 mock 数据与 `Get` 一致！）。
- 重构 `ExportUserSubscription`：先调用内部节点解析函数拿到 `[]*protocol.NodeConfig` 与元数据，再仅针对请求的 `format`（`clash` / `sing-box` / `raw` / 默认 `v2ray` Base64）执行 **1 次** 序列化编码；`GetSubscriptionByToken` 仍填充 `NodesRaw` 与 `Base64Data` 保证兼容。

### 2.4 `ListInbounds` 并发探测与 UDP 协议修正 (`config_service.go`)
- 先查询一次核心是否运行（若需要，或针对每个 `ib`）：
  - 若 `!ib.Enabled`：直接跳过拨号，`PortListening = false`；
  - 若协议为纯 UDP（`strings.EqualFold(ib.Protocol, "hysteria2")` 或 `dokodemo-door` 且流配置仅启用 UDP）：不做 TCP Dial，通过核心状态 `s.supervisor.GetStatus(ctx)` 的 `Running` 布尔值（全列表仅需获取 1 次）标记运行态，避免将正常工作的 Hysteria2 节点误标为端口未监听；
  - 其余启用中的 TCP 入站：使用 `sync.WaitGroup` 并发执行 `net.DialTimeout("tcp", addr, 200*time.Millisecond)`（每个 goroutine 写入各自索引 `result[i]` 无共享写竞争），并将 `""` / `"0.0.0.0"` 映射为 `"127.0.0.1"`、`"::"` 映射为 `"::1"`。

### 2.5 `http.Server` 超时、Telegram 竞态消除与 CF Worker 流释放
- `server.go`：`NewServer` 默认初始化 `ReadHeaderTimeout: 10 * time.Second`, `ReadTimeout: 30 * time.Second`, `WriteTimeout: 60 * time.Second`, `IdleTimeout: 120 * time.Second`, `MaxHeaderBytes: 1 << 20`，随后执行 `opts...` 允许覆盖。
- `telegram/bot.go` & `notifier.go`：
  - 创建 `tgbotapi.NewBotAPIWithClient(token, tgbotapi.APIEndpoint, &http.Client{Timeout: 45 * time.Second})`（或在初始化后赋值独立超时 `Client`）；
  - `Start(ctx)` 在持读锁拷贝 `bot` 指针后，浅拷贝结构体 `pollBot := *bot` 并将 `pollBot.Client = &contextAwareHTTPClient{ctx: ctx, base: bot.Client}` 仅挂在局部 `pollBot` 上调用 `pollBot.GetUpdatesChan(u)`，彻底消除与并发 `SendMessage` 读取 `bot.Client` 的数据竞态；
  - `BotHandler` 新增 `sem chan struct{}`（容量 8），分发消息/回调前获取令牌（支持 `ctx.Done()` 退出），处理完毕释放。
- `deploy/cloudflare-pages/_worker.js` & `deploy/cloudflare-worker-sub-proxy.js`：在 `for (const origin of origins)` 循环内，当 `evaluation.action !== "HIT"` 时立即调用 `if (response.body) { await response.body.cancel().catch(() => {}); }`。

---

## 3. 风险与回滚预案

- **Mock 接口变更同步**：`domain.TrafficLogRepository` 新增方法后，`m2-core` 必须全局检索所有实现该接口的测试 Mock（如 `user_service_test.go` 等）一并补齐桩方法，确保 `go test ./...` 100% 编译通过。
- **分支基线**：M2 两个 `share` worker 启动时先合并 `refs/swarm-opt/m1-xray` 与 `refs/swarm-opt/m1-net`，确保基于 M1 最新代码增量提交。
