# 节点命名、订阅暴露与配置分散现状审计报告

> 审计范围：`xray-panel` 后端订阅/编译/节点服务（`internal/domain`、`internal/service`、`internal/protocol`、`internal/sub`、`internal/adapter/xray`、`internal/agent`、`internal/delivery/http`）及前端 Web 面板（`web/src/views/*`、`web/src/composables/*`、`web/src/App.vue`）。
> 本文严格区分 **已验证源码事实** 与 **架构/产品建议**。

---

## 1. 订阅节点命名全链路审计（为什么会出现 `[**]直连` 等混乱名字？）

### 1.1 命名生成全链路追踪

当客户端请求 `/sub/:token`（或 `/users/:id/share`、`/portal/claim`）时，节点最终名称经历以下阶段：

1. **数据源头与前端默认种子（Seeds）**
   - **远端节点元数据**：`Node.Name`（必填，如 `jp-tokyo-01`）与 `Node.Region`（选填，如 `JP` / `HK`）（`internal/domain/node.go:109-111`、`web/src/views/nodes/components/NodeFormDrawer.vue:17-63`）。
   - **入站元数据**：`Inbound.Remark`（选填；为空时退回 `Inbound.Tag`）（`internal/domain/inbound.go:12,19`、`internal/service/sub_service.go:97-100`、`internal/protocol/node_converter.go:194-197`）。
   - **分流子线路默认种子（关键诱因）**：前端新建入站表单 `getDefaultInboundFormData()` 会为**所有新建 VLESS 入站（包含远端节点入站）** 默认预填一条 `SubRoute`：`{ name: '🇯🇵 日本原生直连', routeId: 1, outboundTag: 'direct', enabled: true }`（`web/src/views/inbounds/types.ts:163-173`）。如果管理员将它简写为 `"直连"` 或直接使用 `"🇯🇵 日本原生直连"`，该入站就不会使用 `Inbound.Remark`，而是按 `SubRoute.Name` 导出（`internal/protocol/node_converter.go:431,461-472`）。

2. **远端节点前缀强制注入：`SubService.prepareInboundForNode`**（`internal/service/sub_service.go:56-123`）
   - **主控 Master（`NodeID == 0`）**：`ib.IsLocal()` 直接返回原 `ib`，**不添加任何节点前缀**（`internal/service/sub_service.go:82-84`）。
   - **远端 Node（`NodeID > 0`）**：
     - `formatNodePrefix(node)` 取 `node.Region`（若为空则退回 `node.Name`），拼出 `[<label>]`（`internal/service/sub_service.go:56-65`）。
     - `applyNodePrefix(name, prefix)`：若 `name` 为空则返回 `prefix`；若 `name` 不包含 `prefix` 子串，则返回 `prefix + " " + trimmed`（`internal/service/sub_service.go:67-79`）。
     - 将上述前缀**直接改写到副本的 `ibCopy.Remark`**（`internal/service/sub_service.go:96-101`），并且遍历 `SubRoutes` 时，对每个非空 `subRoutes[i].Name` **同样强制加上 `[<label>] ` 前缀**并重新序列化 `SubRoutesJson`（`internal/service/sub_service.go:103-121`）。
     - **因此**：只要远端节点填了 `Region`（或未填 `Region` 但有 `Name`），而 `Inbound.Remark` 或 `SubRoute.Name` 填了 `"直连"`（或 `"日本直连"`），导出名必定变成 `[<Region或Name>] 直连`（经 RFC 3986 URL 编码为 `#%5B...%5D%20%E7%9B%B4%E8%BF%9E`，参见 `internal/protocol/helpers.go:10-12`）。

3. **入站 → `protocol.NodeConfig` 转换：`InboundsToNodeConfigsWithDefaultFP`**（`internal/protocol/node_converter.go:403-477`）
   - 若无 `SubRoutes`（或非 VLESS 协议）：调用 `InboundToNodeConfigWithDefaultFP`，取 `remark := inbound.Remark`（空则 `inbound.Tag`）作为 `node.Name`，同时写入 `node.Params["remark"] = inbound.Remark`、`node.Params["tag"] = inbound.Tag`（`internal/protocol/node_converter.go:194-201`）。
   - 若有启用的 `SubRoutes` 且为 VLESS：
     - 若 `sr.Name != ""`：`tempInbound.Remark = sr.Name` 且 `node.SetParam("remark", sr.Name)`（`internal/protocol/node_converter.go:462-471`）；注意此时经过 `prepareInboundForNode` 的 `sr.Name` 已经被污染为 `[Region] 原线路名`。
     - 若 `sr.Name == ""`：`tempInbound.Remark = fmt.Sprintf("%s - 线路 #%d", baseName, sr.RouteID)`（`baseName` 为已加前缀的 `in.Remark`），但 `node.SetParam("remark", ...)` 未再次覆盖，所以 `Params["remark"]` 继承 `tempInbound.Remark`（`internal/protocol/node_converter.go:464-471`）。

