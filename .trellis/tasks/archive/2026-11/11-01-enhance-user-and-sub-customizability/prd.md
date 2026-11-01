# 用户与订阅高可定制化改造 PRD

## Goal

全面开放用户核心身份凭证（Email、UUID、SubToken）、流控策略（Flow）、到期生命周期与备注的可变编辑与定制能力，彻底解决用户凭据变更时的 Xray 内存热重载与配置一致性缺陷；系统化增强订阅客户端的分发与定制能力，提供基于权重的节点排序、顶部虚拟信息提示节点、自定义节点命名模板以及多客户端导出参数注入，为多租户管理与多端客户端分发提供工业级定制化支持。

---

## Confirmed Facts & Technical Anchors

1. **用户凭证与数据模型现状**：
   - `internal/domain/user.go:10-40`：`domain.User` 模型目前缺少 `Remark`（备注）字段；
   - `internal/domain/user.go:108-117`：`UpdateUserDTO` 仅包含 `InboundTags`, `InboundTag`, `Flow`, `TotalBytes`, `ExpireTime`, `ResetDay`, `IPLimit`, `Enabled`，不支持修改 `Email`、`UUID`、`SubToken` 与 `Remark`；
   - `internal/service/user_service.go:18-28`：`CreateUserDTO` 缺少 `UUID`, `SubToken`, `Remark`, `ExpireTime` 字段，新建用户时在 `user_service.go:105-115` 无条件由系统随机生成 UUID 和 SubToken，不允许管理员指定自定义凭证。

2. **Xray 核心热重载凭证同步缺陷**：
   - `internal/service/user_service.go:259-285`：`UpdateUser` 在与 Xray 核心进行 gRPC 热同步时，调用 `xrayManager.RemoveUser(ctx, tag, updatedUser.Email)`。Xray-core gRPC API 的 `RemoveUser` 是通过 `Email` 字符串作为 Handler 内部用户的唯一索引进行匹配的。若用户的 `Email` 发生变更，传入新 Email 将无法匹配并移除 Xray 内存中的旧用户，导致旧凭证残留与新凭证注入冲突；
   - `internal/domain/inbound.go:41-52`：`SubRoute.AllowedUsers []string` 存储的是用户的 `Email`。若用户 `Email` 变更，现有各入站分流线路的授权列表将产生悬挂旧邮箱引用；
   - `internal/service/user_service.go:230-250`：`UpdateUser` 的 Saga 补偿回滚逻辑（`rollback`）未涵盖 `Email`/`UUID` 变更回滚以及 `SubRoute.AllowedUsers` 引用回滚。

3. **流控策略（Flow）多层解析断层**：
   - `internal/domain/user.go:16`：`User.Flow` 字段已持久化，但前端 `UserFormDrawer.vue` 缺少显式流控选择器；
   - `internal/protocol/node_converter.go:240-261`：`InboundToNodeConfig` 对 VLESS Flow 进行了判断，但未严格支持用户级 `"none"`（显式禁用 Flow，即便节点为 Vision 节点也强行剔除 flow 参数）；
   - `internal/adapter/xray/protocol_adapter.go:136, 151`：`VLESSAdapter.CompileClients` 与 `BuildAccount` 直接调用 `accessor.ResolveVisionFlow(inbound.Protocol)`，完全忽略了用户维度的 `u.Flow` 配置。

4. **有效期与备注管理交互断层**：
   - `internal/domain/user.go:21`：`ExpireTime` 存储 Unix 毫秒时间戳，`0` 表示永不过期；
   - `web/src/views/users/components/UserFormDrawer.vue:104-128`：创建时仅支持相对天数 `expireDays`，编辑时仅支持增量延期 `extendDays`，无法直接指定具体截止日期，亦无法一键将到期时间重置为永久有效（0）；
   - `web/src/views/users/` 体系中（`UserTable.vue`, `UserDetailDrawer.vue`, `UserFormDrawer.vue`）均未支持管理员填报和查看用户备注。

