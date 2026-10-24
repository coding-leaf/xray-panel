# 移动端响应式布局与交互体验全量适配 - 实施计划 (Implement Plan)

## Ordered Implementation Checklist

### 1. 通用容器与底层组件安全区适配
- [x] 1.1 `web/src/components/ui/Drawer.vue`: Footer 增加 `pb-[max(1rem,env(safe-area-inset-bottom))]`，防止移动端底部按键遮挡。
- [x] 1.2 `web/src/components/ui/Modal.vue`: Footer 增加 safe-area 兼容与紧凑内边距。

### 2. 核心数据表格移动端卡片流实现
- [x] 2.1 `web/src/views/users/components/UserTable.vue`:
  - 增加 `<div class="sm:hidden space-y-2.5">` 移动端用户卡片流；
  - 完整保留多选、状态展示、流量进度条、权限线路统计、操作按钮事件派发；
  - `<Table>` 容器添加 `hidden sm:block`。
- [x] 2.2 `web/src/views/inbounds/components/InboundTable.vue`:
  - 原表格添加 `hidden sm:block`；
  - 增加 `<div class="sm:hidden space-y-2.5">` 移动端入站卡片流，完整包含 Tag、协议、端口、安全、线路数、流量与快捷操作。
- [x] 2.3 `web/src/views/outbounds/components/OutboundTable.vue`:
  - 修复 `overflow-hidden` 改为 `overflow-x-auto`；
  - 桌面表格增加 `hidden sm:block`；
  - 增加 `<div class="sm:hidden space-y-2.5">` 移动端出站卡片流。
- [x] 2.4 `web/src/views/DashboardView.vue`:
  - 入站节点概要表格改为 `overflow-x-auto`，移动端优化表头与间距。

### 3. 门禁验证与构建
- [x] 3.1 `cd web && mise x -- npm run typecheck` 静态类型验证。
- [x] 3.2 `cd web && mise x -- npm run build` 生产构建打包。
- [x] 3.3 `mise x -- go test ./...` 与 `mise x -- go vet ./...` 全绿。
