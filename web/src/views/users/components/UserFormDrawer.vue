<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="isEditing ? `编辑用户: ${form.email}` : '新建用户'"
    :description="isEditing ? '调整用户的授权节点、配额策略与有效期' : '创建新凭据并自动下发同步至核心节点'"
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <form @submit.prevent="saveUser" id="userForm" class="space-y-4 text-xs">
      <!-- 1. Authentication -->
      <SectionCard title="1. 基础认证与身份">
        <div class="space-y-3">
          <FormField
            label="用户名 / 邮箱"
            :required="true"
            hint="创建后将作为订阅唯一索引与 Xray 客户端身份标记"
          >
            <input
              v-model="form.email"
              type="text"
              required
              :disabled="isEditing"
              placeholder="user@example.com 或 纯用户名"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-60"
            />
          </FormField>

          <div class="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="form-enabled"
              v-model="form.enabled"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
            <label for="form-enabled" class="text-foreground font-medium cursor-pointer select-none">
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
              <div class="text-[10px] text-muted-foreground uppercase">{{ inb.protocol }} :{{ inb.port }}</div>
            </div>
          </label>
        </div>
      </SectionCard>

      <!-- 3. Quota & Lifecycle Policy -->
      <SectionCard title="3. 配额限额与计费周期策略">
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

          <FormField
            v-if="!isEditing"
            label="初始有效天数"
            hint="从创建当前时刻起计算"
          >
            <input
              v-model.number="form.expireDays"
              type="number"
              min="0"
              placeholder="0 为永久有效，如 30"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>

          <FormField
            v-else
            label="延长有效天数 (+天)"
            hint="在当前有效期基础上顺延"
          >
            <input
              v-model.number="form.extendDays"
              type="number"
              min="0"
              placeholder="如增加 30 天"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>

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
      </SectionCard>
    </form>

    <template #footer>
      <Button
        variant="outline"
        size="sm"
        @click="emit('update:modelValue', false)"
      >
        取消
      </Button>
      <Button
        variant="default"
        size="sm"
        type="submit"
        form="userForm"
        :loading="saving"
      >
        {{ saving ? '保存中...' : '确认保存' }}
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import api from '../../../api'
import { toast } from '../../../utils/toast'
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

const form = ref<UserFormData>({
  id: 0,
  email: '',
  selectedTags: [],
  flow: '',
  totalGB: 0,
  expireDays: 30,
  extendDays: 0,
  resetDay: 0,
  ipLimit: 0,
  enabled: true,
})

const resetForm = () => {
  if (props.isEditing && props.initialData) {
    const u = props.initialData
    form.value = {
      id: u.id,
      email: u.email,
      selectedTags: UserSubscriptionService.getNodeTags(u, props.availableInbounds),
      flow: u.flow || '',
      totalGB: u.totalBytes > 0 ? Math.max(0.01, parseFloat((u.totalBytes / 1073741824).toFixed(2))) : 0,
      expireDays: 0,
      extendDays: 0,
      resetDay: u.resetDay || 0,
      ipLimit: u.ipLimit || 0,
      enabled: u.enabled,
    }
  } else {
    form.value = {
      id: 0,
      email: '',
      selectedTags: props.availableInbounds.map((i) => i.tag),
      flow: '',
      totalGB: 0,
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

const isAllInboundsSelected = computed(() => {
  if (!props.availableInbounds.length) return false
  return form.value.selectedTags.length === props.availableInbounds.length
})

const toggleSelectAllInbounds = () => {
  if (isAllInboundsSelected.value) {
    form.value.selectedTags = []
  } else {
    form.value.selectedTags = props.availableInbounds.map((i) => i.tag)
  }
}

const saveUser = async () => {
  if (!form.value.selectedTags.length) {
    toast.warning('请至少选择一个归属的入站节点！')
    return
  }

  saving.value = true
  try {
    const payload = sanitizeUserPayload(form.value, props.isEditing, props.initialData || undefined)

    if (props.isEditing && form.value.id) {
      await api.put(`/users/${form.value.id}`, payload)
      toast.success('用户信息与授权节点已保存更新！')
    } else {
      await api.post('/users', payload)
      toast.success('用户已成功创建并同步至核心节点！')
    }

    emit('update:modelValue', false)
    emit('saved')
  } catch (err: any) {
    toast.error('保存失败: ' + (err.message || err))
  } finally {
    saving.value = false
  }
}
</script>
