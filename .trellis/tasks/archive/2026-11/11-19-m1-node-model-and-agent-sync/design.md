# M1 Technical Design: Node Model, Master Sync Control Plane & Thin Agent

## 1. Architecture & Slice Isolation

为了支持并行开发与最小耦合，M1 明确拆分为**文件完全互斥的两个模块**：

1. **Master Control Plane (`Slice 1`)**：
   - 领域模型：`internal/domain/node.go`、`internal/domain/inbound.go`（仅追加 `NodeID uint`）、`internal/domain/repository.go`（追加 `NodeRepository` / `NodeTrafficBatchRepository`）
   - 持久化：`internal/adapter/repository/sqlite_db.go`、`internal/adapter/repository/node_repo.go`、`internal/adapter/repository/inbound_repo.go`
   - 编译器扩展：`internal/adapter/xray/compiler.go`（保留原有本机编译逻辑，补充按指定 `[]domain.Inbound` 编译独立直连 `config.json` 的能力）
   - 业务编排：`internal/service/node_service.go`（Node CRUD、Token SHA-256 管理、内存 `sync.RWMutex` 摘要缓存、聚合 `Sync` 用例、安装脚本渲染）；在 `InboundService`/`UserService`/`ConfigService` 关键写入口接入缓存失效钩子，并确保本机编译仅取 `NodeID == 0` 入站
   - HTTP 交付：`internal/delivery/http/middleware_agent.go`、`internal/delivery/http/handler_node.go`、`internal/delivery/http/handler_agent.go`、`internal/delivery/http/router.go`、`main.go`
2. **Standalone Thin Agent (`Slice 2`)**：
   - 独立包：`internal/agent/protocol.go`、`internal/agent/spool.go`、`internal/agent/config_manager.go`、`internal/agent/user_sync.go`、`internal/agent/metrics.go`、`internal/agent/metrics_linux.go`、`internal/agent/metrics_other.go`、`internal/agent/runner.go`
   - 独立入口：`cmd/agent/main.go`
   - **依赖红线**：`cmd/agent` 仅允许依赖 Go 标准库 + `internal/adapter/xray`（或仅轻量 gRPC client + 现有 `domain.Inbound`/`domain.User` 纯结构，严禁拉入 `gin` / `gorm` / `sqlite` / `telegram` / `web`）。`internal/agent/protocol.go` 遵循下文锁定的 HTTP JSON 契约，不依赖 Slice 1 新建文件，保证并行独立编译和测试！

---

## 2. Locked HTTP JSON Contract: `POST /api/agent/sync`

### Authentication
- Header: `Authorization: Bearer <64-hex-node-token>`
- Missing/invalid token -> `401 Unauthorized`
- Node disabled (`Enabled == false`) -> `403 Forbidden`

### Request Body (`application/json`)

```json
{
  "configHash": "sha256-hex-or-empty",
  "usersHash": "sha256-hex-or-empty",
  "agentVersion": "0.1.0",
  "xrayVersion": "25.x.x",
  "lastError": "",
  "hostMetrics": {
    "cpuPercent": 12.5,
    "memPercent": 34.2,
    "nicUpDelta": 1048576,
    "nicDownDelta": 8388608
  },
  "trafficBatch": {
    "batchId": "20261119T101500Z-a1b2c3d4",
    "users": [
      { "email": "alice@example.com", "upBytes": 1024, "downBytes": 20480 }
    ],
    "inbounds": [
      { "tag": "us-direct-vless", "upBytes": 1024, "downBytes": 20480 }
    ]
  }
}
```

> `trafficBatch` 可省略或 `batchId == ""`（无新增流量时免写库）。

### Response Body (`200 OK`, `application/json`)

```json
{
  "ok": true,
  "desiredState": "active",
  "syncIntervalSec": 20,
  "configHash": "sha256-hex",
  "usersHash": "sha256-hex",
  "configUnchanged": false,
  "usersUnchanged": false,
  "configJson": "{ ... full xray json when configUnchanged=false ... }",
  "users": [
    {
      "inboundTag": "us-direct-vless",
      "protocol": "vless",
      "email": "alice@example.com",
      "uuid": "550e8400-e29b-41d4-a716-446655440000",
      "flow": "xtls-rprx-vision"
    }
  ]
}
```

- 当 `req.configHash == latestConfigHash`：`configUnchanged: true`，`configJson` 省略（`omitempty`）。
- 当 `req.usersHash == latestUsersHash`：`usersUnchanged: true`，`users` 省略（`omitempty`）。
- 当 `desiredState == "circuit_broken"`：Agent 停用业务入站（保留管理进程与定时心跳），待 Master 恢复 `desiredState == "active"` 后重新加载配置与用户。

---

## 3. Data Model & SQLite Transaction Design

