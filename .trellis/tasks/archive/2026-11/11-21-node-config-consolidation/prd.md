# 节点命名与配置收束 (PRD)

## Goal & User Value
解决多节点架构下三大高频运维痛点：
1. **消除混乱命名与称呼割裂**：移除后端对远端节点硬编码强塞 `[Region || Name] ` 的逻辑（告别 `[**]直连`），移除新建入站默认预填的冗余 `🇯🇵 日本原生直连` 分流线路，让节点名称真正「所见即所得」，并全站统一「托管节点 / 节点标识」与「订阅显示名称」称呼；
2. **彻底杜绝中转落地节点误泄露到用户订阅**：引入正交的双层布尔控制（`Node.DirectSubEnabled` 节点总开关 + `Inbound.SubExposed` 入站覆盖开关），让 GCP 等落地节点可一键锁死为「仅中转落地」，绝不进入用户订阅、单节点分享或用户默认全选授权，避免大陆直连流量产生意外费用；
3. **保持极简、低复杂度（KISS）**：聚焦配置与命名收束，不做黑盒默认直连协议，也不在本轮叠加额外跨页一键出站编排。

## Confirmed Facts & Evidence
完整源码审计见 [research/node-config-audit.md](file:///d:/code/Workspace/xray-panel/.trellis/tasks/11-21-node-config-consolidation/research/node-config-audit.md)。
- **命名根因**：`SubService.prepareInboundForNode`（`internal/service/sub_service.go:56-123`）对 `NodeID > 0` 强制拼接 `[Region || Name] ` 到 `Remark` 与 `SubRoute.Name`；前端 `getDefaultInboundFormData`（`web/src/views/inbounds/types.ts:163-173`）为所有新建 VLESS 入站默认注入 `🇯🇵 日本原生直连` SubRoute 覆盖 `Remark`；`sub_node_name_template`（`internal/protocol/node_converter.go:479-517`）缺失 `{region}/{node}/{route}/{outbound}`；`GetUserShareInfo`（`sub_service.go:390-497`）完全绕过命名模板；Clash/Sing-box 导出缺少同名节点消歧。
- **泄露根因**：`InboundFormDrawer` 新建入站默认 `selectedUserEmails: usersList.map(u => u.email)`（`types.ts:179`），保存时写入全部已有用户；`UserFormDrawer` 新建用户默认全选全部多用户入站（`UserFormDrawer.vue:560`）；当前无正交的订阅暴露开关（`Node.Enabled`、`Inbound.Enabled`、`DesiredState=draining` 均有停服/断中转/跳熔断副作用）。
- **互联边界**：`extractStaticRelayClients`（`internal/service/node_service.go:1510-1616`）自动向远端注入 `relay-auto:<obTag>@internal` 静态客户端，独立于用户授权与订阅导出；关闭订阅暴露不会影响中转转发。

## Requirements

### R1: 双层正交订阅暴露控制（后端 Schema + 订阅/分享过滤）
- **`domain.Node` 新增 `DirectSubEnabled *bool`**（GORM `column:direct_sub_enabled;default:true`，JSON `directSubEnabled`，辅助方法 `IsDirectSubEnabled() bool` 在 `nil` 时返回 `true`）：
  - `CreateNodeDTO` / `UpdateNodeDTO` 支持 `DirectSubEnabled *bool`（创建缺省为 `true`；更新时 `nil` 保持原值）；
  - 关闭（`false`）表示该节点定位为「仅中转落地」，`SubService.resolveActiveNodeMap` 直接跳过，从 `/sub`、`/users/:id/share`、`/portal/claim` 彻底剔除，且**不影响** Agent 心跳、配置编译、中转转发与 95% 自动流量熔断保护。
- **`domain.Inbound` 新增 `SubExposed *bool`**（GORM `column:sub_exposed;default:true`，JSON `subExposed`，辅助方法 `IsSubExposed() bool` 在 `nil` 时返回 `true`）：
  - 创建/更新入站支持显式持久化 `false`（规避 GORM `default:true` 吞零值），省略时默认 `true` 兼容旧调用；
  - `SubService.prepareInboundForNode`（同时覆盖 Master `NodeID == 0` 与远端 `NodeID > 0`）若 `!ib.IsSubExposed()` 直接跳过，不进入订阅导出与单节点分享，且**不影响** Xray 监听编译与 `extractStaticRelayClients`。
- **历史数据兼容**：SQLite `AutoMigrate` 新增带 `default:true` 的列，既有节点与入站升级后默认保持开启，不破坏线上现有订阅。

### R2: 统一所见即所得订阅命名引擎与重名消歧
- **移除硬编码前缀**：删除 `SubService.prepareInboundForNode` 中对远端节点强制拼接 `[Region || Name] ` 的逻辑；保留 `ExternalHost` 回退 `node.Host` 与 `ExternalPort` 回退 `ib.Port`。
- **结构化命名元数据与模板扩充**：
  - 将 `node_name`、`node_region`、`inbound_remark`、`route_name`、`outbound_tag` 注入 `protocol.NodeConfig.Params`；
  - 扩充 `RenderNodeName` / `ApplyNodeNameTemplate`（`internal/protocol/node_converter.go`）：默认模板仍为 `{remark}`（所见即所得：有启用分流线路取 `SubRoute.Name`，否则取 `Inbound.Remark`，为空回退 `Inbound.Tag`）；新增占位符 `{region}`、`{node}`、`{route}`、`{outbound}`（兼容既有 `{remark}`、`{tag}`、`{protocol}`、`{port}`、`{index}`），并自动清理因变量为空产生的空括号 `[]`/`()` 与多余空白/孤立分隔符。
- **单出口统一渲染**：`GetUserShareInfo`（`sub_service.go:390-497`）与 `ExportUserSubscription` 共用同一套节点转换与 `ApplyNodeNameTemplate` 渲染逻辑，消除面板分享链接名与订阅链接名不一致。
- **导出重名安全消歧**：在多格式导出（尤其 Clash / Sing-box）前对重复节点名追加稳定序号后缀（如 `名称 #2`），防止 Sing-box / Mihomo 配置因重名报错。

### R3: 前端称呼统一、表单收束与防误触闭环
- **全站称呼统一**：
  - `/nodes`（`NodesView.vue`、`NodeFormDrawer.vue`、`NodeOpsDrawer.vue`、侧栏/标题）：将「远端直连节点」统一改为 **「托管节点」**；`Node.Name` 标注为 **「节点标识（仅后台管理）」**（说明默认不会拼入用户订阅名）；
  - `/inbounds`（`InboundTable.vue`、`InboundFormDrawer.vue`）：将 `Inbound.Remark` 统一标注为 **「订阅显示名称」**（附说明：客户端看到的节点名，留空回退 Tag），并在 `InboundTable` 直接展示「订阅显示名称」与「公开订阅 / 仅内部中转」徽章；将 `SubRoute.name` 统一标注为 **「订阅显示名称（分流线路）」**，并显式提示“添加分流线路后将按各线路的订阅显示名称导出，替代上方单一入站名称”；
  - `/settings`（`SettingsView.vue`）：将 `{remark}` 说明更新为「订阅显示名称（入站或分流线路名）」，补全 `{region}`/`{node}`/`{route}`/`{outbound}` 占位符标签、常用预设模板及**实时渲染预览**。
- **配置收束与防误触**：
  - `NodeFormDrawer.vue`：新增醒目的 **「对外直连订阅 (`directSubEnabled`)」** 开关（开启 = 允许对外直连订阅；关闭 = 仅中转落地，绝不进入用户订阅），`NodesView.vue` 卡片清晰展示「仅中转落地」vs「直连开放」徽章；
  - `web/src/views/inbounds/types.ts`：**移除**新建 VLESS 入站时默认预填的 `🇯🇵 日本原生直连` SubRoute（新建入站默认 `subRoutes: []`）；
  - `InboundFormDrawer.vue`：在顶部基础区放置 **「对外提供订阅 (`subExposed`)」** 开关（新建时默认跟随当前所属节点的 `directSubEnabled`；若所属节点已关闭 `directSubEnabled`，提示节点级已锁死仅中转落地）。**当 `subExposed === false` 时**：自动隐藏/收起 `ExternalHost`、`ExternalPort`、`SortOrder`、`⑤ 分流订阅线路`、`⑥ 关联授权用户`，新建时 `selectedUserEmails` 为空且保存时不批量向用户追加该 Tag；
  - `UserFormDrawer.vue`：授权入站列表过滤掉 `subExposed === false` 或所属节点 `directSubEnabled === false` 的内部中转入站（新建用户「默认全选」绝不再勾中 GCP 落地入站）；
  - 同步更新 `web/src/mock/` 与前端 TypeScript 类型定义，保证 Demo Mock 模式与真实接口类型一致。

## Acceptance Criteria
- [ ] **AC-1（双层暴露硬隔离）**：节点设 `directSubEnabled=false` 或入站设 `subExposed=false` 后，经 SQLite 持久化往返不丢失 `false`，`/sub`（Base64/Clash/Sing-box）与 `/users/:id/share` 均彻底剔除该入站，而 `CompileRemoteNodeFull` 与 `extractStaticRelayClients` 中转编译不受影响；历史未设置数据默认 `true`。
- [ ] **AC-2（所见即所得命名与统一链路）**：默认模板下远端入站不再出现 `[**] ` 强拼前缀，直接展示入站/分流线路的「订阅显示名称」；`ExportUserSubscription` 与 `GetUserShareInfo` 名称完全一致；新占位符 `{region}`/`{node}`/`{route}`/`{outbound}` 正确替换且空值不留空括号；重名节点在 Clash/Sing-box 导出时自动消歧。
- [ ] **AC-3（前端收束与防误触）**：新建 VLESS 入站不再自带 `🇯🇵 日本原生直连`；在「仅中转落地」节点下新建入站默认关闭 `subExposed`、不展示订阅/分流/用户授权区、保存不写入任何用户 `inboundTags`；`UserFormDrawer` 授权列表自动排除内部中转入站；`InboundTable` 展示「订阅显示名称」与暴露徽章；`SettingsView` 支持新占位符与实时预览；全站称呼统一。
- [ ] **AC-4（全量质量门禁）**：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`mise x -- go build ./cmd/agent`、`cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run build` 全部通过，且无真实公网 IP/域名/Token 泄露。

## Out of Scope
- 黑盒默认对外直连协议配置。
- 从远端入站一键预填 Master 中转出站快捷按钮（决策 D3 选 A，延后独立小任务按需增量）。
- 修改 Xray-core 协议实现或 SOCKS5 中转自动配对机制。
- 版本发布、Tag 推送与线上服务器部署（留待验收后由用户确认执行）。
