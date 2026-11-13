<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="isEditing ? `编辑用户：${form.email}` : '新建用户'"
    :description="
      isEditing
        ? '调整用户的核心凭据、流控策略、授权节点与有效期'
        : '创建新凭据并自动下发同步至核心节点'
    "
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <form @submit.prevent="saveUser" id="userForm" class="space-y-4 text-xs">
      <!-- 1. Authentication -->
      <SectionCard title="1. 基础认证与身份">
        <div class="space-y-3">
          <FormField
            label="用户名 / 邮箱"
            :required="true"
            :hint="
              isEditing
                ? '修改邮箱将自动清理 Xray 核心旧凭证，并级联更新各分流线路授权引用'
                : '创建后将作为订阅唯一索引与 Xray 客户端身份标记'
            "
          >
            <input
              v-model="form.email"
              type="text"
              required
              placeholder="user@example.com 或 纯用户名"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField
              label="UUID / 连接密码"
              :hint="
                isEditing
                  ? '修改将立即重置 Xray 客户端连接认证凭证'
                  : '选填，留空自动生成标准 RFC4122 v4 UUID'
              "
            >
              <div class="flex items-center gap-1.5">
                <input
                  v-model="form.uuid"
                  type="text"
                  placeholder="留空自动生成"
                  class="flex-1 bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  class="h-9 px-2 text-[11px] shrink-0"
                  @click="generateUUID"
                  title="随机生成 UUID"
                >
                  <Wand2 class="w-3.5 h-3.5 mr-1 text-cyan-400" />
                  <span>生成</span>
                </Button>
              </div>
            </FormField>

            <FormField
              label="订阅安全 Token"
              :hint="
                isEditing ? '修改将使该用户的旧订阅链接失效' : '选填，留空自动生成高强度随机 Token'
              "
            >
              <div class="flex items-center gap-1.5">
                <input
                  v-model="form.subToken"
                  type="text"
                  placeholder="留空自动生成 (≥8位字母数字)"
                  class="flex-1 bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
                />
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  class="h-9 px-2 text-[11px] shrink-0"
                  @click="generateSubToken"
                  title="随机生成订阅 Token"
                >
                  <Wand2 class="w-3.5 h-3.5 mr-1 text-cyan-400" />
                  <span>生成</span>
                </Button>
              </div>
            </FormField>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField
              label="用户备注 (Remark)"
              hint="选填，客户姓名、部门或备忘信息 (最多255字符)"
            >
              <input
                v-model="form.remark"
                type="text"
                maxlength="255"
                placeholder="例如：VIP 客户张三 / 测试组"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>

            <FormField
              label="流控策略模式 (Flow)"
              hint="配置用户级流控，选择「显式禁用」将强制剥除 flow"
            >
              <select
                v-model="form.flow"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              >
                <option value="">跟随节点默认策略 (推荐)</option>
                <option value="xtls-rprx-vision">xtls-rprx-vision (强制开启 Vision)</option>
                <option value="none">none (显式禁用流控)</option>
              </select>
            </FormField>
          </div>

          <div class="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="form-enabled"
              v-model="form.enabled"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
            <label
              for="form-enabled"
              class="text-foreground font-medium cursor-pointer select-none"
            >
              账号处于启用状态（禁用后将立即从节点断开）
            </label>
          </div>
        </div>
      </SectionCard>

      <!-- 2. Authorized Inbounds -->
      <SectionCard title="2. 授权入站节点 (Inbounds)">
        <template #actions>
          <button
            type="button"
            @click="toggleSelectAllInbounds"
            class="text-[11px] text-foreground hover:underline font-mono"
          >
            {{ isAllInboundsSelected ? '取消全选' : '全选所有节点' }}
          </button>
        </template>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-44 overflow-y-auto p-1">
          <label
            v-for="inb in availableInbounds"
            :key="inb.id"
            class="flex items-center gap-2 p-2.5 rounded-md border border-border bg-neutral-950 hover:bg-muted/40 cursor-pointer transition-colors"
            :class="{ 'border-neutral-500 bg-muted/30': form.selectedTags.includes(inb.tag) }"
          >
            <input
              type="checkbox"
              :value="inb.tag"
              v-model="form.selectedTags"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
            <div class="min-w-0 font-mono">
              <div class="text-xs font-semibold text-foreground truncate">{{ inb.tag }}</div>
              <div class="text-[10px] text-muted-foreground uppercase">
                {{ inb.protocol }} :{{ inb.port }}
              </div>
            </div>
          </label>
        </div>
      </SectionCard>

      <!-- 3. Quota & Lifecycle Policy -->
      <SectionCard title="3. 配额限额与计费周期策略">
        <div class="space-y-3">
          <!-- Traffic & Expiry Mode Selector -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="流量限额 (GB)" hint="达到配额后自动切断连接">
              <input
                v-model.number="form.totalGB"
                type="number"
                min="0"
                step="0.01"
                placeholder="0 为无限制"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>

            <div>
              <label class="block text-muted-foreground font-mono text-[11px] mb-1"
                >账号有效期模式 (Expire Mode)</label
              >
              <div
                class="grid grid-cols-3 gap-1.5 p-0.5 bg-neutral-950 border border-border rounded-md h-9 items-center"
              >
                <button
                  type="button"
                  @click="setExpireMode('never')"
                  class="h-7 text-xs rounded transition-colors font-medium flex items-center justify-center cursor-pointer select-none"
                  :class="
                    expireMode === 'never'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:text-foreground'
                  "
                >
                  永不过期
                </button>
                <button
                  type="button"
                  @click="setExpireMode('days')"
                  class="h-7 text-xs rounded transition-colors font-medium flex items-center justify-center cursor-pointer select-none"
                  :class="
                    expireMode === 'days'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:text-foreground'
                  "
                >
                  {{ isEditing ? '快捷延期' : '按有效天数' }}
                </button>
                <button
                  type="button"
                  @click="setExpireMode('date')"
                  class="h-7 text-xs rounded transition-colors font-medium flex items-center justify-center cursor-pointer select-none"
                  :class="
                    expireMode === 'date'
                      ? 'bg-muted text-foreground'
                      : 'text-muted-foreground hover:text-foreground'
                  "
                >
                  指定截止日期
                </button>
              </div>
            </div>
          </div>

          <!-- Expire Mode Dynamic Panel -->
          <div class="p-3 rounded-md bg-neutral-950 border border-border/80 space-y-2">
            <!-- Mode 1: Never -->
            <div
              v-if="expireMode === 'never'"
              class="flex items-center justify-between text-xs font-mono text-muted-foreground py-1"
            >
              <span class="text-emerald-400 font-medium">⏳ 账号长期有效，不设到期时间限制。</span>
              <span v-if="isEditing && initialData?.expireTime" class="text-[10px] text-amber-400">
                (保存后将清除原到期时间并转为永久有效)
              </span>
            </div>

            <!-- Mode 2: Days -->
            <div v-else-if="expireMode === 'days'" class="space-y-2">
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <FormField
                  :label="!isEditing ? '有效天数 (从当前时刻起)' : '顺延有效天数 (+天)'"
                  :hint="!isEditing ? '例如：30 天' : '在现有到期时间或当前时间基础上顺延'"
                  class="flex-1"
                >
                  <input
                    v-if="!isEditing"
                    v-model.number="form.expireDays"
                    type="number"
                    min="1"
                    placeholder="例如 30"
                    class="w-full bg-neutral-900 border border-border rounded-md px-3 h-8 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
                  />
                  <input
                    v-else
                    v-model.number="form.extendDays"
                    type="number"
                    min="0"
                    placeholder="例如 30"
                    class="w-full bg-neutral-900 border border-border rounded-md px-3 h-8 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
                  />
                </FormField>

                <div class="flex items-center gap-1.5 pt-1 sm:pt-4">
                  <Button
                    v-for="d in [30, 90, 180, 365]"
                    :key="d"
                    type="button"
                    variant="outline"
                    size="sm"
                    class="h-7 text-[10px] px-2 font-mono"
                    @click="applyDaysPreset(d)"
                  >
                    +{{ d }}天
                  </Button>
                </div>
              </div>

              <div class="text-[11px] text-muted-foreground font-mono">
                <span v-if="!isEditing">
                  预计到期时间:
                  <span class="text-foreground font-medium">{{
                    computeCreateExpirePreview(form.expireDays)
                  }}</span>
                </span>
                <span v-else>
                  当前到期:
                  <span class="text-foreground">{{
                    initialData?.expireTime ? formatDate(initialData.expireTime) : '永久有效'
                  }}</span>
                  <span v-if="form.extendDays > 0" class="text-emerald-400 ml-2">
                    → 顺延后预计到期：{{ computeExtendExpirePreview(form.extendDays) }}
                  </span>
                </span>
              </div>
            </div>

            <!-- Mode 3: Date -->
            <div v-else-if="expireMode === 'date'" class="space-y-2">
              <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2">
                <FormField
                  label="精确截止日期与时间"
                  hint="选择具体到期年月日与时间"
                  class="flex-1"
                >
                  <input
                    v-model="expireDateInput"
                    type="datetime-local"
                    class="w-full bg-neutral-900 border border-border rounded-md px-3 h-8 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
                  />
                </FormField>

                <div class="flex items-center gap-1.5 pt-1 sm:pt-4">
                  <Button
                    v-for="d in [30, 90, 180, 365]"
                    :key="d"
                    type="button"
                    variant="outline"
                    size="sm"
                    class="h-7 text-[10px] px-2 font-mono"
                    @click="addDaysToDateInput(d)"
                  >
                    +{{ d }}天
                  </Button>
                </div>
              </div>
            </div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="每月重置流量日" hint="0 为不按月重置流量">
              <input
                v-model.number="form.resetDay"
                type="number"
                min="0"
                max="31"
                placeholder="0-31 (如每月1号清零)"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>

            <FormField label="并发连接设备数 (IP)" hint="限制同时在线客户端 IP 数">
              <input
                v-model.number="form.ipLimit"
                type="number"
                min="0"
                max="100"
                placeholder="0 为不限制"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>
          </div>
        </div>
      </SectionCard>
    </form>

    <template #footer>
      <Button variant="outline" size="sm" @click="emit('update:modelValue', false)"> 取消 </Button>
      <Button variant="default" size="sm" type="submit" form="userForm" :loading="saving">
        {{ saving ? '保存中...' : '确认保存' }}
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { Wand2 } from 'lucide-vue-next'
import api from '../../../api'
import { toast } from '../../../utils/toast'
import { formatDate } from '../../../utils/format'
import { UserSubscriptionService } from '../services/subscription'
import { sanitizeUserPayload, type UserItem, type UserFormData } from '../types'

