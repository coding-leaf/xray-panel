# 功能稳定性与安全风险评估（02 · P0 / P1 / P2 分级）

> 评级标准：
> - **P0（阻断/核心功能失效）**：存在真实生产链路必现或高频破坏、用户操作失败、核心监控/诊断完全不可用。
> - **P1（高危/一致性与超时边界）**：前后端状态/路由配置不一致、HTTP 超时与连接断开、安全暴露、高概率并发覆盖或持久化失控。
> - **P2（中低危/交互割裂与工程债）**：契约字段遗漏、文案/默认值漂移、Demo/Mock 模式缺失、边界体验瑕疵。

---

## 一、P0 级稳定性风险（必须立即修复）

### [P0-1] 远端入站 (`NodeID != 0`) 导致 `UserService` 写操作与 Telegram `/adduser` 在本机 Xray gRPC 处失败回滚

- **证据链**：
  - `internal/adapter/xray/manager.go:36-66`：`AddUser` 与 `RemoveUser` 查询 `inbound, err := m.inboundRepo.GetByTag(ctx, inboundTag)` 后，仅检查 `SupportsDynamicUser(inbound.Protocol)`，**完全未检查 `inbound.NodeID != 0`**，直接调用本机 `m.grpcClient.AddUser(ctx, inbound, user)` / `RemoveUser`。
  - `internal/service/user_service.go:88-99` vs `101-108`：删除路径的 `isXrayIgnorableRemoveError`（88-99 行）忽略了 `"handler not found"` / `"failed to get handler"` / `"not found"` / `"not exist"`，而新增路径的 `isXrayIgnorableAddUserError`（101-108 行）**仅忽略 `"not a usermanager"` 和 `"does not support dynamic account operations"`**。
  - `internal/service/user_service.go:259-279` (`CreateUser`)、`469-481` (`UpdateUser`)、`685-689` (`resetTrafficNoSave`)、`748-758` (`ResetSubToken`)：全部对用户授权列表中的每个 tag 调用 `s.xrayManager.AddUser`。
  - `internal/adapter/telegram/bot.go:385-395`：`/adduser` 指令直接遍历所有已启用入站（**未过滤 `NodeID == 0`**）作为默认授权列表传入 `userSvc.CreateUser`。
  - **测试盲区**：`internal/delivery/http/agent_e2e_test.go:170, 581` 构造 `UserService` 时传入 `xrayManager = nil`，导致端到端测试绕过了真实 `xray.Manager` 对本机 gRPC 的调用。
- **影响与复现路径**：
  - 当系统存在任意启用的远端节点入站（`inbound.NodeID > 0`）并被分配给用户（或管理员通过 TG `/adduser` 自动全选入站）时，本机 Xray 实例中不存在该远端 `tag` 的 handler；
  - 本机 gRPC 返回 `handler not found`，`isXrayIgnorableAddUserError` 返回 `false`，触发 Saga 事务回滚，用户创建/编辑/流量重置/Token 重置全部失败。
- **修复建议**：
  1. 在 `internal/adapter/xray/manager.go` 的 `AddUser` 与 `RemoveUser` 开头补充 `if inbound.NodeID != 0 { return nil }`（远端入站由对应 Agent 的 `UserSync` 通过心跳接管，严禁操作主控机本机 gRPC）；
  2. 在 `internal/adapter/telegram/bot.go:385-395` 按需明确区分本机入站或直连远端入站权限，并为 `agent_e2e_test.go` 补充带非空 `xrayManager` 校验“只操作 `NodeID == 0` 入站”的回归测试。

---

### [P0-2] 按需远程日志拉取在 60s 默认心跳下结构性必超时，且存在并发覆盖竞态

