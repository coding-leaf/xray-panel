# 用户CRUD三态一致性与Saga失败补偿重构 PRD

## Goal

在 `UserService` 的核心用例（Create, Update, Delete, ResetTraffic, ResetSubToken）中消除对 Xray gRPC 与物理配置同步错误的静默忽略（`_ =`），引入基于应用层 Saga 的反向补偿机制与明确错误传播，保证 SQLite、Xray 运行时内存与 `config.json` 三态一致性，并补充完整的故障注入单元测试。

## Requirements

1. **消除静默吞错**：
   - `UserService.CreateUser`、`UpdateUser`、`DeleteUser`、`ResetTraffic`、`ResetSubToken` 中，严禁忽略 `xrayManager` 与 `configSvc` 的错误返回值。
   - 任何环节出错必须向上层调用者（HTTP Handler / Bot Handler）暴露带有明确上下文的错误。

2. **Saga 反向补偿机制（一滚全滚）**：
   - **CreateUser**：若 Xray 注入失败或写文件失败，必须反向补偿：撤销已成功加入的 Xray 节点，并从 SQLite 中删除刚创建的用户记录，确保不会残留孤儿数据，且用户后续可使用相同 Email 重试创建。
   - **UpdateUser**：在修改前记录旧数据快照（Snapshot）。若后续 Xray 刷新失败或写盘失败，必须反向补偿：将 Xray 节点状态与 SQLite 数据精确回滚至修改前快照。
   - **ResetSubToken**：若更新 Xray 节点凭证或写盘失败，必须反向补偿：恢复原 UUID 与 SubToken，防止凭证半更新导致客户端断联且无法恢复。
   - **DeleteUser**：保证删除流程的逆向防护，遇到非幂等致命错误时阻止破坏性删除并报错。

3. **批量操作的错误聚合**：
   - `BatchRenew`、`BatchResetTraffic`、`BatchSetStatus` 不能吞掉单项错误，应聚合失败的 ID 列表与错误信息并返回，便于管理后台向用户报告部分成功/部分失败。

4. **故障注入测试覆盖**：
   - 为 `UserService` 编写 Mock XrayUserManager 和故障注入测试用例。
   - 显式验证当 Xray 返回超时、网络拒绝或内部错误时，数据库是否被干净回滚。

## Acceptance Criteria

- [ ] `UserService` 内不存在针对 `xrayManager` 和 `configSvc` 的 `_ =` 静默吞错。
- [ ] 针对 `CreateUser` 的故障注入测试：模拟 Xray 注入失败，断言返回 error 且 DB 中该用户已被删除。
- [ ] 针对 `CreateUser` 的故障注入测试：模拟配置文件落盘失败，断言返回 error，Xray 内存被调用 `RemoveUser` 清理，DB 被回滚。
- [ ] 针对 `UpdateUser` 的故障注入测试：模拟 Xray 失败，断言 DB 中的用户信息完全恢复为修改前值。
- [ ] 针对 `ResetSubToken` 的故障注入测试：断言失败后原 UUID 与 Token 未被破坏。
- [ ] 所有新旧单元测试在终端通过：`mise x -- go test ./...` 绿灯，`go vet ./...` 无警告。
