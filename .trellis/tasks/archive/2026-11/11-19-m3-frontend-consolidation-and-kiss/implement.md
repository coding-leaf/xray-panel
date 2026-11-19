# M3 Implementation Plan

## Execution Slices (File-Disjoint)

### Slice A: Backend KISS 精简 + `internal/pkg/geodata` + Deploy Portal 单源化
**Owned Paths**: `internal/**`, `cmd/agent/**`, `main.go`, `deploy/**`, `docs/**`

- [ ] **A1. 收敛 `domain.NodeRepository`（消除 3 个碎片接口与 3 处运行时类型断言）**
  - `internal/domain/repository.go`：将 `NodeTrafficBatchRepository`、`NodeQuotaRepository`、`NodeConfigRepository` 的全部方法并入 `NodeRepository` 并删除这 3 个接口。
  - `internal/service/node_service.go`：移除 `batchRepo`/`quotaRepo`、`WithQuotaRepo` 与 `nodeRepo.(domain.NodeQuotaRepository)` 断言；简化 `NewNodeService` 签名。
  - `internal/service/alert_service.go`：移除 `quotaRepo` 与 `nodeRepo.(domain.NodeQuotaRepository)` 断言，`WithNodeQuota(nodeRepo domain.NodeRepository)` 仅接收 1 个参数。
  - `internal/service/config_service.go`：移除 `s.nodeRepo.(domain.NodeConfigRepository)` 断言，直接调用 `s.nodeRepo.UpdateNodeConfig`。
  - `internal/delivery/cron/traffic_sync.go`：`WithNodeBatchRepo` 参数类型更新为 `domain.NodeRepository`。
  - `main.go` 及全部受影响测试文件（`node_service_test.go`, `alert_service_test.go`, `sub_service_test.go`, `traffic_sync_test.go`, `agent_e2e_test.go`, `handler_*_test.go`）同步更新构造调用与 mock。
- [ ] **A2. 合并 `RemoteNodeCompiler` 与 `RemoteNodeFullCompiler`**
  - 将 `CompileRemoteNodeFullToJSON` 纳入 `RemoteNodeCompiler`（与 `ConfigCompiler` 或清晰共享接口），删除 `config_service.go` 中分裂的 `RemoteNodeFullCompiler` 定义与 `node_service.go` / `config_service.go` 中的运行时类型断言，补全 `config_service_test.go` 的 `mockCompiler`。
- [ ] **A3. 抽取纯标准库 `internal/pkg/geodata`**
  - 新建 `internal/pkg/geodata/downloader.go` 与 `downloader_test.go`（零外部依赖，封装镜像列表、默认目录、`100KB` 最小文件校验、`NewHTTPClient`、`DownloadFile`、`DownloadWithFallback`）。
  - 重构 `internal/service/geodata_service.go` 与 `internal/agent/geodata_updater.go` 复用 `internal/pkg/geodata`（保留各自进度条/重启 vs `.bak` 回滚/`ReloadAndProbe` 编排，同时为 Master 补齐 `minSize` 校验与 `ctx` 取消支持）。
- [ ] **A4. 静态 Portal HTML 单源化 (`deploy/**`, `docs/ARCHITECTURE_AND_LOGIC.md`)**
  - 删除 `deploy/cloudflare-pages/portal.html` 与 `deploy/standalone-portal.html`，保留 `deploy/cloudflare-pages/portal/index.html` 为唯一权威源。
  - 更新 `deploy/cloudflare-pages/_worker.js:223-232`，将 `/portal`、`/portal/`、`/portal.html`、`/portal/index.html` 统一改写到 `/portal/`（保留 query string）后交由 `env.ASSETS.fetch`。
  - 在 `deploy/gateway.test.js` 补充 4 条 `/portal*` 路径改写与单源文件存在性断言；更新 `docs/ARCHITECTURE_AND_LOGIC.md:229`。

### Slice B: Frontend 交互收敛、运维抽屉、GeoData 迁移与 Mock 收尾
**Owned Paths**: `web/**`

