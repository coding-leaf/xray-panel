<template>
  <div class="space-y-4">
    <!-- Header banner -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">运行监控与仪表盘</h1>
          <Badge
            :variant="dashboard?.metrics?.xrayRunning ? 'default' : 'destructive'"
            class="text-[10px] font-mono flex items-center gap-1"
          >
            <span class="w-1.5 h-1.5 rounded-full" :class="dashboard?.metrics?.xrayRunning ? 'bg-emerald-400 animate-pulse' : 'bg-rose-400'"></span>
            <span>Xray {{ dashboard?.metrics?.xrayRunning ? '运行中' : '已停止' }}</span>
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          实时系统资源负载、网络实时吞吐与 Xray 核心运行指标
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          :loading="refreshing"
          @click="fetchData"
          title="刷新最新运行状态"
        >
          <RefreshCw class="w-3.5 h-3.5 mr-1" />
          <span>刷新</span>
        </Button>
      </div>
    </div>

    <!-- Reality 域名异常/临期告警横幅 -->
    <div
      v-if="realityAlertCount > 0"
      class="p-3 rounded-lg border text-xs flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3"
      :class="realitySummary?.errorCount ? 'bg-rose-950/20 border-rose-800/40 text-rose-300' : 'bg-amber-950/20 border-amber-800/40 text-amber-300'"
    >
      <div class="flex items-center gap-2.5">
        <component :is="realitySummary?.errorCount ? AlertCircle : AlertTriangle" class="w-4 h-4 shrink-0" :class="realitySummary?.errorCount ? 'text-rose-400' : 'text-amber-400'" />
        <div>
          <p class="font-semibold text-xs">
            {{ realitySummary?.errorCount ? 'Reality 伪装域名存在异常风险' : 'Reality 伪装域名临期或配置预警' }}
          </p>
          <p class="text-[11px] opacity-90 mt-0.5 font-mono">
            检测到 {{ realityAlertCount }} 个 Reality 入站目标需关注，请及时检查与更换以确保护航。
          </p>
        </div>
      </div>
      <router-link
        to="/inbounds"
        class="shrink-0 px-2.5 py-1 rounded-md font-mono text-xs transition-colors flex items-center gap-1 border"
        :class="realitySummary?.errorCount ? 'bg-rose-900/40 hover:bg-rose-900/60 text-rose-200 border-rose-700/50' : 'bg-amber-900/40 hover:bg-amber-900/60 text-amber-200 border-amber-700/50'"
      >
        <span>前往处理</span>
        <span>➔</span>
      </router-link>
    </div>

    <!-- Quick Stats 4 Grid Cards -->
    <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-3">
      <!-- CPU -->
      <div class="p-4 rounded-lg bg-card border border-border">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-muted-foreground">CPU 使用率</span>
          <Cpu class="w-4 h-4 text-cyan-400" />
        </div>
        <div class="mt-2.5 flex items-baseline gap-2">
          <span class="text-2xl font-mono font-bold text-foreground">{{ dashboard?.metrics?.cpuUsagePercent?.toFixed(1) || 0 }}%</span>
        </div>
        <div class="mt-3 w-full bg-neutral-900 rounded-full h-1 overflow-hidden">
          <div
            class="bg-cyan-500 h-1 rounded-full transition-all duration-500"
            :style="{ width: `${Math.min(dashboard?.metrics?.cpuUsagePercent || 0, 100)}%` }"
          ></div>
        </div>
      </div>

      <!-- RAM -->
      <div class="p-4 rounded-lg bg-card border border-border">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-muted-foreground">内存占用</span>
          <Activity class="w-4 h-4 text-indigo-400" />
        </div>
        <div class="mt-2.5 flex items-baseline gap-2">
          <span class="text-2xl font-mono font-bold text-foreground">{{ dashboard?.metrics?.memoryUsagePercent?.toFixed(1) || 0 }}%</span>
          <span class="text-[11px] text-muted-foreground font-mono truncate">
            {{ formatBytes(dashboard?.metrics?.memoryUsedBytes || 0) }} / {{ formatBytes(dashboard?.metrics?.memoryTotalBytes || 0) }}
          </span>
        </div>
        <div class="mt-3 w-full bg-neutral-900 rounded-full h-1 overflow-hidden">
          <div
            class="bg-indigo-500 h-1 rounded-full transition-all duration-500"
            :style="{ width: `${Math.min(dashboard?.metrics?.memoryUsagePercent || 0, 100)}%` }"
          ></div>
        </div>
      </div>

      <!-- Disk -->
      <div class="p-4 rounded-lg bg-card border border-border">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-muted-foreground">磁盘空间</span>
          <HardDrive class="w-4 h-4 text-amber-400" />
        </div>
        <div class="mt-2.5 flex items-baseline gap-2">
          <span class="text-2xl font-mono font-bold text-foreground">{{ dashboard?.metrics?.diskUsagePercent?.toFixed(1) || 0 }}%</span>
          <span class="text-[11px] text-muted-foreground font-mono truncate">
            {{ formatBytes(dashboard?.metrics?.diskUsedBytes || 0) }} / {{ formatBytes(dashboard?.metrics?.diskTotalBytes || 0) }}
          </span>
        </div>
        <div class="mt-3 w-full bg-neutral-900 rounded-full h-1 overflow-hidden">
          <div
            class="bg-amber-500 h-1 rounded-full transition-all duration-500"
            :style="{ width: `${Math.min(dashboard?.metrics?.diskUsagePercent || 0, 100)}%` }"
          ></div>
        </div>
      </div>

      <!-- Active Users -->
      <div class="p-4 rounded-lg bg-card border border-border">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-muted-foreground">用户统计</span>
          <Users class="w-4 h-4 text-emerald-400" />
        </div>
        <div class="mt-2.5 flex items-baseline gap-2">
          <span class="text-2xl font-mono font-bold text-foreground">{{ dashboard?.activeUsers || 0 }}</span>
          <span class="text-[11px] text-muted-foreground font-mono">/ {{ dashboard?.userCount || 0 }} 活跃中</span>
        </div>
        <div class="mt-3 text-[11px] text-muted-foreground font-mono">
          已配置节点: <span class="text-foreground font-semibold">{{ dashboard?.inbounds?.length || 0 }}</span> 个
        </div>
      </div>
    </div>

    <!-- Network Bandwidth & Traffic Section -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-4">
      <!-- Real-time Speed Card -->
      <div class="p-4 rounded-lg bg-card border border-border lg:col-span-2 space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider flex items-center gap-1.5">
            <Zap class="w-3.5 h-3.5 text-cyan-400" />
            <span>实时网络速率与总吞吐量</span>
          </h2>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="bg-neutral-900/60 border border-border/70 p-3 rounded-md flex items-center justify-between">
            <div class="flex items-center gap-2.5">
              <div class="p-2 rounded bg-cyan-950/60 border border-cyan-800/40 text-cyan-400">
                <ArrowUpRight class="w-4 h-4" />
              </div>
              <div>
                <p class="text-[11px] text-muted-foreground font-medium">实时上行速率</p>
                <p class="text-lg font-mono font-bold text-foreground">
                  {{ formatSpeed(dashboard?.metrics?.netUpSpeedBps || 0) }}
                </p>
              </div>
            </div>
            <div class="text-right">
              <p class="text-[10px] text-muted-foreground">累计上行</p>
              <p class="text-xs font-mono text-foreground">{{ formatBytes(dashboard?.metrics?.netTotalSent || dashboard?.totalUp || 0) }}</p>
            </div>
          </div>

          <div class="bg-neutral-900/60 border border-border/70 p-3 rounded-md flex items-center justify-between">
            <div class="flex items-center gap-2.5">
              <div class="p-2 rounded bg-indigo-950/60 border border-indigo-800/40 text-indigo-400">
                <ArrowDownRight class="w-4 h-4" />
              </div>
              <div>
                <p class="text-[11px] text-muted-foreground font-medium">实时下行速率</p>
                <p class="text-lg font-mono font-bold text-foreground">
                  {{ formatSpeed(dashboard?.metrics?.netDownSpeedBps || 0) }}
                </p>
              </div>
            </div>
            <div class="text-right">
              <p class="text-[10px] text-muted-foreground">累计下行</p>
              <p class="text-xs font-mono text-foreground">{{ formatBytes(dashboard?.metrics?.netTotalRecv || dashboard?.totalDown || 0) }}</p>
            </div>
          </div>
        </div>

        <!-- Inbound status table preview -->
        <div class="pt-2">
          <div class="flex items-center justify-between mb-2">
            <h3 class="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider font-mono">入站节点概要</h3>
            <router-link to="/inbounds" class="text-[11px] text-brand-400 hover:text-brand-300 font-mono">
              查看全部入站 ➔
            </router-link>
          </div>
          <div class="overflow-hidden rounded-md border border-border bg-neutral-950">
            <table class="w-full text-left text-xs border-collapse">
              <thead class="text-muted-foreground bg-neutral-900 border-b border-border font-mono text-[11px]">
                <tr>
                  <th class="py-2 px-3">标签 (Tag)</th>
                  <th class="py-2 px-3">端口</th>
                  <th class="py-2 px-3">协议</th>
                  <th class="py-2 px-3">状态</th>
                  <th class="py-2 px-3 text-right">总流量</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border/40 font-mono">
                <tr v-for="inb in dashboard?.inbounds || []" :key="inb.id" class="hover:bg-muted/20 transition-colors">
                  <td class="py-2 px-3 font-medium text-foreground">{{ inb.tag }}</td>
                  <td class="py-2 px-3 text-cyan-400">{{ inb.port }}</td>
                  <td class="py-2 px-3 uppercase text-muted-foreground">{{ inb.protocol }}</td>
                  <td class="py-2 px-3">
                    <span class="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                      正常
                    </span>
                  </td>
                  <td class="py-2 px-3 text-right text-foreground">
                    {{ formatBytes(inb.upBytes + inb.downBytes) }}
                  </td>
                </tr>
                <tr v-if="!dashboard?.inbounds?.length">
                  <td colspan="5" class="py-4 text-center text-muted-foreground text-xs font-sans">暂无入站节点</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Service Info & Quick Ops -->
      <div class="p-4 rounded-lg bg-card border border-border space-y-4">
        <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider flex items-center gap-1.5">
          <Server class="w-3.5 h-3.5 text-brand-400" />
          <span>服务与系统信息</span>
        </h2>

        <div class="space-y-2.5 text-xs font-mono">
          <div class="flex justify-between py-1.5 border-b border-border/50">
            <span class="text-muted-foreground">Xray 核心版本</span>
            <span class="text-foreground font-semibold">{{ dashboard?.metrics?.xrayVersion || '未知' }}</span>
          </div>
          <div class="flex justify-between py-1.5 border-b border-border/50">
            <span class="text-muted-foreground">系统开机时长</span>
            <span class="text-foreground">{{ formatUptime(dashboard?.metrics?.uptimeSeconds || 0) }}</span>
          </div>
          <div class="flex justify-between py-1.5 border-b border-border/50">
            <span class="text-muted-foreground">服务子状态</span>
            <span class="text-foreground">{{ dashboard?.service?.subState || 'running' }}</span>
          </div>
        </div>

        <div class="pt-2">
          <h3 class="text-[11px] font-semibold text-muted-foreground uppercase tracking-wider mb-2 font-mono">快捷运维指令</h3>
          <Button
            variant="secondary"
            size="sm"
            class="w-full justify-center"
            :loading="restarting"
            @click="restartXray"
          >
            <RefreshCw v-if="!restarting" class="w-3.5 h-3.5 mr-1.5" />
            <span>{{ restarting ? '正在平滑重启...' : '平滑重启 Xray 核心' }}</span>
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
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
  AlertTriangle,
  AlertCircle,
} from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { toast } from '../utils/toast'
import api from '../api'
import { getRealityStatus, type RealitySummaryStatus } from '../api/reality'

