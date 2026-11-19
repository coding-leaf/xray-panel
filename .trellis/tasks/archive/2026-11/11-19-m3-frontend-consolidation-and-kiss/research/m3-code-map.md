# Research: M3 Frontend Consolidation, Portal Single-Source, Backend KISS & P2 Tail Items

- **Query**: M3 涉及的前端、后端及部署文件现状只读调研（U-1 / U-2 / U-3 / A-2 / P2-2 / P2-3 / P2-4）
- **Scope**: `web/src/**`, `deploy/**`, `internal/domain/**`, `internal/service/**`, `internal/agent/**`, `internal/delivery/**`, `internal/adapter/**`, `main.go`
- **Date**: 2026-11-19

---

## 1. U-1：全局节点切换器作用域边界 & `InboundsView` 冗余筛选

### 1.1 `web/src/App.vue` 节点切换器与顶栏“重启核心”逻辑

- **全局节点切换器渲染位置**（均受 `v-if="isAuthenticated"` 控制，即登录态且非 `/login`、`/portal` 空白布局）：
  - **桌面端顶栏**：`web/src/App.vue:219-281`（`<select :value="activeNodeId" @change="onNodeSelectChange">` 含 `<option :value="0">🖥 本机 Master</option>` 与 `nodesList` 遍历；`:269-280` 附带 `activeNodeSyncBadge` 三态同步徽标）。
  - **移动端顶栏**：`web/src/App.vue:369-417`（同款 `<select>` + `activeNodeSyncBadge`）。
  - **现状问题**：切换器在**所有**已认证路由均保持可见，未区分当前路由是否支持远端节点上下文（如 `/`、` /users`、`/settings`、`/nodes` 均无远端视角）。
- **顶栏/抽屉“重启核心”按钮与逻辑**：
  - 移动端侧边抽屉：`web/src/App.vue:141-148`（按钮文案 `重启` / `...`，`@click="restartCore"`）。
  - 桌面端顶栏：`web/src/App.vue:337-346`（按钮文案 `重启核心` / `重启中...`，`@click="restartCore"`）。
  - 移动端顶栏：`web/src/App.vue:434-443`（图标按钮，`@click="restartCore"`）。
  - 处理函数 `restartCore`（`web/src/App.vue:732-743`）：
    ```ts
    const restartCore = async () => {
      restarting.value = true
      try {
        await api.post('/service/restart')
        toast.success('Xray 核心已重启')
        await fetchCoreStatus()
      } catch (err: any) {
        toast.error('重启失败：' + err)
      } finally {
        restarting.value = false
      }
    }
    ```
  - **现状缺陷**：
    1. `restartCore` 与 `pollCoreStatus`（`web/src/App.vue:694-708`，`GET /service/status`）**无条件操作本机 Master**（不携带 `nodeId`，后端 `router.go:177` `POST /service/restart` 亦无 `nodeId` 参数）；
    2. 当 `isRemoteNode === true` 时，顶栏核心状态胶囊与“重启核心”按钮仍展示并重启**本机 Master**，容易误导操作者以为在重启当前选中的远端节点（同款本机按钮也存在于 `DashboardView.vue:346-350,426-438`）。

### 1.2 全部路由清单与作用域分类

来自 `web/src/router/index.ts:18-33` 与导航配置 `web/src/App.vue:624-654`：

| 路由 Path | 组件 | 导航分组 | 作用域分类 | 节点上下文现状 (`useActiveNode` / `nodeId`) |
|---|---|---|---|---|
| `/login` | `LoginView` | - | Blank (无壳) | 不渲染切换器 |
| `/portal` | `PortalClaimView` | - | Blank (无壳) | 不渲染切换器 |
| `/` | `DashboardView` | OVERVIEW (`运行监控`) | **Global / Master-only** | 未使用 `useActiveNode`，仅请求本机 `/dashboard` 与 `/service/restart` |
| `/users` | `UsersView` | RESOURCES (`用户与订阅`) | **Global** (跨节点用户/订阅聚合) | 未使用 `useActiveNode` |
| `/nodes` | `NodesView` | RESOURCES (`节点管理`) | **Global** (远端节点总表) | 当前使用局部 `useNodes()`，未消费 `useActiveNode` |
| `/settings` | `SettingsView` | SYSTEM (`系统设置`) | **Global / Master-only** | 未使用 `useActiveNode`，管理本机 `xray_*` 路径、订阅、TG、密码、2FA |
| `/inbounds` | `InboundsView` | RESOURCES (`入站网关`) | **Node-Scoped** | 消费 `useActiveNode`，但 `fetchAll` 请求全量 `/inbounds` 后用 `nodeFilter` 本地过滤 |
| `/outbounds` | `OutboundsView` | RESOURCES (`出站出口`) | **Node-Scoped** | `OutboundsView.vue:64,77` 消费 `useActiveNode`，请求 `/outbounds?nodeId=${nodeId}` |
| `/routing` | `RoutingView` | RESOURCES (`路由分流`) | **Node-Scoped** | `RoutingView.vue:1168,1445-1450` 消费 `useActiveNode`，请求 `/routing?nodeId=...` 等 |
| `/topology` | `TopologyView` | OPERATIONS (`线路与拓扑`) | **Node-Scoped** | `TopologyView.vue:140,379` 消费 `useActiveNode`，请求 `/inbounds|outbounds|routing?nodeId=...` |
| `/logs` | `LogsView` | OPERATIONS (`运行日志`) | **Node-Scoped** | `LogsView.vue:1443` 消费 `useActiveNode`，`nodeId=0` 查本机访问/错误/审计日志，`nodeId>0` 下发远端 `log_tail` 异步命令 |
| `/dns` | `DNSView` | SYSTEM (`DNS 设置`) | **Node-Scoped** | `DNSView.vue:242,259` 消费 `useActiveNode`，请求 `/dns?nodeId=${nodeId}` |
| `/config` | `ConfigView` | SYSTEM (`配置编辑`) | **Node-Scoped** | `ConfigView.vue:200,219` 消费 `useActiveNode`，请求 `/config/raw?nodeId=${nodeId}`（远端只读） |

