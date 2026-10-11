# Implementation Plan: 用户删除一致性与Saga独立超时补偿治理

## Ordered Execution Checklist

### Phase 1: 编译器与核心适配层修正 (P0)
- [ ] 1.1 修改 `internal/adapter/xray/compiler.go`，修正活跃用户为空时误保留 `settingsJson` 中旧 `clients` 的逻辑。
- [ ] 1.2 运行并补充 `internal/adapter/xray/compiler_test.go` 针对用户清零后 `clients` 彻底清空（`len(clients) == 0`）的单测。

### Phase 2: UserService 独立超时 Context 与 DeleteUser 状态机重构 (P0 & P1)
- [ ] 2.1 引入 `createCompensatingContext` 独立限时超时上下文（基于 `context.WithoutCancel`）。
- [ ] 2.2 全面改造 `UserService.CreateUser`、`UpdateUser`、`ResetSubToken` 补偿调用使用独立上下文。
- [ ] 2.3 重构 `UserService.DeleteUser`：
  - 调整执行顺序：Xray gRPC 移除 ➔ SQLite 删除 ➔ 物理文件同步 `SyncUserToFile` ➔ 运行时追踪清理；
  - 引入多节点移除部分失败的 `rollbackXray()` 反向恢复；
  - 引入写盘失败时的数据库记录重新恢复（`userRepo.Create(&snapshot)`）与 Xray 恢复补偿。
- [ ] 2.4 更新 `internal/service/user_service_test.go` 中原有的单测以适配新的删除与补偿流程。

### Phase 3: 编写端到端真实组件回归测试 (P0 核心回归)
- [ ] 3.1 创建 `internal/service/user_delete_consistency_test.go`；
- [ ] 3.2 编写 `TestUserDeleteConsistency_RestartResurrectionDefense`（真实 SQLite + 真实 ConfigService + 临时 config.json 验证重启不复活）；
- [ ] 3.3 编写 `TestUserDeleteConsistency_SagaCompensateOnDiskFailure`（写盘故障反向恢复）；
- [ ] 3.4 编写 `TestUserDeleteConsistency_ContextCanceledResilience`（Context 取消下的独立补偿）；
- [ ] 3.5 编写 `TestUserDeleteConsistency_PartialNodeFailureRollback`（多节点部分失败恢复）。

### Phase 4: CI 与容灾加固 (P1 & P2)
- [ ] 4.1 更新 `main.go` 启动时配置同步与应用的错误降级提示与恢复记录；
- [ ] 4.2 更新 `.github/workflows/ci.yml` 中的 Xray-core 下载版本至生产稳定版本；
- [ ] 4.3 运行全量质量门禁：`go test ./...`、`go vet ./...`、`go build .`、`npm run typecheck`。

---

## Validation Commands
```bash
# 后端回归测试与静态分析
mise x -- go test -v -run TestUserDeleteConsistency ./internal/service
mise x -- go test ./...
mise x -- go vet ./...
mise x -- go build .

# 前端类型检查
mise x -- npm --prefix web run typecheck
```