- **证据链**：
  - `internal/service/node_service.go:23-26`：在 `b0330a6` 将 `defaultNodeSyncIntervalSec` 调整为 `60`（秒）后，`defaultRemoteLogWaitTimout`（注：变量名存在拼写 `Timout`）仍为 **`25 * time.Second`**。
  - `internal/service/node_service.go:1037-1062` (`RequestNodeLogs`)：
    - 第 `1039` 行直接赋值 `s.pendingLogReqs[nodeID] = &domain.AgentLogRequest{...}`，若同一节点有第二个请求到达，会直接覆写并孤立前一个请求的等待通道；
    - 第 `1058-1060` 行 `defer` 在超时返回时执行 `if cur, ok := s.pendingLogReqs[nodeID]; ok && cur.RequestID == reqID { delete(s.pendingLogReqs, nodeID) }`；
  - `internal/service/node_service.go:1153-1166` (`popPendingLogRequest`)：依赖 Agent 心跳时来消费 `pendingLogReqs[nodeID]`。
- **影响与复现路径**：
  - 由于 Agent 默认每 `60s` 心跳一次，而 Master 仅等待 `25s`：当缓存 TTL（30s）过期后发起日志查看，极大概率（约 `35/60` 窗口）在 Agent 下一次心跳到来前即达到 25s 超时；
  - 更致命的是：**超时退出时 `defer` 直接删掉了 `pendingLogReqs[nodeID]`**，导致后续到来的 Agent 心跳根本拿不到采集任务，刷新页面重新请求又开始下一轮 25s 超时删单，形成**持续无法拉取日志的死循环**。
- **修复建议**：
  1. 将远程日志拉取改为**非阻塞异步任务或保留已入队请求**：触发时入队命令（若已有未过期请求则复用等待或返回 202 `pending` 状态由前端短轮询），超时等待不应销毁尚未被 Agent 取走的指令（改为独立 TTL 如 120s 过期清理）；
  2. 将 `defaultRemoteLogWaitTimout` 拼写修正并合理匹配异步轮询模式，防止单槽位并发互相踩踏。

---

## 二、P1 级稳定性、安全与数据一致性风险

### [P1-1] 远程 GeoData 下发等待时间 (`75s`) 超过 `http.Server.WriteTimeout` (`60s`)，且同步阻塞 Agent 心跳

- **证据链**：
  - `internal/delivery/http/server.go:44-45`：HTTP 服务硬编码 `ReadTimeout: 30 * time.Second`、`WriteTimeout: 60 * time.Second`。
  - `internal/service/node_service.go:26`：`defaultRemoteGeoWaitTimeout = 75 * time.Second`（比 HTTP 写超时还多出 15s），且 `1188-1195` / `1255-1268` 无类似日志的短缓存兜底。
  - `internal/agent/runner.go:203-208`：`SyncOnce` 在同步主循环内**同步执行** `handleGeoUpdateRequest`。
  - `internal/agent/geodata_updater.go:19, 165`：单次下载超时 `defaultGeoDownloadTimeout = 60s`，且含多镜像回退重试。
- **影响**：
  1. 即使 Agent 在 60s~75s 内完成 GeoData 更新并回报结果，Master 的 HTTP 响应在 60s 时已被 Go `http.Server.WriteTimeout` 强行掐断连接，前端收到网络错误；
  2. 在网络较差的多镜像下载场景下，`SyncOnce` 可被阻塞超过 `180s`（`domain/node.go:203` 在线判定窗口），导致节点被误判 `offline` 甚至触发离线保护逻辑，且更新完成后不会立刻重新发送状态。
- **修复建议**：
  1. Master 侧改为“**下发异步任务 -> 立即返回 `jobId/accepted` -> 前端轮询状态**”（复用 `RoutingView` 已有的轮询模式），彻底脱离 `WriteTimeout=60s` 限制；
  2. Agent 侧将 GeoData 下载/替换移出 `SyncOnce` 阻塞路径，在独立 goroutine（带互斥防重入）中执行，完成后通过下一次心跳或专用结果上报接口汇报。

---

### [P1-2] 远端节点 `SubRoute` 自动路由规则仅在 UI 读取时临时生成，未注入 Agent 下发配置与 `ConfigView` 预览

