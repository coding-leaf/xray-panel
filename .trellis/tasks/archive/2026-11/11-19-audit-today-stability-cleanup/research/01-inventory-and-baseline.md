# 今日改动全景盘点与质量门禁基线（01）

- **Task**: `.trellis/tasks/11-19-audit-today-stability-cleanup`
- **Audit Date**: `2026-11-19`（统计至 `2026-11-19 22:09 +08:00`）
- **Commit Boundary**: 基线为上一开发日提交 `a978ede`（`2026-11-16 17:09:45 +0800 docs(release): 发布 v2.6.10 更新日志与版本元数据`）；今日范围为 `a978ede..HEAD`，共 **16 个提交**（时间跨度 `2026-11-19 18:43:10` 至 `21:57:11 +0800`）+ 当前工作区未提交变更。

---

## 1. 提交时间线与边界说明

### 1.1 今日 16 个 Git 提交明细

| # | Commit | 时间 (`+0800`) | 分类 | 变更规模 (非 dist/trellis / 全部) | 主题摘要 |
|---|---|---|---|---|---|
| 1 | `e3bf065` | `18:43:10` | Feat (M1) | 36 文件 `+5347/-49`（含 spec 共 37 文件） | 多节点模型、Master 同步控制面与轻量 `xray-agent` 二进制 |
| 2 | `ee162a6` | `18:43:11` | Chore/Task | 6 `.trellis` 文件 `+375` | 归档 `11-19-m1-node-model-and-agent-sync` |
| 3 | `e76747f` | `19:05:32` | Feat (M2) | 22 文件 `+2240/-115` | 订阅直连多节点聚合、UA/Clash 传输层兜底、Nodes 页面 UI |
| 4 | `b52b234` | `19:05:32` | Chore/Task | 7 `.trellis` 文件 `+372` | 归档 `11-19-m2-direct-sub-aggregation-and-node-ui` |
| 5 | `b64734b` | `19:32:50` | Feat (M3) | 28 文件 `+3186/-137` | 节点月度双口径配额、阶梯 TG 告警、自动/离线熔断、TG `/nodes` |
| 6 | `bbfb5e4` | `19:32:51` | Chore/Task | 7 `.trellis` 文件 `+462` | 归档 `11-19-m3-node-quota-alert-and-circuit-breaker` |
| 7 | `32f7f4b` | `19:33:16` | Chore | 2 `.trellis` 文件 `+28/-3` | 记录开发日志 |
| 8 | `d23b104` | `19:39:54` | Chore/Web | 6 源码/静态页 `+1479/-636` + `web/dist` | Portal 页面同步更新与 `web/dist` 重编译 |
| 9 | `f7d7d06` | `20:41:56` | Fix | 2 文件 `+26/-6` | 修复 `xray -test` 临时配置保留 `.json` 后缀与 `0644` 权限 |
| 10 | `b0330a6` | `20:43:57` | Chore | 2 文件 `+3/-3` | 默认 Agent 心跳同步间隔调整为 `60s`、在线窗口 `180s` |
| 11 | `5c9f559` | `21:24:35` | Feat | 33 源码文件 `+3891/-215` + `web/dist` | 全局节点切换器、节点全量配置/日志下发与中转静态密钥自动注入 |
| 12 | `894737e` | `21:24:36` | Chore/Task | 7 `.trellis` 文件 `+442` | 归档 `11-19-multi-node-full-config-and-switcher` |
| 13 | `a7ce0bf` | `21:24:37` | Chore | 2 `.trellis` 文件 `+27/-2` | 记录开发日志 |
| 14 | `b77f0de` | `21:57:09` | Feat/Fix | 26 源码文件 `+2864/-195` + `web/dist` (36 变更) | 配置同步状态徽章与即时 Ack、多节点拓扑修复、远端 GeoData 更新 |
| 15 | `aab5b67` | `21:57:10` | Chore/Task | 7 `.trellis` 文件 `+395` | 归档 `11-19-node-polish-sync-topology-geo` |
| 16 | `3977133` | `21:57:11` | Chore | 2 `.trellis` 文件 `+25/-2` | 记录开发日志 |

### 1.2 累计 Diff 统计（`a978ede..HEAD`）

| 范围 | 变更文件数 | 新增行数 | 删除行数 |
|---|---:|---:|---:|
| **总计 (`git diff --shortstat a978ede..HEAD`)** | 173 | 20,826 | 1,016 |
| **业务源码与静态页 (`:!*web/dist :!*.trellis`)** | 93 | 18,631 | 951 |
| **前端构建产物 (`web/dist`, `HEAD` 含 33 跟踪文件)** | 43 | 66 | 61 |
| **Trellis 文档与归档 (`.trellis`)** | 37 | 2,129 | 4 |

