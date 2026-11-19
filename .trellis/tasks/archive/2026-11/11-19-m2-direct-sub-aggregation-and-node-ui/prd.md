# M2: Multi-Node Direct Subscription Aggregation & Node UI

> Parent: `11-19-multi-node-direct-agent` | Depends on: `11-19-m1-node-model-and-agent-sync` (Completed)

## Goal

打通「多机直连节点分发 + 后台可视化运维」的完整闭环：用户拉取订阅即可同时获得主机线路与所有健康远端直连节点（直接指向各节点公网 Host/IP，主机炸了用户可自行切换直连节点），同时修复 Clash 客户端直接拉取 `/sub/:token` 报错问题，并交付轻量直观的 Vue 3 节点管理与入站归属绑定界面。

## Confirmed Facts

- M1 已提交（`e3bf065`）并归档：后端已提供 `/api/admin/nodes*` 全套接口（CRUD、Token 轮换、安装脚本）与 `Inbound.NodeID` 持久化支持（参见 [m2-audit.md](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-19-m2-direct-sub-aggregation-and-node-ui/research/m2-audit.md)）。
- 现有 `SubService` 仅注入 `userRepo`/`inboundRepo`/`settingRepo`（`internal/service/sub_service.go:17-29`），尚未按 `Node` 状态过滤或回退 `Node.Host`。
- 现有 `handler_sub.go:34-66` 无 `User-Agent` 识别，`clash.go:46-49` 遇不兼容协议直接中断整份导出；产品定位明确**天然不向 Clash 妥协 Xray 最新协议**，Clash 仅需安全跳过不兼容节点且不崩溃。
- 工作区存在用户既有未提交前端/部署改动（`web/src/style.css`、`PortalClaimView.vue`、`UserShareModal.vue`、`deploy/*`），M2 严禁触碰或覆盖这些文件。

## Requirements

### R1. 多机直连节点订阅与分享链接聚合
- `SubService` 增加可选 `WithNodeRepo(nodeRepo domain.NodeRepository) *SubService`（保持既有 `NewSubService` 构造签名零破坏，并在 `main.go` 装配）。
- 在 `resolveSubscriptionNodesWithSettings` 与 `GetUserShareInfo` 中对每个 `Inbound` 按 `NodeID` 处理：
  - `NodeID == 0`：本机 Master 节点，完全保持现有 `hostDomain` 与 `SubRoute` 逻辑；
  - `NodeID > 0`：查询对应 `Node`，仅当 `node.Enabled && node.DesiredState == domain.NodeStateActive` 时纳入输出（自动过滤已禁用、`draining`、`circuit_broken` 或不存在的节点）；
  - **Host 回退优先级**：`Inbound.ExternalHost` > `Node.Host` > 全局 `sub_domain`/`public_url`/`reqHost`；
  - **直连节点清晰标识**：`NodeID > 0` 且原节点名未带对应前缀时，自动加 `[<Region>] `（若 `Region` 为空则用 `[<Node.Name>] `）前缀，便于用户在客户端一眼区分各直连节点。

### R2. 订阅客户端识别与 Clash 安全降级（不妥协最新协议）
- `handler_sub.go`：当 URL 未显式指定 `format`/`type` 时，自动检测 `User-Agent`（不区分大小写）：
  - 含 `clash` / `mihomo` / `stash` → `clash`（响应 `Content-Type: application/yaml; charset=utf-8`）；
  - 含 `sing-box` / `singbox` → `sing-box`（响应 `Content-Type: application/json; charset=utf-8`）；
  - 其他（含 v2rayN / Shadowrocket / Happ / curl 等）→ 默认 `base64`（保留全部最新 Xray 协议与参数）。