### 1.3 `InboundsView.vue` 与 `useInboundList.ts` 中 `nodeFilter` 与 `activeNodeId` 联动位置

- **`web/src/views/inbounds/composables/useInboundList.ts`**：
  - `:234-235`：解构 `activeNodeId, activeNode, isRemoteNode, nodesList, loadNodes, setActiveNode` from `useActiveNode()`。
  - `:244`：`const nodeFilter = ref(String(activeNodeId.value))`（初始绑定全局节点）。
  - `:256-262`：`filteredInbounds` 计算属性按 `nodeFilter.value !== 'all'` 对 `(ib.nodeId || 0) !== targetNodeId` 进行过滤。
  - `:348-353`：`fetchAll()` 调用 `api.get('/inbounds')`（**未传 `?nodeId=`**，拉取全部节点入站供 `'all'` 模式使用），同时拉取 `api.get('/outbounds?nodeId=${activeNodeId.value}')`。
  - `:378-381`：`watch(activeNodeId, (newId) => { nodeFilter.value = String(newId); fetchOutboundsForNode(newId) })`（全局切换器变更时覆盖本地 `nodeFilter` 并重载出站）。
- **`web/src/views/InboundsView.vue`**：
  - `:13-31`：标题栏徽标 —— `v-if="nodeFilter === 'all'"` 显示“全局筛选：全部节点”与“切回当前节点”按钮（`:20` `@click="restoreActiveNodeFilter"`）；`v-else-if="isRemoteNode"` 显示当前远程节点徽标。
  - `:71-83`：筛选栏独立下拉框 `<select v-model="nodeFilter" @change="onNodeFilterChange">`，包含 `全部节点` (`all`)、`本机 Master` (`0`) 与 `nodesList`。
  - `:143`：`InboundFormDrawer` 传入 `:default-node-id="activeNodeId"`（表单内 `InboundFormDrawer.vue:1145` 也单独调了 `useActiveNode()`）。
  - `:197-205`：
    - `onNodeFilterChange()`：若 `nodeFilter.value !== 'all'`，反向调用 `setActiveNode(Number(nodeFilter.value))` 修改全局节点；
    - `restoreActiveNodeFilter()`：重置 `nodeFilter.value = String(activeNodeId.value)`。
  - `:212-221`：`viewLogs(inb)` 先调用 `setActiveNode(Number(inb.nodeId || 0))`，再跳转 `/logs?type=access&inbound=...`。
  - `:281-284`：`watch(activeNodeId, ...)` 关闭详情抽屉并清空 `selectedInbound`。

### 1.4 `TopologyView.vue` 跳转日志与节点上下文传递

- **准确行号**：`TopologyView.vue` 的 `viewLogs` 位于 **`web/src/views/TopologyView.vue:219-221`**（注：第 `259` 行实际处于 `toggleChannelStatus` 的 `promises.push(...)` 中）：
  ```ts
  const viewLogs = (tag: string) => {
    router.push({ path: '/logs', query: { type: 'access', inbound: tag } })
  }
  ```
- **上下文保持机制与缺口**：
  - `activeNodeId` 存于 `useActiveNode` 模块级单例 `ref` + `localStorage('xray_panel_active_node_id')`，因此从 `TopologyView` 跳转到 `/logs` 时**内存态 `activeNodeId` 会自动保留**；
  - **缺口 1（URL 不含节点上下文）**：`router.push` 的 `query` 未携带 `nodeId`（`LogsView.vue:1660` 仅 watch `[route.query.type, route.query.inbound]`），若直接分享/刷新包含特定节点日志的 URL 或跨标签打开，无法还原节点作用域；
  - **缺口 2（远端节点下的无效 query）**：当 `isRemoteNode.value === true` 时，`LogsView.vue:1599` 在 `fetchLogs()` 首行直接 `return`（不执行本地 access log 过滤），`LogsView.vue:2239` 在 `onMounted` 跳过 `fetchInbounds()`；远端日志必须用户手动点击 `fetchRemoteLogs()`（`LogsView.vue:2134-2156`）拉取 `xray` / `xray-agent` journalctl 文本流，此时 `?inbound=<tag>` 不会自动预填到 `remoteKeyword` 或触发自动拉取。

---

## 2. U-2：合并 `useNodes` / `useActiveNode`、节点运维抽屉、GeoData 迁移

### 2.1 `useActiveNode.ts` 与 `useNodes.ts` 现状对比与全量调用方

