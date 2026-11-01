# 用户与订阅高可定制化改造 - 实施计划 (Implement Plan)

## Ordered Implementation Checklist

### 1. Fail-Repro 阶段（红灯测试先行）
- [ ] 1.1 在 `internal/service/user_service_test.go` 编写失败测试用例：
  - 测试 `UserService.CreateUser` 传入自定义 `UUID` 和 `SubToken` 能正确保存，且重复凭据返回 `ErrAlreadyExists`；
  - 测试 `UserService.UpdateUser` 修改 `Email` 与 `UUID` 时，验证 Mock XrayManager 严格按 `oldSnapshot.Email` 收到 `RemoveUser` 调用，再以新身份执行 `AddUser`；
  - 测试当修改 `Email` 时，所有入站节点中包含该用户邮箱的 `SubRoute.AllowedUsers` 被自动同步替换为新邮箱；
  - 故障注入测试：在 `UpdateUser` 发生 Email 变更时模拟 Xray 或物理文件持久化失败，断言 Saga 补偿完整执行（DB 恢复旧数据、Xray 重新注入旧凭据、`SubRoute.AllowedUsers` 恢复旧邮箱）。
- [ ] 1.2 在 `internal/protocol/node_converter_test.go` 编写流控与命名测试用例：
  - 测试 `user.Flow = "none"` 时，即便入站配置为 TCP+REALITY 的 VLESS 节点，转换出的节点 `flow` 参数依然为空；
  - 测试 `user.Flow = "xtls-rprx-vision"` 时，在兼容节点上正常附加 flow 参数；
  - 测试 `sub_node_name_template` 占位符解析（支持 `{remark}`, `{tag}`, `{protocol}`, `{port}`, `{index}`）。
- [ ] 1.3 在 `internal/service/sub_service_test.go` 编写订阅节点排序与信息节点测试用例：
  - 验证节点严格按照 `Inbound.SortOrder ASC, Inbound.ID ASC, SubRoute.SortOrder ASC` 确定性排序；
  - 验证当 `sub_show_info_nodes = true` 时，订阅列表最前置生成流量与到期提示节点；
  - 验证当配置了 `sub_custom_announcement` 时，最前置生成公告提示节点。
- [ ] 1.4 在 `internal/sub/sub_test.go` 编写 Clash 与 Sing-box 参数注入测试用例：
  - 验证 Clash 导出时正确注入 `mixed-port`、`allow-lan` 与 `mode`；
  - 验证 Sing-box 导出配置的合规性。

### 2. 领域层与仓储契约改造 (Domain & Repository Layer)
- [ ] 2.1 修改 `internal/domain/user.go`：
  - `User` 结构体增加 `Remark string`（`gorm:"type:varchar(255)" json:"remark"`）；
  - `UpdateUserDTO` 补充 `Email string`、`UUID string`、`SubToken string`、`Remark string` 字段，并在 `UnmarshalJSON` 中做兼容解析。
- [ ] 2.2 修改 `internal/domain/inbound.go`：
  - `Inbound` 结构体增加 `SortOrder int`（`gorm:"default:0" json:"sortOrder"`）；
  - `SubRoute` 结构体增加 `SortOrder int`（`json:"sortOrder"`）。
- [ ] 2.3 修改 `internal/service/user_service.go`：
  - `CreateUserDTO` 补充 `UUID string`、`SubToken string`、`Remark string`、`ExpireTime int64` 字段。

### 3. 核心服务与编排层实现 (Core Orchestration & Protocol Engine)
- [ ] 3.1 修改 `internal/service/user_service.go`：
  - `CreateUser` 增加自定义凭证分支与唯一性校验（`userRepo.GetByUUID`, `userRepo.GetBySubToken`）；
  - `UpdateUser` 增加字段合并与唯一性校验；
  - 重构 Xray 内存同步：在处理旧节点与现有节点时，以 `oldSnapshot.Email` 调用 `RemoveUser`，并在激活态以新凭证调用 `AddUser`；
  - 实现 `SubRoute.AllowedUsers` 引用级联更新逻辑；
  - 完善 Saga 补偿回滚：纳入 Email/UUID 变更回滚与 `SubRoute.AllowedUsers` 逆向恢复。
- [ ] 3.2 修改 `internal/protocol/node_converter.go`：
  - 完善流控参数判定：严格识别用户级 `user.Flow`（`"none"` 强行剔除，`"xtls-rprx-vision"` 兼容应用，`""` 跟随节点）；
  - 支持 `sub_node_name_template` 占位符渲染；
  - 支持 `sub_default_fingerprint` 缺省回退。
- [ ] 3.3 修改 `internal/adapter/xray/protocol_adapter.go`：
  - 在 `VLESSAdapter.CompileClients` 与 `BuildAccount` 中支持用户级 `u.Flow` 对节点 Flow 的覆盖与剔除。
- [ ] 3.4 修改 `internal/service/sub_service.go`：
  - 改造 `resolveSubscriptionNodes`：先按 `Inbound.SortOrder ASC, Inbound.ID ASC` 排序，展开 SubRoutes 时按 `SubRoute.SortOrder ASC` 排序；
  - 实现顶部虚拟信息节点生成方法 `buildTopInfoNodes(user, announcement)` 并前置插入；
  - 统一在 `GetUserShareInfo` 中应用相同排序。
- [ ] 3.5 修改 `internal/sub/clash.go` 与 `singbox.go`：
  - 改造导出函数，支持外部传入 `ClashParams`（`port`, `allowLan`, `mode` 等）；
  - 完善 `sub.ExportSubscription` 策略调用链。