5. **订阅与客户端分发定制缺失**：
   - `internal/domain/inbound.go:8-45`：`domain.Inbound` 与 `domain.SubRoute` 均无排序权重字段 `SortOrder`；
   - `internal/service/sub_service.go:26-80`：`resolveSubscriptionNodes` 获取入站列表后未做确定性排序，SubRoute 展开顺序依赖 JSON 切片原始顺序；
   - `internal/service/sub_service.go:85-150`：缺乏客户端顶部状态提示节点（流量、到期时间、自定义公告）；
   - `internal/delivery/http/handler_sub.go:49-75`：`Profile-Update-Interval` 硬编码为 `24`，缺少 `Profile-Title` 响应头；
   - `internal/sub/clash.go:46-70` 与 `internal/sub/singbox.go:40-80`：缺少从全局设置动态读取端口、运行模式（mode）、允许局域网（allow-lan）等客户端参数的能力；
   - `web/src/views/SettingsView.vue` 与 `web/src/views/inbounds/components/InboundFormDrawer.vue`：缺少上述配置的交互表单。

---

## Requirements

### 1. 用户核心身份凭据可变性与热同步改造 (A1)
- **1.1 创建用户支持自定义凭证 (`CreateUserDTO`)**：
  - 允许在 `CreateUserDTO` 中选填 `uuid` 与 `subToken`；若未填写或为空，则保持原有逻辑自动生成；
  - 若传入自定义 `uuid`，必须校验其符合标准 UUID 格式（v4/RFC4122），并通过 `userRepo.GetByUUID` 校验数据库全局唯一性；
  - 若传入自定义 `subToken`，必须校验长度（≥8字符）与字符集（字母数字），并通过 `userRepo.GetBySubToken` 校验数据库全局唯一性；
  - 严格保持 `Email` 的唯一性检查（`userRepo.GetByEmail`）。
- **1.2 编辑用户开放身份凭据修改 (`UpdateUserDTO`)**：
  - 在 `domain.UpdateUserDTO` 中增加选填字段：`Email string`、`UUID string`、`SubToken string`、`Remark string`；
  - 在 `UserService.UpdateUser` 中进行变更前瞻校验：若 `Email` 变更，需检查新邮箱未被其他用户占用；若 `UUID` 变更，需校验 UUID 格式及唯一性；若 `SubToken` 变更，需校验格式及唯一性。
- **1.3 Xray 内存热重载与旧凭据安全注销 (Crucial Sync & Hot-Reload)**：
  - 当 `Email` 或 `UUID` 发生变更时，`UserService.UpdateUser` 必须使用旧快照中的身份信息（`oldSnapshot.Email`）先调用 `xrayManager.RemoveUser(ctx, tag, oldSnapshot.Email)`，从该用户原有授权的所有节点（`oldTags`）内存中注销旧凭证；
  - 若用户处于正常激活状态（`updatedUser.IsActive()`），再以新身份（`updatedUser.Email`、`updatedUser.UUID`、`updatedUser.Flow`）调用 `xrayManager.AddUser(ctx, tag, &updatedUser)` 注入到新授权节点；
  - 调用 `configSvc.SyncUserToFile` 将变更后的配置持久化回写至 `config.json`。
- **1.4 级联更新 SubRoute 线路授权引用**：
  - 当用户 `Email` 从 `oldEmail` 变更为 `newEmail` 时，`UserService.UpdateUser` 遍历系统中所有入站配置，将 `SubRoute.AllowedUsers` 中包含的 `oldEmail` 自动更新替换为 `newEmail` 并持久化，避免分流线路授权产生悬挂失效引用。
