# M3 Technical Design: 前端节点交互收敛、Portal 单源化与仓储 KISS 精简

## 1. Architecture & Boundaries

### 1.1 Backend KISS 收敛 (`internal/**`, `main.go`)
1. **单一 `domain.NodeRepository`** (`internal/domain/repository.go`):
   - 将原 `NodeTrafficBatchRepository` (`ApplyNodeTrafficBatch`, `PruneBatchesBefore`)、`NodeQuotaRepository` (`UpdateAutoState`, `ResetCycleTraffic`, `ListRecentDailyTraffic`)、`NodeConfigRepository` (`UpdateNodeConfig`) 全部并入 `NodeRepository` 接口。
   - 删除 3 个子接口，移除 `node_service.go:175`、`alert_service.go:55`、`config_service.go:1628` 的运行时类型断言。
   - 简化签名：
     - `NewNodeService(nodeRepo domain.NodeRepository, inboundRepo domain.InboundRepository, userRepo domain.UserRepository, compiler RemoteNodeCompiler, ...)`（移除多余的第 2 个 `batchRepo` 参数与 `WithQuotaRepo`）。
     - `AlertService.WithNodeQuota(nodeRepo domain.NodeRepository)`（合并为单参数）。
     - `TrafficSyncJob.WithNodeBatchRepo(repo domain.NodeRepository)`。
   - 测试 mock（`mockAlertNodeRepo`、`mockSubNodeRepo`、`mockNodeBatchRepoForCron`）通过匿名嵌入 `domain.NodeRepository` 零样板满足编译期接口约束。
2. **统一 `RemoteNodeCompiler`**:
   - 在 `internal/service/node_service.go` 的 `RemoteNodeCompiler` 直接包含 `CompileRemoteNodeFullToJSON(...)` 与 `BuildRemoteNodeUsers(...)`（及 `CompileRemoteNodeToJSON` 若仍有单测直接调用则保留或委托）。
   - 删除 `internal/service/config_service.go` 中的 `RemoteNodeFullCompiler` 接口及两处 `s.compiler.(RemoteNodeFullCompiler)` 断言：`ConfigService` 可选通过 `WithRemoteCompiler(RemoteNodeCompiler)` 或在 `ConfigCompiler` 上直接支持（更 KISS：在 `ConfigService.compileRemoteNode` 检查 `s.remoteCompiler` 字段或把 `CompileRemoteNodeFullToJSON` 并入 `ConfigCompiler`？检查一下 `ConfigCompiler` 仅被 `*XrayCompiler` 与 1 个 `mockCompiler` 实现——在 `ConfigCompiler` 增加 `CompileRemoteNodeFullToJSON` 或让 `ConfigService` 持有 `remoteCompiler RemoteNodeCompiler` 即可完全消灭 `type assertion`！推荐在 `ConfigService` 复用同一 `RemoteNodeCompiler` 或直接将 `CompileRemoteNodeFullToJSON` 纳入 `ConfigCompiler` 给 `config_service_test.go` 的 `mockCompiler` 补 1 个 3 行方法，零断言、零额外 setter！）。
