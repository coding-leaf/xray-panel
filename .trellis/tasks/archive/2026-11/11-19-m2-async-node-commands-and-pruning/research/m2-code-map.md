# Research: M2 Code Map — 异步节点指令通道、流量批次清理与同步状态收口

- **Query**: M2 涉及的全部现有源码契约与调用链只读调研（P0-2 & P1-1 远端日志/GeoData 同步等待链路；P1-7 流量批次表清理与级联删除；P2-1 写接口 `RefreshSyncStatus` 补齐）
- **Scope**: `internal/domain/`, `internal/service/`, `internal/adapter/repository/`, `internal/delivery/http/`, `internal/delivery/cron/`, `internal/agent/`, `cmd/agent/`, `main.go`, `web/src/`
- **Date**: 2026-11-19

---

## 1. 远端日志与远端 GeoData 现有同步等待链路（P0-2 & P1-1）

### 1.1 `internal/service/node_service.go` 契约、状态字段与锁用法

- **常量与拼写问题** (`internal/service/node_service.go:22-27`):
  - `defaultNodeSyncIntervalSec = 60`
  - `defaultRemoteLogCacheTTL = 30 * time.Second`
  - `defaultRemoteLogWaitTimout = 25 * time.Second` (**拼写错误**: `Timout` 缺 `e`)
  - `defaultRemoteGeoWaitTimeout = 75 * time.Second` (超过 HTTP Server `WriteTimeout = 60s`)
- **响应 DTO**:
  - `RemoteNodeLogsResponse` (`internal/service/node_service.go:61-69`): `nodeId`, `service`, `lines`, `cached`, `pending`, `collectedAt`, `error`
  - `RemoteGeoUpdateResponse` (`internal/service/node_service.go:72-80`): `nodeId`, `pending`, `geoipSize`, `geositeSize`, `durationMs`, `updatedAt`, `error`
- **`NodeService` 内存映射与锁** (`internal/service/node_service.go:96-107`):
  - 单一锁 `mu sync.RWMutex` 同时保护快照缓存、epoch、日志指令队列/缓存/waiter、GeoData 指令队列/去重/waiter：
    - 日志相关：`pendingLogReqs map[uint]*domain.AgentLogRequest`、`logCache map[string]*cachedRemoteLog`（key 为 `"<nodeID>:<service>"`）、`logWaiters map[string][]chan *cachedRemoteLog`（key 为 `reqID`）、`logReqLines map[string]int`
    - GeoData 相关：`pendingGeoReqs map[uint]*domain.AgentGeoUpdateRequest`、`activeGeoReqs map[uint]string`（按 `nodeID` 合并并发请求至同一 `reqID`）、`geoWaiters map[string][]chan *domain.AgentGeoUpdateResult`
