# Technical Design: 全站剩余页面控制台化批量收敛

## 1. 架构与规范
- 严格基于 `.trellis/spec/frontend/components-and-routing.md` 规约；
- 复用 `web/src/components/ui/` 原子组件库（`Button`, `Input`, `Badge`, `Drawer`, `Table*`）；
- 核心资源管理页（`OutboundsView`, `RoutingView`）采用 Table-First + Inspector 抽屉架构；
- 监控与图表页（`DashboardView`）采用紧凑网格（Grid）与 Neutral 指标面板。

## 2. 状态映射与接口契约
- 严格保留所有视图原有 API 请求与状态机映射（`api.get`, `api.post`, `api.put`, `api.delete`）；
- 严禁擅自修改字段名称、类型或删除任何已有功能特性。
