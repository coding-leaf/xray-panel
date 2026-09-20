---
name: sdlc-workflow
description: 引导 AI-Native SDLC 各阶段工件（intent.md / spec.md / plan.md）流转、7 维风险动态扫描与停止/升级判断。在创建新任务、设计技术契约、制定实施方案或任务升降级时激活。
---

# SDLC Workflow Orchestration (工件流转与门禁编排)

本技能定义项目中任务从提出到落地的完整工件推进 SOP。

## 1. 任务工件目录约定
所有任务工件集中归档于：`docs/sdlc/<task-id>/`
- `<task-id>` 统一采用**小写中划线格式**（如 `traffic-limit-alert`, `node-health-check`, `auth-session-guard`），便于与 Git 分支命名无缝衔接。

## 2. 7 维动态风险扫描与分级决策
在制定任何方案或计划时，主动检查 7 大维度以**发现潜在风险**：
1. **Affected Files**：改动文件数量与模块跨度。
2. **Public API**：是否涉及对外接口契约（入参、出参、错误码）变动。
3. **Data Schema**：是否触及数据库表结构、配置 Schema 或持久化状态迁移。
4. **Auth & Security**：是否影响租户权限、面板鉴权逻辑与敏感密钥。
5. **Dependencies**：是否需要引入新第三方依赖。
6. **Rollback Difficulty**：状态变更是否不可逆、回滚成本是否高昂。
7. **Blast Radius**：故障影响面是局部单点还是全局核心服务与进程。

**分级准则**：
- **Tier 1 (Trivial)**：明确低风险、局部、可逆、无公共契约/数据/安全影响（无需前置工件，直接建立复现证据与实现）。
- **Tier 2 (Normal)**：普通单模块特性或局部优化（执行完整的 intent → spec → plan 工件流转）。
- **Tier 3 (High-risk)**：高危变更，强制要求技术方案评审与人类签批。

### 【高危变更强制触发器 (Universal Tier 3 Escalation Triggers)】
**只要命中以下任意 1 项，严禁评定为 Tier 2，必须强制升级为 Tier 3**：
1. **跨领域流向重排**：变更同时跨越 >= 2 个独立顶级子系统/业务模块；
2. **应用全局生命周期**：变更触及应用顶层启动引导 (Bootstrap)、服务依赖装配或进程级生命周期退出机制；
3. **变更规模**：预计涉及文件数 >= 5 个；
4. **高爆炸半径**：一旦出错将导致系统核心进程无法启动或全局瘫痪；
5. **不可逆状态/数据迁移**：触及持久化 Schema、无平滑降级的数据迁移或高破坏性变更。

### 【任务单一关注点与正交互斥律 (Orthogonal Concern Law)】
- **严禁将“局部结构精简 (Local Cleanup)”与“全局流向重排 (Global Rewiring)”混杂在同一任务**。
- **起草 Intent 时的范围自检二选一**：若波及“应用全局启动与装配 (Bootstrap)”或跨越多个顶级子系统，必须且只能二选一：
  A. 升级为 Tier 3 高危任务并启动高危审批流；
  B. 强制将全局装配和跨域流向剔除至 `Non-Goals (明确非目标)`，仅保留纯模块内局部改动。

### 【防伪造与人类独占签批铁律 (Anti-Self-Signoff)】
- 工件底部的 `Gate X Sign-off`（阶段准出签批）是**人类专属权限**。
- AI 在起草 `intent.md` / `spec.md` / `plan.md` 时，**签批区的勾选框 `[ ]` 必须严格保持未勾选状态**，当前状态保持为 `Draft` 或 `In-Review`，签批人保留为 `[待人类签批]`。
- **绝对禁止**：AI 严禁擅自将 `[ ]` 改为 `[x]`，严禁自行写入 `Accepted`，严禁假借 `Dev`、`Tech Lead` 或任何身份自我批准通过！

### 【任务原子性与防巨无霸任务 (Anti-Monolith Tasks)】
- 面对“重构系统架构”、“性能、打包、架构统统优化”等宏大诉求，**严禁立项单一的巨型任务**。
- AI 的标准做法是：先引导人类明确痛点，将其拆解为**单一关注点（Single-Concern）的小步快跑原子任务**，每次只立项并攻克一个原子任务。

## 3. 三阶段工件推进与子代理职责绑定 (SOP)

为防止根会话被大段代码检索污染，主会话与子代理必须严格按照阶段分工：