- `internal/sub/clash.go`：
  - 遇到 Clash 不支持的传输层（`xhttp` / `splithttp`）或 `protocol.ToClash(node)` 返回错误时，记录日志并安全跳过该节点，不再打断整份 YAML 生成；
  - 若过滤后无任何兼容 Clash 的代理节点，输出合法 YAML（保留 `DIRECT` 兜底并在顶部附带清晰注释提示“最新 Xray 协议不适配 Clash，请改用 v2rayN / Shadowrocket / Happ / sing-box”），杜绝客户端 `invalid yaml` 崩溃。
  - 在 Clash 策略组中，有兼容节点时将 `AUTO`（`url-test`）加入 `PROXIES` 首位以便自动选线。

### R3. Vue 3 节点管理页面与入站节点归属绑定
- 新增 `web/src/api/nodes.ts`（严格对齐 M1 后端 `/api/admin/nodes*` 契约）并在 `web/src/api/index.ts` 导出。
- 新增路由 `/nodes`（`web/src/router/index.ts`）与侧边栏导航入口（`web/src/App.vue` `navSections`）。
- 交付 `web/src/views/NodesView.vue` 及模块化子组件（沿用 `inbounds/` 目录风格，无 Pinia）：
  - 节点列表：展示节点名、公网 `host`、地区、在线状态徽章（`online` + `lastSeenAt`）、期望状态（`active`/`draining`/`circuit_broken`）、Agent/Xray 版本、CPU/内存%、累计 Xray/网卡流量、最近同步错误提示；
  - 节点新增/编辑 Drawer（`NodeFormDrawer.vue`）：编辑 `name`、`host`、`region`、`enabled`、`desiredState`、`remark`；
  - 一次性 Token 与一键部署脚本弹窗（`NodeTokenModal.vue`）：创建节点或点击「轮换 Token」后仅此一次高亮展示明文 Token 并支持一键复制；点击「安装脚本」调用 `GET /api/admin/nodes/:id/install-script?masterUrl=...` 展示并一键复制安装命令；
  - 删除节点时友好展示后端 `409`（存在关联入站）错误信息。
- 改造入站管理（`web/src/views/InboundsView.vue`、`InboundFormDrawer.vue`、`types.ts`、`useInboundList.ts`）：
  - 入站表单增加「归属节点」下拉选择器（`0 = 本机 Master (默认)` + 所有已注册远端节点）；
  - 入站列表展示所属节点标签并支持按节点筛选（全部 / 本机 / 指定远端节点）。

## Acceptance Criteria

- [ ] **AC-1**：单元测试验证 `SubService`（含 `ExportUserSubscription` 与 `GetUserShareInfo`）正确聚合本机 + `active` 远端节点（自动回退 `Node.Host` 并带地区/节点前缀），且正确过滤 `disabled`/`draining`/`circuit_broken` 节点。
- [ ] **AC-2**：单元测试验证 `handler_sub.go` 按 `User-Agent` 自动返回 `clash` YAML / `sing-box` JSON / `base64`，且显式 `?format=` 优先级高于 UA；`clash.go` 遇到 `xhttp`/`splithttp` 或不支持协议时安全跳过不报错，全跳过时输出合法 YAML 且含 `DIRECT` 兜底。
- [ ] **AC-3**：前端完成节点管理全流程 UI 与入站 `nodeId` 选择/筛选，未触碰任何用户既有脏文件（`style.css`/`PortalClaimView.vue`/`UserShareModal.vue`/`deploy/*`）。
- [ ] **AC-4**：全量质量门禁全部通过：`mise x -- go test ./...`、`CGO_ENABLED=1 mise x -- go test -race ./internal/service/... ./internal/sub/... ./internal/delivery/http/...`、`mise x -- go vet ./...`、`cd web && mise x -- npm run build`、`mise x -- go build .`。

## Out of Scope

- 节点月流量配额上限、账单日重置与 80%/90%/95% 自动流转告警（属于 M3）。
- 邀请码自助开户与公开用户用量/节点看板（属于 M4）。
- 为 Clash 适配 Xray 最新实验传输协议参数。
