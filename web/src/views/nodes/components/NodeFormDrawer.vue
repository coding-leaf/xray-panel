<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="isEditing ? `编辑节点：${form.name || initialData?.name || ''}` : '新增托管节点'"
    :description="
      isEditing
        ? '更新托管节点接入点、地区标签、订阅暴露策略与调度状态'
        : '注册托管节点后将生成一次性 Agent Token 与部署脚本'
    "
    width="w-full sm:max-w-lg"
  >
    <form id="node-form" @submit.prevent="submitForm" class="space-y-4 text-xs">
      <SectionCard title="① 节点标识与接入点">
        <div class="space-y-3">
          <FormField
            label="节点标识 (仅后台管理)"
            required
            hint="用于后台识别与节点切换器展示（如 jp-tokyo-01），默认不会拼入用户订阅名"
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
            hint="节点公网接入地址（供直连或 Master 中转出站连接，当绑定入站未单独指定 ExternalHost 时生效）"
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
            hint="可选，供后台筛选或在命名模板中通过 {region} 引用（默认不强拼入订阅名）"
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

      <SectionCard title="② 订阅暴露与调度状态">
        <div class="space-y-3">
          <label
            class="flex items-start justify-between gap-3 p-3 rounded-md border cursor-pointer transition-colors"
            :class="
              form.directSubEnabled
                ? 'border-emerald-500/35 bg-emerald-500/5 hover:border-emerald-500/50'
                : 'border-amber-500/35 bg-amber-500/5 hover:border-amber-500/50'
            "
          >
            <div class="space-y-1">
              <div class="flex items-center gap-2 flex-wrap">
                <span class="text-xs font-semibold text-foreground">
                  对外直连订阅 (directSubEnabled)
                </span>
                <Badge :variant="form.directSubEnabled ? 'success' : 'warning'" class="text-[10px]">
                  {{ form.directSubEnabled ? '直连开放' : '仅中转落地' }}
                </Badge>
              </div>
              <p class="text-[10px] text-muted-foreground leading-relaxed font-mono">
                {{
                  form.directSubEnabled
                    ? '开启：允许此节点下公开订阅的入站进入用户订阅与单节点分享。'
                    : '关闭：整机锁死为「仅中转落地」，绝不进入任何用户订阅、单节点分享或默认全选（不影响 Master 中转转发、Agent 心跳与流量熔断）。'
                }}
              </p>
            </div>
            <input
              type="checkbox"
              v-model="form.directSubEnabled"
              class="rounded bg-neutral-900 border-border text-neutral-200 mt-0.5 shrink-0"
            />
          </label>

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

      <SectionCard title="③ 流量配额与熔断策略">
        <div class="space-y-3">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="月流量上限 (GB)" hint="0 表示不限月流量">
              <template #default="{ id, describedBy }">
                <Input
                  :id="id"
                  :aria-describedby="describedBy"
                  v-model.number="form.trafficLimitGB"
                  type="number"
                  min="0"
                  step="0.1"
                  placeholder="0"
                />
              </template>
            </FormField>

            <FormField label="账单重置日 (0-31)" hint="0 为不自动重置；短月份自动适配月底">
              <template #default="{ id, describedBy }">
                <Input
                  :id="id"
                  :aria-describedby="describedBy"
                  v-model.number="form.billingResetDay"
                  type="number"
                  min="0"
                  max="31"
                  step="1"
                  placeholder="0"
                />
              </template>
            </FormField>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="计费口径 (Count Mode)" :hint="selectedCountModeDescription">
              <template #default="{ id, describedBy }">
                <select
                  :id="id"
                  :aria-describedby="describedBy"
                  v-model="form.trafficCountMode"
                  class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono text-xs focus:outline-none focus:ring-1 focus:ring-ring"
                >
                  <option
                    v-for="opt in NODE_TRAFFIC_COUNT_MODE_OPTIONS"
                    :key="opt.value"
                    :value="opt.value"
                  >
                    {{ opt.label }}
                  </option>
                </select>
              </template>
            </FormField>

            <FormField label="安全系数 (Multiplier)" hint="默认 1.0；可设为 1.05 等防口径偏差">
              <template #default="{ id, describedBy }">
                <Input
                  :id="id"
                  :aria-describedby="describedBy"
                  v-model.number="form.trafficMultiplier"
                  type="number"
                  min="0.01"
                  step="0.05"
                  placeholder="1.0"
                />
              </template>
            </FormField>
          </div>

          <div class="space-y-1.5">
            <div class="grid grid-cols-3 gap-2.5">
              <FormField label="预警阈值 (%)">
                <template #default="{ id }">
                  <Input
                    :id="id"
                    v-model.number="form.warnThresholdPct"
                    type="number"
                    min="1"
                    max="100"
                    step="1"
                    placeholder="80"
                  />
                </template>
              </FormField>

              <FormField label="摘流阈值 (%)">
                <template #default="{ id }">
                  <Input
                    :id="id"
                    v-model.number="form.drainThresholdPct"
                    type="number"
                    min="1"
                    max="100"
                    step="1"
                    placeholder="90"
                  />
                </template>
              </FormField>

              <FormField label="熔断阈值 (%)">
                <template #default="{ id }">
                  <Input
                    :id="id"
                    v-model.number="form.breakThresholdPct"
                    type="number"
                    min="1"
                    max="100"
                    step="1"
                    placeholder="95"
                  />
                </template>
              </FormField>
            </div>
            <p class="text-[10px] font-mono text-muted-foreground">
              需满足 0 &lt; 预警 &le; 摘流 &le; 熔断 &le; 100（默认 80% / 90% / 95%）
            </p>
            <div
              class="p-2.5 rounded-md bg-amber-500/5 border border-amber-500/20 text-amber-200/90 text-[10px] leading-relaxed"
            >
              <span class="font-semibold text-amber-300">离线流量口径提示：</span>
              节点离线或 Xray gRPC 统计不可用期间，Agent
              将以物理网卡增量作为兜底计费口径（可能包含宿主机非 Xray 外部流量）；恢复上线后自动切回
              Xray 精确统计口径。
            </div>
          </div>
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
import Badge from '../../../components/ui/Badge.vue'
import { createNode, updateNode, type CreateNodeResult, type NodeItem } from '../../../api/nodes'
import { toast } from '../../../utils/toast'
import {
  NODE_DESIRED_STATE_OPTIONS,
  NODE_TRAFFIC_COUNT_MODE_OPTIONS,
  getDefaultNodeFormData,
  type NodeFormData,
} from '../types'

