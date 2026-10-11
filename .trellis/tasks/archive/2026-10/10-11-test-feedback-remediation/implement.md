# 测试缺陷与质量门禁全面治理 实施计划

## 实施阶段与步骤

### Stage 1: 面向接口解耦与故障注入测试先行 (Red Phase)
- [ ] 在 `internal/service/user_service.go` 中抽象定义 `ConfigFileSynchronizer` 接口，替换对 `*ConfigService` 的强指针依赖。
- [ ] 在 `internal/service/user_service_test.go` 中构建支持故障注入的 `mockXrayManager` 与 `mockConfigSynchronizer`。
- [ ] 编写针对 `CreateUser` 的故障注入测试：模拟 Xray 注入失败，断言返回 error 且 DB 中的用户被干净回滚删除。
- [ ] 编写针对 `CreateUser` 的故障注入测试：模拟配置文件持久化失败，断言返回 error，Xray 内存被调用 `RemoveUser` 补偿，DB 记录被回滚。
- [ ] 编写针对 `UpdateUser` 的故障注入测试：模拟 Xray 失败或写盘失败，断言返回 error 且 DB 中的用户被完全恢复为修改前快照。
- [ ] 编写针对 `ResetSubToken` 的故障注入测试：模拟失败，断言原 UUID 与 SubToken 未被破坏。
- [ ] 运行测试确认测试能够执行并正确捕获异常。

### Stage 2: UserService 核心用例 Saga 补偿重构 (Green Phase)
- [ ] 重构 `UserService.CreateUser`：实现 `addedTags` 跟踪与逆向补偿删除，消除 `_ =`。
- [ ] 重构 `UserService.UpdateUser`：基于快照实现逆向回滚与错误传播，消除 `_ =`。
- [ ] 重构 `UserService.ResetSubToken`：实现凭证原子回滚与状态恢复，消除 `_ =`。
- [ ] 重构 `UserService.ResetTraffic`：增加对 Xray 与文件写盘的错误检查与防御，消除 `_ =`。
- [ ] 重构 `UserService.DeleteUser`：重构步骤顺序（Xray 剔除 ➔ 文件写盘 ➔ DB 删除），消除 `_ =`。
- [ ] 重构批量操作 `BatchRenew`、`BatchResetTraffic`、`BatchSetStatus`：聚合收集错误，消除静默 `continue` 与吞错。
- [ ] 运行故障注入测试与现有单元测试，确保全绿：`mise x -- go test ./internal/service/...`。

### Stage 3: 服务启动阶段错误治理与告警
- [ ] 改造 `main.go` 启动逻辑，捕获 `SyncFromFile` 与 `RecompileAndApply` 错误，加入 `slog.Error` 结构化日志告警，消除 `_ =`。

### Stage 4: CI 质量门禁加固与版本事实源统一
- [ ] 修改 `.github/workflows/ci.yml`，在前端构建前加入 `npm run typecheck`。
- [ ] 修改 `.github/workflows/ci.yml`，在 `test` job 中下载 Xray-core 二进制，激活真实的集成测试。
- [ ] 修改 `.github/workflows/ci.yml`，将硬编码版本号更新为 `v2.6.2-ci`。

### Stage 5: 全量质量门禁验证与 Spec 更新
- [ ] 运行全套后端测试：`mise x -- go test ./...`
- [ ] 运行静态代码分析：`mise x -- go vet ./...`
- [ ] 运行前端类型检查：`mise x -- npm --prefix web run typecheck`
- [ ] 运行前端构建：`mise x -- npm --prefix web run build`
- [ ] 运行完整后端编译：`mise x -- go build .`
