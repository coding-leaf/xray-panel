# Intent: SubRoute 节点分流线路细粒度用户权限隔离

- **任务编号**: subroute-user-isolation
- **提出人**: Dev
- **创建时间**: 2026-09-21 00:42
- **初始 Change Tier**: Tier 3
- **当前状态**: In-Review

---

## 1. 问题与现状背景 (Problem)
- **现状背景**：当前系统入站节点（Inbound）支持配置多个分流线路（SubRoutes，基于 VLESS 16-bit routeId 单端口多出口机制）。然而，用户权限授权仅停留在 Inbound 节点级别（即只要用户具备该 Inbound 授权，即可订阅并访问该 Inbound 下的所有 SubRoute）。
- **具体业务痛点与安全风险**：
  1. **订阅展示层无隔离**：`sub_service.go` 与 `protocol.InboundsToNodeConfigs` 会无差别下发 Inbound 下的所有 SubRoute，无法针对特定用户（如付费 VIP、专属测试组）下发指定线路。
  2. **Xray 引擎层无强隔离（存在伪造越权风险）**：`compiler.go` 编译 Layer 3 接入网关规则时，生成的 `XrayRoutingRule` 仅限定了 `inboundTag` 与 `vlessRoute`，未设置 `user` 字段。任何拥有该 Inbound 凭证的普通用户，只要在其客户端本地手动指定或伪造 `routeId`，即可越权穿透至高价值或专有出站通道。

## 2. 变更性质分类 (Change Archetype - 单选)
- [ ] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [ ] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [x] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. **细粒度权限模型**：在 `domain.SubRoute` 中扩展 `AllowedUsers []string` 字段，存储获准访问该线路的用户邮箱列表；字段为空表示全员开放，保证存量配置无缝平滑兼容。
2. **订阅展示层过滤**：`sub_service.go` 与 `protocol.InboundsToNodeConfigs` 下发订阅节点时，依据用户邮箱匹配 `SubRoute.CanAccess`，仅下发该用户有权限的线路。
3. **Xray 路由引擎强隔离**：`compiler.go` 在生成 Layer 3 路由规则时，若 SubRoute 指定了 `AllowedUsers`，自动向 `XrayRoutingRule.User` 注入该白名单。未授权用户即使伪造 `routeId` 亦无法命中分流出口规则。
4. **前端配置交互**：在 Inbound 线路编辑抽屉提供直观的用户授权选择能力。
5. **平滑演进与高可靠**：无需数据库 DDL 变更，历史数据向前/向后兼容，所有单元测试与并发竞态检测 100% 通过。

## 4. 波及工程分面 (Affected Architectural Layers)
- [x] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [x] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [x] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**:
  - 遵循 Xray 官方 Routing Rule 规范：`user` 字段必须为用户 Email 字符串数组。
  - 向后兼容原则：存量数据中无 `allowedUsers` 字段或该字段为空的 SubRoute，必须保持全员开放行为不变。
  - 权限从属原则：SubRoute 权限从属于 Inbound 权限，用户必须先具备 Inbound 授权才能匹配其下的 SubRoute。
  - KISS 极简实现：在 `domain.SubRoute` 挂载纯判断方法 `CanAccess(email string) bool`，避免多层包装与过度设计。
* **明确非目标 (Non-Goals / Out-of-Scope)**:
  - 不引入独立的 SubRoute 权限管理数据库关联表，继续复用现有 `inbounds.sub_routes_json` JSON 结构存储。
  - 不修改 Xray 配置热应用核心流向与 `ConfigService` 的重编译调度流程。
  - 不改变除 VLESS routeId 之外的其他协议转发体系。
* **完成判定条件 (Definition of Done)**:
  - `go test -race ./...` 全量测试通过，覆盖 SubRoute 权限模型、订阅生成过滤及 Xray 路由编译测试。
  - `cd web && npm run build` 编译打包通过。
  - `python3 tooling/task_cli.py check subroute-user-isolation` 门禁校验全绿通过。

## 6. 未决疑问与待探讨点 (Open Questions)
- **极端场景处理决断（已确认）**：采纳选项 1。若某 Inbound 配置了 SubRoutes，但全部 SubRoutes 均排他排除了某用户，该用户订阅此 Inbound 时严格下发 0 个节点，绝不自动兜底回退为 routeId=0 的基础入站节点，遵循最小特权原则。
- **授权粒度决断（已确认）**：采纳方案 A（SubRoute 细粒度用户权限隔离）。SubRoute 权限从属于 Inbound 权限，用户必须先获 Inbound 授权，且满足 SubRoute.AllowedUsers 白名单（为空则全员放行）才可获取与通行。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-21 00:42
