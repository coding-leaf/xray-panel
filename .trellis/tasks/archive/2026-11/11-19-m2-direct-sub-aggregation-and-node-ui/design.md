# M2 Technical Design: Multi-Node Direct Subscription Aggregation & Node UI

## 1. Architecture & Parallel Slice Boundaries

M2 天然拆分为 **2 个 100% 文件互斥的并行 Slice**：

1. **Slice 1: Backend Subscription Aggregation & Client-Aware Export (`implementer-backend`)**
   - `internal/service/sub_service.go` & `internal/service/sub_service_test.go`
   - `internal/delivery/http/handler_sub.go` & `internal/delivery/http/handler_sub_test.go`
   - `internal/sub/clash.go` & `internal/sub/clash_test.go`
   - `main.go`（仅追加 `.WithNodeRepo(nodeRepo)` 接线）
2. **Slice 2: Vue 3 Node Management UI & Inbound Node Binding (`implementer-frontend`)**
   - 新增：`web/src/api/nodes.ts`、`web/src/views/NodesView.vue`、`web/src/views/nodes/types.ts`、`web/src/views/nodes/composables/useNodes.ts`、`web/src/views/nodes/components/NodeFormDrawer.vue`、`web/src/views/nodes/components/NodeTokenModal.vue`、`web/src/views/nodes/components/NodeInstallScriptModal.vue`（或合并进同一 Modal）
   - 修改：`web/src/api/index.ts`、`web/src/router/index.ts`、`web/src/App.vue`、`web/src/views/inbounds/types.ts`、`web/src/views/InboundsView.vue`、`web/src/views/inbounds/components/InboundFormDrawer.vue`、`web/src/views/inbounds/composables/useInboundList.ts`
   - **严禁触碰**：`web/src/style.css`、`web/src/views/PortalClaimView.vue`、`web/src/views/users/components/UserShareModal.vue`、`deploy/**`，且 worktree commit 中**严禁提交 `web/dist/**`**（避免与主工作区已有 `web/dist/` 冲突，由 `verifier` 在主工作区统一构建验证）。

---

## 2. Backend Design Details

### 2.1 `SubService` Multi-Node Aggregation
- 在 `SubService` 中增加字段 `nodeRepo domain.NodeRepository` 与链式方法：
  ```go
  func (s *SubService) WithNodeRepo(repo domain.NodeRepository) *SubService {
      s.nodeRepo = repo
      return s
  }
  ```
- 抽取纯辅助方法 `resolveActiveNodeMap(ctx) (map[uint]domain.Node, error)`：
  - 当 `s.nodeRepo == nil` 时返回空 map（完全兼容旧单测）；
  - 当 `s.nodeRepo != nil` 时调用 `s.nodeRepo.ListAll(ctx)` 建表。
- 过滤与转换规则（同时覆盖 `resolveSubscriptionNodesWithSettings` 与 `GetUserShareInfo`）：
  1. 对 `ib.NodeID == 0`：沿用原 `hostDomain` 与原 `Remark`/`SubRoute` 转换，0 行为变化；
  2. 对 `ib.NodeID > 0`：
     - 仅放行 `node, ok := nodeMap[ib.NodeID]` 且 `node.Enabled && node.DesiredState == domain.NodeStateActive`；
     - 拷贝入站副本 `ibCopy := ib`：
       - 若 `strings.TrimSpace(ibCopy.ExternalHost) == ""` 且 `strings.TrimSpace(node.Host) != ""`，设置 `ibCopy.ExternalHost = strings.TrimSpace(node.Host)`；
       - 计算直连前缀 `prefix := formatNodePrefix(node)`（优先 `node.Region`，次选 `node.Name`，格式 `[%s]`）；若 `ibCopy.Remark` 不包含该前缀，则设为 `prefix + " " + baseName`（`baseName` 为 `ibCopy.Remark` 或 `ibCopy.Tag`）。
     - 这样无需改动 `protocol.InboundsToNodeConfigsWithDefaultFP` 签名，直接复用既有全部排序、权限（`User.HasInbound`）、指纹与模板能力！

### 2.2 `handler_sub.go` UA Auto-Detection
- 抽取纯函数 `detectFormatFromUA(userAgent string) string`：
  - 仅在 `format == ""` 时生效（显式 `?format=` 或 `?type=` 始终优先）；
  - 小写匹配：
    - 含 `clash` / `mihomo` / `stash` → `"clash"`
    - 含 `sing-box` / `singbox` → `"sing-box"`
    - 其他 → `""`（由导出器走默认 `base64`）
