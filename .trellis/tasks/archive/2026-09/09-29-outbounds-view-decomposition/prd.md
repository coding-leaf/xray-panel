# PRD: 拆解 OutboundsView 巨石视图与组件化重构

## 1. 任务背景与问题分析
- 当前 `web/src/views/OutboundsView.vue` 达到 1154 行，是目前前端代码量最大的单文件巨石视图。
- 视图内部同时耦合了：出站列表渲染、协议类型与流控徽标、节点连通性测速状态、全协议表单编辑（VLESS / VMess / Trojan / Shadowsocks / WireGuard / Freedom / Blackhole / DNS 等）、复杂证书与传输层配置、详情抽屉。
- 表单抽屉与主列表强绑定，导致首屏直接加载全部协议表单依赖，首屏体积无法进一步精简。
- 前端向后端提交出站配置时缺乏纯函数数据清洗逻辑。

## 2. 目标与交付物
1. **沉淀出站领域服务与清洗函数**：
   - 创建 `web/src/views/outbounds/types.ts`；
   - 提取 `sanitizeOutboundPayload` 纯函数，根据选择的 protocol 清洗并剔除无关协议的脏字段。
2. **巨石拆解与分层架构**：
   - 顶层调度：`web/src/views/OutboundsView.vue` 精简为只负责事件分发、状态持有与 API 调用的调度器（目标行数 < 200 行）；
   - 主表格：`web/src/views/outbounds/components/OutboundTable.vue`，负责出站节点列表、协议过滤与操作触发；
   - 详情抽屉：`web/src/views/outbounds/components/OutboundDetailDrawer.vue`，负责节点健康度、实时状态与底层配置只读检视；
   - 表单抽屉：`web/src/views/outbounds/components/OutboundFormDrawer.vue`，负责全协议新建/编辑表单，通过 `defineAsyncComponent` 异步按需加载。
3. **UI 原语复用**：
   - 深度复用 `components/ui/FormField.vue`、`components/ui/SectionCard.vue`、`components/ui/Modal.vue`。
4. **TypeScript 严格约束**：
   - 遵循 `noUnusedLocals`、`noUnusedParameters`，确保 `npm run typecheck` 保持 0 警告 0 错误。

## 3. 验收标准
- [ ] `OutboundsView.vue` 行数 < 200 行；
- [ ] 出站列表、全协议新增/编辑、连通性测速、启用/禁用、删除功能 100% 行为保持一致；
- [ ] `OutboundFormDrawer` 采用异步懒加载，构建时独立打包；
- [ ] `cd web && npm run typecheck && npm run build` 100% 通过且无编译/打包告警；
- [ ] 后端 `go test ./...` 保持全绿。
