# M3: 前端节点交互收敛、Portal 单源化与仓储 KISS 精简

> **Parent Task**: `11-19-audit-today-stability-cleanup`
> **Ordering Dependency**: 依赖 M1（`459c8ab`）与 M2（`6e0ed5c`）均已归档完成。

## Goal
完成当日审查收尾里程碑：
1. 重塑前端全局节点切换器的作用域边界，防止跨节点误操作“重启核心”，移除 `InboundsView` 冗余本地节点下拉并打通 `TopologyView -> LogsView` 节点上下文（**U-1**）；
2. 合并 `useNodes` 与 `useActiveNode` 为单一响应式节点数据源，新增 `NodesView` 节点运维抽屉（整合状态配额 / 快速日志 / 编译预览 / GeoData 与部署），并将 GeoData 从 `RoutingView` 剥离至 `SettingsView`（本机）与节点运维抽屉（远端）（**U-2**）；
3. 收敛 3 份字节完全一致的静态 Portal HTML 为单源维护（**U-3**）；
4. 精简后端碎片化的 4 个 Node 仓储接口与双份远端编译器接口、抽取纯标准库 `internal/pkg/geodata` 消除 Master/Agent 镜像下载重复（**A-2 KISS**）；
5. 修复剩余 P2 体验瑕疵：`90s -> 180s` 心跳在线文案、Demo Mock `/admin/nodes` 全套路由、以及 Agent 离线物理网卡流量口径提示（**P2-2 / P2-3 / P2-4**）。

## Requirements

### R1. 全局节点切换器作用域边界、防误重启与入站/拓扑上下文对齐（U-1）
- **证据与锚点**：`web/src/router/index.ts:18-33`、`web/src/App.vue:141-148,219-281,337-346,369-443,694-743`、`web/src/views/InboundsView.vue:13-31,71-83,197-221`、`web/src/views/inbounds/composables/useInboundList.ts:244-262,348-381`、`web/src/views/TopologyView.vue:219-221`、`web/src/views/LogsView.vue:1599,1660,2134-2156`。
- **要求**：
  - 在 `web/src/router/index.ts` 为所有路由显式标注 `meta.nodeScoped`（7 个 Node-Scoped 路由：`/inbounds`、`/outbounds`、`/routing`、`/topology`、`/logs`、`/dns`、`/config` 为 `true`；其余全局/主控路由 `/`, `/users`, `/nodes`, `/settings` 为 `false`）。
  - `web/src/App.vue`：
    - 仅在 `isNodeScopedRoute` 时渲染可交互的顶栏/移动端节点切换器 `<select>`；在 Global/Master 页隐藏切换下拉并展示清晰的静态全局/主控标识，杜绝跨页误导。
    - 当处于 `isNodeScopedRoute && isRemoteNode` 上下文时，顶栏与侧栏的“重启核心”按钮置为禁用态并提供明确提示（`远端节点通过配置下发自动热重载，无需重启主控核心`），状态胶囊对应展示当前远端节点在线/同步状态而非本机 Master 核心状态。
  - `web/src/views/InboundsView.vue` & `useInboundList.ts`：
    - 移除冗余的本地节点下拉框 `<select v-model="nodeFilter">`；默认严格跟随全局 `activeNodeId`，仅保留一个轻量「全部节点」切换开关（`showAllNodes`，默认关闭），既消除双切换器割裂，又保留跨节点入站总览能力。
  - `web/src/views/TopologyView.vue` & `LogsView.vue`：
    - `TopologyView.viewLogs` 跳转 `/logs` 时在 `query` 中携带 `nodeId`（`String(activeNodeId.value)`）；
    - `LogsView.vue` 读取 `route.query.nodeId` 自动切换节点上下文，并在远端节点模式下收到 `?inbound=<tag>` 时自动填入 `remoteKeyword` 关键词过滤。

