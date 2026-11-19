# M3 (`11-19-m3-node-quota-alert-and-circuit-breaker`) 架构调研报告

> 只读调研结论，所有路径与行号均基于当前工作区实际源码核验。

---

## 1. Node 模型与同步控制面现状

### 1.1 现有源码锚点
- **Node 领域模型与 DTO**：`internal/domain/node.go`
  - 状态常量与校验：`NodeStateActive` / `NodeStateDraining` / `NodeStateCircuitBroken`、`IsValidNodeState` (`L8-L24`)
  - `Node` 结构体：`L27-L53`，已包含累计流量字段 `NicUpBytes`/`NicDownBytes`/`XrayUpBytes`/`XrayDownBytes` (`L44-L47`) 与动态 `Online` (`L52`, `RefreshOnline` `L56-L64`)
  - 管理端 DTO：`CreateNodeDTO` (`L75-L82`)、`UpdateNodeDTO` (`L91-L98`)
  - Agent 同步契约：`AgentHostMetrics` (`L108-L113`)、`AgentTrafficBatch` (`L130-L134`)、`AgentSyncRequest` (`L137-L145`)、`AgentSyncResponse` (`L157-L167`)
- **Node 仓储接口与 GORM 实现**：
  - 接口：`internal/domain/repository.go:113-128` (`NodeRepository` & `NodeTrafficBatchRepository`)
  - 数据库自动迁移：`internal/adapter/repository/sqlite_db.go:42-53` (`db.AutoMigrate(...)`)
  - `GORMNodeRepository.Update`：`internal/adapter/repository/node_repo.go:33-54`（使用显式 `Updates(map[string]interface{}{...})` 白名单，避免覆盖心跳/流量列）
  - `GORMNodeRepository.UpdateHeartbeat`：`internal/adapter/repository/node_repo.go:57-85`（原子累加 `nic_up_bytes`、`nic_down_bytes`）
  - `GORMNodeRepository.ApplyNodeTrafficBatch`：`internal/adapter/repository/node_repo.go:135-274`（事务内幂等去重 `(node_id, batch_id)` -> 累加用户 `up_bytes`/`down_bytes` 与 `TrafficLog` -> 累加 `inbound` -> 累加 `node.xray_up_bytes`/`xray_down_bytes` `L256-L266`）
- **Node 业务服务与 HTTP 路由**：
  - `NodeService`：`internal/service/node_service.go:46-58`；`CreateNode` (`L142-L189`)、`UpdateNode` (`L191-L235`)、`HandleAgentSync` (`L366-L436`)
  - `NodeHandler`：`internal/delivery/http/handler_node.go:70-119` (`Create`/`Update`)
  - 路由：`internal/delivery/http/router.go:184-195` (`/api/admin/nodes`)、`L86-L92` (`/api/agent/sync`)

### 1.2 建议 Schema 与增量落库设计
1. **`domain.Node` 新增配额与周期字段**（`internal/domain/node.go:27-53`，并由 `sqlite_db.go:42-53` 自动迁移）：
   - 配额配置：`TrafficLimitBytes int64`（0 表示不限）、`BillingResetDay int`（`0` 不自动重置，`1..31` 参考 `User.ResetDay` 短月份自适应）、`LastResetMonth int`（`YYYYMM`）、`TrafficCountMode string`（`both` | `outbound` | `max`，默认 `both` 或 `outbound`）、`TrafficMultiplier float64`（默认 `1.0`）、`WarnThresholdPct int`（默认 `80`）、`DrainThresholdPct int`（默认 `90`）、`BreakThresholdPct int`（默认 `95`）。
   - 周期计数与状态归因：`CycleXrayUpBytes int64`、`CycleXrayDownBytes int64`、`CycleNicUpBytes int64`、`CycleNicDownBytes int64`、`AutoStateReason string`（记录自动排空/熔断/恢复原因，便于前端与 TG 展示）。
   - 同步修正 `CreateNodeDTO` / `UpdateNodeDTO`、`NodeService.CreateNode` / `UpdateNode` 及 `GORMNodeRepository.Update`（`node_repo.go:36-46` 白名单），注意 `Update` 不可覆盖 `cycle_*` 与 `last_reset_month`（重置操作应通过专用方法 `ResetCycleTraffic`）。