const BYTES_PER_GB = 1024 * 1024 * 1024

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

const selectedCountModeDescription = computed(() => {
  const found = NODE_TRAFFIC_COUNT_MODE_OPTIONS.find(
    (opt) => opt.value === form.value.trafficCountMode
  )
  return found?.description || ''
})

const initForm = () => {
  if (props.isEditing && props.initialData) {
    const limitBytes = props.initialData.trafficLimitBytes || 0
    form.value = {
      id: props.initialData.id,
      name: props.initialData.name || '',
      host: props.initialData.host || '',
      region: props.initialData.region || '',
      enabled: props.initialData.enabled !== false,
      directSubEnabled: props.initialData.directSubEnabled !== false,
      desiredState: props.initialData.desiredState || 'active',
      trafficLimitGB: limitBytes > 0 ? Number((limitBytes / BYTES_PER_GB).toFixed(2)) : 0,
      billingResetDay: props.initialData.billingResetDay ?? 0,
      trafficCountMode: props.initialData.trafficCountMode || 'both',
      trafficMultiplier:
        props.initialData.trafficMultiplier > 0 ? props.initialData.trafficMultiplier : 1.0,
      warnThresholdPct:
        props.initialData.warnThresholdPct > 0 ? props.initialData.warnThresholdPct : 80,
      drainThresholdPct:
        props.initialData.drainThresholdPct > 0 ? props.initialData.drainThresholdPct : 90,
      breakThresholdPct:
        props.initialData.breakThresholdPct > 0 ? props.initialData.breakThresholdPct : 95,
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

  const trafficLimitGB = Number(form.value.trafficLimitGB || 0)
  if (!Number.isFinite(trafficLimitGB) || trafficLimitGB < 0) {
    toast.error('月流量上限不能小于 0')
    return
  }

  const billingResetDay = Number(form.value.billingResetDay || 0)
  if (!Number.isInteger(billingResetDay) || billingResetDay < 0 || billingResetDay > 31) {
    toast.error('账单重置日须为 0 到 31 的整数')
    return
  }

  const trafficMultiplier = Number(form.value.trafficMultiplier)
  if (!Number.isFinite(trafficMultiplier) || trafficMultiplier <= 0) {
    toast.error('流量安全系数须大于 0')
    return
  }

  const warn = Number(form.value.warnThresholdPct)
  const drain = Number(form.value.drainThresholdPct)
  const breakPct = Number(form.value.breakThresholdPct)
  if (
    !Number.isInteger(warn) ||
    !Number.isInteger(drain) ||
    !Number.isInteger(breakPct) ||
    warn <= 0 ||
    warn > drain ||
    drain > breakPct ||
    breakPct > 100
  ) {
    toast.error('阈值须为 1–100 的整数，且满足 预警 ≤ 摘流 ≤ 熔断')
    return
  }

  const trafficLimitBytes = trafficLimitGB > 0 ? Math.round(trafficLimitGB * BYTES_PER_GB) : 0
  const quotaPayload = {
    trafficLimitBytes,
    billingResetDay,
    trafficCountMode: form.value.trafficCountMode,
    trafficMultiplier,
    warnThresholdPct: warn,
    drainThresholdPct: drain,
    breakThresholdPct: breakPct,
  }

  saving.value = true
  try {
    if (props.isEditing) {
      const updated = await updateNode(form.value.id, {
        name,
        host,
        region: form.value.region.trim(),
        enabled: form.value.enabled,
        directSubEnabled: form.value.directSubEnabled,
        desiredState: form.value.desiredState,
        remark: form.value.remark.trim(),
        ...quotaPayload,
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
        directSubEnabled: form.value.directSubEnabled,
        desiredState: form.value.desiredState,
        remark: form.value.remark.trim(),
        ...quotaPayload,
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