| 维度 | `web/src/composables/useActiveNode.ts` | `web/src/views/nodes/composables/useNodes.ts` |
|---|---|---|
| **作用域** | 模块级单例（跨页共享 `activeNodeId` 与 `nodesList`） | 每次调用新建局部 `ref`（仅 `NodesView.vue` 使用） |
| **核心状态** | `activeNodeId` (读自 `localStorage`), `nodesList`, `nodesLoading`, `subscriberCount`, `isDocumentHidden`, `activeNode` (computed), `isRemoteNode` (computed) | `nodes`, `loading`, `resettingId`, `searchQuery`, `stateFilter`, `onlineFilter`, `summary` (computed), `filteredNodes` (computed) |
| **定时器 / 同步机制** | `inflightLoad` 去重 + `loadSeq` 防乱序；`PENDING_SYNC_POLL_MS = 5000` 定时轮询（仅当 `subscriberCount > 0 && !isDocumentHidden && activeNode sync === 'pending'` 时启动）+ `visibilitychange` 订阅计数管理 | 无内置定时器（轮询逻辑写在 `NodesView.vue:713-752`，`15000ms` + `visibilitychange` + `pollInFlight`） |
| **导出方法** | `loadNodes(force?)`, `refreshNodesNow()`, `setActiveNode(id)` | `fetchNodes(silent?)`, `removeNode(node)`, `toggleNodeEnabled(node)`, `rotateToken(node)`, `handleResetTraffic(node)`；外加 5 个纯函数：`getDesiredStateBadge`, `getAutoStateReasonBadge`, `getNodeQuotaStats`, `formatLastSeen`, `isBoundInboundsError` |
| **数据重叠问题** | 调用 `listNodes()` 更新 `nodesList` | 调用 `listNodes()` 更新 `nodes` —— `NodesView` 增删改查后**不会自动刷新顶栏全局 `nodesList`**，存在双份缓存分叉 |

**全库 `import` 调用方清单**（`git grep -n -E "useActiveNode|useNodes" web/src`）：

1. `web/src/App.vue:516,531` — `useActiveNode()` (`activeNodeId, nodesList, activeNode, isRemoteNode, loadNodes, refreshNodesNow, setActiveNode`)
2. `web/src/views/InboundsView.vue`（经 `useInboundList.ts`）— `useInboundList.ts:10,235` (`activeNodeId, activeNode, isRemoteNode, nodesList, loadNodes, setActiveNode`)
3. `web/src/views/inbounds/components/InboundFormDrawer.vue:1125,1145` — `useActiveNode()` (`activeNodeId`)
4. `web/src/views/OutboundsView.vue:54,64` — `useActiveNode()` (`activeNodeId, activeNode, isRemoteNode`)
5. `web/src/views/RoutingView.vue:1122,1168` — `useActiveNode()` (`activeNodeId, activeNode, isRemoteNode, refreshNodesNow`)
6. `web/src/views/TopologyView.vue:134,140-141` — `useActiveNode()` (`activeNodeId, activeNode, isRemoteNode, nodesList, loadNodes, refreshNodesNow`)
7. `web/src/views/topology/components/OutboundNodeCard.vue:108,119` — `useActiveNode()` (`activeNodeId, nodesList`)
8. `web/src/views/topology/components/TopologyPoolsView.vue:356,366` — `useActiveNode()` (`activeNodeId, nodesList`)
9. `web/src/views/LogsView.vue:1439,1443` — `useActiveNode()` (`activeNodeId, activeNode, isRemoteNode`)
10. `web/src/views/DNSView.vue:240,242` — `useActiveNode()` (`activeNodeId, activeNode, isRemoteNode`)
11. `web/src/views/ConfigView.vue:198,200` — `useActiveNode()` (`activeNodeId, activeNode, isRemoteNode`)
12. `web/src/views/NodesView.vue:633-634,653` — `useNodes()`（唯一调用方）

### 2.2 `NodesView.vue` 操作列、P2-2 文案偏差与可复用 `Drawer` 组件

- **操作列现状**（已存在“按钮过密”问题）：
  - **桌面端操作列**（`web/src/views/NodesView.vue:330-382`）：每行直接平铺 **6 个按钮**（`安装脚本`、`轮换 Token`、`重置周期流量`、`编辑`、`停用/启用`、`删除`）。
  - **移动端卡片操作栏**（`web/src/views/NodesView.vue:530-574`）：平铺 **5 个按钮**（`安装脚本`、`轮换 Token`、`重置周期流量`、`编辑`、`删除`，缺少桌面端的 `停用/启用` 按钮）。
- **P2-2 心跳在线阈值文案偏差**：
  - `web/src/views/NodesView.vue:52`：KPI 卡片标题写为 **`在线心跳 (90s 内)`**；
  - **后端事实源**（`internal/domain/node.go:148,212`）：
    - `:148` 注释：`// 动态判定: LastSeenAt 距今 <= 180s`
    - `:212` 代码：`n.Online = now.Sub(*n.LastSeenAt) <= 180*time.Second`
  - **结论**：前端 `90s` 文案与后端 `180s` 判定窗口不一致，需修正为 `180s`（或 `3 分钟`）。
- **现有弹窗 / 抽屉组件**：
  - `web/src/views/nodes/components/NodeFormDrawer.vue`（新建/编辑节点表单抽屉，基于 `Drawer.vue`）；
  - `web/src/views/nodes/components/NodeTokenModal.vue`（一次性 Token 展示、Master URL 与 systemd 安装脚本弹窗，基于 `Modal.vue`）。
- **可复用通用 `Drawer` 原语**：`web/src/components/ui/Drawer.vue:1-154` 已存在并可直接使用：
  - Props：`modelValue: boolean`, `title?: string`, `description?: string`, `width?: string`（默认 `'w-full sm:max-w-xl md:max-w-2xl'`）；
  - Slots：`default`（主体滚动区）、`footer`（底部安全区操作栏）；
  - 内置 `Teleport to="body"`、遮罩点击关闭、`Esc` 键关闭、`role="dialog"` / `aria-modal="true"` / `aria-labelledby`、`document.body.style.overflow` 锁定与焦点恢复（`restoreFocus`）。

