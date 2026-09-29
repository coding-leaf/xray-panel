# Implementation Plan: 全站剩余页面控制台化批量收敛

## Milestone 1: OutboundsView 出站节点控制台化
- [x] 备份业务逻辑；
- [x] 引入 `Table*` 原语与 `Drawer.vue` 检查器；
- [x] 收敛出站测速、编辑与启停状态。

## Milestone 2: DashboardView 仪表盘总览紧凑化
- [x] 改造核心指标卡片为 Neutral 控制台 KPI 矩阵；
- [x] 适配系统资源与流量图表容器，消除毛玻璃与渐变边框。

## Milestone 3: RoutingView 与辅助视图全面收敛
- [x] 改造 `RoutingView` 为规则列表 Table-First；
- [x] 批量收敛 `DNSView`, `ConfigView`, `LogsView`, `SettingsView`, `TopologyView` 的 `glass-panel` 与样式变量。

## Milestone 4: 全量质量门禁与跨页面验证
- [x] `cd web && npm run build` 确保零错误；
- [x] `mise x -- go test ./...` 确保全绿。