4. **全局命名模板应用：`ApplyNodeNameTemplate` / `RenderNodeName`**（`internal/service/sub_service.go:263-266`、`internal/protocol/node_converter.go:479-517`）
   - 仅当 `settings["sub_node_name_template"]` 非空且不等于 `"{remark}"` 时执行（`internal/protocol/node_converter.go:507-510`）。
   - 仅支持 5 个占位符：`{remark}`、`{tag}`、`{protocol}`（大写）、`{port}`、`{index}`（`internal/protocol/node_converter.go:480,498-503`）。
   - **注意代码库中不存在 `ClientNameTemplate` 字段**（经全局检索 `internal/` 与 `web/src/`，无任何 `ClientNameTemplate` 定义或引用）。

5. **多格式最终输出与分享接口分叉**
   - **Base64 / Raw**：`vless.go:144-145`（以及其他协议格式化器）将 `node.Name` 经 `EscapeRemark`（`helpers.go:10-12`）作为 URI `#fragment`。
   - **Clash / Mihomo**：`clash.go:63-80` 取 `node.Name`（空则 `Node-%d`）写入 `proxyMap["name"]`。**无重名去重**：同名节点可能导致 Clash/Mihomo 行为异常。
   - **Sing-box**：`singbox.go:24-35` 取 `node.Name`（空则 `Node-%d`）写入 `outbound["tag"]`。**无重名去重**：Sing-box 对重复 `outbound.tag` 会直接报错或覆盖。
   - **`GetUserShareInfo`（用户弹窗单节点链接 & Portal 应急节点）分叉**：`internal/service/sub_service.go:390-497` **完全绕过了 `ApplyNodeNameTemplate`**！且 `sub_service.go:478-480` 在 `prepareInboundForNode` 已加过一次前缀后又调用一次 `applyNodePrefix`（依赖 `strings.Contains` 幂等兜底）。

### 1.2 影响节点最终显示名称的所有字段、优先级与冲突点

| 影响源 | 所在位置 / 默认值 | 当前行为与优先级 | 冲突与缺陷 |
|---|---|---|---|
| `Node.Region` | `internal/domain/node.go:111`（选填，默认 `""`） | 远端节点前缀第一优先级：`[<Region>]` | 1. 仅作用于远端节点，Master（`ID=0`）无对应字段，导致主从命名割裂；<br>2. 硬编码为 `[xxx] ` 前缀，无法通过模板调整位置或关闭。 |
| `Node.Name` | `internal/domain/node.go:109`（必填） | 当 `Node.Region == ""` 时退化为 `[<Name>]` 前缀 | 后台管理标识（如 `gcp-tokyo-01`）直接暴露到用户订阅节点名中。 |
| `Inbound.Remark` | `internal/domain/inbound.go:19`（选填） | 无 `SubRoute` 时作为基础名；空则退回 `Inbound.Tag` | 远端节点会被强制改写为 `[Region] Remark`；前端新建入站未要求填 Remark 且默认给了 SubRoute，导致 `Inbound.Remark` 常被忽略。 |
| `Inbound.Tag` | `internal/domain/inbound.go:12`（必填） | `Remark` 为空时兜底；或模板 `{tag}` 引用 | 技术标识直接暴露给终端用户。 |
| `SubRoute.Name` | `internal/domain/inbound.go:45`（JSON 内） | **只要 VLESS 启用了任意 SubRoute，完全覆盖 `Inbound.Remark`** | 1. `web/src/views/inbounds/types.ts:163-173` 对所有新建 VLESS 入站默认生成一条 `🇯🇵 日本原生直连` 线路，极易误导；<br>2. 在 `prepareInboundForNode` 中被强行注入 `[Region]`，造成 `[JP] 🇯🇵 日本原生直连` 或 `[**] 直连` 重复/生硬拼接。 |
| `sub_node_name_template` | `SettingsView.vue:351,757`（默认 `"{remark}"`） | 在 `SubService.resolveSubscriptionNodesWithSettings:264` 末段替换 | 1. **缺少 `{node}`、`{region}`、`{route}`、`{outbound}` 占位符**，用户无法自由编排地区与线路名；<br>2. `{remark}` 在远端节点上拿到的是已被预拼接 `[Region]` 的脏字符串；<br>3. **`GetUserShareInfo`（单节点链接与 Portal 提取）未应用该模板**；<br>4. 无法按节点或入站关闭全局模板。 |

---

## 2. 远端入站订阅暴露与中转落地机制审计

### 2.1 订阅导出过滤全链路（DB → `SubService` → 多格式导出）

1. **DB 模型当前拥有的控制字段**：
   - `Node`（`internal/domain/node.go:114-116`）：`Enabled`（`bool`，默认 `true`）、`DesiredState`（`active` / `draining` / `circuit_broken`，默认 `active`）。**无任何订阅暴露开关或节点角色字段。**
   - `Inbound`（`internal/domain/inbound.go:11,28`）：`NodeID`（`0`=Master，`>0`=远端）、`Enabled`（`bool`，默认 `true`）、`Protocol`、`SubRoutesJson`。**无任何 `ExposeInSub` / `IsInternal` / `RelayOnly` 控制字段。**
   - `User`（`internal/domain/user.go:13-14,25,78-89`）：`Enabled`、`TotalBytes`/`ExpireTime`、`InboundTags`（逗号分隔授权 Tag 列表）。
   - `SubRoute`（`internal/domain/inbound.go:48-49`）：`Enabled`、`AllowedUsers`（空数组 = 全员开放）。

