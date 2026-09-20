# AGENTS.md - xray-panel 全局工程协议

本文件是常驻工程行为基准与中枢索引，启动时自动加载。终端实际运行结果与源码是唯一事实源。

---

## 1. 核心命令 (Commands)

- 自动化测试：`go test -race ./...` (必须全绿)
- 静态质量检查：`go vet ./...`
- 后端构建：`go build .`
- 前端构建：`cd web && npm run build`

---

## 2. 架构拓扑与事实源 (Architecture & Truth)

- 事实源唯一性：终端测试运行结果与有效源码 >> 架构文档 >> 历史记录。
- 架构分层边界：
  - `internal/domain`：纯业务逻辑与领域模型，保持无副作用；
  - `internal/service`：业务用例编排；
  - `internal/adapter`：外部系统、存储与网络协议适配；
  - `web/`：前端交互面板。

---

## 3. 核心红线 (Must-Not)

- 严禁在方案获确认前擅自修改公共 API 契约、数据库 Schema 或核心持久化格式。
- 严禁在并发路径中遗漏互斥保护，严禁产生 goroutine 泄漏。
- 严禁将真实公网 IP、生产域名、Token、私钥或敏感凭证写入代码或配置。
- 严禁静默吞掉 error 或忽略未处理的异常返回值。
- 严禁过度抽象（KISS 原则）：严禁单实现 interface、空转包装类与各类无痛点工厂。
- 严禁主会话越俎代庖（禁止直通改代码）：主会话定位为【纯调度指挥官】，严禁在 Tier 2 / Tier 3 任务中直接调用 edit/write 工具编写业务代码，必须通过 task 委派专职子代理。

---

## 4. 典型错误防御 (Things AI Gets Wrong)

- 并发竞态：并发读写共享状态时遗漏互斥锁或原子操作，导致 `-race` 告警。
- 伪造测试：自称测试已通过但未在终端实际执行，或通过削弱断言掩盖回归。
- 巨石任务：面对复杂诉求试图立项“大爆炸重构”，必须主动提醒人类按领域拆解为原子任务。
- 幻觉签批：误以为自己可代替人类在工件中勾选签批；Sign-off 必须由人类专属签批。
- 单会话直通懒惰：自恃掌握工具而违规跳过子代理；凡 Tier 2 / Tier 3 任务必须严格委派 planner、builder 和 reviewer 分阶段执行。

---

## 5. 协同与能力索引 (Collaboration & Skills)

- 特性与架构变更：加载 `sdlc-workflow` 技能，通过 `python3 tooling/task_cli.py` 驱动工件推进。凡 Tier 2 / Tier 3 任务，**必须强制通过 `task` 工具委派 `planner` / `builder` / `reviewer`**，严禁主会话单会话直通越权实施。
- 缺陷排查与修复：加载 `bug-fix` 技能，遵循 Fail-repro First（失败测试先行）。
- 代码审查与把关：由 `reviewer` 子代理对照根目录 `REVIEW.md` 执行 3-Pass 审计。