- **1.5 Saga 事务补偿回滚闭环**：
  - 若更新过程中任何 Xray 注入或配置落盘发生致命错误，触发 Saga 反向补偿：
    1. 逆序从所有新节点移除新用户凭据（`xrayManager.RemoveUser(compCtx, tag, updatedUser.Email)`）；
    2. 若原快照用户处于激活态，将 `oldSnapshot` 重新注入回 `oldTags`（`xrayManager.AddUser(compCtx, tag, &oldSnapshot)`）；
    3. 恢复入站配置中所有 `SubRoute.AllowedUsers` 的 Email 引用；
    4. 将 SQLite 中的用户记录恢复为 `oldSnapshot`。

### 2. 用户协议流控、灵活到期策略与备注支持 (A2)
- **2.1 用户级 Flow 流控覆盖机制**：
  - `User.Flow` 规范定义为三态字符串：
    - `""`（空字符串）：跟随节点默认流控策略；
    - `"xtls-rprx-vision"`：强制启用 Vision 流控（仅限 TCP + TLS/REALITY 的 VLESS 节点）；
    - `"none"`：强制显式禁用流控（即使当前节点开启了 Vision，也对该用户剔除 flow 参数）；
  - `internal/protocol/node_converter.go:240-261`：在转换客户端连接时，若 `user.Flow == "none"`，强制将 `flow` 置空；若为 `"xtls-rprx-vision"` 且底层传输兼容，则应用该流控；若为空则跟随节点/全局默认；非兼容传输（如 xhttp、ws、grpc）一律强制清除 flow；
  - `internal/adapter/xray/protocol_adapter.go:129-160`：在 `VLESSAdapter.CompileClients` 与 `BuildAccount` 中，读取 `user.Flow` 进行用户级覆盖（若为 `"none"` 则 flow 置空；若为有效值且节点支持则应用；若为空则回退至 `inboundFlow`）。
- **2.2 灵活生命周期与到期时间策略**：
  - `CreateUserDTO` 补充 `ExpireTime int64`，与原有 `ExpireDays int` 兼容：若传入大于 0 的 `ExpireTime` 则直接作为截止毫秒时间戳；若未传则按 `ExpireDays` 计算；若均为 0 则表示永不过期；
  - `UpdateUserDTO` 支持直接传入目标 `ExpireTime`（毫秒时间戳）；传入 `0` 表示清空到期时间，设置为永不过期；
  - 前端 `UserFormDrawer.vue` 支持：
    1. 具体日期时间选择器（选择到年月日时分）；
    2. 快捷加时按钮（`+30天`、`+90天`、`+365天`，在现有到期时间或当前时间基础上累加）；
    3. "设为永久有效" 快捷按钮（将 `expireTime` 置为 0）。
- **2.3 用户备注 (Remark) 闭环**：
  - `domain.User` 增加 `Remark string`（`gorm:"type:varchar(255)" json:"remark"`）；
  - `CreateUserDTO` 与 `UpdateUserDTO` 增加 `Remark string`（支持更新与清空）；
  - 前端在 `UserFormDrawer.vue` 增加备注输入框（最大255字符）；
  - 前端在 `UserTable.vue` 列表与 `UserDetailDrawer.vue` 巡检抽屉中展示备注信息，并在表格搜索栏中支持按备注关键词检索。

### 3. 订阅分发与客户端高级定制扩展 (B1)
- **3.1 客户端显示排序权重 (`sortOrder`)**：
  - `domain.Inbound` 增加 `SortOrder int`（`gorm:"default:0" json:"sortOrder"`）；
  - `domain.SubRoute` 增加 `SortOrder int`（`json:"sortOrder"`）；
  - 排序权重规范：数值越小排序越靠前（`SortOrder ASC`）；
  - `SubService.resolveSubscriptionNodes` 与 `node_converter.go` 针对订阅节点列表执行确定性排序：优先按 `Inbound.SortOrder ASC`，相同时按 `Inbound.ID ASC`；针对入站内展开的 SubRoute，按 `SubRoute.SortOrder ASC`；
  - `InboundFormDrawer.vue` 提供入站节点排序权重与各 SubRoute 线路排序权重的输入控制。
