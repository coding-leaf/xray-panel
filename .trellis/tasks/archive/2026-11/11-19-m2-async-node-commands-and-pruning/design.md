# M2 Technical Design: 统一异步 NodeCommand 通道与流量表清理收口

## 1. Architecture & Contracts

### 1.1 Domain & Agent Protocol (`internal/domain/node.go`, `internal/agent/protocol.go`)
移除 `AgentLogRequest`、`AgentLogResult`、`AgentGeoUpdateRequest`、`AgentGeoUpdateResult` 四组独立协议结构体，收敛为统一强类型命令模型（`domain` 仅依赖 stdlib，`internal/agent/protocol.go` 保持轻量同构镜像）：

```go
const (
    NodeCommandTypeLogTail       = "log_tail"
    NodeCommandTypeGeoDataUpdate = "geodata_update"

    NodeCommandStatusQueued     = "queued"
    NodeCommandStatusDispatched = "dispatched"
    NodeCommandStatusSucceeded  = "succeeded"
    NodeCommandStatusFailed     = "failed"
    NodeCommandStatusExpired    = "expired"
)

type AgentCommand struct {
    ID      string `json:"id"`
    Type    string `json:"type"`
    Service string `json:"service,omitempty"` // log_tail: "xray" | "xray-agent"
    Lines   int    `json:"lines,omitempty"`   // log_tail: 1..1000
}

type AgentCommandResult struct {
    ID              string   `json:"id"`
    Type            string   `json:"type"`
    Status          string   `json:"status"` // "succeeded" | "failed"
    Error           string   `json:"error,omitempty"`
    Service         string   `json:"service,omitempty"`
    Lines           []string `json:"lines,omitempty"`
    CollectedAtUnix int64    `json:"collectedAtUnix,omitempty"`
    GeoIPSize       int64    `json:"geoipSize,omitempty"`
    GeoSiteSize     int64    `json:"geositeSize,omitempty"`
    DurationMs      int64    `json:"durationMs,omitempty"`
    UpdatedAtUnix   int64    `json:"updatedAtUnix,omitempty"`
}
```
- `AgentSyncRequest`：`CommandResults []AgentCommandResult \`json:"commandResults,omitempty"\``（用于即时上报失败时的心跳补传）。
- `AgentSyncResponse`：`Commands []AgentCommand \`json:"commands,omitempty"\``。

### 1.2 Master `NodeService` Command State Machine (`internal/service/node_service.go`)
- **常量**：
  - `defaultRemoteLogCacheTTL = 30 * time.Second`
  - `defaultNodeCommandTTL = 180 * time.Second`（队列中或已下发超时窗口）
  - `defaultNodeCommandResultTTL = 180 * time.Second`（终态结果供前端查询保留窗口）
- **数据结构**（受 `s.mu sync.RWMutex` 保护，支持可注入 `nowFn func() time.Time` 便于单测无 sleep 验证 TTL 过期）：
  - `commands map[string]*nodeCommandRecord`（按 `commandID` 索引）
  - `activeCmdByKey map[string]string`（去重键：`"log_tail:<nodeID>:<service>"` 或 `"geodata_update:<nodeID>"` -> 当前活跃 `commandID`）
  - `logCache map[string]*cachedRemoteLog`（键：`"<nodeID>:<service>"`）
