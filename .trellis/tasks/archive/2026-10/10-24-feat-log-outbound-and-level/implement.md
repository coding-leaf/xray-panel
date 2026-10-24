# 新增出站日志过滤与日志等级全栈能力 - 实施计划 (Implement Plan)

## Ordered Implementation Checklist

### 1. 后端日志过滤与核心日志等级实现
- [ ] 1.1 扩展 `internal/domain/log.go` 中的 `LogFilter` 结构体，增加 `OutboundTag` 与 `Level` 字段。
- [ ] 1.2 在 `internal/adapter/xray/log_reader.go` 实现 `matchOutboundTagBytes` 与 `matchLevelBytes`，并在 `ReadLastLinesFiltered` 与 `ParseLogLineFast` 中集成。
- [ ] 1.3 在 `internal/adapter/xray/log_reader_test.go` 编写出站标签与错误日志等级的高效匹配单测。
- [ ] 1.4 在 `internal/delivery/http/handler_log.go` 接收 `outbound` 与 `level` 查询参数。
- [ ] 1.5 在 `internal/adapter/xray/compiler.go` 增加 `logLevel` 字段与 `SetLogLevel` / `GetLogLevel`，并在 `Compile` 中输出动态配置的 loglevel。
- [ ] 1.6 在 `internal/adapter/xray/compiler_test.go` 增加日志等级测试用例。
- [ ] 1.7 在 `internal/service/setting_service.go` 中集成 `xray_log_level` 配置与重编排管道，并在 `main.go` 启动时读取并注入。
- [ ] 1.8 在 `internal/service/setting_service_test.go` 增加相应测试用例。

### 2. 前端界面与交互扩展
- [ ] 2.1 在 `web/src/views/LogsView.vue` 访问模式增加“出站 (Outbound)”过滤下拉组件，支持动态识别出站标签与点击表格出站徽章快捷过滤；在错误模式下将 `level` 作为查询参数透传。
- [ ] 2.2 在 `web/src/mock/index.ts` 中增强 `/logs` 路由，支持 `outbound` 与 `level` 模拟过滤。
- [ ] 2.3 在 `web/src/views/SettingsView.vue` 的核心托管卡片中增加“Xray 运行日志等级”选择器并绑定保存。
- [ ] 2.4 在 `web/src/mock/storage.ts` 与 `web/src/mock/index.ts` 补充 `xray_log_level` 默认值与模拟保存。

### 3. 门禁验证与交付
- [ ] 3.1 运行后端单元测试 `mise x -- go test ./...`
- [ ] 3.2 运行后端静态分析 `mise x -- go vet ./...`
- [ ] 3.3 运行前端生产编译 `cd web && mise x -- npm run build`