### 2.3 `RoutingView.vue` GeoData 逻辑与 `SettingsView.vue` 结构（GeoData 迁移边界评估）

- **`RoutingView.vue` 中 GeoData 的全部组成**：
  - **模板 UI**：`web/src/views/RoutingView.vue:49-200`（单张卡片，无额外 GeoData 弹窗；`RoutingView.vue:986-1097` 为 `PresetModal` 与 `ReorderModal`，与 GeoData 无关）。
    - `:50-114`：卡片头（区分 `isRemoteNode` 显示远端按需更新按钮 `获取最新 GeoData`，否则显示本机 `geoip.dat` / `geosite.dat` 大小、`targetDirectory`、`platform` 及 `一键升级规则库` 按钮）。
    - `:117-168`：远端节点更新状态提示条（`queued` / `dispatched` / 成功体积与耗时 / 失败报错）。
    - `:170-199`：本机 Master 实时下载进度条（`geoProgress.percentage` + `geoProgress.message`）。
  - **状态与逻辑**：`web/src/views/RoutingView.vue:1121,1177-1179,1186-1398,1858-1885`：
    - `geodataStatus = ref<any>(null)`、`updatingGeo = ref(false)`、`remoteGeoResult = ref<RemoteGeoUpdateResult | null>(null)`、`geoProgress = ref(...)`；
    - 本机方法：`fetchGeoStatus()` (`GET /geodata/status`)、`pollGeoProgress()` (`GET /geodata/progress`，`500ms` 轮询、`progressInFlight` 防重入，完成后 `setTimeout` 4s 清零并刷新 `fetchGeoStatus()`)、`updateGeoData()` 本地分支 (`POST /geodata/update`)；
    - 远端方法：`startRemoteGeoPolling(targetNodeId, commandId, reqSeq)` (`getNodeCommandStatus` 2000ms 轮询、`remoteGeoPollInFlight` 防重入 + `remoteGeoSeq` + `isUnmounted` 守卫，完成后调用 `refreshNodesNow()`)、`updateGeoData()` 远端分支 (`POST /geodata/update?nodeId=${targetNodeId}`)；
    - 生命周期清理：`watch(activeNodeId)` (`:1858-1870`)、`onMounted` (`:1874`)、`onUnmounted` (`:1877-1885`)。
- **`SettingsView.vue` 现有分段结构**（`web/src/views/SettingsView.vue:24-634`）：
  1. `① Xray 核心解耦与 Systemd 托管环境`（`:25-131`）— **包含 `settings.xray_geodata_dir` 输入框（`:99-110`）**；
  2. `② 面板公网访问与订阅地址配置`（`:134-206`）；
  3. `③ 订阅导出与客户端展示配置`（`:209-378`）；
  4. `④ Telegram 机器人运维与告警配置`（`:381-426`）；
  5. `⑤ 管理员密码安全修改`（`:429-464`）；
  6. `⑥ TOTP 双因素身份认证 (2FA)`（`:467-502` + `:504-633` 两个弹窗）。
- **迁移边界与依赖评估**：
  - **本机 GeoData 维护 -> `SettingsView.vue`**：
    - `RoutingView` 的本机 GeoData 卡片与轮询逻辑（`GET /geodata/status`、`POST /geodata/update`、`GET /geodata/progress`）完全不依赖路由规则状态，迁移至 `SettingsView.vue`（置于 ① 区之后或作为 ① 区子块，紧邻 `xray_geodata_dir` 配置）在语义和依赖上零耦合；注意保存 `xray_geodata_dir` 后可触发一次 `fetchGeoStatus()` 刷新路径展示，并在 `onUnmounted` 清理 `progressTimer`。
  - **远端 GeoData 维护 -> `NodesView.vue` 节点运维抽屉 (`NodeOpsDrawer.vue`)**：
    - 远端 GeoData 下发依赖 `node.id`、`POST /geodata/update?nodeId=<id>`、及 `getNodeCommandStatus(nodeId, commandId)` 轮询；将其放入 `NodesView` 运维抽屉后，不再受顶栏全局 `activeNodeId` 切换影响，抽屉只需绑定目标 `node` 对象并在关闭/切换节点/卸载时清理轮询定时器即可。
  - **后端接口无需改动**：`GET /api/geodata/status`、`GET /api/geodata/progress`、`POST /api/geodata/update?nodeId=...`、`GET /api/admin/nodes/:id/commands/:commandId` 均已就绪。

---

## 3. U-3：静态 Portal HTML 单源化

### 3.1 文件一致性核验

通过 `Get-FileHash` 验证以下 3 个文件 **SHA-256 完全一致**（逐字节相同）：

| 文件路径 | SHA-256 | 角色 |
|---|---|---|
| `deploy/cloudflare-pages/portal.html` | `A2BB7D6299B0A3E1D8BAE3F0DCE1AD5A67384C2558CE04A8FD469AEA2443C666` | Cloudflare Pages `/portal.html` 与 `/portal` 静态资源 |
| `deploy/cloudflare-pages/portal/index.html` | `A2BB7D6299B0A3E1D8BAE3F0DCE1AD5A67384C2558CE04A8FD469AEA2443C666` | Cloudflare Pages `/portal/` 与 `/portal/index.html` 静态资源 |
| `deploy/standalone-portal.html` | `A2BB7D6299B0A3E1D8BAE3F0DCE1AD5A67384C2558CE04A8FD469AEA2443C666` | 独立单文件提取门户副本（供任意静态托管使用） |

