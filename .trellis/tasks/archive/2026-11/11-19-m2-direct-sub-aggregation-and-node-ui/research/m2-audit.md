# M2 架构调研报告 (`11-19-m2-direct-sub-aggregation-and-node-ui`)

## 1. 后端订阅聚合与客户端格式链路

### 1.1 现状链路梳理
1. **HTTP 入口** (`internal/delivery/http/handler_sub.go:22-82`):
   - `SubHandler.GetSubscription` 从路径/查询参数提取 `token`、`tagFilter` (`tag`)、`reqHost` (`c.Request.Host`)、`format` (`format` 或 `type`)、`routeFilters` (`route`)。
   - 调用 `h.subSvc.ExportUserSubscription(ctx, token, tagFilter, reqHost, format, routeFilters...)` (`handler_sub.go:46`)。
   - 响应头 `Content-Type` (`handler_sub.go:59-66`) 目前仅根据显式 `format` 参数切换 `application/yaml` / `application/json` / `text/plain`，**尚未做 `User-Agent` 自动识别**。
2. **订阅聚合服务** (`internal/service/sub_service.go:17-378`):
   - `SubService` 结构体当前仅注入 `userRepo`、`inboundRepo`、`settingRepo` (`sub_service.go:17-29`)，尚未注入 `nodeRepo domain.NodeRepository`。
   - `resolveSubscriptionNodesWithSettings` (`sub_service.go:119-180`):
     - 校验 `user` 存在、`user.Enabled`、`user.IsActive()` (`sub_service.go:124-134`)。
     - 调用 `s.inboundRepo.ListAll(ctx)` 获取全量 `[]domain.Inbound` (`sub_service.go:139`)。
     - `hostDomain` 回退顺序：`settings["sub_domain"]` -> `settings["public_url"]` -> `reqHost` (`sub_service.go:145-151`)。
     - 调用 `protocol.InboundsToNodeConfigsWithDefaultFP(inbounds, user, hostDomain, defaultPort, tagFilter, defaultFP, routeFilter...)` (`sub_service.go:155`)。
     - 应用 `settings["sub_node_name_template"]` (`sub_service.go:158-160`)，全量订阅时在头部注入虚拟信息节点 (`sub_service.go:163-177`)。
   - `GetUserShareInfo` (`sub_service.go:284-378`) 为管理端/用户分享弹窗生成单节点分享链接，同样遍历 `inbounds` 并调用 `protocol.BuildShareLinkWithDefaultFP`，**需同步适配远端节点（健康过滤、`Node.Host` 回退、地区/节点前缀）**。
3. **节点转换与权限过滤** (`internal/protocol/node_converter.go:129-477`):
   - `InboundsToNodeConfigsWithDefaultFP` (`node_converter.go:403-477`) 按 `SortOrder ASC, ID ASC` 排序后，逐个检查 `!in.Enabled || !user.HasInbound(in.Tag) || dokodemo-door` (`node_converter.go:420`，`User.HasInbound` 位于 `internal/domain/user.go:58`) 及 `tagFilter` (`node_converter.go:423`)。
   - 对 VLESS `SubRoute` (`node_converter.go:428-474`) 校验 `sr.Enabled`、`targetRoute` 与 `sr.CanAccess(user.Email)`。
   - `InboundToNodeConfigWithDefaultFP` (`node_converter.go:129-395`) 地址回退链：`cleanHost(inbound.ExternalHost)` -> `cleanHost(hostDomain)` -> `inbound.Listen` (`node_converter.go:142-151`)；端口回退链：`inbound.ExternalPort` -> `defaultPort` -> `inbound.Port` (`node_converter.go:153-158`)。
