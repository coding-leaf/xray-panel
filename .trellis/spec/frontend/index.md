# Frontend Development Guidelines - xray-panel

> Vue 3 + Tailwind CSS + Vite 前端开发、数据流与构建规范

本目录描述 **`web/` 当前真实采用的约定**。前端架构刻意保持扁平，未引入 Store、
composable 或组件库；新增代码应对齐既有形态，而非套用通用最佳实践。

---

## 技术栈

- **框架**：Vue 3（全部 SFC 使用 `<script setup lang="ts">`）；
- **路由**：vue-router 4（单文件路由表 + 懒加载）；
- **HTTP**：axios 单例封装（`web/src/api/index.ts`）；
- **样式**：Tailwind CSS 3 + 全局设计类（`web/src/style.css`）；
- **图标**：`lucide-vue-next`；二维码：`qrcode.vue`；
- **构建**：Vite 5；类型检查 `vue-tsc --noEmit`（`strict: false`）。

> **Pinia 说明**：依赖已在 `web/src/main.ts:2,8` 注册，但**全仓没有任何 store**
> （`defineStore` 零匹配）。当前状态一律使用组件内 `ref`/`computed` 与模块级单例，
> 详见 [Directory Structure](./directory-structure.md) 与 [Components & Routing](./components-and-routing.md)。

---

## 核心规范文件索引

| 规约文档 | 覆盖范围与说明 | 状态 |
|---------|----------------|------|
| [Directory Structure](./directory-structure.md) | `src/` 真实目录、状态存放方式与命名约定 | Active |
| [API & Dataflow](./api-and-dataflow.md) | axios 单例、拦截器、Bearer、401、mock 分流与错误处理 | Active |
| [Components & Routing](./components-and-routing.md) | SFC 约定、路由表与守卫、样式与图标体系 | Active |
| [Quality Guidelines](./quality-guidelines.md) | 构建/类型门禁、无测试现状与安全防线 | Active |

---

## 交付与嵌入

- 构建产物输出至 `web/dist`；
- 根目录 `embedded.go` 通过 `//go:embed all:web/dist` 将前端静态资源打包进单一二进制产物；
- 交付前必须保证 `cd web && mise x -- npm run build` 零报错。

---

## 安全自毁与防探测

- 提取凭据（Portal）等敏感页面必须支持超时锁屏物理自毁；
- 禁止在 `LocalStorage` / `SessionStorage` 中长久保留明文提取码。