### 3.2 全库引用检查 (`git grep`)

- `deploy/cloudflare-pages/_worker.js:223-232`：
  ```js
  if (
    url.pathname === "/portal" ||
    url.pathname === "/portal/" ||
    url.pathname === "/portal.html" ||
    url.pathname === "/portal/index.html"
  ) {
    if (env?.ASSETS) {
      return env.ASSETS.fetch(request);
    }
  }
  ```
- `docs/ARCHITECTURE_AND_LOGIC.md:229`：同时提及 `deploy/cloudflare-pages/portal.html` 与 `deploy/standalone-portal.html`（无其他代码/CI/Dockerfile/wrangler/install.sh 引用这 3 个路径；`.github/`、`deploy/install.sh` 均未引用）。
- **单源化评估**：
  - `deploy/cloudflare-pages/_worker.js` 在收到 `/portal`、`/portal/`、`/portal.html`、`/portal/index.html` 时直接透传 `env.ASSETS.fetch(request)`。
  - 若要消除 3 份手写 HTML 副本，同时兼容 Cloudflare Pages 全部 4 个入口路径以及独立部署场景：
    - **方案 A（推荐，纯静态构建零依赖）**：以 `deploy/standalone-portal.html`（或 `deploy/cloudflare-pages/portal.html`）作为唯一权威源码；在 `_worker.js:223-232` 将 `/portal`、`/portal/`、`/portal.html`、`/portal/index.html` 统一改写 URL pathname 到单一静态文件（例如 `/portal.html`）后调用 `env.ASSETS.fetch(new Request(targetUrl, request))`，即可删除 `deploy/cloudflare-pages/portal/index.html`（以及多余副本），并通过 `deploy/gateway.test.js` 校验资产路由与同步一致性（注意若保留 `deploy/standalone-portal.html` 作为独立部署说明入口，可由 `portal.html` 构建/软链或直接以 `deploy/cloudflare-pages/portal.html` 作为唯一文件并在文档中指明路径）。
    - **注意**：需核查 Cloudflare Pages 对 `_worker.js` 中 `env.ASSETS.fetch` 的静态路由行为（如 `/portal.html` 在 CF Pages 默认 Pretty URLs 下是否返回 308 重定向到 `/portal`，若是则保留 `portal/index.html` 或用 `portal.html` 需实测或避免重定向环）。最稳妥且零 CF 行为风险的做法：保留一份权威文件 + `deploy/gateway.test.js` 增加字节一致性/内容断言（若需移除副本则必须验证 `_worker.js` 的 `ASSETS.fetch` 不触发重定向循环）。

---

## 4. A-2 KISS：后端接口精简

### 4.1 `NodeRepository` 4 个接口的定义与全库引用

定义位置：`internal/domain/repository.go:113-141`：
1. `NodeRepository`（`:114-123`）：`Create`, `Update`, `UpdateHeartbeat`, `Delete`, `GetByID`, `GetByTokenHash`, `ListAll`, `CountInbounds`
2. `NodeTrafficBatchRepository`（`:126-129`）：`ApplyNodeTrafficBatch`, `PruneBatchesBefore`
3. `NodeQuotaRepository`（`:132-136`）：`UpdateAutoState`, `ResetCycleTraffic`, `ListRecentDailyTraffic`
4. `NodeConfigRepository`（`:139-141`）：`UpdateNodeConfig`

**唯一持久化实现**：`internal/adapter/repository/node_repo.go:16-24` (`*GORMNodeRepository` 同时实现上述 4 个接口)。

**生产代码引用点与运行时 `type assertion`**：

| 位置 | 引用方式 | 具体行为 |
|---|---|---|
| `main.go:71,128,131,157,160` | 构造注入 | 创建单个 `nodeRepo := repository.NewNodeRepository(db)`，分别传给 `NewNodeService(nodeRepo, nodeRepo, ...)`、`ConfigService.WithNodeRepo(nodeRepo)`、`SubService.WithNodeRepo(nodeRepo)`、`AlertService.WithNodeQuota(nodeRepo, nodeRepo)`（注意 `main.go` 暂未将 `nodeRepo` 注入 `TrafficSyncJob.WithNodeBatchRepo`，Wait: 需确认 `main.go:180+` 是否有 `WithNodeBatchRepo`） |
| `internal/service/node_service.go:129-131,153-154,175-185` | 字段 + **type assertion** | 持有 `nodeRepo domain.NodeRepository`、`batchRepo domain.NodeTrafficBatchRepository`、`quotaRepo domain.NodeQuotaRepository`；`NewNodeService` 内部执行 `if qr, ok := nodeRepo.(domain.NodeQuotaRepository); ok { svc.quotaRepo = qr }` (`:175`)，并提供 `WithQuotaRepo` (`:182`) |
| `internal/service/alert_service.go:33-34,51-59` | 字段 + **type assertion** | 持有 `nodeRepo` 与 `quotaRepo`；`WithNodeQuota` 若未传可选参数则执行 `if qr, ok := nodeRepo.(domain.NodeQuotaRepository); ok` (`:55`) |
| `internal/service/config_service.go:57,90,1628-1632` | 字段 + **type assertion** | 持有 `nodeRepo domain.NodeRepository`；`persistRemoteNodeConfig` 执行 `cfgRepo, ok := s.nodeRepo.(domain.NodeConfigRepository)` (`:1628`) |
| `internal/service/sub_service.go:22,34` | 字段 | 仅使用 `nodeRepo.ListAll(ctx)` |
| `internal/delivery/cron/traffic_sync.go:37,80-83` | 字段 | `nodeBatchRepo domain.NodeTrafficBatchRepository` + `WithNodeBatchRepo` |