2. **后端 `SubService` 过滤逻辑**（两条路径：`resolveSubscriptionNodesWithSettings` 在 `sub_service.go:213-286`；`GetUserShareInfo` 在 `sub_service.go:390-497`）：
   - 校验用户：`user.Enabled` 与 `user.IsActive()`（未超流量、未过期）（`sub_service.go:223-228`）。
   - 构建活跃远端节点表 `resolveActiveNodeMap`（`sub_service.go:39-54`）：仅纳入 **`n.Enabled == true && n.DesiredState == "active"`** 的远端节点。
   - 遍历 `inboundRepo.ListAll` 并通过 `prepareInboundForNode`（`sub_service.go:81-123`）：
     - `NodeID == 0`（Master）：直接放行进入下一级过滤；
     - `NodeID > 0`（远端）：仅当 `ib.NodeID` 在 `activeNodes` 中时放行。
   - 节点协议与用户权限过滤（`node_converter.go:420-425` 与 `sub_service.go:433`）：
     - 跳过 `!in.Enabled`；
     - 跳过 `!user.HasInbound(in.Tag)`（即**后端要求 `user.InboundTags` 显式包含该 `in.Tag`**）；
     - 跳过 `strings.EqualFold(in.Protocol, "dokodemo-door")`（**注意：`socks` / `http` 并未跳过！** 若将 `socks`/`http` 授权给用户，会导出 `socks5://` 节点，见 `internal/protocol/socks.go:32-55`；仅因前端 `UserFormDrawer` 过滤了 `MULTI_USER_PROTOCOLS` 才未展示，但 `InboundFormDrawer` 仍会回退兼容）。
   - 分流线路过滤：若为 `vless` 且存在 `SubRoutes`，仅导出 `sr.Enabled && sr.CanAccess(user.Email)` 的线路（`node_converter.go:451-460`）。

### 2.2 逐一回答用户核心疑问

#### 问题 (a)：如果在远端节点（如 GCP）创建了一个入站，当前代码会不会默认给所有用户下发这个节点？
- **结论：后端按 `user.HasInbound(in.Tag)` 校验；但前端默认行为会“实质上自动授权并下发给所有现有用户及未来新建用户”！**
- **已验证源码证据**：
  1. **新建入站时默认全选所有已有用户**：`web/src/views/inbounds/components/InboundFormDrawer.vue:1598` 调用 `getDefaultInboundFormData(props.usersList)`；而在 `web/src/views/inbounds/types.ts:179` 中：
     ```ts
     selectedUserEmails: usersList.map((u) => u.email),
     ```
     保存入站时（`InboundFormDrawer.vue:1646-1670`），前端遍历所有用户，只要 `selectedUserEmails.includes(u.email)` 就调用 `PUT /api/users/:id` 把新建 GCP 入站的 `tag` 写入每一个用户的 `inboundTags`！只要管理员没有特意拉到底部 `⑥ 关联授权用户` 点 `取消全选`，**全部已有用户立刻收到 GCP 节点**。
  2. **未来新建用户时默认全选所有多用户入站**：`web/src/views/users/components/UserFormDrawer.vue:560` 在新建用户重置表单时：
     ```ts
     selectedTags: props.availableInbounds.filter(isMultiUserInbound).map((i) => i.tag),
     ```
     其中 `availableInbounds` 是全局 `/inbounds` 列表，**包含所有远端节点（GCP）的 VLESS/VMess/Trojan/SS 入站**！以后每次新增用户，若忘记手动取消勾选 GCP 入站，新用户同样直接拿到 GCP 直连节点。

#### 问题 (b)：目前有没有办法让一台 GCP Agent 节点“只作中转落地（给 `jp` Master 转发流量）、绝不出现在用户订阅里”？如果有，现在怎么配、为什么难找？如果没有，缺什么最小正交字段/开关？
- **结论：没有一等公民开关；只能靠极易误触的“零用户授权”暗技巧勉强规避，且其他看似直观的开关全部有副作用。**
- **现状所有途径逐一核查**：
  1. **唯一能“不出现在订阅里且保持中转可用”的隐晦做法（脆弱 Hack）**：
     - 在 GCP 节点创建 `vless`/`vmess`/`trojan` 入站时，手动滚动到表单底部 `⑥ 关联授权用户` 点击 **`取消全选`**（使没有任何用户拥有该 `tag`）；且此后每次在「用户管理」新建/编辑用户时，**必须人工记住不能勾选该 GCP 入站**（不能点“全选所有节点”）。
     - 原理：后端中转凭据注入 `extractStaticRelayClients`（`internal/service/node_service.go:1510-1616`）**独立于用户列表**，只要 Master 出站指向 GCP，就会自动注入静态客户端到 GCP 的 `config.json`；而 `SubService` 会因为 `!user.HasInbound(in.Tag)` 不导出该入站。
     - **为什么难找且高危**：UI 上没有任何“仅内部中转/不暴露订阅”说明；新建入站默认全选用户、新建用户默认全选入站，**一次漏取消勾选就会立刻把 GCP 直连节点发给用户，产生高额出站流量费**。
  2. **为什么不能用现有的 `Node.Enabled = false` / `Inbound.Enabled = false` / `DesiredState = draining`？**
     - `Node.Enabled = false`：`/api/agent/sync` 直接返回 `403 Forbidden`（`internal/service/node_service.go:800-802`），Agent 彻底停服断连。
     - `Inbound.Enabled = false`：`CompileRemoteNodeFull` 跳过 `!inb.Enabled`（`internal/adapter/xray/compiler.go:397`），GCP 入站停止监听，中转直接中断。
     - `Node.DesiredState = "draining"`：虽然 `SubService.resolveActiveNodeMap`（`sub_service.go:49`）会将其从订阅排除，Agent 也能继续同步（`internal/agent/runner.go:302` 仅 `circuit_broken` 触发静默熔断），但这是**严重的语义滥用与副作用**：
       - 在 UI（`NodesView.vue`、`NodeOpsDrawer.vue`、`App.vue`）中会被标记为异常/告警态“摘流排空 (draining)”；
       - 设置 `draining` 会把 `AutoStateReason` 置为 `"manual"`（`node_service.go:466,523`），导致 `node_service.go:857` **跳过该节点的自动流量阈值熔断保护（95% 停机止损失效）**；
       - 无法做到“同节点内部分入站对外、部分入站仅中转”，也无法阻止 `InboundFormDrawer`/`UserFormDrawer` 继续把它当成普通可授权入站。