- [ ] **B1. U-1 全局节点切换器作用域边界 + 防误重启 + `InboundsView` / `TopologyView`**
  - `web/src/router/index.ts`：为全部路由标注 `meta.nodeScoped`（7 个 Node-Scoped 为 `true`，其余为 `false`）。
  - `web/src/App.vue`：仅在 `isNodeScopedRoute` 显示可交互节点 `<select>`（Global 页展示只读全局/主控标识）；在 `isNodeScopedRoute && isRemoteNode` 下禁用顶栏/侧栏“重启核心”按钮并提示自动热重载，状态胶囊切换为当前远端节点在线/同步状态。
  - `web/src/views/inbounds/composables/useInboundList.ts` & `web/src/views/InboundsView.vue`：移除冗余本地 `nodeFilter` 下拉框，默认跟随 `activeNodeId`，保留轻量「全部节点」切换开关。
  - `web/src/views/TopologyView.vue:219-221` & `web/src/views/LogsView.vue`：跳转 `/logs` 携带 `nodeId` query，`LogsView` 同步 `activeNodeId` 并在远端模式下将 `?inbound=` 预填至 `remoteKeyword`。
- [ ] **B2. U-2 合并 `useNodes` 与 `useActiveNode` + 修正 P2-2 文案 (`90s -> 180s`)**
  - `web/src/composables/useActiveNode.ts` & `web/src/views/nodes/composables/useNodes.ts`：统一单一共享 `nodesList` 与引用计数轮询，`NodesView` 写操作即时同步顶栏切换器；修正 `NodesView.vue:52` 文案为 `在线心跳 (180s 内)`。
- [ ] **B3. U-2 节点运维抽屉 (`NodeOpsDrawer.vue`) + GeoData 迁出 `RoutingView` -> `SettingsView`**
  - 新建 `web/src/views/nodes/components/NodeOpsDrawer.vue`（基于 `Drawer.vue`，4 Tab：状态与配额 / 快速日志 / 编译预览 / GeoData 与部署，含定时器清理与序号守卫）。
  - 精简 `web/src/views/NodesView.vue` 桌面与移动端操作列，接入 `NodeOpsDrawer` 并补齐移动端启/停操作。
  - 从 `web/src/views/RoutingView.vue` 移除全部 GeoData 卡片与轮询逻辑；将本机 Master GeoData 维护面板迁入 `web/src/views/SettingsView.vue` ① 区紧邻 `xray_geodata_dir`，保存后刷新状态并在 `onUnmounted` 清理定时器。
- [ ] **B4. P2-3 & P2-4 收尾 + 前端全量门禁**
  - `web/src/mock/storage.ts` & `web/src/mock/index.ts`：补齐演示 `nodes` 状态与全套 `/admin/nodes` CRUD/`rotate-token`/`reset-traffic`/`install-script` Mock 路由。
  - `web/src/views/nodes/components/NodeFormDrawer.vue` & `NodeOpsDrawer.vue`：补充 Agent 离线期间物理网卡流量兜底口径提示（P2-4）。
  - 运行 `npm run format`、`npm run typecheck`、`npm run lint`、`npm run format:check`、`npm run build`。

## Validation Commands
```bash
# Backend, Agent & Deploy gates
mise x -- go test -count=1 ./...
CGO_ENABLED=1 mise x -- go test -race -count=1 ./...
mise x -- go vet ./...
mise x -- go build -o NUL .
mise x -- go build -o NUL ./cmd/agent
go list -deps ./cmd/agent | Select-String "gin|gorm|sqlite|telegram"
mise x -- node --test deploy/gateway.test.js

# Frontend gates
cd web && mise x -- npm run typecheck
cd web && mise x -- npm run lint
cd web && mise x -- npm run format:check
cd web && mise x -- npm run build
```

## Risky Files & Rollback Points
- `internal/domain/repository.go`：修改 `NodeRepository` 会波及所有注入点与测试 mock，需全量编译 `./...` 验证。
- `internal/pkg/geodata/`：严禁引入任何非 stdlib 依赖，否则会破坏 `cmd/agent` 依赖隔离门禁。
- `web/src/views/nodes/components/NodeOpsDrawer.vue` & `SettingsView.vue`：务必在抽屉关闭/节点切换/`onUnmounted` 清理全部 `setInterval` 定时器。
