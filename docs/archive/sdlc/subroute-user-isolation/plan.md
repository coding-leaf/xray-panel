# Plan: SubRoute 节点分流线路细粒度用户权限隔离 - 实施计划

- **关联 Spec**: subroute-user-isolation
- **实施执行人 / Agent**: Dev
- **当前状态**: Draft
- **Change Tier**: Tier 3 (跨领域重构与端到端协同)

---

## 1. 变更文件清单 (Files that change)

### 领域模型与订阅展示层 (Domain & Service Layer)
* `internal/domain/inbound.go` (Modify: `SubRoute` 结构体增加 `AllowedUsers []string` 字段，实现纯函数 `CanAccess(email string) bool`)
* `internal/domain/inbound_test.go` (Modify/New: 增加 `SubRoute.CanAccess` 的全覆盖单元测试，覆盖空用户、白名单命中与未命中场景)
* `internal/protocol/node_converter.go` (Modify: `InboundsToNodeConfigs` 遍历 SubRoutes 时加入 `!sr.CanAccess(user.Email)` 过滤逻辑)
* `internal/protocol/node_converter_test.go` (Modify: 新增分流线路基于用户白名单过滤的单元测试用例)
* `internal/service/sub_service.go` (Modify: `GetUserShareInfo` 遍历 SubRoutes 时加入 `!sr.CanAccess(user.Email)` 过滤逻辑)
* `internal/service/sub_service_test.go` (Modify: 补充用户根据 `AllowedUsers` 获取不同订阅节点列表的单测)

### Xray 路由编译与内核适配 (Adapter Layer)
* `internal/adapter/xray/schema.go` (Modify: `XrayRoutingRule` 增加 `User []string` 字段，序列化标签为 `json:"user,omitempty"`)
* `internal/adapter/xray/compiler.go` (Modify: 在 Layer 3 网关通道分流规则编译时，将 `sr.AllowedUsers` 映射至 `XrayRoutingRule.User`)
* `internal/adapter/xray/compiler_test.go` (Modify: 补充路由编译测试用例，断言 `user` 匹配规则在有/无白名单时的 JSON 渲染)

### 前端交互与 Mock 适配 (Web Layer)
* `web/src/views/InboundsView.vue` (Modify: 分流线路抽屉中每条 SubRoute 增加授权用户多选控件与全员开放缺省提示，更新 TypeScript 接口)
* `web/src/mock/index.ts` (Modify: Mock 订阅生成函数适配 `sr.allowedUsers` 过滤，确保本地开发测试行为与后端严格一致)

---

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)

### Milestone 1: 后端核心领域模型与展示层过滤 (M1)
> 目标：建立 `SubRoute` 权限隔离核心领域方法，并在展示层（订阅下发与节点转换）实施白名单过滤，确保未授权用户无法在订阅中看到受限节点。

* **Step 1.1 (Fail-repro 先行测试 - 领域模型与方法)**:
  - 在 `internal/domain/inbound_test.go` 中编写 `TestSubRoute_CanAccess` 失败测试：
    - Case A: `AllowedUsers` 为空切片或 nil，任意 email 调用均返回 `true`（全员开放/向后兼容）。
    - Case B: `AllowedUsers` 包含 `["alice@test.com", "bob@test.com"]`，传入 `"alice@test.com"` 返回 `true`，传入 `"charlie@test.com"` 返回 `false`。
  - 局部验证命令: `go test -race -v -run TestSubRoute_CanAccess ./internal/domain`
  - 预期判据: 编译报错（`CanAccess` 未定义）或测试失败（Red）。

* **Step 1.2 (实现领域模型与纯函数)**:
  - 修改 `internal/domain/inbound.go`：
    - `SubRoute` 添加 `AllowedUsers []string` (`json:"allowedUsers,omitempty"`)。
    - 实现纯函数方法 `CanAccess(email string) bool`：若 `len(sr.AllowedUsers) == 0` 返回 `true`；遍历 `AllowedUsers`，若匹配传入的 `email` 则返回 `true`；遍历结束返回 `false`。
  - 局部验证命令: `go test -race -v -run TestSubRoute_CanAccess ./internal/domain`
  - 预期判据: 纯函数测试用例全部通过变绿（Pass），覆盖率 100%。

* **Step 1.3 (Fail-repro 先行测试 - 协议转换与订阅层展示过滤)**:
  - 修改 `internal/protocol/node_converter_test.go`，增加 `TestInboundsToNodeConfigs_UserIsolation`：
    - 构造带 SubRoutes 的 Inbound，线路 A 设为全员开放，线路 B 设为仅 `vip@test.com` 可见。
    - 针对普通用户 `user@test.com` 转换，预期只生成 1 个 NodeConfig（线路 A）。
    - 针对 VIP 用户 `vip@test.com` 转换，预期生成 2 个 NodeConfig（线路 A 与 线路 B）。
    - 若 Inbound 下所有 SubRoutes 用户均无权访问，预期生成 0 个 NodeConfig。
  - 修改 `internal/service/sub_service_test.go`，增加 `TestSubService_GetUserShareInfo_UserIsolation` 失败用例。
  - 局部验证命令: `go test -race -v -run "TestInboundsToNodeConfigs_UserIsolation|TestSubService_GetUserShareInfo_UserIsolation" ./internal/protocol ./internal/service`
  - 预期判据: 测试断言失败，未授权用户依然拿到了未过滤的线路（Red）。

