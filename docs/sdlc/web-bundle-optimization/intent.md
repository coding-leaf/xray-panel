# Intent: 前端包体与打包体积优化

- **任务编号**: web-bundle-optimization
- **提出人**: Dev
- **创建时间**: 2026-09-20 11:42
- **初始 Change Tier**: Tier 2
- **当前状态**: In-Review

---

## 1. 问题与现状背景 (Problem)
当前前端基于 Vite + Vue 3 + Tailwind CSS 构建。随着页面扩充和第三方库（如 lucide-vue-next 图标、Pinia、Vue Router、qrcode.vue 等）的引入，前端产物尚未配置精细的分块策略（Code Splitting）与资源优化，可能存在单一 Vendor Chunk 过大、路由组件未充分懒加载、首屏加载体积偏大等隐患。

## 2. 变更性质分类 (Change Archetype - 单选)
- [x] 局部结构精简 (Local Cleanup - 仅限模块内部冗余消除，不改数据流向与全局装配)
- [ ] 单模块特性演进 (Single-Module Feature - 单一模块业务增量或修复)
- [ ] 跨领域架构重构 (Cross-Domain Rewiring - 触及应用全局装配、生命周期或跨域流向，必须升 Tier 3)

## 3. 期望达成效果 (Proposed Outcome)
1. 配置 Vite/Rollup 细粒度代码拆分（manualChunks 分离 vendor 核心与业务逻辑）；
2. 确保前端路由全部遵循动态 import 懒加载；
3. 检查并优化第三方依赖导入方式（如图标按需 Tree-shaking）；
4. 缩减构建体积，消除 Vite 默认 500kB 单 chunk 超限告警；
5. `cd web && npm run build` 构建零报错，前后端静态资源打包运行无异常。

## 4. 波及工程分面 (Affected Architectural Layers)
- [ ] 核心领域与计算逻辑 (Domain & Core Business Logic)
- [ ] 外部接口与协议入口 (Public Ingress & Controllers & Protocols)
- [ ] 数据持久化与状态存储 (Database & Storage & Schemas)
- [ ] 全局装配与应用入口 (Bootstrap & Lifecycle & Service Wiring)
- 注：仅波及前端展示层与构建配置（`web/vite.config.ts`, `web/src/router` 等）。

## 5. 边界与硬性约束 (Constraints & Boundaries)
* **硬性技术制约**: 
  - 严禁破坏现有页面功能与交互体验；
  - 保持现有 Vue 3 / TS 类型体系完整无错 (`npm run typecheck` 必须全绿)；
  - 静态资源产物必须能被后端 `embed` 或标准静态文件服务正常加载。
* **明确非目标 (Non-Goals / Out-of-Scope)**: 
  - 不做前后端通信 API 契约改动；
  - 不重构前端 Pinia 状态管理架构或重写业务组件内部逻辑；
  - 不涉及后端 Go 代码改动。
* **完成判定条件 (Definition of Done)**:
  - 具备清晰合理的 Vendor / Route 分块；
  - `cd web && npm run build` 构建成功且零 warning / 零 error；
  - 通过 `go test -race ./...` 与后端集成验证。

## 6. 未决疑问与待探讨点 (Open Questions)
- 产物是否需要引入 vite-plugin-compression 进行 gzip/brotli 预压缩（优先仅做 Rollup chunks 拆分与树摇，按需考虑预压缩插件）。

---

## 7. 阶段准出签批 (Gate 1 Sign-off)
- [ ] 场景与问题已客观复现并达成共识
- [ ] 边界、非目标与约束清晰明确
- [ ] 初始 Change Tier 评定合理
- **准出结论**: Pending
- **签批人 / 日期**: [待人类签批] / 2026-09-20 11:42