**测试 Mock 实现点**（合并 4 接口时的影响面）：
- `internal/service/alert_service_test.go:221-252`：`mockAlertNodeRepo` 仅实现 `NodeRepository` + `NodeQuotaRepository`（缺少 `ApplyNodeTrafficBatch`, `PruneBatchesBefore`, `UpdateNodeConfig`）。
- `internal/service/sub_service_test.go:829-851`：`mockSubNodeRepo` 仅实现 `NodeRepository`（缺少其余 3 个接口的方法）。
- `internal/delivery/cron/traffic_sync_test.go:534-551`：`mockNodeBatchRepoForCron` 仅实现 `NodeTrafficBatchRepository`（注意 `TrafficSyncJob` 仅依赖 `PruneBatchesBefore` / `NodeTrafficBatchRepository`，若全量合并进单一 `NodeRepository` 需给该 mock 补空方法或保留/内嵌 stub，或让 `TrafficSyncJob` 依赖最小接口 `NodeBatchPruner` / 补齐 mock）。
- 其余测试（`node_repo_test.go`, `node_service_test.go`, `config_service_test.go`, `agent_e2e_test.go`, `handler_agent_test.go`, `handler_geodata_test.go`, `traffic_sync_test.go:617`）均直接使用真实 `*repository.GORMNodeRepository`。

### 4.2 `RemoteNodeCompiler` 与 `RemoteNodeFullCompiler` 定义与引用

- **`RemoteNodeCompiler`**（`internal/service/node_service.go:39-42`）：
  - 方法：`CompileRemoteNodeToJSON(inbounds, users)`、`BuildRemoteNodeUsers(inbounds, users)`
  - 引用：`node_service.go:134,157,977,981,986`
- **`RemoteNodeFullCompiler`**（`internal/service/config_service.go:38-47`）：
  - 方法：`CompileRemoteNodeFullToJSON(inbounds, outbounds, routing, dns, users, staticClients)`
  - 引用：
    - `config_service.go:1621-1622`：`if fc, ok := s.compiler.(RemoteNodeFullCompiler); ok { return fc.CompileRemoteNodeFullToJSON(...) }`
    - `node_service.go:967-975`：`if fullCompiler, ok := s.compiler.(RemoteNodeFullCompiler); ok { ... }`（否则回退到 `s.compiler.CompileRemoteNodeToJSON`）
- **唯一实现**：`internal/adapter/xray/compiler.go:357,420,436`（`*XrayCompiler` 同时实现 `ConfigCompiler`、`RemoteNodeCompiler`、`RemoteNodeFullCompiler`，且 `CompileRemoteNode` 已在 `:352-354` 直接委托 `CompileRemoteNodeFull(inbounds, nil, nil, nil, users, nil)`）。
- **测试引用**：
  - `internal/adapter/xray/compiler_test.go:2277,2373,2424`（直接调用 `*XrayCompiler` 的 `CompileRemoteNodeFullToJSON` 与 `BuildRemoteNodeUsers`）；
  - `internal/service/config_service_test.go:79-85`：`mockCompiler` 仅嵌入并实现 `ConfigCompiler`（`CompileToJSON`），未实现 `RemoteNodeFullCompiler`。
- **合并可行性**：
  - `node_service.go:967-985` 中的运行时 `RemoteNodeFullCompiler` 断言与回退分支可收敛到 `RemoteNodeCompiler`（将 `CompileRemoteNodeFullToJSON` 纳入 `RemoteNodeCompiler` 或统一接口）；
  - `config_service.go` 因测试 `mockCompiler` 仅实现 `ConfigCompiler.CompileToJSON`，若将 `RemoteNodeFullCompiler` 保留为扩展或由 `ConfigService` 直接通过 `CompileRemoteNodeFullToJSON`（给 `mockCompiler` 补方法）均可消除额外类型断言。

### 4.3 `geodata_service.go` vs `geodata_updater.go` 抽取公共 `internal/pkg/geodata` 可行性

| 对比项 | `internal/service/geodata_service.go` (360 行) | `internal/agent/geodata_updater.go` (295 行) |
|---|---|---|
| **默认常量/镜像 URL** | `:177-180` (`geoipURLs`), `:208-211` (`geositeURLs`) —— **完全一致** (`v2fly` + `Loyalsoldier`)；Linux 默认 `/usr/local/share/xray` | `:17` (`defaultXrayAssetsDir`), `:24-31` (`defaultGeoIPURLs`, `defaultGeoSiteURLs`) —— **完全一致** |
| **HTTP 客户端** | `:145-161` (`newGeoDataHTTPClient`, 180s timeout + explicit Dial/TLS/ResponseHeader timeouts) | `:88-89` (`&http.Client{Timeout: 60s}`) |
| **下载落盘逻辑** | `:284-360`（`.tmp` + `io.TeeReader` + 300ms `progressWriter` 回调 + `os.Rename`，**无** `minFileSize` 校验、**无** `.bak` 回滚） | `:165-221`（`downloadWithFallback` + `.tmp` + `f.Sync()` + `Chmod(0644)` + `minFileSize` 100KB 校验 + `.bak` 备份与 `ReloadAndProbe` 失败回滚） |
| **服务重载** | `CoreController.RestartService`（本机 systemctl） | `ServiceReloader.ReloadAndProbe`（Agent systemctl + gRPC probe） |

