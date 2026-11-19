# M3: Node Monthly Quota Dual-Accounting, Alerts & Auto Circuit Breaker

> Parent: `11-19-multi-node-direct-agent` | Depends on: `11-19-m2-direct-sub-aggregation-and-node-ui` (Completed)

## Goal

彻底闭环「单机/中转机或远端节点流量快爆时来不及发现与处理」的核心痛点：为每台 `Node` 建立月流量上限、独立账单重置日与 **Xray + 主网卡双口径**周期计费模型，落地 **80% TG 预警（含近 24h 耗尽预测）→ 90% 自动软摘流 → 95% 自动硬熔断（含面板失联时 Agent 本地自治硬熔断）→ 账单日自动复位恢复** 的全自动防爆体系，并补齐 Telegram Bot 节点运维指令与前端配额水位界面。

## Confirmed Facts

- M1（`e3bf065`）与 M2（`e76747f`）已归档：Master 已具备 `Node` 模型、`UpdateHeartbeat`（NIC 累计）、`ApplyNodeTrafficBatch`（Xray 累计）、`SubService` 按 `DesiredState == "active"` 过滤订阅与前端 `/nodes` 页面；Agent 已具备 `ConfigManager.ApplyCircuitBreaker` 静音配置能力与 `spool.go` 原子写盘模式（详见 [m3-audit.md](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-19-m3-node-quota-alert-and-circuit-breaker/research/m3-audit.md)）。
- 现有 `AlertService` 已有 24h 冷却缓存与 TG HTML 推送，`TrafficSyncJob` 已有 5 分钟 `alertTicker` 巡检及月度重置日短月份自适应参考实现（`user_service.go:925-984`）。
- 工作区原有未提交脏文件（`web/src/style.css`、`PortalClaimView.vue`、`UserShareModal.vue`、`deploy/*`）必须完整保留，严禁触碰。

## Requirements

### R1. 节点月流量配额、双口径周期计费与按天日志
- `domain.Node` 扩展字段（向后兼容默认值，`sqlite_db.go` 自动迁移）：
  - `TrafficLimitBytes int64`（`0` 不限）
  - `BillingResetDay int`（`0` 不自动重置，`1..31` 独立账单日，短月份自动适配月底）
  - `LastResetMonth int`（`YYYYMM`，防重复位）
  - `TrafficCountMode string`（`both` / `outbound` / `max`，默认 `both`）
  - `TrafficMultiplier float64`（安全系数，默认 `1.0`，允许 `> 0`，如 `1.05` 防机房口径偏差）
  - `WarnThresholdPct int`（默认 `80`）、`DrainThresholdPct int`（默认 `90`）、`BreakThresholdPct int`（默认 `95`）
  - `CycleXrayUpBytes`, `CycleXrayDownBytes`, `CycleNicUpBytes`, `CycleNicDownBytes int64`
  - `AutoStateReason string`（如 `auto_drain_quota` / `auto_break_quota` / `manual`）
- 统一双口径周期已用流量计算规则（纯函数 `Node.CalcCycleUsedBytes()`）：
  - 若 `CycleNicUpBytes > 0 || CycleNicDownBytes > 0`，优先以网卡周期流量 `(CycleNicUpBytes, CycleNicDownBytes)` 计算，否则回退至 `(CycleXrayUpBytes, CycleXrayDownBytes)`；
  - `both`：`up + down`；`outbound`：`up`（出站上行）；`max`：`max(up, down)`；
  - 最终乘以有效 `TrafficMultiplier`（`<= 0` 按 `1.0`）。
- 新增 `domain.NodeTrafficDaily`（`node_traffic_dailies` 表，`uniqueIndex(node_id, date)`）：在 `UpdateHeartbeat`（NIC 增量）和 `ApplyNodeTrafficBatch`（Xray 增量）原子落库周期计数与当日日志，支撑近 24h 速率与预计耗尽天数计算。
- 新增管理员手动重置节点周期流量 API：`POST /api/admin/nodes/:id/reset-traffic`（清零 `Cycle*Bytes`，若节点系配额自动触发的 `draining`/`circuit_broken` 则自动恢复 `active`）。

### R2. 实时水位流转、账单日复位与 Telegram 告警 / Bot 运维
- **实时状态流转**（`NodeService.HandleAgentSync` 落库后重读最新节点评估）：
  - `TrafficLimitBytes > 0` 时计算 `usagePct = CalcCycleUsedBytes() * 100 / TrafficLimitBytes`：
    - `>= BreakThresholdPct` 且当前非 `circuit_broken` → 原子切 `desired_state = "circuit_broken"`, `auto_state_reason = "auto_break_quota"`，本次同步响应立即下发 `circuit_broken`，触发即时 TG 熔断告警；
    - `>= DrainThresholdPct` 且当前为 `active` → 原子切 `desired_state = "draining"`, `auto_state_reason = "auto_drain_quota"`（订阅立即摘除该节点），触发即时 TG 摘流告警；
  - 管理员手动置为 `circuit_broken` 或 `draining`（非自动原因）时，自动逻辑不擅自降级覆盖。