- **关键方法签名与行为**:
  1. `RequestNodeLogs(ctx context.Context, nodeID uint, svcName string, lines int, forceRefresh bool) (*RemoteNodeLogsResponse, error)` (`internal/service/node_service.go:984-1085`)
     - 校验 `nodeID > 0`、`svc` 白名单 (`"xray"` | `"xray-agent"`)、`lines` 钳制 `[1, 1000]`（默认 `100`）、节点存在性。
     - `!forceRefresh` 时检查 `logCache[nodeID:svc]`，若 `now.Sub(cached.CachedAt) <= 30s && cached.RequestedLines >= lines` 直接命中缓存返回 (`Cached: true`)。
     - 未命中缓存则加锁生成 `reqID` (`log-<nano>-<hex4>`)，**覆盖写入** `s.pendingLogReqs[nodeID]`、记录 `s.logReqLines[reqID]` 与 `s.logWaiters[reqID]`，释放锁后阻塞等待 `waitCh` / `timer.C` (`logWaitTimeout=25s`) / `ctx.Done()`。
     - **关键缺陷**：`defer` (`1047-1057`) 在超时或取消返回时若 `s.pendingLogReqs[nodeID].RequestID == reqID`，会删除 `pendingLogReqs[nodeID]`。因此若心跳周期为 60s 而 HTTP 等待仅 25s，指令在等待超时时即被清除，后续心跳可能永远收不到；此外 `HandleAgentSync` 期间若 `popPendingLogRequest` 已被弹出但 Agent 在执行/回传前网络中断或超时，无持久化状态或重查接口。
  2. `popPendingLogRequest(nodeID uint, ack *domain.AgentLogResult) *domain.AgentLogRequest` (`internal/service/node_service.go:1148-1161`)
     - 加锁读取 `pendingLogReqs[nodeID]`；无论 `ack` 是否匹配当前 `pending.RequestID`，均执行 `delete(s.pendingLogReqs, nodeID)`；若 `ack != nil && ack.RequestID == pending.RequestID` 返回 `nil`，否则返回 `pending` 下发给 Agent。
  3. `handleIncomingLogResult(nodeID uint, res *domain.AgentLogResult, now time.Time)` (`internal/service/node_service.go:1116-1146`)
     - 校验 `service` 白名单，深拷贝 `Lines`，写入 `logCache[nodeID:svc]` 并通过 `dispatchWaitersLocked` 唤醒所有等待 `res.RequestID` 的 channel。
  4. `RequestNodeGeoUpdate(ctx context.Context, nodeID uint) (*RemoteGeoUpdateResponse, error)` (`internal/service/node_service.go:1172-1235`)
     - 校验 `nodeID > 0` 与节点存在性。
     - 通过 `activeGeoReqs[nodeID]` 合并同一节点并发请求：若为空则新建 `reqID` (`geo-<nano>-<hex4>`) 并写入 `pendingGeoReqs[nodeID]`；否则复用已有 `reqID` 并追加 `waitCh` 至 `geoWaiters[reqID]`。
     - 阻塞等待 `waitCh` / `timer.C` (`geoWaitTimeout=75s`) / `ctx.Done()`。
     - `defer` (`1198-1210`) 移除 `waitCh`；若 `geoWaiters[reqID]` 清空，清理 `activeGeoReqs[nodeID]` 与 `pendingGeoReqs[nodeID]`。
  5. `popPendingGeoRequest(nodeID uint, ack *domain.AgentGeoUpdateResult) *domain.AgentGeoUpdateRequest` (`internal/service/node_service.go:1250-1263`)
     - 加锁读取并删除 `pendingGeoReqs[nodeID]`（若 `ack` 匹配则返回 `nil`，否则下发 `pending`）。
  6. `handleIncomingGeoResult(nodeID uint, res *domain.AgentGeoUpdateResult)` (`internal/service/node_service.go:1237-1248`)
     - 若 `activeGeoReqs[nodeID] == res.RequestID` 删除之，并 `dispatchWaitersLocked` 唤醒 `geoWaiters[res.RequestID]`。
  7. `HandleAgentSync(ctx context.Context, nodeID uint, req *domain.AgentSyncRequest) (*domain.AgentSyncResponse, error)` (`internal/service/node_service.go:709-836`)
     - 顺序：校验节点存在与 `Enabled` -> 应用 `TrafficBatch`（若有） -> `UpdateHeartbeat` (`740`) -> 若 `req.LogResult != nil` 调 `handleIncomingLogResult` (`749-751`) -> 若 `req.GeoUpdateResult != nil` 调 `handleIncomingGeoResult` (`752-754`) -> 配额状态评估与快照构建 -> 弹出指令 `resp.LogRequest = s.popPendingLogRequest(...)` (`832`) 和 `resp.GeoUpdateRequest = s.popPendingGeoRequest(...)` (`833`)。

### 1.2 HTTP 路由、Handler 与 Server 超时约束