import Drawer from '../../../components/ui/Drawer.vue'
import Button from '../../../components/ui/Button.vue'
import FormField from '../../../components/ui/FormField.vue'
import SectionCard from '../../../components/ui/SectionCard.vue'

interface Props {
  modelValue: boolean
  isEditing: boolean
  initialData?: UserItem | null
  availableInbounds: any[]
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'saved'): void
}>()

const saving = ref(false)
const expireMode = ref<'never' | 'days' | 'date'>('days')
const expireDateInput = ref('')

const form = ref<UserFormData>({
  id: 0,
  email: '',
  uuid: '',
  subToken: '',
  remark: '',
  selectedTags: [],
  flow: '',
  totalGB: 0,
  expireTime: undefined,
  expireDays: 30,
  extendDays: 0,
  resetDay: 0,
  ipLimit: 0,
  enabled: true,
})

const formatDatetimeLocal = (timestamp: number): string => {
  if (!timestamp || timestamp <= 0) return ''
  const d = new Date(timestamp)
  const pad = (n: number) => n.toString().padStart(2, '0')
  const Y = d.getFullYear()
  const M = pad(d.getMonth() + 1)
  const D = pad(d.getDate())
  const h = pad(d.getHours())
  const m = pad(d.getMinutes())
  return `${Y}-${M}-${D}T${h}:${m}`
}

