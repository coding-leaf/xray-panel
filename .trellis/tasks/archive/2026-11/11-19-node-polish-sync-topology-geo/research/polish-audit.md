# 节点体验打磨架构调研报告（Sync 状态 / 多节点拓扑 / 远端 GeoData）

## 1. 配置同步三态徽章闭环（`synced` / `pending` / `error`）

### 1.1 现状与关键锚点
- **领域模型 (`internal/domain/node.go:93-136,187-202`)**:
  - `Node` 已有持久化字段 `LastConfigHash` (`:106`)、`LastUsersHash` (`:107`)、`LastSyncError` (`:108`) 以及动态计算字段 `Online` (`:134`)、`CycleUsedBytes` (`:135`)。
  - `RefreshOnline` / `RefreshComputed` (`:187-202`) 在返回给调用方前刷新动态字段。
- **服务层 (`internal/service/node_service.go:193-212,738-745,799-880`)**:
  - `ListNodes` (`:193-203`) 和 `GetNode` (`:205-212`) 仅调用 `RefreshOnline(now)`，未填充期望配置哈希。
  - `getOrBuildSnapshot(ctx, nodeID)` (`:799`) 使用 `s.snapshots[nodeID]` 内存缓存（在配置/入站/出站/路由/DNS/用户变更时通过 `InvalidateNode` / `InvalidateAll` 失效），其返回的 `cachedNodeSnapshot.ConfigHash` 即为 Master 期望该节点运行的 `expectedConfigHash`。
  - 注意：`getOrBuildSnapshot` 在 `s.compiler == nil` 时会返回错误 (`:822-824`)，因此在 `ListNodes`/`GetNode` 中调用时需容错降级（若 `err != nil` 或 `snap == nil`，不阻断节点列表返回，`expectedConfigHash` 置空）。
- **Agent 循环与轻量 Ack 安全性 (`internal/agent/runner.go:158-260`, `internal/adapter/repository/node_repo.go:74-142`, `internal/service/node_service.go:654-777`)**:
  - `runner.go:222-238`：`ConfigManager.Apply` 成功后仅在本地更新 `r.configHash = resp.ConfigHash`、`r.lastError = ""`，**未立即上报 Master**，需等下一个 `SyncIntervalSec`（默认 20s）心跳才回传 `req.ConfigHash`。同理 `runner.go:240-253` 用户差量同步后也是本地更新 `r.usersHash`。
  - **轻量 Ack 安全性核查**：
    - `HandleAgentSync` (`node_service.go:667`) 仅当 `req.TrafficBatch != nil && BatchID != ""` 时执行 `ApplyNodeTrafficBatch`；传 `TrafficBatch: nil` **绝对不会重复记账**。
    - `UpdateHeartbeat` (`node_repo.go:105-127`) 仅当 `req.HostMetrics != nil` 时更新 `cpu_percent`/`mem_percent` 并累加 `nic_up_bytes`/`nic_down_bytes`；传 `HostMetrics: nil` **不会重复累加网卡流量**。
    - **隐蔽细节 (`node_repo.go:76-103`)**：目前 `isLightweightLogAck := req.LogResult != nil && req.TrafficBatch == nil && req.HostMetrics == nil`。若 Agent 发送仅带 `ConfigHash`/`UsersHash`/`LastError: ""`（`LogResult: nil`, `HostMetrics: nil`, `TrafficBatch: nil`）的轻量 ack，会走 `!isLightweightLogAck` 分支（`:81-86`），恰好将 `last_config_hash`、`last_users_hash`、`last_sync_error = ""` 全部覆盖写入（清空旧 `LastSyncError`）。
    - **建议**：在 `runner.go` 的 `reconcileSyncResponse` 返回 `(applied bool, err error)`（或在 `ConfigJSON` 应用成功 / `Users` 差量同步成功后），由 `SyncOnce` 立即补发一次轻量 ack `SyncRequest{ConfigHash: r.configHash, UsersHash: r.usersHash, AgentVersion: r.agentVersion, XrayVersion: r.xrayVersion, LastError: "", HostMetrics: nil, TrafficBatch: nil}`；Master 收到后因 `ConfigUnchanged == true && UsersUnchanged == true` 不会重复下发配置，也不触发递归 ack。