- `Content-Type` 响应头依据最终生效的 `resolvedFormat` 判定。

### 2.3 `internal/sub/clash.go` Safe Skip & Fallback
- 新增辅助函数 `isClashUnsupportedNode(node *protocol.NodeConfig) bool`：
  - 明确识别不适配 Clash 的最新 Xray 传输层：`net := strings.ToLower(strings.TrimSpace(node.Network))` 为 `"xhttp"` 或 `"splithttp"` 时返回 `true`。
- 在 `ExportClashWithParams` 遍历 `nodes` 时：
  - 若 `isClashUnsupportedNode(node)` 为 `true`，记录 `slog.Debug` 并 `continue` 跳过；
  - 若 `protocol.ToClash(node)` 返回 error，记录 `slog.Warn` 并 `continue` 跳过，不再中止整份 YAML；
  - 当最终 `len(proxyNames) > 0` 时，`PROXIES` 组将 `"AUTO"` 置于首位（`[]string{"AUTO", ...proxyNames}`）实现默认自动测速切换；
  - 当最终 `len(proxyNames) == 0` 时，保留 `DIRECT` 兜底并在输出 YAML 顶部附加注释：
    `# Notice: No Clash-compatible proxies available (latest Xray protocols like XHTTP are not supported by Clash). Please use v2rayN, Shadowrocket, Happ, or sing-box.\n`

---

## 3. Frontend Design Details

### 3.1 Strict TypeScript Contract (`web/src/api/nodes.ts`)
严格对齐 M1 `internal/domain/node.go` 与 `internal/delivery/http/handler_node.go`：
- `NodeDesiredState = 'active' | 'draining' | 'circuit_broken'`
- `NodeItem`: `id`, `name`, `host`, `region`, `tokenPrefix`, `enabled`, `desiredState`, `lastSeenAt`, `agentVersion`, `xrayVersion`, `lastConfigHash`, `lastUsersHash`, `lastSyncError`, `cpuPercent`, `memPercent`, `nicUpBytes`, `nicDownBytes`, `xrayUpBytes`, `xrayDownBytes`, `remark`, `createdAt`, `updatedAt`, `online`
- `CreateNodePayload`, `CreateNodeResult` (`NodeItem & { token: string }`), `UpdateNodePayload`, `RotateTokenResult` (`{ nodeId: number; tokenPrefix: string; token: string }`), `InstallScriptResult` (`{ nodeId: number; script: string }`)
- API 函数：`listNodes()`, `getNode(id)`, `createNode(payload)`, `updateNode(id, payload)`, `deleteNode(id)`, `rotateNodeToken(id)`, `getNodeInstallScript(id, masterUrl?)`

### 3.2 UX & Component Reuse
- 严格遵守 `.trellis/spec/frontend/` 规范：零 Pinia、`<script setup lang="ts">`、复用 `Modal.vue`、`Drawer.vue`、`Badge.vue`、`toast`、`copyText`。
- **NodesView.vue**：
  - 顶部概览卡片（总节点数 / 在线数 / 活跃可分发数 / 累计直连流量）+ 「新增节点」主按钮 + 手动刷新按钮；
  - 节点表格/卡片列表：在线圆点徽章、状态徽章（`active` 绿 / `draining` 黄 / `circuit_broken` 红）、主机/地区、CPU & 内存%、Xray 与网卡总流量、Token 前缀、同步异常浮标（`lastSyncError`）、快捷操作（编辑、安装脚本、轮换 Token、删除）；
  - `NodeTokenModal.vue`：创建节点或轮换 Token 后弹出，明确提示“此明文 Token 仅展示一次，关闭后不可再次查看”，提供一键复制 Token 及直接一键复制完整带 Token 的安装命令（自动拼接 `window.location.origin` 作为默认 `masterUrl`，可编辑）。
- **InboundsView & InboundFormDrawer**：
  - `Inbound` 类型补充 `nodeId?: number`；
  - `InboundFormDrawer` 加载节点列表，提供「归属节点」下拉项（默认 `0 — 本机 Master` + 各远端节点 `name (host)`）；新建/编辑时透传 `nodeId`；
  - `InboundsView` 顶部新增按节点筛选条，且每条入站卡片/行展示清晰节点徽章（`本机` 或远端节点名）。
