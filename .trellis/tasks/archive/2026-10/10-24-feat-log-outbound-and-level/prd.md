# 新增出站日志过滤与日志等级全栈能力

## Goal
实现访问日志（Access Log）按出站出口标签（Outbound Tag）过滤，以及支持 Xray 核心运行日志等级（LogLevel）动态配置与错误日志按级别（Level）过滤的全栈能力。

## Requirements

### 1. 访问日志按出站出口标签过滤 (Outbound Tag Filter)
- **后端契约与解析**：
  - 在 `domain.LogFilter` 中增加 `OutboundTag string` 字段；
  - 在 `internal/adapter/xray/log_reader.go` 的反向读取扫描中，支持对形如 `[inbound -> outbound]` 或 `[inbound >> outbound]` 的出站标签进行高性能过滤（`matchOutboundTagBytes`）；
  - 在 `internal/delivery/http/handler_log.go` 的 `/logs` 接口中接收 `outbound` 查询参数并传入过滤条件。
- **前端交互**：
  - 在 `LogsView.vue` 访问模式（Access Mode）工具栏新增“出站 (Outbound)”下拉选择器；
  - 自动聚合或从可用出站列表中加载出站选项；
  - 切换出站下拉选择器时，触发日志请求与过滤；点击表格中的出站徽章可快捷设置为过滤条件；
  - Mock 环境（`web/src/mock/index.ts`）同步支持 `outbound` 过滤。

### 2. Xray 核心日志等级配置 (Xray LogLevel Config)
- **核心编译与配置**：
  - 在 `internal/adapter/xray/compiler.go` 中，使 Xray 配置的 `log.loglevel` 具备动态可配置能力（`SetLogLevel` / `GetLogLevel`），支持官方合法级别：`debug`、`info`、`warning`、`error`、`none`，默认 `warning`；
  - 在 `internal/service/setting_service.go` 中支持保存并读取 `xray_log_level`，当修改该设置项时，驱动重新编译并平滑重载 Xray 核心生效；在 `main.go` 启动时自动从 `settingRepo` 读取并预置到编译器；
- **前端配置界面**：
  - 在 `SettingsView.vue` 的“① Xray 核心解耦与 Systemd 托管环境”卡片中，新增“Xray 运行日志等级 (LogLevel)”下拉选择控件；
  - 选项涵盖 `warning`（推荐）、`info`、`debug`、`error`、`none`，附带清晰说明；
  - 保存系统设置时将 `xray_log_level` 提交后端。

### 3. 错误日志按日志级别过滤 (Error Log Level Filter)
- **后端支持**：
  - 在 `domain.LogFilter` 中增加 `Level string` 字段；
  - `log_reader.go` 在反向扫描读取错误日志（Error Log）时，若指定了 `Level`（如 `ERROR`, `WARNING`, `INFO`, `DEBUG`），支持在块扫描时直接按 `[Level]` 过滤匹配，大幅提升超大错误日志文件的查询性能；
  - `/logs?type=error` 接收 `level` 查询参数。
- **前端联动**：
  - `LogsView.vue` 中错误日志级别选择器除前端本地过滤外，将 `level` 作为查询参数带入后端请求；
  - Mock 环境同步支持 `level` 参数。

## Acceptance Criteria
- [ ] 编写测试用例覆盖 `matchOutboundTagBytes`：匹配 `[in -> out]`、`[in >> out]` 以及独立 tag；
- [ ] 编写测试用例覆盖 `matchLevelBytes` 及 `ErrorLog` 级别过滤；
- [ ] 编写测试用例验证 `XrayCompiler` 的 `log.loglevel` 编译与动态修改；
- [ ] 编写测试用例验证 `SettingService` 更新 `xray_log_level` 触发持久化与重载管道；
- [ ] 前端 `LogsView.vue` 访问日志支持出站过滤下拉与点击徽章联动过滤；
- [ ] 前端 `SettingsView.vue` 支持选择并保存 Xray 日志等级；
- [ ] `mise x -- go test ./...` 与 `mise x -- go vet ./...` 全绿；
- [ ] `cd web && mise x -- npm run build` 构建通过。
