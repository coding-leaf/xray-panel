# Directory Structure & State

> `web/src/` 的真实目录分工、状态存放方式与命名约定

---

## 目录拓扑

```
web/src/
├── views/        # 12 个页面级 SFC，命名 *View.vue (PascalCase)
├── components/   # 共享组件；当前仅 ToastContainer.vue
├── api/          # index.ts (axios 单例) + reality.ts (类型化领域接口)
├── router/       # index.ts (单文件路由表)
├── mock/         # index.ts (请求分发器) + storage.ts (localStorage 状态)
├── utils/        # 通用工具；当前仅 toast.ts
├── App.vue       # 布局骨架 + 全局轮询/可见性处理
├── main.ts       # createApp + createPinia 注册
├── style.css     # Tailwind 指令 + 全局设计类
└── vite-env.d.ts # Vite 客户端引用 + *.vue shim
```

- **不存在** `stores/` / `composables/` / `types/` / `assets/` 目录；新增文件前先确认是否真的需要新开目录。
- 页面按规模单文件承载（最大 `views/InboundsView.vue` 约 1600 行），
  模板与 `<script setup>` 同处一文件，**views 不写 `<style>` 块**。

---

## 状态存放方式（写实）

当前**没有 Pinia store**，状态按以下三类存放：

1. **组件局部状态**：页面内 `ref` / `computed`，是绝大多数状态的做法
   （例：`web/src/views/InboundsView.vue:925-995`）。
2. **模块级单例**：跨组件共享的横切状态用模块顶层 `ref` 导出，
   现有唯一案例是 toast（`web/src/utils/toast.ts:12`）。
3. **持久化状态**：认证态存于 `localStorage` 的 `token` / `username`
   （`web/src/router/index.ts:39-42`、`api/index.ts:10,21`、
   `LoginView.vue:235-236`、`App.vue:342,443-444`）。

> **规范**：默认沿用上述三类方式，**不要仅为「规范化」而引入 Pinia Store**。
> 若确有必要，应在任务 PRD 中说明痛点并单独评审。

- 领域类型就近定义：随用随在组件内声明 `interface`
  （例：`web/src/views/InboundsView.vue:916-923`），不集中到 `types/`。

---

## 命名约定

- 组件/页面文件使用 PascalCase；页面统一以 `View.vue` 结尾（`DashboardView.vue`、`UsersView.vue`）。
- 工具与 API 模块使用小写：`index.ts`、`reality.ts`、`toast.ts`。
- 导入使用**相对路径**（`'../api'`、`'../utils/toast'`）。
  虽然 `web/tsconfig.json:16-18` 与 `vite.config.ts:8-12` 配置了 `@/* → src/*` 别名，
  但源码中**从未使用**；新增代码保持相对路径以与现状一致。
