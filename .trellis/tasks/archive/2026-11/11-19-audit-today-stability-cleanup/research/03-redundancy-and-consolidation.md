# 冗余识别与功能/架构收敛方案（03）

---

## 一、冗余内容识别（剔除与去重候选）

### 1.1 死代码与未挂载方法（建议直接删除或补全接线）

| 类型 | 精确位置 | 实证证据 | 建议动作 |
|---|---|---|---|
| **死函数** | `internal/service/user_service.go:110` | `isXrayUserNotFoundError(err error) bool` 在全库 `internal/` 中**零调用点**（`git grep` 仅匹配函数定义本身），与同文件 `88-99` 行 `isXrayIgnorableRemoveError` 语义重叠。 | **直接删除**（清理历史残留） |
| **未接线的仓储方法** | `internal/adapter/repository/node_repo.go:416` & `internal/domain/repository.go:127` | `PruneBatchesBefore(ctx, before)` 仅有接口声明与实现，无任何 Cron 或 Service 调用（关联 P1-7）。 | **接入定时任务清理**（若短期不启用历史清理则勿留死接口） |
| **无 Mock 路由的死路径** | `web/src/mock/index.ts` (1025-1048 行末尾兜底) | 无任何 `/admin/nodes` mock 分支，Demo 模式请求节点 API 全落入默认 `{ success: true }`。 | 补充最小可用 Node Mock，避免开发演示模式崩溃 |

### 1.2 重复代码与重复逻辑（建议合并为共享实现）

| 重复模块 | 重复位置 A vs B | 具体重复证据 | 建议动作 |
|---|---|---|---|
| **节点入站查询 Helper** | `internal/service/node_service.go:831-849` (`findNodeInbounds`)<br>vs `internal/service/config_service.go:1490-1508` (`findInboundsByNode`) | 两处文件各自声明了完全相同的局部接口 `nodeInboundFinder`（`node_service.go:44` vs `config_service.go:1494`）和完全相同的类型断言回退逻辑，只因 `InboundRepository` 未正式暴露 `FindByNodeID`（尽管 `inbound_repo.go:56` 已实现）。 | **删除两份局部 Helper 与 `nodeInboundFinder`**，直接将 `FindByNodeID` 纳入 `domain.InboundRepository`（或统一包级 helper） |
| **节点流量口径与阈值公式** | `internal/domain/node.go:60-84` (`CalcTrafficByMode`) & `87` (`ReachedQuotaThreshold`)<br>vs `internal/agent/quota_store.go:59-83` (`CalcTrafficByMode`) & 阈值逻辑 | `internal/agent/xray_grpc_adapter.go` 本身已导入 `panel/internal/domain`（而 `domain` 仅依赖 Go 标准库，零外部依赖），但 `quota_store.go` 仍完整复制了一份 `CalcTrafficByMode` 与配额阈值计算逻辑。 | **`agent/quota_store.go` 直接复用 `domain.CalcTrafficByMode` 与 `domain.ReachedQuotaThreshold`**，消除双端计费公式漂移风险 |
| **3 份逐字节一致的静态 Portal HTML** | `deploy/cloudflare-pages/portal.html`<br>`deploy/cloudflare-pages/portal/index.html`<br>`deploy/standalone-portal.html` | 三份文件今日在 `d23b104` 同步修改（各 `+588` 级 diff），实测三者 **SHA-256 完全一致 (`A2BB7D6299B0A3E1D8BAE3F0DCE1AD5A67384C2558CE04A8FD469AEA2443C666`)**。 | **保留唯一源文件**（如 `deploy/standalone-portal.html`），在发布脚本/CI 中生成另外两份副本或改为符号链接/构建分发 |
| **剪贴板复制实现** | `web/src/views/PortalClaimView.vue:368-394` (`copyToClipboard`)<br>vs `web/src/utils/clipboard.ts:27` (`copyToClipboard`) | `PortalClaimView` 在页面内手写了一套带 fallback 的异步复制逻辑，未复用已有工具模块 `web/src/utils/clipboard.ts`。 | **删除页面私有实现**，统一从 `utils/clipboard` 导入 |
| **`nodeId` Query 解析与错误映射** | `internal/delivery/http/handler_config.go:17-40` (`parseOptionalNodeID` / `writePerNodeConfigError`)<br>vs `internal/delivery/http/handler_inbound.go:38-51` | `handler_inbound.go` 手工内联解析 `c.Query("nodeId")` 与错误处理，与 `handler_config.go` 的共享工具重复。 | 在 HTTP 交付层抽取统一的 `parseOptionalNodeID(c)`（或复用同包函数） |
| **GeoData 下载/镜像/校验与回滚** | `internal/service/geodata_service.go:163-253`<br>vs `internal/agent/geodata_updater.go:24-32, 165-221` | 两处分别维护相似的镜像 URL 列表、HTTP 下载超时、临时文件原子替换与失败回滚逻辑。 | 将纯标准库实现提取为共享轻量包（如 `internal/pkg/geodata`），供 Master 与 Agent 共同调用 |
| **SubRoute 排序/展开格式逻辑** | `internal/service/sub_service.go:429-491`<br>vs `internal/protocol/node_converter.go:427-474` | 订阅服务与协议转换层均处理 SubRoute 的排序、名称前缀与扩展规则。 | 收敛到单一构建入口，避免分享页与订阅链接输出不一致 |