4. **格式导出器** (`internal/sub/exporter.go`, `internal/sub/clash.go`, `internal/sub/singbox.go`):
   - `exporter.go:83-107`: 基于 `globalExporters` 策略表分发 `base64`/`raw`/`clash`/`sing-box`（`ClashParams` 走 `ExportClashWithParams`）。
   - `clash.go:33-104`: 遍历 `nodes` 调用 `protocol.ToClash(node)` (`internal/protocol/registry.go:94`)。**关键风险点**：`clash.go:46-49` 一旦 `protocol.ToClash(node)` 返回 `err` 即**整份订阅直接报错返回**；且 `internal/protocol/helpers.go:189-202` 当前将 `xhttp`/`splithttp` 拼装成 `xhttp-opts`，老版本/部分 Clash 客户端不识别（若需兼容过滤，需在 `clash.go` 中识别并安全跳过）。另外 `clash.go:57-60` 在 `len(proxyNames) == 0` 时已有 `fallbackProxies = []string{"DIRECT"}` 兜底，但缺少友好提示注释/名称或可观测日志。
   - `singbox.go:16-93`: 遍历 `nodes` 调用 `protocol.ToSingBox(node)` (`singbox.go:29-32`)，遇错同样直接中断；`helpers.go:242-255` 为 `xhttp`/`splithttp` 生成 `transport.type = "xhttp"`。

### 1.2 M2 后端最干净接线点建议
1. **`SubService` 依赖扩展** (`internal/service/sub_service.go:17-29`, `main.go:155`):
   - 为 `SubService` 增加可选 `nodeRepo domain.NodeRepository`（建议提供 `WithNodeRepo(nodeRepo domain.NodeRepository) *SubService` 链式注入，避免破坏现有 20+ 处 `NewSubService(userRepo, inboundRepo, settingRepo)` 测试构造器签名，并在 `main.go:155` 挂载 `.WithNodeRepo(nodeRepo)`）。
2. **多节点过滤、Host 回退与命名前缀** (`sub_service.go` `resolveSubscriptionNodesWithSettings` & `GetUserShareInfo`):
   - 若 `s.nodeRepo != nil`，加载 `nodes, err := s.nodeRepo.ListAll(ctx)` 并构建 `nodeMap map[uint]domain.Node`。
   - 过滤 `Inbound`:
     - `ib.NodeID == 0`: 本机 Master 节点，保持现有 `hostDomain` 回退逻辑。
     - `ib.NodeID > 0`: 查 `node, ok := nodeMap[ib.NodeID]`；仅当 `ok && node.Enabled && node.DesiredState == domain.NodeStateActive` 时放行（过滤掉 `draining` 和 `circuit_broken`，见 `internal/domain/node.go:8-12`）。
     - **Host 回退**：在调用 `InboundToNodeConfigWithDefaultFP` / `BuildShareLinkWithDefaultFP` 前，若 `ib.NodeID > 0` 且 `strings.TrimSpace(ib.ExternalHost) == ""`，传入 `node.Host` 作为 `hostDomain`（优先级：`Inbound.ExternalHost` > `Node.Host` > 全局 `sub_domain`/`public_url`/`reqHost`）。
     - **节点名称前缀**：当 `ib.NodeID > 0` 时，基于 `node.Region` / `node.Name`（如 `[node.Region] ` 或 `[node.Name] `）为 `NodeConfig.Name` / `remark`（以及 `GetUserShareInfo` 的 `Remark`）注入地区/节点前缀（注意在 `ApplyNodeNameTemplate` 前设置 `remark` 参数或之后统一加前缀，避免被模板覆盖）。
3. **UA 识别与 Clash 安全跳过** (`handler_sub.go:34-66`, `clash.go:37-60`):
   - `handler_sub.go`: 当 `format == ""` 时，检查 `strings.ToLower(c.GetHeader("User-Agent"))`：包含 `clash`/`mihomo`/`stash` 设为 `"clash"`；包含 `sing-box`/`singbox` 设为 `"sing-box"`；否则保持默认 `"base64"`。同时 `Content-Type` switch 需使用解析后的 `format`。
   - `clash.go`: 对不兼容 Clash 的传输层（如 `xhttp` / `splithttp`，或 `protocol.ToClash(node)` 返回 error 时）记录 `slog.Debug`/`Warn` 并 `continue` 安全跳过，而非中断整份 YAML；当过滤后 `len(proxies) == 0` 时，保留 `DIRECT` 兜底并在 `PROXIES`/`AUTO` 或首条注释/提示中给出明确友好提示（避免客户端导入空列表崩溃）。