- **前端展示与轮询**:
  - `web/src/api/nodes.ts:6-44`：`NodeItem` 接口需补充 `expectedConfigHash?: string`、`configSyncStatus?: 'synced' | 'pending' | 'error'`。
  - `web/src/composables/useActiveNode.ts:30-85`：全局 `nodesList` 与 `activeNode` 单例，供 `App.vue` 顶部节点选择器消费。
  - `web/src/App.vue:219-266,682-712`：顶部全局节点切换器当前仅展示 `activeNode?.online` 圆点；可额外在远程节点胶囊旁展示同步状态点/徽章（如 `pending` 琥珀色脉冲 / `error` 红色告警），并在 `pollCoreStatus`（6s）或触发配置变更后按需刷新 `loadNodes()`。
  - `web/src/views/NodesView.vue:273-284,447-453,656-663`：当前第 6 列仅在 `node.lastSyncError` 非空时显示错误、否则统一显示「正常」，且已有 15s 轮询（`:656`）。改为三态徽章：`error`（红 + `lastSyncError` tooltip/详情）、`synced`（绿「已同步」+ 短 hash）、`pending`（黄「待同步」+ 期望/当前短 hash）。

### 1.2 推荐后端计算契约
在 `domain.Node` 新增非持久化字段 (`gorm:"-"`)：
```go
ExpectedConfigHash string `gorm:"-" json:"expectedConfigHash"`
ConfigSyncStatus   string `gorm:"-" json:"configSyncStatus"` // "synced" | "pending" | "error"
```
`RefreshSyncStatus(expectedConfigHash string)` 判定规则：
1. `n.ExpectedConfigHash = expectedConfigHash`
2. `strings.TrimSpace(n.LastSyncError) != ""` → `"error"`
3. `n.LastConfigHash != "" && expectedConfigHash != "" && n.LastConfigHash == expectedConfigHash` → `"synced"`
4. 其余情况（未首次同步或 Master 配置已变更待 Agent 下次心跳/ack 确认）→ `"pending"`

---

## 2. 修复 `TopologyView` 拓扑图

### 2.1 问题定位（`web/src/views/TopologyView.vue`）
- **`fetchAll()` 未传 `nodeId` (`TopologyView.vue:390-414`)**：
  - `api.get('/inbounds')` 未带 `?nodeId=`，后端 `InboundHandler.List` (`internal/delivery/http/handler_inbound.go:38-51`) 在无 `nodeId` 参数时会返回**本机 Master + 所有远端节点的全部入站**，导致多节点混杂。
  - `api.get('/outbounds')` 与 `api.get('/routing')` 未带 `?nodeId=`，后端 `handler_config.go:18-24` 默认按 `nodeID = 0`（本机 Master）处理，导致切换到远端节点时仍展示 Master 的出站与路由。
- **未响应全局 `activeNodeId`**：
  - `TopologyView.vue` 未引入 `useActiveNode()`，也没有 `watch(activeNodeId, fetchAll)`。
- **删除出站漏传 `nodeId` (`TopologyView.vue:247-256`)**：
  - `api.delete(\`/outbounds/\${tag}\`)` 未带 `?nodeId=${activeNodeId.value}`（对比 `OutboundsView.vue:122` 已带 `?nodeId=${activeNodeId.value}` 且对 `tag` 做了 `encodeURIComponent`）。

