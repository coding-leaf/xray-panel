# M1: Node Model, Master Sync Control Plane & Thin Agent

> Parent: `11-19-multi-node-direct-agent`

## Goal

建立多机直连架构的底层控制面与极薄节点端执行器：在保持 1GB 内存纯 Go + SQLite 极简约束与现有单机（`NodeID=0`）100% 向下兼容的前提下，让 `xray-panel` Master 能够管理多台远端直连 `Node` 与其专属 `Inbound`，并通过独立极瘦二进制 `cmd/agent`（运行官方原版 Xray）完成配置校验重载、gRPC 差量用户热更新及幂等流量汇总上报。

## Confirmed Facts

- 现有数据库为纯 Go SQLite（WAL，`SetMaxOpenConns(1)`），见 [`sqlite_db.go:27-51`](file:///d:/code/Workspace/xray-panel/internal/adapter/repository/sqlite_db.go#L27-L51)。
- 现有 `Inbound` 无 `NodeID` 字段（[`inbound.go:9-34`](file:///d:/code/Workspace/xray-panel/internal/domain/inbound.go#L9-L34)），`Tag` 全局唯一，可直接兼容现有 `User.InboundTags` 逗号分隔授权机制。
- 现有 `internal/adapter/xray` 已封装 `XrayCompiler`、`GRPCClient`、`SystemdSupervisor`，可直接复用于远端配置编译与 `cmd/agent` 本机 Xray 控制。

## Requirements

### R1. Node 领域模型与向下兼容 Schema
- 新增 `domain.Node` 模型与 `nodes` 表：包含 `ID`, `Name`, `Host`（公网 IP 或域名）, `Region`, `TokenHash`（SHA-256 十六进制）, `TokenPrefix`（前 8 位展示提示）, `Enabled`, `DesiredState`（`active` / `draining` / `circuit_broken`，M1 默认 `active`）, `Online`, `LastSeenAt`, `AgentVersion`, `XrayVersion`, `LastConfigHash`, `LastUsersHash`, `LastSyncError`, `CPUPercent`, `MemPercent`, `NicUpBytes`, `NicDownBytes`, `XrayUpBytes`, `XrayDownBytes`, `Remark`, `CreatedAt`, `UpdatedAt`。
- `domain.Inbound` 新增 `NodeID uint`（`gorm:"index;default:0"`）：`NodeID == 0` 代表当前 Master 本机（Local Node），现有本机配置编译与本地 `TrafficSyncJob` 仅处理 `NodeID == 0` 的入站，确保单机零迁移兼容。
- 新增 `domain.NodeTrafficBatch` 与 `node_traffic_batches` 表：`(node_id, batch_id)` 唯一索引，用于跨网重试流量上报的严格幂等去重，并支持清理超过 7 天的历史批次记录。

### R2. Master 节点管理 API 与最小权限认证
- 管理员接口（受现有 `JWTAuth` 保护）：
  - `GET /api/admin/nodes`：节点列表（不返回明文 Token 或 `TokenHash`）；
  - `POST /api/admin/nodes`：创建节点并生成 32 字节高熵随机 Token（**仅在创建响应中返回一次明文 Token**，数据库只存 SHA-256）；
  - `PUT /api/admin/nodes/:id`：更新节点元数据（`Name`/`Host`/`Region`/`Enabled`/`DesiredState`/`Remark`）；
  - `DELETE /api/admin/nodes/:id`：删除节点（若该节点下仍绑定入站则拒绝删除并返回 409/400）；
  - `POST /api/admin/nodes/:id/rotate-token`：重置吊销旧 Token，仅返回一次新明文 Token；
  - `GET /api/admin/nodes/:id/install-script`：生成一键安装/配置脚本指令模板（占位或使用显式传入的外部面板 URL，严禁硬编码真实域名/IP）。
- 既有 `Inbound` 管理接口支持读写 `nodeId`（默认 `0`），创建/更新/删除远端节点入站或变更用户授权时自动刷新该节点的内存 `ConfigHash`/`UsersHash` 缓存。
- Agent 专用鉴权中间件：校验 `Authorization: Bearer <token>`，按 SHA-256 查匹配节点且校验 `Enabled == true`，仅将 `nodeID` 注入请求上下文；无权访问任何 `/api/admin/*` 接口。

### R3. 单接口聚合同步协议 (`POST /api/agent/sync`)
- Agent 定时发送聚合同步请求，携带：
  - `configHash`, `usersHash`, `agentVersion`, `xrayVersion`, `lastError`；
  - `hostMetrics`（CPU%、内存%、网卡累计/增量收发字节数）；
  - `trafficBatch`（可选：`batchId` + 按用户和按入站聚合的增量上/下行字节数）。
- Master 幂等与短路处理：
  - 若 `trafficBatch.batchId` 非空，在单个 SQLite 事务内先查 `(node_id, batch_id)`：若已存在则跳过记账直接视为成功；若不存在则写入去重记录，并批量累加 `User.UpBytes/DownBytes`、`Inbound.UpBytes/DownBytes`、`TrafficLog`（按天）及 `Node.XrayUpBytes/XrayDownBytes`；
  - 从内存缓存读取该节点最新 `ConfigHash` 与 `UsersHash`（无缓存时回源 SQLite 并回填）；
  - 若请求 hash 一致，响应返回 `configUnchanged: true`、`usersUnchanged: true`（~200B 极简 JSON，零冗余序列化）；
  - 若 `configHash` 不一致，在响应中附带为该节点独立编译好的 `configJson`（直连出站 `freedom` + 本地 `127.0.0.1:8080` API 入站 + 该节点全部启用 `Inbound` 及初始活跃用户）；
  - 若 `usersHash` 不一致，在响应中附带该节点全部有效授权用户列表 `users`（按入站 Tag 分组）；
  - 始终返回 `desiredState`（`active` / `circuit_broken`）与 `syncIntervalSec`。

### R4. 独立极瘦 `cmd/agent` 二进制
- 新增入口 `cmd/agent/main.go` 与 `internal/agent/` 包，**严禁引入** `web`、`gorm`、`glebarez/sqlite`、`gin`、`telegram` 依赖，保持独立轻量编译。
- 支持环境变量与 CLI 参数配置：`--master-url`、`--token`、`--xray-bin`（默认 `xray`）、`--xray-config`（默认 `/usr/local/etc/xray/config.json`）、`--xray-grpc`（默认 `127.0.0.1:8080`）、`--xray-service`（默认 `xray`）、`--spool-dir`（默认 `/var/lib/xray-agent`）、`--interval`（默认 `20s`）。
- 核心执行闭环（单循环 + 优雅退出 `SIGINT`/`SIGTERM`）：
  1. **配置安全重载与回滚**：收到新 `configJson` 时，先写入临时文件 → 执行 `xray run -test -c <tmp>` 校验 → 备份旧配置 `.bak` → 原子替换 → 触发 `reload/restart` → 3 秒内探活本机 gRPC；任一步失败立即回滚 `.bak` 重载并把 `lastError` 回报 Master；
  2. **gRPC 差量用户热更新**：对比 `currentUsers` 与 `desiredUsers`（按 `inboundTag + email`），仅通过本机 gRPC 精准调用 `AddUser` / `RemoveUser`，绝不因用户变动重启 Xray；
  3. **流量断点暂存与幂等重试**：调用本机 gRPC `QueryTrafficStats(reset=true)` 后立即生成唯一 `batchId` 并持久化到 `spool-dir` 小文件；仅在 Master 返回 200 OK 后删除本地暂存，断网/超时下次心跳自动重传；
  4. **主机与网卡采集**：读取主机 CPU/内存及 Linux `/proc/net/dev` 主网卡增量（非 Linux 平台安全降级为 0 且可编译测试）；
  5. **熔断原语预留**：当 Master 下发 `desiredState == "circuit_broken"` 时，自动停用/移除业务入站（保留 Agent 进程与心跳恢复能力）；恢复 `active` 时自动重建配置恢复上线。

## Acceptance Criteria

- [ ] **AC-1**：`go test ./...`、`go vet ./...`、`go build .`、`go build ./cmd/agent` 全部通过；`go list -deps ./cmd/agent` 不包含 `gin`、`gorm`、`glebarez/sqlite`、`telegram` 或 `web/dist` 嵌入。
- [ ] **AC-2**：`Node` CRUD、Token 创建（明文仅返回一次、库中仅存 SHA-256）、Token 轮换、拒绝删除仍有绑定入站的节点均有单元测试覆盖并通过。
- [ ] **AC-3**：现有单机（`NodeID == 0`）测试全部绿灯，远端 `Inbound`（`NodeID > 0`）不会被混入本机 `XrayCompiler` 编译结果。
- [ ] **AC-4**：`POST /api/agent/sync` 单元测试与集成测试验证：(a) 无效/禁用 Token 返回 401/403；(b) hash 未变时返回 `configUnchanged: true, usersUnchanged: true` 且不回传配置载荷；(c) 相同 `(node_id, batch_id)` 重复上报 2 次，用户与节点流量仅累加 1 次；(d) 跨并发心跳无竞态。
- [ ] **AC-5**：`internal/agent` 单元测试验证：(a) `xray run -test` 失败时自动回滚 `.bak` 并上报 `lastError`；(b) 用户列表变动仅触发差量 `AddUser`/`RemoveUser` 而非重启；(c) Master 500/超时时本地 spool 文件保留并在下次同步成功重传后删除；(d) `desiredState=circuit_broken` 可触发暂停入站并支持恢复。

## Out of Scope

- 多机直连节点订阅聚合改造与 Clash 客户端降级修复（属于 M2）。
- 前端 Vue 3 节点管理页面开发（属于 M2）。
- 节点月流量配额计算、账单日重置、80%/90%/95% 自动状态流转与 TG 告警（属于 M3）。
- 邀请码自助开户与用户状态查询页（属于 M4）。
