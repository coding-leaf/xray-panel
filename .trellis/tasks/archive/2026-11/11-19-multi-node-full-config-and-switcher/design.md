# Technical Design: Global Node Switcher & Per-Node Full Config/Logs

## 1. Parallel Slice Architecture (3 File-Disjoint Slices)

1. **Slice 1: Master Per-Node Config, Auto-Relay Injection & Remote Log Broker (`implementer-master-fullcfg`)**
   - `internal/domain/node.go`, `internal/domain/repository.go`
   - `internal/adapter/repository/node_repo.go`, `internal/adapter/repository/node_repo_test.go`
   - `internal/adapter/xray/compiler.go`, `internal/adapter/xray/compiler_test.go`
   - `internal/service/config_service.go`, `internal/service/config_service_test.go`
   - `internal/service/node_service.go`, `internal/service/node_service_test.go`
   - `internal/delivery/http/handler_outbound.go`, `handler_routing.go`, `handler_dns.go`, `handler_config.go`, `handler_log.go`, `router.go`
   - `main.go`
2. **Slice 2: Agent On-Demand Log Collector & Immediate Ack Sync (`implementer-agent-logs`)**
   - `internal/agent/protocol.go`
   - `internal/agent/log_collector.go`, `internal/agent/log_collector_test.go` (new)
   - `internal/agent/runner.go`, `internal/agent/runner_test.go`
   - `cmd/agent/main.go`
3. **Slice 3: Frontend Global Node Switcher & View Integration (`implementer-ui-switcher`)**
   - `web/src/composables/useActiveNode.ts` (new)
   - `web/src/App.vue`
   - `web/src/views/InboundsView.vue`, `web/src/views/inbounds/composables/useInboundList.ts`, `web/src/views/inbounds/components/InboundFormDrawer.vue`
   - `web/src/views/OutboundsView.vue`
   - `web/src/views/RoutingView.vue`
   - `web/src/views/DNSView.vue`
   - `web/src/views/ConfigView.vue`
   - `web/src/views/LogsView.vue`
   - **严禁提交 `web/dist/**`**（由 `verifier` 在主工作区统一刷新构建）。

---

## 2. Locked Cross-Layer Contracts

### 2.1 `POST /api/agent/sync` On-Demand Log Addition

Master (`internal/domain/node.go`) 与 Agent (`internal/agent/protocol.go`) 均对称新增（`omitempty`，平时为 `nil`）：

```json
// Inside AgentSyncResponse / SyncResponse:
{
  "logRequest": {
    "requestId": "log-1732018800-a1b2c3d4",
    "service": "xray",
    "lines": 100
  }
}

// Inside AgentSyncRequest / SyncRequest:
{
  "logResult": {
    "requestId": "log-1732018800-a1b2c3d4",
    "service": "xray",
    "lines": ["Nov 19 20:50:01 ..."],
    "error": "",
    "collectedAtUnix": 1732018802
  }
}
```

### 2.2 Admin HTTP `?nodeId=` Contract
- 解析规则：公共辅助 `parseOptionalNodeID(c *gin.Context) (uint, bool)`（缺省或 `"0"` 返回 `0, true`；负数/非整数返回 `400 Bad Request`）。
- 统一透传 `?nodeId=<id>` 的接口：
  - `GET /api/outbounds?nodeId=` / `POST /api/outbounds?nodeId=` / `DELETE /api/outbounds/:tag?nodeId=`
  - `GET /api/routing?nodeId=` / `POST /api/routing?nodeId=`
  - `GET /api/dns?nodeId=` / `POST /api/dns?nodeId=`
  - `GET /api/config/raw?nodeId=`（`nodeId > 0` 返回 `{ "raw": "<compiled json>", "readOnly": true }`）
  - `GET /api/logs?nodeId=<id>&service=xray|xray-agent&lines=100&refresh=true`：
    - `nodeId == 0`：维持现有 `{ "logs": [...], "total": N }` 与 `type=access|error`；
    - `nodeId > 0`：返回 `{ "nodeId": 1, "service": "xray", "lines": [...], "cached": false, "pending": false, "collectedAt": 1732018802, "error": "" }`（等待超时 `~25s` 时返回 `200` + `pending: true`，提示前端稍后重试或继续等待，绝不返回 504 报错打断界面）。

