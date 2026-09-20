# Intent: 后端数据流与并发性能优化

- **任务编号**: backend-dataflow-perf
- **提出人**: Dev
- **创建时间**: 2026-09-20 12:02
- **初始 Change Tier**: Tier 2
- **当前状态**: In-Review

---

## 1. 问题与现状背景 (Problem)
Xray-panel 后端承载系统监控、Xray 节点流量统计、实时事件广播（SSE/WebSocket/Channels）及定时聚合持久化等高频数据流。在多入站节点、多客户端长连接并发访问场景下，高频轮询采集与事件扇出可能存在互斥锁粒度过粗、切片/缓冲区重复内存分配、以及事件广播未做非阻塞缓冲隔离等潜在性能瓶颈。

## 2. 变更性质分类 (Change Archetype - 单选)
- [x] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [ ] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. 梳理并优化高频数据流（如监控采集、流量统计聚合与实时广播通道）的并发安全与锁粒度；
2. 减少热点路径上的内存临时对象分配（如复用缓冲区或精简频繁深拷贝）；
3. 强化事件广播管道的背压与非阻塞保护，避免个别慢连接导致广播管道积压或 goroutine 泄漏；
4. 保证所有测试在 `-race` 竞态检测下 100% 通过，且保持业务逻辑与 API 契约完全向后兼容。

## 4. 波及工程分面 (Affected Architectural Layers)
- [x] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [ ] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)
- 注：主要波及 `internal/domain` 或 `internal/service` 中的数据流汇聚、广播与监控处理逻辑。

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**: 
  - 严禁产生任何并发竞态问题（必须通过 `go test -race ./...` 检验）；
  - 严禁引入 goroutine 泄漏或死锁隐患；
  - 保证业务计算逻辑与流量统计数值的绝对准确。
* **明确非目标 (Non-Goals / Out-of-Scope)**: 
  - 不修改已有公共 HTTP API 契约和字段格式；
  - 不修改底层 SQLite 数据库表结构和持久化 Schema；
  - 不进行全局应用启动流程重构。
* **完成判定条件 (Definition of Done)**:
  - 明确数据流瓶颈与并发优化点；
  - 编写或补充关键并发场景单元测试；
  - `go test -race ./...` 100% 绿灯，`go vet ./...` 零告警，`go build .` 正常构建。

## 6. 未决疑问与待探讨点 (Open Questions)
- 需由 planner 深入只读调研当前流量与监控收集器的具体实现（如 TrafficCollector、SystemMonitor、Broadcaster），定位最具收益的具体瓶颈点。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-20 12:02
