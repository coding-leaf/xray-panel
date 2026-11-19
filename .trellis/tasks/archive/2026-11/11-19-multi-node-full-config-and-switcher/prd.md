# Global Node Switcher & Per-Node Full Config/Logs

## Goal

将 `xray-panel` 升级为真正的 **1GB 内存多机全栈控制台**：通过顶部全局下拉栏一键切换目标节点（`本机 Master` 或任意远端直连机），完全复用现有前端界面管理每台节点的完整 **入站 / 出站（含 `warp-out`/`ipv4`/`ipv6`）/ 路由规划 / DNS / 配置预览 / 按需实时日志**；同时实现**编译期自动静态注入中转出站凭据**，彻底消灭用户列表中碍眼的 `relay-*@internal` 内部中转账号。

## Confirmed Facts

- 线上 4 台机（`vps-jp` Master + `vps-us`/`vps-us2`/`vps-tr` Agent）已跑通 M1–M3，现网验证了 `vps-us` 原有 `warp-out` 解锁出站、`vps-us2` 原有 `ipv4/ipv6` 出站及各自路由规则急需统一纳管（详见 [fullcfg-audit.md](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-19-multi-node-full-config-and-switcher/research/fullcfg-audit.md)）。
- 现网 `UserSyncer.Reconcile` 仅比对内存 `applied` 基线（`internal/agent/user_sync.go:40-87`），不会删除 `config.json` 中编译注入的静态中转 client，因此可以安全地在编译期自动静态注入中转凭据并清空 `users` 表中的 `relay-*@internal`。
- 前端严格遵循零 Pinia 规范（`.trellis/spec/frontend/index.md`），使用模块级 composable 单例驱动全局节点上下文。

## Requirements

### R1. 每节点完整 `Outbound / Routing / DNS` 持久化与编译
- `domain.Node` 新增 3 个持久化 TEXT 字段：`OutboundsJSON`、`RoutingJSON`、`DNSJSON`（`json:"-"`，由 `sqlite_db.go` 自动迁移；空字符串回退默认 `direct`/`block` 与基础路由）。
- `ConfigService` 与对应 HTTP Handler 统一支持 `?nodeId=<id>`（缺省或 `0` 表示本机 Master，100% 沿用原本地 `config.json` 读写与 `ConfigSnapshot`，零回归）：
  - `GET/POST/DELETE /api/outbounds?nodeId=`
  - `GET/POST /api/routing?nodeId=`
  - `GET/POST /api/dns?nodeId=`
  - `GET /api/config/raw?nodeId=`（`nodeId > 0` 返回该节点最新编译预览 JSON）
- 写入远端节点 `Outbounds`/`Routing`/`DNS` 前先调用 `XrayCompiler` 编译校验，无误后落库并立即触发 `invalidateNodeCache(nodeID)` 使 Agent 下次心跳同步生效。
- `XrayCompiler` 新增 `CompileRemoteNodeFull`：融合节点自定义 `Outbounds`（自动补齐缺失的 `direct`/`block`）、`Routing`（Layer 1 强制前置 `api -> api` 规则）、`DNS`、`Inbounds`（强制注入 `127.0.0.1:8080` `api` 入站）与静态中转凭据。

### R2. 编译期自动静态注入中转凭据（消灭 `relay-*@internal`）
- `NodeService.getOrBuildSnapshot` 在编译远端节点 `config.json` 时，自动扫描 Master 本机（`NodeID == 0`）的 `Outbounds`：
  - 凡 VLESS / VMess / Trojan 出站的目标 `address`（不区分大小写）匹配该远端 `Node.Host` 且目标 `port` 匹配该远端某启用 `Inbound.Port`（或 `ExternalPort`）的，自动提取其认证凭据（UUID/Password 及原始 `flow`，空即无 flow！）并去重静态注入到对应入站的 `settings.clients` 中（email 标为 `relay-auto:<outboundTag>@internal`）；
  - 静态中转 client 计入 `ConfigHash`（本机出站变更时 `ConfigService` 触发 `InvalidateAll()`），但不放入 `BuildRemoteNodeUsers` 下发给 `UserSyncer`，确保热更新用户永不误删中转通道。
- 上线迁移时可将之前临时建的 3 条 `relay-*@internal` 用户记录直接清理，用户管理恢复干净。