### 1.3 过度设计与 KISS 违背

1. **仓储接口过度碎片化 (`internal/domain/repository.go:113-140`)**：
   - 同一个 `*GORMNodeRepository`（`internal/adapter/repository/node_repo.go:19-24`）被拆分成 `NodeRepository`、`NodeTrafficBatchRepository`、`NodeQuotaRepository`、`NodeConfigRepository` **4 个接口**，而调用方（`NodeService`、`ConfigService`）还不得不在运行时用 `type assertion` 判断扩展接口是否实现（如 `node_service.go` 与 `config_service.go:1581` 对 `NodeConfigRepository` 的断言）——既丧失编译期接口约束，又增加阅读复杂度。
   - **建议**：收敛为单一清晰的 `NodeRepository`（或最多拆分高频读写的 `NodeTrafficRepository`），移除运行时接口探测断言。
2. **编译器接口一分为二 (`node_service.go:38-42` vs `config_service.go:38-47`)**：
   - `RemoteNodeCompiler` 与 `RemoteNodeFullCompiler` 仅有一个真实实现 `*xray.XrayCompiler`，却分别在两个服务文件里定义局部接口并传递同一依赖。
   - **建议**：合并为统一编译端口或共享同一接口定义。
3. **跨文件包私有函数互调 (`node_service.go` ↔ `config_service.go`)**：
   - `node_service.go` 内定义了包私有 JSON 解析及中转抽取函数（`parseNodeOutbounds`/`parseNodeRouting`/`parseNodeDNS`/`extractStaticRelayClients`），由 `config_service.go` 跨文件调用，同时两服务各自持有互斥锁独立操作 `Node` 字段。
   - **建议**：收拢至统一的 Per-Node Config 聚合逻辑中（见第二部分架构收敛）。

### 1.4 临时文件、本地污染与测试冗余

1. **工作区未提交 `.trellis/.template-hashes.json` 污染**：
   - `git diff` 显示该文件当前包含 `+28/-1` 行未提交改动，其中 **20 条**为 `.trellis/scripts/common/__pycache__/*.cpython-311.pyc` 编译字节码哈希及 `.pi/*` 模板哈希。**禁止直接提交**，建议在提交业务前 `git checkout -- .trellis/.template-hashes.json` 或清理 `.pyc` 缓存再刷新模板哈希。
2. **81 处分散 `mock*` 测试桩 (`internal/**/_test.go`)**：
   - `git grep -n -E "^type mock" -- internal/` 共匹配 **81 个** `mock` 结构体（例如 `mockXrayManager`、`mockInboundRepo`、`mockUserRepo`、`mockSettingRepo`、`mockHostMonitor`、`mockLogReader`、`mockAdminRepo` 在多包/多文件重复定义），同时 `agent_e2e_test.go`（1,807 行）、`node_service_test.go`（1,097 行）、`runner_test.go`（895 行）单文件体量过大。
   - **建议**：核心领域测试保留各自最小桩，但对相同包内的重复 mock（如 `internal/service/` 下 `sub_service_test.go`、`config_service_test.go`、`user_service_test.go`、`alert_service_test.go`）和超长 E2E 测试按场景拆分（如 `agent_e2e_sync_test.go`、`agent_e2e_quota_test.go`、`agent_e2e_commands_test.go`），并为 HTTP 测试真实注入非空 `xrayManager` 消除类似 P0-1 的盲区。
