# Design: Frontend Copy, Formatting, and Conservative Web-Guidance Cleanup

## 1. Architecture & Tooling Boundaries

本任务全部改动限定在 `web/`（工程配置 + `web/src/`）与 `.trellis/spec/frontend/`（Phase 3 沉淀），严守以下边界：
- **零运行时侵入**：`prettier`、`eslint` 及相关插件仅写入 `web/package.json` 的 `devDependencies`；Vite 构建输出 `web/dist` 与 Go `embedded.go` 嵌入逻辑保持不变。
- **不引入外部 UI/状态库**：继续沿用 Vue 3.4 `<script setup lang="ts">` + Tailwind CSS 3.4 + `lucide-vue-next`。

## 2. Prettier & ESLint Tooling Design

### 2.1 Dependencies (`web/package.json`)
- `prettier`
- `eslint` (v9 Flat Config: `web/eslint.config.js`)
- `@eslint/js`
- `typescript-eslint`
- `eslint-plugin-vue`
- `eslint-config-prettier`
- `globals`

Scripts added to `web/package.json`:
- `"lint": "eslint ."`
- `"lint:fix": "eslint . --fix"`
- `"format": "prettier --write \"src/**/*.{vue,ts,js,css}\" index.html"`
- `"format:check": "prettier --check \"src/**/*.{vue,ts,js,css}\" index.html"`

### 2.2 Prettier Configuration (`web/.prettierrc.json` & `web/.prettierignore`)
对齐项目既有主流风格（42/44 Vue 文件 `<template>` 在前，17/20 `.ts` 文件无分号）：
- `semi: false`
- `singleQuote: true`
- `tabWidth: 2`
- `trailingComma: "es5"`
- `printWidth: 100`
- `endOfLine: "lf"`
- `vueIndentScriptAndStyle: false`
- `.prettierignore` 忽略 `dist`、`node_modules`。

### 2.3 ESLint Flat Config (`web/eslint.config.js`)
- 忽略 `dist/`、`node_modules/`。
- 继承 `js.configs.recommended`、`...tseslint.configs.recommended`、`...pluginVue.configs['flat/essential']`、以及末尾 `eslintConfigPrettier`。
- 为 `.vue` 文件配置 `parserOptions.parser: tseslint.parser`。
- 关键规则调优（避免存量代码噪音并守住真实缺陷）：
  - `vue/block-order`: `['error', { order: ['template', 'script', 'style'] }]`
  - `vue/multi-word-component-names`: `'off'`（适配 `Modal.vue`、`Drawer.vue`、`Input.vue`、`Button.vue`、`Table.vue` 等原子组件名）
  - `vue/html-button-has-type`: `'warn'` 或 `'error'`（配合 R3 为非提交按钮补齐 `type="button"`）
  - `@typescript-eslint/no-explicit-any`: `'off'`（尊重 `tsconfig.json` `strict: false` 现状）
  - `@typescript-eslint/no-unused-vars`: `['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }]`（与 `tsconfig` 的 `_` 占位约定一致）
  - `no-undef`: `'off'`（TS 文件由 `vue-tsc` 负责类型与全局校验）

## 3. Conservative Web-Guidance Enhancements (`modern-web-guidance` Mapped)

依据本次检索到的 `accessibility`、`forms`、`autofill-sign-in-form`、`customize-scrollbar-color-and-thickness` 指南，采用**不改浮层 DOM 骨架**的增强方案：

1. **`web/src/components/ui/Modal.vue` & `Drawer.vue`**
   - 保留 `Teleport to="body"` + `<transition>` 遮罩/内容结构，不切换 `<dialog>` / `showModal()`。
   - 使用 Vue 3 唯一 ID（`const titleId = \`modal-title-${Math.random()...}\`` 或自定义递增计数器，注意：项目 Vue 为 `3.4.21`，`useId()` 是 Vue 3.5+ 新增 API，**不可使用 `useId()`**！改用模块级递增 ID 生成器，如 `let uid = 0; const titleId = \`modal-title-${++uid}\``）。
   - 对话框主面板添加 `role="dialog"`、`aria-modal="true"`、`:aria-labelledby="title ? titleId : undefined"` 与 `tabindex="-1"`。
   - 标题 `<h3>` 绑定 `:id="titleId"`；关闭 `<button type="button">` 补齐 `aria-label="关闭"`（保留 `title="关闭 (Esc)"` 或精简为 `aria-label="关闭"`）。
   - 打开时通过 `nextTick` 记录触发焦点元素 `previousActiveElement`，将焦点移至面板容器（`panelRef.value?.focus()`）；关闭/卸载时恢复焦点至原触发元素（避免焦点丢失）。
   - 保留既有 Footer 安全区 `pb-[max(...,env(safe-area-inset-bottom))]`。

2. **`web/src/components/ui/FormField.vue` & `Input.vue`**
   - `FormField.vue` 使用模块递增 ID 生成默认 `fieldId` / `hintId` / `errorId`（兼容 Vue 3.4），支持显式传入 `forId` prop 或通过默认插槽/作用域传递（或保持向后兼容：`for` 指向 `inputId`，`FormField` 可同时暴露 slot props `{ id: inputId, describedBy, invalid }` 供复杂控件绑定）。
   - `Input.vue` 增加 `defineOptions({ inheritAttrs: true })` 或显式支持 `id`、`name`、`autocomplete`、`inputmode`、`aria-describedby`、`aria-invalid` 等属性透传到内部 `<input>`（当前 `<div class="relative ..."><input .../></div>` 默认将 attrs 落到外层 `<div>`！**关键点**：必须用 `defineOptions({ inheritAttrs: false })` + `v-bind="$attrs"` 绑定到 `<input>`，同时保留 `class` prop 单独控制 `<input>` 样式！）。