- **缺失的最小正交能力**：
  - `Inbound` 缺少显式的订阅可见性开关（例如 `ExposeInSub bool` / `SubExposed bool`，或归属/用途标识）；
  - `Node` 缺少节点角色/默认暴露策略（例如 `ExposeInSub bool` 或 `Role: direct | relay_exit`），使得在「仅中转落地」节点上新建的入站天然默认不暴露订阅、不自动加入用户授权勾选。

#### 问题 (c)：Master (`jp`) 与远端 Agent (`GCP`) 之间目前的中转互联机制是什么？内部中转凭据会不会被误导出到用户订阅？
- **已验证互联机制（自动化静态凭据配对，仅限 VLESS / VMess / Trojan）**：
  1. 在 **GCP（远端节点）** 上创建一个启用中的 `vless` / `vmess` / `trojan` 入站（`nodeId = gcpNode.ID`）。
  2. 在 **Master (`jp`, `nodeId = 0`)** 上手动创建一个出站（`OutboundsView` → `OutboundFormDrawer`），协议为 `vless` / `vmess` / `trojan`，目标地址填 `gcpNode.Host`，端口填 GCP 入站的 `Port` 或 `ExternalPort`，并填入一个自定义 UUID / Password（`internal/service/node_service.go:1508-1602`）。
  3. **自动静态凭据注入（`extractStaticRelayClients`）**：当 Master 生成或下发 GCP 的配置快照时（`internal/service/node_service.go:978-987,1510-1616`），会扫描 Master 本机出站（`localOutboundsProvider`）：
     - 若发现某条 Master `vless`/`vmess`/`trojan` 出站的 `address == node.Host`（忽略大小写及 IPv6 方括号）且 `port` 匹配 GCP 某启用入站的 `Port` 或 `ExternalPort` 且协议一致；
     - 自动提取该出站的 `UUID`/`Password` 与精确 `flow`（不强行合成 Vision），构造 `StaticInboundClient`：
       ```go
       Email: "relay-auto:" + ob.Tag + "@internal"
       ```
     - 通过 `CompileRemoteNodeFull` → `mergeStaticClients`（`internal/adapter/xray/compiler.go:400,474-544`）静态编译进 GCP 的 `config.json` `inbounds[].settings.clients` 中。
     - **注意**：SOCKS5 / HTTP / Shadowsocks 出站**不在** `extractStaticRelayClients` 自动配对范围内（`node_service.go:1522` 显式只处理 `vless`/`vmess`/`trojan`）。如果使用 SOCKS5 中转，GCP 侧必须手动建 `socks` 入站并在入站里配静态账号密码，Master 侧建 `socks` 出站填入相同账号密码。
  4. 在 **Master (`jp`)** 的入站或路由规则中，将用户流量（或某条 `SubRoute`）指向该中转 `OutboundTag`。
- **`relay-auto:*@internal` 会不会误导出到用户订阅？**
  - **不会。** 理由如下：
    1. `relay-auto:<obTag>@internal` 仅存在于编译给 GCP Xray 的 `configJson` 中的 `StaticInboundClient`（`node_service.go:978-995`），甚至连 Agent 的动态用户增量接口 `BuildRemoteNodeUsers` 都不包含它（`compiler.go:436-468`、`agent/config_manager.go:242-245` 已将其识别为受保护静态客户端以免被 `UserSyncer` 误删）。
    2. `SubService` 仅从 `userRepo` 读取真实用户表（`sub_service.go:218`）和 `inboundRepo` 读取入站表（`sub_service.go:233`），不会读取 `StaticInboundClient`。
  - **但是，承载该中转的“GCP 入站本身”极易被误导出**：正如 2.2(a) 所述，因为那个 GCP 入站本身是公开挂载在 `inbounds` 表中的普通 VLESS/VMess/Trojan 入站，前端默认全选所有用户，管理员一不小心就会把 GCP 入站直连节点发给所有用户。

---

## 3. 节点相关配置分散地图（前后端全盘点）

### 3.1 前端全局上下文与各页面/抽屉职责矩阵

