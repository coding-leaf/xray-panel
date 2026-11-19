# 今日开发内容代码与架构审查总报告（2026-11-19）

- **审查任务**：`.trellis/tasks/11-19-audit-today-stability-cleanup`
- **审查日期**：`2026-11-19`
- **审查范围**：`a978ede..HEAD` 共 **16 个 Git 提交**（`18:43:10` 至 `21:57:11 +0800`，覆盖 M1 多节点控制面与 `xray-agent`、M2 订阅聚合与 Nodes UI、M3 月度配额与熔断告警、全局节点切换器/远端全量配置与日志/同步状态徽章与远端 GeoData 更新）+ 当前工作区未提交变更。
- **变更总览**：总计 **173 文件变更（+20,826 / -1,016）**，其中业务源码与静态页面 **93 文件（+18,631 / -951）**、`web/dist` **43 文件变更（HEAD 含 33 个跟踪文件）**、`.trellis` **37 文件（+2,129 / -4）**。

---

## 分卷报告索引

1. **[01-inventory-and-baseline.md](./01-inventory-and-baseline.md)** — 今日 16 个提交时间线、未提交工作区详情、分模块全量改动清单、全部质量门禁实测基线（含 24 个 Prettier 失败清单）。
2. **[02-stability-risks.md](./02-stability-risks.md)** — **P0 (2 项) / P1 (8 项) / P2 (4 项)** 稳定性、并发安全、超时边界、数据一致性及凭证安全缺陷（全部附精确 `file:line` 证据与修复路径）。
3. **[03-redundancy-and-consolidation.md](./03-redundancy-and-consolidation.md)** — 死代码与重复代码剔除清单、KISS/接口碎片化识别、用户交互与工程架构收敛方案（含 Phase A/B/C 治理路线图）。

---

## 执行摘要（TL;DR）

### 1. 质量门禁基线状态
- **Go 后端**：`go test ./...`、`go vet ./...`、`CGO_ENABLED=1 go test -race ./...` 均退出 `0`；`cmd/agent` 依赖隔离校验通过（仅含 stdlib `embed`，零 `gin/gorm/sqlite/telegram` 泄漏）。
- **Web 前端**：`vue-tsc --noEmit` 与 `eslint .` 均退出 `0`；**`npm run format:check` 失败**（24 个今日新建/修改 `.ts`/`.vue` 文件存在 Prettier 格式问题）；为保持工作区只读未重跑 `npm run build`（`b77f0de` 已包含构建产物）。
- **测试盲区提醒**：`agent_e2e_test.go:170, 581` 传入 `xrayManager = nil`，掩盖了 `P0-1` 真实生产路径必现的 gRPC 回滚故障。

