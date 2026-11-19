# Full-Config, Global Node Switcher & Remote Logs Architecture Audit

> Task: `11-19-multi-node-full-config-and-switcher` | Read-only audit with exact `file:line` anchors.

---

## 1. 后端 `Outbound / Routing / DNS` 配置读写与编译链路

### 1.1 现状锚点
- **HTTP 路由**：`internal/delivery/http/router.go:140-158` 注册 `/outbounds` (`GET/POST/DELETE`)、`/routing` (`GET/POST`)、`/dns` (`GET/POST`)、`/config/raw`、`/config/validate`、`/config/save`、`/config/snapshots`。
- **HTTP Handlers**：
  - `OutboundHandler` (`internal/delivery/http/handler_outbound.go:25-70`)：`List` / `Save` / `Delete` 当前**未读取 `nodeId`**，直接调用 `ConfigService`。
  - `RoutingHandler` (`internal/delivery/http/handler_routing.go:25-49`)：`Get` / `Save` 当前**未读取 `nodeId`**。
  - `DNSHandler` (`internal/delivery/http/handler_dns.go:20-41`)：`Get` / `Save` 当前**未读取 `nodeId`**。
  - `ConfigHandler` (`internal/delivery/http/handler_config.go:27-104`)：`GetRaw` / `ValidateRaw` / `SaveAndApply` / `ListSnapshots` / `RollbackSnapshot` 均仅操作本机 `configMgr`。
  - **参考实现**：`InboundHandler.List` (`internal/delivery/http/handler_inbound.go:38-51`) 已采用 `c.Query("nodeId")` 解析非负整数过滤节点。
- **Service 层读写与编译**：
  - `ConfigService` (`internal/service/config_service.go:37-47`) 持有 `configMgr`、`inboundRepo`、`userRepo`、`snapshotRepo`、`compiler`、`nodeRepo`、`nodeCacheInv`。
  - **Outbounds**：`ListOutbounds` (`config_service.go:902-927`)、`SaveOutbound` (`929-962`)、`DeleteOutbound` (`964-980`)、`saveOutboundsListAndRecompileLocked` (`982-1016`) —— 全部通过 `readParsedSectionsLocked` (`870-900`) 读取本机 `config.json`（JSONC 清洗），修改后调用 `compileAndWriteWithRawLocked` (`196-230`) 写回 `config.json` 并记录 `ConfigSnapshot` + 重载 `supervisor`。
  - **Routing**：`GetRoutingConfig` (`1020-1045`)、`SaveRoutingConfig` (`1047-1089`)、`ensureSubRouteRules` (`1105-1185`) —— 同样读写本机 `config.json`，并仅过滤 `filterLocalInbounds` (`NodeID == 0`, `83-91`)。
  - **DNS**：`GetDNSConfig` (`1198-1223`)、`SaveDNSConfig` (`1225-1264`) —— 同样读写本机 `config.json`。
- **Compiler & 远端节点同步链路**：
  - 本机编译：`XrayCompiler.Compile` (`internal/adapter/xray/compiler.go:127-291`) 编译 DNS、本机 `Inbounds (NodeID == 0)`、`api` 入站 (`178`)、Outbounds（自动补齐 `direct`/`block` `197-209`）、Routing（Layer 1 `api -> api` 规则 `220-224` + Layer 2 去重规则 `227-283`）。
  - 远端编译：`XrayCompiler.CompileRemoteNode` (`compiler.go:330-364`) 目前**硬编码**只输出 `direct`/`block` 出站、无 `DNS`、Routing 只有单条 `api -> api` 规则！
  - 远端快照与哈希：`NodeService.getOrBuildSnapshot` (`internal/service/node_service.go:741-801`) 先调用 `CompileRemoteNodeToJSON(nodeInbounds, nil)` 计算 `ConfigHash`（结构哈希），再调用 `CompileRemoteNodeToJSON(nodeInbounds, allUsers)` 生成 `ConfigJSON`，最后 `BuildRemoteNodeUsers` 生成 `UsersHash`。当 `ConfigHash` 变化时，Agent (`internal/agent/runner.go:205-221`) 自动覆盖 `config.json` 并重启 Xray。