前端通过全局单例 `useActiveNode()`（`web/src/composables/useActiveNode.ts:37-83`）在顶栏提供一个“当前管理节点切换器”（`web/src/App.vue:243-260,379-395`），但其覆盖范围是**割裂的**（由路由 `meta.nodeScoped` 决定，`web/src/router/index.ts:19-31`）：

| 页面 / 抽屉 | 路由 / 位置 | `nodeScoped` | 实际管理/影响的节点配置内容 | 命名 / 排序 / 白名单 / 暴露相关字段 |
|---|---|---|---|---|
| **顶栏节点切换器** | `App.vue:243-260` | 全局控件 | 切换 `activeNodeId`（`0`=本机 Master，`>0`=远端节点） | 仅在 `nodeScoped: true` 页面生效；切换去 `/nodes`、`/users`、`/settings` 时变成只读全局提示徽章 |
| **NodesView** | `/nodes` (`NodesView.vue`) | `false` | 远端节点注册列表、在线心跳、同步 Hash、总流量 | 展示 `Name`、`Region`、`DesiredState`、`Enabled` |
| **NodeFormDrawer** | `NodesView` 内 (`NodeFormDrawer.vue`) | `false` | 节点标识 (`Name`)、接入点 (`Host`)、`Region`、`DesiredState`、`Enabled`、配额与熔断阈值 | **`Name` + `Region`** 决定远端前缀 `[Region]`；**`Enabled` + `DesiredState`** 间接触发订阅摘除；文案自称“远端直连节点” |
| **NodeOpsDrawer** | `NodesView` 内 (`NodeOpsDrawer.vue`) | `false` | 状态与配额、快速日志、编译预览、GeoData 与安装脚本/Token 轮换 | 提供一键“摘流排空 (`draining`) / 恢复活跃 (`active`)”按钮（`:220-239`） |
| **InboundsView & InboundTable** | `/inbounds` (`InboundsView.vue`, `InboundTable.vue`) | `true` | 入站网关列表（可勾选“查看全部节点”跨节点总览 `useInboundList.ts:244,257`） | 展示 `Tag`、节点徽章（`getNodeBadgeLabel`）、端口、用户数、线路数；**列表中完全不展示 `Remark`、不展示 `SortOrder`**！ |
| **InboundFormDrawer** | `InboundsView` 内 (`InboundFormDrawer.vue`, 1690 行巨型抽屉) | `true` | 单条入站的全部参数 + **节点归属 (`nodeId`)** + **分流线路 (`SubRoutes`)** + **用户授权 (`selectedUserEmails`)** | 1. 身份与排序：`Tag`、`Remark`、`SortOrder`、`ExternalPort`、`ExternalHost`、`nodeId`；<br>2. 分流线路：`SubRoute.routeId`、`sortOrder`、`name`、`outboundTag`、`allowedUsers`（白名单 Popover）、`enabled`；<br>3. 授权：**默认全选所有用户**写入各用户 `inboundTags` |
| **OutboundsView & OutboundFormDrawer** | `/outbounds` (`OutboundsView.vue`, `OutboundFormDrawer.vue`) | `true` | 当前 `activeNodeId` 下的出站列表（Master 读 `config.json`，远端读 `Node.OutboundsJSON`） | 中转链路需在此手动填远端 `Host` + `Port` + `UUID/Password` + 传输/TLS/REALITY 参数以触发 `relay-auto` 配对 |
| **RoutingView** | `/routing` (`RoutingView.vue`) | `true` | 当前 `activeNodeId` 下的路由规则与 `domainStrategy` | 规则支持 `inboundTag`、`vlessRoute`、`user`（白名单）、`outboundTag`；保存时后端自动调用 `ensureSubRouteRules` 同步 `SubRoute` |
| **TopologyView** | `/topology` (`TopologyView.vue`) | `true` | 当前 `activeNodeId` 的「入站 → 通道/分流 → 出站」拓扑可视化与快捷开关 | 1. **新建网关/出站按钮实际跳转去 `/inbounds?action=create` / `/outbounds?action=create`**（`:195-205`）；<br>2. 仅 Master 视图下 `OutboundNodeCard.vue:90-101` 会高亮匹配“→ 托管节点 xxx” |
| **UsersView / UserFormDrawer / UserShareModal** | `/users` (`UserFormDrawer.vue`, `UserShareModal.vue`) | `false` | 用户凭据、`InboundTags` 授权、聚合/单节点分享链接、安全提取码 | 1. 新建用户默认全选所有入站（`:560`）；<br>2. 单节点分享展示 `Remark`（不受模板控制）且文案统称“直连链接” |
| **SettingsView** | `/settings` (`SettingsView.vue`) | `false` | 全局域名/端口 (`sub_domain`, `public_port`)、虚拟提示节点、**`sub_node_name_template`**、uTLS 指纹、Clash 参数 | 唯一能配置命名模板的地方，但模板缺少 `{node}`/`{region}`/`{route}` 变量 |

### 3.2 痛点动线对比：首次配置「GCP 仅中转落地」vs「直连节点」

#### 场景 A：首次配置 GCP 节点作为 `jp` Master 的中转落地（期望：绝不出现在用户订阅中）
管理员当前必须在 **4~5 个分散页面**间来回跳转，且存在 **3 处高危易错坑**：

