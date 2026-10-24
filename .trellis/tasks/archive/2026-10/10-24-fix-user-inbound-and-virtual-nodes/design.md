# 修复用户保存异常与虚拟节点转化规范 - 技术设计 (Design)

## 1. 架构边界与受影响模块

- **服务编排层 (`internal/service/user_service.go`)**：
  - 改造点：在执行 Xray 核心内存同步阶段，重构 `RemoveUser` 错误处置，引入统一的 `isXrayIgnorableRemoveError(err error) bool` 判定。
- **订阅与协议层 (`internal/protocol/node_converter.go`, `internal/service/sub_service.go`)**：
  - 改造点：
    1. 限制 SubRoute 展开仅对 `strings.EqualFold(inbound.Protocol, "vless")` 有效；
    2. 统一虚拟节点 Remark 生成格式；
    3. `InboundsToNodeConfigs` 增加 `routeFilter uint16`（或在 `sub_service.go` 中解析 `route` 参数并精确过滤）；
    4. `SingleSub` 附带 `route` 参数。
- **前端辅助层 (`web/src/views/users/services/subscription.ts`, `UserTable.vue`)**：
  - 改造点：修正 `getNodeTags` 兜底逻辑；修正 `UserTable.vue` 中的 `getSubRoutesCount` 过滤判定。

## 2. 核心错误判定规范 (Error Toleration Contract)

```go
func isXrayIgnorableRemoveError(err error) bool {
    if err == nil {
        return false
    }
    msg := strings.ToLower(err.Error())
    return strings.Contains(msg, "handler not found") ||
           strings.Contains(msg, "failed to get handler") ||
           strings.Contains(msg, "not found") ||
           strings.Contains(msg, "not exist")
}
```

在调用 `xrayManager.RemoveUser(ctx, oldTag, email)` 时：
```go
if err := s.xrayManager.RemoveUser(ctx, oldT, updatedUser.Email); err != nil {
    if !isXrayIgnorableRemoveError(err) {
        rollback()
        return nil, fmt.Errorf("remove user from old xray inbound %s failed: %w", oldT, err)
    }
    slog.InfoContext(ctx, "ignored harmless error removing user from absent or clean inbound",
        slog.String("inbound", oldT), slog.String("email", updatedUser.Email), slog.Any("error", err))
}
```

## 3. 虚拟节点转换契约 (Virtual Node Conversion Contract)

1. **协议约束**：
   ```go
   isVless := inbound.Protocol == "" || strings.EqualFold(inbound.Protocol, "vless")
   subRoutes := in.GetSubRoutes()
   if len(subRoutes) == 0 || !isVless {
       // 直接按基础入站产出 1 个节点
   } else {
       // 按各 SubRoute 展开为虚拟节点
   }
   ```
2. **确定性命名**：
   ```go
   baseName := in.Remark
   if baseName == "" {
       baseName = in.Tag
   }
   if sr.Name != "" {
       tempInbound.Remark = sr.Name
   } else {
       tempInbound.Remark = fmt.Sprintf("%s - 线路 #%d", baseName, sr.RouteID)
   }
   ```
3. **RouteID 单独过滤**：
   允许请求指定 `tag` 与 `route`（例如 `/sub/<token>?tag=vless-reality&route=1`），在下发时仅返回对应匹配的虚拟节点。

## 4. 向后兼容性与回滚策略
- 纯逻辑优化，无数据库 Schema / DDL 变更。
- 存量配置中的 SubRoutes 行为保持向下兼容。
- 随时可代码回退，不污染底层持久化状态。
