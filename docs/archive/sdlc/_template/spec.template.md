# Spec: [技术方案与契约设计]

- **关联 Intent**: TASK-XXX
- **主导设计人**: [全栈开发工程师 / 架构师]
- **当前状态**: Draft / In-Review / Approved

---

## 1. 架构流向与设计方案
[文字或 Mermaid 流程图描述数据流、模块调用链与状态机流转]

```mermaid
flowchart LR
    Client --> API
    API --> Logic
    Logic --> Storage
```

## 2. API 与数据契约设计
* **接口路由**: `POST /api/v1/...`
* **入参 Schema (JSON/Struct/DTO)**:
* **出参 Schema**:
* **异常与错误码定义**:

## 3. 可测性设计 (Design for Testability)
* **独立纯函数计算核**: [列出解耦出来用于白盒测试的纯算法函数]
* **外部依赖与 Mock 策略**: [针对外部依赖、存储或网络交互的桩函数/Mock 方案]

## 4. 替代方案与权衡考量 (Alternatives Considered & Trade-offs)
* **评估过的替代方案**: [评估过的替代方案]
* **未采纳原因与权衡分析**: [未采纳原因与权衡分析]

## 5. 动态风险核验与回滚预案 (Risk & Rollback Verification)
* [ ] 已检查 7 大风险维度 (Files, API, Schema, Auth, Deps, Migration, Blast Radius)
* [ ] 确认当前 Change Tier 评级准确（若发现隐藏高风险，已升级评级）
* **回滚与故障应急策略**: [回滚与故障应急策略]

---

## 6. 阶段准出签批 (Gate 2 Sign-off)
- [ ] 架构流向与 API 契约已冻结
- [ ] 替代方案已完成推演与权衡
- [ ] 7 维风险已核验且具备明确回滚预案
- **审查结论**: Pending
- **签批人 / 日期**: [待人类签批] / [YYYY-MM-DD]
