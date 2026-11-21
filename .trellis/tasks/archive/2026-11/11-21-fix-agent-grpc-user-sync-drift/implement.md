# Implementation Plan: `xray-agent` 唯一权威配置源与运行态自愈

> Task: `11-21-fix-agent-grpc-user-sync-drift`  
> Methodology: **Fail-Repro First**（先写失败回归测试复现缺陷，再分步实现修复，最终全套门禁绿灯）

---

## 1. Ordered Implementation Checklist

### Step 1: Fail-Repro First — 编写核心故障复现测试（预期先红灯）
- [ ] 在 `internal/agent/user_sync_test.go` 添加复现测试：
  - `AddUser` 收到 `handler not found` 时不得静默忽略并误标为已应用（复现 D3）。
- [ ] 在 `internal/agent/runner_test.go` / `internal/service/node_service_test.go` 添加复现测试：
  - Master 侧：用户变更且请求携带旧 `DiskConfigHash` 时，应返回 `ConfigUnchanged=true, UsersUnchanged=false` 且携带最新完整 `ConfigJSON`（旧 Agent 不带 `DiskConfigHash` 时仍省略 `ConfigJSON`）。
  - Agent 侧端到端故障链：全量 `Apply` -> 用户变更热更新（验证磁盘 `config.json` 更新且 reload 次数为 0）-> 模拟 Xray 重启清空运行态用户 -> 下一轮 `SyncOnce` 自动通过 gRPC 补回缺失用户。

### Step 2: Xray gRPC Proto 与 Client 扩展
- [ ] 更新 `internal/adapter/xray/proto/proxyman.proto`、`proxyman.pb.go`、`proxyman_grpc.pb.go`：添加 `HandlerService.GetInboundUsers`（`GetInboundUserRequest{tag=1, email=2}` -> `GetInboundUserResponse{users=1}`，方法路径 `/xray.app.proxyman.command.HandlerService/GetInboundUsers`）。
- [ ] 更新 `internal/adapter/xray/proto/stats.proto`、`stats.pb.go`、`stats_grpc.pb.go`：添加 `StatsService.GetSysStats`（`SysStatsRequest{}` -> `SysStatsResponse{..., Uptime=10}`，方法路径 `/xray.app.stats.command.StatsService/GetSysStats`）。
- [ ] 更新 `internal/adapter/xray/grpc_client.go`：新增 `RuntimeInboundUser`、`DecodeAccountMessage`、`GetInboundUsers(ctx, tag)`、`GetSysUptime(ctx)`。
- [ ] 在 `internal/adapter/xray/grpc_client_test.go` 补充真实 loopback gRPC server/client 测试，覆盖 5 种协议（VLESS/VMess/Trojan/Shadowsocks/Shadowsocks-2022）的 `GetInboundUsers` 往返解码与 `GetSysUptime` 验证。

### Step 3: Master 权威快照指纹与按需 `ConfigJSON` 下发
- [ ] 更新 `internal/domain/node.go` 与 `internal/agent/protocol.go`：在 `AgentSyncRequest` / `SyncRequest` 新增 `DiskConfigHash string `json:"diskConfigHash,omitempty"``。
- [ ] 更新 `internal/service/node_service.go`：
  - `cachedNodeSnapshot` 新增 `FullConfigHash`（对已有 `fullConfigBytes` 计算 SHA-256）；
  - `HandleAgentSync` 当 `!configUnchanged || (req.DiskConfigHash != "" && req.DiskConfigHash != snap.FullConfigHash)` 时下发 `resp.ConfigJSON = snap.ConfigJSON`。
- [ ] 在 `internal/service/node_service_test.go` 补充测试：覆盖 `DiskConfigHash` 匹配/不匹配/缺失（旧 Agent 兼容）/用户变更/磁盘漂移修复。

### Step 4: Agent `ConfigManager.Persist` & `SyncStateStore`
- [ ] 更新 `internal/agent/config_manager.go`：
  - 抽取 `validateAndWriteLocked`，实现无重启 `Persist(ctx, configJSON)`；
  - 新增 `HashCurrentConfig() (string, error)`（文件缺失返回 `"missing"`）与 `InspectConfigInbounds()`（只读提取动态 inbound tags 及静态客户端 emails）。
