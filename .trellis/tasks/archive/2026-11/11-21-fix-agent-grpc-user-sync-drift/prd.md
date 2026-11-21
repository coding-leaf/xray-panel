# PRD: 修复 `xray-agent` gRPC 热更新用户后在 Xray 重启时丢失且无法自愈

> Task ID: `11-21-fix-agent-grpc-user-sync-drift`  
> Priority: P1  
> Status: Planning (Ready for User Approval)

---

## 1. Goal & User Value

解决远端节点在仅发生用户变更（gRPC 热更新）后，一旦 `xray.service` 独立重启（人工重启、OOM、系统重启、`geodata_update` 触发 `ReloadAndProbe` 等）就会回退到旧 `config.json` 导致用户丢失/失效，且 `xray-agent` 无法感知运行态漂移导致故障永久持续的问题。

同时确立清晰的**唯一权威配置源（Single Authoritative Source of Truth, SSOT）**与闭环收敛机制，实现：
1. **零断流持久化**：仅用户变更时，不重启 Xray，但磁盘 `config.json` 与 Xray 运行态同步收敛至 Master 权威编译快照；
2. **重启防丢与运行态自愈**：任何外部 Xray 重启、运行态漂移或磁盘篡改均可在至多 1 个心跳周期内自动检测并自愈；
3. **无多余断流**：`xray-agent` 自身重启时，若磁盘 `config.json` 与 Xray 运行态已与 Master 一致，不触发无意义 Xray 重启。

---

## 2. Confirmed Facts & Root Causes

以下事实均经生产实测证据与源码审计验证（详见 [codebase-audit.md](./research/codebase-audit.md)）：

| # | 根因缺陷 | 严重级别 | 代码锚点与事实 |
|---|---|---|---|
| D1 | **gRPC 用户热更新未落盘 `config.json`** | P0 | Master 在 `internal/service/node_service.go:988` 已编译包含全部活跃用户与静态客户端的完整 `snap.ConfigJSON`，但 `HandleAgentSync`（`906-908`）在 `configUnchanged=true` 时省略了 `ConfigJSON`；Agent 在 `internal/agent/runner.go:276-289` 仅调用 `userSync.Reconcile` 修改 Xray 内存，磁盘 `config.json` 保持旧版本。任何 Xray 重启（含 `internal/agent/geodata_updater.go:143` 调用 `ReloadAndProbe`）都会回退到旧 `config.json`。 |
| D2 | **Agent 状态仅存内存且盲信自身缓存，无法感知 Xray 重启或漂移** | P0 | `Runner` 的 `configHash`/`usersHash`/`circuitBroken`（`internal/agent/runner.go:40-43`）及 `UserSyncer.applied`（`internal/agent/user_sync.go:21`）均仅驻留内存。Xray 独立重启后，Agent 仍上报旧哈希，Master 返回 `ConfigUnchanged=true, UsersUnchanged=true`，Agent 跳过同步（`runner.go:291-295`），形成永久分叉。此外 `xray-agent` 重启时因哈希清空会无条件触发 `ConfigManager.Apply` 重启 Xray。 |
| D3 | **`UserSyncer.AddUser` 误吞 `handler not found` 错误** | P1 | `internal/agent/user_sync.go:106,118-129` 对 `AddUser` 复用了 `isIgnorableXrayError`（忽略 `handler not found`/`failed to get handler`/`not found`），与 Master 侧 `internal/service/user_service.go:88-108` 不一致。若 Xray 缺少对应 inbound handler（如静默配置或重载窗口期），Agent 会静默忽略失败并将用户误标为已应用。 |
| D4 | **静态客户端保护约束（否决 Agent 本地 Patch JSON）** | 架构约束 | Master `XrayCompiler` 在 `config.json` 中注入了不出现在 `BuildRemoteNodeUsers` 中的静态客户端：`relay-auto:<tag>@internal`（`internal/service/node_service.go:1502-1594`）和 `<tag>@trojan.local`（`internal/adapter/xray/protocol_adapter.go:243-259`），且包含协议专属编译规则（VLESS flow、SS2022 key）。Agent 端本地拼接/Patch `config.json` 会破坏唯一配置源并导致静态客户端误删或协议分歧。 |
| D5 | **Xray gRPC API 就绪情况** | 架构基础 | Master 远端编译配置（`internal/adapter/xray/compiler.go:86-89`）与 Agent 静默配置（`internal/agent/config_manager.go:293-296`）均已开启 `"HandlerService"` 与 `"StatsService"`；本地 `.proto` 位于 `internal/adapter/xray/proto/`，只需补充 `HandlerService.GetInboundUsers` 与 `StatsService.GetSysStats` 声明，无需引入新 Go 依赖或修改 Xray `api.services`。 |