### 1.2 最小复用改造方案 (`nodeId` 统一分流)
1. **持久化存储（KISS 首选：直接在 `domain.Node` 扩展 3 个 TEXT 字段）**：
   - 在 `internal/domain/node.go:93` `Node` 结构体新增：
     - `OutboundsJSON string` (`gorm:"type:text;default:''" json:"-"`)
     - `RoutingJSON string` (`gorm:"type:text;default:''" json:"-"`)
     - `DNSJSON string` (`gorm:"type:text;default:''" json:"-"`)
   - `sqlite_db.go:45` 已有 `&domain.Node{}` 的 `AutoMigrate`，无需新增表或新 Repository；空字符串即表示使用默认配置（`direct`/`block` + 默认 Routing + 无自定义 DNS）。
   - `NodeRepository` 新增/复用按 `nodeID` 读写上述字段的方法（或直接 `Update` / `GetByID`）。
2. **`ConfigService` 按 `nodeID uint` 路由**：
   - `nodeID == 0`：100% 走原 `configMgr` 读写本机 `config.json` + `ConfigSnapshot`，**零破坏**。
   - `nodeID > 0`：
     - 读：从 `nodeRepo.GetByID(ctx, nodeID)` 解析 `OutboundsJSON` / `RoutingJSON` / `DNSJSON`（复用 `parseOutboundsList`、`normalizeRoutingConfig`、`normalizeDNSConfig`）。
     - 写：序列化后写回 `Node` 对应字段；先用 `XrayCompiler.CompileRemoteNode`（或传入 `nodeID` 的编译方法）校验编译无误后再落库，并调用 `s.invalidateNodeCache(nodeID)` (`config_service.go:93-97`) 使 `NodeService` 缓存失效，Agent 下次心跳（≤20s）自动拉取新配置并重启 Xray。
   - `/config/raw?nodeId=<id>`（只读查看/校验）：`nodeId > 0` 时可直接返回 `CompileRemoteNodeToJSON` 编译结果（只读预览，避免手改 JSON 破坏受管远端节点，或在 `ConfigView` 明确提示远端节点为编译预览）。
3. **`XrayCompiler.CompileRemoteNode` 融合自定义配置与 `api` 兜底**：
   - 签名扩展（或新增 `CompileRemoteNodeFull(inbounds, outbounds, routing, dns, users)` 保持旧签名兼容）：
   - **复⽤ `Compile` 核心逻辑**：
     1. `DNS`：若 `dns != nil` 且有配置，按 `compiler.go:137-159` 编译。
     2. `Inbounds`：遍历已过滤的 `sortedInbounds`（不再限制 `NodeID == 0`），最后**强制 `appendAPIInbound(cfg, 8080)`**（确保远端 gRPC 统计/热更新可用）。
     3. `Outbounds`：编译节点自定义 `outbounds`（支持 `warp-out` / `ipv4` / `ipv6` 等），若未包含 `direct` / `block` 则自动补齐（复用 `181-209`）。
     4. `Routing`：Layer 1 **强制前置 `{Type: "field", InboundTag: ["api"], OutboundTag: "api"}`**，Layer 2 过滤掉用户误填的 `api` 规则后按序输出并去重（复用 `217-288`）。

---

## 2. 后端按需远端日志查看协议（0 平时流量消耗）

### 2.1 现状锚点
- **本机日志**：`LogHandler.GetLogs` (`internal/delivery/http/handler_log.go:26-50`) 调用 `LogService.GetRecentLogs` (`internal/service/log_service.go:34-92`) 读取本地 `access.log` / `error.log`。`router.go:166` 挂载为 `GET /api/logs`。
- **Agent-Master 同步契约**：
  - Master 侧：`domain.AgentSyncRequest` (`internal/domain/node.go:298-306`)、`domain.AgentSyncResponse` (`328-339`)。
  - Agent 侧：`agent.SyncRequest` (`internal/agent/protocol.go:11-19`)、`agent.SyncResponse` (`51-62`)。
  - 默认心跳间隔：`defaultSyncInterval = 20 * time.Second` (`internal/agent/runner.go:11`)，`NodeService.HandleAgentSync` (`internal/service/node_service.go:697`) 返回 `SyncIntervalSec`。

### 2.2 极简按需拉取闭环（0 平时流量）
1. **平时零开销**：`AgentSyncResponse.LogRequest` 与 `AgentSyncRequest.LogResult` 均使用 `omitempty` 指针，平时为 `nil`，心跳包**0 额外字节**。
2. **协议扩展**（`internal/domain/node.go` 与 `internal/agent/protocol.go` 保持字段镜像）：
   - `AgentLogRequest`: `{ RequestID string, Service string ("xray"|"xray-agent"), Lines int }`
   - `AgentLogResult`: `{ RequestID string, Service string, Lines []string, Error string, CollectedAtUnix int64 }`
