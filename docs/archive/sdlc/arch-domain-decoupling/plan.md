# Plan: 架构分层解耦与清晰度重构 - 实施计划

- **关联 Spec**: arch-domain-decoupling
- **实施执行人 / Agent**: Dev
- **当前状态**: Draft
- **Change Tier**: Tier 2 (架构分层清晰化与依赖倒置重构)

---

## 1. 变更文件清单 (Files that change)

### 核心解耦与服务实现文件
* `internal/service/config_service.go` (Modify: 引入 Caller-scoped `ConfigStorage` 与 `ConfigCompiler` 接口，解除对 `*xray.ConfigManager` / `*xray.XrayCompiler` 强依赖)
* `internal/service/geodata_service.go` (Modify: 引入 Caller-scoped `CoreController` 接口，解除对 `*xray.Manager` 强依赖)
* `internal/service/alert_service.go` (Modify: 引入 `CertificateInspector` 接口，解除对 `*xray.ConfigManager` 及 `xray.ParseCertFile` 强依赖)
* `internal/service/log_service.go` (Modify: 引入 `LogReader` 接口，日志 DTO 条目解耦为通用领域结构)
* `internal/protocol/node_converter.go` (New: 将 `InboundToNodeConfig`、`InboundsToNodeConfigs`、`BuildShareLink` 从 `adapter/xray` 迁移至 `protocol`)
* `internal/service/sub_service.go` (Modify: 切换节点转换调用至 `protocol` 包，切断对 `adapter/xray` 的 import)
* `internal/service/auth_service.go` (New: 提炼认证与 2FA/TOTP 业务用例编排服务)
* `internal/service/setting_service.go` (New: 提炼系统配置管理与多适配器热重载编排服务)
* `internal/delivery/http/handler_auth.go` (Modify: 瘦身为纯 HTTP 传输层，委托给 `AuthService`)
* `internal/delivery/http/handler_setting.go` (Modify: 瘦身为纯 HTTP 传输层，委托给 `SettingService`)
* `main.go` (Modify: 统一实例化 `AuthService` 与 `SettingService`，装配解耦后的接口注入)

### 死代码清理与文件瘦身
* `internal/adapter/xray/grpc_client.go` (Modify: 移除废弃空转别名 `buildAccountMessage`)
* `internal/adapter/xray/config_parser.go` (Modify: 移除迁移后的节点分享函数与无引用函数 `BuildShareLinksForInbound`)
* `internal/adapter/xray/log_reader.go` (Modify: 移除无生产代码引用的遗留函数 `ReadLastLines`)

### 测试与验证文件
* `internal/service/auth_service_test.go` (New: 针对密码比对、TOTP 校验与 JWT 签发的单元测试)
* `internal/service/setting_service_test.go` (New: 针对配置读取与联动热更新 Mock 测试)
* `internal/service/config_service_test.go` (Modify/New: 基于 Mock 验证解耦后契约)
* `internal/protocol/node_converter_test.go` (New: 承接节点转换与链接生成的完整单测)
* `internal/adapter/xray/sub_route_test.go` (Modify: 适配迁移后的节点转换函数路径)
* `internal/adapter/xray/share_link_test.go` (Modify: 适配迁移后的节点转换函数路径)
* `internal/adapter/xray/log_reader_test.go` (Modify: 移除废弃 `ReadLastLines` 单测，强化 `ReadLastLinesFiltered` 验证)

---

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)

### Milestone 1: 服务层 Caller-scoped 接口定义与依赖解耦 (M1)
> 目标：解除 `service` 对 `adapter` 具体实现的直接反向绑定，落实依赖倒置原则。

* **Step 1.1 (ConfigService 接口解耦)**:
  - 在 `internal/service/config_service.go` 定义 Caller-scoped 接口 `ConfigStorage` (包含 `ReadRawConfig`, `WriteConfig`, `ValidateConfig`) 与 `ConfigCompiler` (包含 `CompileToJSON`)；
  - 重构 `ConfigService` 结构体字段与构造函数 `NewConfigService`，依赖接口而非 `*xray.ConfigManager` / `*xray.XrayCompiler`；
  - 局部验证命令: `go test -race -v -run TestConfigService ./internal/service`

* **Step 1.2 (GeoDataService 接口解耦)**:
  - 在 `internal/service/geodata_service.go` 定义 Caller-scoped 接口 `CoreController` (包含 `GetVersion`, `RestartService`)；
  - 将 `*xray.Manager` 替换为 `CoreController`；
  - 局部验证命令: `go test -race -v -run TestGeoDataService ./internal/service`

* **Step 1.3 (AlertService 接口解耦)**:
  - 在 `internal/service/alert_service.go` 提取证书巡检调用方接口 `CertificateInspector` (或 `CertificatePathProvider`)，避免直接依赖 `*xray.ConfigManager` 及底层包；
  - 局部验证命令: `go test -race -v -run TestAlertService ./internal/service`

* **Step 1.4 (LogService 契约解耦与 DTO 领域收敛)**:
  - 在 `internal/service/log_service.go` 定义 `LogReader` 接口 (`GetLogPaths`, `ReadLastLinesFiltered`)；
  - 收敛 `LogResponse` 中的条目类型，移除对 `xray` 具体类型的直接暴露；
  - 局部验证命令: `go test -race -v -run TestLogService ./internal/service`

