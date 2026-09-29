# PRD — UsersView 巨石视图拆解与 Modal 原语沉淀

## 1. 目标与背景 (Background & Goals)

当前 `web/src/views/UsersView.vue` 是前端项目中体积最大的单体巨石视图（1935 行，约 78KB）。它同时耦合了主表格、批量操作栏（Batch Bar）、用户详情巡检抽屉（Inspector Drawer）、新增/编辑表单抽屉（Form Drawer）、二维码凭据分享弹窗（Share Modal）以及历史流量趋势弹窗（History Modal）。
同时存在以下主要问题：
1. **弹窗手写类深度膨胀**：页面内直接手写多层 `fixed inset-0 z-50 flex items-center justify-center bg-black/75`，缺少通用的模态弹窗基础原语；
2. **重型依赖与首屏 Chunk 耦合**：`qrcode.vue` 与多协议订阅解析逻辑直接打入 UsersView 主 chunk，导致该路由首屏体积沉重（>53KB）；
3. **无意义参数与脏数据传递**：表单保存直接浅拷贝展开 `existingUser`，将大量只读统计字段和监控状态全量提交给后端；
4. **组件层级与复杂度过高**：缺少分层，状态与 DOM 结构交织难以维护。

本任务旨在建立通用基础原语 `components/ui/Modal.vue`，按业务特性就近拆解 `UsersView`，抽离订阅服务类与 Composable，实现重型抽屉与弹窗的异步按需懒加载，并严格在刚启用的 `noUnusedLocals`/`noUnusedParameters` 代码卫生约束下达成全量门禁。

## 2. 核心功能与需求规范 (Functional Requirements)

### 2.1 通用模态弹窗原语 (`components/ui/Modal.vue`)
- 镜像对齐 `Drawer.vue` 设计风格与交互规范；
- 基于 `Teleport to body`，支持暗色背景遮罩淡入淡出与内容面板居中微缩放过渡；
- 支持 `title`、`description` 与 `size`（sm、md、lg、xl、2xl）配置；
- 提供默认内容插槽、头部 `#header` 与底部 `#footer` 操作插槽；
- 内置 `Esc` 键与遮罩点击关闭能力，支持生命周期成对解绑；
- 自动合并至 `vite.config.ts` 中的 `ui-primitives` 分包。

### 2.2 领域服务类与工具抽离 (`views/users/services/` & `composables/`)
- **`services/subscription.ts`**：提取订阅链接生成工具类 `UserSubscriptionService`，收敛 VLESS/VMess/Trojan/Shadowsocks 订阅协议生成与节点凭据解析；
- **`composables/useUserList.ts`**：收敛用户列表获取、KPI 指标计算、关键字与状态过滤、批量选择、批量启用/停用与流量重置。

### 2.3 巨石视图拆解与就近聚合 (`views/users/components/`)
- **`UserTable.vue`**：承载用户列表主表格、KPI 状态栏与批量浮动条 (Batch Bar)；
- **`UserDetailDrawer.vue`**：承载用户详情诊断、配额消耗进度、连接诊断与快捷状态切换；
- **`UserFormDrawer.vue`**：承载用户新增/编辑表单，使用 `FormField` 与 `SectionCard` 重构，采用 `defineAsyncComponent` 异步懒加载；
- **`UserShareModal.vue`**：承载二维码与多协议订阅链接展示，基于 `Modal.vue` 构建，采用 `defineAsyncComponent` 异步懒加载（按需引入 `qrcode.vue`）；
- **`UserTrafficModal.vue`**：承载历史流量趋势与 24 小时数据矩阵弹窗，基于 `Modal.vue` 构建，采用 `defineAsyncComponent` 异步懒加载。

### 2.4 参数治理与 DTO 清洗 (`views/users/types.ts`)
- 定义强类型 `UserItem`、`UserFormData` 与 `UserFormPayload`；
- 实现 `sanitizeUserPayload(form, isEditing, existingUser)` 纯函数：
  - 自动处理 GB 到 Bytes 的配额换算；
  - 统一 `extendDays` 累加逻辑与到期时间戳转换；
  - 仅提取后端更新所需的确切字段，杜绝展开包含统计与监控字段的脏数据。

### 2.5 宿主轻量化与包体分包
- `web/src/views/UsersView.vue` 收敛为纯调度器，代码行数缩减至 200 行以内；
- 首屏仅打包表格与核心状态，重型弹窗与抽屉独立分块，首屏 chunk 显著缩减。

## 3. 非功能约束与质量红线 (Quality Gates)

1. **零业务逻辑破坏**：用户增删改查、配额重置、批量操作、二维码生成、订阅下发与流量趋势 100% 行为一致；
2. **严格代码卫生门禁**：在 `noUnusedLocals: true` 和 `noUnusedParameters: true` 下，必须保证 `npm run typecheck` 零错误、零警告；
3. **构建与后端全绿**：`npm run build`、`go test ./...`、`go vet ./...`、`go build .` 必须全部通过。

## 4. 验收标准 (Acceptance Criteria)

- [ ] `components/ui/Modal.vue` 正常渲染并支持 Esc、遮罩点击、尺寸调节与插槽；
- [ ] `views/users/` 目录下完成 `types.ts`、`services/subscription.ts`、`composables/useUserList.ts` 以及 5 个子组件的拆分；
- [ ] `UsersView.vue` 宿主文件行数缩减至 200 行以内；
- [ ] `UserFormDrawer`、`UserShareModal`、`UserTrafficModal` 采用 `defineAsyncComponent` 异步按需加载；
- [ ] 保存用户时由 `sanitizeUserPayload` 清洗，剔除无意义监控属性；
- [ ] 终端静态与构建门禁全面 100% 通过（`typecheck`, `build`, `go test`, `go vet`, `go build`）。