### 1.3 当前工作区未提交改动 (`git status -sb`)

- 分支状态：`master...origin/master [ahead 16]`（无暂存 staged 修改）。
- `M .trellis/.template-hashes.json`：新增 `+28/-1` 行，主要记录了 `.pi/*` 模板哈希以及 **20 条 `.trellis/scripts/common/__pycache__/*.cpython-311.pyc` 编译字节码哈希**（属于本地脚本运行污染）。
- `?? .trellis/tasks/11-19-audit-today-stability-cleanup/`：本次审计任务目录。
- `?? .trellis/tasks/11-19-m4-invite-code-onboarding/`：M4 邀请码自助开户草稿任务（`prd.md`、`task.json`、`implement.jsonl`、`check.jsonl`）。
- `?? .trellis/tasks/11-19-multi-node-direct-agent/`：多节点直连方案父规划目录（含 `prd.md`、`design.md`、`task.json`、`research/codebase-audit.md`）。

---

## 2. 分模块今日改动全景清单

### 2.1 领域模型与持久化层

| 文件 | 来源提交 | 职责与核心变更 |
|---|---|---|
| `internal/domain/node.go` | `e3bf065`,`b64734b`,`b0330a6`,`5c9f559`,`b77f0de` | 定义 `Node`、`NodeTrafficDaily`、`NodeTrafficBatch`、同步协议 DTO、`RefreshOnline`（180s 窗口）、`RefreshSyncStatus`、`CalcTrafficByMode`、`ReachedQuotaThreshold` |
| `internal/domain/inbound.go` | `e3bf065` | `Inbound` 新增 `NodeID uint`（默认 `0` 表示主控机本机） |
| `internal/domain/repository.go` | `e3bf065`,`b64734b`,`5c9f559` | 新增 `NodeRepository`、`NodeTrafficBatchRepository`、`NodeQuotaRepository`、`NodeConfigRepository` 4 个接口；`InboundRepository` 增加 `CountByNodeID` |
| `internal/adapter/repository/node_repo.go` | `e3bf065`,`b64734b`,`5c9f559`,`b77f0de` | 实现 `Node` CRUD、幂等 `ApplyNodeTrafficBatch`（按日写入 `NodeTrafficDaily`）、`UpdateHeartbeat`、`UpdateAutoState`、`ResetCycleTraffic`、`UpdateNodeConfig`、未接入调度的 `PruneBatchesBefore` |
| `internal/adapter/repository/inbound_repo.go` | `e3bf065` | 新增 `FindByNodeID` 与 `CountByNodeID` 实现（但未加入 `InboundRepository` 接口） |
| `internal/adapter/repository/sqlite_db.go` | `e3bf065`,`b64734b` | `AutoMigrate` 纳入 `Node`、`NodeTrafficBatch`、`NodeTrafficDaily` |
| `internal/adapter/repository/node_repo_test.go` | `e3bf065`,`b64734b`,`5c9f559`,`b77f0de` | 572 行仓库单元测试 |

### 2.2 Xray 编译、订阅与协议适配层

| 文件 | 来源提交 | 职责与核心变更 |
|---|---|---|
| `internal/adapter/xray/compiler.go` | `e3bf065`,`5c9f559` | 新增 `CompileRemoteNodeConfig`、`CompileRemoteNodeFullConfig`（含 `api` 入站 `8080`、静态中转客户端注入）、`BuildRemoteNodeUsers` |
| `internal/adapter/xray/compiler_test.go` | `5c9f559` | 远端节点全量编译与静态中转客户端注入测试 |
| `internal/service/sub_service.go` | `e76747f` | 订阅导出支持聚合在线且可路由直连节点（`resolveActiveNodeMap`、`prepareInboundForNode`、`buildRelayOutboundNodeMap`） |
| `internal/sub/clash.go` | `e76747f` | Clash 导出对 `xhttp`/`splithttp`/unsupported transport 做降级跳过而非失败 |
| `internal/service/sub_service_test.go`, `internal/sub/clash_test.go`, `internal/sub/sub_test.go` | `e76747f` | 节点聚合、熔断隐藏与 Clash 兜底测试 |

### 2.3 Master 核心编排与通知/定时任务