1. **去 `/nodes` → 打开 `NodeFormDrawer`**：
   - 填写 `Name`（如 `gcp-sg-exit`）、`Host`（GCP 公网 IP）、`Region`（可选填；注意：虽然只想做中转，但如果没避开订阅，这里填的 `Region`/`Name` 会变成 `[Region]` 前缀）。
   - 关闭后弹出 `NodeTokenModal`，复制 Token 与安装命令去 GCP 服务器部署 `xray-agent`。
2. **切顶栏节点切换器 → 选中 `gcp-sg-exit` → 去 `/inbounds` → 点击「添加新入站」**：
   - **重复/混淆点 1**：`nodeId` 默认已跟随顶栏选为 `gcp-sg-exit`，但表单里依然显示 `ExternalHost` / `ExternalPort` / `Remark` / `SortOrder` 等订阅展示字段。
   - **高危坑 1（默认 SubRoute 噪音）**：`InboundFormDrawer` 默认自带一条 VLESS SubRoute `🇯🇵 日本原生直连`（`types.ts:163-173`），若只是普通中转落地入站，需手动点垃圾桶删掉该 SubRoute，否则无端多出一套 `vlessRoute` 分流规则。
   - **高危坑 2（默认全选所有用户！）**：必须滚动到抽屉最底部的 `⑥ 关联授权用户`，**手动点击 `取消全选`**！否则一保存就把 GCP 入站绑定到全部用户，**所有用户订阅立刻多出 GCP 直连节点**！
   - 记住自己刚才在 GCP 入站配的 **端口、协议、传输方式、REALITY/TLS 公私钥/SNI/ShortID**（需手动复制私钥对应的公钥、ShortID、SNI、Flow）。
3. **切回顶栏节点切换器 → 选回 `🖥 本机 Master` → 去 `/outbounds` → 点击「添加出站节点」**：
   - 手动创建一个同名协议出站（如 `to-gcp`），**人工重新填写一遍** GCP 的 `Host`、端口、对端公钥（`pbk`）、`SNI`、`ShortID`、`Flow`，并自造一个 UUID（后端才能靠 `Host + Port + Protocol` 隐式匹配并注入 `relay-auto:to-gcp@internal`）。
   - **隐形契约**：面板没有任何提示告诉你“地址和端口必须与 GCP 节点 Host 和入站 Port/ExternalPort 完全一致才能自动注入凭据”；只有去 `/topology` 看 `OutboundNodeCard` 才会看到一行小字 `→ 托管节点 gcp-sg-exit`。
4. **还在 `Master` 下 → 去 `/inbounds`（编辑 Master 的对外 VLESS 入站）→ `⑤ 分流订阅线路 (Sub-Routes)`**：
   - 添加/编辑一条面向用户的 `SubRoute`（如名称 `🇸🇬 新加坡中转`，目标出站选 `to-gcp`，配置 `allowedUsers` 白名单）。
5. **去 `/users`（之后每次新建用户时）**：
   - **高危坑 3**：新建用户时 `UserFormDrawer` 又会把刚刚那个 `gcp-sg-exit` 的入站默认勾上，每次都要人工取消勾选。

#### 场景 B：首次配置远端直连节点（如 `jp-direct` 对外直连）
1. **`/nodes`**：创建节点，填写 `Name`、`Host`、`Region`（例如 `JP`）→ 部署 Agent。
2. **顶栏切至该节点 → `/inbounds`**：创建入站，填写 `Tag`、`Remark`（例如 `直连`）、`SortOrder`；但默认自带了一条 `SubRoute`（`🇯🇵 日本原生直连`），导致不删它的情况下 `Remark` 被架空，订阅导出为 `[JP] 🇯🇵 日本原生直连`（若改成 `直连` 则导出 `[JP] 直连`）。
3. **想调整全局命名格式 → `/settings`**：修改 `sub_node_name_template`，却发现只有 `{remark}` / `{tag}` / `{protocol}` / `{port}` / `{index}`，无法控制 `[JP]` 的有无或顺序，且无法预览最终导出名字，单节点分享弹窗也不生效。

---

## 4. KISS 收束方向与「一键配置」利弊评估

### 4.1 针对需求 1 & 2（命名收束 + 节点配置/直连暴露收束）的最小侵入方案

严守 KISS 与正交设计原则（不在 `domain` 引入复杂状态机或重型重构），建议分两个正交维度收束：