### Stage 1: Plan / 立项意图 (Intent) —— 主会话轻量处理
- **主会话行为**：聚焦理解用户业务痛点、厘清非目标（Out-of-Scope）与完成定义（DoD）。允许轻量查看用户提及的直观入口文件以确保意图准确。
- **探索防线禁令**：**严禁在主会话发起跨层递归搜索、大规模通读实现细节、或提前执行 `go test` / `npm run build` 等重型构建测试**。
- **执行命令**：`python3 tooling/task_cli.py create <task-id> --title "<标题>" --tier 2`。
- **工件产出**：仅填写 `docs/sdlc/<task-id>/intent.md`。
- **停步确认**：向人类呈现 Intent 核心摘要，等待人类确认后方可进入 Stage 2。

### Stage 2: Design / 架构契约 (Spec) —— 强制委托 Planner
- **进入条件**：人类确认 Intent 后，执行 `python3 tooling/task_cli.py update <task-id> --stage Design` 释放 `spec.md`。
- **强制委派**：**必须通过 `task(subagent_type="planner", ...)` 派发**。
  - **派发规范**：主会话无需读取模板文件，直接按【标准 3 要素】传递 prompt（任务 ID 与目标工件路径、依据与执行目标、限 200 字摘要返回）。
  - `planner` 负责：只读检索源码、梳理数据结构、设计 API 契约与 7 维风险评估，并将结果写入 `spec.md`。
- **主会话行为**：仅负责接收 planner 结果，向人类展示契约与风险点，停步等待人类签批。

### Stage 3: Build / 实施方案 (Plan) 与编码落地 —— 委托 Planner 与 Builder
- **规划委派**：进入实施前，执行 `python3 tooling/task_cli.py update <task-id> --stage Build` 释放 `plan.md`，主会话按【标准 3 要素】委托 `planner` 制定具体的 Milestone、文件修改清单与客观验证命令。
- **构建委派**：实施与测试必须按 Milestone 分批按【标准 3 要素】委托给 `builder` 子代理（`task(subagent_type="builder", ...)`），主会话严禁直接编写业务代码或执行长篇测试。

### Stage 4: Test / 全量集成与客观回归 —— 真实质量门禁
- **进入条件**：`builder` 完成全部 Milestone 的编码后，执行：
  ```bash
  python3 tooling/task_cli.py update <task-id> --stage Test --next "执行全量回归与静态质检"
  ```
- **核心验证矩阵**（由构建环境/子代理严格执行）：
  1. 自动化全套测试：必须 100% 全绿（并发系统必须执行竞态检查）；
  2. 静态代码质量与类型检查（零 warning / 零 error）；
  3. 工程构建与端到端交付产物一致性验证；
  （具体命令严格执行项目 `AGENTS.md` 第一节声明的真实构建与质检指令）。
- **纪律红线**：严禁伪造测试结果，严禁修改已有业务断言掩盖回归缺陷。

### Stage 5: Review / 独立代码审查 —— 强制委托 Reviewer
- **进入条件**：Test 阶段全量测试与构建 100% 全绿后，执行：
  ```bash
  python3 tooling/task_cli.py update <task-id> --stage Review --next "委托 reviewer 执行 3-Pass 审计"
  ```
- **强制委派**：**必须通过 `task(subagent_type="reviewer", ...)` 派发**。
  - **审查依据**：对照 `spec.md` 契约与 `plan.md` 范围，审查 `git diff`。
  - **3-Pass 维度**：
    - Pass 1：架构契约、范围最小化与回滚安全性；
    - Pass 2：并发安全（竞态锁/goroutine 泄漏）与错误处理；
    - Pass 3：KISS 原则（次要建议上限熔断为 5 条）。
- **审查闭环**：
  - 若为 **BLOCK**：由主会话将任务打回 Build 阶段由 `builder` 针对性修复并重新复测；
  - 若为 **APPROVE**：主会话向人类呈报审查结论与变更摘要，等待人类终审签署，执行 `archive` 归档。

## 4. 子代理委派轻量契约 (标准 3 要素)
主会话派发子代理时，子代理已具备自身专职系统提示词。主会话无需额外读取模板文件，直接在 `task` 的 `prompt` 中按**标准 3 要素**传参，极致轻量：

1. **上下文定位**：明确任务 ID 与读写工件路径（如 `docs/sdlc/<task-id>/spec.md`）。
2. **目标与依据**：明确本轮任务具体目标、依据文档（如 `依据 intent.md 设计接口与 7 维风险评估`）及对应阶段要求。
3. **返回与红线约束**：
   - 对 `planner`：强调只读、写入指定工件、向主会话仅返回 200 字精炼摘要；
   - 对 `builder`：限定文件变更范围、强制终端验证命令（如 `go test -race ./...`）全绿、返回真实测试结果摘要；
   - 对 `reviewer`：强调只读、依据根目录 `REVIEW.md` 执行 3-Pass 审计、返回【APPROVE / BLOCK】结论及最多 5 条 Nit。