- **HTTP Server 全局超时** (`internal/delivery/http/server.go:40-48`):
  - `ReadHeaderTimeout: 10s`, `ReadTimeout: 30s`, `WriteTimeout: 60s`, `IdleTimeout: 120s`
  - **直接冲突**：`RequestNodeGeoUpdate` 同步阻塞最长 `75s` (`defaultRemoteGeoWaitTimeout`)，超过 `WriteTimeout = 60s`，在真实 TCP 连接上会被 `http.Server` 在 60s 时强行截断写响应（仅 `httptest` 未受 `WriteTimeout` 影响）。
- **管理端路由** (`internal/delivery/http/router.go:160-167`，均在 `authGroup` JWT 保护下):
  - `GET /api/geodata/status` -> `GeoDataHandler.GetStatus`
  - `GET /api/geodata/progress` -> `GeoDataHandler.GetProgress` (目前仅本机 Master 进度，无 `nodeId` 参数)
  - `POST /api/geodata/update` -> `GeoDataHandler.UpdateGeoData` (`internal/delivery/http/handler_geodata.go:40-64`)
    - 当 `?nodeId > 0`：同步调用 `h.nodeSvc.RequestNodeGeoUpdate(c.Request.Context(), nodeID)`，返回 `200 OK` + `service.RemoteGeoUpdateResponse`。
    - 当无 `nodeId` 或 `nodeId=0`：调用 `h.geoSvc.TriggerUpdate()`，立即返回 `202 Accepted` + `{"message": "..."}`（前端随后轮询 `/geodata/progress`）。
  - `GET /api/logs` -> `LogHandler.GetLogs` (`internal/delivery/http/handler_log.go:34-84`)
    - 当 `?nodeId > 0`：解析 `service` (默认 `"xray"`)、`lines` (默认 `"100"`)、`refresh` (`true`/`1`)，同步调用 `h.nodeSvc.RequestNodeLogs(...)`，返回 `200 OK` + `service.RemoteNodeLogsResponse`。
    - 当无 `nodeId` 或 `nodeId=0`：走 `h.logSvc.GetRecentLogs(...)`。
- **Agent 端路由** (`internal/delivery/http/router.go:86-92`, `internal/delivery/http/handler_agent.go:21-53`, `internal/delivery/http/middleware_agent.go:17-51`):
  - 目前仅暴露一条路由：`POST /api/agent/sync`（`AgentAuth` Bearer SHA-256 鉴权，注入 `agentNodeID`）。
  - 请求绑定 `domain.AgentSyncRequest`，响应返回 `domain.AgentSyncResponse`。**尚无**独立的命令结果上报接口（如 `/api/agent/commands/:id/result`）。

### 1.3 Domain / Agent 协议与执行链路

- **Domain 协议结构体** (`internal/domain/node.go:324-354, 365-411`):
  - `AgentLogRequest { RequestID, Service, Lines }`
  - `AgentLogResult { RequestID, Service, Lines, Error, CollectedAtUnix }`
  - `AgentGeoUpdateRequest { RequestID }`
  - `AgentGeoUpdateResult { RequestID, GeoIPSize, GeoSiteSize, DurationMs, Error, UpdatedAtUnix }`
  - `AgentSyncRequest` 含 `LogResult *AgentLogResult`、`GeoUpdateResult *AgentGeoUpdateResult`
  - `AgentSyncResponse` 含 `LogRequest *AgentLogRequest`、`GeoUpdateRequest *AgentGeoUpdateRequest`
- **Agent 侧镜像协议** (`internal/agent/protocol.go:11-97`):
  - 与 `domain` 一一对应的 `SyncRequest`, `SyncResponse`, `LogRequest`, `LogResult`, `GeoUpdateRequest`, `GeoUpdateResult`。
- **Agent Client** (`internal/agent/client.go:16, 48-93`):
  - 默认 `defaultHTTPTimeout = 15 * time.Second`；仅实现了 `Sync(ctx, *SyncRequest) (*SyncResponse, error)` 调用 `POST /api/agent/sync`，无独立上报方法。