* **Step 1.5 (订阅与节点视图转换逻辑迁移)**:
  - 将 `InboundToNodeConfig`、`InboundsToNodeConfigs`、`BuildShareLink` 迁移至 `internal/protocol/node_converter.go`；
  - `internal/service/sub_service.go` 全面切换为调用 `protocol` 包，彻底移除 `import "panel/internal/adapter/xray"`；
  - 同步调整 `sub_route_test.go` 和 `share_link_test.go` 导入路径；
  - 局部验证命令: `go test -race -v ./internal/protocol ./internal/service`

---

### Milestone 2: 提炼轻量级 AuthService 与 SettingService (M2)
> 目标：消除 HTTP Handler 中散落的业务逻辑、密码加盐比对、TOTP 校验及多组件联动，使 Handler 蜕变为纯传输层。

* **Step 2.1 (AuthService 提炼与 HandlerAuth 瘦身)**:
  - 新建 `internal/service/auth_service.go`，封装 `Login`, `ChangePassword`, `Setup2FA`, `Confirm2FA`, `Disable2FA`, `Get2FAStatus` 等完整认证编排与审计打点；
  - 编写 `internal/service/auth_service_test.go`，覆盖口令校验、TOTP 防爆破与 JWT 签发；
  - 改造 `internal/delivery/http/handler_auth.go`，仅负责入参解析、调用 `AuthService` 与响应状态码映射；
  - 局部验证命令: `go test -race -v -run TestAuth ./internal/service ./internal/delivery/http`

* **Step 2.2 (SettingService 提炼与 HandlerSetting 瘦身)**:
  - 新建 `internal/service/setting_service.go`，定义组件联动重载接口 (`SettingReloader`, `SupervisorReloader`, `BotReloader`)；
  - 封装 `GetAllSettings` 与 `UpdateSettings`，在服务层统筹原子更新 DB 与多适配器热加载；
  - 编写 `internal/service/setting_service_test.go` 验证配置热更编排；
  - 改造 `internal/delivery/http/handler_setting.go`，纯化为透传 HTTP 请求至 `SettingService`；
  - 局部验证命令: `go test -race -v -run TestSetting ./internal/service ./internal/delivery/http`

* **Step 2.3 (Main 与 Router 组装集成)**:
  - 在 `main.go` 中实例化 `authSvc` 与 `settingSvc`，注入各适配器实现并挂载到 HTTP Handlers；
  - 局部验证命令: `go test -race -v ./internal/delivery/http`

---

### Milestone 3: 死代码清理与废弃孤立函数移除 (M3)
> 目标：清理冗余包装、废弃未引用函数，严格排查保持代码清爽。

* **Step 3.1 (移除 grpc_client.go 冗余别名)**:
  - 移除 `internal/adapter/xray/grpc_client.go` 中无意义的套壳别名 `buildAccountMessage`，统一内聚于 `BuildAccountMessage`；
  - 局部验证命令: `go test -race -v ./internal/adapter/xray`

* **Step 3.2 (移除 config_parser.go 遗留死代码)**:
  - 移除 `BuildShareLinksForInbound` 及其测试用例（生产链路已全面由统一节点管道替代）；
  - 局部验证命令: `go test -race -v ./internal/adapter/xray`

* **Step 3.3 (清理 log_reader.go 遗留单测函数)**:
  - 移除 `ReadLastLines`（仅存单元测试引用，生产代码已由高性能分段缓冲 `ReadLastLinesFiltered` 承载），同步清理对应遗留单测；
  - 局部验证命令: `go test -race -v ./internal/adapter/xray`

---

### Milestone 4: 全链路编译与回归验证 (M4)
> 目标：执行全维度工程静态分析、并发竞态测试与前后端二进制构建，确保系统零缺陷与零回归。

* **Step 4.1 (静态代码质量与风格检查)**:
  - 执行命令: `go vet ./...`
  - 预期判据: 零警告、零隐式类型错误。

* **Step 4.2 (全量自动化并发竞态回归测试)**:
  - 执行命令: `go test -race ./...`
  - 预期判据: 全部测试套件 100% PASS，无任何 race condition 告警，无协程泄漏。

* **Step 4.3 (后端完整二进制构建校验)**:
  - 执行命令: `go build .`
  - 预期判据: 成功生成二进制，零链接或符号错误。

* **Step 4.4 (前端完整生产构建校验)**:
  - 执行命令: `cd web && npm run build`
  - 预期判据: 前端构建完全绿灯，静态资源导出无异常。

---

## 3. 回滚保护与应急预案 (Rollback & Protection)
1. **源码级瞬时回滚**: 所有改动属于纯 Go 代码分层重构与依赖倒置，零数据库 Schema 变动，零外部 API 契约破损。出现任何意外可直接 `git revert` 一键恢复。
2. **渐进式接口适配**: 适配器既有方法签名保持不变，仅服务层定义消费端 interface，完全不影响外部系统交互行为。
3. **安全边界等价**: 密码哈希（bcrypt）与 TOTP 时间步长算法在 `AuthService` 中 1:1 无损平移，保证存量账号登录与凭证体系绝对稳定。

---

## 4. 全局质量门禁核验 (Global Quality Gate)
* **代码风格与静态检查**: `go vet ./...`
* **并发竞态与单元测试**: `go test -race ./...`
* **后端完整构建检查**: `go build .`
* **前端生产构建检查**: `cd web && npm run build`
* **核验预期**: 所有命令全部通过，零报错，零告警。

---

## 5. 实施偏差记录 (Deviations Log)
* [暂无偏差 / 严格遵循 spec.md 技术契约实施]

---

## 6. 阶段准出签批 (Gate 3 Sign-off)
- [ ] 所有分步实施项与验证断言均已就地执行并通过
- [ ] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [ ] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Pending
- **验证人 / 日期**: [待人类签批] / 2026-09-20 12:38