#### 候选方案 A：订阅暴露与节点角色控制（解决“只作中转落地、绝不出现在用户订阅里”）
1. **后端最小 Schema 扩展（GORM `AutoMigrate` 自动加列，向后兼容已有数据）**：
   - **方案 A1（推荐：Inbound 粒度 + Node 默认值）**：
     - 在 `domain.Node` 增加 `ExposeInSub *bool`（或 `DirectSubEnabled bool`，GORM `default:true`，或 `Role string` 仅 `direct` / `relay` 枚举，更推荐布尔 `DirectSubEnabled bool` 语义最直白；若希望中转节点也可按入站细粒度控制，则两层组合）：
       - **最小正交方案**：在 `domain.Inbound` 增加 `SubExposed bool`（GORM `gorm:"column:sub_exposed;default:true"`, JSON `subExposed`，`CreateInbound` 默认 `true`；为兼容老库零值问题可用 `*bool` 或迁移 `default:true`）。同时可选在 `domain.Node` 增加 `Role` 或 `DirectSubEnabled`（默认 `true`），作为**该节点下新建入站时前端 `subExposed` 的默认勾选值**及/或 **节点级总开关**。
     - **过滤收口**：在 `SubService.prepareInboundForNode`（`sub_service.go:81`）或 `InboundsToNodeConfigsWithDefaultFP` + `GetUserShareInfo` 统一判断：若 `!node.DirectSubEnabled` 或 `!inbound.SubExposed`，**直接从用户订阅与 Share API 中剔除**（完全不影响 `CompileRemoteNodeFull` 和 `extractStaticRelayClients` 的中转工作）。
   - **前端联动收束（极其关键，堵住默认全选漏洞）**：
     - `InboundFormDrawer`：把「是否暴露到用户订阅 (`subExposed`)」放在顶部 `① 基础网络与端口设置` 显著位置（若所属节点定位为中转落地，默认关闭）。**当 `subExposed == false` 时**：
       - 隐藏或折叠 `ExternalHost`、`ExternalPort`、`SortOrder`、`⑤ 分流订阅线路 (Sub-Routes)`、`⑥ 关联授权用户`（不再自动勾选所有用户！保存时不强行写 `user.inboundTags`）；
       - 保存后清除默认的 `🇯🇵 日本原生直连` 种子 SubRoute（或仅当用户点击“添加分流线路”时才生成，**绝不在默认模板中预填一条虚拟 SubRoute**，见 `web/src/views/inbounds/types.ts:163-173`）。
     - `UserFormDrawer`：在 `② 授权入站节点` 中过滤掉 `subExposed === false`（或所属节点关闭直连订阅）的入站，且默认不选内部中转入站。
     - `InboundTable`：增加“订阅暴露”徽章（如 `公开订阅` vs `仅内部中转`）及 `Remark` 显示，一眼分辨用途。

#### 候选方案 B：订阅命名收束（解决 `[**]直连` 混乱、模板断层与多出口不一致）
1. **统一命名组装点（Single Source of Truth）**：
   - 将当前分散在 `sub_service.go:56-123`（硬编码 `[Region]` 预拼接）、`sub_service.go:462-484`（`GetUserShareInfo` 独立拼接且漏掉模板）和 `node_converter.go:461-517`（`ApplyNodeNameTemplate`）的三套逻辑收拢到一处。
   - 不再在 `prepareInboundForNode` 中提前污染 `ibCopy.Remark` 和 `subRoutes[i].Name`；改为在 `NodeConfig.Params` 中注入结构化元字段：`node_name`、`node_region`（Master 可为空或可配置标签）、`route_name`、`outbound_tag`、`inbound_remark`。
2. **扩充 `sub_node_name_template` 占位符并修正默认策略**：
   - 保留现有 `{remark}`、`{tag}`、`{protocol}`、`{port}`、`{index}` 兼容，新增 `{region}`、`{node}`、`{route}`（SubRoute 名，无则为空）、`{outbound}`，并提供自动清理多余空白/空括号的归一化逻辑（避免 `region` 为空时出现 `[] 直连`）。
   - 例如默认策略可设为清晰可读的组合（如当存在 `region` 时展示 `{region} - {remark}` 或 `[{region}] {remark}`，或由统一默认模板控制 + 支持在 `SettingsView` 一键选择预设模板 + **实时预览示例**）。
   - 让 `GetUserShareInfo`（`sub_service.go:390-497`）与 `ExportUserSubscription` 共用同一套节点转换与命名渲染函数，消除“订阅链接名”和“面板分享弹窗名”不一致。
3. **移除前端新建入站时硬编码的 `🇯🇵 日本原生直连` 默认 SubRoute**（`web/src/views/inbounds/types.ts:163-173`）：
   - 让新建入站默认是“单一纯净入站（使用 `Inbound.Remark`）”，只有管理员显式需要单端口多出口分流时才点「添加线路」。这能立刻消除大量“本来只想建一个普通直连节点，结果被默认 SubRoute 名字覆盖成 `[**] 🇯🇵 日本原生直连`”的迷惑体验。
4. **（可选细节）重复名称自动消歧**：在 `ExportClashWithParams` / `ExportSingBox` 前检查重名，避免两个同名节点导致 Sing-box 导出配置报错。

---

### 4.2 第三项评估：「既然 Agent 互联了，要不要搞一键配置？直连默认协议怎么处理？会不会复杂+易错？」

基于当前源码真实能力，给出客观、克制的评估：

#### 结论先行
- **“跨节点一键中转互联（Master ↔ Agent Relay Pairing）”：值得做轻量级向导/半自动填充（低风险、高收益、已有基础设施支撑）**；
- **“全自动黑盒一键配置对外直连入站（如假设默认协议/端口/伪装域名/TLS 证书）”：强烈不建议做黑盒默认（极易出错、违背 KISS、运维排障灾难）**。

#### 源码依据与利弊拆解

