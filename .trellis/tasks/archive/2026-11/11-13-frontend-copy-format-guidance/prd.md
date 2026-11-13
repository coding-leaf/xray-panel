# Frontend copy, formatting, and web-guidance cleanup

## Goal

在保持 `xray-panel` 前端扁平架构（Vue 3 `<script setup lang="ts">` + Tailwind CSS 3 + 无 Pinia/组件库）与嵌入式构建（`embedded.go` `//go:embed all:web/dist`）约束的前提下：
1. 在 `web/` 引入轻量 Prettier + ESLint 开发期工具链，统一代码风格并建立自动化格式/静态检查脚本；
2. 结合 `modern-web-guidance` 以**保守语义增强**方式修复前端核心原语、表单、滚动条及交互元素的 HTML/CSS/WAI-ARIA 缺陷；
3. 按**克制语言**原则（中文极简控制台风格 + 保留中性防探测外壳标题）统一全站 UI 文案、提示语、标点与中英文排版。

## Background & Confirmed Facts

- **当前前端规模与风格基线**：`web/src/` 共 44 个 `.vue` 文件（42 个 `<template>` 在前，仅 `FormField.vue` 与 `SectionCard.vue` 为 `<script>` 在前）和 20 个 `.ts` 文件（17 个无分号，3 个含分号：`web/src/mock/index.ts`、`web/src/views/users/composables/useUserList.ts`、`web/src/views/users/types.ts`）。
- **工具链与门禁现状**：`web/package.json` 仅有 `vue-tsc --noEmit` 与 `vite build`，未安装 Prettier/ESLint，也无前端自动化测试框架；`web/tsconfig.json` 保持 `strict: false`，启用了 `noUnusedLocals`、`noUnusedParameters`、`noFallthroughCasesInSwitch`、`forceConsistentCasingInFileNames`。
- **核心语义与可访问性待改进点**：
  - `web/src/components/ui/Modal.vue`、`web/src/components/ui/Drawer.vue` 使用 `Teleport + div + window keydown` 实现浮层，缺少 `role="dialog"`、`aria-modal="true"`、`aria-labelledby` 及关闭按钮 `aria-label`（目前仅依赖 `title`）。
  - `web/src/components/ui/FormField.vue` 的 `<label>` 未与控件建立 `for`/`id` 关联，提示与错误文本未绑定 `aria-describedby` / `aria-invalid`。
  - `web/src/views/LoginView.vue` 等表单存在可访问性及移动端输入提示缺失（如缺少 `<main>` 地标、密码显隐按钮缺少 `aria-pressed` 与 `aria-label`、2FA 动态码输入缺少 `inputmode="numeric"` 与 `autocomplete="one-time-code"`、用户名缺少 `autocorrect="off"` / `autocapitalize="none"` / `spellcheck="false"`）。
  - `web/src/components/ToastContainer.vue` 缺少 `aria-live="polite"` / 关闭按钮 `aria-label`；`web/src/style.css` 使用裸 `::-webkit-scrollbar` 且缺少标准 `scrollbar-width` / `scrollbar-color` 及 `prefers-reduced-motion` 保护。
- **文案现状**：全站 Toast 与页面提示语存在感叹号与平铺句混用、半角冒号 `:` 与全角冒号 `：` 混用、冗余修饰词（“已成功……”）、术语大小写不统一（`Reality`/`REALITY`、`Xray Core`/`Core`/`Xray 核心`）、中英文混排无空格以及局部多余英文后缀（如 `LoginView.vue` `"系统服务已就绪 · Ready"`）。

## Requirements

### R1: 前端 Prettier + ESLint 工具链引入（仅开发依赖）
- 在 `web/package.json` 的 `devDependencies` 引入最小必要工具集（Prettier + ESLint 9 + Vue/TS/Prettier 适配插件），不引入任何运行时依赖，不改变 `web/dist` 运行时包结构。
- 在 `web/` 根目录新增配置并新增 `npm run lint`、`npm run lint:fix`、`npm run format`、`npm run format:check` 脚本。
- 规则适配项目现状：单引号、无分号、2 空格缩进、SFC 块顺序 `<template>` → `<script>` → `<style>`；兼容 `strict: false`、现有 `any` 用法及 `_` 前缀未使用参数约定，避免无意义的存量类型重构噪音。

### R2: 全量前端代码格式与静态卫生整理
- 使用统一配置整理 `web/src/` 下全部 44 个 `.vue` 与 20 个 `.ts` 文件（含修正 2 个 `<script>` 在前的 SFC 顺序与 3 个含分号 `.ts` 文件）。
- 消除 ESLint 报错与 `vue-tsc --noEmit` 错误，保持代码可读性与既有业务逻辑零变更。

