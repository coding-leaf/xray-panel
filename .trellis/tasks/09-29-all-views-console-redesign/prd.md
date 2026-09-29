# PRD: 全站剩余页面控制台化批量收敛

## 1. 业务目标与愿景
紧承 `InboundsView` 与 `UsersView` 的标杆成功，在保持 100% 现有业务逻辑与 API 稳定的前提下，将前端所有剩余视图（`OutboundsView`, `RoutingView`, `DashboardView`, `DNSView`, `ConfigView`, `LogsView`, `SettingsView`, `TopologyView`）全面升级为统一的“开发者基础设施控制台”（Vercel 骨架 + Linear 密度与中性设计令牌）。

## 2. 覆盖视图与改造范围

1. **`OutboundsView.vue` (出站节点管理)**：
   - 移除旧 `glass-panel` 与渐变；
   - 迁移为 Table-First 资源列表 + 右侧 Inspector 抽屉（节点详情、连通性测速、延迟诊断与快捷配置编辑）。
2. **`DashboardView.vue` (总览仪表盘)**：
   - 精炼 KPI 统计指标卡片（中性边框、高密度排版）；
   - 紧凑化系统资源负载（CPU、内存、连接数）与实时流量折线图。
3. **`RoutingView.vue` (分流与路由规则)**：
   - 规则流转 Table-First 排版，清晰展示入站、域名/IP 条件、目标出站 Tag；
   - 规则编辑采用侧边抽屉或紧凑卡片，消除旧弹窗与大圆角毛玻璃。
4. **辅助系统视图 (`DNSView`, `ConfigView`, `LogsView`, `SettingsView`, `TopologyView`)**：
   - 清理所有遗留的 `glass-panel`、`backdrop-blur` 与 Hex 硬编码颜色；
   - 统一收敛为 Neutral Design Tokens（`bg-background`, `bg-card`, `border-border`, `text-foreground`）。

## 3. 验收标准
- 彻底消灭前端全站遗留的 `glass-panel` 与发光毛玻璃类；
- 所有页面在暗色控制台下视觉一致，交互紧凑；
- 前端生产构建全绿：`cd web && npm run build`（0 errors, 0 warnings）；
- 后端门禁全绿：`mise x -- go test ./...`。