### 2.2 修复方案
1. **接入 `useActiveNode()`**：
   - 在 `TopologyView.vue` 中引入 `const { activeNodeId, activeNode, isRemoteNode, nodesList, loadNodes } = useActiveNode()`。
   - `fetchAll()` 传入 `?nodeId=${activeNodeId.value}` 拉取 `/inbounds`、`/outbounds`、`/routing`（`/users` 为全局用户池保持不变）。
   - `watch(activeNodeId, () => fetchAll())`，并在 `onMounted` 中确保 `nodesList` 已加载（`if (nodesList.value.length === 0) loadNodes()`）。
   - `deleteOutbound(tag)` 改为 `api.delete(\`/outbounds/\${encodeURIComponent(tag)}?nodeId=\${activeNodeId.value}\`)`。
   - 页面 Header 补充当前节点提示徽章（对齐 `RoutingView.vue:16-33` 风格）。
2. **本机视角 (`activeNodeId === 0`) 识别中转托管节点**：
   - 参考 `OutboundNodeCard.vue:154-171`，从 `ob.settingsJson` 解析目标地址 `address`（`s.vnext?.[0]?.address` 或 `s.servers?.[0]?.address`）与端口 `port`。
   - 当 `activeNodeId === 0` 且解析出的 `address` 非空、端口 `> 0` 时，在 `nodesList` 中匹配 `n.host.trim().toLowerCase() === address.trim().toLowerCase()`；若命中，在 `TopologyNode` / `OutboundNodeCard.vue` 与 `TopologyPoolsView.vue` 的出站卡片上渲染远端托管节点徽章（如 `→ 托管节点 ${matchedNode.name}`，附在线状态点）。

---

## 3. 远端节点按需更新 GeoData (`geoip.dat` / `geosite.dat`)

### 3.1 现有本机实现锚点
- **服务层 (`internal/service/geodata_service.go`)**：
  - 目录解析 `getGeoDataDir` (`:68-84`)：优先读取设置项 `xray_geodata_dir`，否则 Linux 下若 `/usr/local/share/xray` 存在则使用之，否则回退 `filepath.Dir(s.xrayBinPath)`。
  - 下载源 URL：
    - `geoip.dat` (`:177-180`)：
      1. `https://github.com/v2fly/geoip/releases/latest/download/geoip.dat`
      2. `https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat`
    - `geosite.dat` (`:208-211`)：
      1. `https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat`
      2. `https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat`
  - HTTP 接口 (`internal/delivery/http/handler_geodata.go:19-39`, `internal/delivery/http/router.go:160-163`)：
    - `GET /api/geodata/status`
    - `GET /api/geodata/progress`
    - `POST /api/geodata/update`（当前返回 `202 Accepted` + `{"message": "GeoData 规则库更新任务已在后台启动"}`）。
  - 主程序装配 (`main.go:184-186`)：`GeoDataHandler` 当前仅注入 `geoSvc`，未注入 `nodeSvc`（可仿照 `LogHandler.WithNodeService(nodeSvc)`）。

### 3.2 远端按需通道设计（复用 `AgentSyncResponse` + `NodeService` Waiter 模式）
1. **协议契约 (`internal/domain/node.go` & `internal/agent/protocol.go`)**：
   - `AgentGeoUpdateRequest`：`{ RequestID string \`json:"requestId"\` }`
   - `AgentGeoUpdateResult`：`{ RequestID string \`json:"requestId"\`, GeoIPSize int64 \`json:"geoipSize"\`, GeoSiteSize int64 \`json:"geositeSize"\`, DurationMs int64 \`json:"durationMs"\`, Error string \`json:"error,omitempty"\`, UpdatedAtUnix int64 \`json:"updatedAtUnix"\` }`
   - 在 `AgentSyncRequest` 增加 `GeoUpdateResult *AgentGeoUpdateResult \`json:"geoUpdateResult,omitempty"\``，在 `AgentSyncResponse` 增加 `GeoUpdateRequest *AgentGeoUpdateRequest \`json:"geoUpdateRequest,omitempty"\``（`omitempty` 保证平时 0 字节开销）。
