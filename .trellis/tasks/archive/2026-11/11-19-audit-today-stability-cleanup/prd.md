# 审查今日开发功能稳定性与冗余清理合并（Parent）

## Goal
基于 `2026-11-19` 当日 16 个提交（`a978ede..HEAD`）及工作区未提交改动的全面代码审查结果（见 [today-audit-report.md](research/today-audit-report.md)），按三个独立可验证里程碑有序交付稳定性止血、冗余剔除与架构/交互收敛，避免大爆炸重构。

## Source Audit Artifacts
- [research/today-audit-report.md](research/today-audit-report.md) — 总览与候选 `.trellis/spec/` 清单
- [research/01-inventory-and-baseline.md](research/01-inventory-and-baseline.md) — 16 个提交盘点与门禁基线
- [research/02-stability-risks.md](research/02-stability-risks.md) — P0 (2) / P1 (8) / P2 (4) 缺陷证据
- [research/03-redundancy-and-consolidation.md](research/03-redundancy-and-consolidation.md) — 冗余清单与收敛方案

## Milestone Task Map
| 里程碑 | 子任务目录 | 范围与核心交付 | 依赖顺序 | 状态 |
|---|---|---|---|---|
| **M1** | `11-19-m1-stability-hotfix-and-cleanup` | P0-1、P1-2/3/4/6/8 止血修复；**移除拓扑一键发布功能**（替代 P1-5）；死代码与重复逻辑清理；前端 Prettier 门禁与 `.template-hashes.json` 污染恢复 | 首个执行 | Completed & Archived (`459c8ab`) |
| **M2** | `11-19-m2-async-node-commands-and-pruning` | 统一异步 `NodeCommand` 通道合并 `pendingLogReqs` + `pendingGeoReqs`（根治 P0-2 & P1-1）；接入 `PruneBatchesBefore` 与 `DeleteNode` 级联清理（P1-7）；补齐写接口 `RefreshSyncStatus`（P2-1） | 依赖 M1 完成 | Completed & Archived (`6e0ed5c`) |
| **M3** | `11-19-m3-frontend-consolidation-and-kiss` | 全局节点切换器作用域边界与移除 `InboundsView` 冗余本地筛选（U-1）；合并 `useNodes`/`useActiveNode` + 节点运维抽屉 + GeoData 迁出 `RoutingView`（U-2）；Portal HTML 单源化（U-3）；`NodeRepository` 4 接口 KISS 精简（A-2）；P2-2/P2-3/P2-4 收尾 | 依赖 M1、M2 完成 | Completed & Archived (`2a613eb`) |

## Cross-Child Acceptance Criteria
1. 所有 P0（2 项）与 P1（8 项，其中 P1-5 通过产品决策移除拓扑一键发布解决）均有失败复现先行（Fail-Repro First）回归测试或端到端验证覆盖。
2. 每个子里程碑独立通过全套质量门禁：`mise x -- go test ./...`、`CGO_ENABLED=1 mise x -- go test -race ./...`、`mise x -- go vet ./...`、`mise x -- go build .`、`cd web && mise x -- npm run typecheck && mise x -- npm run lint && mise x -- npm run format:check && mise x -- npm run build`。
3. `cmd/agent` 依赖隔离保持纯净（零 `gin/gorm/sqlite/telegram` 依赖泄漏）。
4. 业务提交绝不包含 `.trellis/.template-hashes.json` 的 `__pycache__/*.pyc` 字节码污染。