- **证据链**：
  - `internal/service/config_service.go:1165`：`GetRoutingConfigByNode` 调用 `s.normalizeRoutingConfig(ctx, *rawCfg, nodeInbounds, true)`，在读取给 `RoutingView` / `TopologyView` 时**按需临时生成 `SubRoute`（如 `vlessRoute`）规则**；
  - `internal/service/node_service.go:886-914` (`getOrBuildSnapshot`) 与 `internal/service/config_service.go:1471-1480` (`GetNodeCompiledConfigRaw`)：均直接调用 `parseNodeRouting(node.RoutingJSON)`，**未调用 `ensureSubRouteRules(..., nodeInbounds)`**；
  - `internal/service/config_service.go:700-740`：远端入站增删改仅清理快照缓存，不刷新/合并 `node.RoutingJSON` 中的 `SubRoute` 规则。
- **影响**：
  - 管理员在入站表单或拓扑页配置完远端入站 `SubRoute` 后，打开 `RoutingView` 能看到规则（因为读取时动态补全了），但**实际推送到远端 Agent 的 `config.json` 与 `ConfigView` 只读预览中完全缺少该路由规则**，直到管理员专门点进该节点的 `RoutingView` 再点击一次保存！
- **修复建议**：
  - 抽取统一规范化入口：在 `getOrBuildSnapshot` 与 `GetNodeCompiledConfigRaw` 编译远端完整配置前，统一执行 `ensureSubRouteRules`（或等效纯函数）补全动态 `SubRoute` 规则；同步保持与 `GetRoutingConfigByNode` 完全一致的规则排序与过滤语义。

---

### [P1-3] 远端 Shadowsocks / SS-2022 动态热插拔缺少加密方式 (`Method`)，导致 gRPC `AddUser` 协议失配

- **证据链**：
  - `internal/adapter/xray/compiler.go:440-454` (`BuildRemoteNodeUsers`)：构建的 `AgentSyncUser` 仅包含 `InboundTag`、`Protocol`、`Email`、`UUID`、`Flow`，**未传递入站 `Method`/cipher**。
  - `internal/agent/xray_grpc_adapter.go:37-53`：Agent 仅用 `InboundTag`、`Protocol`、`streamSettings` 构造一个 **`SettingsJSON` 为空的合成 `domain.Inbound`**。
  - `internal/adapter/xray/protocol_adapter.go:100-110` & `stream_accessor.go:150-161`：`ResolveAdapter` 依赖 `ib.SettingsJSON` 中的 `ResolveShadowsocksMethod()`；当 `SettingsJSON` 为空时，默认回退为 `"aes-128-gcm"` 并选用 `ShadowsocksAdapter` 而非 `Shadowsocks2022Adapter`。
- **影响**：
  - 全量 `config.json` 重载时 SS-2022 / `aes-256-gcm` / `chacha20` 入站可正常启动，但运行期间新增或修改用户触发 Agent 增量 gRPC `AddUser` 时，会发送错误的 Account 类型或密码派生参数导致操作失败。
- **修复建议**：
  - 在 `AgentSyncUser`（或 `AgentSyncResponse` 的 inbound 元数据映射）中透传必要的加密方式字段（或下发精简 `InboundMeta` 供 `xray_grpc_adapter.go` 构造完整 `SettingsJSON`）。

---

### [P1-4] 远端直连入站未填 `ExternalPort` (`0`) 时，订阅导出被错误覆写为主控机 `public_port` (`443`)

- **证据链**：
  - `internal/service/sub_service.go:81-99` (`prepareInboundForNode`)：仅在 `ExternalHost == ""` 时填充 `node.Host`，对 `ibCopy.ExternalPort == 0` 未处理；
  - `internal/service/sub_service.go:187` (`parsePortAndFingerprint`) 读取主控机全局设置（默认 `443`）作为 `defaultPort`；
  - `internal/protocol/node_converter.go:153-158`：当 `inbound.ExternalPort <= 0` 且 `defaultPort > 0` 时，直接将 `targetPort` 覆写为 `defaultPort`（即主控机的 `443`），而非入站自身监听端口 `inbound.Port`。
- **影响**：
  - 远端节点如监听在 `:18443` 且直连无前置反代（用户按常识将 `externalPort` 留空/`0`），订阅链接却会把客户导向 `node.Host:443`，导致连接失败。
