# Design — UsersView 巨石视图拆解与 Modal 原语沉淀

## 1. 架构拓扑与目录组织

```
web/src/
├── components/
│   └── ui/
│       ├── Modal.vue            <-- [新增] 通用模态弹窗原语 (对齐 Drawer)
│       └── ...
└── views/
    ├── users/                   <-- [新增] Users 模块就近子目录
    │   ├── types.ts             <-- 用户领域模型、表单契约与 sanitizeUserPayload
    │   ├── services/
    │   │   └── subscription.ts  <-- 多协议订阅链接与节点凭据解析服务类
    │   ├── composables/
    │   │   └── useUserList.ts   <-- 用户列表检索、KPI 指标、批量操作
    │   └── components/
    │       ├── UserTable.vue        <-- 列表主表格、KPI 卡片与批量操作条 (同步加载)
    │       ├── UserDetailDrawer.vue <-- 用户详情检查抽屉 (同步加载)
    │       ├── UserFormDrawer.vue   <-- 用户新增/编辑表单 (defineAsyncComponent 异步懒加载)
    │       ├── UserShareModal.vue   <-- 凭据与二维码弹窗 (defineAsyncComponent 异步懒加载，按需加载 qrcode)
    │       └── UserTrafficModal.vue <-- 流量趋势弹窗 (defineAsyncComponent 异步懒加载)
    ├── UsersView.vue            <-- [重构] 顶层轻量调度器 (< 200 行)
    └── ...
```

---

## 2. 核心技术契约与实现规范

### 2.1 通用模态弹窗组件 (`components/ui/Modal.vue`)

#### 契约与属性
```vue
<script setup lang="ts">
interface ModalProps {
  modelValue: boolean
  title?: string
  description?: string
  size?: 'sm' | 'md' | 'lg' | 'xl' | '2xl'
}

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'close'): void
}>()
```
- **Size 映射表**：
  - `sm`: `max-w-md`
  - `md`: `max-w-lg`
  - `lg`: `max-w-2xl`
  - `xl`: `max-w-4xl`
  - `2xl`: `max-w-6xl`
- **生命周期**：监听 `keydown` 事件（`Escape` 键），在 `onMounted` 注册，`onUnmounted` 卸载。

### 2.2 订阅生成服务类 (`services/subscription.ts`)

```ts
export class UserSubscriptionService {
  /** 生成独立 Token 订阅链接 */
  static getDirectTokenSubUrl(user: UserItem): string { ... }
  
  /** 解析节点入站 Tags 列表 */
  static getNodeTags(user: UserItem): string[] { ... }
  
  /** 生成单节点多协议链接 (vless, vmess, trojan, ss) */
  static generateNodeLink(inb: any, user: UserItem): string { ... }
}
```

### 2.3 参数治理与清洗纯函数 (`types.ts`)

```ts
export function sanitizeUserPayload(
  form: UserFormData,
  isEditing: boolean,
  existingUser?: UserItem
): Record<string, any> {
  const totalBytes = form.totalGB > 0 ? Math.round(form.totalGB * 1073741824) : 0
  
  if (isEditing && existingUser) {
    let expireTime = existingUser.expireTime || 0
    if (form.extendDays > 0) {
      const now = Date.now()
      expireTime = (!expireTime || expireTime < now)
        ? now + form.extendDays * 86400000
        : expireTime + form.extendDays * 86400000
    }
    return {
      email: form.email,
      inboundTags: form.selectedTags.join(','),
      inboundTag: form.selectedTags[0] || '',
      flow: form.flow,
      totalBytes,
      expireTime,
      resetDay: form.resetDay,
      ipLimit: form.ipLimit,
      enabled: form.enabled,
    }
  }

  return {
    email: form.email,
    inboundTags: form.selectedTags,
    inboundTag: form.selectedTags[0] || '',
    flow: form.flow,
    totalBytes,
    expireDays: form.expireDays,
    resetDay: form.resetDay,
    ipLimit: form.ipLimit,
    enabled: form.enabled,
  }
}
```

### 2.4 异步按需懒加载策略 (`UsersView.vue`)

```ts
const UserFormDrawer = defineAsyncComponent(() => import('./users/components/UserFormDrawer.vue'))
const UserShareModal = defineAsyncComponent(() => import('./users/components/UserShareModal.vue'))
const UserTrafficModal = defineAsyncComponent(() => import('./users/components/UserTrafficModal.vue'))
```
- `qrcode.vue` 仅在 `UserShareModal` 中静态引入，由于该 Modal 被异步按需加载，`vendor-qrcode` 在首屏不产生阻塞。

---

## 3. 回滚与兼容策略

- `web/src/views/UsersView.vue` 保持顶层导出与路由引用兼容不变；
- API 提交格式与后端保持 100% 兼容。
