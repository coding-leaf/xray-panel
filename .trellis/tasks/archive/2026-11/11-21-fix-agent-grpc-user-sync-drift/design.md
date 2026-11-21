# Design: 唯一权威配置源与 `xray-agent` 零断流落盘及运行态自愈架构

> Task: `11-21-fix-agent-grpc-user-sync-drift`  
> Reference Audit: [codebase-audit.md](./research/codebase-audit.md)

---

## 1. Architecture & Single Authoritative Source of Truth (SSOT)

```
┌──────────────────────────────────────────────────────────────────────┐
│  Control Plane SSOT (唯一权威配置源)                                    │
│  Master SQLite ──► XrayCompiler ──► cachedNodeSnapshot               │
│    • ConfigJSON (structural + static clients + active DB users)      │
│    • FullConfigHash = sha256(ConfigJSON)                             │
│    • ConfigHash     = sha256(structuralBytes) [reload trigger]       │
│    • Users[]        + UsersHash = sha256(json(Users))                │
└───────────────────────────────┬──────────────────────────────────────┘
                                │ POST /api/agent/sync
                                │ Req: {configHash, usersHash, diskConfigHash}
                                │ Resp: {configUnchanged, usersUnchanged, configJson?, users?}
                                ▼
┌──────────────────────────────────────────────────────────────────────┐
│  Remote Node Convergence Loop (xray-agent Runner)                    │
│                                                                      │
│  1. Disk SSOT Mirror                     2. Local State Cache        │
│     /usr/local/etc/xray/config.json         {spoolDir}/sync_state.json│
│     • Verified by sha256(disk file)         • configHash, usersHash  │
│     • Updated ONLY from Master ConfigJSON   • fullConfigHash         │
│     • Apply(): test + swap + reload + probe • desiredUsers[]         │
│     • Persist(): test + swap (NO reload!)   • circuitBroken          │
│                                                                      │
│  3. Runtime Ground Truth (Xray gRPC 127.0.0.1:8080)                  │
│     • StatsService.GetSysStats() -> Uptime (restart detection)       │
│     • HandlerService.GetInboundUsers(tag) -> live users per inbound  │
│     • HandlerService.AlterInbound(Add/Remove) -> zero-downtime heal  │
└──────────────────────────────────────────────────────────────────────┘
```

### Key Architectural Invariants
1. **Master `XrayCompiler` 是 `config.json` 的唯一生产者**：Agent 严禁解析修改或拼接 `settings.clients`；磁盘 `config.json` 只接受 Master 编译下发的 `ConfigJSON`，杜绝双套协议编译逻辑与静态客户端（`relay-auto:*@internal`、`*@trojan.local`）误删风险。
2. **两级哈希职责分离**：
   - `ConfigHash`（`structuralBytes` 哈希）：继续唯一决定是否需要 `reloadService`（结构级变更），并保持 `nodes.last_config_hash` 与 `Node.RefreshSyncStatus`（`internal/domain/node.go:221-235`）完全不变。
   - `FullConfigHash`（`fullConfigBytes` 哈希）vs `DiskConfigHash`（Agent 实测磁盘 `config.json` 的 SHA-256）：唯一决定磁盘 `config.json` 是否需要由 Master 下发 `ConfigJSON` 并原子落盘。
3. **运行态以 Xray gRPC 实测为准**：每次心跳核对 Xray 实际运行态（`GetInboundUsers` + `GetSysStats`），将 `UserSyncer.applied` 降级为 `GetInboundUsers` 不可用（`codes.Unimplemented`）时的回退缓存。

---

## 2. Data Flow & Wire Protocol Contracts

### 2.1 Master ↔ Agent Sync Protocol (`internal/domain/node.go` & `internal/agent/protocol.go`)

在 `AgentSyncRequest`（Master）与 `SyncRequest`（Agent）新增**唯一可选字段**：
```go
DiskConfigHash string `json:"diskConfigHash,omitempty"`
```
- Agent 每次 `SyncOnce` 实时读取并计算 `/usr/local/etc/xray/config.json` 的 SHA-256 十六进制字符串；若文件不存在，填 sentinel 值 `"missing"`（若读取报权限/IO 错误则记录警告并留空或上报 `"missing"`，避免阻塞心跳）。
- Master `cachedNodeSnapshot`（`internal/service/node_service.go:920-926`）新增字段 `FullConfigHash string`，由已有的 `fullConfigBytes`（`node_service.go:988`）直接 `sha256` 计算，零额外编译开销。
- Master `HandleAgentSync`（`node_service.go:881-912`）决策规则：
  ```go
  configUnchanged := req.ConfigHash != "" && req.ConfigHash == snap.ConfigHash
  usersUnchanged := req.UsersHash != "" && req.UsersHash == snap.UsersHash
  diskConfigStale := req.DiskConfigHash != "" && req.DiskConfigHash != snap.FullConfigHash

  if !configUnchanged || diskConfigStale {
      resp.ConfigJSON = snap.ConfigJSON
  }
  if !usersUnchanged {
      resp.Users = snap.Users
  }
  ```