- **修复建议**：
  - 在 `prepareInboundForNode` 中，对于 `inbound.NodeID > 0` 且 `ibCopy.ExternalPort <= 0`，应显式置为 `ibCopy.ExternalPort = inbound.Port`（仅本机节点沿用主控机全局 `public_port` 兜底规则）。

---

### [P1-5] `QuickPublishWizardModal.vue` 一键发布无视当前选中的远端节点 (`activeNodeId`)

- **证据链**：
  - `web/src/views/topology/components/QuickPublishWizardModal.vue:478`：提交新建出站时固定调用 `await api.post('/outbounds', payload)`，**未拼接 `?nodeId=...`**。
- **影响**：
  - 当用户在 `TopologyView` 切换至某远端节点并使用“1步发布”创建链路时，出站网关被误写入主控机（`nodeId=0`），而入站与 `SubRoute` 规则落在远端节点上，造成跨节点配置割裂与路由指向不存在的 `outboundTag`。
- **修复建议**：
  - 向 `QuickPublishWizardModal.vue` 注入或传入 `activeNodeId`，将请求改为 `api.post(\`/outbounds?nodeId=\${nodeId}\`, payload)`。

---

### [P1-6] Raw JSON 编辑与快照回滚未失效远端节点缓存 + `syncFromRawJSONLocked` 跨节点标签串扰风险

- **证据链**：
  - `internal/service/config_service.go:147-157` (`RollbackSnapshot`) 与 `175-188` (`saveAndApplyRawConfigLocked`)：执行主控机全量配置替换后，**未调用 `invalidateAllNodeCaches()`**；当主控机 `outbounds` 中被远端中转引用的节点密钥发生变更时，远端节点快照缓存中的 `staticClients` 仍保留旧密钥；
  - `internal/service/config_service.go:353` (`syncFromRawJSONLocked`)：根据 Raw JSON 中的 `tag` 反查数据库时调用 `GetByTag(ctx, tag)`，但 `inbound.Tag` 在数据库表定义中为全局 `uniqueIndex`（`internal/domain/inbound.go`），且此处未显式校验 `existing.NodeID == 0`。
- **影响**：
  - 主控机执行快照回滚或高级 Raw 编辑修改 `outbounds`/用户后，远端中转节点不会自动重建 `StaticRelayClients` 快照；若 Raw JSON 中意外写入与某远端入站同名的 `tag`，可能覆盖该远端入站记录。
- **修复建议**：
  - 在 `saveAndApplyRawConfigLocked` 成功后调用 `s.invalidateAllNodeCaches()`；在 `syncFromRawJSONLocked` 中对 `existing.NodeID != 0` 的碰撞明确报错阻断，防止跨节点入站被 Raw 编辑篡改。

---

### [P1-7] `NodeTrafficBatch` / `NodeTrafficDaily` 只写不清理，删除节点遗留孤儿数据

- **证据链**：
  - `internal/adapter/repository/sqlite_db.go:46-47`：`AutoMigrate` 建表 `NodeTrafficBatch`（每次上报增量幂等记录）和 `NodeTrafficDaily`；
  - `internal/adapter/repository/node_repo.go:416` 与 `internal/domain/repository.go:127` 定义了 `PruneBatchesBefore`，**整个代码库没有任何生产调用点**；
  - `internal/service/node_service.go:573-591` (`DeleteNode`) 仅删除 `Node` 行，未清理该节点关联的 `NodeTrafficBatch` 与 `NodeTrafficDaily` 历史行。
- **影响**：
  - 按 60s 心跳、每节点每分钟写入 1 条 `NodeTrafficBatch` 计，每台节点每年产生约 52 万条记录，长期运行将持续膨胀 SQLite 体积并拖慢索引；删除节点后留下无外键约束的孤儿明细行。
- **修复建议**：
  - 在 `TrafficSyncJob`（或每日月结任务）中接入 `PruneBatchesBefore`（如保留 7 天内幂等窗口）；在 `NodeRepository.Delete` 事务内同步清理 `NodeTrafficBatch` 与 `NodeTrafficDaily`。

---

### [P1-8] 安全与配置一致性：`systemd` 安装脚本通过 CLI `--token` 明文暴露凭证，且 `--interval` 与 gRPC 端口存在硬编码漂移

