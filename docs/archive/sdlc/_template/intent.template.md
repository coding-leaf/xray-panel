# Intent: [简明任务或缺陷标题]

- **任务编号**: TASK-XXX
- **提出人**: [姓名/角色]
- **创建时间**: [创建时间]
- **初始 Change Tier**: Tier 2 (Normal) / Tier 3 (High-risk)
- **当前状态**: Draft / In-Review / Accepted

---

## 1. 问题与现状背景 (Problem)
[客观描述当前存在的问题、业务痛点、或 Bug 发生的具体场景与表现]

## 2. 变更性质分类 (Change Archetype - 单选)
- [ ] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [ ] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
[清晰描述期望达到的具体业务成果、接口行为或量化成功指标]

## 4. 波及工程分面 (Affected Architectural Layers)
- [ ] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [ ] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**: [硬性技术制约]
* **明确非目标 (Non-Goals / Out-of-Scope)**: [明确非目标]
* **完成判定条件 (Definition of Done)**: [完成判定条件]

## 6. 未决疑问与待探讨点 (Open Questions)
- [未决疑问与待探讨点]

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / [YYYY-MM-DD]
