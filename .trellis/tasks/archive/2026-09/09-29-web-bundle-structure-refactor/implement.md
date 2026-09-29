# Implementation Plan — 前端包体结构优化与巨石视图分层重构

## 1. 实施阶段拆解 (Ordered Checklist)

### 阶段一：基础 UI 容器与通用工具类下沉 (Foundation)
- [ ] 创建 `web/src/components/ui/FormField.vue`（收敛 Label/Hint/Error 样式类深度）；
- [ ] 创建 `web/src/components/ui/SectionCard.vue`（收敛卡片与区块容器样式类深度）；
- [ ] 创建 `web/src/utils/format.ts`（集中提供 `formatBytes`, `formatDate`, `formatDuration`）；
- [ ] 创建 `web/src/utils/clipboard.ts`（集中提供 `copyText` 带 toast 统一处理）；
- [ ] 优化 `web/vite.config.ts` 中的 `manualChunks`（增加 `src/components/ui/` -> `ui-primitives` 规则，消除碎片）；
- [ ] 执行基建编译验证：`cd web && npm run typecheck`。

### 阶段二：黄金样本拆解 (InboundsView 业务领域解耦)
- [ ] 创建 `web/src/views/inbounds/types.ts`（定义 InboundItem, SubRouteItem, InboundFormData 契约及 `sanitizeInboundPayload` 清洗函数）；
- [ ] 创建 `web/src/views/inbounds/composables/useInboundList.ts`（提取节点列表拉取、过滤搜索、Reality 巡检、节点删除逻辑）；
- [ ] 创建 `web/src/views/inbounds/components/InboundTable.vue`（承载主表格与行级展示）；
- [ ] 创建 `web/src/views/inbounds/components/InboundDetailDrawer.vue`（承载配置抽屉与凭据展示）；
- [ ] 创建 `web/src/views/inbounds/components/InboundFormDrawer.vue`（重构多协议表单，使用 `FormField` 与 `SectionCard` 替换原有手写嵌套）；
- [ ] 重构 `web/src/views/InboundsView.vue`（收敛为仅保留顶栏、过滤栏、Table 以及异步懒加载的 FormDrawer / DetailDrawer，体积控制在 250 行以内）；
- [ ] 验证类型与打包：`cd web && npm run typecheck && npm run build`。

### 阶段三：全仓工具类平移与共用整合
- [ ] 在 `DashboardView.vue`、`UsersView.vue`、`OutboundsView.vue` 中平移替换内联的 `formatBytes`、`copyText`，引用新沉淀的 `utils/format` 和 `utils/clipboard`；
- [ ] 验证回归影响。

### 阶段四：全量门禁与产物验证 (Verification Gate)
- [ ] 前端类型与构建门禁：`cd web && npm run typecheck && npm run build`（验证产物 chunk 大小与无碎片情况）；
- [ ] 后端测试与构建门禁：`mise x -- go test ./...`、`mise x -- go vet ./...`、`mise x -- go build .`；
- [ ] 检查代码整洁度与 Git diff 审查。

---

## 2. 质量门禁与验证命令

```bash
# 1. 前端类型检查 (无报错)
cd web && npm run typecheck

# 2. 前端构建产物 (观察 chunk 输出结构)
cd web && npm run build

# 3. 后端单元测试
mise x -- go test ./...

# 4. 后端静态检查
mise x -- go vet ./...

# 5. 后端最终二进制打包 (验证静态资源 embed 完整性)
mise x -- go build .
```
