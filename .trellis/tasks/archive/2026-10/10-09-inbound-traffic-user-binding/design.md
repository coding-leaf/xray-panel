# Technical Design: Inbound Traffic User Binding

## 1. Architecture & Boundaries

本方案采用轻量、无侵入的“控制面入站-用户流量映射”设计。保持 Xray 内核与 gRPC 交互完全解耦，所有映射与计费累加全部在面板服务层与定时同步引擎内闭环。

```
+-------------------------------------------------------------+
| Xray Core (Data Plane)                                      |
|   Inbound: [tag: "local", socks/http, auth: noauth]          |
|   Stats: inbound>>>local>>>traffic>>>uplink/downlink        |
+-------------------------------------------------------------+
                             | gRPC StatsService.QueryStats
                             v
+-------------------------------------------------------------+
| Xray Panel - TrafficSyncJob (Engine)                        |
|   1. 遍历 Inbound traffic stats: tag = "local"              |
|   2. 查询本地 Inbound 缓存/映射表 -> BoundUserEmail:         |
|      "local@yezineko.top"                                   |
|   3. userDeltas["local@yezineko.top"] += delta              |
|   4. inboundDeltas["local"] += delta                        |
+-------------------------------------------------------------+
                             | 单原子事务 BatchSyncTraffic
                             v
+-------------------------------------------------------------+
| SQLite DB (Persistence)                                     |
|   - 更新 Inbound.up_bytes, Inbound.down_bytes               |
|   - 更新 User.up_bytes, User.down_bytes                     |
|   - 写入/累加 TrafficLog (按日期记录用户流量日志)               |
+-------------------------------------------------------------+
```

## 2. Data Contract & Model Changes

### 2.1 Backend Domain (`internal/domain/inbound.go`)
```go
type Inbound struct {
    // ... 原有字段
    BoundUserEmail string `gorm:"column:bound_user_email;type:varchar(128);default:''" json:"boundUserEmail"` // 流量归属用户 Email (空代表无单租户绑定)
    // ...
}
```

### 2.2 Inbound Repository & Cache
为避免 `TrafficSyncJob` 每 5~15 秒一轮轮询都对数据库执行全量 Inbound 查询，在 `TrafficSyncJob` 内部或 service 维护简单的 tag -> `BoundUserEmail` 映射，或直接通过 `j.inboundRepo.ListAll(ctx)` 获取入站映射（通常只有不到 10 个入站，耗时 < 0.1ms）。

### 2.3 TrafficSyncJob Accounting Logic
在 `TrafficSyncJob.syncOnce` 中：
1. 先建立 `inboundTagToUser` 映射：`map[string]string`。
2. 处理 `TrafficStatTypeInbound` 时：
   ```go
   if s.Type == domain.TrafficStatTypeInbound {
       // 1. 记入站自身吞吐
       inboundDeltas[s.Tag] += s.Value

       // 2. 若存在绑定用户，直接合并进该用户的 userDeltas 中
       if boundEmail, ok := inboundTagToUser[s.Tag]; ok && boundEmail != "" {
           d := getOrCreateUserDelta(boundEmail)
           if s.IsUplink {
               d.up += s.Value
               userDeltaUp[boundEmail] += s.Value
           } else {
               d.down += s.Value
               userDeltaDown[boundEmail] += s.Value
           }
       }
   }
   ```
3. 随后交付现有的 `batchRepo.BatchSyncTraffic(writeCtx, uDeltas, inDeltas, today)`。
4. 原有的用户活跃状态检查（超量、过期即踢出 Xray）、Telegram 预警告警和实时速率追踪逻辑自然继承，无需二次改造！

### 2.4 Inbound Config Validation
在 `ConfigService.CreateInbound` 和 `ConfigService.UpdateInbound` 时：
- 如果 `BoundUserEmail` 非空，验证 `userRepo.GetByEmail(ctx, in.BoundUserEmail)` 是否存在。若不存在，返回清晰错误 `fmt.Errorf("bound user '%s' does not exist", in.BoundUserEmail)`。

### 2.5 Web UI
- `InboundItem` & `InboundFormData` 增加 `boundUserEmail: string`。
- 在 InboundFormDrawer 中，针对 SOCKS、HTTP、Dokodemo-door 以及所有非多用户协议入站，提供“流量归属用户 (可选)”下拉选择框。
- 在 InboundTable / InboundDetailDrawer 展示已绑定的用户徽标。

## 3. Trade-offs & Rollback Strategy

- **兼容性**：字段默认为空字符串 `""`，现有所有 VLESS / Reality / Trojan 入站行为 100% 保持现状。
- **回滚成本**：若需禁用此特性，只需将入站的 `bound_user_email` 置空，即自动退化为原有行为；无破坏性 Schema 变更。
