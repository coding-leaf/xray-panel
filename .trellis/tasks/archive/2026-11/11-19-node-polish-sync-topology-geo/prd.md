# Node Sync Status, Multi-Node Topology & Remote GeoData

## Goal

完成多机控制面的 3 项关键运维体验闭环：
1. **配置同步状态可视闭环**：让管理员在修改出站/路由/入站/DNS 后，通过顶部栏与节点列表的「🟡 待同步 → 🟢 已同步生效 / 🔴 同步失败」三态徽章，明确确认远端机已通过校验并热重载生效；
2. **修复 `TopologyView` 拓扑图**：接入全局 `activeNodeId` 节点切换，消除多节点入站与本机出站混杂错位问题，并在本机视角清晰标注指向远端托管节点的中转出站；
3. **远端按需更新 GeoData**：让 `/routing` 中的「获取最新 Geo」同样支持一键按需更新远端节点机上的 `geoip.dat` / `geosite.dat`（平时 0 流量，带最小体积校验与失败自动回滚）。

## Confirmed Facts

- 源码调研详见 [polish-audit.md](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-19-node-polish-sync-topology-geo/research/polish-audit.md)：
  - `NodeService.getOrBuildSnapshot` 已维护各节点内存缓存的 `ConfigHash`，可直接作为 `expectedConfigHash`；
  - Agent 发送轻量 ack（`HostMetrics: nil`, `TrafficBatch: nil`）不会重复记流量、不会重置心跳指标且会自动更新 `last_config_hash`/`last_users_hash` 与清空旧 `last_sync_error`；
  - `TopologyView.vue` 未接入 `useActiveNode()`，其 `fetchAll()` 与 `deleteOutbound` 漏传 `?nodeId=`，导致多节点入站与本机路由/出站混杂；
  - 本机 GeoData 接口为 `POST /api/geodata/update`，`RoutingView.vue` 当前在远端节点模式下隐藏了该卡片。

## Requirements

### R1. 配置同步三态徽章闭环（`synced` / `pending` / `error`）
- `domain.Node` 新增非持久化计算字段：`ExpectedConfigHash string`（`json:"expectedConfigHash"`）与 `ConfigSyncStatus string`（`json:"configSyncStatus"`，取值为 `"synced"` / `"pending"` / `"error"`）：
  - `LastSyncError != ""` → `"error"`；
  - `LastConfigHash != "" && ExpectedConfigHash != "" && LastConfigHash == ExpectedConfigHash` → `"synced"`；
  - 其他情况 → `"pending"`。
- `NodeService.ListNodes` / `GetNode` 容错读取 `getOrBuildSnapshot` 填充 `ExpectedConfigHash` 与 `ConfigSyncStatus`（编译出错不阻断列表查询）。
- Agent `Runner.SyncOnce` 在成功应用新 `ConfigJSON` 或差量热更新 `Users` 后，**立即补发一次轻量 ack 同步**（仅带最新 `ConfigHash`/`UsersHash`/`LastError: ""`，`HostMetrics: nil`, `TrafficBatch: nil`），让 Master 1 RTT 内刷新 `LastConfigHash` 变绿。
- 前端：
  - `web/src/api/nodes.ts` 扩展 `expectedConfigHash`、`configSyncStatus`；
  - `web/src/App.vue` 顶部栏选中远端节点时，直接展示三态同步状态徽章（`pending` 琥珀脉冲、`synced` 绿、`error` 红带悬浮报错）；
  - `web/src/views/NodesView.vue` 将原“正常/异常”列升级为三态徽章（含短 hash 与失败详情提示）；
  - `web/src/composables/useActiveNode.ts` 暴露 `refreshNodesNow()` 并在存在 `pending` 节点时提供温和的自动轮询刷新直至收敛。

### R2. 修复 `TopologyView` 拓扑图
- `web/src/views/TopologyView.vue` 接入 `useActiveNode()`：
  - `fetchAll()` 严格附带 `?nodeId=${activeNodeId.value}` 拉取 `/inbounds`、`/outbounds`、`/routing`；
  - `watch(activeNodeId, fetchAll)` 随顶部切换器实时切换单机/远端节点拓扑；
  - `deleteOutbound(tag)` 补齐 `encodeURIComponent(tag)` 与 `?nodeId=${activeNodeId.value}`；
  - Header 清晰展示当前查看的节点名称与远程托管标识。
