# M2: 统一异步 NodeCommand 通道与流量表清理收口

> **Parent Task**: `11-19-audit-today-stability-cleanup`
> **Ordering Dependency**: 依赖 M1（`459c8ab`，已归档）完成。

## Goal
1. 将远端按需日志拉取与远端 GeoData 更新合并重构为统一的异步 `NodeCommand` 指令通道，彻底根治 `60s` 心跳下的日志超时死循环与并发覆写（**P0-2**），以及 GeoData 等待 `75s` 超过 HTTP `WriteTimeout=60s` 并阻塞 Agent 心跳循环的缺陷（**P1-1**）。
2. 将 `PruneBatchesBefore` 接入不依赖 `xrayManager` 的后台定时维护任务，并在删除节点事务内级联清理流量明细表，杜绝 SQLite 无限膨胀与孤儿记录（**P1-7**）。
3. 为节点写接口补齐 `RefreshSyncStatus`，消除前端更新/摘流/重置流量后同步状态徽章短暂空白闪烁（**P2-1**）。

## Requirements

### R1. 统一异步节点指令通道（`NodeCommand` — 解决 P0-2 & P1-1）
- **证据与锚点**：
  - `internal/service/node_service.go:22-27, 96-107, 984-1263`：`defaultRemoteLogWaitTimout = 25s`（拼写错误且 `< 60s` 心跳），超时 `defer` 删除 `pendingLogReqs[nodeID]` 且直接赋值覆写并发请求；`defaultRemoteGeoWaitTimeout = 75s > server.go:45 WriteTimeout = 60s`。
  - `internal/agent/runner.go:203-208`：`SyncOnce` 串行阻塞执行 `handleLogRequest` / `handleGeoUpdateRequest`，GeoData 多镜像下载可阻塞超过 `180s` 离线窗口。
  - `web/src/views/LogsView.vue:2004-2035`、`web/src/views/RoutingView.vue:1222-1276`：前端采用长阻塞请求（35s / 90s）而无统一命令状态轮询。
- **要求**：
  - **Master 指令注册表（内存 TTL 模型）**：
    - 移除分裂的 `pendingLogReqs`/`logWaiters`/`logReqLines`/`pendingGeoReqs`/`activeGeoReqs`/`geoWaiters` 与拼写错误的 `defaultRemoteLogWaitTimout`。
    - 在 `NodeService` 内实现统一指令状态机：`queued` -> `dispatched` -> `succeeded` | `failed` | `expired`，指令默认 TTL 为 `180s`（3 倍默认心跳周期），已完成结果保留 `180s` 供前端查询，并按需惰性清理过期记录。
    - **并发去重与永不误删**：同一节点同类型同目标（`log_tail` 按 `(nodeID, service)`、`geodata_update` 按 `nodeID`）已有活跃 `queued`/`dispatched` 指令时直接复用其 `commandId`（若 `log_tail` 仍处于 `queued` 且新请求 `lines` 更大则原地提升 `lines`）；HTTP 请求结束/断开/超时**绝不删除**尚未被 Agent 消费的 `queued` 指令。
    - `DeleteNode` 时同步清理该节点内存中的命令记录与日志缓存。
  - **Master HTTP 管理接口（立即返回，脱离 60s `WriteTimeout`）**：
    - `GET /api/logs?nodeId=<id>&service=<svc>&lines=<n>&refresh=<bool>`：若 `!refresh` 且命中有效 `30s` 日志缓存（`cached.RequestedLines >= lines`），直接返回 `200 OK`（`cached: true, pending: false`）；否则入队或复用 `log_tail` 指令并立即返回 `200 OK`（`pending: true, commandId, status`）。
    - `POST /api/geodata/update?nodeId=<id>`：入队或复用 `geodata_update` 指令并立即返回 **`202 Accepted`**（`nodeId, pending: true, commandId, status`），与本机 Master 异步语义一致。
    - 新增查询端点 `GET /api/admin/nodes/:id/commands/:commandId`（JWT 鉴权）：返回指令当前状态、错误信息与完成后的日志或 GeoData 负载（含 `nodeId` 归属校验，跨节点查不到返回 `404`）。
  - **Agent 异步执行与即时结果上报**：
    - `AgentSyncResponse` 下发 `Commands []AgentCommand`；`AgentSyncRequest` 支持 `CommandResults []AgentCommandResult` 作为降级补传通道（移除旧的分裂 `LogRequest`/`GeoUpdateRequest`/`LogResult`/`GeoUpdateResult` 字段，今日新开发尚未对外发布，保持单源 KISS）。
    - 新增 Agent 端点 `POST /api/agent/commands/:id/result`（`AgentAuth` Bearer 鉴权，校验 `agentNodeID` 归属）：Agent 命令执行完毕后立即主动回传结果，无需再等待下一个 `60s` 心跳周期；若上报失败则暂存内存待下次 `SyncOnce` 随 `CommandResults` 补传。
    - `internal/agent/runner.go`：收到 `resp.Commands` 后，在受控后台 goroutine（`WaitGroup` 追踪、响应 `ctx.Done()` 优雅退出、同类型防重入互斥保护）异步执行 `log_tail` 与 `geodata_update`，`SyncOnce` 立即返回不阻塞主心跳循环。
  - **前端统一轮询体验**：
    - 在 `web/src/api/nodes.ts` 封装 `getNodeCommandStatus(nodeId, commandId)` 及相关类型。
    - `web/src/views/LogsView.vue`：当远端 `/logs` 返回 `pending: true` + `commandId` 时，自动启动 `2s` 定时轮询 `GET /admin/nodes/:id/commands/:commandId`，展示清晰状态（排队等待心跳 / 节点执行中），收到 `succeeded` 后直接渲染日志，`failed`/`expired` 给出错误提示与重试按钮；切换 `activeNodeId` 或 `onUnmounted` 时清理定时器。
    - `web/src/views/RoutingView.vue`：远端触发 `POST /geodata/update?nodeId=...` 改为普通超时（不再 90s 长阻塞），拿到 `commandId` 后启动 `2s` 定时轮询直至终态并展示更新体积/耗时或错误；切换 `activeNodeId` 或 `onUnmounted` 时清理定时器。
    - `web/src/mock/index.ts`：同步补充 `/admin/nodes/:id/commands/:commandId` 最小 mock 处理，保证 Demo 模式远端日志与 GeoData 触发不报错。