---

## 2. Vue 3 前端结构与 UI 组件规范

### 2.1 路由、导航与目录风格
- **路由定义** (`web/src/router/index.ts:4-31`):
  - 使用 `createWebHistory`，懒加载视图组件，受保护路由统一 `meta: { requiresAuth: true }`。
  - 新增节点管理页路由：`{ path: '/nodes', component: NodesView, meta: { requiresAuth: true } }`。
- **侧边栏导航** (`web/src/App.vue:428-478`):
  - 菜单硬编码在 `App.vue:440-469` 的 `navSections` 数组中（分组：`OVERVIEW` / `RESOURCES` / `OPERATIONS` / `SYSTEM`），图标来自 `lucide-vue-next` (`App.vue:378-394`)。
  - 可在 `RESOURCES` 或 `OPERATIONS` 中新增 `{ name: '节点管理', path: '/nodes', icon: Server, shortName: '节点' }`。
- **API 与 Composables 风格**:
  - `web/src/api/index.ts:30-59`: 封装 `api.get<T>`, `api.post<T>`, `api.put<T>`, `api.delete<T>`（自动附加 JWT、统一解包 `response.data`、401 跳转登录、支持 Mock 模式）。领域 API 独立成文件（如 `web/src/api/users.ts:1-9`、`web/src/api/reality.ts`）并在 `api/index.ts:57-58` 导出。
  - Views 分模块组织（参考 `web/src/views/inbounds/`）：`types.ts`、`composables/use*.ts`、`components/*Table.vue` / `*FormDrawer.vue` / `*DetailDrawer.vue`。
- **核心 UI 复用组件**:
  - `Modal` (`web/src/components/ui/Modal.vue`): `v-model`, `title`, `description`, `size`, slots (`header`/`sub-header`/`default`/`footer`)。
  - `Drawer` (`web/src/components/ui/Drawer.vue:86-97`): `v-model`, `title`, `description`, `width`。
  - `Badge` (`web/src/components/ui/Badge.vue:18-30`): `variant` (`'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive'`), `dot`, `dotColor`。
  - `Toast` (`web/src/utils/toast.ts:29-34`): `toast.success/error/info/warning(...)`。
  - `Clipboard` (`web/src/utils/clipboard.ts:3-27`): `copyText(text, successMessage?)` / `copyToClipboard`。

### 2.2 工作区脏文件与 M2 前端文件边界（严禁覆盖用户未提交改动）
`git status --short` 检测到以下已修改文件（**M2 严禁修改或回退这些文件**）：
- `web/src/style.css`
- `web/src/views/PortalClaimView.vue`
- `web/src/views/users/components/UserShareModal.vue`
- `deploy/cloudflare-pages/portal.html`, `deploy/cloudflare-pages/portal/index.html`, `deploy/standalone-portal.html`
- `.trellis/.template-hashes.json`, `web/dist/*`

**M2 前端文件规划**：
- **新增文件**：
  - `web/src/api/nodes.ts`（节点 CRUD、轮换 Token、安装脚本 API 与严格对齐后端的 TS 类型）
  - `web/src/views/NodesView.vue`（节点列表、状态徽章、一键安装命令弹窗、轮换 Token 一次性展示弹窗、新增/编辑 Drawer）
  - `web/src/views/nodes/types.ts` / `composables/useNodes.ts` / `components/NodeFormDrawer.vue` / `components/NodeTokenModal.vue`（按 `inbounds/` 模块规范拆分）