### 2. 最关键稳定性发现（Top P0 / P1）
- **P0-1 — 远端入站破坏主控用户写操作与 TG `/adduser`**：`internal/adapter/xray/manager.go:36-66` 未检查 `inbound.NodeID != 0` 直接操作本机 gRPC，而 `user_service.go:101-108` (`isXrayIgnorableAddUserError`) 不忽略 `handler not found`，导致一旦系统启用任意远端入站，`CreateUser`/`UpdateUser`/`ResetTraffic`/`ResetSubToken` 与 TG `/adduser`（`bot.go:385-395`）触发 Saga 回滚失败。
- **P0-2 — 远端按需日志在 60s 默认心跳下结构性失效**：`node_service.go:23-26, 1037-1062` 将等待超时设为 `25s`（远小于 `b0330a6` 调整后的 `60s` 心跳间隔），超时退出时 `defer` 直接 `delete(s.pendingLogReqs, nodeID)` 销毁任务，且单槽位赋值无并发保护，形成无限超时的死循环。
- **P1-1 — 远程 GeoData 更新超时撞墙并阻塞心跳**：`node_service.go:26` 等待 `75s` 超过 `server.go:45` `WriteTimeout = 60s`；Agent `runner.go:203-208` 在 `SyncOnce` 内同步下载（单次 60s + 多镜像回退），可能阻塞超过 `180s` 离线窗口。
- **P1-2 — 远端 `SubRoutes` 未进入 Agent 实际配置与预览**：`config_service.go:1165` 读取 Routing UI 时调用 `ensureSubRouteRules`，但 `node_service.go:886-914` (`getOrBuildSnapshot`) 与 `config_service.go:1471-1480` (`GetNodeCompiledConfigRaw`) 缺失该规范化步骤。
- **P1-3 — 远端 SS / SS-2022 增量热加载密码协议失配**：`compiler.go:440-454` 未传 `Method`，`agent/xray_grpc_adapter.go:37-53` 构造空 `SettingsJSON` 导致 `ResolveAdapter` 退化为 `aes-128-gcm`。
- **P1-4 — 远端直连未配 `ExternalPort` 被覆写为主控 `443`**：`sub_service.go:81-99` 与 `node_converter.go:153-158` 将远端 `ExternalPort == 0` 错误回落至 Master `public_port`。
- **P1-5 / P1-6 / P1-7 / P1-8**：`QuickPublishWizardModal.vue:478` 发往 `/outbounds` 漏传 `?nodeId`；`RollbackSnapshot`/`saveAndApplyRawConfigLocked` 漏失效远端快照缓存；`PruneBatchesBefore`（`node_repo.go:416`）从未调用且 `DeleteNode` 留孤儿日/批记录；`RenderInstallScript`（`node_service.go:685`）通过 `--token` 明文传参且残留 `--interval 20s`、`compiler.go:403` 硬编码 gRPC 端口 `8080`。

### 3. Top 冗余剔除与架构收敛建议
- **立刻可删 / 可去重**：删除零调用函数 `user_service.go:110` (`isXrayUserNotFoundError`)；消除 `findNodeInbounds` (`node_service.go:831-849`) 与 `findInboundsByNode` (`config_service.go:1490-1508`) 双份副本；让 `agent/quota_store.go` 直接复用 `domain.CalcTrafficByMode` / `ReachedQuotaThreshold`；收敛 3 份字节一致的 `deploy/**/portal*.html` 并让 `PortalClaimView.vue` 复用 `utils/clipboard.ts`；清理 `.trellis/.template-hashes.json` 中的 `__pycache__/*.pyc` 未提交污染。
- **核心架构收敛**：
  1. **统一异步节点指令通道（`NodeCommand`）**，替代 `pendingLogReqs` 与 `pendingGeoReqs` 同步等待模型（同步根治 P0-2 & P1-1）；
  2. **统一 `BuildNodeTargetConfig` 编译规范化管道 + 精简 `NodeRepository` 碎片接口**（同步根治 P1-2 & P1-6）；
  3. **明确前端节点切换器作用域（Master 页 vs Node-Scoped 页）+ 合并 `useNodes` 与 `useActiveNode` + 聚合“节点运维抽屉”**（降低 5 页面跳转的心智负担）。

---

## 候选 `.trellis/spec/` 路径清单

供主会话后续在重构/修复任务中一键批量装配 `implement.jsonl` 与 `check.jsonl`：

### Backend Specs
- `.trellis/spec/backend/index.md`
- `.trellis/spec/backend/directory-structure.md`
- `.trellis/spec/backend/database-guidelines.md`
- `.trellis/spec/backend/error-handling.md`
- `.trellis/spec/backend/quality-guidelines.md`
- `.trellis/spec/backend/logging-guidelines.md`

### Frontend Specs
- `.trellis/spec/frontend/index.md`
- `.trellis/spec/frontend/directory-structure.md`
- `.trellis/spec/frontend/api-and-dataflow.md`
- `.trellis/spec/frontend/components-and-routing.md`
- `.trellis/spec/frontend/quality-guidelines.md`

### Cross-Layer & Review Guides
- `.trellis/spec/guides/index.md`
- `.trellis/spec/guides/code-reuse-thinking-guide.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`
- `.trellis/spec/guides/review-3pass-guide.md`