2. **按天节点流量日志 `NodeTrafficDaily`**（用于近 24h 耗尽预测与历史趋势）：
   - 参考 `domain.TrafficLog`（`internal/domain/traffic.go:20-28`）与 `trafficLogOnConflictClause()`（`internal/adapter/repository/traffic_log_repo.go:21-30`），新增 `NodeTrafficDaily` 模型（`node_id`, `date` `YYYY-MM-DD` 建唯一联合索引 `idx_node_date`, `xray_up_bytes`, `xray_down_bytes`, `nic_up_bytes`, `nic_down_bytes`）。
   - **增量落库位置**：
     - Xray 流量增量：在 `GORMNodeRepository.ApplyNodeTrafficBatch` 事务步骤 3（`node_repo.go:256-266`）同步累加 `cycle_xray_up_bytes`/`cycle_xray_down_bytes` 并 `OnConflict(node_id, date) DoUpdates` 写入 `node_traffic_dailies`。
     - NIC 流量增量：在 `GORMNodeRepository.UpdateHeartbeat`（`node_repo.go:67-76`）同步累加 `cycle_nic_up_bytes`/`cycle_nic_down_bytes`；若 `NicUpDelta > 0 || NicDownDelta > 0`，同一方法内 upsert 当日 `node_traffic_dailies` 的 `nic_up_bytes`/`nic_down_bytes`。

### 1.3 `HandleAgentSync` 实时水位评估与状态流转接线点
- 在 `internal/service/node_service.go:379-415`：
  1. `ApplyNodeTrafficBatch`（`L381`）与 `UpdateHeartbeat`（`L396`）执行完成后，**必须重新读取最新 `node`**（或由 repo 返回更新后的周期计数），因为内存中的 `node`（`L371` 读取）尚未包含本次批次的 `CycleXray*` 和 `CycleNic*` 增量。
  2. 基于 `node.CalcCycleUsedBytes()`（按 `TrafficCountMode` 取 `nic` 上下行组合，再乘以 `TrafficMultiplier`，若未采集到 NIC 则可回退或按策略取 Xray）与 `TrafficLimitBytes` 计算 `usageRatio`：
     - 若 `ratio >= BreakThresholdPct/100` 且 `node.DesiredState != NodeStateCircuitBroken`：原子更新 `desired_state = 'circuit_broken'`、`auto_state_reason`，使本次响应 `resp.DesiredState` (`L421`) 立即下发 `circuit_broken`；同时调用 `AlertService` 异步/即时通知，并使订阅缓存（若有）感知节点摘除。
     - Else if `ratio >= DrainThresholdPct/100` 且 `node.DesiredState == NodeStateActive`：原子更新 `desired_state = 'draining'`、`auto_state_reason`，响应立即返回 `draining`（保留存量连接但停止新订阅分发）。
     - 若管理员已手动设为 `circuit_broken`，水位评估不得自动将其升回 `draining`/`active`。

---

## 2. Agent 侧本地自治熔断接线点

### 2.1 现有源码锚点
- **通信协议**：`internal/agent/protocol.go:50-61` (`SyncResponse`)、`L4-L8` (`DesiredState*` 常量)
- **磁盘持久化 Spool**：`internal/agent/spool.go:15-144`（使用 `pending_batch.json` + `.tmp` write-fsync-rename 原子落盘模式 `L93-L118`）
- **配置与熔断执行器**：`internal/agent/config_manager.go:148-156` (`ApplyCircuitBreaker` 已实现：生成仅保留本地 gRPC API 与 freedom 出站的静默配置并调用 `Apply` 热重载 + gRPC 探活)
- **主循环 Runner**：`internal/agent/runner.go:21-37` (`Runner` 结构体)、`L114-L229` (`SyncOnce`)

### 2.2 轻量 `quotaPolicy` 下发、本地持久化与断网自治熔断方案
1. **协议扩展**：
   - Master `domain.AgentSyncResponse`（`internal/domain/node.go:157-167`）与 Agent `agent.SyncResponse`（`internal/agent/protocol.go:51-61`）对称新增 `QuotaPolicy *AgentQuotaPolicy` (`json:"quotaPolicy,omitempty"`)，包含：`TrafficLimitBytes`, `BreakThresholdPct`, `CountMode`, `Multiplier`, `CycleUsedBytes`, `CycleMonth` (`YYYYMM`)。
