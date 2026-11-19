# Implementation Plan: Node Sync Status, Multi-Node Topology & Remote GeoData

## Execution Strategy & Parallel Slices

拆分为 **3 个 100% 文件互斥的并行 Slice**（Swarm `workspace:"share"`），完成后归拢至主工作区并由 `verifier-polish` 补写端到端集成测试与全量门禁验收。

---

### Slice 1: Master Sync Status & Remote GeoData Broker (`master-polish`)

**Owned Files:**
- `internal/domain/node.go`
- `internal/adapter/repository/node_repo.go`（仅确保 `GeoUpdateResult` 轻量 ack 同 `LogResult` 一样不清空/覆盖无关状态，且配置应用 ack 正常更新 `last_config_hash` 与清空 `last_sync_error`）
- `internal/service/node_service.go`, `internal/service/node_service_test.go`
- `internal/delivery/http/handler_geodata.go`, `internal/delivery/http/handler_geodata_test.go` (new)
- `main.go`

**Ordered Checklist (Slice 1):**
1. `domain.Node` 新增 `ExpectedConfigHash`、`ConfigSyncStatus`、`RefreshSyncStatus(expectedHash)` 及 `AgentGeoUpdateRequest`/`AgentGeoUpdateResult` 协议 DTO。
2. `NodeService.ListNodes` / `GetNode` 容错读取 `getOrBuildSnapshot` 填充 `ExpectedConfigHash` 与 `ConfigSyncStatus`；实现 `RequestNodeGeoUpdate(ctx, nodeID)`（`pendingGeoReqs` + `geoWaiters` 带 `time.NewTimer` 与 `defer` 清理，无泄漏）并在 `HandleAgentSync` 投递/接收唤醒。
3. `handler_geodata.go` 新增 `WithNodeService` 与 `?nodeId=` 分流（`nodeId == 0` 零回归；`nodeId > 0` 调用 `RequestNodeGeoUpdate`），并在 `main.go` 装配。
4. 补充全面单测并跑通后端测试、`-race`、`go vet`、`go build .`。

---

### Slice 2: Agent Immediate Config Ack & Safe GeoData Updater (`agent-polish`)

**Owned Files:**
- `internal/agent/protocol.go`
- `internal/agent/config_manager.go`, `internal/agent/config_manager_test.go`
- `internal/agent/geodata_updater.go` (new), `internal/agent/geodata_updater_test.go` (new)
- `internal/agent/runner.go`, `internal/agent/runner_test.go`
- `cmd/agent/main.go`

**Ordered Checklist (Slice 2):**
1. 按 `design.md` §2.2 扩展 `internal/agent/protocol.go`（`GeoUpdateRequest` / `GeoUpdateResult`）。
2. 在 `config_manager.go` 暴露 `ReloadAndProbe(ctx)` 复用安全重载与探活逻辑。
3. 实现 `internal/agent/geodata_updater.go`（双源 fallback 下载 `geoip.dat` + `geosite.dat`、校验 `>= 100KB`、`.bak` 备份与原子 `os.Rename`、重载探活失败自动还原 `.bak`）+ 全面单测（含 mock HTTP 返回小体积 404 HTML 拦截测试、探活失败回滚测试）。
4. 在 `runner.go` 实现：(a) 应用新 `ConfigJSON` 或差量同步 `Users` 成功后立即补发轻量 ack（`HostMetrics: nil`, `TrafficBatch: nil`）；(b) 收到 `resp.GeoUpdateRequest` 后执行 `GeoDataUpdater.Update` 并立即轻量 ack 回传 `GeoUpdateResult`；在 `cmd/agent/main.go` 新增 `--xray-assets-dir`（env `XRAY_ASSETS_DIR`，默认 `/usr/local/share/xray`）并注入 `GeoDataUpdater`。
5. 跑通 `go test -race ./internal/agent/...`、`go build ./cmd/agent` 及 `go list -deps ./cmd/agent` 0 重型依赖检查。

---

### Slice 3: Frontend Sync Status Badges, Topology Fix & Remote GeoData UI (`ui-polish`)

**Owned Files:**
- `web/src/api/nodes.ts`
- `web/src/composables/useActiveNode.ts`
- `web/src/App.vue`
- `web/src/views/NodesView.vue`
- `web/src/views/TopologyView.vue` 及 `web/src/views/topology/components/*`（最小必要修改）
- `web/src/views/RoutingView.vue`

**Strict Prohibitions (Slice 3):**
- 严禁引入 Pinia 或修改 `internal/**`/`cmd/**`/`.trellis/**`/`deploy/**`；
- 跑完 `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build` 后**严禁提交 `web/dist/**`**（只提交 `web/src/**`）。

---

### Post-Merge Integration & Verification (`verifier-polish`)

1. 在主工作区归拢 3 个 Slice，核查三端字段与路由契约 100% 一致；
2. 在 `internal/delivery/http/agent_e2e_test.go` 扩写端到端集成测试，验证：
   - 修改远端节点出站后 `Node.ConfigSyncStatus` 立刻变 `"pending"` → Agent 单次 `SyncOnce` 应用配置并自动轻量 ack → `Node.ConfigSyncStatus` 立即变 `"synced"` 且零重复记账；
   - 发起 `POST /api/geodata/update?nodeId=<id>` → Agent 收到 `geoUpdateRequest` 下载更新并立即回传 `geoUpdateResult` 唤醒前端 HTTP 请求！
3. 执行全量前后端质量门禁。

## Validation Commands

```bash
mise x -- go test -count=1 ./...
CGO_ENABLED=1 mise x -- go test -race -count=1 ./internal/service/... ./internal/agent/... ./internal/delivery/http/... ./internal/adapter/repository/...
mise x -- go vet ./...
cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build
mise x -- go build .
mise x -- go build ./cmd/agent
mise x -- go list -deps ./cmd/agent
```
