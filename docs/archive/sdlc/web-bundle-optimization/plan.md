# Plan: 前端包体与打包体积优化 - 实施计划

- **关联 Spec**: web-bundle-optimization
- **实施执行人 / Agent**: Builder
- **当前状态**: Ready for Execution
- **变更定级**: Tier 2 (局部构建工程与展示层演进)

---

## 1. 变更文件清单 (Files that change)
* `web/src/router/index.ts` (Modify) - 12 个视图组件改用动态 `() => import(...)` 懒加载，保留导航守卫逻辑
* `web/vite.config.ts` (Modify) - 配置 `build.rollupOptions.output.manualChunks` 细粒度分包与文件命名策略

---

## 2. 伴随式分步实施与验证 (Step-by-Step Implementation Loops)

### Milestone 1: 路由组件按需懒加载重构 (Dynamic Import)
* **操作目标**: 重构 `web/src/router/index.ts`，将所有 12 个页面视图组件由同步静态导入转换为标准 ES 动态导入 `() => import('../views/XxxView.vue')`，维持 `beforeEach` 鉴权守卫与 `afterEach` 标题设置原样不变。
* **涉及文件**: `web/src/router/index.ts`
* **局部验证命令**: `cd web && npm run typecheck`
* **预期判据**: TypeScript/Vue 类型检查 0 错误（`vue-tsc --noEmit` 绿灯），动态导入组件类型与 `RouteRecordRaw` 契约完全吻合。
* **风险缓解**: 统一在路由表外部定义组件加载常量；保持原有相对路径绝对一致，防止路径拼写失误；保留 `routes` 结构与路由元信息 (`meta`) 不变。

### Milestone 2: Rollup manualChunks 细粒度分包配置
* **操作目标**: 在 `web/vite.config.ts` 中配置 `build.rollupOptions.output.manualChunks`，按模块特征划分为 `vendor-vue`、`vendor-icons`、`vendor-qrcode`、`vendor-utils` 与 `vendor-libs`，配置规范化产物哈希命名及 `chunkSizeWarningLimit: 600`。
* **涉及文件**: `web/vite.config.ts`
* **局部验证命令**: `cd web && npm run build`
* **预期判据**: 打包成功，`dist/assets/js/` 产物中显式生成分离的 vendor chunks 与各 view chunks，控制台消除超大分包告警。
* **风险缓解**: 分包判断前置校验 `id.includes('node_modules')`，防止业务代码被意外分离；依赖分组仅聚合独立无交叉环形依赖的第三方库。

### Milestone 3: 端到端全链路构建与质检验证
* **操作目标**: 执行前端类型与生产构建双重检查，并运行后端竞态测试与全量构建，确保前后端集成与生产产物完整可用。
* **涉及文件**: 无（验证执行）
* **端到端验证命令**:
  1. 前端类型检查: `cd web && npm run typecheck`
  2. 前端生产构建: `cd web && npm run build`
  3. 后端自动化竞态测试: `go test -race ./...`
  4. 后端二进制构建: `go build .`
* **预期判据**:
  - `vue-tsc` 0 警告 0 报错；
  - 前端生成完整 `dist/`，产物结构符合 spec 契约要求；
  - 后端全部单测与集成测试通过，`-race` 检测零数据竞态；
  - 后端主程序编译成功无错误。

---

## 3. 全局质量门禁核验 (Global Quality Gate)
* **代码风格与静态检查**: `go vet ./...` (后端) + `cd web && npm run typecheck` (前端)
* **类型与契约安全校验**: `cd web && npm run typecheck && npm run build`
* **全量相关测试回归**: `go test -race ./...`
* **核验结果**: [待执行核验]

---

## 4. 实施偏差记录 (Deviations Log)
* [待执行过程记录 / 遵循无越权变更原则]

---

## 5. 阶段准出签批 (Gate 3 Sign-off)
- [ ] 所有分步实施项与验证断言均已就地执行并通过
- [ ] 全局质量门禁（Lint / Type / Regression）全部绿灯
- [ ] 变更文件与 plan.md 清单完全吻合，无越权修改
- **验收结论**: Pending
- **验证人 / 日期**: [待人类签批] / 2026-09-20