# Intent: 架构分层解耦与清晰度重构

- **任务编号**: arch-domain-decoupling
- **提出人**: Dev
- **创建时间**: 2026-09-20 12:25
- **初始 Change Tier**: Tier 2
- **当前状态**: In-Review

---

## 1. 问题与现状背景 (Problem)
项目按照 `domain`、`service`、`adapter`、`delivery` 进行了初步分层，但在实际演进中，部分模块存在职责混杂与不一致的现象：例如目录结构中并存了 `internal/delivery` 与 `internal/adapter` 的部分边界重叠、接口定义偏大或直接依赖具体结构体、部分业务编排渗透到了数据传输层、以及存在未使用或过时的历史代码碎片。需要对分层边界、包组织结构及接口抽象做清晰度收敛。

## 2. 变更性质分类 (Change Archetype - 单选)
- [x] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [ ] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. 明确并严守 `internal/domain` (纯粹领域模型与业务规则，保持无副作用与零外部基础设施依赖)；
2. 保持依赖单向流向（Delivery/Adapter -> Service -> Domain / Repository Interface）；
3. 遵循 Go 语言习惯（Caller-scoped interfaces），消除宽接口与不必要的冗余类型转换；
4. 清理残留废弃代码与不符合 KISS 原则的过度抽象，提升整体代码的可读性与工程维护性。

## 4. 波及工程分面 (Affected Architectural Layers)
- [x] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [ ] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)
- 注：主要为内部包结构整理与依赖解耦，不改动应用启动流程与公共协议。

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**: 
  - 严禁产生包循环依赖 (import cycle)；
  - 遵守 KISS 原则，严禁为了解耦而引入单一实现的无痛点工厂和过度包装层；
  - 保证所有既有测试保持全绿通过 (`go test -race ./...`)。
* **明确非目标 (Non-Goals / Out-of-Scope)**: 
  - 不修改任何对外 HTTP REST API 契约与路由路径；
  - 不修改数据库表结构与持久化 Schema；
  - 不做全局启动 Bootstrap 架构大爆炸重构。
* **完成判定条件 (Definition of Done)**:
  - 消除跨层违规依赖与冗余接口；
  - 结构清晰规范，`go vet ./...` 零告警；
  - `go test -race ./...` 100% 绿灯。

## 6. 未决疑问与待探讨点 (Open Questions)
- 需由 planner 进行深度只读扫描，精确定位具体的跨层调用点、过宽接口或死代码片段，输出清单。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-20 12:25
