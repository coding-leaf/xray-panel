<template>
  <div class="space-y-5">
    <!-- Header & Controls Bar -->
    <div class="flex flex-col lg:flex-row items-start lg:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight flex items-center gap-2.5">
          <span>运行与访问日志</span>
          <span class="text-xs px-2.5 py-0.5 rounded-full font-mono bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
            Lightweight Inspector
          </span>
        </h1>
        <p class="text-xs text-gray-400 mt-1">后端 64KB 定块秒级解析 Xray 客户端访问流向、路由决策与错误诊断</p>
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

        <!-- Auto Refresh Toggle (默认关闭以保持零后台开销) -->
        <button
          @click="toggleAutoRefresh"
          class="px-3 py-2 rounded-xl text-xs border transition-all flex items-center gap-2 shadow-sm font-mono"
          :class="autoRefresh ? 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30 font-semibold' : 'bg-gray-900/80 text-gray-400 border-gray-800'"
        >
          <span class="w-2 h-2 rounded-full" :class="autoRefresh ? 'bg-emerald-400 animate-ping' : 'bg-gray-600'"></span>
          <span>{{ autoRefresh ? '自动刷新 (5s)' : '按需加载' }}</span>
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
            <option value="ERROR" class="bg-gray-900 text-rose-400">仅 Error (错误)</option>
            <option value="WARN" class="bg-gray-900 text-amber-400">仅 Warn (警告)</option>
            <option value="INFO" class="bg-gray-900 text-cyan-400">仅 Info (信息)</option>
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
          <option :value="1000">1000 行</option>
        </select>

        <!-- Sort Order (Desc / Asc) -->
        <button
          @click="toggleSortOrder"
          class="px-3 py-2 rounded-xl text-xs border transition-all flex items-center gap-1.5 shadow-sm font-mono"
          :class="sortOrder === 'desc' ? 'bg-indigo-500/15 text-indigo-300 border-indigo-500/30 font-semibold' : 'bg-gray-900/80 text-gray-400 border-gray-800'"
          :title="sortOrder === 'desc' ? '当前：最新记录在顶端 (倒序)' : '当前：旧记录在顶端 (正序)'"
        >
          <ArrowUpDown class="w-3.5 h-3.5" />
          <span>{{ sortOrder === 'desc' ? '最新在顶' : '正序排列' }}</span>
        </button>
      </div>
    </div>

    <!-- 1. Access Logs Structured View -->
    <div v-if="viewMode === 'table' && logType === 'access'" class="space-y-3">
      <!-- Desktop Access Table -->
      <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90 hidden md:block">
        <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
            <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
            <span>结构化访问记录 (共 {{ filteredAccessLogs.length }} 条)</span>
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
                v-for="(row, idx) in filteredAccessLogs"
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
                <td class="py-3 px-4 text-gray-300 whitespace-nowrap">{{ row.from_ip || '-' }}</td>

                <!-- Target Address -->
                <td class="py-3 px-4 text-white font-medium break-all max-w-[280px]">
                  <span class="text-cyan-400 font-semibold">{{ row.protocol || 'TCP' }}</span>
                  <span class="text-gray-500 mx-1">:</span>
                  <span class="text-gray-200">{{ row.target }}</span>
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
                    {{ row.action || 'accepted' }}
                  </span>
                </td>
              </tr>

              <tr v-if="!filteredAccessLogs.length">
                <td colspan="6" class="text-center py-16 text-gray-500 font-sans text-xs">
                  暂无符合条件的访问记录
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Mobile Access Log Stream -->
      <div class="space-y-2.5 md:hidden">
        <div
          v-for="(row, idx) in filteredAccessLogs"
          :key="idx"
          class="glass-panel p-3.5 rounded-2xl border border-gray-800 space-y-2 text-xs"
        >
          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-gray-400">{{ row.time }}</span>
            <span
              class="px-2 py-0.5 rounded text-[10px] font-bold"
              :class="row.action === 'accepted' ? 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30' : 'bg-rose-500/15 text-rose-400 border border-rose-500/30'"
            >
              {{ row.action || 'accepted' }}
            </span>
          </div>

          <div class="font-mono text-xs text-white break-all flex items-start gap-1">
            <span class="text-cyan-400 font-bold shrink-0">{{ row.protocol || 'TCP' }}</span>
            <span class="text-gray-500">:</span>
            <span class="text-gray-200">{{ row.target }}</span>
          </div>

          <div class="flex items-center justify-between text-[11px] font-mono pt-1.5 border-t border-white/[0.04]">
            <span class="text-indigo-300 font-medium truncate max-w-[160px]">{{ row.email || '匿名' }}</span>
            <span
              class="px-2 py-0.5 rounded text-[10px] font-bold border shrink-0"
              :class="getRouteBadgeClass(row.route)"
            >
              {{ row.route || 'direct' }}
            </span>
          </div>
        </div>

        <div v-if="!filteredAccessLogs.length" class="text-center py-12 text-gray-500 font-sans text-xs glass-panel rounded-2xl border border-gray-800">
          暂无符合条件的访问记录
        </div>
      </div>
    </div>

    <!-- 2. Error / Diagnostic Structured View -->
    <div v-else-if="viewMode === 'table' && logType === 'error'" class="space-y-3">
      <!-- Desktop Error Table -->
      <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90 hidden md:block">
        <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
            <span class="w-2 h-2 rounded-full bg-rose-400"></span>
            <span>结构化错误与诊断报告 (共 {{ filteredErrorLogs.length }} 条)</span>
          </div>
          <div class="text-gray-400 font-mono text-[11px]">
            后端即时故障诊断
          </div>
        </div>

        <div class="overflow-x-auto max-h-[620px] overflow-y-auto">
          <table class="w-full text-left text-[13px] border-collapse font-sans">
            <thead class="text-gray-400 bg-gray-950/80 border-b border-gray-800 sticky top-0 z-10 text-xs font-semibold uppercase tracking-wider backdrop-blur-md">
              <tr>
                <th class="py-3 px-4">时间</th>
                <th class="py-3 px-4">级别</th>
                <th class="py-3 px-4">来源模块</th>
                <th class="py-3 px-4">错误原因与诊断详情</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-800/50 font-mono text-xs">
              <tr
                v-for="(row, idx) in filteredErrorLogs"
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
                    {{ row.level || 'ERROR' }}
                  </span>
                </td>

                <!-- Module -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <span class="px-2 py-0.5 rounded bg-gray-800/80 text-gray-300 border border-gray-700 text-[11px]">
                    {{ row.module || 'core' }}
                  </span>
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

              <tr v-if="!filteredErrorLogs.length">
                <td colspan="4" class="text-center py-16 text-gray-500 font-sans text-xs">
                  暂无匹配的错误诊断日志 (系统运行平稳)
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Mobile Error Log Stream -->
      <div class="space-y-2.5 md:hidden">
        <div
          v-for="(row, idx) in filteredErrorLogs"
          :key="idx"
          class="glass-panel p-3.5 rounded-2xl border border-gray-800 space-y-2 text-xs"
        >
          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-gray-400">{{ row.time }}</span>
            <span
              class="px-2.5 py-0.5 rounded text-[10px] font-bold border font-mono"
              :class="getErrorLevelBadge(row.level)"
            >
              {{ row.level || 'ERROR' }}
            </span>
          </div>

          <div class="text-[11px] font-mono text-gray-400 flex items-center gap-1.5">
            <span class="px-2 py-0.5 rounded bg-gray-800/80 text-gray-300 border border-gray-700 text-[10px]">{{ row.module || 'core' }}</span>
          </div>

          <div class="text-xs font-mono text-gray-200 break-all leading-relaxed">{{ row.message }}</div>

          <div v-if="row.smartTip" class="text-[11px] text-amber-300 bg-amber-500/10 p-2.5 rounded-xl border border-amber-500/20 font-mono">
            💡 {{ row.smartTip }}
          </div>
        </div>

        <div v-if="!filteredErrorLogs.length" class="text-center py-12 text-gray-500 font-sans text-xs glass-panel rounded-2xl border border-gray-800">
          暂无匹配的错误诊断日志
        </div>
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

      <!-- Terminal Output Box -->
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
import { ref, shallowRef, computed, onMounted, onUnmounted } from 'vue'
import {
  RefreshCw,
  Search,
  Table,
  Terminal,
  User,
  ShieldAlert,
  ArrowUpDown,
} from 'lucide-vue-next'
import api from '../api'

