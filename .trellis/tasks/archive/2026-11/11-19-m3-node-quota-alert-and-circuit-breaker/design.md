# M3 Technical Design: Node Monthly Quota Dual-Accounting, Alerts & Auto Circuit Breaker

## 1. Parallel Slice Architecture (3 File-Disjoint Slices)

1. **Slice 1: Master Quota Engine, Alerts, Cron & TG Bot (`implementer-master-quota`)**
   - `internal/domain/node.go`, `internal/domain/repository.go`
   - `internal/adapter/repository/sqlite_db.go`, `internal/adapter/repository/node_repo.go`, `internal/adapter/repository/node_repo_test.go`
   - `internal/service/node_service.go`, `internal/service/node_service_test.go`
   - `internal/service/alert_service.go`, `internal/service/alert_service_test.go`
   - `internal/delivery/http/handler_node.go`, `internal/delivery/http/router.go`, `internal/delivery/http/handler_agent_test.go`
   - `internal/delivery/cron/traffic_sync.go`
   - `internal/adapter/telegram/bot.go`, `internal/adapter/telegram/bot_test.go`
   - `main.go`
2. **Slice 2: Agent Local Autonomous Circuit Breaker (`implementer-agent-quota`)**
   - `internal/agent/protocol.go`（按 §2 锁定 JSON 契约新增 `QuotaPolicy`）
   - `internal/agent/quota_store.go`, `internal/agent/quota_store_test.go`（新建：`spool-dir/quota_state.json` 原子持久化与本地配额累加）
   - `internal/agent/runner.go`, `internal/agent/runner_test.go`（在线刷新基线 + 断网本地累加超阈值调用 `ApplyCircuitBreaker`）
   - `cmd/agent/main.go`（仅确保 `QuotaStore` 注入到 `Runner`）
3. **Slice 3: Frontend Quota Config & Watermark UI (`implementer-ui-quota`)**
   - `web/src/api/nodes.ts`
   - `web/src/views/nodes/types.ts`
   - `web/src/views/nodes/composables/useNodes.ts`
   - `web/src/views/nodes/components/NodeFormDrawer.vue`
   - `web/src/views/NodesView.vue`
   - **严禁触碰**原有脏文件，**严禁提交 `web/dist/**`**。

---

## 2. Locked Cross-Layer Contracts

### 2.1 `POST /api/agent/sync` Response Addition (`quotaPolicy`)

Master (`internal/domain/node.go` `AgentSyncResponse`) 与 Agent (`internal/agent/protocol.go` `SyncResponse`) 均新增可选字段：

```json
{
  "ok": true,
  "desiredState": "active",
  "syncIntervalSec": 20,
  "configHash": "...",
  "usersHash": "...",
  "configUnchanged": true,
  "usersUnchanged": true,
  "quotaPolicy": {
    "trafficLimitBytes": 1073741824000,
    "breakThresholdPct": 95,
    "countMode": "both",
    "multiplier": 1.05,
    "cycleUsedBytes": 536870912000,
    "cycleMonth": 202611
  }
}
```

- 当 `TrafficLimitBytes <= 0` 时，Master 可下发 `quotaPolicy` 带 `trafficLimitBytes: 0`（Agent 据此清除本地硬限额限制，确保管理员取消限额后 Agent 同步放行）。

### 2.2 Admin Node JSON Fields Addition (`web/src/api/nodes.ts` ↔ `internal/domain/node.go`)

`Node` / `CreateNodeDTO` / `UpdateNodeDTO` 新增 JSON 字段名（严格 camelCase）：
- `trafficLimitBytes` (`int64`, default `0`)
- `billingResetDay` (`int`, `0..31`, default `0`)
- `lastResetMonth` (`int`, `YYYYMM`, read-only in Node response)
- `trafficCountMode` (`"both" | "outbound" | "max"`, default `"both"`)
- `trafficMultiplier` (`float64`, default `1.0`)
- `warnThresholdPct` (`int`, default `80`)
- `drainThresholdPct` (`int`, default `90`)
- `breakThresholdPct` (`int`, default `95`)
- `cycleXrayUpBytes`, `cycleXrayDownBytes`, `cycleNicUpBytes`, `cycleNicDownBytes` (`int64`, read-only)
- `cycleUsedBytes` (`int64`, `gorm:"-"` 计算字段，由 `RefreshComputed()` / `RefreshOnline()` 填充，前端直接展示无需重复实现计费逻辑！)
- `autoStateReason` (`string`, read-only)