* **Step 1.4 (实现展示层过滤逻辑并转绿)**:
  - 修改 `internal/protocol/node_converter.go` 的 `InboundsToNodeConfigs`：在 `for _, sr := range subRoutes` 循环中，增加 `if !sr.CanAccess(user.Email) { continue }`。
  - 修改 `internal/service/sub_service.go` 的 `GetUserShareInfo`：在 `for _, sr := range subRoutes` 循环中，增加 `if !sr.CanAccess(user.Email) { continue }`。
  - 局部验证命令: `go test -race -v -run "TestInboundsToNodeConfigs_UserIsolation|TestSubService_GetUserShareInfo_UserIsolation" ./internal/protocol ./internal/service`
  - 预期判据: 协议转换与订阅生成单测全部通过变绿（Pass）。

---

### Milestone 2: Xray 内核路由强隔离编译与测试 (M2)
> 目标：在底层 Xray 路由规则中注入 `user` 匹配列表，在内核层面物理阻断伪造 `routeId` 的跨线路越权穿透。

* **Step 2.1 (Fail-repro 先行测试 - Xray 路由规则与编译器)**:
  - 修改 `internal/adapter/xray/compiler_test.go`，增加 `TestCompiler_SubRoute_UserIsolation`：
    - Case A: Inbound 配置含 `AllowedUsers: ["alice@test.com"]` 的 SubRoute，编译结果中对应 Layer 3 路由规则必须包含 `"user": ["alice@test.com"]`。
    - Case B: SubRoute 未指定 `AllowedUsers` 时，编译生成的 JSON 路由规则中不得包含 `"user"` 键（保持 omitempty 契约与原有行为无缝兼容）。
  - 局部验证命令: `go test -race -v -run TestCompiler_SubRoute_UserIsolation ./internal/adapter/xray`
  - 预期判据: 编译报错（`XrayRoutingRule.User` 未定义）或测试断言不符（Red）。

* **Step 2.2 (实现 Xray Schema 扩展与 Layer 3 路由编译逻辑)**:
  - 修改 `internal/adapter/xray/schema.go`：在 `XrayRoutingRule` 结构体中新增 `User []string` (`json:"user,omitempty"`)。
  - 修改 `internal/adapter/xray/compiler.go`：在编译 Layer 3 分流规则循环中，检查 `if len(sr.AllowedUsers) > 0 { rule.User = sr.AllowedUsers }`。
  - 局部验证命令: `go test -race -v -run TestCompiler_SubRoute_UserIsolation ./internal/adapter/xray`
  - 预期判据: 编译器单元测试全绿（Pass）。

* **Step 2.3 (集成与防回归校验)**:
  - 运行 Xray 模块全量测试，验证现有默认分流、Reality、VLESS 路由均无回归：
  - 局部验证命令: `go test -race -v ./internal/adapter/xray/...`
  - 预期判据: 所有用例通过，无竞态问题。

---

### Milestone 3: 前端交互适配与全链路端到端构建验证 (M3)
> 目标：提供直观的 SubRoute 授权用户配置界面，适配 Mock 订阅生成，并执行前端打包与全仓库质量门禁。

* **Step 3.1 (前端视图交互适配 - InboundsView.vue)**:
  - 修改 `web/src/views/InboundsView.vue`：
    - 更新 `SubRoute` 表单类型定义，增加 `allowedUsers?: string[]`。
    - 在分流线路列表项中增加用户授权选择器（支持多选已注册用户的邮箱，空选时文字提示“全员开放”）。
    - 确保保存与编辑入站时，`subRoutesJson` 中能正确序列化与反序列化 `allowedUsers` 字段。
  - 局部验证命令: `npm run build`
  - 预期判据: TypeScript 类型检查与 Vite 生产构建无报错。

* **Step 3.2 (前端 Mock 逻辑同步适配 - mock/index.ts)**:
  - 修改 `web/src/mock/index.ts` 中根据 token 获取订阅节点的处理逻辑：
    - 当 `sr.allowedUsers` 存在且非空时，检查当前用户的 email 是否在 `sr.allowedUsers` 中，若不在则跳过该线路。
  - 局部验证命令: `npm run build`
  - 预期判据: 前端构建通过，离线/开发环境与真实后端行为一致。

* **Step 3.3 (全链路端到端质量验证与冒烟测试)**:
  - 执行全局后端静态检查与竞态测试：`go vet ./...` && `go test -race ./...`
  - 执行前端打包与类型检查：`cd web && npm run build`
  - 局部验证命令: `go vet ./... && go test -race ./... && (cd web && npm run build)`
  - 预期判据: 前后端全量检查均无报错无警告。

---

## 3. 全局质量门禁核验 (Global Quality Gate)
* **代码风格与静态检查**: `go vet ./...` (通过，0 新增警告)
* **后端自动化测试 (含并发竞态检测)**: `go test -race ./...` (全部通过，0 失败)
* **前端生产构建与类型检查**: `cd web && npm run build` (编译输出正常，0 错误)
* **核验结果**: 待实施完成后核验全部通过。

---

## 4. 实施偏差记录 (Deviations Log)
* 严格按照 spec.md 既定架构范围实施，不引入额外数据库表字段或第三方依赖。

---

## 5. 阶段准出签批 (Gate 3 Sign-off)
- [ ] 所有分步实施项与验证断言均已就地执行并通过
- [ ] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [ ] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Pending
- **验证人 / 日期**: [待人类签批] / 2026-09-21
