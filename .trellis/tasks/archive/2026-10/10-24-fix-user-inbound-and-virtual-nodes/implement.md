# 修复用户保存异常与虚拟节点转化规范 - 实施计划 (Implement Plan)

## Ordered Implementation Checklist

### 1. Fail-Repro 阶段（先编写失败测试）
- [ ] 1.1 在 `internal/service/user_service_test.go` 编写失败测试用例：
  - 模拟 `XrayManager.RemoveUser` 返回 `rpc error: code = Unknown desc = app/proxyman/command: failed to get handler: vless-reality > app/proxyman/inbound: handler not found: vless-reality`；
  - 调用 `UpdateUser`，验证当前代码会错误触发 `rollback()` 并返回失败；
  - 模拟用户禁用时从节点移除触发同样报错。
- [ ] 1.2 在 `internal/protocol/node_converter_test.go` 编写虚拟节点测试用例：
  - 测试非 VLESS 协议（如 trojan/vmess）配置 SubRoutes 时不被错误展开为多条无意义节点；
  - 测试空名称 SubRoute 具备确定性唯一的 Remark（`<base> - 线路 #<routeId>`）；
  - 测试 RouteID 过滤支持。

### 2. 后端核心实现与修复
- [ ] 2.1 在 `internal/service/user_service.go` 提炼 `isXrayIgnorableRemoveError(err error) bool`，替换各处 `RemoveUser` 错误判断（`UpdateUser` 旧节点移除、停用状态移除、补偿逻辑、`DeleteUser` 等）。
- [ ] 2.2 在 `internal/protocol/node_converter.go` 改造 `InboundsToNodeConfigs`：
  - 限制 SubRoute 展开仅对 `strings.EqualFold(inbound.Protocol, "vless")` 有效；
  - 规范空名称线路 fallback 命名；
  - 支持 `routeIDFilter uint16` 参数（或统一重载方法）。
- [ ] 2.3 在 `internal/service/sub_service.go` 中：
  - 统一 `GetUserShareInfo` 与 `resolveSubscriptionNodes` 的命名逻辑与 route 参数传递；
  - `SingleSub` 针对 SubRoute 附带 `route=<routeId>`。
  - HTTP `sub_handler` 支持解析 `route` 查询参数。

### 3. 前端修复与优化
- [ ] 3.1 在 `web/src/views/users/services/subscription.ts` 中修正 `getNodeTags`：
  - 当 `filtered.length === 0` 时，禁止全选 `availableInbounds`，安全返回原 tags 或空。
- [ ] 3.2 在 `web/src/views/users/components/UserTable.vue` 中优化 `getSubRoutesCount`：
  - 增加对用户邮箱授权的过滤检查。

### 4. 门禁验证与审查
- [ ] 4.1 运行后端单元测试 `mise x -- go test ./...`
- [ ] 4.2 运行后端静态分析 `mise x -- go vet ./...`
- [ ] 4.3 运行前端构建 `cd web && mise x -- npm run build`
