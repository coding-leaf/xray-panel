<template>
  <div class="space-y-5">
    <!-- Header & Controls Bar -->
    <div class="flex flex-col lg:flex-row items-start lg:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight flex items-center gap-2.5">
          <span>运行与访问日志</span>
          <span class="text-xs px-2.5 py-0.5 rounded-full font-mono bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
            Realtime Inspector
          </span>
        </h1>
        <p class="text-xs text-gray-400 mt-1">深度解析 Xray 客户端访问流向、路由分流决策与运行错误诊断</p>
      </div>

      <div class="flex flex-wrap items-center gap-2.5">
        <!-- View Mode (Table / Terminal) -->
        <div class="bg-gray-900/90 p-1 rounded-xl border border-gray-800 flex shadow-sm">
          <button
            @click="viewMode = 'table'"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5"
            :class="viewMode === 'table' ? 'bg-indigo-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            <Table class="w-3.5 h-3.5" />
            <span>结构化表格</span>
          </button>
          <button
            @click="viewMode = 'terminal'"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5"
            :class="viewMode === 'terminal' ? 'bg-indigo-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            <Terminal class="w-3.5 h-3.5" />
            <span>原始终端</span>
          </button>
        </div>

        <!-- Log Type (Access / Error) -->
        <div class="bg-gray-900/90 p-1 rounded-xl border border-gray-800 flex shadow-sm">
          <button
            @click="switchType('access')"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'access' ? 'bg-emerald-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            访问日志 (Access)
          </button>
          <button
            @click="switchType('error')"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'error' ? 'bg-rose-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            错误诊断 (Error)
          </button>
        </div>

        <!-- Auto Refresh Toggle -->
        <button
          @click="toggleAutoRefresh"
          class="px-3 py-2 rounded-xl text-xs border transition-all flex items-center gap-2 shadow-sm"
          :class="autoRefresh ? 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30 font-semibold' : 'bg-gray-900/80 text-gray-400 border-gray-800'"
        >
          <span class="w-2 h-2 rounded-full" :class="autoRefresh ? 'bg-emerald-400 animate-ping' : 'bg-gray-600'"></span>
          <span>{{ autoRefresh ? '自动刷新 (3s)' : '自动刷新已关' }}</span>
        </button>

        <button
          @click="fetchLogs"
          :disabled="loading"
          class="p-2.5 rounded-xl bg-gray-900 hover:bg-gray-800 text-gray-200 border border-gray-800 transition-colors shadow-sm"
          title="手动刷新"
        >
          <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        </button>
      </div>
    </div>

    <!-- Filters & User Selector Card -->
    <div class="glass-panel p-3.5 sm:p-4 rounded-2xl border border-gray-800/80 flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 bg-[#0a0d14]/70">
      <!-- Search Input -->
      <div class="flex items-center gap-2.5 flex-1 bg-black/40 px-3.5 py-2 rounded-xl border border-gray-800/80 focus-within:border-indigo-500/60 transition-colors">
        <Search class="w-4 h-4 text-gray-500 shrink-0" />
        <input
          v-model="searchKeyword"
          type="text"
          :placeholder="logType === 'access' ? '快速搜索关键词 (如 IP、目标域名、分流出站)...' : '快速搜索错误内容 (如 DNS, account, timeout, rejected)...'"
          class="w-full bg-transparent text-white focus:outline-none placeholder-gray-500 font-mono text-xs"
        />
        <button v-if="searchKeyword" @click="searchKeyword = ''" class="text-gray-500 hover:text-white text-xs">
          ✕
        </button>
      </div>

      <!-- Filters depending on type -->
      <div class="flex flex-wrap items-center gap-2 shrink-0">
        <!-- User Dropdown Filter (Access Mode) -->
        <div v-if="logType === 'access'" class="flex items-center gap-1.5 bg-black/40 px-3 py-1.5 rounded-xl border border-gray-800 text-xs">
          <User class="w-3.5 h-3.5 text-indigo-400 shrink-0" />
          <span class="text-gray-400 shrink-0">用户:</span>
          <select
            v-model="selectedUserEmail"
            class="bg-transparent text-white font-mono font-medium focus:outline-none cursor-pointer pr-2 text-xs"
          >
            <option value="" class="bg-gray-900 text-gray-300">全部用户</option>
            <option v-for="email in userOptions" :key="email" :value="email" class="bg-gray-900 text-cyan-300">
              {{ email }}
            </option>
          </select>
        </div>

        <!-- Level Filter (Error Mode) -->
        <div v-else class="flex items-center gap-1.5 bg-black/40 px-3 py-1.5 rounded-xl border border-gray-800 text-xs">
          <ShieldAlert class="w-3.5 h-3.5 text-amber-400 shrink-0" />
          <span class="text-gray-400 shrink-0">级别:</span>
          <select
            v-model="selectedLevel"
            class="bg-transparent text-white font-mono font-medium focus:outline-none cursor-pointer pr-2 text-xs"
          >
            <option value="" class="bg-gray-900 text-gray-300">全部级别</option>
            <option value="Error" class="bg-gray-900 text-rose-400">仅 Error (错误)</option>
            <option value="Warning" class="bg-gray-900 text-amber-400">仅 Warning (警告)</option>
            <option value="Info" class="bg-gray-900 text-cyan-400">仅 Info (信息)</option>
          </select>
        </div>

        <!-- Lines Selector -->
        <select
          v-model="maxLines"
          @change="fetchLogs"
          class="bg-black/40 text-gray-300 px-3 py-2 rounded-xl border border-gray-800 text-xs font-mono focus:outline-none cursor-pointer"
        >
          <option :value="100">100 行</option>
          <option :value="200">200 行</option>
          <option :value="500">500 行</option>
        </select>
      </div>
    </div>

    <!-- 1. Access Logs Structured Table View -->
    <div v-if="viewMode === 'table' && logType === 'access'" class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90">
      <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
        <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
          <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
          <span>结构化访问记录 (共匹配 {{ parsedAccessLogs.length }} 条)</span>
        </div>
        <div v-if="selectedUserEmail" class="text-cyan-400 font-mono text-[11px] bg-cyan-950/40 px-2 py-0.5 rounded border border-cyan-800/40">
          过滤用户: {{ selectedUserEmail }}
        </div>
      </div>

      <div class="overflow-x-auto max-h-[620px] overflow-y-auto">
        <table class="w-full text-left text-[13px] border-collapse font-sans">
          <thead class="text-gray-400 bg-gray-950/80 border-b border-gray-800 sticky top-0 z-10 text-xs font-semibold uppercase tracking-wider backdrop-blur-md">
            <tr>
              <th class="py-3 px-4">时间</th>
              <th class="py-3 px-4">用户 Email</th>
              <th class="py-3 px-4">客户端源</th>
              <th class="py-3 px-4">目标地址 / 端口</th>
              <th class="py-3 px-4">分流出站路由</th>
              <th class="py-3 px-4 text-center">状态</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-800/50 font-mono text-xs">
            <tr
              v-for="(row, idx) in parsedAccessLogs"
              :key="idx"
              class="hover:bg-white/[0.03] transition-colors"
            >
              <td class="py-3 px-4 text-gray-400 whitespace-nowrap">{{ row.time }}</td>

              <!-- User Email -->
              <td class="py-3 px-4 whitespace-nowrap">
                <span
                  v-if="row.email"
                  @click="selectedUserEmail = row.email"
                  class="px-2.5 py-1 rounded-lg text-[11px] font-semibold bg-indigo-500/10 text-indigo-300 border border-indigo-500/20 hover:bg-indigo-500/20 cursor-pointer transition-all inline-block"
                  title="点击以此用户过滤"
                >
                  {{ row.email }}
                </span>
                <span v-else class="text-gray-500 text-[11px] italic">匿名 / 系统</span>
              </td>

              <!-- Client IP -->
              <td class="py-3 px-4 text-gray-300 whitespace-nowrap">{{ row.clientIp || '-' }}</td>

              <!-- Target Address -->
              <td class="py-3 px-4 text-white font-medium break-all max-w-[280px]">
                <span class="text-cyan-400 font-semibold">{{ row.protocol }}</span>
                <span class="text-gray-500 mx-1">:</span>
                <span class="text-gray-200">{{ row.targetHost }}</span>
              </td>

              <!-- Routing Decision -->
              <td class="py-3 px-4 whitespace-nowrap">
                <span
                  class="px-2.5 py-1 rounded-lg text-[11px] font-bold border"
                  :class="getRouteBadgeClass(row.route)"
                >
                  {{ row.route || 'direct' }}
                </span>
              </td>

              <!-- Action / Status -->
              <td class="py-3 px-4 text-center whitespace-nowrap">
                <span
                  class="px-2 py-0.5 rounded text-[10px] font-semibold font-mono"
                  :class="row.action === 'accepted' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'"
                >
                  {{ row.action }}
                </span>
              </td>
            </tr>

            <tr v-if="!parsedAccessLogs.length">
              <td colspan="6" class="text-center py-16 text-gray-500 font-sans text-xs">
                暂无符合条件的访问记录
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 2. Error / Diagnostic Structured Table View -->
    <div v-else-if="viewMode === 'table' && logType === 'error'" class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90">
      <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
        <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
          <span class="w-2 h-2 rounded-full bg-rose-400"></span>
          <span>结构化错误与诊断报告 (共匹配 {{ parsedErrorLogs.length }} 条)</span>
        </div>
        <div class="text-gray-400 font-mono text-[11px]">
          点击关键词可快速过滤排查
        </div>
      </div>

      <div class="overflow-x-auto max-h-[620px] overflow-y-auto">
        <table class="w-full text-left text-[13px] border-collapse font-sans">
          <thead class="text-gray-400 bg-gray-950/80 border-b border-gray-800 sticky top-0 z-10 text-xs font-semibold uppercase tracking-wider backdrop-blur-md">
            <tr>
              <th class="py-3 px-4">时间</th>
              <th class="py-3 px-4">级别</th>
              <th class="py-3 px-4">来源模块</th>
              <th class="py-3 px-4">连接 ID</th>
              <th class="py-3 px-4">错误原因与诊断详情</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-800/50 font-mono text-xs">
            <tr
              v-for="(row, idx) in parsedErrorLogs"
              :key="idx"
              class="hover:bg-white/[0.03] transition-colors"
            >
              <!-- Time -->
              <td class="py-3 px-4 text-gray-400 whitespace-nowrap">{{ row.time }}</td>

              <!-- Level -->
              <td class="py-3 px-4 whitespace-nowrap">
                <span
                  class="px-2.5 py-0.5 rounded text-[11px] font-bold border font-mono"
                  :class="getErrorLevelBadge(row.level)"
                >
                  {{ row.level }}
                </span>
              </td>

              <!-- Module -->
              <td class="py-3 px-4 whitespace-nowrap">
                <span class="px-2 py-0.5 rounded bg-gray-800/80 text-gray-300 border border-gray-700 text-[11px]">
                  {{ row.module }}
                </span>
              </td>

              <!-- Conn ID -->
              <td class="py-3 px-4 text-gray-500 whitespace-nowrap font-mono text-[11px]">
                {{ row.connId ? `[${row.connId}]` : '-' }}
              </td>

              <!-- Message & Diagnosis -->
              <td class="py-3 px-4 text-gray-200 break-all leading-relaxed font-sans text-xs">
                <div class="flex items-start gap-2">
                  <span class="font-mono text-gray-300">{{ row.message }}</span>
                </div>
                <div v-if="row.smartTip" class="mt-1 text-[11px] text-amber-300/90 flex items-center gap-1 font-mono">
                  <span>💡 诊断建议: {{ row.smartTip }}</span>
                </div>
              </td>
            </tr>

            <tr v-if="!parsedErrorLogs.length">
              <td colspan="5" class="text-center py-16 text-gray-500 font-sans text-xs">
                暂无匹配的错误诊断日志 (系统运行平稳)
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 3. Terminal High-Contrast Log View (Raw Mode) -->
    <div v-else class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#04060B]">
      <!-- Terminal Header -->
      <div class="flex items-center justify-between px-4 py-2.5 bg-gray-900/90 border-b border-gray-800 text-xs">
        <div class="flex items-center gap-2">
          <span class="w-3 h-3 rounded-full bg-rose-500/80 inline-block shadow-sm"></span>
          <span class="w-3 h-3 rounded-full bg-amber-500/80 inline-block shadow-sm"></span>
          <span class="w-3 h-3 rounded-full bg-emerald-500/80 inline-block shadow-sm"></span>
          <span class="ml-2 font-mono text-gray-300 font-semibold">
            {{ logType === 'access' ? 'xray-access.log' : 'xray-error.log' }}
          </span>
        </div>
        <div class="text-gray-400 font-mono text-xs">
          展示最近 {{ filteredRawLines.length }} 行
        </div>
      </div>

      <!-- Terminal Output Box (Font Size 13.5px) -->
      <div
        ref="logBox"
        class="h-[600px] overflow-y-auto p-4 sm:p-5 font-['JetBrains_Mono',monospace] text-[13.5px] leading-[1.7] select-text space-y-0.5"
      >
        <div
          v-for="(line, idx) in filteredRawLines"
          :key="idx"
          class="py-1 px-2 rounded hover:bg-white/[0.04] transition-colors flex items-start gap-3"
          :class="highlightLine(line)"
        >
          <span class="text-gray-600 select-none shrink-0 w-10 text-right font-mono text-xs pt-0.5">
            {{ Number(idx) + 1 }}
          </span>
          <span class="break-all whitespace-pre-wrap flex-1">{{ line }}</span>
        </div>

        <div v-if="!filteredRawLines.length" class="text-center text-gray-600 py-24 font-sans text-xs">
          暂无符合条件的日志记录
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  RefreshCw,
  Search,
  Table,
  Terminal,
  User,
  ShieldAlert,
} from 'lucide-vue-next'
import api from '../api'