const computeCreateExpirePreview = (days: number): string => {
  if (!days || days <= 0) return '长期有效'
  return new Date(Date.now() + days * 86400000).toLocaleString()
}

const computeExtendExpirePreview = (extendDays: number): string => {
  const current = props.initialData?.expireTime || 0
  const now = Date.now()
  const base = !current || current < now ? now : current
  return new Date(base + extendDays * 86400000).toLocaleString()
}

const setExpireMode = (mode: 'never' | 'days' | 'date') => {
  expireMode.value = mode
  if (mode === 'never') {
    form.value.expireTime = 0
    form.value.expireDays = 0
    form.value.extendDays = 0
  } else if (mode === 'days') {
    if (!props.isEditing) {
      if (!form.value.expireDays) form.value.expireDays = 30
    }
  } else if (mode === 'date') {
    if (!expireDateInput.value) {
      const base =
        props.initialData?.expireTime && props.initialData.expireTime > Date.now()
          ? props.initialData.expireTime
          : Date.now() + 30 * 86400000
      expireDateInput.value = formatDatetimeLocal(base)
    }
  }
}

const applyDaysPreset = (days: number) => {
  if (!props.isEditing) {
    form.value.expireDays = days
  } else {
    form.value.extendDays = days
  }
}

const addDaysToDateInput = (days: number) => {
  const base = expireDateInput.value ? new Date(expireDateInput.value).getTime() : Date.now()
  const target = (base < Date.now() ? Date.now() : base) + days * 86400000
  expireDateInput.value = formatDatetimeLocal(target)
}

const MULTI_USER_PROTOCOLS = ['vless', 'vmess', 'trojan', 'shadowsocks', 'shadowsocks-2022']
const isMultiUserInbound = (inb: any) =>
  MULTI_USER_PROTOCOLS.includes((inb?.protocol || '').toLowerCase())