- **Agent Runner 处理顺序** (`internal/agent/runner.go:108-210, 277-352`):
  - `Run()` (`108-138`) 启动先执行一次 `SyncOnce(ctx)`，之后单协程 `ticker`（默认 `60s`）循环调用 `SyncOnce(ctx)`。
  - `SyncOnce(ctx)` (`141-210`) 顺序执行：
    1. 读取/采集流量并落盘 spool -> 采集 `hostMetrics` -> POST `/api/agent/sync` (`178`)。
    2. `Ack` 本地 spool (`186`) -> 同步 `SyncIntervalSec` -> `syncQuotaStoreOnline` (`197`)。
    3. `reconcileSyncResponse` (`199`)，若配置或用户变更则调用 `r.sendLightweightAck(ctx, nil, nil)` (`201`)。
    4. 若 `resp.LogRequest != nil`：**同步串行**调用 `r.handleLogRequest(ctx, resp.LogRequest)` (`204`) -> 执行 `JournalctlLogCollector.Collect` -> `r.sendLightweightAck(ctx, result, nil)` (`302`)。
    5. 若 `resp.GeoUpdateRequest != nil`：**同步串行**调用 `r.handleGeoUpdateRequest(ctx, resp.GeoUpdateRequest)` (`207`) -> 执行 `HTTPGeoDataUpdater.Update(ctx)`（下载两个 `.dat` 文件、各最多 60s HTTP 超时、`ReloadAndProbe` + 失败回滚，自身带 `u.mu sync.Mutex` 防重入）-> `r.sendLightweightAck(ctx, nil, result)` (`331`)。
  - **关键阻塞点**：`handleLogRequest` 与 `handleGeoUpdateRequest` 在 `SyncOnce` 同一 goroutine 内同步执行，直接阻塞主心跳 ticker 达数十秒甚至 120s+（GeoIP + GeoSite 各 60s 下载 + ReloadProbe）。
- **仓储层对轻量回执的特殊兼容** (`internal/adapter/repository/node_repo.go:76-103`):
  - `UpdateHeartbeat` 判断 `isLightweightResultAck := (req.LogResult != nil || req.GeoUpdateResult != nil) && req.TrafficBatch == nil && req.HostMetrics == nil`，避免轻量回执把 `agent_version`/`last_config_hash`/`last_sync_error` 等空字段覆盖清零。若未来拆分独立上报接口，可不再依赖此分支（但需保留向后兼容）。

### 1.4 前端调用方式与现有轮询模式

- **`web/src/api/index.ts`**: axios 单例，`baseURL: '/api'`，默认 `timeout: 30000` (30s)，响应拦截器直接解包 `response.data`。
- **`web/src/api/nodes.ts`**:
  - 定义了 `RemoteGeoUpdateResult` 接口 (`96-104`)。
  - 当前未封装远端日志或 GeoData 的 API 函数（均在 View 内直接调用 `api.get`/`api.post`）。
- **`web/src/views/LogsView.vue`**:
  - 接口 `RemoteLogResponse` (`1395-1403`)。
  - `fetchRemoteLogs` (`2004-2035`)：直接 `api.get('/logs?nodeId=...&service=...&lines=...&refresh=true', { timeout: 35000 })`，无后台轮询；若返回 `pending: true`，仅显示黄色提示条与「重新拉取」按钮 (`165-185`)。
  - 本地日志模式已有 `autoRefresh` + `setInterval(pollLogs, 5000)` + `document.visibilitychange` 监听，切换至远端节点时 (`watch(activeNodeId)` `2071-2089`) 强制关闭 `autoRefresh`。