3. **纯标准库 `internal/pkg/geodata`**:
   - 新建 `internal/pkg/geodata/downloader.go` 与 `downloader_test.go`（**仅导入 Go stdlib**）：
     - `const DefaultAssetsDir = "/usr/local/share/xray"`
     - `const DefaultMinFileSize int64 = 100 * 1024`
     - `var DefaultGeoIPURLs = [...]`、`var DefaultGeoSiteURLs = [...]`
     - `NewHTTPClient(timeout time.Duration) *http.Client`（显式 Dial / TLS / ResponseHeader 超时）
     - `DownloadFile(ctx context.Context, client *http.Client, url, tmpPath string, minSize int64, onProgress func(written, total int64)) (int64, error)`（流式下载、检查 `200 OK`、`io.Copy` + 进度回调、校验 `written >= minSize`、`f.Sync()` + `os.Chmod(0644)`，失败自动清理 `.tmp`）
     - `DownloadWithFallback(ctx context.Context, client *http.Client, urls []string, tmpPath string, minSize int64, onProgress func(written, total int64)) (int64, error)`
   - `internal/service/geodata_service.go` 改用 `geodata.DefaultGeoIPURLs` / `DefaultGeoSiteURLs` / `NewHTTPClient` / `DownloadWithFallback`（补齐 `minSize` 100KB 拦截与 `ctx` 超时）；
   - `internal/agent/geodata_updater.go` 改用 `geodata.DefaultAssetsDir` / `DefaultGeoIPURLs` / `DefaultGeoSiteURLs` / `DownloadWithFallback`（保留原有 `.bak` 事务备份回滚与 `ReloadAndProbe`）。

### 1.2 Deploy Portal HTML 单源化 (`deploy/**`, `docs/ARCHITECTURE_AND_LOGIC.md`)
- 保留唯一源文件：`deploy/cloudflare-pages/portal/index.html`。
- 删除冗余副本：`deploy/cloudflare-pages/portal.html`、`deploy/standalone-portal.html`。
- 修改 `deploy/cloudflare-pages/_worker.js:223-232`：
  ```js
  if (
    url.pathname === "/portal" ||
    url.pathname === "/portal/" ||
    url.pathname === "/portal.html" ||
    url.pathname === "/portal/index.html"
  ) {
    if (env?.ASSETS) {
      const targetUrl = new URL("/portal/", url.origin);
      targetUrl.search = url.search;
      return env.ASSETS.fetch(new Request(targetUrl.toString(), request));
    }
  }
  ```
- 在 `deploy/gateway.test.js` 补充单测：验证 4 条 `/portal*` 路径均被改写至 `/portal/` 并保留 query string，且 `deploy/cloudflare-pages/portal/index.html` 存在而两份旧副本不存在。
- 更新 `docs/ARCHITECTURE_AND_LOGIC.md:229` 引用路径。

### 1.3 Frontend 交互收敛与组件架构 (`web/src/**`)
1. **路由作用域元数据 (`web/src/router/index.ts`, `web/src/App.vue`)**：
   - 在路由 `meta` 中标注 `nodeScoped: true`（`/inbounds`, `/outbounds`, `/routing`, `/topology`, `/logs`, `/dns`, `/config`）与 `nodeScoped: false`（`/`, `/users`, `/nodes`, `/settings`）。
   - `App.vue` 计算 `isNodeScopedRoute = computed(() => Boolean(route.meta?.nodeScoped))`：
     - 顶栏/移动端节点 `<select>` 仅在 `isNodeScopedRoute` 时显示；在 Global 页展示紧凑的「全局 / Master 视图」只读徽标（不渲染易误导的节点下拉框）。
     - 顶栏/侧栏/移动端“重启核心”按钮与核心状态胶囊：当 `isNodeScopedRoute && isRemoteNode` 时，按钮 `disabled` 并展示提示 `远端节点通过配置下发自动热重载`，胶囊显示当前远端节点在线与配置同步状态（`activeNode.online` + `activeNodeSyncBadge`），防止误重启 Master。
2. **`InboundsView` & `TopologyView -> LogsView`**：
   - `useInboundList.ts`：移除 `nodeFilter` 下拉状态，改为 `showAllNodes = ref(false)`（默认按 `activeNodeId` 过滤，仅在勾选“全部节点”时展示跨节点汇总）；`InboundsView.vue` 移除 `<select v-model="nodeFilter">`，替换为轻量 toggle 按钮。
   - `TopologyView.vue:219-221`：`viewLogs(tag)` 改为 `router.push({ path: '/logs', query: { type: 'access', inbound: tag, nodeId: String(activeNodeId.value) } })`。
   - `LogsView.vue`：解析 `route.query.nodeId`（合法非负整数且非当前值时调用 `setActiveNode(parsed)`）；当处于远端节点且存在 `route.query.inbound` 时，将其自动填入 `remoteKeyword` 方便直接按入站 tag 筛选远端日志。