3. **`web/src/components/ToastContainer.vue`**
   - 外层容器增加 `aria-live="polite"`、`aria-atomic="false"`；每条 Toast 根据类型设 `role="alert"`（error）或 `role="status"`（info/success/warning）。
   - 关闭按钮增加 `type="button"` 与 `aria-label="关闭提示"`。

4. **`web/src/views/LoginView.vue` & `PortalClaimView.vue` & `App.vue`**
   - `LoginView.vue` / `PortalClaimView.vue` 主体包裹 `<main>` 语义地标；`<label for="...">` 绑定对应 `id`。
   - 用户名输入框：`id="login-username"`、`name="username"`、`autocomplete="username"`、`autocorrect="off"`、`autocapitalize="none"`、`spellcheck="false"`、`enterkeyhint="next"`。
   - 密码输入框：`id="login-password"`、`name="password"`、`autocomplete="current-password"`、`enterkeyhint="done"`；密码显隐按钮补全 `:aria-pressed="showPassword"` 与 `:aria-label="showPassword ? '隐藏密码' : '显示密码'"`。
   - 2FA 动态码输入框：`id="login-totp"`、`name="passcode"`、`inputmode="numeric"`、`pattern="\\d{6}"`、`autocomplete="one-time-code"`、`autocorrect="off"`、`autocapitalize="none"`、`spellcheck="false"`、`enterkeyhint="done"`。
   - `App.vue`：主内容区域确保使用 `<main>` 地标，侧边栏/移动端导航抽屉按钮补齐 `type="button"` 与 `aria-label`。
   - 全站 `<button>` 检查：所有非表单提交用途按钮补齐 `type="button"`，防止表单内误触发提交。

5. **`web/src/style.css`**
   - 滚动条渐进增强：在 `:root` / `html` 设定 `--scrollbar-thumb: rgba(255, 255, 255, 0.14); --scrollbar-track: transparent;`，全局应用 `scrollbar-width: thin; scrollbar-color: var(--scrollbar-thumb) var(--scrollbar-track);`，并将 `::-webkit-scrollbar` 规则包裹在 `@supports not (scrollbar-color: auto)` 中。
   - 补充 `@media (prefers-reduced-motion: reduce)` 降低 `.pulse-green` 与过渡动画动效。

## 4. UI Copy & Terminology Standardization Rules

遵循**克制中文控制台风格 + 保留中性防探测外壳**：
1. **外壳与伪装标题保留**：
   - 保留 `web/index.html`、`web/src/router/index.ts` 中的 `System Management Console`、`分布式网管接入点`；保留 `App.vue` 的 `XRAY PANEL`；保留 `PortalClaimView.vue` 的 `NetOps` 风格标识。
   - 移除同一控件/徽标内冗余英文后缀（如 `LoginView.vue` `'系统服务已就绪 · Ready'` → `'系统服务已就绪'`）。
2. **克制语气与标点**：
   - Toast 与状态文案去除感叹号 `！`（如 `'已恢复初始状态！'` → `'已恢复初始状态'`；`'Xray 核心已成功重启！'` → `'Xray 核心已重启'`）。
   - 去除冗余“已成功/成功”（如 `'出站配置已保存成功！'` → `'出站配置已保存'`；`'已成功批量删除选中用户！'` → `'已批量删除选中用户'`）。
   - 冒号统一全角中文冒号 `：`（如 `'保存失败: '` → `'保存失败：'`，`'重启失败: '` → `'重启失败：'`）。
3. **中英文排版与术语统一**：
   - 中文与英文/数字间保留半角空格（如 `'6 位动态码'`、`'Xray 核心'`、`'2FA 双因素认证'`、`'GeoData 规则库'`）。
   - 协议与专有名词统一：`REALITY`、`VLESS`、`VMess`、`Trojan`、`XTLS Vision`、`TLS`、`DNS`、`2FA`、`Telegram`、`Mux`。
4. **红线保护**：
   - 不改写任何协议能力边界语义（“面板当前管理策略暂未开放”等必须保持准确，不改为“Xray 不支持”）；不改动 `freedom.settings.domainStrategy` / `UseIPv4v6` / `UseIPv6v4` 相关枚举文案逻辑。

## 5. Compatibility, Risks & Rollback

- **Vue 3.4 兼容风险**：`web/package.json` 锁定 `vue: ^3.4.21`，不可使用 Vue 3.5+ 的 `useId()`；设计中统一采用模块级自增 ID，确保 `vue-tsc` 和运行期零报错。
- **Prettier/ESLint 批量格式化 diff 风险**：先单独完成 Batch 1（工具链配置与纯格式对齐）并验证 `typecheck` + `build`，再进入 Batch 2（语义增强）与 Batch 3（文案统一），便于审阅与按批次回滚。
- **回滚策略**：每个批次前确认工作区干净或记录阶段性检查点；若某项修改导致 `typecheck` / `build` 或浏览器手工回归异常，可精确回退对应文件。