### R2. 合并节点状态源、新增节点运维抽屉与 GeoData 迁出 `RoutingView`（U-2 & P2-2）
- **证据与锚点**：`web/src/composables/useActiveNode.ts`、`web/src/views/nodes/composables/useNodes.ts`、`web/src/views/NodesView.vue:52,330-382,530-574,713-752`、`web/src/views/RoutingView.vue:49-200,1177-1398,1858-1885`、`web/src/views/SettingsView.vue:25-131`、`web/src/components/ui/Drawer.vue`。
- **要求**：
  - **单一节点状态源**：让 `useNodes.ts` 废弃局部 `nodes` ref 与独立 `listNodes()` 缓存，统一读写 `useActiveNode.ts` 的响应式 `nodesList`；在 `NodesView` 的增删改/启停/轮换 Token/重置流量后即时刷新共享状态并收敛重复轮询定时器；将 `NodesView.vue:52` 文案由 `在线心跳 (90s 内)` 修正为 `在线心跳 (180s 内)`（与后端 `domain/node.go:212` 一致）。
  - **节点运维抽屉 (`NodeOpsDrawer.vue`)**：基于通用 `Drawer.vue` 新建节点运维工作台抽屉，聚合 4 个 Tab：
    1. `状态与配额`（在线/延迟/版本、Xray vs 物理 NIC 周期流量、配额阈值与重置流量/摘流操作，附离线物理网卡口径说明）；
    2. `快速日志`（调用 M2 异步 `GET /logs?nodeId=` + `2s` 轮询 `getNodeCommandStatus` 查看最近 100 行 `xray`/`xray-agent` 日志 + 一键跳转完整 `/logs`）；
    3. `编译预览`（调用 `GET /config/raw?nodeId=` 展示只读编译 JSON + 一键复制 + 一键跳转完整 `/config`）；
    4. `GeoData 与部署`（调用 M2 异步 `POST /geodata/update?nodeId=` + `2s` 轮询更新远端 GeoData + 一键打开安装脚本/轮换 Token）。
    - 抽屉关闭、切换目标 `node.id` 与组件卸载时必须清理全部轮询定时器并防止旧响应覆写。
  - **精简 `NodesView.vue` 操作列**：桌面与移动端统一以「运维」主入口（打开 `NodeOpsDrawer`）+「编辑」「启/停」「删除」精简展示，补齐移动端启/停操作。
  - **GeoData 从 `RoutingView` 迁出**：
    - 移除 `RoutingView.vue` 的 GeoData 卡片、状态与轮询逻辑，使 `RoutingView` 回归纯粹的路由规则编辑；
    - 将**本机 Master GeoData 维护模块**（`GET /geodata/status`、`POST /geodata/update`、`GET /geodata/progress` 500ms 轮询）迁入 `SettingsView.vue` ① 区紧邻 `xray_geodata_dir` 配置处，保存设置后自动刷新状态并在 `onUnmounted` 清理定时器。

### R3. 静态 Portal HTML 单源化（U-3）
- **证据与锚点**：`deploy/cloudflare-pages/portal.html`、`deploy/cloudflare-pages/portal/index.html`、`deploy/standalone-portal.html`（三者 SHA-256 完全一致）、`deploy/cloudflare-pages/_worker.js:223-232`、`deploy/gateway.test.js`、`docs/ARCHITECTURE_AND_LOGIC.md:229`。
- **要求**：
  - 保留 `deploy/cloudflare-pages/portal/index.html` 作为唯一静态 Portal HTML 权威源，删除冗余副本 `deploy/cloudflare-pages/portal.html` 与 `deploy/standalone-portal.html`。
  - 在 `deploy/cloudflare-pages/_worker.js:223-232` 将 `/portal`、`/portal/`、`/portal.html`、`/portal/index.html` 统一改写目标路径为 `/portal/` 后调用 `env.ASSETS.fetch(new Request(targetUrl, request))`（避免 CF Pages Pretty URLs 308 循环）。
  - 在 `deploy/gateway.test.js` 新增测试覆盖 4 条 `/portal*` 路径转发与单源静态文件存在性校验；同步更新 `docs/ARCHITECTURE_AND_LOGIC.md:229`。

### R4. 后端接口 KISS 精简与 `internal/pkg/geodata` 共享下载原语（A-2 KISS）
- **证据与锚点**：`internal/domain/repository.go:113-141`、`internal/service/node_service.go:39-42,175,967`、`internal/service/config_service.go:38-47,1621,1628`、`internal/service/alert_service.go:55`、`internal/delivery/cron/traffic_sync.go:37,80`、`internal/service/geodata_service.go`、`internal/agent/geodata_updater.go`。
- **要求**：
  - 将 `NodeTrafficBatchRepository`、`NodeQuotaRepository`、`NodeConfigRepository` 并入单一 `domain.NodeRepository`，删除 3 个碎片化接口及全库 3 处运行时 `type assertion`（`node_service.go`、`alert_service.go`、`config_service.go`）；简化 `NewNodeService` 与 `AlertService.WithNodeQuota` 注入签名并同步更新 `main.go` 与测试 mock。
  - 将 `CompileRemoteNodeFullToJSON` 并入 `RemoteNodeCompiler`（或统一编译器契约），删除 `config_service.go` 中分裂的 `RemoteNodeFullCompiler` 与 2 处运行时 `type assertion`。
  - 新增纯标准库包 `internal/pkg/geodata` 封装共享的 `DefaultAssetsDir`、`DefaultGeoIPURLs`、`DefaultGeoSiteURLs`、`DefaultMinFileSize`（100KB）、`NewHTTPClient` 与带 `ctx`/进度回调/`minSize` 校验的原子下载原语，供 `internal/service/geodata_service.go`（顺便补齐 100KB 损坏页拦截与 `ctx` 取消）与 `internal/agent/geodata_updater.go` 共用，同时保持 `cmd/agent` 零重型依赖。

