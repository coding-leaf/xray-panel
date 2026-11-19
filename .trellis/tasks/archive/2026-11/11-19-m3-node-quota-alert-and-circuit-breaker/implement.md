# M3 Implementation Plan: Node Monthly Quota Dual-Accounting, Alerts & Auto Circuit Breaker

## Execution Strategy & Parallel Slices

本里程碑拆分为 **3 个 100% 文件互斥的并行 Slice**（适配 Swarm `workspace:"share"`），完成后归拢到主工作区并由 `verifier-m3` 补跑 Master↔Agent 端到端自动熔断与断网自治熔断集成测试及全量门禁。

---

### Slice 1: Master Quota Engine, Alerts, Cron & TG Bot (`implementer-master-quota`)

**Owned Files:**
- `internal/domain/node.go`, `internal/domain/repository.go`
- `internal/adapter/repository/sqlite_db.go`, `internal/adapter/repository/node_repo.go`, `internal/adapter/repository/node_repo_test.go`
- `internal/service/node_service.go`, `internal/service/node_service_test.go`
- `internal/service/alert_service.go`, `internal/service/alert_service_test.go`
- `internal/delivery/http/handler_node.go`, `internal/delivery/http/router.go`, `internal/delivery/http/handler_agent_test.go`
- `internal/delivery/cron/traffic_sync.go`
- `internal/adapter/telegram/bot.go`, `internal/adapter/telegram/bot_test.go`
- `main.go`

**Ordered Checklist (Slice 1):**
1. 扩展 `domain.Node`、`CreateNodeDTO`、`UpdateNodeDTO`、`AgentSyncResponse` (`QuotaPolicy`)，新增 `domain.NodeTrafficDaily` 与纯函数 `CalcTrafficByMode` / `CalcCycleUsedBytes` / `RefreshComputed`。
2. 在 `sqlite_db.go` 注册 `NodeTrafficDaily` AutoMigrate；扩展 `node_repo.go` 支持周期流量原子累加、按天日志 upsert、更新自动状态、查询近期每日日志、账单重置清零及白名单字段更新 + 单元测试。
3. 扩展 `NodeService`：
   - 校验并保存配额/口径/系数/阈值；
   - `HandleAgentSync` 落库后重读节点，评估 90% `draining` / 95% `circuit_broken` 自动状态流转、回调 `AlertService` 即时推送通知、并在响应中下发 `QuotaPolicy`；
   - 新增 `ResetNodeCycleTraffic`（供手动 API 与恢复调用）与 `CheckAndResetMonthlyTraffic`（独立账单日短月份自适应 + 自动恢复）；
   - 新增 `SetNodeDesiredState`（供 TG Bot `/nodedrain` & `/noderestore` 调用，记录 `manual` 避免误覆盖）。
4. 扩展 `AlertService`（注入可选 `WithNodeQuotaRepos(nodeRepo, dailyRepo)` 保持旧构造兼容）：实现 `CheckNodeQuotas`（80% 预警 + 近 24h 耗尽预测 + 24h 冷却）与即时 `NotifyNodeStateChange`。
5. 在 `TrafficSyncJob` 5 分钟巡检挂载 `nodeSvc.CheckAndResetMonthlyTraffic` 与 `alertSvc.CheckNodeQuotas`；在 `handler_node.go`/`router.go` 挂载 `POST /api/admin/nodes/:id/reset-traffic`；在 `telegram/bot.go` 新增 `/nodes`、`/nodedrain <id>`、`/noderestore <id>` 并更新 `main.go` 装配。
6. 全套单元测试 + `-race` + `go vet` + `go build .`。

---

### Slice 2: Agent Local Autonomous Circuit Breaker (`implementer-agent-quota`)

**Owned Files:**
- `internal/agent/protocol.go`
- `internal/agent/quota_store.go` (new)
- `internal/agent/quota_store_test.go` (new)
- `internal/agent/runner.go`
- `internal/agent/runner_test.go`
- `cmd/agent/main.go`

**Ordered Checklist (Slice 2):**
1. 按 `design.md` §2.1 锁定协议扩展 `internal/agent/protocol.go` 新增 `QuotaPolicy` 与 `SyncResponse.QuotaPolicy`。
2. 实现 `internal/agent/quota_store.go`（复刻 `spool.go` 的 `write-fsync-rename` 原子落盘模式，存储 `quota_state.json`，实现 `UpdatePolicy`、`RecordOfflineUsage`、`ShouldBreak`、`MarkBroken`、`CalcTrafficByMode`）+ 单元测试。
3. 升级 `internal/agent/runner.go` 与 `cmd/agent/main.go`：
   - 同步成功时将 `resp.QuotaPolicy` 同步至 `QuotaStore`（清零离线累计）；
   - 同步失败时将本轮 NIC（或 Xray 回退）增量累加到 `QuotaStore`，达到硬熔断阈值后立即调用 `cfgMgr.ApplyCircuitBreaker(ctx)` 自主断流；
   - Master 恢复且下发 `active` 时清除本地熔断态并自动恢复配置；
   - 补全断网自治熔断与恢复单元测试。
4. 校验 `go test -race ./internal/agent/...`、`go build ./cmd/agent` 及 `go list -deps ./cmd/agent` 0 重型依赖。

---

### Slice 3: Frontend Quota Config & Watermark UI (`implementer-ui-quota`)

**Owned Files:**
- `web/src/api/nodes.ts`
- `web/src/views/nodes/types.ts`
- `web/src/views/nodes/composables/useNodes.ts`
- `web/src/views/nodes/components/NodeFormDrawer.vue`
- `web/src/views/NodesView.vue`

**Strict Prohibitions (Slice 3):**
- **严禁触碰** `web/src/style.css`、`PortalClaimView.vue`、`UserShareModal.vue`、`deploy/**`、`internal/**`、`.trellis/**`；
- 构建测试后**严禁提交 `web/dist/**`**（只 commit `web/src/**`）。

---

### Post-Merge Integration & Verification (`verifier-m3`)

1. 在主工作区归拢 3 个 Slice，核查 Master ↔ Agent `quotaPolicy` 字段与前端 `nodes.ts` 字段 100% 对齐；
2. 扩展 `internal/delivery/http/agent_e2e_test.go` 端到端集成测试，验证：
   - 实时跨越 90% 自动 `draining`（订阅摘除）→ 跨越 95% 自动 `circuit_broken`（Agent 停业务入站）；
   - 关闭测试 `httptest.Server` 模拟 Master 失联 → Agent 本地累计流量突破阈值触发断网自治硬熔断！
3. 执行完整前后端质量门禁。

## Validation Commands

```bash
mise x -- go test ./...
CGO_ENABLED=1 mise x -- go test -race -count=1 ./internal/service/... ./internal/agent/... ./internal/delivery/http/... ./internal/adapter/repository/... ./internal/adapter/telegram/...
mise x -- go vet ./...
cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build
mise x -- go build .
mise x -- go build ./cmd/agent
mise x -- go list -deps ./cmd/agent
```
