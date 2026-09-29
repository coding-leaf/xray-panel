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

## 现状说明（避免误用）

- **不存在全局错误码体系**：代码中**没有** `{"code": 40001, "message": "..."}` 这类统一错误码
  封装，也没有 `code` 常量表。错误契约就是「HTTP 状态码 + `{"error": "..."}`」。
- **唯一的 `{code, msg, data}` 信封是 Reality 专用**：
  `InboundHandler` 的 Reality 状态/检查接口返回
  `{"code": 0, "msg": "success", "data": {...}}`
  （`internal/delivery/http/handler_inbound.go:114-158`，
  由 `handler_reality_test.go:131-179` 锁定）。
  这是**特例**，新增接口不要沿用该信封，除非修改的是 Reality 相关接口。