const viewMode = ref<'table' | 'terminal'>('table')
const logType = ref('access')
const autoRefresh = ref(true)
const loading = ref(false)
const searchKeyword = ref('')
const selectedUserEmail = ref('')
const selectedLevel = ref('')
const maxLines = ref(200)
const logData = ref<any>(null)
const knownUsers = ref<any[]>([])
let timer: any = null

const fetchLogs = async () => {
  loading.value = true
  try {
    const res: any = await api.get(`/logs?type=${logType.value}&lines=${maxLines.value}`)
    logData.value = res
  } catch (err) {
    console.error(err)
  } finally {
    loading.value = false
  }
}

const fetchUsers = async () => {
  try {
    const uList: any = await api.get('/users')
    knownUsers.value = uList || []
  } catch (e) {
    console.error(e)
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

// 收集所有已知与日志中解析出的 User Email
const userOptions = computed(() => {
  const emailSet = new Set<string>()
  for (const u of knownUsers.value) {
    if (u.email) emailSet.add(u.email)
  }
  if (logData.value?.lines) {
    for (const line of logData.value.lines) {
      const match = line.match(/email:\s*([^\s]+)/)
      if (match && match[1]) {
        emailSet.add(match[1])
      }
    }
  }
  return Array.from(emailSet)
})

// 1. 结构化解析 Access 访问日志
const parsedAccessLogs = computed(() => {
  if (!logData.value?.lines || logType.value !== 'access') return []

  const list: any[] = []
  const kw = searchKeyword.value.toLowerCase()
  const filterEmail = selectedUserEmail.value.toLowerCase()

  for (const line of logData.value.lines) {
    if (kw && !line.toLowerCase().includes(kw)) continue
    if (filterEmail && !line.toLowerCase().includes(filterEmail)) continue

    const match = line.match(/^(\d{4}\/\d{2}\/\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+from\s+([^\s]+)\s+(\w+)\s+([^\s]+)\s+\[([^\]]+)\](?:\s+email:\s+([^\s]+))?/)

    if (match) {
      const fullTarget = match[4]
      let proto = 'tcp'
      let targetHost = fullTarget
      if (fullTarget.startsWith('tcp:')) {
        proto = 'TCP'
        targetHost = fullTarget.substring(4)
      } else if (fullTarget.startsWith('udp:')) {
        proto = 'UDP'
        targetHost = fullTarget.substring(4)
      }

      list.push({
        raw: line,
        time: match[1],
        clientIp: match[2],
        action: match[3],
        targetHost,
        protocol: proto,
        route: match[5],
        email: match[6] || '',
      })
    } else if (line.includes('DOH') || line.includes('answer:')) {
      list.push({
        raw: line,
        time: line.substring(0, 19),
        clientIp: '127.0.0.1 (DNS)',
        action: 'query',
        targetHost: line.substring(line.indexOf('answer:') > 0 ? line.indexOf('answer:') : 20),
        protocol: 'DNS',
        route: 'dns-resolver',
        email: '',
      })
    }
  }

  return list
})

// 2. 结构化解析 Error 诊断日志
const parsedErrorLogs = computed(() => {
  if (!logData.value?.lines || logType.value !== 'error') return []

  const list: any[] = []
  const kw = searchKeyword.value.toLowerCase()
  const filterLvl = selectedLevel.value.toLowerCase()

  for (const line of logData.value.lines) {
    if (kw && !line.toLowerCase().includes(kw)) continue

    // 匹配: 2026/08/28 01:18:29.038336 [Error] (可选 [3373508809]) module: message
    const match = line.match(/^(\d{4}\/\d{2}\/\d{2}\s+\d{2}:\d{2}:\d{2}(?:\.\d+)?)\s+\[(Debug|Info|Warning|Error)\](?:\s+\[(\d+)\])?\s+([^:]+):\s+(.*)$/i)

    if (match) {
      const lvl = match[2].toUpperCase()
      if (filterLvl && !lvl.toLowerCase().includes(filterLvl)) continue

      const moduleName = match[4]
      const msg = match[5]
      let smartTip = ''

      if (msg.includes('client flow is empty')) {
        smartTip = '客户端未配置 Vision 流控，或当前入站协议不为 TCP'
      } else if (msg.includes('dns-query') && msg.includes('context canceled')) {
        smartTip = 'DoH 远端解析握手超时，建议优先使用 8.8.8.8 UDP DNS'
      } else if (msg.includes('NoKernelTun')) {
        smartTip = 'WireGuard WARP 采用用户态 gVisor TUN 启动成功'
      } else if (msg.includes('started')) {
        smartTip = 'Xray 核心服务已成功初始化并加载配置'
      }

      list.push({
        raw: line,
        time: match[1],
        level: lvl,
        connId: match[3] || '',
        module: moduleName,
        message: msg,
        smartTip,
      })
    } else {
      // 备用未归类行
      if (!filterLvl) {
        list.push({
          raw: line,
          time: line.substring(0, 19) || '-',
          level: line.includes('Error') ? 'ERROR' : (line.includes('Warning') ? 'WARN' : 'INFO'),
          connId: '',
          module: 'system',
          message: line,
          smartTip: '',
        })
      }
    }
  }

  return list
})

// 3. 原始终端高亮筛选
const filteredRawLines = computed(() => {
  if (!logData.value?.lines) return []
  const kw = searchKeyword.value.toLowerCase()
  const filterEmail = selectedUserEmail.value.toLowerCase()
  const filterLvl = selectedLevel.value.toLowerCase()

  return logData.value.lines.filter((line: string) => {
    const l = line.toLowerCase()
    if (kw && !l.includes(kw)) return false
    if (logType.value === 'access' && filterEmail && !l.includes(filterEmail)) return false
    if (logType.value === 'error' && filterLvl && !l.includes(filterLvl)) return false
    return true
  })
})

const getRouteBadgeClass = (route: string) => {
  if (!route) return 'bg-gray-800 text-gray-400 border-gray-700'
  if (route.includes('warp')) return 'bg-cyan-500/10 text-cyan-300 border-cyan-500/30'
  if (route.includes('direct')) return 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
  if (route.includes('block')) return 'bg-rose-500/10 text-rose-300 border-rose-500/30'
  return 'bg-indigo-500/10 text-indigo-300 border-indigo-500/30'
}

const getErrorLevelBadge = (level: string) => {
  const l = (level || '').toUpperCase()
  if (l.includes('ERROR')) return 'bg-rose-500/15 text-rose-400 border-rose-500/30'
  if (l.includes('WARN')) return 'bg-amber-500/15 text-amber-400 border-amber-500/30'
  if (l.includes('INFO')) return 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30'
  return 'bg-gray-800 text-gray-300 border-gray-700'
}

const highlightLine = (line: string) => {
  const lower = line.toLowerCase()
  if (lower.includes('warning') || lower.includes('[warning]')) return 'text-amber-300 bg-amber-500/5'
  if (lower.includes('error') || lower.includes('[error]') || lower.includes('failed') || lower.includes('rejected')) return 'text-rose-300 bg-rose-500/10 font-bold'
  if (lower.includes('accepted')) return 'text-emerald-300'
  if (lower.includes('warp')) return 'text-cyan-300'
  if (lower.includes('doh')) return 'text-indigo-300'
  return 'text-gray-300'
}

onMounted(() => {
  fetchLogs()
  fetchUsers()
  if (autoRefresh.value) {
    timer = setInterval(fetchLogs, 3000)
  }
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