- **状态流转**：
  1. `pruneExpiredCommandsLocked(now)`：在每次入队/弹出/查询/上报时惰性执行：
     - 对 `queued` / `dispatched` 且 `now.Sub(rec.CreatedAt) > s.commandTTL` 的记录标记为 `expired`、写入 `UpdatedAt = now`、`Error = "command timed out waiting for agent"` 并从 `activeCmdByKey` 移除；
     - 对终态（`succeeded`/`failed`/`expired`）且 `now.Sub(rec.UpdatedAt) > s.commandResultTTL` 的记录从 `commands` 删除，防止内存泄漏。
  2. **入队（`RequestNodeLogs` / `RequestNodeGeoUpdate`）**：
     - `RequestNodeLogs(ctx, nodeID, svc, lines, forceRefresh)`：校验参数与节点存在 -> 若 `!forceRefresh` 命中 `logCache` (`now.Sub(CachedAt) <= 30s && RequestedLines >= lines`) 直接返回 `Cached: true, Pending: false` -> 否则加锁 `pruneExpiredCommandsLocked` -> 查 `activeCmdByKey[key]`：若已存在活跃指令则复用其 `ID`（若为 `queued` 且 `rec.Command.Lines < lines` 则原地更新 `Lines`），否则新建 `cmd-<nano>-<hex4>` 记录（`Status: queued`） -> 立即返回 `RemoteNodeLogsResponse{NodeID, Service, Pending: true, CommandID, Status}`。
     - `RequestNodeGeoUpdate(ctx, nodeID)`：校验节点存在 -> 加锁 `pruneExpiredCommandsLocked` -> 查 `activeCmdByKey["geodata_update:<nodeID>"]` 复用或新建 `Status: queued` -> 立即返回 `RemoteGeoUpdateResponse{NodeID, Pending: true, CommandID, Status}`。
  3. **心跳弹出（`popPendingCommandsLocked(nodeID, now)`）**：
     - 遍历 `commands` 中 `rec.NodeID == nodeID && rec.Status == queued` 的未过期指令（按 `CreatedAt` 升序），将其状态置为 `dispatched`（`UpdatedAt = now`）并返回副本列表给 `AgentSyncResponse.Commands`。
  4. **结果回填（`ReportAgentCommandResult(ctx, nodeID, res)`）**：
     - 校验 `res.ID != ""` 且 `commands[res.ID]` 存在且 `rec.NodeID == nodeID`（不匹配返回 `domain.ErrNotFound`）。
     - 若已终态则幂等忽略返回 `nil`；否则更新 `Status`（仅接受 `succeeded`/`failed`，否则按 `failed` 归一化）、`Error`、`UpdatedAt`、深拷贝结果负载，并从 `activeCmdByKey` 清理该 `commandID`。
     - 若为 `log_tail` 且 `succeeded`，同步写入 `logCache["<nodeID>:<service>"]`（记录 `RequestedLines: rec.Command.Lines`、`Lines` 深拷贝、`CollectedAt`、`CachedAt: now`）。
  5. **管理端状态查询（`GetNodeCommandStatus(ctx, nodeID, commandID)`）**：
     - 加锁 `pruneExpiredCommandsLocked`，查找 `rec, ok := s.commands[commandID]`，若 `!ok || rec.NodeID != nodeID` 返回 `domain.ErrNotFound`；返回只读深拷贝 `NodeCommandSnapshot`（包含 `ID`, `NodeID`, `Type`, `Status`, `CreatedAt`, `UpdatedAt`, `Error`, `LogResult`, `GeoResult`）。
  6. **节点删除清理**：`DeleteNode` 删库成功后，同时调用 `s.purgeNodeMemoryState(id)` 清理快照缓存、`nodeEpochs`、`commands`、`activeCmdByKey`、`logCache`。

### 1.3 HTTP Delivery (`internal/delivery/http/`)
- `router.go`：
  - `authGroup` 新增：`GET /api/admin/nodes/:id/commands/:commandId` -> `nodeHandler.GetCommandStatus`
  - `agentGroup` (`AgentAuth`) 新增：`POST /api/agent/commands/:id/result` -> `agentHandler.ReportCommandResult`
