# Technical Design: Node Sync Status, Multi-Node Topology & Remote GeoData

## 1. Parallel Slice Architecture (3 File-Disjoint Slices)

1. **Slice 1: Master Sync Status & Remote GeoData Broker (`master-polish`)**
   - `internal/domain/node.go`
   - `internal/service/node_service.go`, `internal/service/node_service_test.go`
   - `internal/delivery/http/handler_geodata.go`, `internal/delivery/http/handler_geodata_test.go` (new/update)
   - `main.go`（`geoDataHandler.WithNodeService(nodeSvc)`）
2. **Slice 2: Agent Immediate Config Ack & Safe GeoData Updater (`agent-polish`)**
   - `internal/agent/protocol.go`
   - `internal/agent/config_manager.go`（新增导出方法 `ReloadAndProbe(ctx)` 复用已有 `reloadService` + `waitForProbe`）
   - `internal/agent/geodata_updater.go`, `internal/agent/geodata_updater_test.go` (new)
   - `internal/agent/runner.go`, `internal/agent/runner_test.go`
   - `cmd/agent/main.go`
3. **Slice 3: Frontend Sync Status Badges, Topology Fix & Remote GeoData UI (`ui-polish`)**
   - `web/src/api/nodes.ts`
   - `web/src/composables/useActiveNode.ts`
   - `web/src/App.vue`
   - `web/src/views/NodesView.vue`
   - `web/src/views/TopologyView.vue`, `web/src/views/topology/components/OutboundNodeCard.vue`, `web/src/views/topology/components/TopologyPoolsView.vue`（按实际组件路径最小修改）
   - `web/src/views/RoutingView.vue`
   - **严禁提交 `web/dist/**`**（由 `verifier-polish` 在主工作区统一刷新构建）。

---

## 2. Locked Cross-Layer Contracts

### 2.1 Node Sync Status Fields (`internal/domain/node.go` ↔ `web/src/api/nodes.ts`)
```go
const (
    NodeSyncStatusSynced  = "synced"
    NodeSyncStatusPending = "pending"
    NodeSyncStatusError   = "error"
)
// On domain.Node (gorm:"-"):
ExpectedConfigHash string `gorm:"-" json:"expectedConfigHash"`
ConfigSyncStatus   string `gorm:"-" json:"configSyncStatus"` // "synced" | "pending" | "error"
```

### 2.2 Agent Sync GeoData Fields (`internal/domain/node.go` ↔ `internal/agent/protocol.go`)
```json
// Inside AgentSyncResponse / SyncResponse (omitempty):
{
  "geoUpdateRequest": {
    "requestId": "geo-1732020000-a1b2c3d4"
  }
}

// Inside AgentSyncRequest / SyncRequest (omitempty):
{
  "geoUpdateResult": {
    "requestId": "geo-1732020000-a1b2c3d4",
    "geoipSize": 10485760,
    "geositeSize": 5242880,
    "durationMs": 3240,
    "error": "",
    "updatedAtUnix": 1732020004
  }
}
```

### 2.3 HTTP `POST /api/geodata/update?nodeId=<id>`
- `nodeId == 0`（或缺省）：维持现有 `202 Accepted` + `{"message": "..."}`；
- `nodeId > 0`：返回 `200 OK`：
```json
{
  "nodeId": 1,
  "pending": false,
  "geoipSize": 10485760,
  "geositeSize": 5242880,
  "durationMs": 3240,
  "updatedAt": 1732020004,
  "error": ""
}
```
- 等待窗口默认 `75s`（略大于 60s 心跳周期 + 下载耗时，可由单测覆盖）；超时返回 `200 OK` + `pending: true`。

---

## 3. Critical Safety Invariants

1. **防轻量 Ack 重复记账与防递归**：
   - Agent 补发的轻量 ack 严格设置 `HostMetrics: nil`, `TrafficBatch: nil`；
   - Master 在 `HandleAgentSync` 处理完 `GeoUpdateResult` / 新 `ConfigHash` 后，不再重复下发相同 `GeoUpdateRequest`（已 pop）且 `ConfigUnchanged == true && UsersUnchanged == true`，绝不触发二次递归 ack；
   - 核查 `node_repo.go` `UpdateHeartbeat`：`isLightweightLogAck` 应仅跳过覆盖 `last_sync_error` 当纯粹是 `LogResult`/`GeoUpdateResult` 回传时，而配置应用成功后的轻量 ack（`LogResult == nil && GeoUpdateResult == nil`）允许把 `last_config_hash`/`last_users_hash` 更新并清空旧 `last_sync_error`！
2. **GeoData 原子安全回滚**：
   - 下载必须使用流式拷贝至 `.tmp` 并校验 `size >= 100*1024` bytes（100KB），避免镜像站 404/限流 HTML 页损坏 Xray 规则库；
   - 替换前备份 `.bak`；全部替换完调用 `cfgMgr.ReloadAndProbe(ctx)`，探活失败自动把两份 `.bak` 还原回目标路径并再次重载 Xray，向 Master 回报详细 `error`。
3. **cmd/agent 零重型依赖**：严禁在 `internal/agent/geodata_updater.go` 引入任何外部框架依赖（仅 Go 标准库 `net/http`, `io`, `os`, `path/filepath`）。