2. **Master `NodeService` 编排 (`internal/service/node_service.go`)**：
   - 复用 `RequestNodeLogs` (`:937-1070`) 的内存 waiter 模式：新增 `pendingGeoReqs map[uint]*domain.AgentGeoUpdateRequest` 与 `geoWaiters map[string][]chan *domain.AgentGeoUpdateResult`。
   - 提供 `RequestNodeGeoUpdate(ctx context.Context, nodeID uint) (*RemoteGeoUpdateResponse, error)`：校验 `nodeID > 0` 与节点存在性，写入 `pendingGeoReqs[nodeID]` 并阻塞等待 `waitCh`（超时如 `90s` 返回 `pending: true`，与 `RequestNodeLogs` 一致；若 Agent 在超时前通过轻量 ack 回传结果，则立即唤醒并返回 `GeoIPSize` / `GeoSiteSize` / `DurationMs` / `Error`）。
   - `HandleAgentSync` 中：若 `req.GeoUpdateResult != nil` 则调用 `s.handleIncomingGeoResult(nodeID, req.GeoUpdateResult)` 唤醒 waiter；响应中附带 `resp.GeoUpdateRequest = s.popPendingGeoRequest(nodeID, req.GeoUpdateResult)`。
3. **HTTP Handler (`internal/delivery/http/handler_geodata.go`)**：
   - 为 `GeoDataHandler` 增加 `WithNodeService(nodeSvc *service.NodeService)`（并在 `main.go:186` 装配）。
   - `UpdateGeoData` 解析可选 `?nodeId=`（`parseOptionalNodeID(c)`）：
     - `nodeID == 0`：保持原异步本机逻辑不变（`202 Accepted`）。
     - `nodeID > 0`：调用 `h.nodeSvc.RequestNodeGeoUpdate(c.Request.Context(), nodeID)`，同步等待并返回 `200 OK`（含 `pending`、`geoipSize`、`geositeSize`、`durationMs`、`error`）。
4. **Agent 执行器 (`cmd/agent/main.go`, `internal/agent/runner.go`, `internal/agent/geodata_updater.go`)**：
   - `cmd/agent/main.go:38-50` 新增 `--xray-assets-dir`（env `XRAY_ASSETS_DIR`，默认 `/usr/local/share/xray`）。
   - `Runner` 收到 `resp.GeoUpdateRequest != nil` 后异步或当前周期执行 `GeoDataUpdater.Update(ctx)`：
     1. 下载 `geoip.dat` 与 `geosite.dat`（沿用 Master 的双源 fallback URL）至 `.tmp` 临时文件；
     2. 校验文件大小 `>= 100 * 1024` bytes（100KB），防止镜像返回 404 HTML 错误页覆盖现有规则库；
     3. 若目标文件已存在，备份为 `.bak`；
     4. 原子替换（`os.Rename`）至 `assetsDir/geoip.dat` 与 `assetsDir/geosite.dat`；
     5. 调用 `cfgMgr.ReloadAndProbe(ctx)`（重载 Xray 服务并执行 gRPC 探活）；若探活失败则自动将 `.bak` 还原并再次重载；
     6. 无论成功或失败，立即通过轻量 `SyncRequest{ConfigHash: r.configHash, UsersHash: r.usersHash, HostMetrics: nil, TrafficBatch: nil, GeoUpdateResult: result}` 回传 Master。
5. **前端 (`web/src/views/RoutingView.vue:49-125,1122-1183`)**：
   - 取消 `v-if="isRemoteNode"` 对 GeoData 卡片的隐藏，改为同时支持本机与远端节点。
   - 远端节点模式下隐藏本机磁盘 `/geodata/status` 大小展示（或提示按需下发更新），点击「一键升级远端 GeoData」时调用 `POST /api/geodata/update?nodeId=${activeNodeId}`，等待返回结果并直接展示 `geoipSize` / `geositeSize` / `durationMs` 或错误信息（`pending` 时提示节点正在后台更新）。

---

## 4. 候选 `.trellis/spec/` 清单

- `D:/code/Workspace/xray-panel/.trellis/spec/backend/index.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/error-handling.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/quality-guidelines.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/index.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/api-and-dataflow.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/components-and-routing.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/quality-guidelines.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/guides/cross-layer-thinking-guide.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/guides/code-reuse-thinking-guide.md`