- **3.2 客户端顶部虚拟状态提示节点 (Top Info Virtual Nodes)**：
  - 依赖全局系统设置开关 `sub_show_info_nodes`（布尔值，默认 `true`）；
  - 当开启且请求全量聚合订阅时，在生成的节点列表最顶部（第 1、2 位）前置插入格式兼容的虚拟信息节点：
    - **节点 #1 (流量状态)**：
      - 若设置了流量上限：`📊 剩余流量: X.XX GB / Y.XX GB`（已用达量时显示剩余配额）；
      - 若未限制流量：`📊 已用流量: X.XX GB (不限配额)`；
    - **节点 #2 (到期状态)**：
      - 若设置了到期时间：`⏳ 到期时间: YYYY-MM-DD`（格式化为本地标准日期）；
      - 若为永不过期：`⏳ 到期时间: 长期有效`；
    - **节点 #3 (可选自定义公告)**：
      - 若系统设置 `sub_custom_announcement` 非空，前置展示 `📢 公告: <内容>`；
  - 虚拟节点技术实现：采用通用且安全的假节点结构（如 `trojan://00000000-0000-0000-0000-000000000000@127.0.0.1:10000#<名称>`），在 Base64、Clash（作为纯展示 proxy）、Sing-box 下均能被各主流客户端平滑解析为只读提示项，杜绝解析崩溃。
- **3.3 节点命名自定义模板 (`sub_node_name_template`)**：
  - 支持在系统设置中配置 `sub_node_name_template`（留空或未设置时默认使用 `{remark}`）；
  - 支持占位符变量：
    - `{remark}`：节点/分流线路备注名（若为空自动回退为 `{tag}`）；
    - `{tag}`：Inbound Tag；
    - `{protocol}`：协议大写（如 VLESS, TROJAN）；
    - `{port}`：外部连接端口；
    - `{index}`：该用户订阅中的序号（1, 2, 3...）；
  - 例如模板 `{remark} [{protocol}]` 或 `[{tag}] {remark}`，生成如 `🇯🇵 日本原生直连 [VLESS]`。
- **3.4 订阅客户端导出高级参数注入 (Export Parameters)**：
  - 全局系统设置支持以下客户端分发参数（存储于 `domain.SettingRepository`）：
    - `sub_profile_title`：订阅集合名称，注入至 HTTP 响应头 `Profile-Title: <title>`；
    - `sub_update_interval`：客户端自动更新周期（小时，默认 24），注入至 HTTP 响应头 `Profile-Update-Interval: <hours>`；
    - `sub_clash_mixed_port`：Clash 混合监听端口（默认 7890）；
    - `sub_clash_allow_lan`：Clash 是否允许局域网连接（`allow-lan`，布尔值，默认 false）；
    - `sub_clash_mode`：Clash 默认运行模式（`rule` / `global` / `direct`，默认 `rule`）；
    - `sub_default_fingerprint`：TLS/REALITY 客户端默认伪装指纹（`chrome` / `firefox` / `safari` / `random`，默认 `chrome`）；
  - `internal/delivery/http/handler_sub.go`、`internal/sub/clash.go`、`internal/sub/singbox.go` 与 `internal/protocol/node_converter.go` 消费上述参数并在导出配置中生效。
- **3.5 系统设置交互面板更新**：
  - `web/src/views/SettingsView.vue` 增加 "订阅客户端分发与展示配置" 模块，集中维护上述配置项，提供零硬编码的动态调整能力。

---

## Acceptance Criteria

