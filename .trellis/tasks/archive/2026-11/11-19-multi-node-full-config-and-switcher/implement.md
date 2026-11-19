# Implementation Plan: Global Node Switcher & Per-Node Full Config/Logs

## Execution Strategy & Parallel Slices

本任务拆分为 **3 个 100% 文件互斥的并行 Slice**（适配 Swarm `workspace:"share"`），完成后归拢至主工作区并由 `verifier-fullcfg` 补写端到端集成测试与跑全量前后端质量门禁。

---

### Slice 1: Master Per-Node Config, Auto-Relay Injection & Log Broker (`implementer-master-fullcfg`)

**Owned Files:**
- `internal/domain/node.go`, `internal/domain/repository.go`
- `internal/adapter/repository/node_repo.go`, `internal/adapter/repository/node_repo_test.go`
- `internal/adapter/xray/compiler.go`, `internal/adapter/xray/compiler_test.go`
- `internal/service/config_service.go`, `internal/service/config_service_test.go`
- `internal/service/node_service.go`, `internal/service/node_service_test.go`
- `internal/delivery/http/handler_outbound.go`, `internal/delivery/http/handler_routing.go`, `internal/delivery/http/handler_dns.go`, `internal/delivery/http/handler_config.go`, `internal/delivery/http/handler_log.go`, `internal/delivery/http/router.go`
- `main.go`

**Ordered Checklist (Slice 1):**
1. 扩展 `domain.Node`（`OutboundsJSON`/`RoutingJSON`/`DNSJSON`）、`AgentLogRequest`/`AgentLogResult`、`StaticInboundClient`，在 `node_repo.go` 实现 `UpdateNodeConfig` + 单测。
2. 扩展 `XrayCompiler` 新增 `CompileRemoteNodeFull(inbounds, outbounds, routing, dns, users, staticClients)`（强制保底 `api` 入站、`direct`/`block` 出站、Layer 1 `api -> api` 路由规则、精确保留静态中转 client 的原始 `flow`）+ 单测。
3. 扩展 `ConfigService` 支持 `ByNode(ctx, nodeID)` 系列方法（`nodeID==0` 保持 100% 兼容，`nodeID>0` 读写 `Node` 并编译校验 + `invalidateNodeCache`；本机出站修改后 `invalidateAllNodeCaches`）+ 单测。
4. 扩展 `NodeService`：
   - 接入 `LocalOutboundsProvider` 自动提取匹配 `node.Host:port` 的 VLESS/VMess/Trojan 中转凭据并传给 `CompileRemoteNodeFull`；
   - 实现 `RequestNodeLogs(ctx, nodeID, service, lines, forceRefresh)` + `pendingLogReqs`/`logCache`/`logWaiters` 状态机并在 `HandleAgentSync` 投递 `LogRequest` / 接收 `LogResult` 唤醒等待方；
   - 补全全面单测。
5. 扩展 5 个 HTTP Handler 解析 `?nodeId=` 并更新 `main.go` 装配，跑通后端全套单测、`-race`、`go vet`、`go build .`。

---

### Slice 2: Agent On-Demand Log Collector (`implementer-agent-logs`)

**Owned Files:**
- `internal/agent/protocol.go`
- `internal/agent/log_collector.go` (new)
- `internal/agent/log_collector_test.go` (new)
- `internal/agent/runner.go`
- `internal/agent/runner_test.go`
- `cmd/agent/main.go`

**Ordered Checklist (Slice 2):**
1. 按 `design.md` §2.1 锁定协议扩展 `internal/agent/protocol.go`（`LogRequest` / `LogResult`）。
2. 实现 `internal/agent/log_collector.go`（白名单仅允许 `xray`/`xray-agent`、行数钳位 `[1,1000]`、执行 `journalctl`）+ 单元测试（含非法 service 拦截测试）。
3. 扩展 `Runner.SyncOnce` 处理 `resp.LogRequest` 后立即发一次轻量 `Sync`（`TrafficBatch: nil`, `HostMetrics: nil`）回传 `LogResult`，在 `cmd/agent/main.go` 注入 `JournalctlLogCollector`；补充单测验证 0 平时开销与不重复记账。
4. 跑通 `go test -race ./internal/agent/...`、`go build ./cmd/agent` 及 `go list -deps ./cmd/agent` 0 重型依赖检查。

---

### Slice 3: Frontend Global Node Switcher & View Integration (`implementer-ui-switcher`)

**Owned Files:**
- `web/src/composables/useActiveNode.ts` (new)
- `web/src/App.vue`
- `web/src/views/InboundsView.vue`, `web/src/views/inbounds/composables/useInboundList.ts`, `web/src/views/inbounds/components/InboundFormDrawer.vue`
- `web/src/views/OutboundsView.vue`
- `web/src/views/RoutingView.vue`
- `web/src/views/DNSView.vue`
- `web/src/views/ConfigView.vue`
- `web/src/views/LogsView.vue`

**Strict Prohibitions (Slice 3):**
- 严禁引入 Pinia/Vuex 或第三方新依赖；
- 严禁修改 `internal/**`、`cmd/**`、`.trellis/**`、`deploy/**`；
- 跑完 `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build` 后**严禁提交 `web/dist/**`**（只提交 `web/src/**`）。

---

### Post-Merge Integration & Verification (`verifier-fullcfg`)

1. 在主工作区归拢 3 个 Slice，核查三端契约（`LogRequest`/`LogResult`、`?nodeId=` 接口）100% 对齐；
2. 扩展 `internal/delivery/http/agent_e2e_test.go` 端到端集成测试，验证：
   - 远端节点自定义 `warp-out` 出站 + 路由 + DNS 编译下发成功且包含 `api` 入站/路由；
   - Master 本机出站自动静态注入远端中转凭据（原空 `flow` 不被误补 `vision`）且 `UserSyncer` 不误删；
   - 发起远端日志拉取 → Agent 收到 `logRequest` 采集并立即回传 `logResult` 唤醒前端 HTTP 请求！
3. 执行全量前后端质量门禁。

## Validation Commands

```bash
mise x -- go test -count=1 ./...
CGO_ENABLED=1 mise x -- go test -race -count=1 ./internal/service/... ./internal/agent/... ./internal/delivery/http/... ./internal/adapter/xray/... ./internal/adapter/repository/...
mise x -- go vet ./...
cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build
mise x -- go build .
mise x -- go build ./cmd/agent
mise x -- go list -deps ./cmd/agent
```