---

## 5. 风险升级与停止/继续条件 (Stop & Escalate Procedure)
实施过程中若发现：
- 需求或用户业务目标发生变化；
- 任务范围实质性扩大；
- 公共 API 契约发生变动；
- 数据模型发生实质变化；
- 鉴权与安全边界发生变动；
- 核心架构决策发生变化；
- 原方案依赖的人类权衡条件发生变化。

**动作**：立即**停止实施**，更新相关工件并**请求人类确认**。

实施过程中若仅属于：
- 文件位置或内部命名的合理调整；
- 内部实现方式或算法路径的优化；
- 内部实现顺序微调；
- 必要的局部内部重构。

**动作**：在不改变需求、范围和关键设计决策的前提下，**更新 plan.md 后自主继续执行**。

## 6. 活跃任务索引与成果归档流转 (CLI 自动化速查)
项目提供专用的轻量管理工具 `tooling/task_cli.py`，Agent 应直接按照下列参数签名执行，无需调用 `--help` 探测：

1. **查看任务列表 (list)**：
   ```bash
   python3 tooling/task_cli.py list
   ```

2. **查看单任务快照与工件生命周期 (show)**：
   ```bash
   python3 tooling/task_cli.py show <task-id>
   ```

3. **立项/初始化工件 (create)**：
   - 必选参数：`task_id`（小写中划线标识）、`--title "<简明标题>"`
   - 可选参数：`--tier {1|2|3}` (默认 2)、`--owner "<角色/名字>"` (默认 Dev)、`--stage Plan`、`--next "<下一步说明>"`
   ```bash
   # 标准 Tier 2 任务立项 (自动在 Plan 阶段仅释放 intent.md)
   python3 tooling/task_cli.py create <task-id> --title "<简明任务标题>" --tier 2 --owner "Dev"
   ```

4. **推进阶段与更新状态 (update)**：
   - 必选参数：`task_id`
   - 可选参数：
     - `--stage {Plan|Design|Build|Test|Review|Deploy}` (推进到 Design/Build 会自动按需释放 spec.md/plan.md)
     - `--status {in_progress|blocked|ready_for_qa|ready_for_review}`
     - `--risk {"Tier 1"|"Tier 2"|"Tier 3"}`
     - `--owner "<负责人>"`
     - `--next "<下一步动作描述>"`
     - `--blocked "<阻塞根因说明>"` (传 'None' 解除阻塞)
   ```bash
   # 推进至 Design 阶段并释放 spec.md
   python3 tooling/task_cli.py update <task-id> --stage Design --next "编写 spec.md 技术契约"

   # 推进至 Build 阶段并释放 plan.md
   python3 tooling/task_cli.py update <task-id> --stage Build --next "编写 plan.md 实施计划"

   # 阶段回退/打回重做（自动将高于目标阶段的工件备份为 .bak，解除 check 门禁死锁）
   python3 tooling/task_cli.py update <task-id> --stage Plan --next "回退方案，重新明确意图与边界"

   # 标记阻塞状态
   python3 tooling/task_cli.py update <task-id> --status blocked --blocked "等待架构确认接口字段"
   ```

5. **门禁与工件合规自检 (check)**：
   - 可选参数：`[task_id]`（缺省时检查全部活跃任务）
   - *自动执行*：工件完整性、占位符残留检测、防偷跑逆向校验（Plan 阶段严禁出现 spec/plan，Design 阶段严禁出现 plan）。
   ```bash
   python3 tooling/task_cli.py check <task-id>
   ```

6. **任务完成与成果归档 (archive)**：
   - 必选参数：`task_id`、`--outcome "<成果摘要>"`、`--commit "<Commit_SHA_或_PR>"`、`--verify "<验证测试输出摘要>"`
   - 可选参数：`--final-stage Deploy`、`--date YYYY-MM-DD`
   ```bash
   python3 tooling/task_cli.py archive <task-id> --outcome "<成果摘要>" --commit "<Commit_SHA_或_PR>" --verify "go test -race ./... 100% 通过"
   ```

7. **注销/删除任务 (delete)**：
   - 必选参数：`task_id`
   - 可选参数：`--force` (同时物理删除工件目录)
   ```bash
   python3 tooling/task_cli.py delete <task-id>
   ```

8. **认识论纪律（Epistemic Discipline）**：
   - `ACTIVE_TASKS.md` 是当前任务导航索引，**不是任务事实的唯一来源**。
   - Agent 不得仅凭索引字段自我证明任务处于某状态；若索引与代码、测试或底层工件冲突，必须以实际客观证据为准，并纠偏索引。