- **80% 预警与耗尽预测**（`AlertService.CheckNodeQuotas` + 5 分钟巡检）：
  - 遍历启用且配置了 `TrafficLimitBytes > 0` 的节点，若水位 `>= WarnThresholdPct`（按 `node_alert:{id}:warn:{YYYYMM}` 24h 冷却），结合当日+前一日 `NodeTrafficDaily` 估算近 24h 速率与预计剩余天数，推送 TG HTML 告警。
- **独立账单重置日自动复位**（`NodeService.CheckAndResetMonthlyTraffic`，挂载到 `TrafficSyncJob` 5 分钟巡检）：
  - 按每节点 `BillingResetDay`（兼容 28/29/30/31 短月）与 `LastResetMonth` 防重清零 `Cycle*Bytes`；若 `AutoStateReason` 为配额自动触发（`auto_drain_quota` / `auto_break_quota`），自动恢复 `DesiredState = "active"` 并推送 TG 恢复通知。
- **Telegram Bot 节点运维指令**：
  - `/nodes`：列出全部节点在线状态、当前 `DesiredState`、周期已用/配额与水位百分比；
  - `/nodedrain <id>`：手动将节点切为 `draining`（从订阅摘除）；
  - `/noderestore <id>`：手动将节点恢复为 `active`。

### R3. Agent 本地断网自治硬熔断
- Master `AgentSyncResponse` 与 Agent `SyncResponse` 对称新增 `QuotaPolicy *QuotaPolicy`（`trafficLimitBytes`, `breakThresholdPct`, `countMode`, `multiplier`, `cycleUsedBytes`, `cycleMonth`）。
- Agent 新增 `internal/agent/quota_store.go`（原子写 `spool-dir/quota_state.json`）：
  - Master 同步成功时：刷新本地 `quota_state.json` 基线并清零 `localPendingUsedBytes`；
  - Master 同步失败（失联/超时/5xx）时：将本次采集的网卡增量（若无则用本次 Xray 流量增量）按 `countMode` 与 `multiplier` 累加到 `localPendingUsedBytes` 并持久化；一旦 `cycleUsedBytes + localPendingUsedBytes >= trafficLimitBytes * breakThresholdPct / 100`，立即自主调用 `cfgMgr.ApplyCircuitBreaker(ctx)` 停止业务入站保命，避免面板宕机时单机跑爆流量！
- 严格保持 `cmd/agent` 零重型依赖红线（不含 `gin`/`gorm`/`sqlite`/`telegram`/`web`）。

### R4. 前端节点配额表单与水位进度条
- `NodeFormDrawer.vue` 新增「流量配额与熔断策略」区块：月流量上限 (GB, 0=不限)、账单重置日 (0–31)、计费口径（双向/仅出站/取大值）、安全系数、预警/摘流/熔断阈值 (%)。
- `NodesView.vue` 增加每个节点的周期流量水位进度条（颜色分级 + `autoStateReason` 标签）与「重置周期流量」操作按钮。

## Acceptance Criteria

- [ ] **AC-1**：单元测试覆盖 `Node.CalcCycleUsedBytes`（NIC 优先 / Xray 回退 / `both`/`outbound`/`max` / `multiplier`）、`HandleAgentSync` 跨 90% 自动 `draining` 与跨 95% 自动 `circuit_broken`、`CheckAndResetMonthlyTraffic` 短月份账单日自动清零与恢复，以及 `POST /api/admin/nodes/:id/reset-traffic`。
- [ ] **AC-2**：单元测试覆盖 `AlertService.CheckNodeQuotas`（80% 预警、24h 耗尽预测、24h 冷却去重）与 Telegram Bot `/nodes`、`/nodedrain`、`/noderestore` 指令。
- [ ] **AC-3**：Agent 单元测试与 Master↔Agent 端到端集成测试覆盖：Master 下发 `quotaPolicy` 落盘 `quota_state.json` → 模拟 Master 失联后本地累计流量超 95% 触发 `ApplyCircuitBreaker` 自主断流 → Master 恢复后按最新状态解除/同步。
- [ ] **AC-4**：全量前后端质量门禁全部通过（`go test ./...`、`-race`、`go vet ./...`、`cd web && npm run typecheck && npm run lint && npm run build`、`go build .`、`go build ./cmd/agent`、`go list -deps ./cmd/agent` 0 重型依赖），且未触碰原有脏文件。

## Out of Scope

- 邀请码自助开户与用户自助状态看板（属于 M4）。
- 第三方云厂商 API 自动关机或 DNS 动态切 IP。