3. **单一节点状态源 (`useActiveNode.ts` + `useNodes.ts`)**：
   - `useActiveNode.ts` 负责维护模块级共享 `nodesList`、`nodesLoading`、单一定时刷新接口（`loadNodes(force?)`, `refreshNodesNow()`, `startListPolling()`, `stopListPolling()` 基于引用计数避免多页面重复 `setInterval`）。
   - `useNodes.ts` 直接解构复用 `useActiveNode().nodesList` 作为 `nodes`，所有写操作（删除/启停/轮换 Token/重置流量）完成后调用 `refreshNodesNow()` 即时同步顶栏切换器与表格，同时删除 `NodesView.vue` 内的重复轮询状态分叉；修正 `NodesView.vue:52` 文案为 `在线心跳 (180s 内)`。
4. **节点运维抽屉 (`web/src/views/nodes/components/NodeOpsDrawer.vue`)**：
   - Props：`modelValue: boolean`、`node: NodeItem | null`；Emits：`update:modelValue`、`open-token`、`reset-traffic`、`toggle-state`、`refresh`。
   - 4 个清晰 Tab：`overview`（状态与配额 + 离线物理网卡口径提示 + 重置流量/摘流恢复快捷按钮）、`logs`（异步 `GET /logs?nodeId=` + `2s` 轮询 `getNodeCommandStatus` + 切换 `xray`/`xray-agent` + 跳转完整 `/logs`）、`config`（`GET /config/raw?nodeId=` 只读预览 + 复制 + 跳转完整 `/config`）、`geodata`（异步 `POST /geodata/update?nodeId=` + `2s` 轮询 + 打开安装脚本/轮换 Token）。
   - 严格生命周期守卫：`stopAllTimers()` 在 `watch(() => props.modelValue)` 关闭、`watch(() => props.node?.id)` 切换与 `onUnmounted` 时执行，且使用单调序号守卫拦截过期异步回调。
5. **GeoData 从 `RoutingView` 迁出 -> `SettingsView`**：
   - 清理 `RoutingView.vue` 的 GeoData 卡片 UI、状态、轮询与 `useActiveNode` 中仅供 GeoData 使用的逻辑。
   - 在 `SettingsView.vue` ① 区紧邻 `xray_geodata_dir` 植入本机 Master GeoData 状态与一键升级面板（复用 `/geodata/status`、`/geodata/update`、`/geodata/progress` 500ms 轮询），保存设置后刷新 `fetchGeoStatus()`，并在 `onUnmounted` 清理定时器。
6. **Mock 补全 & 离线流量口径提示**：
   - `web/src/mock/storage.ts` 增加 `nodes` 默认演示节点（含在线/待同步/配额示例）；`web/src/mock/index.ts` 补齐 `GET/POST /admin/nodes`、`GET/PUT/DELETE /admin/nodes/:id`、`POST /admin/nodes/:id/rotate-token`、`POST /admin/nodes/:id/reset-traffic`、`GET /admin/nodes/:id/install-script`。
   - `web/src/views/nodes/components/NodeFormDrawer.vue:229-231` 与 `NodeOpsDrawer.vue` 补充离线物理网卡流量口径提示。

## 2. Parallel Ownership & Rollback Points
- **Slice A (`m3-backend-deploy`)**：独占 `internal/**`、`cmd/agent/**`、`main.go`、`deploy/**`、`docs/**`。
- **Slice B (`m3-frontend`)**：独占 `web/**`。
- 两侧文件 100% 互斥，且 M3 不改变任何 REST API 路径/JSON 契约，天然适合零冲突并行！
