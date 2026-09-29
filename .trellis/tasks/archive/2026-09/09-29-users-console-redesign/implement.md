# Implementation Plan: UsersView 控制台重构

## Milestone 1: 核心表格与批量操作栏重构
- [x] 备份原 `UsersView.vue` 业务逻辑和完整事件处理；
- [x] 构建顶部工具条（统计 KPI 概览、搜索框与状态过滤单选/下拉）；
- [x] 迁移主列表为 `Table-First` 结构（使用 `Table.vue`, `TableHeader.vue`, `TableRow.vue`, `TableCell.vue` 等）；
- [x] 重构批量操作工具栏（使用 Neutral 语义卡片，支持批量续费、批量清空流量、批量状态启停）。

## Milestone 2: 巡检抽屉与表单抽屉落地
- [x] 接入 `Drawer.vue` 实现右侧 `UserInspectorDrawer`：
  - 用户凭据与 UUID 快速复制；
  - 流量明细与到期时间诊断；
  - 授权入站节点列表与便捷开关；
- [x] 接入 `Drawer.vue` 实现右侧 `UserFormDrawer`（新增/编辑用户表单）：
  - 凭据、流量限额、过期设置、设备数限制折叠分组；
  - 入站节点权限多选。

## Milestone 3: 订阅链接、二维码弹窗与样式收敛
- [x] 二维码展示弹窗样式收敛至 Neutral 风格；
- [x] 移动端自适应与横向滚动保护；
- [x] 移除旧版 `glass-panel` 与散落的 Hex 颜色类。

## Milestone 4: 质量门禁与端到端验证
- [x] 执行前端类型检查与生产构建：`cd web && npm run build`；
- [x] 执行后端全量单元测试：`mise x -- go test ./...`；
- [x] 浏览器验证 CRUD、批量操作、订阅提取等交互。