- `handler_geodata.go`：`POST /api/geodata/update?nodeId=...` 返回 **`202 Accepted`** + `RemoteGeoUpdateResponse{NodeID, Pending: true, CommandID, Status}`。
- `handler_log.go`：`GET /api/logs?nodeId=...` 返回 `200 OK` + `RemoteNodeLogsResponse`（命中缓存时 `cached: true, pending: false`；未命中时 `pending: true, commandId, status`）。
- `handler_agent.go`：`ReportCommandResult` 校验 `:id` 与 body `id` 一致（body 为空时用路径 `:id` 填充），传入 `GetAgentNodeID(c)` 调用 `nodeSvc.ReportAgentCommandResult`，`ErrNotFound` 映射 `404`。
- `internal/adapter/repository/node_repo.go`：`UpdateHeartbeat` 移除已无意义的 `isLightweightResultAck` 绕过（心跳就是心跳，不再靠空 `POST /api/agent/sync` 当结果回执）。

### 1.4 Agent Async Command Execution (`internal/agent/`)
- `client.go`：新增 `ReportCommandResult(ctx context.Context, res *CommandResult) error` 调用 `POST /api/agent/commands/{id}/result`。
- `runner.go`：
  - 增加 `cmdMu sync.Mutex`、`inFlight map[string]bool`（按命令 `Type` 或 `ID` 防重入，如同时只允许 1 个 `geodata_update`）、`pendingResults []CommandResult`（上限 32 条，防断网内存增长）、`cmdWG sync.WaitGroup`。
  - `SyncOnce(ctx)`：
    1. 采集流量/指标前取出 `pendingResults` 快照随 `SyncRequest.CommandResults` 一并上报；`Sync` 成功后从队列移除已确认结果。
    2. `reconcileSyncResponse` 完成后，若有 `resp.Commands`，遍历调用 `r.spawnCommand(ctx, cmd)`（立即在独立 goroutine 启动，不阻塞 `SyncOnce`）。
    3. `Run(ctx)` 退出前等待 `r.cmdWG.Wait()`（带有限超时或依赖 `ctx` 取消），确保零 goroutine 泄漏。
  - `executeCommand(ctx, cmd)`：
    - `log_tail`：调用 `r.logCollector.Collect(ctx, cmd.Service, cmd.Lines)`。
    - `geodata_update`：检查 `inFlight["geodata_update"]`（虽已有 `HTTPGeoDataUpdater.mu`，外层早检可避免重复排队），调用 `r.geoUpdater.Update(ctx)`。
    - 执行完成后优先通过 `r.client.ReportCommandResult(reportCtx, &result)` 即时回报；若失败则写入 `pendingResults` 由下一次心跳补传。

### 1.5 Batch Pruning & Cascade Delete (`P1-7`)
- `internal/adapter/repository/node_repo.go` (`Delete`)：
  ```go
  func (r *GORMNodeRepository) Delete(ctx context.Context, id uint) error {
      return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
          if err := tx.Where("node_id = ?", id).Delete(&domain.NodeTrafficBatch{}).Error; err != nil {
              return fmt.Errorf("delete node traffic batches: %w", err)
          }
          if err := tx.Where("node_id = ?", id).Delete(&domain.NodeTrafficDaily{}).Error; err != nil {
              return fmt.Errorf("delete node traffic dailies: %w", err)
          }
          res := tx.Delete(&domain.Node{}, id)
          if res.Error != nil {
              return fmt.Errorf("delete node: %w", res.Error)
          }
          if res.RowsAffected == 0 {
              return domain.ErrNotFound
          }
          return nil
      })
  }
  ```
- `internal/delivery/cron/traffic_sync.go` & `main.go`：
  - 新增 `nodeTrafficBatchRetentionDays = 7`。
  - 新增 `nodeBatchRepo domain.NodeTrafficBatchRepository`、`lastNodeBatchPruneDate string`（复用 `pruneMu`）及 `WithNodeBatchRepo(repo domain.NodeTrafficBatchRepository) *TrafficSyncJob`。
  - 新增 `pruneOldNodeTrafficBatchesIfNeeded(ctx context.Context, now time.Time)`：若 `j.nodeBatchRepo == nil` 直接返回；同自然日去重后执行 `PruneBatchesBefore(ctx, now.AddDate(0, 0, -nodeTrafficBatchRetentionDays))`（失败记 `slog.Warn` 且不更新 `lastNodeBatchPruneDate` 以便下次重试）。
  - 在 `Start(ctx)` 的后台维护协程启动时立即调用一次，并在 `alertTicker`（每 5 分钟）中周期调用 — **完全独立于 `j.xrayManager != nil`**。
  - `main.go:202` 链式挂载 `.WithNodeBatchRepo(nodeRepo)`。