---

## 3. Single Authoritative Source of Truth (唯一权威配置源)

确立三层单向权威链，消除多副本分裂：

1. **全局控制面唯一权威源 — Master `cachedNodeSnapshot`（`XrayCompiler` 唯一编译产物）**
   - 所有节点的完整 `config.json`（含结构配置、静态客户端、全部活跃 DB 用户）**仅由 Master `XrayCompiler` 编译生成**，产出不可变三元组：
     - `ConfigJSON` 及 `FullConfigHash = sha256(ConfigJSON)`（完整文件权威指纹）；
     - `ConfigHash = sha256(structuralBytes)`（结构指纹，仅用于判断是否必须 reload/restart Xray，同时兼容 `Node.RefreshSyncStatus`）；
     - `Users` 及 `UsersHash = sha256(json(Users))`（动态 DB 用户投影，用于零断流 gRPC 差分热更新）。
   - **禁止 Agent 自行合成或 Patch `config.json`**。
2. **节点本地持久化镜像 — `/usr/local/etc/xray/config.json` + `{spoolDir}/sync_state.json`**
   - 磁盘 `config.json` 必须通过密码学哈希（`DiskConfigHash = sha256(disk config.json)`）与 Master `FullConfigHash` 持续校验对齐；
   - `{spoolDir}/sync_state.json` 原子持久化最近一次完整收敛的 `ConfigHash`、`UsersHash`、`FullConfigHash`、`DesiredUsers`、`CircuitBroken`，仅作为 Agent 重启免断流与断网/快速自愈的本地镜像，绝不覆盖 Master 权威。
3. **运行态实测事实源 — Xray Live gRPC（`GetInboundUsers` + `GetSysStats`）**
   - `UserSyncer.applied` 降级为优化/降级缓存，不再作为运行态真相；Agent 直接通过 Xray gRPC 实测运行态用户列表与进程 `Uptime`，与权威期望状态闭环比对并自愈。

---

## 4. Requirements

### R1: Master 完整配置指纹与按需下发（兼容滚动升级）
- `cachedNodeSnapshot` 计算并缓存 `FullConfigHash = sha256(fullConfigBytes)`。
- `AgentSyncRequest` / `SyncRequest` 新增可选字段 `diskConfigHash`（`omitempty`）。
- Master `HandleAgentSync` 当 `!configUnchanged` **或** `req.DiskConfigHash != "" && req.DiskConfigHash != snap.FullConfigHash` 时，在 `AgentSyncResponse` 中下发 `ConfigJSON = snap.ConfigJSON`；`ConfigUnchanged` 仍严格由 `req.ConfigHash == snap.ConfigHash` 决定（保持“是否需要 reload Xray”语义与旧版 Agent 100% 线协议兼容）。

### R2: Agent 无重启校验落盘能力（`ConfigManager.Persist`）
- 在 `ConfigManager` 新增 `Persist(ctx, configJSON)` 方法（受 `m.mu` 互斥锁保护，与 `Apply` / `ReloadAndProbe` 串行）：写入临时文件 -> 执行 `xray run -test -c <tmp>` -> 备份 `.bak` -> 原子 `os.Rename` 覆盖 `config.json`，**不调用 `reloadService`**。
- 若 `xray run -test` 或写入失败，保持原 `config.json` 不变并返回错误，阻止推进 `usersHash` 与运行态变更。