3. **Git 历史中的 `web/dist` 构建噪点**：
   - 今日 `d23b104`、`5c9f559`、`b77f0de` 三次将 Vite 哈希产物直接提交到 Git（`.gitignore` 仅忽略 `/dist/` 而非 `web/dist/`），造成每次小修前端均伴随数十个 hashed `.js`/`.css` 文件增删。若确实因 `go:embed` 必须入库 `web/dist`，建议**仅在版本发布或 Milestone 结束统一重编提交一次**，避免功能提交混杂构建产物。

---

## 二、功能合并与架构收敛建议

### 2.1 用户视角（前端入口 / 心智模型收敛）

#### 建议 U-1：重塑“全局节点切换器”的作用域边界与视觉反馈
- **现状痛点**：
  - 顶栏与侧栏的全局 `useActiveNode` 切换器在**所有已登录路由**均常驻渲染（`App.vue:250, 389`），但 `/`（仪表盘）、`/users`（用户管理）、`/settings`（系统设置）、`/nodes`（节点管理）以及顶栏的“重启核心”按钮（`App.vue:732-735`）完全无视 `activeNodeId`，始终操作主控机 Master。用户在顶栏切到“日本东京节点”后点击“重启核心”或进入“用户管理”，极易误以为自己在操作东京节点。
  - 同时 `InboundsView.vue` 内既受顶栏 `activeNodeId` 联动，又额外保留了一套本地 `nodeFilter` 下拉框（`useInboundList.ts:244, 378`），形成“双切换器重叠”。
- **合并与边界方案**：
  - 将前端路由明确划分为 **Global/Master 域**（`/`, `/users`, `/nodes`, `/settings`）与 **Node-Scoped 域**（`/inbounds`, `/outbounds`, `/routing`, `/dns`, `/topology`, `/config`, `/logs`）；
  - 在非 Node-Scoped 页面将节点切换器弱化为“全局节点上下文（当前页为全局视图）”只读徽章或仅在 Node-Scoped 页面激活切换；
  - 将顶栏“重启核心”按钮当 `activeNodeId > 0` 时改为禁用并提示“远端节点通过配置下发自动热重载”或触发对应的节点状态同步，杜绝误重启主控机；
  - 移除 `InboundsView` 冗余的本地 `nodeFilter`（默认使用全局 `activeNodeId`，仅保留“显示全部节点入站”快捷开关）。
- **收益 / 风险 / 迁移成本**：
  - **收益**：彻底消除跨节点误操作风险；减少用户切换节点时的认知摩擦。
  - **风险与成本**：极低，仅需调整 `App.vue` 顶栏状态提示与 `useInboundList.ts` 过滤逻辑（约 0.5 人天）。

#### 建议 U-2：收敛碎片化的远端运维入口（合并至“节点详情工作台” + 重定位 GeoData）
- **现状痛点**：
  - 管理员排查/维护一台远端节点目前必须在 **5 个独立页面**间来回跳转：`NodesView`（状态/配额/安装脚本） -> `LogsView`（远端日志） -> `ConfigView`（远端编译 JSON 预览） -> `RoutingView`（远端 GeoData 下发！GeoData 规则更新与业务路由混在一个页面） -> `TopologyView`（拓扑编排）。
  - 同时 `useNodes.ts` 与 `useActiveNode.ts` 各自维护一套节点列表与轮询定时器（15s + 5s），数据不同步。
