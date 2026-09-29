# Components & Routing

> SFC 约定、路由表与导航守卫、样式与图标体系

---

## SFC 约定

- 所有 `.vue` 一律使用 `<script setup lang="ts">`，**禁止 Options API**。
- **不使用 props / emits**：全仓 `defineProps` / `defineEmits` / `defineExpose` / `withDefaults`
  零匹配。组件协作通过导入模块单例（`toast`、`api`）完成，而非 prop 传递。
- 没有自定义 composable（`useX()`）；`useRoute` / `useRouter` 是仅有的 `use*` 调用
  （`App.vue:326-327`、`InboundsView.vue:933`、`OutboundsView.vue:500`、
  `LoginView.vue:201`、`TopologyView.vue:419`）。
- 页面尽量自包含：模板 + 单个 `<script setup>`；局部类型就近 `interface` 声明。
- 生命周期配合清理：`onMounted` 注册 `setInterval`，`onUnmounted` 清理，
  并处理 `visibilitychange`（`App.vue:417-440`）——新增轮询必须成对清理。
- 仅 `components/ToastContainer.vue` 含 `<style scoped>`（`:67-95`，用于过渡关键帧）；
  其余组件样式全部走 Tailwind class。

---

## 路由（`web/src/router/index.ts`）

- 单文件、扁平路由表，无嵌套 `children`。
- **懒加载**：每个视图以 `() => import('...')` 常量声明后再组表（`:4-15`）。
- 路径：`/login`、`/portal`、`/`、`/topology`、`/inbounds`、`/outbounds`、`/routing`、
  `/dns`、`/users`、`/config`、`/logs`、`/settings`，以及 catch-all 重定向（`:30`）。
- **导航守卫**（单一全局 `router.beforeEach`，`:38-50`）：
  - 从 `localStorage.getItem('token')` 判断登录态（`:42`）；
  - 路由 `meta.requiresAuth` 且无 token → 跳 `/login`（`:43-44`）；
  - mock 模式自动注入 token（`:39-41`）；
  - 已登录访问 `/login` → 弹回首页（`:45-46`）。
- **布局切换**：`/login` 与 `/portal` 标记 `meta.layout: 'blank'`（`:18-19`），
  由 `App.vue:338-341` 消费以隐藏侧栏/导航。
- `afterEach` 仅设置 `document.title`（`:52-60`）。

> **规范**：新增页面 = 在 `views/` 建 `XxxView.vue` + 在路由表加懒加载条目；
> 需要鉴权的路由显式标注 `meta.requiresAuth`。

---

## 样式体系

- Tailwind CSS + PostCSS，入口 `web/src/style.css:1-3`（`@tailwind base/components/utilities`）。
- 全局设计类集中在 `style.css`，优先复用而非重写：
  `.glass-panel`（`:20-26`）、`.glass-card`（`:28-41`）、`.btn-primary`（`:44-56`）、
  `.btn-secondary`（`:58-68`）、`.pulse-green`（`:71-88`）、`.fade-slide-*`（`:91-102`）、
  `.modal-scale-*`（`:105-113`）及自定义滚动条（`:116-129`）。
- `web/tailwind.config.js`：`darkMode: 'class'`，扩展 `dark` / `brand` / `cyan` 调色板，
  `plugins: []`（无 typography/forms 插件）。
- **无组件库**（无 Element Plus / Naive / PrimeVue / Headless UI）；
  界面由手写 Tailwind 组成，允许直接使用原始色值（如 `bg-[#07090E]`）。
- 字体在 `web/index.html:8-10` 通过 Google Fonts 引入；
  `web/index.html:2` 根节点固定 `class="dark"`。

---

## 图标与第三方 UI 依赖

- 图标统一使用 `lucide-vue-next`，按需在文件内 import，
  以 `<component :is="item.icon">` 动态渲染（`App.vue:304-320,47-51`、`ToastContainer.vue:38`）。
- 二维码使用 `qrcode.vue`（`UsersView.vue:909`、`SettingsView.vue:380`）。
- 引入新 UI 依赖前需确认现有 Tailwind + lucide 能力不足，避免权重膨胀。