### R3: Xray gRPC 运行态观测扩展（`GetInboundUsers` & `GetSysStats`）
- 在 `internal/adapter/xray/proto/` 按上游 `XTLS/Xray-core` 精确线协议补充：
  - `HandlerService.GetInboundUsers(GetInboundUserRequest{tag, email}) -> GetInboundUserResponse{users}`；
  - `StatsService.GetSysStats(SysStatsRequest{}) -> SysStatsResponse{..., Uptime = 10}`。
- 在 `internal/adapter/xray/grpc_client.go` 与 `internal/agent/xray_grpc_adapter.go` 暴露 `GetSysUptime(ctx)` 与 `GetInboundUsers(ctx, inboundTag)`（并将 `TypedMessage` 账户解码为可比对的 `Protocol`/`UUID`/`Flow`/`Method`）。

### R4: 修复 `UserSyncer` 错误分类与运行态真实比对自愈
- **修复 D3**：拆分 `AddUser` 与 `RemoveUser` 的可忽略错误判断；`AddUser` 仅忽略 `already exists`、`not a usermanager`、`does not support dynamic account operations`，**严禁忽略 `handler not found`/`failed to get handler`/`not found`**。
- 当 Xray 支持 `GetInboundUsers` 时，基于 Xray 实际运行态用户集合（按动态 inbound tag 逐个查询，自动跳过/保护静态客户端 `relay-auto:*@internal`、`*@trojan.local` 及磁盘权威配置中的非动态客户端）与 `desired` 计算真实差集执行 `RemoveUser` / `AddUser`。
- 若 `GetInboundUsers` 返回 `handler not found`，向上返回明确错误信号触发结构级配置恢复（`ConfigManager.Apply` 或重载）；若返回 `codes.Unimplemented`（兼容极旧 Xray），自动降级至 `GetSysStats` `Uptime` 下降检测 + 清空基线全量补齐。

### R5: Agent 持久化同步状态与闭环自愈状态机
- 新增原子持久化存储 `{spoolDir}/sync_state.json`（与 `Spool`/`QuotaStore` 同目录规范）。
- Agent 启动时加载 `sync_state.json` 并校验实际磁盘 `config.json` SHA-256 及 `QuotaStore.LocalCircuitBroken`：一致时复用已同步哈希，避免 `xray-agent` 重启触发无意义 Xray 重载。
- 每次 `SyncOnce` 形成闭环：
  1. 实测磁盘 `config.json` 的 `DiskConfigHash`（文件缺失时上报 `"missing"`）发往 Master；
  2. 结构变更（`!ConfigUnchanged && ConfigJSON != ""`）：执行 `ConfigManager.Apply`（校验+落盘+重载+探活），更新状态并落盘 `sync_state.json`；
  3. 仅用户变更或磁盘漂移（`ConfigUnchanged == true`）：
     - 若 `ConfigJSON != ""`：先执行 `ConfigManager.Persist` 校验并原子落盘（无重启）；
     - 执行 gRPC 运行态核对与收敛（当 `!UsersUnchanged` 用 `resp.Users`；当 `UsersUnchanged` 用 `GetSysStats` + `GetInboundUsers` 核对本地 `desiredUsers`）；
     - 若检测到缺失 handler 且磁盘配置完好，执行 `ReloadAndProbe` 后复核；若本地缺少 `desiredUsers` 但检测到 Xray 重启/漂移，清空 `usersHash` 触发从 Master 重拉；
  4. 仅在磁盘落盘（如有）与运行态收敛**均成功**后，才更新内存哈希、`sync_state.json` 并发送 `sendConfigAck`。
- 熔断态（`circuit_broken` / `LocalCircuitBroken`）下跳过业务用户运行态核对，恢复为 `active` 时完整重建。