- **`web/src/views/RoutingView.vue`**:
  - **本机 Master 已有异步触发 + 轮询模式** (`1182-1220, 1281-1285, 1766-1770`)：`POST /geodata/update` 后启动 `progressTimer = setInterval(pollGeoProgress, 500)` 轮询 `GET /geodata/progress`，收到 `res.isUpdating === false` 时停止定时器并提示 `done` / `error`，`onUnmounted` 清理定时器。
  - **远端节点当前为长阻塞请求** (`1222-1276`)：confirm 后直接 `await api.post<RemoteGeoUpdateResult>('/geodata/update?nodeId=' + targetNodeId, undefined, { timeout: 90000 })`，阻塞 UI 等待最多 90s（实际上服务端 `WriteTimeout=60s` 会在 60s 处断流报错）。`watch(activeNodeId)` (`1749-1759`) 会重置 `remoteGeoResult` 与 `updatingGeo`。
- **`web/src/mock/index.ts`**: Mock 模式下 `/logs` (`650`) 与 `/geodata/update` (`1027`) / `/geodata/progress` (`1031`) 有拦截逻辑，若修改接口路径或新增命令状态轮询接口需同步适配。

---

## 2. 流量批次表清理与级联删除（P1-7）

### 2.1 模型、索引与仓储实现

- **表模型与索引** (`internal/domain/node.go:229-246`):
  - `NodeTrafficDaily`: `id` PK, `(node_id, date)` 唯一复合索引 `idx_node_daily`。
  - `NodeTrafficBatch`: `id` PK, `(node_id, batch_id)` 唯一复合索引 `idx_node_batch`, `created_at` 单列索引 (`gorm:"index"`)。
  - 两者均由 `InitSQLite` (`internal/adapter/repository/sqlite_db.go:46-47`) 自动迁移，SQLite 模式下无数据库外键约束（不会自动 `ON DELETE CASCADE`）。
- **仓储接口与实现**:
  - `domain.NodeTrafficBatchRepository` (`internal/domain/repository.go:126-129`) 已声明 `PruneBatchesBefore(ctx context.Context, before time.Time) error`。
  - `GORMNodeRepository.PruneBatchesBefore` (`internal/adapter/repository/node_repo.go:416-418`):
    ```go
    func (r *GORMNodeRepository) PruneBatchesBefore(ctx context.Context, before time.Time) error {
        return r.db.WithContext(ctx).Where("created_at < ?", before).Delete(&domain.NodeTrafficBatch{}).Error
    }
    ```
    （已实现且有 `created_at` 索引，但**全局无任何生产调用点**，仅接口实现本身。）
  - `GORMNodeRepository.Delete` (`internal/adapter/repository/node_repo.go:147-156`):
    ```go
    func (r *GORMNodeRepository) Delete(ctx context.Context, id uint) error {
        res := r.db.WithContext(ctx).Delete(&domain.Node{}, id)
        ...
    }
    ```
    **当前仅删除 `nodes` 表行，未级联清理 `node_traffic_batches` 与 `node_traffic_dailies` 中对应 `node_id = id` 的孤儿记录**。

### 2.2 `DeleteNode` 与定时任务调度器接入点

- **`NodeService.DeleteNode`** (`internal/service/node_service.go:569-587`):
  - 校验节点存在 -> `CountInbounds` 检查仍有绑定入站返回 `ErrNodeHasInbounds` -> 调用 `s.nodeRepo.Delete(ctx, id)` -> `s.InvalidateNode(id)`。
  - **级联删除建议**：在 `GORMNodeRepository.Delete` 内部使用 `r.db.WithContext(ctx).Transaction(...)` 原子删除 `NodeTrafficBatch` (`Where("node_id = ?", id)`)、`NodeTrafficDaily` (`Where("node_id = ?", id)`) 以及 `Node`（先删子表再删主表，判断主表 `RowsAffected == 0` 返回 `domain.ErrNotFound`），使事务边界与仓库内已有 `UpdateHeartbeat`/`ApplyNodeTrafficBatch`/`ResetCycleTraffic` 一致；另外 `DeleteNode` 还可顺带清理 `pendingLogReqs`/`pendingGeoReqs`（或新命令队列）中该节点的残留内存状态。
