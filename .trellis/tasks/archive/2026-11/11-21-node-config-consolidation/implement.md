# Implementation Plan: 节点命名与配置收束

本次采用 **2 个文件互斥的并行分片 + 独立 Verifier** 推进：
- **Slice 1 (`impl-backend`)**：独占 `internal/**`
- **Slice 2 (`impl-frontend`)**：独占 `web/**`（不碰 `web/dist` 构建留待合流后统一执行，或各自验证 `npm run build` 后由协调者最终重建 `web/dist`）

---

## Slice 1: Backend (`internal/**`)

1. **领域模型与 DTO (`internal/domain/node.go`, `internal/domain/inbound.go`)**：
   - `Node` 新增 `DirectSubEnabled *bool` + `IsDirectSubEnabled() bool`；`CreateNodeDTO` / `UpdateNodeDTO` 新增 `DirectSubEnabled *bool`；
   - `Inbound` 新增 `SubExposed *bool` + `IsSubExposed() bool`。
2. **Service 持久化与默认值 (`internal/service/node_service.go`, `internal/service/inbound_service.go` 及对应 handler/repo 核查)**：
   - `CreateNode` / `UpdateNode` 正确保存 `DirectSubEnabled`（缺省 `&true`，显式 `&false` 落库为 `false`）；
   - `CreateInbound` / `UpdateInbound` 正确保存 `SubExposed`（缺省 `&true`，显式 `&false` 落库为 `false`）；
   - 确认 `CompileRemoteNodeFull` 与 `extractStaticRelayClients` 完全不受 `DirectSubEnabled=false` / `SubExposed=false` 影响。
3. **统一命名引擎与过滤 (`internal/service/sub_service.go`, `internal/protocol/node_converter.go`, `internal/sub/*`)**：
   - `resolveActiveNodeMap` 增加 `&& n.IsDirectSubEnabled()`；
   - `prepareInboundForNode` 增加 `if !ib.IsSubExposed()` 过滤，并**删除**硬编码 `[Region || Name] ` 前缀改写逻辑；
   - `InboundsToNodeConfigsWithDefaultFP` 支持注入节点元数据（`node_name`、`node_region`、`inbound_remark`、`route_name`、`outbound_tag`）；
   - 扩充 `RenderNodeName` / `ApplyNodeNameTemplate` 支持 `{region}`/`{node}`/`{route}`/`{outbound}` + 空括号与空白归一化；
   - 统一 `GetUserShareInfo` 走同一转换与 `ApplyNodeNameTemplate` + `DeduplicateNodeNames` 管线；
   - 新增 `DeduplicateNodeNames` 在重名时追加 ` #2`、` #3`，防止 Clash/Sing-box 导出重名冲突。
4. **单元测试（Fail-Repro First）**：
   - 新增 SQLite 往返测试验证 `DirectSubEnabled=false` / `SubExposed=false` 落库不丢失、订阅/分享剔除而中转编译保留；
   - 更新并新增命名单测：所见即所得无 `[**]` 前缀、自定义模板 `[{region}] {remark}` 与空 `{region}` 归一化、`GetUserShareInfo` 模板一致、重名消歧；
   - 运行 `mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`mise x -- go build ./cmd/agent`。

---

## Slice 2: Frontend (`web/**`)

1. **类型与 Mock 同步**：
   - `web/src/views/nodes/types.ts`、`web/src/api/nodes.ts`、`web/src/views/inbounds/types.ts`、`web/src/api/inbounds.ts`、`web/src/mock/*` 同步加入 `directSubEnabled` 与 `subExposed`；
   - `web/src/views/inbounds/types.ts` 移除 `getDefaultInboundFormData` 中硬编码的 `🇯🇵 日本原生直连` 默认 SubRoute，并支持 `defaultSubExposed` 参数控制 `selectedUserEmails` 默认值。
2. **托管节点页面与称呼统一 (`NodesView.vue`, `NodeFormDrawer.vue`, `NodeOpsDrawer.vue`, `App.vue` / `router/index.ts`)**：
   - 统一将「远端直连节点」改为「托管节点」；`Node.Name` 标注为「节点标识（仅后台管理）」；
   - `NodeFormDrawer.vue` 增加「对外直连订阅 (`directSubEnabled`)」开关与清晰说明（关闭 = 仅作中转落地，不进入任何用户订阅）；
   - `NodesView.vue` 展示「仅中转落地」/「直连开放」状态徽章。
3. **入站管理页面收束 (`InboundTable.vue`, `InboundFormDrawer.vue`)**：
   - `InboundTable.vue` 显示「订阅显示名称」（`remark || tag`）与「公开订阅 / 仅内部中转」徽章；
   - `InboundFormDrawer.vue`：
     - `Remark` 统一标注为「订阅显示名称」，`SubRoute.name` 统一标注为「订阅显示名称（分流线路）」并加覆盖说明；
     - 顶部基础区加入「对外提供订阅 (`subExposed`)」开关（新建跟随当前节点 `directSubEnabled`，节点已关时显示节点级锁定提示）；
     - `subExposed === false` 时隐藏外网连接/排序/分流线路/关联用户区块，保存时不批量授权任何用户。
4. **用户管理防误触 (`UserFormDrawer.vue`)**：
   - 过滤 `subExposed === false` 或所属节点 `directSubEnabled === false` 的入站，不出现在授权勾选和默认全选中。
5. **系统设置命名模板升级 (`SettingsView.vue`)**：
   - `{remark}` 标注为「订阅显示名称（入站或分流线路名）」；新增 `{region}`、`{node}`、`{route}`、`{outbound}` 可点击变量芯片、快捷预设与实时命名预览。
6. **前端自检**：
   - `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build`。

---

## Integration & Final Verification (Coordinator + Verifier)
1. 协调者将 `impl-backend` 与 `impl-frontend` 两个互斥 `share` 分支 `git merge --squash` 归拢到主工作区；
2. 统一重建 `web/dist/` 确保嵌入产物与最新前端源码完全一致；
3. `verifier` 独立执行 3-Pass 审查与全套门禁：
   - `mise x -- go test ./...`
   - `mise x -- go vet ./...`
   - `mise x -- go build -o panel.tmp.exe . && rm -f panel.tmp.exe`
   - `mise x -- go build -o agent.tmp.exe ./cmd/agent && rm -f agent.tmp.exe`
   - `cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build`