### R3. 按需拉取远端节点日志（平时 0 流量消耗）
- Master `AgentSyncResponse` 与 Agent `SyncResponse` 对称新增 `LogRequest *AgentLogRequest`（`omitempty`），`AgentSyncRequest` 与 `SyncRequest` 对称新增 `LogResult *AgentLogResult`（`omitempty`），无日志请求时为 `nil`，0 额外流量。
- `GET /api/logs?nodeId=<id>&service=xray|xray-agent&lines=100`：
  - `nodeId == 0`：保持原本地 `LogService.GetRecentLogs` 行为不变；
  - `nodeId > 0`：命中 30s 内存缓存直接返回；否则登记 `pendingLogReqs[nodeID]` 并等待唤醒（若超时仍未回传返回清晰状态提示，不永久卡死连接）。
- Agent 收到 `LogRequest` 时：
  - 白名单校验 `Service` 仅允许 `"xray"` 或 `"xray-agent"`，钳位 `Lines` 至 `[1, 1000]`；
  - 调用可注入的日志采集器（Linux 执行 `journalctl -u <service> -n <lines> --no-pager`）；
  - **立即补发一次轻量同步**（仅带 `ConfigHash`/`UsersHash`/`LogResult`，不重复采集/上报 `TrafficBatch`），实现 1 RTT 快速回传并唤醒前端；保持 `cmd/agent` 零重型依赖红线。

### R4. 前端顶部全局「节点切换器」与现有页面复用
- 新增零 Pinia 全局 composable `web/src/composables/useActiveNode.ts`（`activeNodeId` 持久化至 `localStorage`，默认 `0 = 本机 Master`，提供 `nodesList` / `activeNode` / `loadNodes` / `setActiveNode`）。
- 在 `web/src/App.vue` 顶部栏（桌面与移动端）集成节点下拉切换器（显示 `本机 Master` 与各远端节点在线状态/地区）。
- 6 个受管视图自动响应 `activeNodeId` 并传递 `?nodeId=`：
  - `InboundsView.vue`（与全局 `activeNodeId` 联动，新建入站默认归属当前节点）；
  - `OutboundsView.vue`（管理当前节点出站，含 `warp-out` 等）；
  - `RoutingView.vue`（管理当前节点路由规划；远端节点时隐藏或标注 GeoData 仅本机生效）；
  - `DNSView.vue`（管理当前节点 DNS）；
  - `ConfigView.vue`（远端节点模式展示只读编译预览 JSON 并隐藏本地覆盖保存/快照回滚）；
  - `LogsView.vue`（远端节点模式切换为按需日志面板：选 `xray` / `xray-agent`、行数、点击「拉取日志」展示终端输出）。
- `UsersView.vue` / `NodesView.vue` / `SettingsView.vue` 保持全局视角不受节点切换影响。

## Acceptance Criteria

- [ ] **AC-1**：单元测试覆盖 `Node` 自定义 `Outbounds`/`Routing`/`DNS` 读写、编译期强制保留 `api` 入站与 `api -> api` 规则、`ConfigService` `nodeId=0` 零回归与 `nodeId>0` 缓存失效。
- [ ] **AC-2**：单元测试与端到端集成测试覆盖：Master 本机 VLESS/Trojan 出站匹配远端 `Node.Host:Port` 时自动静态注入中转 client（保留原 `flow` 为空不被错补 `vision`），且 `UserSyncer` 热更新用户不误删该静态 client。
- [ ] **AC-3**：单元测试与端到端集成测试覆盖：无日志请求时心跳包零 `logRequest`/`logResult`；请求远端日志时 Agent 白名单校验、采集并立即补发轻量同步唤醒等待方（不重复上报 `trafficBatch`）；非法 `service` 名称被拒绝。
- [ ] **AC-4**：前端完成顶部全局节点切换器与 6 大视图 `?nodeId=` 适配；全量前后端质量门禁全部通过（`go test ./...`、`-race`、`go vet`、`cd web && npm run typecheck && npm run lint && npm run build`、`go build .`、`go build ./cmd/agent`、`go list -deps ./cmd/agent` 0 重型依赖）。

## Out of Scope

- 为远端节点自动执行 `wgcf`/WARP 账号注册或安装系统级软件包（仅托管 Xray 配置层面的 WireGuard/WARP 出站与路由规则）。
- 持续流式 WebSocket 推送远端全量访问日志。
