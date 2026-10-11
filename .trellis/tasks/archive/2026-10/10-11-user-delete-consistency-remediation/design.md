# Technical Design: 用户删除三态一致性、Saga超时补偿与编译器重构

## 1. 架构拓扑与一致性状态机 (Consistency State Machine)

系统维护三份异构状态：
- **S1: 内存/gRPC 态** (`Xray-core` 运行时)
- **S2: 持久化真相源** (`SQLite` 数据库)
- **S3: 静态文件态** (`config.json` 磁盘载荷)

### 1.1 `DeleteUser` 状态机执行流重构

```
[开始 DeleteUser]
       │
       ▼
 0. 快照保存：oldSnapshot = *user
       │
       ▼
 1. Xray 运行态剔除 (gRPC)
    ├─ 遍历 tags：记录 removedTags = []string
    ├─ 任意节点失败？ ─── YES ──► 触发 rollbackXray()：遍历 removedTags 调用 AddUser
    │                             返回错误并阻断
    ▼ NO
 2. SQLite 持久化记录删除 (DB Delete)
    ├─ 数据库删除失败？ ── YES ──► 触发 rollbackXray()：恢复 Xray 内存
    │                             返回错误并阻断
    ▼ NO
 3. 配置文件静态编译与落盘 (SyncUserToFile)
    ├─ ConfigService 读取 DB (此时 DB 已无该用户)
    ├─ 编译出不含该用户的 config.json 并落盘
    ├─ 磁盘写入失败？ ─── YES ──► 触发完整 Saga 反向补偿：
    │                             ① DB 恢复：userRepo.Create(&oldSnapshot)
    │                             ② Xray 恢复：rollbackXray()
    │                             返回错误并阻断
    ▼ NO
 4. 运行时速率追踪清理：domain.RemoveUserRuntimeSpeed(email)
       │
       ▼
[删除成功完成]
```

### 1.2 独立限时超时补偿上下文契约 (Decoupled Compensating Context)
Go 1.21 引入了 `context.WithoutCancel(parentCtx)`。我们统一抽象辅助函数或上下文工厂：
```go
func createCompensatingContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
    if ctx == nil {
        ctx = context.Background()
    }
    return context.WithTimeout(context.WithoutCancel(ctx), timeout)
}
```
保证即使外部请求由于客户端掐断网络、HTTP 连接超时或微服务超时被 Cancel，执行 Saga 反向回滚动作的 gRPC、GORM 操作均能在独立的 15 秒生存期内平稳跑完。

---

## 2. 编译器 Clients 组装逻辑重构 (`compiler.go`)

### 2.1 缺陷根因分析
原代码逻辑：
```go
if clients != nil {
    rawClients, hasRaw := settingsMap["clients"].([]interface{})
    if hasRaw && len(rawClients) > 0 && len(filterActiveUsersForInbound(inb, users)) == 0 {
        // 保留原本存在于 settingsJson 的 clients
    } else {
        settingsMap["clients"] = clients
    }
}
```
**问题点**：当入站原本有 1 个面板用户，该用户被删除后，`filterActiveUsersForInbound(inb, users)` 为 0，而 `rawClients` 在之前落盘时已经包含了该用户（因为当时写入了 `SettingsJSON` 或 `config.json`）。此时代码误以为这是“需要保留的外部手工客户端”，从而未将其清空，导致用户死灰复燃。

### 2.2 重构方案
对于面板支持的协议适配器（`hasAdapter == true`）：
- 面板即是该入站用户认证的唯一权威管理者（Source of Truth）。
- `adapter.CompileClients(inb, users, accessor)` 负责根据传入的 `users` 列表生成该 Inbound 的合法 Clients。
- 若传入的 `users` 中该 Inbound 的活跃用户为 0，`CompileClients` 返回空切片 `[]XrayClient{}`。
- 此时 `settingsMap["clients"]` 必须直接赋值为该空切片，严禁保留已删除用户的历史残留。
- 仅当入站为非面板托管的自由透传入站（即没有已注册适配器，`hasAdapter == false`）时，才直接原样保留其 `SettingsJSON` 中的原始内容。

---

## 3. 回归测试架构 (Regression Test Architecture)

在 `internal/service/user_delete_consistency_test.go` 中建立高保真真实集成环境：
- **真实依赖**：
  - `gorm.Open(sqlite.Open(":memory:"))` 内存 SQLite 真实引擎；
  - `repository.NewUserRepository(db)`；
  - `repository.NewInboundRepository(db)`；
  - `repository.NewConfigSnapshotRepository(db)`；
  - `xray.NewXrayCompiler(8080)`；
  - `os.CreateTemp` 生成真实的临时 `config.json` 文件；
  - `xray.NewLocalConfigManager(tempConfigPath, "")` 真实读写文件；
  - `service.NewConfigService(...)` 驱动完整编译链路。
- **Mock 依赖**：
  - `MockSupervisor`：仅记录 Reload 调用，避免启动真实的 Xray 外部系统守护进程；
  - `MockXrayManager`：记录 AddUser / RemoveUser 调用并支持故障注入。

### 测试用例拓扑：
1. `TestUserDeleteConsistency_RestartResurrectionDefense`：端到端删除与重启防复活测试；
2. `TestUserDeleteConsistency_SagaCompensateOnDiskFailure`：写盘故障时完整 DB 与 Xray 回滚测试；
3. `TestUserDeleteConsistency_ContextCanceledResilience`：外部 Context 取消时独立 Context 补偿保障测试；
4. `TestUserDeleteConsistency_PartialNodeFailureRollback`：多节点删除部分失败时运行态回滚测试。