---

## 3. Core Backend Design Details

### 3.1 Per-Node Storage on `domain.Node`
- 新增 3 列：
  - `OutboundsJSON string` `gorm:"column:outbounds_json;type:text;default:''" json:"-"`
  - `RoutingJSON string` `gorm:"column:routing_json;type:text;default:''" json:"-"`
  - `DNSJSON string` `gorm:"column:dns_json;type:text;default:''" json:"-"`
- `NodeConfigRepository`（可选扩展接口或直接在 `NodeRepository` 扩展由 `GORMNodeRepository` 实现 `UpdateNodeConfig(ctx, nodeID, outboundsJSON, routingJSON, dnsJSON)`）：独立白名单更新，绝不触碰心跳/流量/Token 字段。
- `ConfigService` 新增带 `nodeID` 的方法（保留原有无参方法作为 `nodeID=0` 薄转发，保证现有调用方与单测 100% 零破坏）：
  - `ListOutboundsByNode(ctx, nodeID)` / `SaveOutboundByNode(ctx, nodeID, dto)` / `DeleteOutboundByNode(ctx, nodeID, tag)`
  - `GetRoutingConfigByNode(ctx, nodeID)` / `SaveRoutingConfigByNode(ctx, nodeID, cfg)`
  - `GetDNSConfigByNode(ctx, nodeID)` / `SaveDNSConfigByNode(ctx, nodeID, cfg)`
  - `GetNodeCompiledConfigRaw(ctx, nodeID)`
  - 当本机（`nodeID == 0`）出站发生保存/删除时，额外调用 `s.invalidateAllNodeCaches()`，触发远端节点静态中转凭据重新编译生效！

### 3.2 Auto-Injection of Relay Credentials (`CompileRemoteNodeFull`)
- 新增辅助结构 `domain.StaticInboundClient{ InboundTag, Protocol, ID, Flow, Email }`；
- `NodeService` 注入可选 `LocalOutboundsProvider`（由 `ConfigService.ListOutbounds` 提供），在 `getOrBuildSnapshot` 编译远端节点时：
  1. 读取本机 `Outbounds` 列表；
  2. 对每个协议为 `vless` / `vmess` / `trojan` 的出站，解析 `settings.vnext[0]`（或 `servers[0]`）的 `address`、`port` 与首个 user 的 `id`/`password`、`flow`；
  3. 当 `strings.EqualFold(strings.TrimSpace(addr), strings.TrimSpace(node.Host))` 且 `port == inb.Port || (inb.ExternalPort > 0 && port == inb.ExternalPort)` 时，生成静态 client（`Email = "relay-auto:" + ob.Tag + "@internal"`，**精确保留 `ob.Flow`，空即空，绝不错误补齐 `xtls-rprx-vision`**！）；
  4. 将这些静态 client 传入 `CompileRemoteNodeFull` 合并进 `settings.clients`（若已有相同 UUID 的 DB 用户则跳过防重），并计入 `ConfigHash`；**绝不**写入 `BuildRemoteNodeUsers`，因此 `UserSyncer` 不会删掉它！

### 3.3 Agent On-Demand Log Collector (`internal/agent/log_collector.go`)
- 窄接口 `LogCollector`：`Collect(ctx context.Context, service string, lines int) ([]string, error)`；
- 默认实现 `JournalctlLogCollector`：
  - 严格白名单校验 `service == "xray" || service == "xray-agent"`（其余直接拒绝返回 error，杜绝命令注入）；
  - `lines` 钳位至 `[1, 1000]`（默认 `100`）；
  - 执行 `CommandRunner.Run(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager")` 按行拆分返回；
- `Runner.SyncOnce`：
  - 若 `resp.LogRequest != nil`：执行 `collector.Collect`，随后立即发一次**轻量 `client.Sync`**（仅携带 `ConfigHash`、`UsersHash`、`AgentVersion`、`XrayVersion`、`LogResult`，**`TrafficBatch: nil`, `HostMetrics: nil`** 防止重复累加流量或重复记账！）。