### 3.1 `domain.Node` (`nodes`)
```go
const (
    NodeStateActive        = "active"
    NodeStateDraining      = "draining"
    NodeStateCircuitBroken = "circuit_broken"
)

type Node struct {
    ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
    Name           string     `gorm:"type:varchar(128);not null" json:"name"`
    Host           string     `gorm:"type:varchar(255);not null" json:"host"`
    Region         string     `gorm:"type:varchar(64);default:''" json:"region"`
    TokenHash      string     `gorm:"type:varchar(64);uniqueIndex;not null" json:"-"`
    TokenPrefix    string     `gorm:"type:varchar(16);default:''" json:"tokenPrefix"`
    Enabled        bool       `gorm:"default:true" json:"enabled"`
    DesiredState   string     `gorm:"type:varchar(32);default:'active'" json:"desiredState"`
    LastSeenAt     *time.Time `json:"lastSeenAt"`
    AgentVersion   string     `gorm:"type:varchar(64);default:''" json:"agentVersion"`
    XrayVersion    string     `gorm:"type:varchar(64);default:''" json:"xrayVersion"`
    LastConfigHash string     `gorm:"type:varchar(64);default:''" json:"lastConfigHash"`
    LastUsersHash  string     `gorm:"type:varchar(64);default:''" json:"lastUsersHash"`
    LastSyncError  string     `gorm:"type:text;default:''" json:"lastSyncError"`
    CPUPercent     float64    `gorm:"default:0" json:"cpuPercent"`
    MemPercent     float64    `gorm:"default:0" json:"memPercent"`
    NicUpBytes     int64      `gorm:"default:0" json:"nicUpBytes"`
    NicDownBytes   int64      `gorm:"default:0" json:"nicDownBytes"`
    XrayUpBytes    int64      `gorm:"default:0" json:"xrayUpBytes"`
    XrayDownBytes  int64      `gorm:"default:0" json:"xrayDownBytes"`
    Remark         string     `gorm:"type:varchar(255);default:''" json:"remark"`
    CreatedAt      time.Time  `json:"createdAt"`
    UpdatedAt      time.Time  `json:"updatedAt"`

    Online bool `gorm:"-" json:"online"` // 动态判定: LastSeenAt 距今 <= 90s
}
```

### 3.2 `domain.NodeTrafficBatch` (`node_traffic_batches`)
```go
type NodeTrafficBatch struct {
    ID        uint      `gorm:"primaryKey;autoIncrement"`
    NodeID    uint      `gorm:"uniqueIndex:idx_node_batch;not null"`
    BatchID   string    `gorm:"type:varchar(96);uniqueIndex:idx_node_batch;not null"`
    CreatedAt time.Time `gorm:"index"`
}
```

### 3.3 SQLite Single-Writer Safety
- `SyncTrafficBatch` 在**单个 `db.Transaction`**内完成：
  1. 尝试插入 `NodeTrafficBatch{NodeID, BatchID}`；若触发唯一冲突（已处理），直接 `return nil`（幂等跳过）；
  2. 累加 `User.UpBytes / DownBytes` 与当日 `TrafficLog`；
  3. 累加 `Inbound.UpBytes / DownBytes`（校验 `inbound.NodeID == nodeID` 防止越权污染其他节点统计）；
  4. 累加 `Node.XrayUpBytes / XrayDownBytes`。
- 节点摘要缓存 `nodeSyncCache`（`sync.RWMutex` 保护的 `map[uint]*cachedNodeSnapshot`）：
  - 在节点入站增删改或用户增删改/启停/过期踢人时调用 `InvalidateAll()` 或 `InvalidateNode(nodeID)`；
  - 每次心跳先命中内存缓存比对 `ConfigHash`/`UsersHash`，无变化时不重新编译 `config.json`、不查询全表用户。

---

## 4. Thin Agent Execution & Failure Recovery

1. **Config Apply with Safe Rollback (`internal/agent/config_manager.go`)**:
   - 依赖注入接口抽象外部命令（`CommandRunner`）与 gRPC 探活（`HealthProber`），保证在 Windows/Linux 均可通过单元测试 100% 覆盖：
     1. 写 `<configPath>.tmp`（`0600`）；
     2. 跑 `xray run -test -c <configPath>.tmp`；失败则删 `.tmp` 并返回错误；
     3. 若 `<configPath>` 存在，备份到 `<configPath>.bak`；
     4. 原子 `os.Rename(tmp, configPath)`；
     5. 调用 `reload`（如 `systemctl reload-or-restart <service>`）；
     6. 3s 超时窗口内轮询本机 gRPC 端口探活；若失败则把 `.bak` 还原回 `<configPath>` 并再次 `restart` 回滚，返回详细错误给 Master。
2. **Traffic Spool (`internal/agent/spool.go`)**:
   - 本地文件 `<spoolDir>/pending_batch.json`；
   - 心跳前若已有未 ack 的 `pending_batch.json`，优先携带该批次（含原 `batchId`）重试，不覆盖、不丢失；
   - 若无待发批次，调本机 gRPC `QueryTrafficStats(reset=true)`，若存在非零流量则生成唯一 `batchId` 原子落盘 `pending_batch.json` 再上报；
   - 仅当 Master 返回 HTTP 200 且 `ok: true` 时删除 `pending_batch.json`。
3. **Differential User Sync (`internal/agent/user_sync.go`)**:
   - 内存中以 `key = inboundTag + "\x00" + email` 维护 `appliedUsers`；
   - 收到新 `users` 列表后计算 `toAdd`、`toRemove`、`toUpdate`（`uuid`/`flow` 变更时先删后增）；
   - 仅当刚执行完配置整机重载时重置 `appliedUsers` 基线，平时 0 重启、0 断流。
4. **Circuit Breaker Primitive (`desiredState`)**:
   - 收到 `desiredState == "circuit_broken"` 时，调用配置管理器应用最小“仅保留 `api` 入站、无业务入站”的静音配置（或暂停 Xray 业务服务），记录当前熔断态；
   - 当 Master 恢复 `desiredState == "active"` 且下发正常 `configJson` 时自动恢复业务入站。

---

## 5. Compatibility & Rollback

- **零破坏兼容**：现有表仅 `inbounds` 增加 `node_id`（`default:0`），新增 `nodes` 和 `node_traffic_batches` 两张表；`ConfigService` 编译本机配置与 `TrafficSyncJob` 轮询本机入站只作用于 `NodeID == 0`。
- **回滚策略**：新增 API 与 `cmd/agent` 均为增量能力，不改变任何既有单机路由与前端交互，随时可回退单一提交。
