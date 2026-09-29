# 3-Pass 架构与代码审查规范 (Review Guide)

> 用于 trellis-check 及 PR / 代码合并前审计的 3-Pass 门禁指南

---

## Pass 1: 功能正确性与测试真实性 (Correctness & Testing)

- **逻辑覆盖**：核心业务分支、边界值与空值处理是否周全；
- **错误处理**：排查是否存在任何未处理的 error 或静默吞错；
- **测试真实性**：
  - 测试用例必须在终端真实执行并通过（`mise x -- go test ./...`）；
  - 严禁为了测试变绿而弱化断言；
  - Bug 修复必须遵循 Fail-repro First（失败测试先行复现）。

---

## Pass 2: 运行时与业务安全红线 (Runtime & Domain Invariants)

- **并发竞态排查**：共享状态访问是否均有互斥锁防护，检查 `-race` 警告；
- **资源泄漏排查**：确保 goroutine、HTTP Body、DB 连接及定时器均可正常终止释放；
- **敏感信息防护**：严禁硬编码真实 IP、密钥、生产 Token。

---

## Pass 3: 架构契约与 KISS 规范 (Spec Compliance & KISS)

- **契约对照**：变更是否符合 `.trellis/spec/` 及任务 PRD/Design 契约；
- **极简原则审计**：排查是否引入了单实现 interface、过度封装、空转包装层等复杂性负债；
- **Nit 控制**：非功能性轻微改进建议（Nit）单次评审上限 5 条，避免过度噪音。