- **证据链**：
  - `internal/service/node_service.go:685` (`RenderInstallScript`)：生成 `ExecStart=/usr/local/bin/xray-agent --master-url ${MASTER_URL} --token ${NODE_TOKEN} --spool-dir /var/lib/xray-agent --interval 20s`；
    - 明文命令行参数在同主机任意非 root 用户执行 `ps aux` 或读取 `/proc/<pid>/cmdline` 时可见（尽管 `cmd/agent/main.go` 已支持从环境变量读取）；
    - 默认 `--interval 20s`（`node_service.go:685`、`cmd/agent/main.go:47`、`internal/agent/runner.go:11`）仍残留旧值，与 `b0330a6` 改为 `60s` 不一致（Agent 首次连接前按 `20s` 运行，连接后才被覆盖为 `60s`）；
  - `internal/adapter/xray/compiler.go:403`：`appendAPIInbound(cfg, 8080)` 硬编码远端节点的 Xray API gRPC 端口为 `8080`，而 `cmd/agent/main.go` 暴露了可自定义端口的 `--xray-grpc` 参数，一旦管理员改了 Agent gRPC 端口，编译出来的 Xray 配置与 Agent 参数立即错位。
- **修复建议**：
  - 安装脚本将 `MASTER_URL` 与 `AGENT_TOKEN` 写入权限 `0600` 的 `/etc/xray-agent/agent.env` 并配合 `EnvironmentFile=` 注入；将 `--interval` 默认值统一为 `60s`；在同步协议或节点配置中允许下发一致的 API port（或明确文档/约束禁止不匹配覆盖）。

---

## 三、P2 级问题与边界瑕疵

| 编号 | 问题点 | 源码位置 | 说明与建议 |
|---|---|---|---|
| **P2-1** | 写操作返回 DTO 缺失 `configSyncStatus` | `internal/service/node_service.go:466, 498, 514` | `UpdateNode`、`SetNodeDesiredState`、`ResetNodeCycleTraffic` 仅调 `RefreshComputed` 而未调 `RefreshSyncStatus`，导致前端收到空的 `configSyncStatus` 并短暂闪烁后再等轮询修正。建议统一补齐 `RefreshSyncStatus`（需传入预期 hash）。 |
| **P2-2** | `NodesView` 文案与双份轮询状态分裂 | `web/src/views/NodesView.vue:52`; `useNodes.ts`; `useActiveNode.ts` | `NodesView.vue:52` 仍写着 `在线心跳 (90s 内)`，与后端 `180s` 窗口（`domain/node.go:203`）不一致；同时 `useNodes.ts` 与 `useActiveNode.ts` 各自维护独立节点列表与 `15s`/`5s` 定时器（加上 `App.vue` 的 `6s` 状态轮询）。建议统一为单一节点数据源与文案。 |
| **P2-3** | 前端 Demo Mock 断链、切换器作用域误导与 Prettier 失败 | `web/src/mock/index.ts`; `App.vue:732-735`; `TopologyView.vue:259` | ① `mock/index.ts` 完全没有 `/admin/nodes` mock 处理器，开启 `?mock=true` 后节点管理与 `?nodeId=` 页面全失效；② `App.vue` 在 Dashboard/Users/Settings 等仅作用于 Master 的页面也展示节点切换器，且顶栏“重启核心”永远重启 Master；③ `TopologyView.vue:259` 跳转日志未传递节点上下文；④ 24 个新/改前端文件未通过 Prettier 格式门禁。 |
| **P2-4** | Agent 离线物理网卡流量混杂 & 首拍零值 | `internal/agent/metrics.go:189-198`; `internal/agent/runner.go:148, 392` | 当存在 `pending_batch` 时，`SyncOnce` 跳过采集 Xray 流量，此时 `handleOfflineQuota` 只能依赖网卡级增量 `NicUpDelta`/`NicDownDelta`；若宿主机存在非 Xray 流量（如宿主机备份、未被 `br-`/`docker`/`veth`/`wg`/`tun` 过滤的虚拟接口或 SSH），可能提前触发离线配额熔断。建议作为已知权衡在文档中明确或在 UI 提示口径差异。 |
