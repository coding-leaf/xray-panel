# Design — 前端包体结构优化与巨石视图分层重构

## 1. 架构拓扑与目录组织

遵循“按业务特性就近聚合”规范，前端架构调整如下：

```
web/src/
├── components/
│   └── ui/
│       ├── Button.vue
│       ├── Input.vue
│       ├── Badge.vue
│       ├── Drawer.vue
│       ├── Table*.vue
│       ├── FormField.vue       <-- [新增] 统一样式类深度的表单字段组件
│       └── SectionCard.vue     <-- [新增] 统一样式类深度的区块卡片组件
├── utils/
│   ├── format.ts               <-- [新增] 流量/字节、时间日期通用格式化
│   ├── clipboard.ts            <-- [新增] 统一安全剪贴板复制工具
│   └── toast.ts
└── views/
    ├── inbounds/               <-- [新增] 黄金样本就近聚合目录
    │   ├── types.ts            <-- 领域类型与 DTO 定义
    │   ├── composables/
    │   │   └── useInboundList.ts <-- 列表获取、过滤筛选、批量操作
    │   └── components/
    │       ├── InboundTable.vue        <-- 列表主表格（同步加载）
    │       ├── InboundDetailDrawer.vue <-- 详情抽屉（可按需加载）
    │       └── InboundFormDrawer.vue   <-- 庞大多协议编辑表单抽屉（defineAsyncComponent 懒加载）
    ├── InboundsView.vue        <-- [重构] 顶层轻量调度器 (< 250 行)
    └── ...
```

---

## 2. 核心技术契约与实现规范

### 2.1 样式类深度优化 (FormField & SectionCard)

#### `FormField.vue`
```vue
<script setup lang="ts">
defineProps<{
  label?: string
  required?: boolean
  hint?: string
  error?: string
}>()
</script>

<template>
  <div class="space-y-1">
    <div v-if="label || $slots.label" class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
      <label class="flex items-center gap-1">
        <slot name="label">{{ label }}</slot>
        <span v-if="required" class="text-rose-400">*</span>
      </label>
      <slot name="extra" />
    </div>
    <div>
      <slot />
    </div>
    <p v-if="hint && !error" class="text-[10px] text-muted-foreground font-mono leading-tight">
      {{ hint }}
    </p>
    <p v-if="error" class="text-[10px] text-rose-400 font-mono leading-tight">
      {{ error }}
    </p>
  </div>
</template>
```

#### `SectionCard.vue`
```vue
<script setup lang="ts">
defineProps<{
  title?: string
  stepNumber?: number | string
}>()
</script>

<template>
  <div class="rounded-lg border border-border bg-card p-4 space-y-3">
    <div v-if="title || $slots.header" class="flex items-center justify-between border-b border-border/40 pb-2.5">
      <div class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider flex items-center gap-1.5">
        <span v-if="stepNumber" class="inline-flex items-center justify-center w-4 h-4 rounded-full bg-primary/20 text-primary text-[10px]">
          {{ stepNumber }}
        </span>
        <slot name="header">
          <span>{{ title }}</span>
        </slot>
      </div>
      <slot name="actions" />
    </div>
    <slot />
  </div>
</template>
```

### 2.2 通用领域服务类 (`utils/format.ts` & `utils/clipboard.ts`)

#### `utils/format.ts`
```ts
export function formatBytes(bytes: number, decimals = 2): string {
  if (bytes === 0) return '0 B'
  const k = 1024
  const dm = decimals < 0 ? 0 : decimals
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(dm))} ${sizes[i]}`
}

export function formatDate(timestamp: number | string | Date): string {
  if (!timestamp) return '-'
  const d = new Date(timestamp)
  return d.toLocaleString('zh-CN', { hour12: false })
}
```

#### `utils/clipboard.ts`
```ts
import { toast } from './toast'

export async function copyText(text: string, successMessage = '已复制到剪贴板'): Promise<boolean> {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = text
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.focus()
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    toast.success(successMessage)
    return true
  } catch (err) {
    toast.error('复制失败，请手动选中文本复制')
    return false
  }
}
```

### 2.3 参数裁剪与清洗契约 (Parameter Sanitization)

在 `views/inbounds/types.ts` 中实现纯函数 `sanitizeInboundPayload(raw: InboundFormData)`：
- 基础字段保活：`id`, `tag`, `listen`, `port`, `externalPort`, `externalHost`, `protocol`, `network`, `security`, `sniffingEnabled`, `sniffingRouteOnly`；
- 根据 `raw.protocol` 裁剪：
  - 若为 `vless`：保留 `vlessFlow`, `selectedUserEmails`, `subRoutes`；
  - 若为 `socks`：保留 `socksAuth`, `socksUdp`, `socksUsername`, `socksPassword`；
  - 若为 `http`：保留 `httpUsername`, `httpPassword`；
  - 若为 `dokodemo-door`：保留 `dokoAddress`, `dokoPort`, `dokoNetwork`；
- 根据 `raw.security` 裁剪：
  - 仅当 `security === 'reality'` 时保留 `realityTarget`, `realityServerNames`, `realityPrivateKey`, `realityPublicKey`, `realityShortIds`；
  - 仅当 `security === 'tls'` 时保留 `tlsServerName`, `tlsCertFile`, `tlsKeyFile`；
- 根据 `raw.network` 裁剪：
  - 仅当 `network === 'xhttp'` 时保留 `xhttpPath`, `xhttpMode`；
  - 仅当 `network === 'ws'` 时保留 `wsPath`；
  - 仅当 `network === 'grpc'` 时保留 `grpcService`。

### 2.4 构建分包配置 (`web/vite.config.ts`)

```ts
rollupOptions: {
  output: {
    manualChunks(id) {
      if (id.includes('node_modules')) {
        if (id.includes('lucide-vue-next')) return 'vendor-icons'
        if (id.includes('qrcode.vue')) return 'vendor-qrcode'
        if (id.includes('axios')) return 'vendor-utils'
        if (id.includes('vue') || id.includes('pinia')) return 'vendor-vue'
        return 'vendor-libs'
      }
      // 收敛基础 UI 组件，杜绝微小碎片 Chunk
      if (id.includes('src/components/ui/')) {
        return 'ui-primitives'
      }
    }
  }
}
```

---

## 3. 回滚与兼容策略

- 顶层保持 `web/src/views/InboundsView.vue` 入口路径与导出不变，`router/index.ts` 零破坏；
- 保持对外发送的 HTTP API Request Payload 兼容后端的字段格式（仅裁剪未启用的脏字段，不改变启用字段的命名与数据类型）。