const viewMode = ref<'table' | 'terminal'>('table')
const logType = ref<'access' | 'error'>('access')
const autoRefresh = ref(false)
const loading = ref(false)
const sortOrder = ref<'desc' | 'asc'>('desc')
const searchKeyword = ref('')
const selectedUserEmail = ref('')
const selectedLevel = ref('')
const maxLines = ref(200)

// 使用 shallowRef 避免 Vue 深度响应式开销
const rawAccessEntries = shallowRef<any[]>([])
const rawErrorEntries = shallowRef<any[]>([])
const rawLines = shallowRef<string[]>([])
const knownUsers = ref<any[]>([])
let timer: any = null

const toggleSortOrder = () => {
  sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
}

const fetchLogs = async () => {
  loading.value = true
  try {
    const res: any = await api.get(`/logs?type=${logType.value}&lines=${maxLines.value}`)
    if (res) {
      if (res.access) {
        rawAccessEntries.value = res.access
      }
      if (res.errors) {
        rawErrorEntries.value = res.errors
      }
      if (res.lines) {
        rawLines.value = res.lines
      }
    }
  } catch (err) {
    console.error('Failed to fetch logs', err)
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

const switchType = (type: 'access' | 'error') => {
  if (logType.value === type) return
  logType.value = type
  fetchLogs()
}

const toggleAutoRefresh = () => {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    timer = setInterval(fetchLogs, 5000)
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
  for (const item of rawAccessEntries.value) {
    if (item.email) emailSet.add(item.email)
  }
  return Array.from(emailSet)
})

// 1. 结构化 Access 访问日志 (直接基于后端已解析结构，无前端正则开销)
const filteredAccessLogs = computed(() => {
  if (logType.value !== 'access') return []

  const kw = searchKeyword.value.toLowerCase()
  const filterEmail = selectedUserEmail.value.toLowerCase()

  const list = rawAccessEntries.value.filter((item) => {
    if (filterEmail && (!item.email || !item.email.toLowerCase().includes(filterEmail))) {
      return false
    }
    if (kw) {
      const match =
        (item.target && item.target.toLowerCase().includes(kw)) ||
        (item.from_ip && item.from_ip.includes(kw)) ||
        (item.route && item.route.toLowerCase().includes(kw)) ||
        (item.email && item.email.toLowerCase().includes(kw))
      if (!match) return false
    }
    return true
  })

  return sortOrder.value === 'desc' ? [...list].reverse() : list
})

// 2. 结构化 Error 诊断日志
const filteredErrorLogs = computed(() => {
  if (logType.value !== 'error') return []

  const kw = searchKeyword.value.toLowerCase()
  const filterLvl = selectedLevel.value.toUpperCase()

  const list = rawErrorEntries.value.filter((item) => {
    if (filterLvl && item.level !== filterLvl) {
      return false
    }
    if (kw) {
      const match =
        (item.message && item.message.toLowerCase().includes(kw)) ||
        (item.module && item.module.toLowerCase().includes(kw)) ||
        (item.smartTip && item.smartTip.toLowerCase().includes(kw))
      if (!match) return false
    }
    return true
  })

  return sortOrder.value === 'desc' ? [...list].reverse() : list
})

// 3. 原始终端高亮筛选
const filteredRawLines = computed(() => {
  if (!rawLines.value) return []
  const kw = searchKeyword.value.toLowerCase()
  const filterEmail = selectedUserEmail.value.toLowerCase()
  const filterLvl = selectedLevel.value.toLowerCase()

  const list = rawLines.value.filter((line: string) => {
    const l = line.toLowerCase()
    if (kw && !l.includes(kw)) return false
    if (logType.value === 'access' && filterEmail && !l.includes(filterEmail)) return false
    if (logType.value === 'error' && filterLvl && !l.includes(filterLvl)) return false
    return true
  })

  return sortOrder.value === 'desc' ? [...list].reverse() : list
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
  if (lower.includes('warn') || lower.includes('[warning]')) return 'text-amber-300 bg-amber-500/5'
  if (lower.includes('error') || lower.includes('[error]') || lower.includes('failed') || lower.includes('rejected')) return 'text-rose-300 bg-rose-500/10 font-bold'
  if (lower.includes('accepted')) return 'text-emerald-300'
  if (lower.includes('warp')) return 'text-cyan-300'
  if (lower.includes('doh')) return 'text-indigo-300'
  return 'text-gray-300'
}

onMounted(() => {
  fetchLogs()
  fetchUsers()
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