- **定时任务调度器 (`TrafficSyncJob`)** (`internal/delivery/cron/traffic_sync.go:27-146, 335-350`):
  - 在 `main.go:202-215` 注册并由 `errgroup` 管理生命周期：`syncJob := deliveryCron.NewTrafficSyncJob(..., 5*time.Second).WithNodeQuotaCron(nodeSvc)`。
  - `TrafficSyncJob.Start` (`74-146`) 包含：
    1. `ticker` (`5s`) -> `syncOnce` -> 末尾调用 `j.pruneOldTrafficLogsIfNeeded(writeCtx, today)` (`317`)，使用 `pruneMu` + `lastPruneDate == today` 保证**每日只执行一次** (`trafficLogRetentionDays = 180`)。
    2. 后台维护协程 (`86-128`)：`ticketCleanTicker` (`1m`) 和 `alertTicker` (`5m`，内含 `userSvc.CheckAndResetMonthlyTraffic` 与 `nodeSvc.CheckAndResetMonthlyTraffic`)。
  - **最佳接入点**：
    - 为 `TrafficSyncJob` 增加可选字段 `batchRepo domain.NodeTrafficBatchRepository`（及 `WithNodeBatchRepo(repo domain.NodeTrafficBatchRepository) *TrafficSyncJob` builder，遵循现有 `WithNodeQuotaCron` 风格以保持构造签名向后兼容），在 `main.go:202-203` 链式注入 `.WithNodeBatchRepo(nodeRepo)`。
    - 注意：`syncOnce` 首行在 `j.xrayManager == nil` 时直接 `return` (`149-151`)！若把批次清理放在后台维护协程（例如 `alertTicker` / 每日去重 helper `pruneExpiredDataIfNeeded` 中，或独立在 `Start` 中调用且不依赖 `xrayManager != nil`），即使仅管远端节点而本地 `xrayManager` 不可用/未配置也能正常清理 `node_traffic_batches`。保留窗口建议如 7 天 (`time.Now().AddDate(0, 0, -7)`)。

---

## 3. 写接口 `RefreshSyncStatus` 补齐（P2-1）

### 3.1 `RefreshSyncStatus` 与读接口现有用法

- **Domain 方法** (`internal/domain/node.go:211-226`):
  - `RefreshSyncStatus(expectedConfigHash string)`：设置 `n.ExpectedConfigHash = expectedConfigHash`；若 `strings.TrimSpace(n.LastSyncError) != ""` 则置 `"error"`；否则若 `n.LastConfigHash != "" && expectedConfigHash != "" && n.LastConfigHash == expectedConfigHash` 则置 `"synced"`；其余置 `"pending"`。
- **Helper 与读接口** (`internal/service/node_service.go:217-256`):
  - `s.resolveExpectedConfigHash(ctx, nodeID)` (`217-233`)：若 `s.compiler == nil` 返回 `""`；否则调用 `s.getOrBuildSnapshot(ctx, nodeID)` 取 `snap.ConfigHash`（失败仅 `slog.Warn` 并返回 `""`，不阻断主流程）。
  - `ListNodes` (`241-244`)：`nodes[i].RefreshOnline(now)` + `nodes[i].RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, nodes[i].ID))`
  - `GetNode` (`253-254`)：`node.RefreshOnline(time.Now())` + `node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))`

### 3.2 各写接口现状与补齐方式

| 方法 (`internal/service/node_service.go`) | 当前返回前调用 | Host/配置是否可能变化 | 补齐方式 |
|---|---|---|---|
| `UpdateNode` (`375-464`) | 仅 `node.RefreshOnline(time.Now())` (`462`)；未调 `RefreshSyncStatus` | **是**：`458-460` 在 `prevHost != node.Host` 时已调用 `s.InvalidateNode(id)`（用于静态中继凭据重新编译） | 在 `InvalidateNode` 之后补充 `node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))` |
| `SetNodeDesiredState` (`467-496`) | 仅 `updated.RefreshComputed(time.Now())` (`494`)；未调 `RefreshSyncStatus` | 否（仅改 `DesiredState`/`AutoStateReason`） | 补充 `updated.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, updated.ID))` |
| `ResetNodeCycleTraffic` (`499-512`) | 仅 `node.RefreshComputed(time.Now())` (`510`)；未调 `RefreshSyncStatus` | 否（清零周期流量及恢复状态） | 补充 `node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))` |
| *(附)* `CreateNode` (`286-373`) | 仅 `node.RefreshComputed(now)` (`367`)；未调 `RefreshSyncStatus` | 新建节点 `LastConfigHash=""`，期望哈希可由 `resolveExpectedConfigHash` 计算（当前无入站也可得出基础结构哈希，状态为 `pending`） | 可按需评估是否一并补齐 `node.RefreshSyncStatus(...)`（注意检查现有单测是否假设空值） |