| 文件 | 来源提交 | 职责与核心变更 |
|---|---|---|
| `internal/service/node_service.go` | `e3bf065`,`b64734b`,`b0330a6`,`5c9f559`,`b77f0de` | 1,427 行核心服务：节点 CRUD/Token 轮换/安装脚本生成、快照缓存、Agent 心跳同步状态机、双口径配额自动摘流/熔断、按需远程日志（25s wait）与远程 GeoData 下发（75s wait） |
| `internal/service/config_service.go` | `e3bf065`,`5c9f559` | 按 `nodeID` 分流 Outbounds/Routing/DNS/Raw Preview 读取与保存，自动提取 Master 静态中转账号注入远端快照 |
| `internal/service/user_service.go` | `e3bf065` | 用户/入站变更触发 `NodeService.InvalidateNodeCache` / `InvalidateAllNodeCaches` |
| `internal/service/alert_service.go` | `b64734b` | `CheckNodeQuotasAt`（80% 预警 + 耗尽预估）、`NotifyNodeStateChange` 节流 Telegram 通知 |
| `internal/delivery/cron/traffic_sync.go` | `b64734b` | 定时触发 `CheckAndResetMonthlyTraffic` 与 `CheckNodeQuotas` |
| `internal/adapter/telegram/bot.go` | `b64734b` | 新增 `/nodes`、`/nodedrain`、`/noderestore` 指令及单元测试（`bot_test.go`） |
| `main.go` | `e3bf065`,`e76747f`,`b64734b`,`5c9f559`,`b77f0de` | 装配 `NodeRepo`、`NodeService` 并关联至 `ConfigService`、`UserService`、`SubService`、`AlertService`、`BotHandler`、`LogHandler`、`GeoDataHandler` |
| 单元测试 (`*_test.go`) | `e3bf065`..`b77f0de` | `node_service_test.go`(1,097)、`config_service_test.go`(+171)、`alert_service_test.go`(+128) |

### 2.4 HTTP API 与鉴权边界

| 文件 | 来源提交 | 职责与核心变更 |
|---|---|---|
| `internal/delivery/http/router.go` | `e3bf065`,`b64734b` | 挂载 `/api/agent/*`（Bearer NodeToken 鉴权）与 `/api/admin/nodes/*`（Admin JWT 鉴权） |
| `internal/delivery/http/middleware_agent.go` | `e3bf065` | `AgentAuthMiddleware` 基于 SHA-256 Token 校验并注入 `nodeID` |
| `internal/delivery/http/handler_agent.go` | `e3bf065` | `POST /api/agent/sync` 与 `GET /api/agent/health` |
| `internal/delivery/http/handler_node.go` | `e3bf065`,`b64734b` | `/admin/nodes` CRUD、ResetTraffic、RotateToken、InstallScript |
| `internal/delivery/http/handler_config.go`, `handler_outbound.go`, `handler_routing.go`, `handler_dns.go`, `handler_inbound.go`, `handler_log.go`, `handler_geodata.go`, `handler_sub.go` | `e3bf065`,`e76747f`,`5c9f559`,`b77f0de` | 支持 `?nodeId=` 查询/修改分流、远程节点日志与远程 GeoData 下发、订阅路由 Client-UA 透传 |
| `internal/delivery/http/agent_e2e_test.go`, `handler_agent_test.go`, `handler_geodata_test.go`, `handler_sub_test.go` | `e3bf065`..`b77f0de` | 端到端测试（其中 `agent_e2e_test.go` 达 1,807 行） |

### 2.5 独立轻量 Agent 二进制 (`cmd/agent` + `internal/agent`)

| 文件 | 来源提交 | 职责与核心变更 |
|---|---|---|
| `cmd/agent/main.go` | `e3bf065`,`b64734b`,`5c9f559`,`b77f0de` | Agent CLI 入口（依赖隔离，仅引入 stdlib `embed`，零 Gin/GORM/SQLite/TG） |
| `internal/agent/protocol.go`, `client.go` | `e3bf065`,`b64734b`,`5c9f559`,`b77f0de` | Agent JSON 协议定义与 HTTP Client |
| `internal/agent/runner.go` | `e3bf065`,`b64734b`,`5c9f559`,`b77f0de` | 心跳编排器（拉取配置 -> reload/gRPC 增量用户 -> 采集流量/主机指标 -> 离线配额熔断 -> 日志/GeoData 响应） |
| `internal/agent/config_manager.go` | `e3bf065`,`f7d7d06`,`b77f0de` | 原子写配置、`xray -test`（保留 `.json` 后缀、`0644` 权限）、失败回滚、`systemctl` 重载 |
| `internal/agent/user_sync.go`, `xray_grpc_adapter.go` | `e3bf065` | 增量比对本地缓存用户列表并调用本机 Xray gRPC Add/Remove |
| `internal/agent/spool.go`, `quota_store.go`, `metrics*.go` | `e3bf065`,`b64734b` | `pending_batch.json` 断网补偿、本地离线配额账本与熔断、Linux `/proc` & NIC 增量采集 |
| `internal/agent/log_collector.go`, `geodata_updater.go` | `5c9f559`,`b77f0de` | Journalctl/文件日志采集；Mirror 下载与校验回滚替换 `geoip.dat`/`geosite.dat` |
| `internal/agent/*_test.go` | `e3bf065`..`b77f0de` | 对应 8 个测试文件（共 2,119 行） |