### 2.2 Rolling Upgrade Compatibility Matrix

| 组合 | 行为 |
|---|---|
| **新 Master + 旧 Agent** | 旧 Agent 不发送 `diskConfigHash`（`""`）-> `diskConfigStale == false` -> 线协议与响应体 100% 与改造前一致。 |
| **旧 Master + 新 Agent** | 旧 Master 忽略 `diskConfigHash`，用户变更时返回 `ConfigUnchanged=true, UsersUnchanged=false, ConfigJSON=""`。新 Agent 记录提示日志跳过磁盘 `Persist`，仍正常执行 gRPC `Reconcile`、持久化 `sync_state.json` 并在 Xray 重启后通过 `GetInboundUsers`/`GetSysStats` 在 1 个周期内自愈运行态。 |
| **新 Master + 新 Agent** | 稳态零冗余载荷；用户变更时 `ConfigUnchanged=true` + `ConfigJSON` 下发 -> `Persist` 无重启落盘 + gRPC 热更新；磁盘篡改/丢失时 `diskConfigStale=true` -> 自动下发 `ConfigJSON` 修复落盘。 |

---

## 3. Xray gRPC Proto Extensions (`internal/adapter/xray/`)

经直接核对上游 `XTLS/Xray-core` `main` 分支源码：
1. `internal/adapter/xray/proto/proxyman.proto` + `proxyman.pb.go` + `proxyman_grpc.pb.go`：
   - Service: `xray.app.proxyman.command.HandlerService`
   - RPC: `rpc GetInboundUsers(GetInboundUserRequest) returns (GetInboundUserResponse);`
   - FullMethodName: `"/xray.app.proxyman.command.HandlerService/GetInboundUsers"`
   - Messages:
     ```protobuf
     message GetInboundUserRequest {
       string tag = 1;
       string email = 2;
     }
     message GetInboundUserResponse {
       repeated xray.panel.protocol.User users = 1;
     }
     ```
2. `internal/adapter/xray/proto/stats.proto` + `stats.pb.go` + `stats_grpc.pb.go`：
   - Service: `xray.app.stats.command.StatsService`
   - RPC: `rpc GetSysStats(SysStatsRequest) returns (SysStatsResponse);`
   - FullMethodName: `"/xray.app.stats.command.StatsService/GetSysStats"`
   - Messages:
     ```protobuf
     message SysStatsRequest {}
     message SysStatsResponse {
       uint32 NumGoroutine = 1;
       uint32 NumGC = 2;
       uint64 Alloc = 3;
       uint64 TotalAlloc = 4;
       uint64 Sys = 5;
       uint64 Mallocs = 6;
       uint64 Frees = 7;
       uint64 LiveObjects = 8;
       uint64 PauseTotalNs = 9;
       uint32 Uptime = 10;
     }
     ```
   *(注：proto3 线协议仅编码字段编号与 wire type，`Uptime = 10` 为已核实的 Xray-core 精确字段号。如宿主机有 `protoc` 则直接重新生成，否则严格按 `protoc-gen-go` / `protoc-gen-go-grpc` 规范补充并通过真实 gRPC loopback server/client 单测验证编解码往返。)*

3. `internal/adapter/xray/grpc_client.go` 新增方法：
   - `GetSysUptime(ctx context.Context) (uint32, error)`：调用 `StatsService.GetSysStats`，返回 `resp.Uptime`。
   - `GetInboundUsers(ctx context.Context, inboundTag string) ([]RuntimeInboundUser, error)`：调用 `HandlerService.GetInboundUsers`，将返回的 `proto.User` 解码为规范化结构：
     ```go
     type RuntimeInboundUser struct {
         InboundTag string
         Email      string
         Protocol   string // "vless" | "vmess" | "trojan" | "shadowsocks" | "shadowsocks-2022"
         UUID       string // VLESS/VMess id, Trojan/SS password, SS2022 key
         Flow       string // VLESS flow
         Method     string // Shadowsocks cipher (lowercase, e.g. "aes-256-gcm"); empty for SS2022
     }
     ```
   - `DecodeAccountMessage(account *proto.TypedMessage)`：复用 `internal/adapter/xray/proto/proxy.pb.go` 的 `VLESSAccount` / `VMessAccount` / `TrojanAccount` / `ShadowsocksAccount` / `Shadowsocks2022Account` 进行反向解包，实现零外部依赖、与序列化顺序无关的凭证级漂移比对。

