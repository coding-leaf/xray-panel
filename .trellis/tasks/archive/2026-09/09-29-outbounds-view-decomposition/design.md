# Design: 拆解 OutboundsView 巨石视图与组件化重构

## 1. 架构目标与分层职责

将 1222 行的 `OutboundsView.vue` 巨石视图解耦，对齐 Inbounds 和 Users 已经落地的设计规范，建立 `web/src/views/outbounds/` 目录结构：

```
web/src/views/
├── OutboundsView.vue                  # 顶层调度器 (< 150 行)
└── outbounds/
    ├── types.ts                       # 出站领域模型、表单结构体与纯函数序列化/解析器
    └── components/
        ├── OutboundTable.vue          # 主表格：搜索、协议过滤、列表渲染与操作分发
        ├── OutboundDetailDrawer.vue   # 巡检抽屉：结构化协议参数与只读 JSON 检视
        └── OutboundFormDrawer.vue     # 表单抽屉：全协议配置表单 (异步按需懒加载)
```

## 2. 详细接口与契约设计

### 2.1 领域模型与清洗 (`types.ts`)
- `OutboundItem`: 对应后端 `/api/outbounds` 返回的数据结构（`tag`, `protocol`, `settingsJson`, `streamSettings`）。
- `OutboundFormState`: 抽离出站表单的所有响应式字段。
- 纯函数工具库：
  - `createDefaultOutboundForm()`: 构造干净的初始表单对象。
  - `populateOutboundForm(ob: OutboundItem)`: 将后端实体安全解包至表单对象，杜绝浅拷贝污染。
  - `buildSettingsJSON(form: OutboundFormState)`: 协议 settings JSON 构建器。
  - `buildStreamSettingsJSON(form: OutboundFormState)`: streamSettings 构建器。
  - `sanitizeOutboundPayload(form: OutboundFormState)`: 构造提交给 `/api/outbounds` 的清洁 Payload。
  - `getTargetEndpoint(ob: OutboundItem)`、`getProtocolBadgeVariant(proto)`、`getUsageDesc(ob)`、`formatSettingsSummary(ob)`。

### 2.2 视图与子组件通信契约

- **`OutboundTable.vue`**:
  - Props: `outbounds: OutboundItem[]`, `selectedTag?: string`
  - Emits: `inspect(ob)`, `edit(ob)`, `delete(tag)`, `create`
  - 维护内部 `searchQuery` 与 `protocolFilter`，计算 `filteredOutbounds`。

- **`OutboundDetailDrawer.vue`**:
  - Props: `modelValue: boolean`, `outbound: OutboundItem | null`
  - Emits: `update:modelValue`, `edit(ob)`, `delete(tag)`

- **`OutboundFormDrawer.vue`**:
  - Props: `modelValue: boolean`, `editingOutbound: OutboundItem | null`, `saving: boolean`
  - Emits: `update:modelValue`, `save(payload: OutboundPayload)`
  - 整合 `FormField` 与 `SectionCard`，结构清晰。

- **`OutboundsView.vue`**:
  - `const OutboundFormDrawer = defineAsyncComponent(() => import('./outbounds/components/OutboundFormDrawer.vue'))` 保证首屏 chunk 体积最小。
  - 统一持有 API 数据生命周期与路由 query 参数同步逻辑。

## 3. 兼容性与安全防线
- 保证自由直连（freedom）、黑洞（blackhole）、WARP（wireguard）、VLESS、VMess、Trojan、Shadowsocks、Socks、HTTP 等现有协议配置 100% 字段行为与序列化兼容；
- 严禁对系统默认保留节点（`direct`, `block`）执行删除操作；
- 所有 TypeScript 类型必须明确，符合 `noUnusedLocals` / `noUnusedParameters` 规则，无 `any` 漏检。
