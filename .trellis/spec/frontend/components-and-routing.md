# Components & Routing

> SFC 约定、路由表与导航守卫、样式与图标体系

---

## SFC 约定与组件分层

- 所有 `.vue` 一律使用 `<script setup lang="ts">`，**禁止 Options API**。
- **组件分层**：
  - **原子组件（`web/src/components/ui/`）**：强类型通用原语（如 `Button`, `Input`, `Badge`, `Drawer`, `Modal`, `Table` 系列，以及 `FormField`, `SectionCard` 容器），严格使用 `defineProps` 约束变体、尺寸与状态，基于 CSS 变量语义令牌，负责收敛 Label/Hint/Error 与居中/抽屉浮层深度，禁止散落内联 Hex 颜色。
  - **业务领域子组件（`web/src/views/<feature>/components/`）**：针对 50KB+ 的庞大视图，按领域就近聚合拆分为子组件（如 `InboundTable`, `UserTable`, `UserDetailDrawer`, `UserFormDrawer`）；重型表单抽屉与大型弹窗（如二维码、趋势图）采用 `defineAsyncComponent` 异步懒加载以优化首屏 chunk 体积。
  - **业务调度视图（`web/src/views/`）**：遵循自包含与轻量化原则，主视图作为调度器协调过滤状态、表格与双抽屉，局部类型与参数清洗收敛至 `views/<feature>/types.ts`。
- 页面协作与模块解耦：跨层单例协作继续通过导入模块单例（`toast`、`api`）完成。
- 生命周期配合清理：`onMounted` 注册 `setInterval`，`onUnmounted` 清理，
  并处理 `visibilitychange`——新增轮询必须成对清理。
- 样式主要全部走 Tailwind class 与 Design Tokens 语义类。

---

## 样式体系与 Design Tokens 规范

- Tailwind CSS + PostCSS，入口 `web/src/style.css`。
- **开发者基础设施控制台设计令牌 (Neutral Design Tokens)**：
  - 基于 CSS 语义变量（`--background`, `--foreground`, `--card`, `--muted`, `--border`, `--accent`）；
  - 全站禁止毛玻璃 `glass-card`、`backdrop-blur` 和彩色发光渐变；
  - 严禁随意内联 Hex 色值，一律收敛至 `bg-background`、`bg-card`、`border-border` 等语义令牌；
  - 圆角统一收敛：控件 `6px` (`rounded-md`)，容器/面板 `8px` (`rounded-lg`)，抽屉/弹窗 `10px/12px`；
  - 语义功能色严格收敛：仅允许绿 (正常/运行)、红 (异常/停止/危险操作)、黄 (警告/临期)、蓝/紫 (当前激活态/主操作)。
- 字体在 `web/index.html` 引入，根节点固定 `class="dark"`。

---

## 图标与第三方 UI 依赖

- 图标统一使用 `lucide-vue-next`，按需在文件内 import，
  以 `<component :is="item.icon">` 动态渲染（`App.vue:304-320,47-51`、`ToastContainer.vue:38`）。
- 二维码使用 `qrcode.vue`（`UsersView.vue:909`、`SettingsView.vue:380`）。
- 引入新 UI 依赖前需确认现有 Tailwind + lucide 能力不足，避免权重膨胀。

---

## 核心视图交互范式 (Table-First, Double Drawers & Batch Bar)

- **Table-First 布局**：主视图默认通过紧凑高密度数据表格呈现资源列表，支持多选、关键字检索与状态过滤。
- **双抽屉架构 (Double Drawers)**：
  - **Inspector Drawer**：行点击唤起右侧巡检面板（只读状态诊断、凭据复制、快捷开关与关联拓扑矩阵）；
  - **Form Drawer**：新建或编辑资源时滑出右侧结构化表单（分组收拢，告别传统几十个字段的居中弹窗墙）。
- **批量操作条 (Batch Action Bar)**：当选择项数量 `> 0` 时以中性卡片浮动呈现，提供高频批量变更与一键清空选择，操作后即时刷新列表状态。