const dashboard = ref<any>(null)
const refreshing = ref(false)
const restarting = ref(false)
const realitySummary = ref<RealitySummaryStatus | null>(null)
let timer: any = null

const realityAlertCount = computed(() => {
  if (!realitySummary.value) return 0
  return (realitySummary.value.errorCount || 0) + (realitySummary.value.warningCount || 0)
})

const fetchRealityAlerts = async () => {
  try {
    const rawRes: any = await getRealityStatus()
    realitySummary.value = (rawRes?.data && typeof rawRes.data === 'object' && rawRes.data.totalChecked !== undefined)
      ? rawRes.data
      : rawRes
  } catch (err) {
    // 静默降级
  }
}

const fetchData = async () => {
  refreshing.value = true
  try {
    dashboard.value = await api.get('/dashboard')
    fetchRealityAlerts()
  } catch (err) {
    console.error(err)
  } finally {
    refreshing.value = false
  }
}

const restartXray = async () => {
  if (!confirm('确认全量重启 Xray 核心进程吗？（重启耗时约 1 秒）')) return
  restarting.value = true
  try {
    await api.post('/service/restart')
    await fetchData()
    toast.success('Xray 核心已成功重启！')
  } catch (err: any) {
    toast.error('重启失败: ' + err)
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

const handleVisibilityChange = () => {
  if (document.hidden) {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  } else {
    fetchData()
    if (!timer) {
      timer = setInterval(fetchData, 4000)
    }
  }
}

onMounted(() => {
  fetchData()
  timer = setInterval(fetchData, 4000)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