### 1.6 Write DTO `RefreshSyncStatus` (`P2-1`)
- `internal/service/node_service.go`：在 `CreateNode`、`UpdateNode`（在 `InvalidateNode` 之后）、`SetNodeDesiredState`、`ResetNodeCycleTraffic` 返回前统一调用：
  ```go
  now := time.Now()
  node.RefreshComputed(now)
  node.RefreshSyncStatus(s.resolveExpectedConfigHash(ctx, node.ID))
  ```

### 1.7 Frontend Polling (`web/src/`)
- `web/src/api/nodes.ts`：新增 `NodeCommandSnapshot`、`RemoteLogResponse`、`RemoteGeoUpdateResult`（补充 `commandId?`, `status?`）与 `getNodeCommandStatus(nodeId: number, commandId: string)`。
- `web/src/views/LogsView.vue`：远端 `/logs` 返回 `pending: true && commandId` 时启动 `2000ms` 轮询定时器；展示排队/执行中状态；收到 `succeeded` 渲染日志、`failed`/`expired` 展示错误与手动重试；在 `watch(activeNodeId)`、切换 `remoteService` 与 `onUnmounted` 清理定时器并递增值守卫（`requestSeq`）防止旧响应覆盖。
- `web/src/views/RoutingView.vue`：远端 `POST /geodata/update?nodeId=...` 使用默认超时（移除 `timeout: 90000`），拿到 `commandId` 启动 `2000ms` 轮询直至终态；在 `watch(activeNodeId)` 与 `onUnmounted` 清理定时器。
- `web/src/mock/index.ts`：补充 `GET /admin/nodes/:id/commands/:commandId` 以及远端 `/logs?nodeId=`、`/geodata/update?nodeId=` mock 支持。

## 2. Fail-Repro First Testing Plan
1. **P0-2**：单元测试验证 `RequestNodeLogs` 立即返回 `pending: true` + `commandId`，客户端 cancel 不删除队列指令，并发请求复用同一 `commandId`，Agent 领走并上报后 `GetNodeCommandStatus` 返回 `succeeded` 且 30s 缓存命中，超过 `180s` 转为 `expired`。
2. **P1-1**：HTTP/E2E 测试验证 `POST /api/geodata/update?nodeId=<id>` 立即返回 `202` + `commandId`；Agent `runner_test.go` 验证慢速 GeoData 下载（模拟阻塞）不阻塞并发 `SyncOnce` 心跳且支持即时上报 + 上报失败时下次心跳补传；验证跨节点查/报指令返回 `404`。
3. **P1-7**：`node_repo_test.go` 验证 `Delete` 事务同时清空 `node_traffic_batches` 与 `node_traffic_dailies`（且删不存在节点返回 `ErrNotFound`）；`traffic_sync_test.go` 验证 `xrayManager == nil` 时 `TrafficSyncJob` 仍调用 `PruneBatchesBefore` 且同日不重复调用。
4. **P2-1**：`node_service_test.go` 验证 `CreateNode` / `UpdateNode` / `SetNodeDesiredState` / `ResetNodeCycleTraffic` 返回非空 `ConfigSyncStatus` 与 `ExpectedConfigHash`。

## 3. Parallel Ownership & Rollback Points
- **Slice A (Backend + Agent)**：独占 `internal/**`、`cmd/agent/**`、`main.go`。
- **Slice B (Frontend)**：独占 `web/**`。
- 前后端约定清晰的 JSON 契约（见 §1.2 / §1.3 / §1.7），两侧文件 100% 互斥，可安全并行。
