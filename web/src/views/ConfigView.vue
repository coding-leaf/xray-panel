<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">Xray 原始配置管理</h1>
        <p class="text-xs text-gray-400 mt-0.5">完整的 config.json 在线编辑器，支持 JSON 格式化、官方 xray -test 严格校验与保存平滑热重载</p>
      </div>
      <div class="flex items-center gap-2">
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
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { CheckCircle2, AlertCircle, Save } from 'lucide-vue-next'
import api from '../api'

const rawContent = ref('')
const validating = ref(false)
const saving = ref(false)
const testResult = ref<any>(null)

const fetchRawConfig = async () => {
  try {
    const res: any = await api.get('/config/raw')
    rawContent.value = typeof res === 'string' ? res : JSON.stringify(res, null, 4)
  } catch (err: any) {
    alert('加载配置失败: ' + err)
  }
}

const formatJSON = () => {
  try {
    const obj = JSON.parse(rawContent.value)
    rawContent.value = JSON.stringify(obj, null, 4)
  } catch (err: any) {
    alert('无法美化，JSON 语法存在错误: ' + err.message)
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
  if (!confirm('确定保存并平滑重载 Xray 核心吗？')) return
  saving.value = true
  testResult.value = null
  try {
    const res: any = await api.post('/config/save', rawContent.value, {
      headers: { 'Content-Type': 'application/json' },
    })
    testResult.value = { valid: true, message: res.message || '配置已成功保存并重新加载！' }
  } catch (err: any) {
    testResult.value = { valid: false, message: typeof err === 'string' ? err : '保存失败' }
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  fetchRawConfig()
})
</script>