---

## 4. Agent Components Design (`internal/agent/`)

### 4.1 `ConfigManager.Persist` & `DiskConfigHash` (`config_manager.go`)
- 提取公共私有辅助方法 `validateAndWriteLocked(ctx context.Context, configJSON string) (backupPath string, hasBackup bool, err error)`：
  1. 创建父目录 -> 写入 `tempConfigPath(ConfigPath)`（`0644` + `Sync`）并 `defer os.Remove(tmpPath)`；
  2. 执行 `xray run -test -c <tmpPath>`；失败直接返回错误，绝不触碰原 `config.json`；
  3. 若原 `config.json` 存在，备份到 `config.json.bak`（`0600`）；
  4. `os.Rename(tmpPath, ConfigPath)` 原子替换。
- `Apply(ctx, configJSON)` 复用 `validateAndWriteLocked`，随后执行 `reloadService` + `waitForProbe`（失败调用 `rollbackLocked`）。
- 新增 `Persist(ctx, configJSON) error`：持有 `m.mu.Lock()`，仅调用 `validateAndWriteLocked`，**不调用 `reloadService`**。
- 新增 `HashCurrentConfig() (string, error)`：读取 `m.opts.ConfigPath` 计算 SHA-256 hex；文件不存在返回 `"missing", nil`。
- 新增只读解析辅助 `InspectConfigInbounds() ([]ConfigInboundMeta, error)`（仅用于读取 Master 已编译落盘的 `config.json` 中的动态入站 tag、protocol 及静态客户端 email 集合，**绝不修改 `config.json`**）。

### 4.2 `SyncStateStore` (`sync_state.go`)
- 持久化路径：`filepath.Join(spoolDir, "sync_state.json")`（未配置 `spoolDir` 或空字符串时退化为内存模式，保持单测兼容）。
- 结构：
  ```go
  type SyncState struct {
      ConfigHash     string       `json:"configHash"`
      UsersHash      string       `json:"usersHash"`
      FullConfigHash string       `json:"fullConfigHash"`
      DesiredUsers   []RemoteUser `json:"desiredUsers,omitempty"`
      CircuitBroken  bool         `json:"circuitBroken"`
      UpdatedAt      int64        `json:"updatedAt"`
  }
  ```
- 写入采用与 `QuotaStore` 一致的原子 temp + `Sync` + `os.Rename`（`0600` 权限）；读取若文件不存在返回零值 `SyncState{}, false, nil`，若 JSON 损坏记录 warn 并视同未初始化（强制从 Master 拉取权威状态，安全自愈）。

### 4.3 `UserSyncer` (`user_sync.go`)
1. **错误分类修复（D3）**：
   - `isIgnorableAddUserError(err)`：仅忽略 `"already exists"`、`"not a usermanager"`、`"does not support dynamic account operations"`；**不再忽略** `"handler not found"`、`"failed to get handler"`、`"not found"`。
   - `isIgnorableRemoveUserError(err)`：忽略 `"not found"`、`"not exist"`、`"not a usermanager"`、`"does not support dynamic account operations"`、`"handler not found"`。
   - 新增 `isHandlerNotFoundError(err)`（匹配 `"handler not found"` / `"failed to get handler"`）与 `isUnimplementedError(err)`（`status.Code(err) == codes.Unimplemented`），封装为哨兵错误 `ErrInboundHandlerMissing` 返回给 `Runner`。
2. **静态客户端保护判定 `isStaticClientEmail(email string, staticEmails map[string]struct{}) bool`**：
   - 保护三类静态客户端永不被 `RemoveUser`：
     1. `strings.HasPrefix(email, "relay-auto:") && strings.HasSuffix(email, "@internal")`；
     2. `strings.HasSuffix(email, "@trojan.local")`；
     3. 在当前磁盘权威 `config.json` 中存在且不在 `desiredUsers` 中的静态客户端（若有）。
