<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">Xray 原始配置管理</h1>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-neutral-900 border border-border text-cyan-400">
            config.json
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          完整的 config.json 在线编辑器，支持 JSON 格式化、官方 xray -test 严格校验与快照一键回滚
        </p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          @click="openSnapshotsModal"
        >
          <History class="w-3.5 h-3.5 mr-1" />
          <span>版本历史</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="formatJSON"
        >
          美化 JSON
        </Button>
        <Button
          variant="secondary"
          size="sm"
          :loading="validating"
          @click="validateConfig"
        >
          <CheckCircle2 class="w-3.5 h-3.5 mr-1 text-cyan-400" />
          <span>{{ validating ? '校验中...' : '测试有效性' }}</span>
        </Button>
        <Button
          variant="default"
          size="sm"
          :loading="saving"
          @click="saveConfig"
        >
          <Save class="w-3.5 h-3.5 mr-1" />
          <span>{{ saving ? '保存并重载中...' : '保存并重载 Xray' }}</span>
        </Button>
      </div>
    </div>

    <!-- Alert / Validation Output Banner -->
    <div
      v-if="testResult"
      class="p-3 rounded-lg border text-xs flex items-start gap-2.5 font-mono"
      :class="testResult.valid ? 'bg-emerald-950/20 border-emerald-800/40 text-emerald-300' : 'bg-rose-950/20 border-rose-800/40 text-rose-300'"
    >
      <component :is="testResult.valid ? CheckCircle2 : AlertCircle" class="w-4 h-4 mt-0.5 shrink-0" />
      <div class="space-y-0.5">
        <p class="font-bold font-sans">{{ testResult.valid ? '✅ 配置校验通过' : '❌ 配置存在错误' }}</p>
        <p class="text-[11px] opacity-90">{{ testResult.message }}</p>
      </div>
    </div>

    <!-- Code Editor Area -->
    <div class="rounded-lg border border-border bg-neutral-950 overflow-hidden relative">
      <div class="flex items-center justify-between px-3.5 py-2 bg-neutral-900 border-b border-border text-xs text-muted-foreground font-mono">
        <span>config.json</span>
        <span class="text-[11px]">UTF-8 / JSON</span>
      </div>
      <textarea
        v-model="rawContent"
        spellcheck="false"
        class="w-full h-[600px] bg-neutral-950 text-foreground font-['JetBrains_Mono',monospace] text-xs p-4 focus:outline-none leading-relaxed resize-none selection:bg-brand-500 selection:text-white"
        placeholder="正在加载配置文件..."
      ></textarea>
    </div>

    <!-- History Snapshots Modal -->
    <div v-if="showSnapshots" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75">
      <div class="w-full max-w-lg p-5 rounded-lg border border-border bg-card shadow-2xl space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <div>
            <h2 class="text-sm font-bold text-foreground flex items-center gap-1.5">
              <History class="w-4 h-4 text-cyan-400" />
              <span>配置文件历史快照</span>
            </h2>
            <p class="text-xs text-muted-foreground mt-0.5">保存前自动备份的快照版本，支持一键安全回滚</p>
          </div>
          <button @click="showSnapshots = false" class="text-muted-foreground hover:text-foreground text-base">✕</button>
        </div>

        <div class="space-y-2 max-h-80 overflow-y-auto">
          <div
            v-for="snap in snapshots"
            :key="snap.id"
            class="p-2.5 bg-neutral-950 rounded-md border border-border flex items-center justify-between gap-3 hover:border-neutral-700 transition-colors"
          >
            <div class="space-y-0.5 text-xs font-mono">
              <span class="font-bold text-foreground text-[11px]">{{ snap.remark || '系统自动快照' }}</span>
              <p class="text-[10px] text-muted-foreground">{{ formatDate(snap.createdAt) }}</p>
            </div>

            <Button
              variant="secondary"
              size="sm"
              class="h-7 text-xs text-brand-400 hover:text-brand-300"
              :disabled="rollingBack"
              @click="rollbackToSnapshot(snap.id)"
            >
              回滚至此版本
            </Button>
          </div>

          <div v-if="!snapshots.length" class="text-center py-8 text-xs text-muted-foreground">
            暂无历史快照记录，在修改保存配置后将自动生成
          </div>
        </div>

        <div class="flex justify-end pt-2 border-t border-border">
          <Button
            variant="secondary"
            size="sm"
            @click="showSnapshots = false"
          >
            关闭
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { CheckCircle2, AlertCircle, Save, History } from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import { toast } from '../utils/toast'
import api from '../api'

const rawContent = ref('')
const validating = ref(false)
const saving = ref(false)
const rollingBack = ref(false)
const testResult = ref<any>(null)

const showSnapshots = ref(false)
const snapshots = ref<any[]>([])

const fetchRawConfig = async () => {
  try {
    const res: any = await api.get('/config/raw')
    rawContent.value = typeof res === 'string' ? res : JSON.stringify(res, null, 4)
  } catch (err: any) {
    toast.error('加载配置失败: ' + err)
  }
}

const openSnapshotsModal = async () => {
  try {
    const res: any = await api.get('/config/snapshots')
    snapshots.value = res || []
    showSnapshots.value = true
  } catch (err: any) {
    toast.error('获取快照列表失败: ' + err)
  }
}

const rollbackToSnapshot = async (id: number) => {
  if (!confirm('确定回滚至该历史快照版本吗？当前配置将被替换并自动重载。')) return
  rollingBack.value = true
  try {
    await api.post(`/config/snapshots/${id}/rollback`)
    toast.success('已成功回滚至历史版本并重载 Xray 核心！')
    showSnapshots.value = false
    await fetchRawConfig()
  } catch (err: any) {
    toast.error('回滚失败: ' + err)
  } finally {
    rollingBack.value = false
  }
}

const formatJSON = () => {
  try {
    const obj = JSON.parse(rawContent.value)
    rawContent.value = JSON.stringify(obj, null, 4)
    toast.info('JSON 配置已格式化排版')
  } catch (err: any) {
    toast.error('无法美化，JSON 语法存在错误: ' + err.message)
  }
}

const validateConfig = async () => {
  validating.value = true
  testResult.value = null
  try {
    const res: any = await api.post('/config/validate', rawContent.value, {
      headers: { 'Content-Type': 'application/json' },
    })
    testResult.value = { valid: true, message: res.message || 'Xray 核心已成功解析并确认此配置有效！' }
  } catch (err: any) {
    testResult.value = { valid: false, message: typeof err === 'string' ? err : '校验未通过' }
  } finally {
    validating.value = false
  }
}

const saveConfig = async () => {
  if (!confirm('确定保存并覆盖当前的 Xray 核心配置吗？')) return
  saving.value = true
  testResult.value = null
  try {
    await api.post('/config/save', rawContent.value, {
      headers: { 'Content-Type': 'application/json' },
    })
    testResult.value = { valid: true, message: '配置已成功保存落盘并完成重载！' }
    await fetchRawConfig()
  } catch (err: any) {
    testResult.value = { valid: false, message: '保存失败: ' + err }
  } finally {
    saving.value = false
  }
}

const formatDate = (dateStr: string) => {
  if (!dateStr) return ''
  return new Date(dateStr).toLocaleString()
}

onMounted(() => {
  fetchRawConfig()
})
</script>