- [ ] 新增 `internal/agent/sync_state.go`（及 `sync_state_test.go`）：原子持久化 `{spoolDir}/sync_state.json`。
- [ ] 在 `internal/agent/config_manager_test.go` 补充 `Persist` 测试：校验成功原子替换且 `reloadCount == 0`、`xray run -test` 失败不覆盖原文件、`HashCurrentConfig` 行为。

### Step 5: `XrayGRPCAdapter`、`UserSyncer` 与 `Runner` 闭环自愈
- [ ] 更新 `internal/agent/xray_grpc_adapter.go`：暴露 `GetSysUptime(ctx)` 与 `GetInboundUsers(ctx, inboundTag)`，转换为 `RemoteUser`（或共享结构）。
- [ ] 更新 `internal/agent/user_sync.go`：
  - 拆分 `isIgnorableAddUserError` / `isIgnorableRemoveUserError`，修复 `AddUser` 误吞 `handler not found`；
  - 新增 `ErrInboundHandlerMissing` 与 `ReconcileWithRuntime(ctx, desired, dynamicTags, staticEmails)`（保护 `relay-auto:*@internal`、`*@trojan.local` 与权威配置静态客户端，支持 `codes.Unimplemented` 降级）。
- [ ] 更新 `internal/agent/runner.go` 与 `cmd/agent/main.go`：
  - 接入 `SyncStateStore` 启动恢复（与 `QuotaStore.LocalCircuitBroken` 对齐，健康重启免 Xray reload）；
  - 心跳携带 `DiskConfigHash`；
  - `reconcileSyncResponse` 实现「`Persist` 无重启落盘 -> `GetSysUptime` 重启探测 -> `ReconcileWithRuntime` 运行态闭环核对 -> `ErrInboundHandlerMissing` 重载恢复 -> 原子保存 `sync_state.json` -> `sendConfigAck`」。
- [ ] 完善单元测试与集成测试：
  - `internal/agent/user_sync_test.go`
  - `internal/agent/runner_test.go`
  - `internal/service/agent_e2e_test.go`（覆盖 Master+Agent 全链路热更新落盘、Xray 重启自愈、磁盘篡改修复、Agent 重启免重载、静态客户端保护、旧 Master/旧 Agent 双向兼容）。

---

## 2. Validation Commands (Mandatory Quality Gates)

```bash
# 1. 全套单元测试
mise x -- go test ./...

# 2. 竞态检测（Windows CGO）
CGO_ENABLED=1 mise x -- go test -race ./internal/adapter/xray/... ./internal/agent/... ./internal/service/... ./internal/delivery/http/...

# 3. 静态检查
mise x -- go vet ./...

# 4. 后端主程序 + Agent 二进制构建与依赖隔离验证
mise x -- go build .
mise x -- go build ./cmd/agent
```

---

## 3. Risky Files & Rollback Notes

- **线协议风险点**：`internal/adapter/xray/proto/*` 必须严格匹配上游 Xray-core 字段编号（`GetInboundUserRequest{tag=1,email=2}`, `GetInboundUserResponse{users=1}`, `SysStatsResponse{Uptime=10}`）与 gRPC FullMethodName。
- **并发风险点**：`internal/agent/runner.go` 与 `config_manager.go` 须保持锁粒度清晰，禁止在持有 `r.mu` 期间执行阻塞 RPC/IO，防止与 `geodata_update` 或状态查询死锁。
- **Git 工作区注意**：调研阶段发现仓库当前处于空的 `Cherry-pick currently in progress` 状态（`git diff` 为空），实施前请先确认是否安全清理或保持干净基线，切勿误提交无关 `.trellis/tasks/*` 未跟踪目录。
- **回滚点**：所有改动局限于 `internal/adapter/xray/`、`internal/domain/node.go`、`internal/service/node_service.go`、`internal/agent/`、`cmd/agent/main.go` 及对应 `*_test.go`，无 DB Schema 迁移，可原子 `git revert` 且支持 Master/Agent 独立滚动回滚。