- **抽取评估**：
  - **高收益无风险可抽取部分**：默认镜像 URL（`DefaultGeoIPURLs` / `DefaultGeoSiteURLs`）、`DefaultAssetsDir`、`DefaultMinFileSize`（100KB）、`NewHTTPClient(timeout)`、带 `io.Writer` / 字节回调的 `DownloadFile(ctx, client, url, destTmpPath, minSize, progressFn)` 原语（~80-100 行）。
  - **保留在各自包内的部分**：`GeoDataService` 的进度状态机（`GeoDataProgress` + `RestartService`）与 `HTTPGeoDataUpdater` 的 `.bak` 事务备份/回滚编排（两者职责不同，不宜强行合并编排层）。
  - **额外发现**：`GeoDataService.downloadWithProgress` 当前缺少 `minFileSize`（100KB）校验与 `ctx` 取消传递，复用 `internal/pkg/geodata` 后可顺便补齐 HTML 错误页拦截与 `ctx` 超时/取消支持。

---

## 5. P2-3 / P2-4 收尾

### 5.1 `web/src/mock/index.ts` 与 `storage.ts` 缺失 `/admin/nodes` 路由

- **`web/src/mock/index.ts` 现状**：
  - 第 `:1085-1165` 行**已实现** `GET /admin/nodes/:id/commands/:commandId`（服务于远端日志与远端 GeoData 异步命令轮询），第 `:662-687` 行已处理 `GET /logs?nodeId=...`，第 `:1063-1083` 行已处理 `POST /geodata/update?nodeId=...`；
  - **完全缺失** `web/src/api/nodes.ts:180-216` 调用的核心 CRUD 与操作路由：
    - `GET /admin/nodes`
    - `GET /admin/nodes/:id`
    - `POST /admin/nodes`
    - `PUT /admin/nodes/:id`
    - `DELETE /admin/nodes/:id`
    - `POST /admin/nodes/:id/rotate-token`
    - `POST /admin/nodes/:id/reset-traffic`
    - `GET /admin/nodes/:id/install-script`
  - **后果**：在 Mock/Demo 模式下，任何未命中的请求会跌落到 `web/src/mock/index.ts:1183` 的 `return delay({ success: true })`——导致 `listNodes()` 中 `Array.isArray(res)` 为 `false` 返回空数组 `[]`，且创建/编辑/轮换 Token 均无真实 Mock 状态支撑。
- **`web/src/mock/storage.ts` 现状**（`:3-14`）：`MockState` 接口与 `DEFAULT_RAW_STATE` 尚未包含 `nodes?: any[]` 初始演示节点数据。

### 5.2 配额 / 熔断规则与流量口径展示位置（P2-4）

需补充“节点离线后周期物理网卡流量口径提示（离线期间若存在宿主机物理网卡外部流量 / 恢复上线后的累计补偿统计口径）”的精确 UI 位置：
1. **`web/src/views/nodes/components/NodeFormDrawer.vue:117-234`**：`SectionCard title="③ 流量配额与熔断策略"`（包含月流量上限、账单重置日、计费口径 `trafficCountMode`、安全系数 `trafficMultiplier`、预警/摘流/熔断阈值与 `:229-231` 的规则提示段落）。
2. **`web/src/views/nodes/types.ts:52-72`**：`NODE_TRAFFIC_COUNT_MODE_OPTIONS`（各计费口径的 `description` 提示文本）。
3. **`web/src/views/NodesView.vue:238-271`（桌面表格“周期配额与流量”列）与 `:447-468`（移动卡片配额区）**：展示 `cycleUsedBytes / trafficLimitBytes`、Xray 统计与物理 NIC 统计对比。

---

## 6. Files Found