### 2.6 前端 Web UI 与静态 Portal (`web/` + `deploy/`)

| 文件 | 来源提交 | 职责与核心变更 |
|---|---|---|
| `web/src/composables/useActiveNode.ts` | `5c9f559`,`b77f0de` | 全局激活节点上下文（`activeNodeId` 存 `localStorage`，含 `5s` pending 轮询） |
| `web/src/App.vue`, `web/src/router/index.ts`, `web/src/api/index.ts` | `e76747f`,`5c9f559`,`b77f0de` | 新增 `/nodes` 路由、顶栏/侧栏全局节点切换器与同步状态徽章 |
| `web/src/api/nodes.ts`, `web/src/views/NodesView.vue`, `web/src/views/nodes/**` | `e76747f`,`b64734b`,`b77f0de` | 节点管理页（节点列表、配额水桶、状态控制、配额/安装脚本抽屉、Token 轮换弹窗） |
| `web/src/views/{InboundsView,OutboundsView,RoutingView,DNSView,ConfigView,LogsView,TopologyView}.vue` 及子组件 | `e76747f`,`5c9f559`,`b77f0de` | 按 `activeNodeId` 切换节点数据源，新增远端日志查询、RoutingView 远端 GeoData 触发、TopologyView 远端标识适配 |
| `web/src/views/PortalClaimView.vue`, `UserShareModal.vue`, `style.css` | `d23b104` | Portal 领取页与分享弹窗样式更新 |
| `deploy/cloudflare-pages/portal.html`, `portal/index.html`, `deploy/standalone-portal.html` | `d23b104` | 三份静态 Portal HTML 同步修改（内容 SHA-256 完全一致） |
| `web/dist/**` | `d23b104`,`5c9f559`,`b77f0de` | 重新构建并提交静态前端资源（当前 33 个跟踪文件） |

---

## 3. 质量门禁基线执行结果（只读实测）

全部命令均在本轮审查期间（`2026-11-19 22:08~22:09 +08:00`）于工作区根目录/`web/` 实测完成：

| 门禁命令 | 状态 | 实测输出摘要 | 备注 |
|---|---|---|---|
| `mise x -- go test ./...` | **PASS** (exit 0) | 所有含测试包通过；`panel`、`cmd/agent`、`internal/pkg/logger`、`flatted` 为 `[no test files]` | 本次重跑命中 Go test cache (`(cached)`)，未改动代码 |
| `mise x -- go vet ./...` | **PASS** (exit 0) | 零警告输出 | — |
| `CGO_ENABLED=1 mise x -- go test -race ./...` | **PASS** (exit 0) | 所有含测试包通过（`(cached)`） | 竞态检测器下无报错 |
| `go list -deps ./cmd/agent \| Select-String "gin\|gorm\|sqlite\|telegram\|embed"` | **PASS** | 仅输出 `embed`（标准库） | 确认 `cmd/agent` 零重型依赖泄漏 |
| `cd web && mise x -- npm run typecheck` | **PASS** (exit 0) | `vue-tsc --noEmit` 零类型错误 | — |
| `cd web && mise x -- npm run lint` | **PASS** (exit 0) | `eslint .` 零 Lint 报错 | — |
| `cd web && mise x -- npm run format:check` | **FAIL** (warn/exit 1) | `prettier --check` 报 **24 个前端文件** 格式不规范 | **全为今日新增/修改的 `.ts`/`.vue` 文件** |
| `cd web && mise x -- npm run build` | **未运行** | 为避免污染 Git 跟踪的 33 个 `web/dist` 构建产物，审计阶段跳过 | `b77f0de` 已包含对应构建产物 |

### Prettier 格式失败文件清单（24 个）
`src/api/index.ts`, `src/api/nodes.ts`, `src/App.vue`, `src/composables/useActiveNode.ts`, `src/router/index.ts`, `src/views/ConfigView.vue`, `src/views/DNSView.vue`, `src/views/inbounds/components/InboundFormDrawer.vue`, `src/views/inbounds/components/InboundTable.vue`, `src/views/inbounds/composables/useInboundList.ts`, `src/views/inbounds/types.ts`, `src/views/InboundsView.vue`, `src/views/LogsView.vue`, `src/views/nodes/components/NodeFormDrawer.vue`, `src/views/nodes/components/NodeTokenModal.vue`, `src/views/nodes/composables/useNodes.ts`, `src/views/nodes/types.ts`, `src/views/NodesView.vue`, `src/views/OutboundsView.vue`, `src/views/RoutingView.vue`, `src/views/topology/components/OutboundNodeCard.vue`, `src/views/topology/components/TopologyPoolsView.vue`, `src/views/topology/types.ts`, `src/views/TopologyView.vue`。
