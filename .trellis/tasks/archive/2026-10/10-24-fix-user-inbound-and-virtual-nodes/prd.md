# 修复用户保存异常与虚拟节点转化规范

## Goal
彻底消除在 Xray Inbound 已被移除或未就绪时保存用户报 `handler not found: vless-reality` 的阻断性 Bug；厘清并规范单端口多出口（SubRoute / 虚拟节点）的协议兼容边界、命名生成与订阅分发映射，避免虚拟节点混乱与越权歧义。

## Requirements

### 1. 消除 Inbound RemoveUser 容错缺陷 (Idempotent Deletion)
- 在 `UserService.UpdateUser`、`UserService.DeleteUser` 以及 `UserService.SetUserStatus` 中，当从旧 Inbound 或失效节点移除用户时：
  - 若 Xray gRPC 返回包含 `handler not found`、`failed to get handler`、`not found`、`not exist`，说明该入站已在核心中注销或该用户本就未注册于该入站。
  - 此类错误属于良性/幂等状态，必须视为非致命操作，记录 Warning 日志后平滑继续，严禁触发 Saga 回滚或向前端抛出 500/保存失败异常。
- 提炼统一且严谨的错误判定函数 `isXrayIgnorableRemoveError(err error) bool`，统一复用。

### 2. 前端授权节点解析防篡改修复
- 修复 `UserSubscriptionService.getNodeTags`：当用户绑定的 Inbound 标签因被删除等原因未匹配到任何当前可用 Inbound 时，禁止自动回退为“全选所有可用节点 (`availableInbounds.map(...)`)”，杜绝前端表单自动给用户绑定全部节点甚至引发失效 Inbound 移除异常。

### 3. 规范单端口多出口 (SubRoute / 虚拟节点) 转换契约
- **协议兼容隔离**：SubRoute（虚拟节点）基于 VLESS UUID RouteID 编码，仅适用于 `vless` 协议。对于 VMess、Trojan、Shadowsocks 等不支持 RouteID 编码的协议，禁止错误展开为相同配置的假虚拟节点。
- **节点命名确定性与去重**：
  - 当 SubRoute 拥有名称（如 `🇯🇵 日本原生直连`）时，以此作为节点 Remark；
  - 当 SubRoute 未命名时，采用标准兜底格式 `<Inbound名称> - 线路 #<RouteID>`，杜绝多条线路生成完全相同的同名节点。
- **独立单节点订阅能力**：
  - `sub_service.go` 与 `node_converter.go` 支持按 `route`（RouteID）过滤，使前端 `NodeShareInfo.SingleSub` 能够精确指向单条分流线路（`?tag=<inboundTag>&route=<routeId>`），而非将整组入站混杂下发。
- **用户表格线路计数真实性**：
  - `UserTable.vue` 中的 `getSubRoutesCount(tag, user)` 需结合 `CanAccess(user.Email)` 进行权限过滤，仅向管理员展示该用户真实有权访问的线路数量。

## Acceptance Criteria
- [ ] 编写失败测试先行复现：模拟 Xray 返回 `handler not found`，验证 `UserService.UpdateUser` 与 `DeleteUser` 不触发回滚并成功持久化。
- [ ] 编写测试覆盖 `isXrayIgnorableRemoveError` 涵盖各类 gRPC 包装错误消息。
- [ ] 编写测试验证 SubRoute 虚拟节点转换：
  - VLESS 协议正确展开虚拟节点与 UUID RouteID；
  - 非 VLESS 协议不展开无效虚拟节点；
  - 未命名 SubRoute 具备确定性唯一的 Remark；
  - 支持 `route` 参数精确过滤单条虚拟节点。
- [ ] 前端 `getNodeTags` 不再发生空匹配时全选所有可用节点的回退。
- [ ] `mise x -- go test ./...` 与 `mise x -- go vet ./...` 全绿。
- [ ] `cd web && mise x -- npm run build` 构建通过。