| 维度 | 一键「Master ↔ Agent 中转链路」配对 | 一键「对外直连入站」黑盒默认配置 |
|---|---|---|
| **后端现有基础** | `extractStaticRelayClients`（`node_service.go:1510-1616`）**已经实现了自动凭据配对**，但仅完成了“从 Master Outbound 反推注入 Agent Inbound”的一半；前端仍需用户切节点、跨 3 个页面手动对齐 `Host`/`Port`/`UUID`/`pbk`/`SNI`/`ShortID`/`Flow` | `XrayCompiler` 对外直连入站强依赖宿主网络条件：443 端口是否被 Nginx 占用、REALITY `dest` 与 `serverNames` 连通性、X25519 密钥、域名解析与防火墙（`compiler.go:559-616`） |
| **复杂度与出错率** | **低复杂度、大幅降低出错率**：两边数据都在 Master 的 SQLite/Config 中，100% 确定性信息（`Node.Host`、随机生成的 UUID/端口、或内部中转参数），不存在外部公网探测不确定性 | **高出错率、黑盒幻觉**：<br>1. 443 端口在 GCP/JP 上常被占用或需要自定义端口；<br>2. REALITY 目标域名（`dest`/`serverNames`）硬编码公共域名极易被墙或被同一 SNI 封禁；<br>3. 若选 TLS 还要外部证书路径，选 XHTTP/WS 又要配合反代 |
| **协议选择争议** | **内部中转链路协议是内部实现细节，完全可控**：<br>• `VLESS + TCP + None`（或 `VLESS + Vision + REALITY` / `Shadowsocks` / `SOCKS5`）仅用于 Master → Agent 机房内/跨区互联。<br>• 注意源码现状：`extractStaticRelayClients` 目前支持 `vless`/`vmess`/`trojan`（`node_service.go:1522`），**不支持 `socks`**；且机房间公网直连建议带加密（如 `VLESS + REALITY` 或 `Trojan`/`VLESS-TLS`），不建议裸明文 SOCKS5 跨公网暴露（除非走内网/WireGuard/SSH 隧道） | **对外直连没有“放之四海皆准”的协议**：不同用户网络环境对 `VLESS-TCP-REALITY`、`VLESS-XHTTP`、`Trojan-TLS`、`VMess-WS` 需求完全不同，强行黑盒默认等于制造隐性故障 |
| **KISS 推荐做法** | **“一键生成中转链路”动作（可选、显式透明）**：在某远端中转节点的入站（或节点操作区）提供一个显式动作，例如「在 Master 生成指向此入站的中转出站」——自动填入 `node.Host`、`inbound.Port/ExternalPort`、协议、公钥 `pbk`、SNI、ShortID 与随机 UUID，**保存前让管理员可见可改**（而非黑盒隐藏） | **拒绝黑盒默认直连**：保留现有 `InboundFormDrawer` 的显式协议选择与参数填写（已有「一键生成 REALITY 密钥对」`generateRealityKey`），仅需移除默认冗余 `SubRoute` 并加清晰表单引导即可 |

---

## 5. 候选 `.trellis/spec/` 路径清单

本次若立项改造（涉及 DB Schema 变更、Service 订阅/节点逻辑、HTTP API 契约以及前端多页面/抽屉联动），需要加载以下 `.trellis/spec/` 规范文件：

| Spec 路径 | 核心覆盖内容与本次改造相关性 |
|---|---|
| `.trellis/spec/backend/index.md` | 后端分层拓扑（`domain` / `service` / `adapter` / `delivery` / `protocol` / `sub`）与边界约束 |
| `.trellis/spec/backend/database-guidelines.md` | SQLite WAL 模式、`AutoMigrate` 字段演进规范（新增 `Inbound.SubExposed` / `Node` 字段的向后兼容默认值处理） |
| `.trellis/spec/backend/directory-structure.md` | 领域模型、服务层、协议层职责划分（避免将订阅逻辑泄漏至 HTTP Handler 或 Repository） |
| `.trellis/spec/backend/error-handling.md` | 显式错误传播、`domain.ErrInvalidInput` 与 HTTP 状态码映射 |
| `.trellis/spec/backend/quality-guidelines.md` | KISS 原则、并发锁安全、单测覆盖要求 |
| `.trellis/spec/frontend/index.md` | Vue 3 扁平架构规范（禁止引入 Pinia，保持组件 `ref`/`computed` + `composables/`） |
| `.trellis/spec/frontend/api-and-dataflow.md` | `axios` 单例封装、类型定义同步与 mock 分流（改动 API DTO 时需同步检查 `web/src/mock/`） |
| `.trellis/spec/frontend/components-and-routing.md` | SFC `<script setup lang="ts">` 规范、UI 基础组件（`Drawer`/`SectionCard`/`FormField`/`Badge`）与路由 `nodeScoped` 约定 |
| `.trellis/spec/frontend/quality-guidelines.md` | `cd web && mise x -- npm run build` 静态构建与 `vue-tsc` 类型门禁 |
| `.trellis/spec/guides/cross-layer-thinking-guide.md` | **跨层必读**：DB → Go Domain/Service → Protocol/Sub → HTTP API → Vue UI 全链路字段一致性检查清单 |
| `.trellis/spec/guides/code-reuse-thinking-guide.md` | 消除 `SubService.ExportUserSubscription` 与 `GetUserShareInfo` 重复命名/过滤逻辑的复用指南 |
