# Error Handling Guidelines

> 显式错误传播与 HTTP 交付契约

---

## 核心原则

1. **绝对禁止静默吞错**：
   - 严禁用 `_ = ...` 忽略可能失败的 I/O、数据库操作或 gRPC 调用；
   - 无法处理的错误必须显式返回上层，并包裹上下文。

2. **错误上下文包裹 (`%w`)**：
   使用 `%w` 包装底层错误，保持 `errors.Is` / `errors.As` 链条可用：
   ```go
   if err != nil {
       return nil, fmt.Errorf("open sqlite db failed: %w", err)
   }
   ```
   参考 `internal/adapter/repository/sqlite_db.go:32`。

3. **HTTP 交付层错误映射**：
   - 内部错误不可原样无过滤抛给前端；
   - 以业务可理解的 HTTP Status Code 返回，并统一使用 **`error` 字段**的错误体：
     ```go
     c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
     ```
     参考 `internal/delivery/http/handler_auth.go:41`、`handler_inbound.go:43`。
   - 成功且无数据返回时使用 `message` 字段：
     ```go
     c.JSON(http.StatusOK, gin.H{"message": "inbound deleted"})
     ```
     参考 `internal/delivery/http/handler_inbound.go:101`。
   - 认证失败可附带额外标记字段（如 `require2fa`）——
     `internal/delivery/http/handler_auth.go:38`。

---

## 4. 跨系统三态一致性与 Saga 反向补偿规范 (Saga Compensating Pattern)

系统维护三份异构存储状态：
- **SQLite DB (GORM)**：业务持久化真相源；
- **Xray-core (gRPC)**：进程间运行态内存；
- **config.json (文件系统)**：守护进程冷启动载荷。

### 核心约束：
1. **禁止单纯 return err 制造孤儿数据**：
   - 在 `CreateUser` 等资源创建路径中，若 DB 写入成功，后续 Xray 内存注入或写盘失败时，**严禁仅返回错误**，必须逆序执行 `RemoveUser` 尽力清理已注入的节点，并调用 `userRepo.Delete` 补偿清除已插入的 DB 记录，确保调用方用相同唯一键（Email）重试不会遭遇主键/唯一冲突。
2. **更新操作基于状态快照补偿**：
   - 在变更复杂状态前必须保存前置内存快照（Snapshot）。后续外部服务抛错时，需将已持久化的 DB 记录和 Xray 节点回滚至快照值。
3. **批量操作错误聚合**：
   - 严禁在批量循环中因单项失败直接静默 `continue`；
   - 必须维护错误收集切片，最终通过 `errors.Join(errList...)` 聚合返回，以便交付层感知部分失败明细。
4. **补偿动作的观测性防线**：
   - 逆向补偿步骤本身如果遇到异常（如网络中断），严禁使用 `_ =` 静默吞掉，必须通过结构化日志（`slog.Warn` / `slog.Error`）记录关联实体 ID、节点标签与底层错误，供运维溯源审计。


- **不存在全局错误码体系**：代码中**没有** `{"code": 40001, "message": "..."}` 这类统一错误码
  封装，也没有 `code` 常量表。错误契约就是「HTTP 状态码 + `{"error": "..."}`」。
- **唯一的 `{code, msg, data}` 信封是 Reality 专用**：
  `InboundHandler` 的 Reality 状态/检查接口返回
  `{"code": 0, "msg": "success", "data": {...}}`
  （`internal/delivery/http/handler_inbound.go:114-158`，
  由 `handler_reality_test.go:131-179` 锁定）。
  这是**特例**，新增接口不要沿用该信封，除非修改的是 Reality 相关接口。