2. **`spool-dir` 本地状态持久化（`quota_state.json`）**：
   - 复用 `internal/agent/spool.go` 的原子写文件模式（`write-fsync-rename`），新增 `QuotaStateStore`（或集成在 `Spool` 同目录下 `filepath.Join(dir, "quota_state.json")`），保存最近一次 Master 下发的 `QuotaPolicy` 基线、`LocalPendingUsedBytes`（断网期间本地累加字节数）及 `LocalCircuitBroken bool`。
   - **Master 可达时**（`runner.go:149-166`）：`Sync` 成功后，用 `resp.QuotaPolicy` 刷新本地 `quota_state.json` 基线并清零 `LocalPendingUsedBytes`（若 `DesiredState != circuit_broken` 且 `CycleMonth` 更新或限额调高，可解除 `LocalCircuitBroken`）。
   - **Master 不可达时**（`runner.go:149-153` `client.Sync` 返回 `err != nil`）：
     - 在 `SyncOnce` 失败分支（`L150-L153`）中，利用本次采集到的 `hostMetrics.NicUpDelta`/`NicDownDelta`（以及未确认的 `pending` 流量）按 `QuotaPolicy.CountMode` 与 `Multiplier` 累加至 `LocalPendingUsedBytes` 并原子落盘 `quota_state.json`。
     - 当 `TrafficLimitBytes > 0` 且 `CycleUsedBytes + LocalPendingUsedBytes >= TrafficLimitBytes * BreakThresholdPct / 100` 时，即使 Master 失联，Agent 直接调用 `r.cfgMgr.ApplyCircuitBreaker(ctx)`（`config_manager.go:150-156`）并设置 `r.circuitBroken = true` & `LocalCircuitBroken = true`，实现断网保命。

---

## 3. 告警服务、定时巡检与 Telegram Bot 现状

### 3.1 现有源码锚点
- **告警服务**：`internal/service/alert_service.go`
  - `AlertService` 结构体与 24h TTL 内存冷却缓存：`L29-L45` (`cache.New[bool](24*time.Hour, 1*time.Hour)`)
  - 80% 流量告警模式：`CheckTrafficQuotas` (`L47-L81`，`cacheKey := fmt.Sprintf("traffic_alert:%s", u.Email)`，冷却 24h)
  - 直接发送 HTML 消息模式：`NotifyRealityAbnormal` (`L168-L214`，调用 `s.notifier.SendMessage(ctx, text)`)
- **TG 消息格式化**：`internal/adapter/telegram/notifier.go:120-167`（`SendMessage` 使用 `ParseMode = "HTML"`；`SendTrafficAlert` `L140-L152`）
- **5 分钟巡检定时任务**：`internal/delivery/cron/traffic_sync.go:66-108`
  - `alertTicker := time.NewTicker(5 * time.Minute)` (`L66`)
  - `case <-alertTicker.C:` (`L89-L106`) 依次调用 `alertSvc.CheckTrafficQuotas`、`CheckSystemLoad`、`CheckCertificates` 与 `userSvc.CheckAndResetMonthlyTraffic`（`user_service.go:925-984`）。
- **Telegram Bot 命令分发与装配**：
  - `BotHandler`：`internal/adapter/telegram/bot.go:41-72`
  - 云端命令注册：`registerBotCommands` (`bot.go:74-93`)
  - 命令路由：`handleMessage` (`bot.go:299-480`)
  - 依赖装配：`main.go:128-168`（`nodeSvc` `L128`、`botHandler` `L154`、`alertSvc` `L157`、`syncJob` `L198`）

### 3.2 接线建议
1. **告警与耗尽预测**：
   - 在 `AlertService` 注入 `nodeRepo`（及可选 `nodeTrafficDailyRepo`），新增 `CheckNodeQuotas(ctx)`：检查节点当前周期水位，对 `>= WarnThresholdPct`（默认 80%）或结合近 24h 速率预测 `< 24h` 耗尽的节点发送 TG 告警（按 `node_alert:{nodeID}:{level}:{cycleMonth}` 24h 去重）。
   - 在 `NodeService.HandleAgentSync` 触发自动 `draining` / `circuit_broken` 状态切换时，也可通过回调/注入 `AlertService` 即时推送状态变更通知（复用 `s.notifier.SendMessage`）。