3. **Master 内存态状态机（放在 `NodeService`，受 `s.mu` 保护）**：
   - `pendingLogReqs map[uint]*domain.AgentLogRequest`（每节点最多 1 个活跃请求）。
   - `logCache map[string]*cachedRemoteLog`（key = `fmt.Sprintf("%d:%s", nodeID, service)`，缓存 TTL = 30s）。
   - `logWaiters map[string][]chan *cachedRemoteLog`（key = `requestId`，用于前端请求长等待唤醒）。
4. **快速回传（<3s 无需等下一个 20s/60s 周期）**：
   - 前端请求 `GET /api/logs?nodeId=<id>&service=xray|xray-agent&lines=100`（或 `/api/admin/logs` 兼容）：
     - 若 `nodeId == 0`：走原 `LogService.GetRecentLogs`。
     - 若 `nodeId > 0`：若命中 30s 内 `logCache`（且 `len >= lines`）直接返回；否则写入 `pendingLogReqs[nodeID]` 并注册 `logWaiters[reqID]`，在 HTTP handler 中 `select` 等待（如最多 25s，略大于 1 个心跳周期，或客户端可轮询）。
   - Agent `Runner.SyncOnce` (`internal/agent/runner.go:160-243`) 收到 `resp.LogRequest != nil` 时：
     - 校验 `Service` 白名单仅允许 `"xray"` 或 `"xray-agent"`（防命令注入），`Lines` 钳位 `[1, 1000]`；
     - 执行 `exec.CommandContext(ctx, "journalctl", "-u", service, "-n", strconv.Itoa(lines), "--no-pager")`；
     - **立即发起一次轻量补发同步**（只带 `ConfigHash`/`UsersHash`/`LogResult`，不采集/不重复上报 `TrafficBatch`），Master 在 `HandleAgentSync` (`node_service.go:600`) 收到 `req.LogResult` 后立即写入 `logCache` 并 `close/send` 唤醒 `logWaiters[reqID]`！
   - 这样一旦 Agent 收到指令，**1 RTT (<1~3s)** 即完成回传，平时无日志请求时零流量。

---

## 3. 前端顶部「全局节点切换器」与各视图复用方案

### 3.1 现状锚点
- **规范约束**：`.trellis/spec/frontend/index.md:19` 明确**严禁引入 Pinia/Vuex**，状态一律使用组件内 `ref`/`computed`、领域 `composables/` 与模块级单例。
- **顶部栏**：`web/src/App.vue:206-283`（桌面端 Top Console Bar，位于面包屑与状态胶囊右侧）及 `286-330`（移动端 Navbar）。
- **API 封装**：`web/src/api/index.ts:30-55` 暴露 `api.get/post/put/delete(url, ...)`，`web/src/api/nodes.ts:93-96` 已有 `listNodes()`。
- **现有节点过滤**：仅 `InboundsView.vue:53-62` 在页面内部有局部 `nodeFilter` 下拉框，其余视图均未感知 `nodeId`。

### 3.2 零 Pinia 全局切换器与视图适配清单
1. **全局 Composable (`web/src/composables/useActiveNode.ts`)**：
   - 模块级单例 `ref<number>`（从 `localStorage.getItem('activeNodeId')` 初始化，默认 `0` = 本机 Master）+ `nodesList = ref<NodeItem[]>([])`。
   - 提供 `activeNodeId`、`activeNode`（computed）、`nodesList`、`loadNodes()`、`setActiveNode(id: number)`（同步写 `localStorage`）。
2. **`App.vue` 顶部栏集成**：
   - 在桌面端 `header` (`App.vue:217` 状态区左侧) 与移动端合适位置放置简洁节点下拉框（`<select>` 或轻量菜单）：`0: 本机 Master` + `nodesList`（显示在线状态绿/灰点与区域）。
   - `onMounted` 调用 `loadNodes()`；切换节点时若当前处于受管视图，各视图通过 `watch(activeNodeId, ...)` 自动重新拉取数据，无需整页刷新。