const generateUUID = () => {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    form.value.uuid = crypto.randomUUID()
  } else if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const bytes = new Uint8Array(16)
    crypto.getRandomValues(bytes)
    bytes[6] = (bytes[6] & 0x0f) | 0x40
    bytes[8] = (bytes[8] & 0x3f) | 0x80
    const hex = Array.from(bytes, (b) => b.toString(16).padStart(2, '0'))
    form.value.uuid = `${hex.slice(0, 4).join('')}-${hex.slice(4, 6).join('')}-${hex.slice(6, 8).join('')}-${hex.slice(8, 10).join('')}-${hex.slice(10, 16).join('')}`
  } else {
    form.value.uuid = 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
      const r = (Math.random() * 16) | 0
      const v = c === 'x' ? r : (r & 0x3) | 0x8
      return v.toString(16)
    })
  }
}

const generateSubToken = () => {
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    const bytes = new Uint8Array(16)
    crypto.getRandomValues(bytes)
    form.value.subToken = Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('')
  } else {
    const chars = '0123456789abcdef'
    let token = ''
    for (let i = 0; i < 32; i++) {
      token += chars.charAt(Math.floor(Math.random() * chars.length))
    }
    form.value.subToken = token
  }
}

const resetForm = () => {
  if (props.isEditing && props.initialData) {
    const u = props.initialData
    const hasExpire = u.expireTime && u.expireTime > 0
    expireMode.value = hasExpire ? 'date' : 'never'
    expireDateInput.value = hasExpire ? formatDatetimeLocal(u.expireTime) : ''

    form.value = {
      id: u.id,
      email: u.email,
      uuid: u.uuid || '',
      subToken: u.subToken || '',
      remark: u.remark || '',
      selectedTags: UserSubscriptionService.getNodeTags(u, props.availableInbounds),
      flow: u.flow || '',
      totalGB:
        u.totalBytes > 0 ? Math.max(0.01, parseFloat((u.totalBytes / 1073741824).toFixed(2))) : 0,
      expireTime: u.expireTime || 0,
      expireDays: 0,
      extendDays: 0,
      resetDay: u.resetDay || 0,
      ipLimit: u.ipLimit || 0,
      enabled: u.enabled,
    }
  } else {
    expireMode.value = 'days'
    expireDateInput.value = ''
    form.value = {
      id: 0,
      email: '',
      uuid: '',
      subToken: '',
      remark: '',
      selectedTags: props.availableInbounds.filter(isMultiUserInbound).map((i) => i.tag),
      flow: '',
      totalGB: 0,
      expireTime: undefined,
      expireDays: 30,
      extendDays: 0,
      resetDay: 0,
      ipLimit: 0,
      enabled: true,
    }
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      resetForm()
    }
  },
  { immediate: true }
)

const multiUserInbounds = computed(() => props.availableInbounds.filter(isMultiUserInbound))

const isAllInboundsSelected = computed(() => {
  if (!multiUserInbounds.value.length) return false
  return multiUserInbounds.value.every((i) => form.value.selectedTags.includes(i.tag))
})

const toggleSelectAllInbounds = () => {
  if (isAllInboundsSelected.value) {
    form.value.selectedTags = []
  } else {
    form.value.selectedTags = multiUserInbounds.value.map((i) => i.tag)
  }
}

const saveUser = async () => {
  if (!form.value.selectedTags.length) {
    toast.warning('请至少选择一个归属的入站节点')
    return
  }

  saving.value = true
  try {
    if (expireMode.value === 'never') {
      form.value.expireTime = 0
      form.value.expireDays = 0
      form.value.extendDays = 0
    } else if (expireMode.value === 'date') {
      if (expireDateInput.value) {
        form.value.expireTime = new Date(expireDateInput.value).getTime()
      } else {
        form.value.expireTime = 0
      }
      form.value.expireDays = 0
      form.value.extendDays = 0
    } else if (expireMode.value === 'days') {
      if (!props.isEditing) {
        form.value.expireTime = 0
      } else {
        form.value.expireTime = props.initialData?.expireTime || 0
      }
    }

    const payload = sanitizeUserPayload(form.value, props.isEditing, props.initialData || undefined)

    if (props.isEditing && form.value.id) {
      await api.put(`/users/${form.value.id}`, payload)
      toast.success('用户信息与授权节点已更新')
    } else {
      await api.post('/users', payload)
      toast.success('用户已创建并同步至核心节点')
    }

    emit('update:modelValue', false)
    emit('saved')
  } catch (err: any) {
    toast.error('保存失败：' + (err.message || err))
  } finally {
    saving.value = false
  }
}
</script>
