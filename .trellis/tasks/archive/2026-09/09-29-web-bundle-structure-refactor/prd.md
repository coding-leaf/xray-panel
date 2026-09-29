# PRD — 前端包体结构优化与巨石视图分层重构

## 1. 目标与背景 (Background & Goals)

当前 `web/src/views/` 下存在多个 50KB+ 的单体巨石视图（例如 `InboundsView.vue` 1973行约 80KB，`UsersView.vue` 1951行约 78KB），代码高度集中、耦合了大量异构协议表单、抽屉组件与重复工具函数。此外存在以下痛点：
1. **模板与样式类深度严重膨胀**：每个表单字段手写 4~5 层重复的 `<div class="..."><label class="text-[11px] font-mono...">...<Input/><p class="text-[10px]...">`，在全项目中重复百次以上；
2. **缺乏共用服务与工具抽象**：`formatBytes`、`formatDate`、`copyText` 等在各视图内各自复制实现；
3. **参数冗余与无意义参数堆叠**：表单数据结构混合了多协议几十个无关字段（如 Socks 入站携带 Reality 证书、Fallbacks 等无意义字段），缺乏清洗与判别类型；
4. **包体与 Chunk 切片粗糙**：Vite manualChunks 将基础 UI 切碎为微小 Chunk（<2KB），而重型表单直接打包在所属路由的主 chunk 中，导致首屏加载较重。

本任务旨在进行**全链路分层重构**：提炼通用 UI 容器、领域工具类与 Composable，按业务特性就近拆分巨石视图与抽屉组件，重构表单参数模型，并通过重型抽屉异步懒加载与 Vite 构建分包优化，提升可维护性与加载性能。

## 2. 核心功能与需求规范 (Functional Requirements)

### 2.1 通用 UI 容器抽象 (UI Primitives)
- **`FormField.vue`**：封装 Label、可选标签、Help Text、Error 提示及 Input 容器槽位，标准化统一的表单字段排版，收缩上层 DOM 与 Tailwind class 深度；
- **`SectionCard.vue`**：封装分组卡片容器样式（圆角边框、标题、序号徽章、内边距与网格），减少巨石表单手写多层重复 card 容器。

### 2.2 通用领域服务与工具类抽象 (Shared Utils & Composables)
- **`utils/format.ts`**：集中提取 `formatBytes` (字节速率与流量单位自适应)、`formatDate` / `formatDateTime`、`formatDuration`；
- **`utils/clipboard.ts`**：统一实现 `copyText` 安全剪贴板复制逻辑并集成 toast 提示，消除散落各处的重复逻辑。

### 2.3 巨石视图拆分与就近聚合 (Domain Decomposition)
- 遵循“分步演进与黄金样本先行”策略，首期彻底拆解最庞大的黄金样本 **`InboundsView.vue`**：
  - 新建 `web/src/views/inbounds/` 目录结构；
  - 拆分 **`components/InboundTable.vue`**：承载节点列表渲染与行级快捷操作；
  - 拆分 **`components/InboundDetailDrawer.vue`**：承载节点详细参数审查、Reality 状态检测与订阅凭据信息；
  - 拆分 **`components/InboundFormDrawer.vue`**：承载新增/编辑入站的核心多协议表单，并通过 `defineAsyncComponent` 异步按需加载；
  - 抽离 **`composables/useInboundList.ts`** 与 **`types.ts`**：收敛入站列表过滤、搜索、删除及状态更新。

### 2.4 参数治理与无意义参数裁剪 (Parameter & DTO Sanitization)
- 定义强类型 `InboundFormPayload`，按协议类型 (`protocol`) 与流控特性收敛表单参数；
- 在表单提交至 API 前实现 `sanitizeInboundPayload` 清洗函数：自动裁剪当前协议不适用的无关字段（如非 Reality 节点剔除私钥公钥目标、Socks 节点剔除 TLS/xHTTP 参数），杜绝脏数据与无意义字段入库。

### 2.5 Vite 构建与 Chunk 优化 (Bundle Optimization)
- 在 `web/vite.config.ts` 中优化 `manualChunks`：
  - 将基础 UI 容器组件（Button/Input/Drawer/FormField/SectionCard 等）合并为 `ui-primitives`，避免微小 Chunk 碎片；
  - 保持 vendor 类库分类清晰（`vendor-vue`, `vendor-icons`, `vendor-qrcode`, `vendor-utils`）；
  - 验证重型抽屉组件在产物中按需独立拆块。

## 3. 非功能约束与质量红线 (Non-Functional Requirements & Gates)

1. **零业务行为破坏**：重构前后入站网关的新增、编辑、删除、Reality 巡检、分流子路由等功能必须保持 100% 行为一致；
2. **零类型与构建报错**：必须通过 `npm run typecheck` (`vue-tsc --noEmit`) 与 `npm run build`；
3. **后端嵌入二进制门禁**：必须通过 `go test ./...`、`go vet ./...` 与 `go build .`，确保静态资源嵌入正常。

## 4. 验收标准 (Acceptance Criteria)

- [ ] `FormField.vue` 和 `SectionCard.vue` 建立并能正常渲染；
- [ ] `utils/format.ts` 与 `utils/clipboard.ts` 集中提供服务并替换原重复代码；
- [ ] `InboundsView.vue` 拆分为就近的 Table、DetailDrawer、FormDrawer 组件，且原文件行数从 1973 行缩减至 300 行以内的轻量调度器；
- [ ] FormDrawer 采用 `defineAsyncComponent` 懒加载，Vite 打包产物正常分割；
- [ ] 提交入站配置时，无关协议参数被有效清洗剔除；
- [ ] 终端门禁全面通过（`npm run typecheck`、`npm run build`、`go vet ./...`、`go test ./...`）。
