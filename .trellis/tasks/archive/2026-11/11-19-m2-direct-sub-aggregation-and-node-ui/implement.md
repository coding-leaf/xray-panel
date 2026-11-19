# M2 Implementation Plan: Multi-Node Direct Subscription Aggregation & Node UI

## Execution Strategy & Parallel Slices

本里程碑拆分为 **2 个 100% 文件互斥的并行 Slice**（适配 Swarm `workspace:"share"`），完成后归拢到主工作区并由 `verifier` 执行完整前后端联调、构建与质量门禁。

---

### Slice 1: Backend Subscription Aggregation & Client-Aware Export (`implementer-backend`)

**Owned Files:**
- `internal/service/sub_service.go`
- `internal/service/sub_service_test.go`
- `internal/delivery/http/handler_sub.go`
- `internal/delivery/http/handler_sub_test.go` (new or update)
- `internal/sub/clash.go`
- `internal/sub/clash_test.go`
- `main.go` (only add `.WithNodeRepo(nodeRepo)` to `NewSubService`)

**Ordered Checklist (Slice 1):**
1. 为 `SubService` 增加 `nodeRepo` 与 `WithNodeRepo` 方法，并在 `main.go` 注入 `nodeRepo`。
2. 在 `resolveSubscriptionNodesWithSettings` 与 `GetUserShareInfo` 中落地多节点直连聚合：
   - `NodeID == 0` 行为 100% 不变；
   - `NodeID > 0` 仅放行 `Enabled && DesiredState == "active"` 节点，自动回退 `Node.Host` 并附加 `[Region]` / `[Node.Name]` 前缀。
3. 在 `handler_sub.go` 实现 `detectFormatFromUA`（识别 `clash`/`mihomo`/`stash`、`sing-box`/`singbox`，显式 `?format=`/`?type=` 优先）并修正响应 `Content-Type`。
4. 在 `internal/sub/clash.go` 跳过 `xhttp`/`splithttp` 与 `protocol.ToClash` 失败节点，兼容节点存在时将 `AUTO` 放进 `PROXIES` 首位，全过滤为空时返回带友好注释的合法 `DIRECT` YAML。
5. 补全上述所有场景的单元测试并跑通后端测试、`-race`、`go vet` 与 `go build .`。

---

### Slice 2: Vue 3 Node Management UI & Inbound Node Binding (`implementer-frontend`)

**Owned Files:**
- `web/src/api/nodes.ts` (new)
- `web/src/api/index.ts` (export `./nodes`)
- `web/src/router/index.ts` (add `/nodes` route)
- `web/src/App.vue` (add Node menu item to `navSections`)
- `web/src/views/NodesView.vue` (new)
- `web/src/views/nodes/types.ts` (new)
- `web/src/views/nodes/composables/useNodes.ts` (new)
- `web/src/views/nodes/components/NodeFormDrawer.vue` (new)
- `web/src/views/nodes/components/NodeTokenModal.vue` (new)
- `web/src/views/inbounds/types.ts` (add `nodeId?: number`)
- `web/src/views/InboundsView.vue` (node badge + node filter)
- `web/src/views/inbounds/components/InboundFormDrawer.vue` (node selector `0 = 本机 Master` + remote nodes)
- `web/src/views/inbounds/composables/useInboundList.ts` (load nodes & filter by `nodeId`)

**Strict Prohibitions (Slice 2):**
- **严禁修改** `web/src/style.css`、`web/src/views/PortalClaimView.vue`、`web/src/views/users/components/UserShareModal.vue`、`deploy/**`、`internal/**`、`.trellis/**`；
- 在 worktree 中运行 `cd web && mise x -- npm run build` 验证通过后，**严禁将 `web/dist/**` 提交进分支 commit**（`git checkout -- web/dist` 或仅 `git add web/src`），以免与主工作区现有 `web/dist/` 冲突。

---

### Post-Merge Verification (`verifier`)

1. 在主工作区归拢 Slice 1 与 Slice 2 改动（确保未触碰原有脏文件）；
2. 审查后端订阅聚合逻辑、Clash 降级 YAML 合法性、前端 TS 类型与 M1 后端 `/api/admin/nodes*` 契约一致性；
3. 在主工作区运行完整前后端质量门禁（含 `cd web && mise x -- npm run build` 刷新嵌入产物后执行 `mise x -- go build .`）。

## Validation Commands

```bash
# 后端质量门禁
mise x -- go test ./...
CGO_ENABLED=1 mise x -- go test -race -count=1 ./internal/service/... ./internal/sub/... ./internal/delivery/http/...
mise x -- go vet ./...

# 前端构建与类型检查
cd web && mise x -- npm run build

# 后端全量二进制构建 (嵌入更新后的 web/dist)
mise x -- go build .
mise x -- go build ./cmd/agent
```
