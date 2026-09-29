# Implementation Plan — UsersView 巨石视图拆解与 Modal 原语沉淀

## 1. 实施阶段拆解 (Ordered Checklist)

### 阶段一：通用模态弹窗原语建设 (Foundation)
- [ ] 创建 `web/src/components/ui/Modal.vue`（实现 Teleport、动效、尺寸适配、Esc 与遮罩点击，提供 Header/Body/Footer 插槽）；
- [ ] 确保 `Modal.vue` 满足 TypeScript 代码卫生（无未使用参数、导出明确）；
- [ ] 验证 `web/vite.config.ts` 中的 `ui-primitives` 自动包含该原语；
- [ ] 运行 `npm run typecheck` 进行基建自验。

### 阶段二：用户领域服务、类型与 Composable 提炼 (Domain Extraction)
- [ ] 创建 `web/src/views/users/types.ts`（定义 UserItem, UserFormData, TrafficRecord 契约及 `sanitizeUserPayload`）；
- [ ] 创建 `web/src/views/users/services/subscription.ts`（收敛 `UserSubscriptionService` 订阅与节点链接生成类）；
- [ ] 创建 `web/src/views/users/composables/useUserList.ts`（提取用户列表、统计指标、过滤搜索、批量选中与批量操作方法）。

### 阶段三：组件拆解与重构 (Components & Async Loading)
- [ ] 创建 `web/src/views/users/components/UserTable.vue`（主列表展示、KPI 指标、批量操作浮动条）；
- [ ] 创建 `web/src/views/users/components/UserDetailDrawer.vue`（用户诊断抽屉、配额图表与快捷开关）；
- [ ] 创建 `web/src/views/users/components/UserFormDrawer.vue`（重构新增/编辑表单，使用 `FormField` 与 `SectionCard`，接入 `sanitizeUserPayload`）；
- [ ] 创建 `web/src/views/users/components/UserShareModal.vue`（基于 `Modal.vue` 重构二维码与订阅节点弹窗）；
- [ ] 创建 `web/src/views/users/components/UserTrafficModal.vue`（基于 `Modal.vue` 重构流量趋势与诊断矩阵弹窗）；
- [ ] 重构 `web/src/views/UsersView.vue`（作为轻量调度器，异步懒加载 FormDrawer、ShareModal、TrafficModal，行数缩减至 200 行以内）。

### 阶段四：全量门禁与质量审查 (Quality Gates & Verification)
- [ ] 运行严格前端类型检查：`cd web && npm run typecheck`（0 警告、0 错误）；
- [ ] 运行生产构建：`cd web && npm run build`（核验 `UserShareModal`、`UserFormDrawer` 独立分块）；
- [ ] 运行全套后端测试与构建：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`；
- [ ] 检查代码整洁度。