---

## 4. Files Found

| Path | Role |
|---|---|
| `internal/service/node_service.go` | 核心节点管理、`HandleAgentSync`、`pendingLogReqs`/`pendingGeoReqs` 同步等待、`UpdateNode`/`SetNodeDesiredState`/`ResetNodeCycleTraffic`/`DeleteNode`、`resolveExpectedConfigHash` |
| `internal/domain/node.go` | `Node`、`NodeTrafficBatch`、`NodeTrafficDaily` 模型及索引、`RefreshSyncStatus`、`AgentSyncRequest`/`AgentSyncResponse`、`AgentLog*`/`AgentGeoUpdate*` |
| `internal/domain/repository.go` | `NodeRepository`、`NodeTrafficBatchRepository` (`PruneBatchesBefore`) 等仓储契约 |
| `internal/adapter/repository/node_repo.go` | `GORMNodeRepository`：`UpdateHeartbeat`、`Delete`、`PruneBatchesBefore`、`ApplyNodeTrafficBatch` |
| `internal/adapter/repository/sqlite_db.go` | SQLite 初始化与 `AutoMigrate`（`NodeTrafficBatch`、`NodeTrafficDaily`） |
| `internal/delivery/http/router.go` | `/api/agent/sync`、`/api/logs`、`/api/geodata/*`、`/api/admin/nodes/*` 路由注册 |
| `internal/delivery/http/server.go` | HTTP Server 超时配置（`WriteTimeout: 60s`） |
| `internal/delivery/http/handler_log.go` | `GET /api/logs?nodeId=...` 远程分支调用 `RequestNodeLogs` |
| `internal/delivery/http/handler_geodata.go` | `POST /api/geodata/update?nodeId=...` 远程分支调用 `RequestNodeGeoUpdate` |
| `internal/delivery/http/handler_agent.go` | `POST /api/agent/sync` Agent 同步入口 |
| `internal/delivery/http/middleware_agent.go` | `AgentAuth` Bearer Token 中间件与 `GetAgentNodeID` |
| `internal/delivery/http/handler_node.go` | `/api/admin/nodes` CRUD、ResetTraffic 等 HTTP Handler |
| `internal/delivery/cron/traffic_sync.go` | `TrafficSyncJob`（5s 采样、每日单次 `pruneOldTrafficLogsIfNeeded`、5m `alertTicker`） |
| `main.go` | `TrafficSyncJob` 构造、`.WithNodeQuotaCron(nodeSvc)` 及 `errgroup` 注册 |
| `internal/agent/protocol.go` | Agent 侧 `SyncRequest` / `SyncResponse` / `Log*` / `GeoUpdate*` JSON 协议 |
| `internal/agent/client.go` | Agent HTTP Client（当前仅 `POST /api/agent/sync`，15s 超时） |
| `internal/agent/runner.go` | `Runner.SyncOnce` 串行处理 `LogRequest` / `GeoUpdateRequest` 与 `sendLightweightAck` |
| `internal/agent/geodata_updater.go` | `HTTPGeoDataUpdater`（带 `mu sync.Mutex` 防重入、下载校验与失败回滚） |
| `internal/agent/log_collector.go` | `JournalctlLogCollector`（服务名白名单与行数限制） |
| `cmd/agent/main.go` | Agent CLI 装配 `Runner`、`LogCollector`、`GeoDataUpdater` |
| `internal/adapter/telegram/bot.go` | Telegram `/nodedrain` `/noderestore` 调用 `SetNodeDesiredState` |
| `web/src/api/index.ts` | axios 单例（30s 默认超时）与 mock 分流 |
| `web/src/api/nodes.ts` | 节点 API 封装及 `RemoteGeoUpdateResult` 类型 |
| `web/src/views/LogsView.vue` | 远端按需日志 UI（35s 单次请求 + pending 手动重试） |
| `web/src/views/RoutingView.vue` | 本机 `/geodata/progress` 500ms 轮询 vs 远端 90s 长阻塞请求 |
| `web/src/mock/index.ts` | 前端 mock 模式 `/logs` 与 `/geodata/*` 拦截器 |
| `internal/service/node_service_test.go` | `RequestNodeLogs`、`RequestNodeGeoUpdate`、`RefreshSyncStatus` 单测 |
| `internal/delivery/http/handler_geodata_test.go` | 远程 GeoData HTTP Handler 测试 |
| `internal/delivery/http/agent_e2e_test.go` | Master-Agent E2E 含远程日志与 GeoData 协同测试 |