### R6: Fail-Repro First 回归测试覆盖
- 先编写失败复现测试，再实施修复，最终全绿验证以下核心路径：
  1. `Apply` -> gRPC `Reconcile` 新增/修改/删除用户 -> 验证无 Xray reload 且磁盘 `config.json` 已包含最新用户 -> 模拟 Xray 重启丢失内存用户 -> 下一次 `SyncOnce` 自动补回；
  2. Xray 外部重启且磁盘 `config.json` 被篡改/回退 -> 下一次 `SyncOnce` 自动重写 `config.json`（无重启）并通过 gRPC 补齐缺失用户且清理僵尸用户，同时保留 `relay-auto:*@internal` 和 `@trojan.local` 静态客户端；
  3. `AddUser` 遇到 `handler not found` 时不再静默吞错、不推进 `usersHash`，并触发恢复；
  4. `xray run -test` 失败时不覆盖磁盘 `config.json`、不推进 `usersHash`；
  5. `xray-agent` 重启后，磁盘与运行态一致时不触发 Xray 重载；
  6. 滚动升级双向兼容（旧 Agent + 新 Master、新 Agent + 旧 Master）。

---

## 5. Acceptance Criteria

- **AC1（SSOT 落盘零断流）**：仅发生用户增/删/改时，Master 下发 `ConfigJSON`（`ConfigUnchanged=true, UsersUnchanged=false`），Agent 经 `xray run -test` 校验后原子更新磁盘 `config.json` 并通过 gRPC 热更新用户，**Xray reload 次数为 0**，且磁盘 `config.json` SHA-256 与 Master `FullConfigHash` 完全一致。
- **AC2（Xray 重启不丢且运行态漂移自愈）**：无论模拟 Xray 重启丢失内存用户、手动回退磁盘 `config.json`、还是运行态出现缺失/僵尸/凭证过期的动态用户，在至多 1 个 `SyncOnce` 周期内，磁盘 `config.json` 与 Xray 运行态均自动收敛回 Master 权威状态，且静态客户端（`relay-auto:*@internal`、`*@trojan.local`）完好保留。
- **AC3（无静默吞错）**：`UserSyncer.AddUser` 收到 `handler not found` 时明确报错且不将该用户写入 `applied`、不推进 `usersHash`，并在检测到 handler 缺失时触发恢复。
- **AC4（Agent 重启零断流）**：`xray-agent` 进程重启后，若磁盘 `config.json` 与 Xray 运行态均已与 Master 一致，不触发 Xray reload。
- **AC5（安全回滚/防护）**：`ConfigManager.Persist` 在 `xray run -test` 失败时原 `config.json` 保持不变且返回错误；`geodata_update` 并发触发 `ReloadAndProbe` 与 `Persist`/`Apply` 无竞态且不丢用户。
- **AC6（滚动升级兼容）**：旧 Agent（不带 `diskConfigHash`）对接新 Master 行为与现有协议 100% 一致；新 Agent 对接旧 Master（`ConfigJSON == ""`）仍可完成 gRPC 热更新与运行态自愈而不报错崩溃。
- **AC7（质量门禁全绿）**：`mise x -- go test ./...`、`CGO_ENABLED=1 mise x -- go test -race ./...`、`mise x -- go vet ./...`、`mise x -- go build .` 全部通过（且保持 `cmd/agent` 零 `gin`/`gorm`/`sqlite`/`telegram` 依赖隔离）。

---

## 6. Out of Scope

- 不修改 Master 本地节点（`NodeID == 0`）逻辑（已具备 `SaveConfigQuietly` 落盘能力）。
- 不修改 SQLite 数据库 Schema、前端 `web/` 页面或现有 API 路由路径。
- 不依赖 `systemctl show` 等平台绑定命令检测 Xray 重启（统一采用跨平台 Xray gRPC 协议，兼容 Docker / 非 systemd / Windows 单测）。
- 不在本任务中对线上 `vps-us2` 执行 SSH 运维操作。
