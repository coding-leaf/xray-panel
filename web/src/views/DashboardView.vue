<template>
  <div class="space-y-6">
    <!-- Header banner -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">运行监控与仪表盘</h1>
        <p class="text-xs text-gray-400 mt-0.5">实时系统负载、网络带宽与 Xray 核心运行状态</p>
      </div>
      <div class="flex items-center gap-3">
        <span
          class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-xs font-semibold"
          :class="dashboard?.metrics?.xrayRunning ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'"
        >
          <span class="w-2 h-2 rounded-full" :class="dashboard?.metrics?.xrayRunning ? 'bg-emerald-400 animate-pulse' : 'bg-rose-400'"></span>
          <span>Xray {{ dashboard?.metrics?.xrayRunning ? '运行中' : '已停止' }}</span>
        </span>
        <button
          @click="fetchData"
          class="px-3 py-1.5 rounded-xl text-xs bg-gray-800/80 hover:bg-gray-700/80 text-gray-200 border border-gray-700/60 transition-colors flex items-center gap-1.5"
        >
          <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': refreshing }" />
          <span>刷新</span>
        </button>
      </div>
    </div>

    <!-- Quick Stats 4 Grid Cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
      <!-- CPU -->
      <div class="glass-card p-5 rounded-2xl relative overflow-hidden">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-400">CPU 使用率</span>
          <div class="p-2 rounded-xl bg-blue-500/10 text-blue-400">
            <Cpu class="w-4 h-4" />
          </div>
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-2xl font-extrabold text-white">{{ dashboard?.metrics?.cpuUsagePercent?.toFixed(1) || 0 }}%</span>
        </div>
        <div class="mt-3 w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
          <div
            class="bg-blue-500 h-1.5 rounded-full transition-all duration-500"
            :style="{ width: `${Math.min(dashboard?.metrics?.cpuUsagePercent || 0, 100)}%` }"
          ></div>
        </div>
      </div>

      <!-- RAM -->
      <div class="glass-card p-5 rounded-2xl relative overflow-hidden">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-400">内存占用</span>
          <div class="p-2 rounded-xl bg-purple-500/10 text-purple-400">
            <Activity class="w-4 h-4" />
          </div>
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-2xl font-extrabold text-white">{{ dashboard?.metrics?.memoryUsagePercent?.toFixed(1) || 0 }}%</span>
          <span class="text-xs text-gray-400 font-mono">
            {{ formatBytes(dashboard?.metrics?.memoryUsedBytes || 0) }} / {{ formatBytes(dashboard?.metrics?.memoryTotalBytes || 0) }}
          </span>
        </div>
        <div class="mt-3 w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
          <div
            class="bg-purple-500 h-1.5 rounded-full transition-all duration-500"
            :style="{ width: `${Math.min(dashboard?.metrics?.memoryUsagePercent || 0, 100)}%` }"
          ></div>
        </div>
      </div>

      <!-- Disk -->
      <div class="glass-card p-5 rounded-2xl relative overflow-hidden">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-400">磁盘空间</span>
          <div class="p-2 rounded-xl bg-amber-500/10 text-amber-400">
            <HardDrive class="w-4 h-4" />
          </div>
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-2xl font-extrabold text-white">{{ dashboard?.metrics?.diskUsagePercent?.toFixed(1) || 0 }}%</span>
          <span class="text-xs text-gray-400 font-mono">
            {{ formatBytes(dashboard?.metrics?.diskUsedBytes || 0) }} / {{ formatBytes(dashboard?.metrics?.diskTotalBytes || 0) }}
          </span>
        </div>
        <div class="mt-3 w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
          <div
            class="bg-amber-500 h-1.5 rounded-full transition-all duration-500"
            :style="{ width: `${Math.min(dashboard?.metrics?.diskUsagePercent || 0, 100)}%` }"
          ></div>
        </div>
      </div>

      <!-- Active Users -->
      <div class="glass-card p-5 rounded-2xl relative overflow-hidden">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-gray-400">用户统计</span>
          <div class="p-2 rounded-xl bg-emerald-500/10 text-emerald-400">
            <Users class="w-4 h-4" />
          </div>
        </div>
        <div class="mt-3 flex items-baseline gap-2">
          <span class="text-2xl font-extrabold text-white">{{ dashboard?.activeUsers || 0 }}</span>
          <span class="text-xs text-gray-400 font-mono">/ {{ dashboard?.userCount || 0 }} 活跃中</span>
        </div>
        <div class="mt-3 text-xs text-gray-400">
          已配置节点: <span class="text-white font-semibold">{{ dashboard?.inbounds?.length || 0 }}</span> 个
        </div>
      </div>
    </div>

    <!-- Network Bandwidth & Traffic Section -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
      <!-- Real-time Speed Card -->
      <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 lg:col-span-2 space-y-6">
        <h2 class="text-base font-bold text-white flex items-center gap-2">
          <Zap class="w-4 h-4 text-cyan-400" />
          <span>实时网络速率与总吞吐量</span>
        </h2>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div class="bg-gray-900/60 border border-gray-800/80 p-4 rounded-xl flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="p-2.5 rounded-xl bg-cyan-500/10 text-cyan-400">
                <ArrowUpRight class="w-5 h-5" />
              </div>
              <div>
                <p class="text-xs text-gray-400 font-medium">实时上行速率</p>
                <p class="text-xl font-mono font-bold text-white mt-0.5">
                  {{ formatSpeed(dashboard?.metrics?.netUpSpeedBps || 0) }}
                </p>
              </div>
            </div>
            <div class="text-right">
              <p class="text-[11px] text-gray-500">累计上行</p>
              <p class="text-xs font-mono text-gray-300">{{ formatBytes(dashboard?.metrics?.netTotalSent || dashboard?.totalUp || 0) }}</p>
            </div>
          </div>

          <div class="bg-gray-900/60 border border-gray-800/80 p-4 rounded-xl flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="p-2.5 rounded-xl bg-indigo-500/10 text-indigo-400">
                <ArrowDownRight class="w-5 h-5" />
              </div>
              <div>
                <p class="text-xs text-gray-400 font-medium">实时下行速率</p>
                <p class="text-xl font-mono font-bold text-white mt-0.5">
                  {{ formatSpeed(dashboard?.metrics?.netDownSpeedBps || 0) }}
                </p>
              </div>
            </div>
            <div class="text-right">
              <p class="text-[11px] text-gray-500">累计下行</p>
              <p class="text-xs font-mono text-gray-300">{{ formatBytes(dashboard?.metrics?.netTotalRecv || dashboard?.totalDown || 0) }}</p>
            </div>
          </div>
        </div>

        <!-- Inbound status table preview -->
        <div>
          <h3 class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-3">入站节点概要</h3>
          <div class="overflow-x-auto">
            <table class="w-full text-left text-xs">
              <thead class="text-gray-400 bg-gray-900/40 border-y border-gray-800">
                <tr>
                  <th class="py-2.5 px-3">标签</th>
                  <th class="py-2.5 px-3">端口</th>
                  <th class="py-2.5 px-3">协议</th>
                  <th class="py-2.5 px-3">状态</th>
                  <th class="py-2.5 px-3 text-right">总流量</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-800/60">
                <tr v-for="inb in dashboard?.inbounds || []" :key="inb.id" class="hover:bg-gray-800/30">
                  <td class="py-2.5 px-3 font-medium text-white">{{ inb.tag }}</td>
                  <td class="py-2.5 px-3 font-mono text-brand-400">{{ inb.port }}</td>
                  <td class="py-2.5 px-3 uppercase font-mono">{{ inb.protocol }}</td>
                  <td class="py-2.5 px-3">
                    <span class="px-2 py-0.5 rounded-full text-[10px] font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                      正常
                    </span>
                  </td>
                  <td class="py-2.5 px-3 text-right font-mono text-gray-300">
                    {{ formatBytes(inb.upBytes + inb.downBytes) }}
                  </td>
                </tr>
                <tr v-if="!dashboard?.inbounds?.length">
                  <td colspan="5" class="py-4 text-center text-gray-500">暂无入站节点</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Service Info & Quick Ops -->
      <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-5">
        <h2 class="text-base font-bold text-white flex items-center gap-2">
          <Server class="w-4 h-4 text-brand-400" />
          <span>服务与系统信息</span>
        </h2>

        <div class="space-y-3 text-xs">
          <div class="flex justify-between py-2 border-b border-gray-800/60">
            <span class="text-gray-400">Xray 核心版本</span>
            <span class="text-white font-mono font-medium">{{ dashboard?.metrics?.xrayVersion || '未知' }}</span>
          </div>
          <div class="flex justify-between py-2 border-b border-gray-800/60">
            <span class="text-gray-400">系统开机时长</span>
            <span class="text-white font-mono">{{ formatUptime(dashboard?.metrics?.uptimeSeconds || 0) }}</span>
          </div>
          <div class="flex justify-between py-2 border-b border-gray-800/60">
            <span class="text-gray-400">服务子状态</span>
            <span class="text-gray-300 font-mono">{{ dashboard?.service?.subState || 'running' }}</span>
          </div>
        </div>

        <div class="pt-2">
          <h3 class="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2.5">快捷运维指令</h3>
          <button
            @click="restartXray"
            :disabled="restarting"
            class="w-full bg-gray-800 hover:bg-gray-700 text-white font-medium py-2.5 rounded-xl text-xs transition-colors border border-gray-700 flex items-center justify-center gap-2 disabled:opacity-50"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': restarting }" />
            <span>{{ restarting ? '正在平滑重启...' : '平滑重启 Xray 核心' }}</span>
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import {
  Cpu,
  Activity,
  HardDrive,
  Users,
  Zap,
  ArrowUpRight,
  ArrowDownRight,
  Server,
  RefreshCw,
} from 'lucide-vue-next'
import api from '../api'

const dashboard = ref<any>(null)
const refreshing = ref(false)
const restarting = ref(false)
let timer: any = null

const fetchData = async () => {
  refreshing.value = true
  try {
    dashboard.value = await api.get('/dashboard')
  } catch (err) {
    console.error(err)
  } finally {
    refreshing.value = false
  }
}

const restartXray = async () => {
  if (!confirm('确认平滑重启 Xray 核心服务吗？')) return
  restarting.value = true
  try {
    const raw: any = await api.get('/config/raw')
    await api.post('/config/save', raw)
    await fetchData()
  } catch (err: any) {
    alert('重启失败: ' + err)
  } finally {
    restarting.value = false
  }
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

const formatSpeed = (bps: number) => {
  return formatBytes(bps) + '/s'
}

const formatUptime = (seconds: number) => {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days}天 ${hours}小时 ${mins}分`
  return `${hours}小时 ${mins}分`
}

onMounted(() => {
  fetchData()
  timer = setInterval(fetchData, 4000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