---

## 5. Relevant `.trellis/spec/` Files

- `.trellis/spec/backend/index.md`
- `.trellis/spec/backend/directory-structure.md`
- `.trellis/spec/backend/database-guidelines.md`
- `.trellis/spec/backend/error-handling.md`
- `.trellis/spec/backend/quality-guidelines.md`
- `.trellis/spec/backend/logging-guidelines.md`
- `.trellis/spec/frontend/index.md`
- `.trellis/spec/frontend/api-and-dataflow.md`
- `.trellis/spec/frontend/components-and-routing.md`
- `.trellis/spec/frontend/quality-guidelines.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`
- `.trellis/spec/guides/code-reuse-thinking-guide.md`

---

## 6. Caveats / Open Questions

1. **HTTP `WriteTimeout=60s` 与 `75s` 等待直接冲突**：`internal/delivery/http/server.go:45` 硬编码 `WriteTimeout: 60 * time.Second`，但 `defaultRemoteGeoWaitTimeout` 为 `75s`（`RoutingView.vue` 设了 `90s`），真实生产环境下远程 GeoUpdate 必在 60s 被服务端强制断开；改为异步触发（立即返回 `commandId`）+ 状态轮询即可彻底消除。
2. **`TrafficSyncJob.syncOnce` 的早退路径**：`internal/delivery/cron/traffic_sync.go:149-151` 在 `j.xrayManager == nil` 时直接 return，导致末尾 `pruneOldTrafficLogsIfNeeded` 不会执行；接入 `PruneBatchesBefore` 时应挂在独立维护循环（如 `alertTicker` 分支或每日去重逻辑中，且独立于 `xrayManager`）。
3. **Agent 与 Master 协议兼容 / 测试迁移**：`internal/service/node_service_test.go` (`700-809`, `980-1097`)、`internal/delivery/http/handler_geodata_test.go` (`108-210`) 与 `internal/delivery/http/agent_e2e_test.go` (`1326-1427`, `1457-1813`) 当前强依赖 `RequestNodeLogs` / `RequestNodeGeoUpdate` 同步阻塞 + `LogRequest`/`GeoUpdateRequest` + `sendLightweightAck` 契约，设计 M2 异步 `NodeCommand` 时需规划好旧接口/单测的演进或兼容策略。
4. **`NodeService.DeleteNode` 与 `UpdateNode` 对并发态的影响**：`DeleteNode` 删库时顺手清理节点内存命令队列（以及 `snapshots`/`nodeEpochs`），可避免残余命令泄漏；`UpdateHeartbeat` 中的 `isLightweightResultAck` 分支若未来改为独立上报端点，需注意保留向后兼容或协同改造。
