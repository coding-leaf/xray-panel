<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">Xray 原始配置管理</h1>
        <p class="text-xs text-gray-400 mt-0.5">完整的 config.json 在线编辑器，支持 JSON 格式化、官方 xray -test 严格校验、自动历史快照与一键回滚</p>
      </div>
      <div class="flex items-center gap-2">
        <button
          @click="openSnapshotsModal"
          class="px-3.5 py-2 rounded-xl text-xs bg-gray-800 hover:bg-gray-700 text-cyan-400 border border-gray-700 transition-colors flex items-center gap-1.5"
        >
          <History class="w-3.5 h-3.5" />
          <span>版本历史与回滚</span>
        </button>
        <button
          @click="formatJSON"
          class="px-3.5 py-2 rounded-xl text-xs bg-gray-800 hover:bg-gray-700 text-gray-200 border border-gray-700 transition-colors"
        >
          美化 JSON
        </button>
        <button
          @click="validateConfig"
          :disabled="validating"
          class="px-3.5 py-2 rounded-xl text-xs bg-cyan-600/20 hover:bg-cyan-600/30 text-cyan-400 border border-cyan-500/30 font-semibold transition-colors flex items-center gap-1.5"
        >
          <CheckCircle2 class="w-3.5 h-3.5" />
          <span>{{ validating ? '校验中...' : '测试有效性' }}</span>
        </button>
        <button
          @click="saveConfig"
          :disabled="saving"
          class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
        >
          <Save class="w-3.5 h-3.5" />
          <span>{{ saving ? '保存并重载中...' : '保存并重载 Xray' }}</span>
        </button>
      </div>
    </div>

    <!-- Alert / Validation Output Banner -->
    <div v-if="testResult" class="p-4 rounded-2xl border text-xs flex items-start gap-3" :class="testResult.valid ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-300' : 'bg-red-500/10 border-red-500/20 text-red-300'">
      <component :is="testResult.valid ? CheckCircle2 : AlertCircle" class="w-4 h-4 mt-0.5 shrink-0" />
      <div class="space-y-1">
        <p class="font-bold">{{ testResult.valid ? '✅ 配置校验通过' : '❌ 配置存在错误' }}</p>
        <p class="font-mono text-[11px] opacity-90">{{ testResult.message }}</p>
      </div>
    </div>

    <!-- Code Editor Area -->
    <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden relative">
      <div class="flex items-center justify-between px-4 py-2 bg-gray-900/80 border-b border-gray-800 text-xs text-gray-400">
        <span class="font-mono">config.json</span>
        <span>UTF-8 / JSON</span>
      </div>
      <textarea
        v-model="rawContent"
        spellcheck="false"
        class="w-full h-[600px] bg-[#070A11] text-gray-200 font-['JetBrains_Mono',monospace] text-xs p-4 focus:outline-none leading-relaxed resize-none selection:bg-brand-500 selection:text-white"
        placeholder="正在加载配置文件..."
      ></textarea>
    </div>

    <!-- History Snapshots Modal -->
    <div v-if="showSnapshots" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-lg p-6 sm:p-7 rounded-3xl border border-gray-800 shadow-2xl space-y-5">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-base font-bold text-white flex items-center gap-2">
              <History class="w-4 h-4 text-cyan-400" />
              <span>配置文件历史快照</span>
            </h2>
            <p class="text-xs text-gray-400 mt-0.5">每次修改保存前自动生成的备份快照，支持一键安全回滚</p>
          </div>
          <button @click="showSnapshots = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <div class="space-y-2 max-h-80 overflow-y-auto">
          <div
            v-for="snap in snapshots"
            :key="snap.id"
            class="p-3 bg-gray-900/80 rounded-2xl border border-gray-800 flex items-center justify-between gap-3 hover:border-gray-700 transition-colors"
          >
            <div class="space-y-0.5 text-xs">
              <span class="font-bold text-white font-mono text-[11px]">{{ snap.remark || '系统自动快照' }}</span>
              <p class="text-[10px] text-gray-400 font-mono">{{ formatDate(snap.createdAt) }}</p>
            </div>

            <button
              @click="rollbackToSnapshot(snap.id)"
              :disabled="rollingBack"
              class="px-3 py-1.5 bg-brand-600/20 hover:bg-brand-600/30 text-brand-300 hover:text-white rounded-xl text-xs font-semibold border border-brand-500/30 transition-all shrink-0"
            >
              回滚至此版本
            </button>
          </div>

          <div v-if="!snapshots.length" class="text-center py-8 text-xs text-gray-500">
            暂无历史快照记录，在修改保存配置后将自动生成
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { CheckCircle2, AlertCircle, Save, History } from 'lucide-vue-next'
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