- 本机视角（`activeNodeId === 0`）下，自动解析 `Outbound.settingsJson` 中的 `address` 与 `port`，若匹配 `nodesList` 中某个托管节点的 `host`，在出站节点卡片（`OutboundNodeCard.vue` / `TopologyPoolsView.vue`）显示清晰的远端托管节点徽章（如 `→ 托管节点 US-1` 及在线状态点）。

### R3. 远端节点按需更新 GeoData（平时 0 流量，带校验回滚）
- Master `AgentSyncResponse` 与 Agent `SyncResponse` 对称新增 `GeoUpdateRequest *AgentGeoUpdateRequest`（`json:"geoUpdateRequest,omitempty"`），`AgentSyncRequest` 与 `SyncRequest` 对称新增 `GeoUpdateResult *AgentGeoUpdateResult`（`json:"geoUpdateResult,omitempty"`），平时为 `nil`（0 额外字节）。
- `POST /api/geodata/update?nodeId=<id>`：
  - `nodeId == 0`：保持原有本机异步更新逻辑与 `202 Accepted` 零破坏；
  - `nodeId > 0`：调用 `NodeService.RequestNodeGeoUpdate(ctx, nodeID)`，复用内存 `pendingGeoReqs` + `geoWaiters` 通道（防 goroutine/channel 泄漏），等待 Agent 1-RTT 轻量 ack 回传并返回 `200 OK`（含 `nodeId`, `pending`, `geoipSize`, `geositeSize`, `durationMs`, `error`, `updatedAt`；超时返回 `pending: true` 不抛 504）。
- Agent 新增 `--xray-assets-dir`（env `XRAY_ASSETS_DIR`，默认 `/usr/local/share/xray`）与 `internal/agent/geodata_updater.go`：
  - 下载 `geoip.dat` 与 `geosite.dat`（沿用官方 v2fly + Loyalsoldier 双源 fallback）到 `.tmp`；
  - 强校验文件大小 `>= 100KB` 防镜像返回 HTML 错误页覆写现有规则库；
  - 备份 `.bak` → 原子 `os.Rename` → 重载 Xray 并 gRPC 探活（失败自动还原 `.bak` 再重载）；
  - 立即发一次轻量 ack 回传 `GeoUpdateResult`（`HostMetrics: nil`, `TrafficBatch: nil`）；严格保持 `cmd/agent` 零重型依赖红线。
- 前端 `web/src/views/RoutingView.vue` 取消远端节点对 GeoData 卡片的隐藏：远端模式下点击「获取最新 GeoData」调用 `POST /api/geodata/update?nodeId=${activeNodeId}` 并直接展示更新后的文件大小、耗时或错误信息。

## Acceptance Criteria

- [ ] **AC-1**：单元测试覆盖 `Node.RefreshSyncStatus`（`synced`/`pending`/`error`）、`NodeService.ListNodes`/`GetNode` 填充 `expectedConfigHash` 与 `configSyncStatus`，以及 Agent 应用配置后立即发送轻量 ack（不重记流量、不覆盖网卡指标）。
- [ ] **AC-2**：`TopologyView` 接入 `activeNodeId`，传参 `?nodeId=` 隔离多节点入站/出站/路由并修复 `deleteOutbound`，且本机视角正确标注指向远端托管节点的中转出站。
- [ ] **AC-3**：单元测试与端到端集成测试覆盖远端按需 GeoData 更新：`>=100KB` 校验拦截坏文件、重载/探活失败自动回滚 `.bak`、`POST /api/geodata/update?nodeId>0` 1-RTT 唤醒且不重复记流量、`nodeId=0` 零回归。
- [ ] **AC-4**：全量前后端质量门禁全部通过（`go test -count=1 ./...`、`-race`、`go vet`、`cd web && npm run typecheck && npm run lint && npm run build`、`go build .`、`go build ./cmd/agent`、`go list -deps ./cmd/agent` 0 重型依赖）。

## Out of Scope

- 修改 `DashboardView` 全局聚合图表或新增 WebSocket 推送通道。
- 自动定时周期性拉取远端 GeoData（保持按需触发，零平时流量开销）。