3. **视图分类**：
   - **响应 `activeNodeId` 的视图（传 `?nodeId=` 参数）**：
     - `InboundsView.vue`：同步全局 `activeNodeId`（移除或联动局部 `nodeFilter`，新建入站默认归属 `activeNodeId`）。
     - `OutboundsView.vue` (`56, 85, 99`)：`GET /outbounds?nodeId=`, `POST /outbounds?nodeId=`, `DELETE /outbounds/:tag?nodeId=`。
     - `RoutingView.vue`：`GET /routing?nodeId=`, `POST /routing?nodeId=`（GeoData 升级卡片仅 `activeNodeId === 0` 时可用或加提示）。
     - `DNSView.vue`：`GET /dns?nodeId=`, `POST /dns?nodeId=`。
     - `ConfigView.vue`：`GET /config/raw?nodeId=`（`nodeId > 0` 时展示只读编译配置预览并隐藏保存/回滚按钮，或明确标注）。
     - `LogsView.vue`：`nodeId === 0` 展示 Access/Error/Audit；`nodeId > 0` 切换为远端服务日志模式（`service=xray` / `xray-agent`，按需拉取按钮 + 终端输出）。
   - **保持全局视角的视图（不随 `activeNodeId` 过滤）**：
     - `UsersView.vue`（用户全局授权与配额）、`NodesView.vue`（节点注册与状态总览）、`SettingsView.vue`（系统全局设置）。

---

## 4. 优雅清理内部中转账号 (`relay-*@internal`)

### 4.1 现状与机制核查
- 代码库中无硬编码 `relay-` / `@internal`（`git grep` 确认），此类账号目前为人工在 `users` 表创建的系统级中转凭证。
- **关键发现（`internal/agent/user_sync.go:40-87` & `internal/agent/runner.go:214-236`）**：
  - Agent 的 `UserSyncer.Reconcile` **仅对比内存 `s.applied` baseline**（由 `resp.Users` 初始化），**不会**去扫描 Xray 运行时已有 clients 并删除未知 client！
  - 因此，只要静态中转 client 由 `CompileRemoteNode` 直接注入到落盘的 `config.json`（并计入 `structuralBytes` / `ConfigHash`），而 `BuildRemoteNodeUsers` 只输出普通 DB 用户，**`UserSyncer` 永远不会误删静态中转 client**。

### 4.2 方案对比（KISS 评估）
| 方案 | 原理 | 优点 | 缺点 / 风险 | 推荐度 |
|---|---|---|---|---|
| **方案 A：编译期自动静态注入** | `CompileRemoteNode` 扫描 Master（及所有节点）`Outbounds` 中的 VLESS/VMess/Trojan 出站，若 `address:port` 匹配某远端节点 `Host` + 启用 `Inbound.Port`，自动将出站中的 `uuid` 作为静态 client 注入该远端节点对应入站的 `settings.clients`（email 标记如 `relay-auto@internal`），并触发目标节点缓存失效 | **彻底消灭 `users` 表中的 `relay-*@internal` 脏数据**；中转流量不污染普通用户统计与限额；新增中转出站即自动生效 | 需确保出站变更时（`ConfigService`）调用 `InvalidateAll()` 或定向 `InvalidateNode`；若出站使用域名而非 `node.Host` IP 则需按域名/IP 匹配 | **强烈推荐（最 KISS、最干净）** |
| **方案 B：前端默认折叠/过滤系统账号** | 保留 `users` 表中的 `relay-*@internal`，在 `UsersView.vue` 默认隐藏 | 改动极小，零编译逻辑改动 | 仍占用用户配额/流量表记录、仍出现在统计与同步列表中，治标不治本 | 备选 / 兼容兜底 |

> **最佳实践**：采用 **方案 A** 为主（自动识别并静态注入 `config.json`，`UserSyncer` 天然不干扰），同时在 `UsersView` 提供可选清理/过滤提示帮助用户平滑删除历史 `relay-*@internal` 记录。

---

## 5. 候选 `.trellis/spec/` 清单

后续编写 `design.md` / `implement.md` 与执行 `implement.jsonl` / `check.jsonl` 时应注入以下规范：
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/index.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/directory-structure.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/database-guidelines.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/error-handling.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/quality-guidelines.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/backend/logging-guidelines.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/index.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/directory-structure.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/api-and-dataflow.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/components-and-routing.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/frontend/quality-guidelines.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/guides/cross-layer-thinking-guide.md`
- `D:/code/Workspace/xray-panel/.trellis/spec/guides/code-reuse-thinking-guide.md`