### R2. 流量批次表定时清理与级联删除（P1-7）
- **证据与锚点**：`internal/adapter/repository/node_repo.go:147-156, 416-418`、`internal/delivery/cron/traffic_sync.go:27-149`、`main.go:202`。
- **要求**：
  - `GORMNodeRepository.Delete(ctx, id)` 在同一 SQLite 事务内先删除 `node_traffic_batches` 与 `node_traffic_dailies` 中 `node_id = id` 的记录，再删除 `nodes` 主记录（主记录 `RowsAffected == 0` 返回 `domain.ErrNotFound`）。
  - `TrafficSyncJob` 新增可选注入方法 `WithNodeBatchRepo(repo domain.NodeTrafficBatchRepository) *TrafficSyncJob`（在 `main.go` 链式注入 `nodeRepo`），在**不依赖 `xrayManager != nil`** 的后台维护协程内每日执行一次 `PruneBatchesBefore(ctx, now.AddDate(0, 0, -7))`（启动时执行一次 + `alertTicker` 周期按自然日去重）。

### R3. 节点写接口补齐 `RefreshSyncStatus`（P2-1）
- **证据与锚点**：`internal/service/node_service.go:286-512`（`CreateNode`、`UpdateNode`、`SetNodeDesiredState`、`ResetNodeCycleTraffic`）。
- **要求**：
  - `UpdateNode`（在 `prevHost != node.Host` 调用 `InvalidateNode` 之后）、`SetNodeDesiredState`、`ResetNodeCycleTraffic` 与 `CreateNode` 返回前均调用 `RefreshComputed(now)` 与 `RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))`，确保前端立即可得完整 `configSyncStatus` 与 `expectedConfigHash`。

## Out of Scope
- 前端全局切换器作用域重构、`useNodes`/`useActiveNode` 合并、节点运维抽屉、GeoData 从 `RoutingView` 迁出、Portal HTML 单源化、`NodeRepository` 4 接口精简、P2-2/P2-3/P2-4 — 归属 **M3**。

## Acceptance Criteria
1. **AC-1 (P0-2 异步日志与防并发覆写)**：触发远端日志请求在 `60s` 心跳场景下立即返回 `pending: true` + `commandId`；并发重复触发复用同一活跃指令不覆写；等待期间客户端取消不会删除队列指令；Agent 心跳领走后通过 `POST /api/agent/commands/:id/result` 回填结果，查询命令接口返回 `succeeded` 与日志内容且后续 30s 内再次查 `/logs` 命中缓存；超时 `180s` 未完成自动转为 `expired`。
2. **AC-2 (P1-1 异步 GeoData 与非阻塞心跳)**：`POST /api/geodata/update?nodeId=<id>` 立即返回 `202 Accepted` + `commandId`（响应耗时 `< 1s`，彻底脱离 `WriteTimeout=60s`）；Agent 在独立后台 goroutine 执行 GeoData 下载（含防重入与 `ctx` 取消退出），期间 `SyncOnce` 心跳正常推进不阻塞；完成后主动上报结果，前端轮询获得 `succeeded`（含 `geoipSize`/`geositeSize`/`durationMs`）或 `failed`；若即时上报失败，下次心跳通过 `CommandResults` 补传成功。
3. **AC-3 (跨节点归属安全)**：查询 `GET /api/admin/nodes/:id/commands/:commandId` 或上报 `POST /api/agent/commands/:id/result` 若 `nodeId` 与指令所属节点不匹配，拒绝并返回 `404` / `403`。
4. **AC-4 (P1-7 级联删除与每日清理)**：删除节点后数据库 `node_traffic_batches` 与 `node_traffic_dailies` 中该节点记录为 `0`；`TrafficSyncJob` 在 `xrayManager == nil` 条件下也能触发每日一次 `PruneBatchesBefore`（清除 7 天前旧批次，保留 7 天内批次）。
5. **AC-5 (P2-1 同步状态完整返回)**：`CreateNode`、`UpdateNode`、`SetNodeDesiredState`、`ResetNodeCycleTraffic` 返回的 `domain.Node` 均含非空 `configSyncStatus`（`synced`/`pending`/`error`）及 `expectedConfigHash`（在 `compiler != nil` 时）。
6. **AC-6 (全量门禁与依赖纯净)**：以下全量门禁全绿且零 goroutine 泄漏 / 竞态告警：
   - `mise x -- go test -count=1 ./...`
   - `CGO_ENABLED=1 mise x -- go test -race -count=1 ./...`
   - `mise x -- go vet ./...`
   - `mise x -- go build -o NUL .` & `mise x -- go build -o NUL ./cmd/agent`
   - `go list -deps ./cmd/agent | Select-String "gin|gorm|sqlite|telegram"` 输出为空
   - `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run format:check && mise x -- npm run build`
