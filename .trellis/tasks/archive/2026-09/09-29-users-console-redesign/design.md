# Technical Design: 用户管理 (UsersView) 控制台重构

## 1. 架构与设计规范
遵从 `.trellis/spec/frontend/components-and-routing.md` 中定义的基础设施控制台规范：
- 严格使用 Neutral 语义令牌（`bg-background`, `bg-card`, `border-border`, `text-foreground`, `text-muted-foreground` 等），消除 Hex 色值与 `backdrop-blur`。
- 采用复用的原子组件库 `web/src/components/ui/`（`Button.vue`, `Input.vue`, `Badge.vue`, `Table*.vue`, `Drawer.vue`）。

## 2. 状态机与组件结构

```text
UsersView.vue
├── TopBar & KPI Metrics (用户统计 & 搜索过滤)
├── BatchActionBar (当 selectedUserIds.length > 0 时浮动显示)
├── UsersTable (Table-First 核心列表，支持整行选中唤起 Inspector)
│    ├── Checkbox
│    ├── User Identity (Username, Email, UUID)
│    ├── Inbound Tags (Badge List)
│    ├── Traffic Usage & Progress
│    ├── Expire Date & Reset Cycle
│    ├── Status Badge
│    └── Action Buttons (Copy Sub, QR Code, Quick Edit, More)
├── UserInspectorDrawer (Drawer.vue 右侧只读/快速运维面板)
│    ├── Quick Sub URL & QR View
│    ├── Quota & Bandwidth Diagnostics
│    ├── Associated Inbounds Matrix
│    └── Quick Action Triggers
├── UserFormDrawer (Drawer.vue 创建与编辑用户抽屉)
│    ├── Auth Section (Username, Password/UUID, Email)
│    ├── Quota & Cycle Section (Traffic Limit, Expire Date, Monthly Reset)
│    └── Permission Section (Inbound Tags Multi-select, Max Devices)
└── QrModal (二维码弹窗与订阅链接一键提取)
```

## 3. 数据流与接口兼容
现有 API 调用的契约保持 100% 稳定：
- `GET /api/users` -> `users` 列表拉取
- `POST /api/users` -> 创建新用户
- `PUT /api/users/:id` -> 更新用户详情与权限
- `DELETE /api/users/:id` -> 删除单用户
- `POST /api/users/batch-renew` -> 批量延期
- `POST /api/users/batch-reset-traffic` -> 批量重置流量
- `POST /api/users/batch-status` -> 批量启停
- `GET /api/inbounds` -> 获取可用入站节点 tags 供权限勾选

## 4. 回滚与风险控制
- 原 `UsersView.vue` 业务逻辑和状态映射均被保留，仅改变呈现层与交互层组件。
- 每次改动后通过 `npm run build` 和 `go test ./...` 严防类型与运行期退化。
