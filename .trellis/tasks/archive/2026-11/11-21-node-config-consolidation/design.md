# Technical Design: 节点命名与配置收束

## 1. Architecture & Layer Boundaries
保持 Clean Architecture 分层不变，按后端（`internal/**`）与前端（`web/**`）两个完全文件互斥的契约边界实施：

```
SQLite (nodes.direct_sub_enabled, inbounds.sub_exposed)
  └─> domain.Node / domain.Inbound (*bool + IsDirectSubEnabled() / IsSubExposed())
        ├─> NodeService / InboundService (显式持久化 true/false，缺省补 true)
        ├─> SubService (resolveActiveNodeMap + prepareInboundForNode 统一过滤，不再拼 [Region])
        │     ├─> ExportUserSubscription -> InboundsToNodeConfigsWithDefaultFP -> ApplyNodeNameTemplate -> DeduplicateNodeNames -> Base64/Clash/SingBox
        │     └─> GetUserShareInfo       -> 复用同一节点转换 + ApplyNodeNameTemplate 管线
        └─> XrayCompiler / extractStaticRelayClients (不受订阅暴露开关影响，中转持续工作)
```

## 2. Data Model & API Contracts

### 2.1 Backend Schema (`*bool` 防零值与历史兼容契约)
- **`internal/domain/node.go`**:
  - `Node.DirectSubEnabled *bool` `gorm:"column:direct_sub_enabled;default:true" json:"directSubEnabled"`
  - `func (n Node) IsDirectSubEnabled() bool { return n.DirectSubEnabled == nil || *n.DirectSubEnabled }`
  - `CreateNodeDTO.DirectSubEnabled *bool` (`json:"directSubEnabled"`)
  - `UpdateNodeDTO.DirectSubEnabled *bool` (`json:"directSubEnabled"`)
- **`internal/domain/inbound.go`**:
  - `Inbound.SubExposed *bool` `gorm:"column:sub_exposed;default:true" json:"subExposed"`
  - `func (i Inbound) IsSubExposed() bool { return i.SubExposed == nil || *i.SubExposed }`
- **为什么用 `*bool` 而不是 `bool`**：
  1. GORM 对带 `default:true` 的普通 `bool` 在 `Create` 时会忽略 Go 零值 `false` 导致写库错变 `true`；使用 `*bool` 时 `&false` 会被 GORM 显式写入 `0`；
  2. 现有所有单测中直接字面量构造的 `domain.Node{...}` / `domain.Inbound{...}` 未填该字段时为 `nil`，通过 `IsDirectSubEnabled()` / `IsSubExposed()` 默认返回 `true`，100% 兼容既有单测与历史数据；
  3. Service 层在 `CreateNode` / `CreateInbound` / 响应补齐时确保字段非 `nil`（`nil` 补 `&true`），使 JSON API 稳定输出 `true` 或 `false`。

### 2.2 统一订阅命名引擎 (`internal/protocol/node_converter.go` & `internal/service/sub_service.go`)
- **`prepareInboundForNode` 职责收口**：
  1. 先拦截 `if !ib.IsSubExposed() { return domain.Inbound{}, domain.Node{}, false }`（Master 与远端入站一视同仁）；
  2. 远端节点（`NodeID > 0`）查询 `activeNodes[ib.NodeID]`（`resolveActiveNodeMap` 要求 `n.Enabled && n.DesiredState == domain.NodeStateActive && n.IsDirectSubEnabled()`）；
  3. 仅补齐连接地址回退：`ExternalHost` 空则用 `node.Host`，`ExternalPort <= 0` 则用 `ib.Port`；
  4. **不再改写 `ibCopy.Remark` 或 `subRoutes[i].Name`**。
- **结构化 `NodeConfig.Params` 元数据**：
  - 在 `InboundsToNodeConfigsWithDefaultFP`（或辅助入参携带 `nodeMap map[uint]domain.Node`）为每个转换出的 `NodeConfig` 写入：
    - `remark`：有效显示名称（有启用 `SubRoute` 时取 `sr.Name`，为空则 `baseName - 线路 #routeId`；无 `SubRoute` 时取 `in.Remark`，为空则 `in.Tag`）
    - `inbound_remark`：`strings.TrimSpace(in.Remark)`
    - `route_name`：`strings.TrimSpace(sr.Name)`（无分流线路时为空）
    - `outbound_tag`：`strings.TrimSpace(sr.OutboundTag)`（无分流线路时为空）
    - `node_name`：远端为 `strings.TrimSpace(node.Name)`，Master 为 `""`
    - `node_region`：远端为 `strings.TrimSpace(node.Region)`，Master 为 `""`