3. **运行态闭环核对 `ReconcileWithRuntime(ctx, desired, dynamicInbounds, staticEmails)`**：
   - 收集所有待核对的动态 inbound tags = `union(dynamicInbounds, tags in desired)`（跳过 `api` 及 `!domain.SupportsDynamicUser(protocol)`）。
   - 对每个 tag 调用 `xrayClient.GetInboundUsers(ctx, tag)`：
     - 若任一 tag 返回 `isHandlerNotFoundError` -> 立即返回 `ErrInboundHandlerMissing`（由 `Runner` 执行结构恢复）；
     - 若返回 `isUnimplementedError` -> 自动回退到经典 `Reconcile(ctx, desired)`（配合 `GetSysStats` `Uptime` 下降时提前 `ResetBaseline(nil)`）；
     - 若返回 `not a usermanager` -> 跳过该非动态入站；
     - 若返回其他 gRPC 错误（如 Xray 正在重启/连接拒绝）-> 返回错误，不更新基线与哈希。
   - 过滤掉 `isStaticClientEmail` 的条目后，得到 Xray 当前真实的动态用户 map `live[tag\x00email]`：
     - 对 `live` 中存在但 `desired` 不存在、或 `userRuntimeChanged(liveUser, desiredUser)` 的条目执行 `RemoveUser`；
     - 对 `desired` 中存在但 `live` 不存在、或已发生属性变更被移除的条目执行 `AddUser`；
     - 比较维度：`UUID`（必比）；VLESS 额外比 `Flow`；传统 Shadowsocks（`shadowsocks`/`ss`）额外比 `Method`（大小写不敏感）；SS2022 仅比 `UUID`（即 key，因 cipher 在入站层不在用户账户层）。
   - 同步更新内存缓存 `s.applied`（仅记录成功项）。

### 4.4 `Runner` 状态机与并发/失败处理 (`runner.go`)

1. **启动恢复（`NewRunner` / 初始化）**：
   - 若配置了 `SyncStateStore`，加载 `sync_state.json`：
     - 若 `QuotaStore.State().LocalCircuitBroken == true`，同步置 `r.circuitBroken = true` 并清空哈希；
     - 否则，计算实际磁盘 `HashCurrentConfig()`：仅当 `!state.CircuitBroken && state.FullConfigHash != "" && diskHash == state.FullConfigHash` 时，恢复 `r.configHash = state.ConfigHash`、`r.usersHash = state.UsersHash`、`r.desiredUsers = state.DesiredUsers`，并用 `state.DesiredUsers` 初始化 `userSync.ResetBaseline`（避免 `xray-agent` 重启时无意义重载 Xray）。
     - 若磁盘缺失/不匹配，保持哈希为空，首轮心跳自动从 Master 拉取全量 `ConfigJSON` 收敛。
2. **心跳前探活与 `DiskConfigHash` 采集**：
   - `SyncRequest.DiskConfigHash` = `cfgMgr.HashCurrentConfig()`（若 `cfgMgr` 支持且 `!r.circuitBroken`；熔断态可留空避免误拉业务配置）。
