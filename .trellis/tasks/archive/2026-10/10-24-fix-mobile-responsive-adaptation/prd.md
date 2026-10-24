# 移动端响应式布局与交互体验全量适配

## Goal
全面消除手机端与窄屏设备下的样式截断、表格横向死锁溢出、底部手势栏遮挡与抽屉弹窗溢出等体验缺陷，实现多终端一致的响应式卡片流交互体验。

## Requirements

### 1. 核心业务表格的“移动端卡片流”改造
- **用户管理 (`UserTable.vue`)**：
  - 桌面端（`>= sm`）保持密集数据表格展现；
  - 移动端（`< sm`）提供专属自适应卡片流视图，直观展示用户邮箱、状态标识、归属节点与分流线路、已用/配额进度条、到期时间以及一键分享/编辑/删除等高频操作。
- **入站节点 (`InboundTable.vue`)**：
  - 移动端提供自适应卡片流，直观呈现节点 Tag、协议/流控、端口、传输安全（REALITY/TLS 状态）、关联线路数、流量与操作入口。
- **出站节点 (`OutboundTable.vue`)**：
  - 修复原本 `overflow-hidden` 导致的横向内容被强行截断问题，并在移动端补充卡片视图。
- **仪表盘 (`DashboardView.vue`)**：
  - 修复节点概要表格横向溢出问题，增加 `overflow-x-auto` 滚动容错与紧凑排版。

### 2. 抽屉 (Drawer) 与弹窗 (Modal) 移动端自适应
- 在 `Drawer.vue` 与 `Modal.vue` 的底部操作栏 (`footer`) 引入底部安全区内边距（`pb-[max(1rem,env(safe-area-inset-bottom))]`），杜绝 iPhone Home 指示条或安卓全面屏手势遮挡保存/取消按钮。
- 确保表单抽屉在移动端全屏撑开且表单内多列网格自动降为单列。

### 3. 全局布局与底部导航栏优化
- 检查 `App.vue` 移动端底部高频导航条，确保与各子视图的底部留白（`pb-20`）良好配合，无内容死角。

## Acceptance Criteria
- [x] `UserTable.vue` 在 `< sm` 视图下展示卡片流，且支持多选、查看、分享、编辑、删除与实时测速显示；
- [x] `InboundTable.vue` 在 `< sm` 视图下展示卡片流，且操作按钮无横向截断；
- [x] `OutboundTable.vue` 修复内容截断，移动端具备卡片视图；
- [x] `Drawer.vue` 与 `Modal.vue` 具备底部安全区支持；
- [x] `cd web && mise x -- npm run typecheck` 零类型报错；
- [x] `cd web && mise x -- npm run build` 成功输出生产构建产物；
- [x] 后端测试与静态检查 `mise x -- go test ./...` 保持全绿。