### R5. Demo Mock 补齐与离线网卡口径提示（P2-3 & P2-4）
- **证据与锚点**：`web/src/mock/storage.ts:3-14`、`web/src/mock/index.ts:1085-1185`、`web/src/views/nodes/components/NodeFormDrawer.vue:229-231`、`web/src/views/NodesView.vue:238-271`。
- **要求**：
  - 在 `web/src/mock/storage.ts` 预置演示 `nodes` 数据，在 `web/src/mock/index.ts` 补全 `/admin/nodes` 列表/详情/增删改/`rotate-token`/`reset-traffic`/`install-script` 全套 Mock 路由。
  - 在 `NodeFormDrawer.vue` 配额熔断策略区与 `NodeOpsDrawer.vue` 状态配额区补充清晰提示：节点离线期间 Agent 使用物理网卡增量兜底统计（可能包含宿主机非 Xray 流量），上线后自动切回 Xray 精确统计口径。

## Out of Scope
- 不改变任何后端 HTTP REST API 路径与数据库 Schema；
- 不新增远端节点重启核心 API（远端重启仍由配置下发自动热重载承担）。

## Acceptance Criteria
1. **AC-1 (U-1)**：在 `/`, `/users`, `/nodes`, `/settings` 不显示可切换的远端节点下拉框；在 Node-Scoped 页面切换至远端节点时，顶栏/侧栏“重启核心”被禁用并提示；`InboundsView` 无冗余本地节点下拉（仅保留「全部节点」轻量开关）；`TopologyView` 跳 `/logs` 携带 `nodeId` 且远端模式自动带入 `remoteKeyword`。
2. **AC-2 (U-2 & P2-2)**：`useNodes` 共享 `useActiveNode.nodesList`（在 `/nodes` 新增/编辑/启停/删除节点后顶栏切换器立即同步）；`NodesView` 心跳文案为 `180s 内`；`NodeOpsDrawer` 4 个 Tab 正常工作且关闭/切换/卸载零定时器泄漏；`RoutingView` 无 GeoData 卡片，本机 GeoData 维护位于 `SettingsView` ① 区正常触发与轮询。
3. **AC-3 (U-3)**：`deploy/cloudflare-pages/portal.html` 与 `deploy/standalone-portal.html` 已删除，仅留 `deploy/cloudflare-pages/portal/index.html`；`node --test deploy/gateway.test.js` 全绿（含 4 条 `/portal*` 路径改写断言）。
4. **AC-4 (A-2 KISS)**：`domain/repository.go` 仅保留单一 `NodeRepository`；全库零 `NodeQuotaRepository`/`NodeConfigRepository`/`NodeTrafficBatchRepository`/`RemoteNodeFullCompiler` 残留与运行时类型断言；`internal/pkg/geodata` 被 Master 与 Agent 共用且带单测覆盖；`cmd/agent` 依赖纯净（0 匹配 `gin|gorm|sqlite|telegram`）。
5. **AC-5 (P2-3 & P2-4)**：Mock 模式下 `/nodes` 增删改查/轮换 Token/重置流量/安装脚本与运维抽屉均正常；配额表单与运维抽屉展示清晰离线物理网卡流量口径提示。
6. **AC-6 (全量门禁)**：
   - `mise x -- go test -count=1 ./...`
   - `CGO_ENABLED=1 mise x -- go test -race -count=1 ./...`
   - `mise x -- go vet ./...`
   - `mise x -- go build -o NUL .` & `mise x -- go build -o NUL ./cmd/agent`
   - `go list -deps ./cmd/agent | Select-String "gin|gorm|sqlite|telegram"` 输出为空
   - `mise x -- node --test deploy/gateway.test.js`
   - `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run format:check && mise x -- npm run build`