- [ ] **A1.1 凭据创建定制**：调用 `CreateUser` 传入指定 `UUID` 与 `SubToken`，持久化并注入 Xray；传入重复 `UUID`、`SubToken` 或不合规格式时被明确拒绝。
- [ ] **A1.2 凭据编辑与唯一性**：调用 `UpdateUser` 修改 `Email`、`UUID`、`SubToken`、`Remark`；若修改为已被其他用户占用的 `Email`、`UUID` 或 `SubToken`，返回冲突错误并不破坏现有数据。
- [ ] **A1.3 Xray 旧身份清理验证**：在 `UpdateUser` 修改 `Email` 或 `UUID` 时，Mock XrayManager 的 `RemoveUser` 必须收到 `oldSnapshot.Email` 调用，随后以新凭据调用 `AddUser`，验证 Xray 内存无孤儿旧身份。
- [ ] **A1.4 SubRoute 引用级联更新**：当修改用户 `Email` 后，所有入站中 `SubRoute.AllowedUsers` 内包含的旧邮箱均被同步更新为新邮箱。
- [ ] **A1.5 Saga 补偿回滚测试**：在修改 `Email`/`UUID` 流程中注入 Xray 或配置落盘故障，验证 SQLite、Xray 内存与 SubRoute 引用完整恢复至 `oldSnapshot`。
- [ ] **A2.1 Flow 覆盖验证**：
  - 用户 `Flow = "none"` 时，节点即便为 TCP+REALITY 的 VLESS 节点，生成的订阅连接与 Xray 客户端配置中 `flow` 参数均被强制剔除；
  - 用户 `Flow = "xtls-rprx-vision"` 时，在兼容节点上正确附加 `xtls-rprx-vision`；
  - 非兼容传输（如 xhttp、ws）无论用户设置何种 Flow，均强制清除 `flow` 参数。
- [ ] **A2.2 到期策略与清零**：在前端与 API 将 `expireTime` 设置为具体时间戳或 `0`，验证到期判断逻辑符合预期（`0` 时 `IsExpired() == false` 且客户端顶部显示长期有效）。
- [ ] **A2.3 备注功能**：数据库能正确保存 `Remark`，并在前端用户列表、编辑抽屉、巡检详情中完整呈现与检索。
- [ ] **B1.1 节点权重排序**：调整 Inbound 与 SubRoute 的 `sortOrder`，导出的节点列表严格遵循 `Inbound.SortOrder ASC, Inbound.ID ASC, SubRoute.SortOrder ASC` 顺序。
- [ ] **B1.2 顶部信息节点生成**：当 `sub_show_info_nodes = true` 时，订阅内容最前置包含流量、到期时间及自定义公告虚拟节点；在 Base64、Clash、Sing-box 导出格式中均能合规解析。
- [ ] **B1.3 命名模板生效**：配置 `sub_node_name_template`，验证生成的节点名称符合模板替换规则。
- [ ] **B1.4 客户端导出参数注入**：验证 `/sub/:token` 响应头包含配置的 `Profile-Title` 与 `Profile-Update-Interval`；导出的 Clash 配置包含配置的 `port`、`allow-lan` 与 `mode`。
- [ ] **全量门禁保证**：
  - `mise x -- go test ./...` 单元测试全部绿灯通过；
  - `mise x -- go vet ./...` 静态分析无报错与警告；
  - `mise x -- go build .` 后端完整二进制编译成功；
  - `cd web && mise x -- npm run build` 前端 Vite 构建与 TypeScript 检查成功。

---

## Out of Scope

1. **实时客户端推送通知**：不引入针对第三方代理客户端的实时推送机制（客户端通过原有的周期性拉取更新订阅即可获取最新凭据）。
2. **商业化支付与发票自动化**：不涉及在线支付网关、自动续费充值等商业财务逻辑。
3. **针对单用户的自定义分流脚本**：用户分流继续基于现有的多节点授权（`InboundTags`）与分流线路白名单（`AllowedUsers`），不在此任务中引入单用户级动态路由脚本引擎。
4. **非标准协议与私有代理格式**：虚拟信息节点限定遵循标准 Trojan / VLESS 协议虚构语法，不开发私有格式协议扩展。

---

## Open Questions

无（所有关于凭证可变性、Flow 覆盖优先级、Saga 补偿机制、节点排序权重与虚拟信息节点实现的方案均已在规划评审中获得确认）。
