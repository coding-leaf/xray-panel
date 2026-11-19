# M2 Implementation Plan

## Execution Slices (File-Disjoint)

### Slice A: Backend & Agent 异步指令通道、流量清理与状态补齐
**Owned Paths**: `internal/**`, `cmd/agent/**`, `main.go`

- [ ] **A1. 统一命令协议模型 (`internal/domain/node.go`, `internal/agent/protocol.go`)**
  - 定义 `NodeCommandType*`、`NodeCommandStatus*`、`AgentCommand`、`AgentCommandResult`（Agent 侧镜像 `Command`、`CommandResult`）。
  - 更新 `AgentSyncRequest`（`CommandResults []AgentCommandResult`）与 `AgentSyncResponse`（`Commands []AgentCommand`），移除旧分裂的 `AgentLog*` / `AgentGeoUpdate*` 字段。
- [ ] **A2. Master `NodeService` 异步命令状态机与清理 (`internal/service/node_service.go`)**
  - Fail-Repro：编写测试覆盖立即返回 `pending+commandId`、ctx 取消不删队、并发去重复用、TTL `180s` 过期、`ReportAgentCommandResult` 跨节点归属拒绝、`DeleteNode` 内存清理。
  - 实现 `commands` + `activeCmdByKey` + `pruneExpiredCommandsLocked` + `popPendingCommandsLocked` + `ReportAgentCommandResult` + `GetNodeCommandStatus`；移除 `pendingLogReqs`/`logWaiters`/`logReqLines`/`pendingGeoReqs`/`activeGeoReqs`/`geoWaiters` 与拼写错误的 `defaultRemoteLogWaitTimout`。
  - 更新 `RequestNodeLogs`、`RequestNodeGeoUpdate`、`HandleAgentSync`、`DeleteNode`。
- [ ] **A3. HTTP 路由与 Handler (`internal/delivery/http/`)**
  - `router.go` 注册 `GET /api/admin/nodes/:id/commands/:commandId`（JWT）与 `POST /api/agent/commands/:id/result`（`AgentAuth`）。
  - `handler_node.go` 实现 `GetCommandStatus`；`handler_agent.go` 实现 `ReportCommandResult`；`handler_geodata.go` 远端分支改为返回 `202 Accepted`；`handler_log.go` 返回更新后的 `RemoteNodeLogsResponse`（含 `commandId`/`status`）。
- [ ] **A4. Agent 非阻塞后台执行与即时上报 (`internal/agent/client.go`, `internal/agent/runner.go`)**
  - Fail-Repro：在 `runner_test.go` 编写测试验证慢速 GeoData 更新不阻塞后续 `SyncOnce`、即时上报成功、以及即时上报失败后随下次心跳 `CommandResults` 补传成功、关闭 `ctx` 无 goroutine 泄漏。
  - 实现 `Client.ReportCommandResult` 与 `Runner` 后台受控协程执行器（`cmdWG` + 防重入 + `pendingResults` 有界兜底）。
- [ ] **A5. P1-7 流量批次定时清理与级联删除**
  - Fail-Repro：在 `node_repo_test.go` 复现删节点残留孤儿 `node_traffic_batches`/`node_traffic_dailies`；在 `traffic_sync_test.go` 复现 `xrayManager == nil` 时未触发 `PruneBatchesBefore`。
  - `internal/adapter/repository/node_repo.go`：`Delete` 改为事务级联删除子表后删主表；顺带清理 `UpdateHeartbeat` 中不再需要的 `isLightweightResultAck` 分支。
  - `internal/delivery/cron/traffic_sync.go` & `main.go`：新增 `WithNodeBatchRepo` 与每日去重 `pruneOldNodeTrafficBatchesIfNeeded`，挂入后台维护协程（不依赖 `xrayManager`），保留 7 天。
- [ ] **A6. P2-1 写接口补齐 `RefreshSyncStatus`**
  - Fail-Repro：在 `node_service_test.go` 验证 `CreateNode`、`UpdateNode`、`SetNodeDesiredState`、`ResetNodeCycleTraffic` 返回非空 `ConfigSyncStatus` 与 `ExpectedConfigHash`。
  - 补齐 `internal/service/node_service.go` 四个写接口的 `RefreshComputed` + `RefreshSyncStatus`。
- [ ] **A7. 迁移并增强现有测试 (`node_service_test.go`, `handler_geodata_test.go`, `agent_e2e_test.go`, `runner_test.go`)**
  - 更新原有同步等待断言至异步 `NodeCommand` + `POST /api/agent/commands/:id/result` + `GET /api/admin/nodes/:id/commands/:commandId` 端到端全链路覆盖。

### Slice B: Frontend 远端日志与 GeoData 异步轮询改造
**Owned Paths**: `web/**`

- [ ] **B1. 封装节点命令查询 API (`web/src/api/nodes.ts`)**
  - 新增 `NodeCommandStatus`、`NodeCommandSnapshot`、`RemoteNodeLogResponse`、`RemoteGeoUpdateResponse`（含 `commandId`、`status`、`pending`）及 `getNodeCommandStatus(nodeId, commandId)`。
- [ ] **B2. `LogsView.vue` 远端日志自动轮询 (`web/src/views/LogsView.vue`)**
  - 触发 `/logs?nodeId=...` 返回 `pending: true && commandId` 时自动启动 `2s` 轮询 `getNodeCommandStatus`，展示排队/执行中状态文案，终态自动停止并渲染日志或错误提示；在 `watch(activeNodeId)`、切换 `remoteService`、组件卸载 `onUnmounted` 时清理定时器并使用请求序号防止竞态覆盖。
- [ ] **B3. `RoutingView.vue` 远端 GeoData 异步轮询 (`web/src/views/RoutingView.vue`)**
  - 移除远端 `POST /geodata/update?nodeId=...` 的 `timeout: 90000` 长阻塞；收到 `commandId` 后启动 `2s` 轮询 `getNodeCommandStatus` 直至 `succeeded`/`failed`/`expired`，展示体积/耗时或错误；在 `watch(activeNodeId)` 与 `onUnmounted` 清理定时器。
- [ ] **B4. 同步 Mock 与前端门禁 (`web/src/mock/index.ts`, `web/dist/`)**
  - 为 `/admin/nodes/:id/commands/:commandId` 及远端 `/logs` / `/geodata/update` 补充最小 mock 响应。
  - 执行 `npm run format`、`npm run typecheck`、`npm run lint`、`npm run format:check`、`npm run build`。

## Validation Commands
```bash
# Backend & Agent gates
mise x -- go test -count=1 ./...
CGO_ENABLED=1 mise x -- go test -race -count=1 ./...
mise x -- go vet ./...
mise x -- go build -o NUL .
mise x -- go build -o NUL ./cmd/agent
go list -deps ./cmd/agent | Select-String "gin|gorm|sqlite|telegram"

# Frontend gates
cd web && mise x -- npm run typecheck
cd web && mise x -- npm run lint
cd web && mise x -- npm run format:check
cd web && mise x -- npm run build
```

## Risky Files & Rollback Points
- `internal/agent/runner.go`：后台命令协程必须用 `cmdWG` 与 `ctx` 严控生命周期，避免 `-race` 报错或退出时悬空协程。
- `internal/service/node_service.go`：`commands` 与 `activeCmdByKey` 必须在同一 `s.mu` 锁内保持一致性；调用 `resolveExpectedConfigHash` 时不得在已持 `s.mu` 锁期间重入死锁。
- `web/src/views/LogsView.vue` & `RoutingView.vue`：切换节点或离开页面时必须清除轮询 `setInterval`，防止后台无效请求泄漏。