3. **响应收敛与自愈分支（`reconcileSyncResponse`）**：
   - **Branch 1 (`DesiredState == "circuit_broken"`)**：保持现有熔断逻辑，同时持久化 `sync_state.json{CircuitBroken: true}`，重置 `r.lastXrayUptime = 0`、`r.desiredUsers = nil`。
   - **Branch 2 (`!resp.ConfigUnchanged && resp.ConfigJSON != ""`)** — 结构变更：
     1. 调用 `cfgMgr.Apply(ctx, resp.ConfigJSON)`（校验 + 落盘 + 重载 + 探活）；
     2. 更新 `r.configHash = resp.ConfigHash`；若 `!resp.UsersUnchanged`，更新 `r.usersHash = resp.UsersHash`、`r.desiredUsers = clone(resp.Users)`、`userSync.ResetBaseline(resp.Users)`；
     3. 刷新 `r.lastXrayUptime`（通过 `GetSysUptime`），计算新 `FullConfigHash = sha256(resp.ConfigJSON)` 并原子保存 `sync_state.json`；
     4. 返回 `applied = true` 触发 `sendConfigAck`。
   - **Branch 3 (`resp.ConfigUnchanged == true`)** — 结构未变（用户热更新 / 磁盘漂移修复 / 稳态巡检）：
     1. **Step 3a 磁盘 SSOT 落盘（无重启）**：若 `resp.ConfigJSON != ""`：
        - 调用 `cfgMgr.Persist(ctx, resp.ConfigJSON)`（`xray run -test` + `.bak` + 原子替换，**绝不 reload**）；
        - 若失败，直接记录 `lastError` 返回错误（不改 `usersHash`、不改 `sync_state.json`）。
     2. **Step 3b Xray 运行态重启检测（`GetSysUptime`）**：
        - 若接口支持 `GetSysUptime`，读取当前 `uptime`：若 `r.lastXrayUptime > 0 && uptime < r.lastXrayUptime`，记录 `WARN "Detected xray process restart via uptime drop"` 并标记 `restarted = true`；无论是否下降，更新 `r.lastXrayUptime = uptime`。
     3. **Step 3c 运行态用户核对与自愈**：
        - 确定 `targetUsers`：若 `!resp.UsersUnchanged` 则使用 `resp.Users`；否则使用 `r.desiredUsers`（注意：若 `restarted && !r.hasDesiredUsers`，清空 `r.usersHash = ""` 让下一轮或立即从 Master 拉取权威用户！）。
        - 仅当 `!resp.UsersUnchanged || r.hasDesiredUsers || restarted` 时执行运行态核对：
          - 从 `cfgMgr.InspectConfigInbounds()` 提取动态 inbound tags 及静态客户端 emails；
          - 调用 `userSync.ReconcileWithRuntime(ctx, targetUsers, dynamicTags, staticEmails)`（若 `GetInboundUsers` 不可用且 `restarted == true`，先 `ResetBaseline(nil)` 再 `Reconcile`）：
          - **若返回 `ErrInboundHandlerMissing`**：说明 Xray 进程跑在缺失入站的旧/异常配置上 -> 若磁盘 `config.json` 已最新（`resp.ConfigJSON != ""` 或 `diskHash == state.FullConfigHash`），立即执行 `cfgMgr.ReloadAndProbe(ctx)` 使 Xray 加载最新磁盘 `config.json` 并刷新 `lastXrayUptime`；若仍缺失则清空 `r.configHash = ""`、`r.usersHash = ""` 下轮全量恢复！
          - 若返回其他错误（如 Xray 瞬时不可达）：记录 `lastError` 返回错误，不推进 `r.usersHash`（但磁盘 `config.json` 已在 3a 合法落盘，下轮重试即可收敛）。
     4. **Step 3d 提交状态与 Ack**：
        - 更新 `r.circuitBroken = false`、`r.lastError = ""`；若 `!resp.UsersUnchanged`，更新 `r.usersHash = resp.UsersHash`、`r.desiredUsers = clone(resp.Users)`、`r.hasDesiredUsers = true`；
        - 原子保存 `sync_state.json`；
        - 若发生实际变更（`!resp.UsersUnchanged || resp.ConfigJSON != "" || added > 0 || removed > 0 || reloaded`），返回 `applied = true` 触发 `sendConfigAck`。

---

## 5. Concurrency, Ordering & Failure-Mode Guarantees

| 场景 | 保障机制 |
|---|---|
| `geodata_update` 并发触发 `ReloadAndProbe` | `ConfigManager.mu` 严格串行 `Apply` / `Persist` / `ReloadAndProbe`；`Persist` 已先原子更新磁盘 `config.json`，`ReloadAndProbe` 重载的是最新用户配置，绝不回退。 |
| `xray run -test` 失败 | `Persist` 在 `os.Rename` 前中止并删除临时文件，原 `config.json` 完好，`usersHash` 不推进，`lastError` 上报 Master。 |
| `Persist` 成功但 gRPC `AlterInbound` 失败（如 Xray 正在重启） | 磁盘 `config.json` 已是合法新配置（Xray 启动即生效）；`r.usersHash` 与 `sync_state.json` 不推进，下一轮心跳继续校验并补齐运行态。 |
| 0 活跃用户（`len(Users) == 0`） | `r.hasDesiredUsers = true` 区分“明确 0 用户”与“未初始化”；`InspectConfigInbounds()` 提供动态 inbound tags，`GetInboundUsers` 清除所有僵尸动态用户并保留静态客户端。 |
| 并发 `go test -race` | `Runner` 字段仅在单事件循环 `SyncOnce`（或 `r.mu` 保护的 `lastError`/`configHash`/`usersHash`）读写；新增 `lastXrayUptime`/`desiredUsers`/`hasDesiredUsers` 严格遵循同锁保护。 |
| `cmd/agent` 依赖隔离 | 所有新增代码仅依赖标准库、`internal/domain`、`internal/adapter/xray`（无 `gin`/`gorm`/`sqlite`/`telegram`）。 |