- **修改既有文件（最小集）**：
  - `web/src/api/index.ts`（`export * from './nodes'`，按需补充 `mock` 支持）
  - `web/src/router/index.ts`（注册 `/nodes` 路由）
  - `web/src/App.vue`（`navSections` 添加节点管理入口）
  - `web/src/views/inbounds/types.ts`、`web/src/views/InboundsView.vue`、`web/src/views/inbounds/components/InboundFormDrawer.vue`、`web/src/views/inbounds/composables/useInboundList.ts`（为入站表单与列表增加 `nodeId` 归属节点选择器，支持 `0 = 本机 Master` 及远端节点列表，并在 `InboundsView` 支持按节点筛选；后端 `handler_inbound.go:38-46` 和 `config_service.go:669-738` 已原生支持 `nodeId`）。

### 2.3 M1 后端 `/api/admin/nodes*` 接口契约对齐（TypeScript 类型基准）
依据 `internal/delivery/http/router.go:185-195`、`internal/delivery/http/handler_node.go:44-206` 与 `internal/domain/node.go:8-105`：
- **路由端点**：
  - `GET /api/admin/nodes` -> `Node[]`
  - `GET /api/admin/nodes/:id` -> `Node`
  - `POST /api/admin/nodes` (body: `CreateNodeDTO`) -> `201 CreateNodeResponse` (`Node & { token: string }`)
  - `PUT /api/admin/nodes/:id` (body: `UpdateNodeDTO`) -> `Node`
  - `DELETE /api/admin/nodes/:id` -> `{ message: string }`（若存在关联入站返回 `409 Conflict`）
  - `POST /api/admin/nodes/:id/rotate-token` -> `RotateTokenResponse` (`{ nodeId: number, tokenPrefix: string, token: string }`)
  - `GET /api/admin/nodes/:id/install-script?masterUrl=...&format=raw` -> 默认 `{ nodeId: number, script: string }`（`format=raw` 返回纯文本）
- **`Node` JSON 字段** (`internal/domain/node.go:27-53`):
  - `id: number`, `name: string`, `host: string`, `region: string`, `tokenPrefix: string`, `enabled: boolean`, `desiredState: 'active' | 'draining' | 'circuit_broken'`, `lastSeenAt: string | null`, `agentVersion: string`, `xrayVersion: string`, `lastConfigHash: string`, `lastUsersHash: string`, `lastSyncError: string`, `cpuPercent: number`, `memPercent: number`, `nicUpBytes: number`, `nicDownBytes: number`, `xrayUpBytes: number`, `xrayDownBytes: number`, `remark: string`, `createdAt: string`, `updatedAt: string`, `online: boolean`（`TokenHash` 已 `json:"-"` 隐藏）。
- **`CreateNodeDTO`** (`node.go:75-82`): `name` (required), `host` (required), `region?`, `enabled?`, `desiredState?`, `remark?`。
- **`UpdateNodeDTO`** (`node.go:91-98`): `name?`, `host?`, `region?`, `enabled?`, `desiredState?`, `remark?`（全指针可选）。

---

## 3. 候选 `.trellis/spec/` 清单
- 后端：
  - `.trellis/spec/backend/index.md`
  - `.trellis/spec/backend/directory-structure.md`
  - `.trellis/spec/backend/error-handling.md`
  - `.trellis/spec/backend/logging-guidelines.md`
  - `.trellis/spec/backend/quality-guidelines.md`
- 前端：
  - `.trellis/spec/frontend/index.md`
  - `.trellis/spec/frontend/directory-structure.md`
  - `.trellis/spec/frontend/api-and-dataflow.md`
  - `.trellis/spec/frontend/components-and-routing.md`
  - `.trellis/spec/frontend/quality-guidelines.md`
- 通用规范：
  - `.trellis/spec/guides/cross-layer-thinking-guide.md`
  - `.trellis/spec/guides/code-reuse-thinking-guide.md`