- **合并与边界方案**：
  1. **合并前端节点数据源**：废弃 `useNodes.ts` 中的独立 `nodes` 列表与 15s 定时器，统一从 `useActiveNode`（或重命名为 `useNodeStore`/`useNodeRegistry`）消费唯一响应式节点数组与同步状态，使 `/nodes` 操作（新建/编辑/摘流/恢复）即时反映到顶栏切换器；
  2. **增加“节点运维抽屉（Node Workbench Drawer）”**：在 `NodesView` 点击某节点“运维/详情”时，以抽屉 Tab 聚合「实时状态与配额」「快速日志（内联最近 100 行）」「编译配置预览」「GeoData 更新与安装脚本」；保留原有的全屏 `ConfigView`/`LogsView` 作为深度视图并一键直达；
  3. **把 GeoData 更新从 `RoutingView` 剥离**：将全局/节点级 GeoData 更新迁入 `NodesView`（节点运维入口）或 `SettingsView`（核心组件维护），让 `RoutingView` 回归纯粹的路由规则编辑。
- **收益 / 风险 / 迁移成本**：
  - **收益**：减少 50% 以上页面跳转与重复轮询请求；配置与运维边界清晰。
  - **风险与成本**：中等，前端组件重组（约 1~1.5 人天），后端 API 可完全兼容复用。

#### 建议 U-3：收敛 3 份静态 Portal HTML 为单源维护
- **现状痛点**：`deploy/cloudflare-pages/portal.html`、`deploy/cloudflare-pages/portal/index.html`、`deploy/standalone-portal.html` 完全相同却手动维护三份提交。
- **合并方案**：保留一份 canonical 模板，其余两处通过发布脚本/Makefile 自动复制或清理不必要冗余副本（确认 Cloudflare Pages 部署根目录规范后保留对应入口即可）。
- **收益 / 风险 / 迁移成本**：零业务风险，成本 < 0.2 人天。

---

### 2.2 工程架构视角（Service / Domain / API 收敛）

#### 建议 A-1：重构异步节点指令通道（合并 `pendingLogReqs` + `pendingGeoReqs` + 未来控制指令）
- **现状痛点**：
  - `NodeService` 在结构体上硬编码了两套独立但半成品的同步等待 map：`pendingLogReqs`（25s 超时 + 超时删除单据 bug -> P0-2）和 `pendingGeoReqs`（75s 超时 > HTTP WriteTimeout 60s + 阻塞 `SyncOnce` -> P1-1），每次新增一种远端动作都要重写一遍同步结构与 Agent 协议字段。
- **合并方案**：
  - 在 `NodeService` 内建立统一的轻量 **异步节点指令队列（`NodeCommand`：`log_tail` / `geodata_update` / `restart`）**，带 `commandId`、创建时间与 TTL（如 `180s`）：
    1. HTTP 管理员发起请求 -> 返回 `202 Accepted` + `commandId`（若已有进行中同类型未过期指令直接复用并返回，避免并发踩踏）；
    2. Agent 心跳拉取待执行 `commands`（不从 Wait 侧超时删除尚未下发的命令！），在独立 goroutine 执行耗时的 GeoData/日志动作；
    3. Agent 执行完毕后通过 `POST /api/agent/command-result`（或下次心跳携带结果）回填结果缓存；前端通过轻量轮询获取完成状态与负载。
- **收益 / 风险 / 迁移成本**：
  - **收益**：一并根治 **P0-2**（日志 25s 超时死循环）、**P1-1**（75s 超时撞 60s `WriteTimeout` 及阻塞 `SyncOnce`）、并发覆写竞态，同时为未来远程重启/测速/诊断提供唯一扩展点。
  - **风险与成本**：需要前后端日志/GeoData 交互与 Agent 协议做一次轻量异步化调整（保留旧响应兼容过渡），预计 1.5~2 人天，风险可控。

#### 建议 A-2：收拢 Per-Node 配置编译与规则规范化边界
- **现状痛点**：
  - 远端节点配置读取/编译/写入分散在 `ConfigService`、`NodeService` 与 `XrayCompiler` 三处，导致出现“UI 读取走了 `normalizeRoutingConfig` + `ensureSubRouteRules`，而 `getOrBuildSnapshot` 与 `GetNodeCompiledConfigRaw` 却跳过 `ensureSubRouteRules`”（P1-2），以及 `SaveRawConfig`/`RollbackSnapshot` 漏清节点缓存（P1-6）。
