# Implementation Plan: Frontend Copy, Formatting, and Web-Guidance Cleanup

## Execution Strategy

任务拆分为 3 个顺序执行的增量批次，确保“先工具链与纯格式、再语义增强、最后文案收敛”，减少混合 diff：

### Batch 1: Prettier + ESLint 工具链引入与纯格式对齐
- [x] 在 `web/` 安装开发依赖：`prettier`、`eslint`、`@eslint/js`、`typescript-eslint`、`eslint-plugin-vue`、`eslint-config-prettier`、`globals`（仅 `devDependencies`）。
- [x] 更新 `web/package.json` 脚本：`lint`、`lint:fix`、`format`、`format:check`。
- [x] 新增 `web/.prettierrc.json`、`web/.prettierignore`、`web/eslint.config.js`。
- [x] 调整 `FormField.vue`、`SectionCard.vue` 的 SFC 块顺序为 `<template>` → `<script setup lang="ts">`。
- [x] 运行 `npm run format` 与 `npm run lint:fix`（并在必要时手工修复残留 ESLint 规则问题，如 `vue/html-button-has-type` 等）。
- [x] **Batch 1 验证门禁**：`cd web && mise x -- npm run format:check && mise x -- npm run lint && mise x -- npm run typecheck && mise x -- npm run build`。

### Batch 2: `modern-web-guidance` 保守语义增强
- [x] `web/src/components/ui/Modal.vue` & `Drawer.vue`：补齐 `role="dialog"`、`aria-modal="true"`、`aria-labelledby`、唯一标题 ID（Vue 3.4 自增 ID）、关闭按钮 `type="button"` + `aria-label="关闭"`、开/关焦点管理与恢复（保留 `Teleport + Transition` 骨架及 Footer 安全区）。
- [x] `web/src/components/ui/FormField.vue` & `Input.vue`：`FormField` 补齐 `for`/slot props 关联能力；`Input.vue` 修复 `inheritAttrs: false` 透传 `id`/`name`/`autocomplete`/`inputmode`/`aria-*` 到内部 `<input>`。
- [x] `web/src/components/ToastContainer.vue`：补充 `aria-live="polite"`、`role="status"` / `role="alert"`、关闭按钮 `type="button"` + `aria-label`。
- [x] `web/src/views/LoginView.vue`、`PortalClaimView.vue`、`App.vue`：补全 `<main>` 地标、`<label for>`、`autocomplete`、`inputmode="numeric"`、`autocorrect="off"`、`autocapitalize="none"`、`spellcheck="false"`、密码切换 `aria-pressed` + `aria-label`、全站非提交按钮 `type="button"` 巡检。
- [x] `web/src/style.css`：引入标准 `scrollbar-width` / `scrollbar-color` + `@supports not (scrollbar-color: auto)` 回退 + `@media (prefers-reduced-motion: reduce)`。
- [x] **Batch 2 验证门禁**：`cd web && mise x -- npm run format:check && mise x -- npm run lint && mise x -- npm run typecheck && mise x -- npm run build`。

### Batch 3: 全站 UI 文案克制化统一与最终全量门禁
- [x] 逐文件排查并统一 `web/src/` 下所有视图、组件与 composable 的用户可见文案：
  - 保留防探测外壳标题（`System Management Console`、`XRAY PANEL`、`分布式网管接入点`）；
  - 清理冗余英文后缀（如 `· Ready`）；
  - Toast 与状态反馈去感叹号 `！`、去冗余“已成功”、冒号统一全角 `：`；
  - 统一中英文/数字空格与专有名词写法（`REALITY`、`Xray 核心`、`2FA` 等），严守协议文案红线。
- [x] 对照 `web/src/` 运行最终全量门禁：
  - `cd web && mise x -- npm run format:check`
  - `cd web && mise x -- npm run lint`
  - `cd web && mise x -- npm run typecheck`
  - `cd web && mise x -- npm run build`
  - `mise x -- go test ./...`
  - `mise x -- go vet ./...`
  - `mise x -- go build .`
- [x] 记录手工验证指引清单（登录、Modal/Drawer Esc 与焦点恢复、移动端安全区、Toast 提示、协议配置表单）。

## Rollback Points
- Batch 1 完成后即具备稳定格式基线；若工具配置不适配某些存量代码，可在 `web/eslint.config.js` 调整对应规则而不改业务逻辑。
- Batch 2 仅涉及无障碍属性、表单透传与 CSS 渐进增强，每个组件可独立回退。
- Batch 3 仅改文案字符串，不涉及逻辑分支与接口字段。