### R3: HTML/CSS/表单语义与无障碍保守增强（依据 `modern-web-guidance`）
- **决策约束**：保留 `Modal.vue` / `Drawer.vue` 既有 `Teleport + Transition` DOM 骨架与过渡动画，不迁移至原生 `<dialog>` 或 `@starting-style`，确保零动画/移动端 WebView 回归风险。
- 补齐浮层与通知语义：`Modal.vue`、`Drawer.vue` 增加 `role="dialog"`、`aria-modal="true"`、`aria-labelledby`、唯一标题 ID 及关闭按钮 `aria-label="关闭"`；`ToastContainer.vue` 增加礼貌播报语义与关闭按钮 `aria-label`。
- 补齐表单与输入语义：`FormField.vue` 借助唯一 ID（`useId()`）提供 `for`/`id` 与 `aria-describedby` / `aria-invalid` 关联能力；`Input.vue` 支持透传 `id` / `aria-describedby` / `aria-invalid` 等属性；`LoginView.vue`、`PortalClaimView.vue` 等关键登录/认证表单补全 `<main>`、`label` 关联、`autocomplete`、`inputmode`、`autocorrect="off"`、`autocapitalize="none"`、`spellcheck="false"` 及密码切换按钮 `aria-pressed` / `aria-label`；非提交按钮显式补齐 `type="button"`。
- 补齐 CSS 现代渐进增强：`web/src/style.css` 按 `customize-scrollbar-color-and-thickness` 规范使用标准 `scrollbar-width: thin` + `scrollbar-color`，并将 `::-webkit-scrollbar` 封装在 `@supports not (scrollbar-color: auto)` 回退块内；补充 `@media (prefers-reduced-motion: reduce)` 对脉冲与过渡动画降噪。

### R4: 前端 UI 文案与术语克制统一
- **保留中性防探测外壳**：保留 `web/index.html`、`web/src/router/index.ts`、`LoginView.vue`、`PortalClaimView.vue` 既有的中性防探测页面标题与品牌外壳（如 `System Management Console`、`XRAY PANEL`、`分布式网管接入点`），不改成暴露代理身份的中文标题。
- **清理冗余双语后缀与口语化表达**：去除无意义双语拼接（如 `'系统服务已就绪 · Ready'` → `'系统服务已就绪'`）；全站 Toast、弹窗、按钮与状态提示采用克制中文控制台语气，去除感叹号 `！` 与冗余“已成功”修饰（如 `'Xray 核心已重启'`、`'DNS 配置已保存并生效'`）。
- **统一标点、空格与术语**：错误/详情提示统一使用全角中文冒号 `：`（如 `'保存失败：'`）；中英文及数字之间统一保留半角空格（如 `'6 位动态码'`、`'Xray 核心'`）；统一协议与认证专有名词写法（如 `REALITY`、`XTLS Vision`、`2FA`）。
- **严守项目既有文案红线**：不得修改既有的真实能力边界提示语义（如不可将“面板当前管理策略暂未开放”改写为“Xray 不支持”；保留 `freedom.settings.domainStrategy` / `UseIPv4v6` / `UseIPv6v4` 语义说明）。

## Out of Scope

- 不重构前端状态管理（严禁引入 Pinia/Vuex 或第三方 UI 组件库）。
- 不重做页面视觉风格、Tailwind 色彩令牌、桌面表格 + 移动卡片流双模布局及拓扑图 SVG 架构。
- 不在 `web/` 引入 vitest / playwright 等前端自动化测试框架。
- 不修改后端 Go 代码、API 契约与数据库 Schema（仅在最终交付前同步 `web/dist` 并执行后端测试与构建门禁）。

## Acceptance Criteria

- [x] `web/package.json` 仅在 `devDependencies` 新增 Prettier/ESLint 相关包，运行时 `dependencies` 无新增，且包含 `lint`、`format`、`format:check` 脚本。
- [x] `cd web && mise x -- npm run format:check` 零差异通过。
- [x] `cd web && mise x -- npm run lint` 零错误通过。
- [x] `cd web && mise x -- npm run typecheck`（`vue-tsc --noEmit`）零错误通过。
- [x] `cd web && mise x -- npm run build` 成功生成 `web/dist` 且产物无异常膨胀。
- [x] `Modal.vue`、`Drawer.vue`、`FormField.vue`、`Input.vue`、`ToastContainer.vue`、`LoginView.vue`、`style.css` 均完成 R3 保守语义增强且不破坏现有移动端安全区与交互行为。
- [x] 全站 `web/src/` UI 文案符合 R4 克制语言、全角冒号、中英文空格与防探测外壳规范，且未违反 `.trellis/spec/frontend/quality-guidelines.md` 文案红线。
- [x] `mise x -- go test ./...`、`mise x -- go vet ./...` 与 `mise x -- go build .` 全量通过。