- **模板渲染 (`RenderNodeName` / `ApplyNodeNameTemplate`)**：
  - 默认模板为空或 `"{remark}"` 时，直接保留所见即所得的 `node.Name`（即有效显示名称），不做额外拼接。
  - 支持全部占位符：`{remark}`、`{tag}`、`{protocol}`、`{port}`、`{index}`、`{region}`、`{node}`、`{route}`、`{outbound}`。
  - 空值归一化（仅当使用了自定义模板时清理）：清除空方括号 `[]`、`()`、`【】`，折叠连续空白，剥离首尾多余的 `-` / `|` / `·` / `/` 分隔符；若渲染结果为空，回退原始 `node.Name`。
- **`GetUserShareInfo` 对齐**：
  - 读取 `sub_node_name_template`（与 `ExportUserSubscription` 相同 settings 来源），调用同一 `prepareInboundForNode` + `InboundsToNodeConfigsWithDefaultFP` + `ApplyNodeNameTemplate` + `DeduplicateNodeNames`，彻底消除“订阅链接名”和“单节点分享弹窗名”不一致。
- **重名消歧 (`DeduplicateNodeNames`)**：
  - 仅在节点列表存在相同 `Name` 时，对第 2 个及以后的同名节点追加 ` #2`、` #3`（避免 Clash / Sing-box 因重复 `name` / `tag` 报错）；无重名时完全不改动原名。

### 2.3 Frontend Cross-Page Contract (`web/src/**`)
- **Node DTO (`web/src/views/nodes/types.ts` / `api/nodes.ts`)**：`NodeItem`、`CreateNodePayload`、`UpdateNodePayload` 新增 `directSubEnabled: boolean`（缺省按 `true` 处理）。
- **Inbound DTO (`web/src/views/inbounds/types.ts` / `api/inbounds.ts`)**：`Inbound` 新增 `subExposed?: boolean`（缺省按 `true` 处理）。
- **表单联动与防误触规则**：
  - `web/src/views/inbounds/types.ts`：`getDefaultInboundFormData(usersList, defaultSubExposed = true)`：
    - 移除默认 `🇯🇵 日本原生直连` SubRoute，改为 `subRoutes: []`；
    - `subExposed: defaultSubExposed`；
    - `selectedUserEmails: defaultSubExposed ? usersList.map((u) => u.email) : []`。
  - `InboundFormDrawer.vue`：
    - 新建入站时读取当前所属节点的 `directSubEnabled`（Master 默认 `true`，远端节点取 `node.directSubEnabled ?? true`）作为 `subExposed` 初始值；
    - 若所属远端节点 `directSubEnabled === false`，在 `subExposed` 旁显示提示徽章「所属节点已设为仅中转落地（整机不对外提供直连订阅）」；
    - 当 `form.subExposed === false` 时：折叠/隐藏 `ExternalHost`、`ExternalPort`、`SortOrder`、`⑤ 分流订阅线路`、`⑥ 关联授权用户`；保存时**不执行**批量把该入站 Tag 写入用户 `inboundTags` 的逻辑（新建不写入；编辑将 `subExposed` 从 `true` 切为 `false` 时也保持用户表安全或按需跳过追加）。
  - `UserFormDrawer.vue`：
    - 传入节点列表（或通过 `useActiveNode().nodes` 获取远端节点 `directSubEnabled` 状态），计算 `subscribableInbounds`：仅展示 `inbound.subExposed !== false` 且所属远端节点 `directSubEnabled !== false` 的多用户入站（若用户历史已绑定某个已转为内部的 tag，保存时保留或静默清理均可，建议不显示在可选列表且新建默认全选只勾选 `subscribableInbounds`）。
  - `InboundTable.vue`：
    - 在列表中直观展示「订阅显示名称」（`Remark || Tag`）及「公开订阅」/「仅内部中转」徽章（若所属节点 `directSubEnabled === false`，同样标出「节点仅中转」）。
  - `SettingsView.vue`：
    - 更新 `{remark}` 说明为「订阅显示名称（入站或分流线路名）」，补全 `{region}`、`{node}`、`{route}`、`{outbound}` 可点击插入芯片 + 2~3 个常用预设 + 纯前端实时预览示例（展示 Master 与远端节点两种样例）。

## 3. Compatibility & Rollback
- **向后兼容**：新增两列均为 `default:true`，现有节点与入站在数据库自动迁移后保持原订阅行为，管理员仅需到「托管节点」把 GCP 节点的「对外直连订阅」关闭即可立即生效。
- **注意已有单测更新**：既有 `sub_service_test.go` 中若有断言 `[JP] ...` 硬编码前缀的测试，需同步更新为所见即所得行为并新增自定义模板 `[{region}] {remark}` 测试覆盖！
- **回滚**：无破坏性删列或重写字段，代码回退可安全兼容已存在的新列。
