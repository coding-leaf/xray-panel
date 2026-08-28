<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">Xray 运行日志</h1>
        <p class="text-xs text-gray-400 mt-0.5">实时在线查看 Xray 访问日志与错误诊断日志</p>
      </div>

      <div class="flex items-center gap-3">
        <!-- Log type switch -->
        <div class="bg-gray-900/80 p-1 rounded-xl border border-gray-800 flex">
          <button
            @click="switchType('access')"
            class="px-3 py-1 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'access' ? 'bg-brand-600 text-white shadow-sm' : 'text-gray-400 hover:text-white'"
          >
            访问日志 (Access)
          </button>
          <button
            @click="switchType('error')"
            class="px-3 py-1 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'error' ? 'bg-rose-600 text-white shadow-sm' : 'text-gray-400 hover:text-white'"
          >
            错误日志 (Error)
          </button>
        </div>

        <!-- Auto refresh toggle -->
        <button
          @click="toggleAutoRefresh"
          class="px-3 py-1.5 rounded-xl text-xs border transition-colors flex items-center gap-1.5"
          :class="autoRefresh ? 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30 font-semibold' : 'bg-gray-800/80 text-gray-400 border-gray-700'"
        >
          <span class="w-1.5 h-1.5 rounded-full" :class="autoRefresh ? 'bg-emerald-400 animate-ping' : 'bg-gray-500'"></span>
          <span>{{ autoRefresh ? '自动刷新中 (3s)' : '自动刷新已关' }}</span>
        </button>

        <button
          @click="fetchLogs"
          :disabled="loading"
          class="p-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-200 border border-gray-700 transition-colors"
          title="手动刷新"
        >
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <!-- Search / Filter bar -->
    <div class="glass-panel p-3.5 rounded-2xl border border-gray-800/80 flex items-center justify-between gap-4">
      <div class="flex items-center gap-2 flex-1 text-xs">
        <Search class="w-4 h-4 text-gray-500" />
        <input
          v-model="searchKeyword"
          type="text"
          placeholder="在日志中快速搜索关键词 (如 IP, 域名, 邮箱)..."
          class="w-full bg-transparent text-white focus:outline-none placeholder-gray-500 font-mono text-[11px]"
        />
      </div>
      <div class="text-[11px] text-gray-500 font-mono">
        文件路径: <span class="text-gray-300">{{ logData?.filePath || '加载中...' }}</span>
      </div>
    </div>

    <!-- Terminal Log View Container -->
    <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl">
      <div class="flex items-center justify-between px-4 py-2 bg-gray-900/90 border-b border-gray-800 text-xs">
        <div class="flex items-center gap-2">
          <span class="w-2.5 h-2.5 rounded-full bg-rose-500/80 inline-block"></span>
          <span class="w-2.5 h-2.5 rounded-full bg-amber-500/80 inline-block"></span>
          <span class="w-2.5 h-2.5 rounded-full bg-emerald-500/80 inline-block"></span>
          <span class="ml-2 font-mono text-gray-400">{{ logType === 'access' ? 'access.log' : 'error.log' }}</span>
        </div>
        <div class="text-gray-500 font-mono text-[11px]">
          展示最近 {{ filteredLines.length }} 行
        </div>
      </div>

      <div
        ref="logBox"
        class="bg-[#050810] h-[550px] overflow-y-auto p-4 font-['JetBrains_Mono',monospace] text-[11px] leading-relaxed select-text"
      >
        <div
          v-for="(line, idx) in filteredLines"
          :key="idx"
          class="py-0.5 hover:bg-white/5 px-1 rounded transition-colors"
          :class="highlightLine(line)"
        >
          <span class="text-gray-600 select-none mr-3 inline-block w-8 text-right font-mono">{{ Number(idx) + 1 }}</span>
          <span>{{ line }}</span>
        </div>

        <div v-if="!filteredLines.length" class="text-center text-gray-600 py-20 font-sans">
          暂无符合条件的日志记录
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { RefreshCw, Search } from 'lucide-vue-next'
import api from '../api'

const logType = ref('access')
const autoRefresh = ref(true)
const loading = ref(false)
const searchKeyword = ref('')
const logData = ref<any>(null)
let timer: any = null

const fetchLogs = async () => {
  loading.value = true
  try {
    const res: any = await api.get(`/logs?type=${logType.value}&lines=150`)
    logData.value = res
  } catch (err) {
    console.error(err)
  } finally {
    loading.value = false
  }
}

const switchType = (type: string) => {
  logType.value = type
  fetchLogs()
}

const toggleAutoRefresh = () => {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    timer = setInterval(fetchLogs, 3000)
  } else if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const filteredLines = computed(() => {
  if (!logData.value?.lines) return []
  if (!searchKeyword.value) return logData.value.lines
  const kw = searchKeyword.value.toLowerCase()
  return logData.value.lines.filter((line: string) => line.toLowerCase().includes(kw))
})

const highlightLine = (line: string) => {
  const lower = line.toLowerCase()
  if (lower.includes('warning')) return 'text-amber-400'
  if (lower.includes('error') || lower.includes('failed') || lower.includes('rejected')) return 'text-rose-400'
  if (lower.includes('accepted')) return 'text-emerald-400'
  return 'text-gray-300'
}

onMounted(() => {
  fetchLogs()
  if (autoRefresh.value) {
    timer = setInterval(fetchLogs, 3000)
  }
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
