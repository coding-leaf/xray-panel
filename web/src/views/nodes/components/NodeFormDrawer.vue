<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="isEditing ? `编辑节点：${form.name || initialData?.name || ''}` : '新增远端直连节点'"
    :description="
      isEditing
        ? '更新节点公网接入点、地区前缀与调度状态'
        : '注册远端直连节点后将生成一次性 Agent Token 与部署脚本'
    "
    width="w-full sm:max-w-lg"
  >
    <form id="node-form" @submit.prevent="submitForm" class="space-y-4 text-xs">
      <SectionCard title="① 节点标识与接入点">
        <div class="space-y-3">
          <FormField
            label="节点名称 (Name)"
            required
            hint="用于后台识别与订阅默认前缀（如 jp-tokyo-01）"
          >
            <template #default="{ id, describedBy }">
              <Input
                :id="id"
                :aria-describedby="describedBy"
                v-model="form.name"
                type="text"
                required
                placeholder="例如：jp-tokyo-01"
              />
            </template>
          </FormField>

          <FormField
            label="公网 Host / IP"
            required
            hint="客户端直连地址（当绑定入站未单独指定 ExternalHost 时生效）"
          >
            <template #default="{ id, describedBy }">
              <Input
                :id="id"
                :aria-describedby="describedBy"
                v-model="form.host"
                type="text"
                required
                placeholder="例如：jp1.example.com 或 203.0.113.10"
              />
            </template>
          </FormField>

          <FormField
            label="地区标签 (Region)"
            hint="可选，订阅导出时将自动附加 [<Region>] 标识前缀"
          >
            <template #default="{ id, describedBy }">
              <Input
                :id="id"
                :aria-describedby="describedBy"
                v-model="form.region"
                type="text"
                placeholder="例如：JP / 日本 / HK"
              />
            </template>
          </FormField>
        </div>
      </SectionCard>

      <SectionCard title="② 调度状态与备注">
        <div class="space-y-3">
          <FormField label="期望调度状态 (Desired State)" :hint="selectedStateDescription">
            <template #default="{ id, describedBy }">
              <select
                :id="id"
                :aria-describedby="describedBy"
                v-model="form.desiredState"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono text-xs focus:outline-none focus:ring-1 focus:ring-ring"
              >
                <option
                  v-for="opt in NODE_DESIRED_STATE_OPTIONS"
                  :key="opt.value"
                  :value="opt.value"
                >
                  {{ opt.label }}
                </option>
              </select>
            </template>
          </FormField>

          <label
            class="flex items-center justify-between p-2.5 rounded-md border border-border/60 bg-neutral-950 cursor-pointer hover:border-border transition-colors"
          >
            <div>
              <span class="text-xs font-medium text-foreground block">启用该节点 (Enabled)</span>
              <span class="text-[10px] text-muted-foreground font-mono">
                关闭后 Agent 同步请求将被拒绝，且所有关联入站停止纳入订阅
              </span>
            </div>
            <input
              type="checkbox"
              v-model="form.enabled"
              class="rounded bg-neutral-900 border-border text-neutral-200 ml-3 shrink-0"
            />
          </label>

          <FormField label="运维备注 (Remark)">
            <template #default="{ id }">
              <Input
                :id="id"
                v-model="form.remark"
                type="text"
                placeholder="可选，如机房供应商、到期日或线路说明"
              />
            </template>
          </FormField>
        </div>
      </SectionCard>
    </form>

    <template #footer>
      <Button type="button" variant="outline" size="sm" @click="emit('update:modelValue', false)">
        取消
      </Button>
      <Button type="submit" form="node-form" variant="default" size="sm" :loading="saving">
        <span>{{ saving ? '提交中...' : isEditing ? '保存修改' : '创建并生成 Token' }}</span>
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import Drawer from '../../../components/ui/Drawer.vue'
import SectionCard from '../../../components/ui/SectionCard.vue'
import FormField from '../../../components/ui/FormField.vue'
import Input from '../../../components/ui/Input.vue'
import Button from '../../../components/ui/Button.vue'
import { createNode, updateNode, type CreateNodeResult, type NodeItem } from '../../../api/nodes'
import { toast } from '../../../utils/toast'
import { NODE_DESIRED_STATE_OPTIONS, getDefaultNodeFormData, type NodeFormData } from '../types'

const props = defineProps<{
  modelValue: boolean
  isEditing: boolean
  initialData?: NodeItem | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'created', result: CreateNodeResult): void
  (e: 'saved', result: NodeItem): void
}>()

const saving = ref(false)
const form = ref<NodeFormData>(getDefaultNodeFormData())

const selectedStateDescription = computed(() => {
  const found = NODE_DESIRED_STATE_OPTIONS.find((opt) => opt.value === form.value.desiredState)
  return found?.description || ''
})

const initForm = () => {
  if (props.isEditing && props.initialData) {
    form.value = {
      id: props.initialData.id,
      name: props.initialData.name || '',
      host: props.initialData.host || '',
      region: props.initialData.region || '',
      enabled: props.initialData.enabled !== false,
      desiredState: props.initialData.desiredState || 'active',
      remark: props.initialData.remark || '',
    }
  } else {
    form.value = getDefaultNodeFormData()
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      initForm()
    }
  },
  { immediate: true }
)

const submitForm = async () => {
  const name = form.value.name.trim()
  const host = form.value.host.trim()
  if (!name) {
    toast.error('请填写节点名称')
    return
  }
  if (!host) {
    toast.error('请填写节点公网 Host 或 IP')
    return
  }

  saving.value = true
  try {
    if (props.isEditing) {
      const updated = await updateNode(form.value.id, {
        name,
        host,
        region: form.value.region.trim(),
        enabled: form.value.enabled,
        desiredState: form.value.desiredState,
        remark: form.value.remark.trim(),
      })
      toast.success(`节点「${updated.name}」配置已更新`)
      emit('update:modelValue', false)
      emit('saved', updated)
    } else {
      const created = await createNode({
        name,
        host,
        region: form.value.region.trim(),
        enabled: form.value.enabled,
        desiredState: form.value.desiredState,
        remark: form.value.remark.trim(),
      })
      toast.success(`节点「${created.name}」已创建`)
      emit('update:modelValue', false)
      emit('created', created)
    }
  } catch (err: any) {
    toast.error('保存失败：' + (err?.message || err))
  } finally {
    saving.value = false
  }
}
</script>