- [ ] 3.6 修改 `internal/delivery/http/handler_sub.go`：
  - 从 `settingRepo` 读取 `sub_profile_title` 与 `sub_update_interval`，动态写入 `Profile-Title` 与 `Profile-Update-Interval` 响应头。

### 4. 前端界面与交互改造 (Web Presentation Layer)
- [ ] 4.1 修改 `web/src/views/users/types.ts` 与 `sanitizeUserPayload`：
  - `UserItem` 接口增加 `remark?: string`；
  - `UserFormData` 接口增加 `uuid?: string`、`subToken?: string`、`remark?: string`、`expireTime?: number`；
  - 更新 `sanitizeUserPayload` 保持 DTO 字段对齐。
- [ ] 4.2 改造 `web/src/views/users/components/UserFormDrawer.vue`：
  - 解锁 `email` 输入框（编辑模式允许修改）；
  - 增加 `UUID` 与 `SubToken` 输入框，附带一键随机生成按钮；
  - 增加 `Flow` 模式下拉单选（默认跟随节点、强制开启 Vision、强制禁用 Flow）；
  - 增加到期时间具体日期时间选择器，保留快捷加时按钮（+30d, +90d, +365d），增加 "设为永久有效" 按钮；
  - 增加 `Remark`（备注）文本输入框。
- [ ] 4.3 改造 `web/src/views/users/components/UserTable.vue` 与 `UserDetailDrawer.vue`：
  - 在用户列表表格和巡检抽屉中呈现 `Remark`（备注）；
  - 在表格顶部搜索栏中支持按备注内容过滤用户。
- [ ] 4.4 改造 `web/src/views/inbounds/types.ts` 与 `InboundFormDrawer.vue`：
  - 入站基础配置增加 `sortOrder`（显示排序权重，数值越小越靠前）输入框；
  - SubRoute 线路列表中增加单线路 `sortOrder` 输入框。
- [ ] 4.5 改造 `web/src/views/SettingsView.vue`：
  - 增加 "订阅客户端高级定制与展示配置" 卡片模块；
  - 表单项涵盖：状态提示节点开关 (`sub_show_info_nodes`)、自定义公告 (`sub_custom_announcement`)、订阅集合标题 (`sub_profile_title`)、自动更新间隔 (`sub_update_interval`)、节点命名模板 (`sub_node_name_template`)、Clash 混合端口 (`sub_clash_mixed_port`)、Clash 允许局域网 (`sub_clash_allow_lan`)、Clash 运行模式 (`sub_clash_mode`)、默认 TLS 指纹 (`sub_default_fingerprint`)；
  - 对接 `api.post('/settings')` 统一持久化。

### 5. 门禁验证与交付测试
- [ ] 5.1 运行后端全套单元测试：`mise x -- go test ./...`（确保 100% 绿灯）。
- [ ] 5.2 运行后端静态代码检查：`mise x -- go vet ./...`（确保无 warning）。
- [ ] 5.3 运行后端二进制构建验证：`mise x -- go build .`。
- [ ] 5.4 运行前端静态检查与生产打包：`cd web && mise x -- npm run build`（确保 TypeScript 类型与 Vite 打包通过）。

---

## Validation Commands

在完成所有代码修改后，必须在终端依次执行并通过以下门禁命令：

```bash
# 1. 运行相关单元测试（红灯先行与全绿确认）
mise x -- go test -v ./internal/service -run "TestUser"
mise x -- go test -v ./internal/protocol -run "TestNodeConverter"
mise x -- go test -v ./internal/sub/...

# 2. 运行后端全量单元测试与静态检查
mise x -- go test ./...
mise x -- go vet ./...

# 3. 运行后端二进制构建
mise x -- go build .

# 4. 运行前端生产编译构建与类型门禁
cd web && mise x -- npm run build
```

---

## Risky Files & Rollback Points

1. **`internal/service/user_service.go`**：
   - **风险点**：`UpdateUser` 涉及 Xray 核心内存中旧身份与新身份的交替注销/注入以及跨 Inbound 的 `SubRoute.AllowedUsers` 级联修改。若旧身份注销失败且吞错，将造成内存孤儿凭据。
   - **防御策略**：引入完备的 Saga 补偿事务，确保在任何错误分支下，数据库、Xray 内存和 SubRoute 引用均原子化回滚至原快照 `oldSnapshot`。
2. **`internal/protocol/node_converter.go` & `internal/adapter/xray/protocol_adapter.go`**：
   - **风险点**：在非 TCP 或非 TLS/REALITY 传输协议上意外下发 `flow: "xtls-rprx-vision"` 会导致 Xray-core 启动校验失败崩溃。
   - **防御策略**：严格遵循传输层白名单守卫（仅 VLESS + TCP + TLS/REALITY 允许携带 flow），其他所有网络组合无条件剥除 flow 参数。
3. **`internal/domain/user.go` & `inbound.go`**：
   - **风险点**：数据库表字段新增（`remark`, `sort_order`）。
   - **防御策略**：借助 GORM 自动迁移机制新增列，不修改任何主键、唯一键或既有数据类型，具备完全向前与向后兼容性。

---

## Follow-up Checks before task.py start

- [x] `prd.md` 已通过 PRD 收敛审查（目标明确、文件行号事实源锚定、需求与验收标准逐条映射、无未解决开放问题）。
- [x] `design.md` 架构契约、Saga 补偿时序与订阅管道设计完整。
- [x] `implement.md` 步骤可执行性、测试先行规划与门禁命令齐备。
- [x] `.trellis/tasks/11-01-enhance-user-and-sub-customizability/` 上下文清单（`implement.jsonl`, `check.jsonl`）已通过 `task.py validate` 验证。
