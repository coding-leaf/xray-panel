# 用户CRUD三态一致性与Saga失败补偿实施计划

## 实施阶段与清单

### Step 1: 故障注入测试用例先行 (Red Test)
- [ ] 在 `internal/service/user_service_test.go` 中定义 `mockXrayManager`，支持记录调用轨迹和注入指定错误。
- [ ] 编写 `TestUserService_CreateUser_XrayFailure_Rollback`：测试当 `AddUser` 失败时，返回错误且断言用户未残留在 DB 中。
- [ ] 编写 `TestUserService_CreateUser_ConfigFileFailure_Rollback`：测试当 `SyncUserToFile` 失败时，返回错误且断言 Xray `RemoveUser` 已被补偿调用、DB 已被回滚。
- [ ] 运行测试确认失败（红灯）。

### Step 2: UserService 核心用例改造 (Green Implementation)
- [ ] 改造 `CreateUser`：实现 `addedTags` 累加记录、失败时逆向 `RemoveUser` 以及回滚 `userRepo.Delete(user.ID)`。
- [ ] 改造 `UpdateUser`：实现基于快照的逆向回滚与错误传播。
- [ ] 改造 `DeleteUser`：重构删除步骤顺序，完善错误处理。
- [ ] 改造 `ResetTraffic` 与 `ResetSubToken`：消除 `_ =`，接入逆向补偿。
- [ ] 改造 `BatchRenew`、`BatchResetTraffic`、`BatchSetStatus`：收集失败项并返回聚合错误。

### Step 3: 全套门禁与回归验证 (Refactor & Verify)
- [ ] 运行全套后端测试：`mise x -- go test -v ./internal/service/...`
- [ ] 运行全库单元测试与静态分析：`mise x -- go test ./...` 与 `mise x -- go vet ./...`
- [ ] 检查代码整洁度，杜绝任何新增的静态警告。
