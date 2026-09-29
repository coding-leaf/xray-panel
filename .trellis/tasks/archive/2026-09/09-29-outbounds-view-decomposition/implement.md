# Implementation: 拆解 OutboundsView 巨石视图与组件化重构

## Ordered Checklist

- [x] **Step 1: 建立出站领域模型与纯函数清洗器**
  - 创建 `web/src/views/outbounds/types.ts`；
  - 声明 `OutboundItem`、`OutboundFormState`、`OutboundPayload` 等类型；
  - 迁移并封装 `buildSettingsJSON`、`buildStreamSettingsJSON`、`sanitizeOutboundPayload`、`populateOutboundForm` 以及展示辅助函数。

- [x] **Step 2: 拆解主列表表格组件 `OutboundTable.vue`**
  - 创建 `web/src/views/outbounds/components/OutboundTable.vue`；
  - 迁移顶部操作栏、过滤/搜索框、表格主体与行内快捷操作；
  - 暴露 `inspect`、`edit`、`delete`、`create` 等规范事件。

- [x] **Step 3: 拆解详情检视抽屉 `OutboundDetailDrawer.vue`**
  - 创建 `web/src/views/outbounds/components/OutboundDetailDrawer.vue`；
  - 渲染节点元信息、WireGuard/WARP 详情、StreamSettings 传输安全及底盘 JSON 高亮展示；
  - 保护系统保留标签（`direct`, `block`）不可误删。

- [x] **Step 4: 拆解配置表单抽屉 `OutboundFormDrawer.vue`**
  - 创建 `web/src/views/outbounds/components/OutboundFormDrawer.vue`；
  - 规范使用 `FormField` 与 `SectionCard` 原语替代冗余样式；
  - 完整迁移 Freedom、Blackhole、WireGuard、VLESS、VMess、Trojan、Shadowsocks、Socks、HTTP 各协议表单控件；
  - 实现 protocol 切换及网络安全协议联动重置。

- [x] **Step 5: 重构顶层调度器 `OutboundsView.vue`**
  - 改造为 < 150 行的简洁视图；
  - 使用 `defineAsyncComponent` 异步加载 `OutboundFormDrawer`；
  - 编排 API 交互（获取、保存、删除）与路由 query 联动。

- [x] **Step 6: 门禁验证与质量确认**
  - 运行 `cd web && npm run typecheck` 确保 0 警告 0 错误；
  - 运行 `cd web && npm run build` 确保前端构建与按需分包正常；
  - 运行 `go test ./...` 与 `go vet ./...` 确保后端全绿。