### 2.3 Manual Reset Endpoint
- `POST /api/admin/nodes/:id/reset-traffic`（JWT 保护）
- 响应 `200 OK`：返回重置后的 `Node` 对象（`Cycle*Bytes = 0`，若 `AutoStateReason` 属于配额自动原因则自动恢复 `DesiredState = "active"`, `AutoStateReason = ""`）。

---

## 3. Core Algorithms & Invariants

### 3.1 Dual-Accounting Formula (`domain.CalcTrafficByMode`)
```go
const (
    TrafficCountModeBoth     = "both"
    TrafficCountModeOutbound = "outbound"
    TrafficCountModeMax      = "max"

    AutoReasonDrainQuota = "auto_drain_quota"
    AutoReasonBreakQuota = "auto_break_quota"
)

func CalcTrafficByMode(up, down int64, mode string, multiplier float64) int64 {
    if up < 0 { up = 0 }
    if down < 0 { down = 0 }
    var base int64
    switch mode {
    case TrafficCountModeOutbound:
        base = up
    case TrafficCountModeMax:
        if up >= down { base = up } else { base = down }
    default:
        base = up + down
    }
    if multiplier <= 0 {
        multiplier = 1.0
    }
    return int64(math.Round(float64(base) * multiplier))
}
```
- `Node.CalcCycleUsedBytes()`：若 `CycleNicUpBytes > 0 || CycleNicDownBytes > 0`，使用 `(CycleNicUpBytes, CycleNicDownBytes)`，否则使用 `(CycleXrayUpBytes, CycleXrayDownBytes)`。
- 阈值校验：`WarnThresholdPct` 默认 80、`DrainThresholdPct` 默认 90、`BreakThresholdPct` 默认 95；范围 `1..100`，创建/更新时校验 `0 < warn <= drain <= break <= 100`（未传使用默认值）。

### 3.2 `NodeTrafficDaily` & 24h Depletion Forecast
```go
type NodeTrafficDaily struct {
    ID            uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    NodeID        uint      `gorm:"uniqueIndex:idx_node_daily;not null" json:"nodeId"`
    Date          string    `gorm:"type:varchar(10);uniqueIndex:idx_node_daily;not null" json:"date"` // YYYY-MM-DD
    XrayUpBytes   int64     `gorm:"default:0" json:"xrayUpBytes"`
    XrayDownBytes int64     `gorm:"default:0" json:"xrayDownBytes"`
    NicUpBytes    int64     `gorm:"default:0" json:"nicUpBytes"`
    NicDownBytes  int64     `gorm:"default:0" json:"nicDownBytes"`
    UpdatedAt     time.Time `json:"updatedAt"`
}
```
- `AlertService.CheckNodeQuotas` 取昨日与今日两日日志估算日均消耗速率 `dailyRate`（优先按今日已过小时外推与昨日均值平滑），若 `dailyRate > 0` 则计算 `daysLeft = remaining / dailyRate` 附在 80% 预警文案中。

### 3.3 Agent Autonomous Breaker (`internal/agent/quota_store.go`)
- 本地持久化结构体：
  ```go
  type LocalQuotaState struct {
      Policy                QuotaPolicy `json:"policy"`
      LocalPendingUsedBytes int64       `json:"localPendingUsedBytes"`
      LocalCircuitBroken    bool        `json:"localCircuitBroken"`
      UpdatedAt             time.Time   `json:"updatedAt"`
  }
  ```
- 当 `Runner.SyncOnce` 调用 `client.Sync` 返回 error：
  1. 提取本轮流量增量：优先 `(hostMetrics.NicUpDelta, hostMetrics.NicDownDelta)`，若均为 0 且有本轮新采集的 Xray 增量则回退使用 Xray 增量；
  2. 调用 `quotaStore.RecordOfflineUsage(up, down)` 累加 `LocalPendingUsedBytes` 并原子写盘；
  3. 检查 `ShouldBreak()`：`Policy.TrafficLimitBytes > 0 && (Policy.CycleUsedBytes + LocalPendingUsedBytes) * 100 >= Policy.TrafficLimitBytes * int64(Policy.BreakThresholdPct)`；
  4. 满足且尚未 `r.circuitBroken` → 立即执行 `r.cfgMgr.ApplyCircuitBreaker(ctx)` 并标记 `LocalCircuitBroken = true`。
- 当 Master 重新连通返回 `200 OK`：
  - 更新本地 `Policy = *resp.QuotaPolicy`、`LocalPendingUsedBytes = 0`；
  - 若 `resp.DesiredState != "circuit_broken"` 则清除 `LocalCircuitBroken = false`，并由 `Runner` 按 Master 下发的 `configJson` 自动恢复业务入站。