2. **5 分钟巡检钩子**：
   - 在 `TrafficSyncJob` 注入 `nodeSvc`（或 `NodeQuotaCron` 接口），于 `traffic_sync.go:89-106` 的 `alertTicker.C` 分支挂载 `nodeSvc.CheckAndResetMonthlyTraffic(ctx)`（参考 `user_service.go:925-984` 的 `effectiveResetDay` 与 `LastResetMonth` 逻辑，重置 `Cycle*Bytes` 并在 `DesiredState` 为因配额自动触发的 `draining`/`circuit_broken` 时恢复为 `active`）与 `alertSvc.CheckNodeQuotas(ctx)`。
3. **Telegram Bot 命令扩展**：
   - 在 `BotHandler`（`bot.go:41-72`）注入最小 `NodeController` 接口（`ListNodes`, `UpdateNodeState` / `DrainNode`, `RestoreNode`），并在 `main.go:154` 传入 `nodeSvc`。
   - 在 `registerBotCommands`（`bot.go:78-87`）与 `handleMessage`（`bot.go:299`）新增：
     - `/nodes`：展示所有节点在线状态、当前 `DesiredState`、周期已用流量/配额及水位百分比；
     - `/nodedrain <id>`：将节点 `DesiredState` 设为 `draining`；
     - `/noderestore <id>`：将节点 `DesiredState` 恢复为 `active`。

---

## 4. 前端节点配额表单与水位展示接线点

> **注意**：当前工作区存在未提交变更（如 `web/dist/*`, `web/src/style.css`, `web/src/views/PortalClaimView.vue`, `web/src/views/users/components/UserShareModal.vue`, `deploy/*`），M3 实施时**严禁触碰这些原有脏文件**。

### 4.1 现有源码锚点 & 最小改动范围
1. **`web/src/api/nodes.ts`**：
   - `NodeItem` (`L5-L29`)、`CreateNodePayload` (`L31-L38`)、`UpdateNodePayload` (`L44-L51`) 补充配额配置与周期字段（`trafficLimitBytes`, `billingResetDay`, `trafficCountMode`, `trafficMultiplier`, `warnThresholdPct`, `drainThresholdPct`, `breakThresholdPct`, `cycleXrayUpBytes`, `cycleXrayDownBytes`, `cycleNicUpBytes`, `cycleNicDownBytes`, `autoStateReason`）。
   - 新增 `resetNodeTraffic(id: number)` 对应后端 `POST /api/admin/nodes/:id/reset-traffic`（若提供手动重置接口）。
2. **`web/src/views/nodes/types.ts`**：
   - `NodeFormData` (`L5-L13`) 与 `getDefaultNodeFormData()` (`L45-L55`) 增加表单友好字段（如 `trafficLimitGB: number`、`billingResetDay: number`、`trafficCountMode: 'both' | 'outbound' | 'max'`、`trafficMultiplier: number`、阈值百分比等）及 `TRAFFIC_COUNT_MODE_OPTIONS`。
3. **`web/src/views/nodes/components/NodeFormDrawer.vue`**：
   - 在 `SectionCard` ② 之后新增 `SectionCard` ③「流量配额与熔断策略」（`L115` 之后），绑定月流量上限 (GB，0=不限)、账单重置日 (0-31)、计费口径 (`both`/`outbound`/`max`)、安全系数 (`trafficMultiplier`) 与告警/排空/熔断阈值；
   - 在 `initForm` (`L160-L174`) 与 `submitForm` (`L186-L230`) 完成 `GB <-> Bytes` 换算与提交。
4. **`web/src/views/NodesView.vue`**：
   - Desktop 表格流量列（`L231-L241`）与 Mobile 卡片流量区（`L369-L375`）增加**周期水位进度条**（已用 / 上限 GB、百分比颜色分级：`>=90%` 琥珀/红、`>=80%` 黄、正常绿）及 `autoStateReason` 提示；
   - 操作列（`L257-L300` 及 Mobile `L386-L422`）增加「重置周期流量」按钮（调用 `resetNodeTraffic` 并刷新列表）。