- **合并方案**：
  - 将“根据 `nodeID` 加载入站 + 解析 Outbound/Routing/DNS + 统一执行 `ensureSubRouteRules` 规范化 + 注入静态中转账号 + 编译 JSON”抽为唯一的共享纯管道函数/服务方法（如 `BuildNodeTargetConfig(ctx, nodeID)`），由 `NodeService`（生成 Snapshot 下发 Agent）与 `ConfigService`（返回 `ConfigView` 只读预览）共同调用；
  - 将 `node_repo.go` 拆分过细的 4 个接口收敛为 1~2 个清晰仓储契约，并将 `FindByNodeID` 正式写入 `domain.InboundRepository`。
- **收益 / 风险 / 迁移成本**：
  - **收益**：从架构根源上保证“UI 展示、Preview 预览、Agent 实际运行的 `config.json`”三者绝对一致（**single source of truth**），消灭 P1-2 与 P1-6。
  - **风险与成本**：低~中（已有完整单元测试与 E2E 兜底），约 1 人天。

#### 建议 A-3：收敛多节点下 `UserService` / `XrayManager` / `SubService` 语义边界
- **现状痛点**：
  - `xray.Manager` 不区分本机/远端入站（P0-1）；TG `/adduser` 无过滤选入远端入站；`sub_service.go` 对远端 `ExternalPort == 0` 沿用本机 `public_port` 兜底（P1-4）；`agent/quota_store.go` 重复拷贝 `domain` 配额公式。
- **合并方案**：
  - 在 `domain.Inbound` 增加纯方法 `IsLocal() bool { return i.NodeID == 0 }`，由 `xray.Manager`（`AddUser`/`RemoveUser`）与 TG `/adduser` 统一按 `IsLocal()` 决定是否操作本机 gRPC；
  - 在 `sub_service.go` 明确区分“本机入口端口回退”与“远端直连入站默认取自身 `Inbound.Port`”；
  - 让 `internal/agent/quota_store.go` 直接引用 `internal/domain` 纯计算函数。
- **收益 / 风险 / 迁移成本**：
  - **收益**：直接解决 P0-1、P1-4 并消除计费公式重复；迁移成本低（约 0.5 人天）。

---

## 三、推荐分步治理路线图（供后续立项参考）

| 阶段 | 目标 | 包含项 | 预估工作量 |
|---|---|---|---|
| **Phase A（止血修复，1 天）** | 消除所有 P0 与高确定性 P1 生产故障 | 修复 **P0-1**（`xray.Manager` 拦截 `NodeID != 0` + TG `/adduser` + E2E 非空回归测试）、**P1-2**（Snapshot & Preview 补全 `ensureSubRouteRules`）、**P1-3**（SS/SS-2022 Cipher 透传）、**P1-4**（远端直连 `ExternalPort == 0` 回退至 `inbound.Port`）、**P1-5**（`QuickPublishWizardModal` 传 `?nodeId`）、**P1-6**（Rollback/Raw 保存清缓存）、**P1-8**（安装脚本 `--token` 改用 `EnvironmentFile` 0600 + `--interval 60s`）与 24 个前端 Prettier 格式修正、清理 `.trellis/.template-hashes.json` 污染 | ~1~1.5d |
| **Phase B（异步指令与存储收口，1.5 天）** | 解决超时/阻塞与数据膨胀 | 实施 **A-1**（统一异步 `NodeCommand` 通道解决 **P0-2** & **P1-1**）+ **P1-7**（接入 `PruneBatchesBefore` 清理与 `DeleteNode` 级联删除）+ **P2-1**（写接口返回完整 `RefreshSyncStatus`） | ~1.5d |
| **Phase C（体验与冗余收敛，1.5 天）** | 提升可维护性与操作直觉 | 实施 **U-1/U-2/U-3**（作用域感知全局切换器、合并 `useNodes`/`useActiveNode`、Node Workbench 抽屉、GeoData 迁移、Portal 单源化）、**A-2**（仓储与编译接口 KISS 精简）、补全 `mock/index.ts` 与 `90s->180s` 文案统一 | ~1.5d |