| Path | Role |
|---|---|
| `web/src/App.vue` | 顶栏节点切换器（`:219-281,369-417`）、重启核心（`:141-148,337-346,434-443,732-743`）、导航分类（`:624-654`） |
| `web/src/router/index.ts` | 前端全量路由定义（`:18-33`） |
| `web/src/composables/useActiveNode.ts` | 全局节点单例 composable（`:1-176`） |
| `web/src/views/nodes/composables/useNodes.ts` | 节点列表局部 composable 与纯函数工具（`:1-309`） |
| `web/src/views/NodesView.vue` | 节点管理主视图、KPI 卡片（`90s` 文案 `:52`）、操作列（`:330-382,530-574`）、15s 轮询（`:713-752`） |
| `web/src/views/nodes/components/NodeFormDrawer.vue` | 节点新增/编辑抽屉与配额/熔断策略表单（`:117-234`） |
| `web/src/views/nodes/components/NodeTokenModal.vue` | 一次性 Token 与部署脚本弹窗 |
| `web/src/views/nodes/types.ts` | 节点表单类型与计费口径/调度状态选项 |
| `web/src/components/ui/Drawer.vue` | 通用可复用右侧抽屉组件（`:1-154`） |
| `web/src/views/InboundsView.vue` | 入站主视图与冗余 `nodeFilter` 联动（`:13-31,71-83,197-221`） |
| `web/src/views/inbounds/composables/useInboundList.ts` | 入站列表 composable，`nodeFilter` 过滤与全量 `/inbounds` 请求（`:244-262,346-381`） |
| `web/src/views/inbounds/components/InboundFormDrawer.vue` | 入站表单抽屉（`:1125,1145` 消费 `useActiveNode`） |
| `web/src/views/TopologyView.vue` | 拓扑主视图与 `viewLogs` 跳转（`:219-221`） |
| `web/src/views/LogsView.vue` | 日志主视图，`route.query` 驱动本地日志 vs 远端命令日志（`:1443,1558-1695,2134-2156,2208-2243`） |
| `web/src/views/RoutingView.vue` | 路由视图与待迁出的 GeoData 卡片/轮询（`:49-200,1177-1398,1858-1885`） |
| `web/src/views/SettingsView.vue` | 系统设置视图 6 段布局（`:24-634`，`xray_geodata_dir` 在 `:99-110`） |
| `web/src/api/nodes.ts` | 节点与命令 API 封装（`/admin/nodes/**`） |
| `web/src/mock/index.ts` | 前端 Mock 请求分发器（缺 `/admin/nodes` CRUD 路由） |
| `web/src/mock/storage.ts` | 前端 Mock 持久化状态（缺 `nodes` 字段） |
| `deploy/cloudflare-pages/portal.html` | CF Pages `/portal.html`（与另两份 SHA-256 完全一致） |
| `deploy/cloudflare-pages/portal/index.html` | CF Pages `/portal/`（与另两份 SHA-256 完全一致） |
| `deploy/standalone-portal.html` | 独立提取门户 HTML（与另两份 SHA-256 完全一致） |
| `deploy/cloudflare-pages/_worker.js` | CF Pages 边缘网关（`:223-232` 静态放行 portal 路径） |
| `internal/domain/node.go` | 节点领域模型与 `180s` 在线阈值定义（`:148,212`） |
| `internal/domain/repository.go` | `NodeRepository` 等 4 个节点仓储接口定义（`:113-141`） |
| `internal/adapter/repository/node_repo.go` | `GORMNodeRepository` 统一实现（`:16-24`） |
| `internal/service/node_service.go` | `RemoteNodeCompiler`（`:39-42`）、`quotaRepo` 断言（`:175`）、`RemoteNodeFullCompiler` 断言（`:967`） |
| `internal/service/config_service.go` | `RemoteNodeFullCompiler`（`:38-47`）、`NodeConfigRepository` 断言（`:1628`） |
| `internal/service/alert_service.go` | `NodeQuotaRepository` 断言（`:55`） |
| `internal/delivery/cron/traffic_sync.go` | `NodeTrafficBatchRepository` 注入与批次清理（`:37,80`） |
| `internal/adapter/xray/compiler.go` | `CompileRemoteNodeToJSON` / `CompileRemoteNodeFullToJSON` / `BuildRemoteNodeUsers` 实现（`:352-450`） |
| `internal/service/geodata_service.go` | Master GeoData 下载与进度服务（360 行） |
| `internal/agent/geodata_updater.go` | Agent GeoData 下载校验与回滚更新器（295 行） |

---

## 7. Relevant `.trellis/spec/` Files（`implement.jsonl` / `check.jsonl` 候选列表）

- `.trellis/spec/frontend/index.md`
- `.trellis/spec/frontend/directory-structure.md`
- `.trellis/spec/frontend/components-and-routing.md`
- `.trellis/spec/frontend/api-and-dataflow.md`
- `.trellis/spec/frontend/quality-guidelines.md`
- `.trellis/spec/backend/index.md`
- `.trellis/spec/backend/directory-structure.md`
- `.trellis/spec/backend/quality-guidelines.md`
- `.trellis/spec/backend/error-handling.md`
- `.trellis/spec/guides/code-reuse-thinking-guide.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`

---

## 8. Caveats / Open Questions

1. **`main.go` 链路补充核查**：本次检索看到 `main.go:71-180` 创建并注入了 `nodeRepo`，未完整读取 `main.go:181-276` 以确认 `TrafficSyncJob.WithNodeBatchRepo(nodeRepo)` 的接线位置；如合并 `NodeRepository` 时同步重构 `TrafficSyncJob`，需顺带核查 `main.go` 尾部。
2. **`TopologyView.vue` 行号差异**：请求中提及 `TopologyView.vue:259`，实际源码中跳转 `/logs` 的 `viewLogs` 位于 `TopologyView.vue:219-221`（`:259` 处于 `toggleChannelStatus`）。
3. **Cloudflare Pages `_worker.js` 静态路由与 Pretty URLs**：若删除 `deploy/cloudflare-pages/portal/index.html` 而仅留 `portal.html`（或反之），需确保 `_worker.js` 内 `env.ASSETS.fetch` 不会因 CF Pages 默认 Pretty URLs 规范化产生 308 循环或 404；最稳妥方案需在 `deploy/gateway.test.js` 覆盖 4 条 `/portal*` 路径测试。
4. **`InboundsView` 是否保留“查看全部节点”能力**：若彻底移除 `nodeFilter` 仅用全局 `activeNodeId` 并请求 `GET /inbounds?nodeId=...`，需确认用户是否仍需在一个页面同时总览跨节点入站（如需要，全局切换器本身可考虑或保留为显式 `?nodeId=` / 全局视图；若按 M3 约定完全对齐其他 Node-Scoped 页面，则删除 `nodeFilter` 下拉框并直接按 `activeNodeId` 查询）。
