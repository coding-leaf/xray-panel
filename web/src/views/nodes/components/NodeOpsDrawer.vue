<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="node ? `节点运维：${node.name}` : '节点运维'"
    :description="
      node ? `${node.host}${node.region ? ` · ${node.region}` : ''} · 节点 #${node.id}` : ''
    "
    width="w-full sm:max-w-2xl md:max-w-3xl"
  >
    <div v-if="node" class="space-y-4 text-xs">
      <!-- Tab Bar -->
      <div
        role="tablist"
        aria-label="节点运维功能标签"
        class="grid grid-cols-2 sm:grid-cols-4 gap-1.5 p-1 rounded-lg bg-neutral-900 border border-border"
      >
        <button
          v-for="tab in tabs"
          :key="tab.key"
          type="button"
          role="tab"
          :aria-selected="activeTab === tab.key"
          @click="activeTab = tab.key"
          class="flex items-center justify-center gap-1.5 px-2.5 py-1.5 rounded-md text-xs font-medium transition-colors"
          :class="
            activeTab === tab.key
              ? 'bg-card text-foreground border border-border shadow-xs'
              : 'text-muted-foreground hover:text-foreground hover:bg-muted/40 border border-transparent'
          "
        >
          <component :is="tab.icon" class="w-3.5 h-3.5 shrink-0" />
          <span>{{ tab.label }}</span>
        </button>
      </div>

      <!-- Tab 1: 状态与配额 -->
      <div v-if="activeTab === 'overview'" class="space-y-4">
        <!-- Status Summary Grid -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div class="rounded-lg border border-border bg-card p-3.5 space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-[11px] font-mono text-muted-foreground uppercase">在线与调度</span>
              <Badge
                :variant="node.online ? 'success' : 'secondary'"
                :dot="true"
                class="text-[10px]"
              >
                {{ node.online ? '在线' : '离线' }} · {{ formatLastSeen(node.lastSeenAt) }}
              </Badge>
            </div>
            <div class="flex items-center gap-1.5 flex-wrap">
              <Badge :variant="getDesiredStateBadge(node.desiredState).variant" class="text-[10px]">
                {{ getDesiredStateBadge(node.desiredState).label }}
              </Badge>
              <Badge v-if="!node.enabled" variant="secondary" class="text-[10px]">已停用</Badge>
              <Badge
                v-if="getAutoStateReasonBadge(node.autoStateReason)"
                :variant="getAutoStateReasonBadge(node.autoStateReason)!.variant"
                class="text-[10px]"
              >
                {{ getAutoStateReasonBadge(node.autoStateReason)!.label }}
              </Badge>
            </div>
            <div class="font-mono text-[11px] text-muted-foreground space-y-0.5 pt-1">
              <div>
                系统负载：CPU {{ (node.cpuPercent || 0).toFixed(1) }}% · MEM
                {{ (node.memPercent || 0).toFixed(1) }}%
              </div>
              <div>Agent {{ node.agentVersion || '-' }} · Xray {{ node.xrayVersion || '-' }}</div>
              <div v-if="node.lastSeenAt">最近心跳：{{ formatDateTime(node.lastSeenAt) }}</div>
            </div>
          </div>

          <div class="rounded-lg border border-border bg-card p-3.5 space-y-2">
            <div class="flex items-center justify-between">
              <span class="text-[11px] font-mono text-muted-foreground uppercase">配置同步</span>
              <Badge
                v-if="nodeSyncStatus === 'error'"
                variant="destructive"
                :dot="true"
                class="text-[10px]"
              >
                同步失败
              </Badge>
              <Badge
                v-else-if="nodeSyncStatus === 'synced'"
                variant="success"
                :dot="true"
                class="text-[10px]"
              >
                已同步
              </Badge>
              <Badge
                v-else
                variant="warning"
                :dot="true"
                dot-color="bg-amber-400 animate-pulse"
                class="text-[10px]"
              >
                待同步
              </Badge>
            </div>
            <div class="font-mono text-[11px] text-muted-foreground space-y-0.5">
              <div>
                期望 Hash：<span class="text-foreground">{{
                  formatShortHash(node.expectedConfigHash)
                }}</span>
              </div>
              <div>
                当前 Hash：<span class="text-foreground">{{
                  formatShortHash(node.lastConfigHash)
                }}</span>
              </div>
              <div>
                Token 前缀：<span class="text-foreground">{{ node.tokenPrefix || '-' }}*</span>
              </div>
            </div>
            <div
              v-if="node.lastSyncError"
              class="p-2 rounded bg-rose-500/10 border border-rose-500/20 text-rose-300 font-mono text-[10px] break-all flex items-start gap-1.5"
            >
              <AlertTriangle class="w-3.5 h-3.5 text-rose-400 shrink-0 mt-0.5" />
              <span>{{ node.lastSyncError }}</span>
            </div>
          </div>
        </div>

        <!-- Cycle Quota & Traffic Card -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between gap-2 flex-wrap">
            <div class="font-semibold text-foreground flex items-center gap-1.5">
              <Activity class="w-3.5 h-3.5 text-cyan-400" />
              <span>周期流量与配额监控</span>
            </div>
            <span class="text-[11px] font-mono text-muted-foreground">
              计费口径：{{ node.trafficCountMode || 'both' }} · 系数
              {{ node.trafficMultiplier > 0 ? node.trafficMultiplier : 1 }}x ·
              {{ node.billingResetDay > 0 ? `每月 ${node.billingResetDay} 日重置` : '手动重置' }}
            </span>
          </div>

          <div v-if="quotaStats.hasLimit" class="space-y-1.5 font-mono">
            <div class="flex items-center justify-between text-[11px]">
              <span class="text-foreground">
                周期已用 {{ formatBytes(quotaStats.usedBytes) }} /
                {{ formatBytes(quotaStats.limitBytes) }}
              </span>
              <span :class="['font-semibold', quotaStats.textClass]">
                {{ quotaStats.displayPct }}%
              </span>
            </div>
            <div class="w-full h-2 rounded-full bg-neutral-800 overflow-hidden">
              <div
                :class="['h-full rounded-full transition-all', quotaStats.barClass]"
                :style="{ width: `${quotaStats.barWidth}%` }"
              />
            </div>
            <div class="text-[10px] text-muted-foreground">
              预警 {{ node.warnThresholdPct || 80 }}% / 摘流 {{ node.drainThresholdPct || 90 }}% /
              熔断 {{ node.breakThresholdPct || 95 }}%
            </div>
          </div>
          <div v-else class="text-[11px] font-mono text-muted-foreground">
            本周期已计费流量：
            <span class="text-foreground font-semibold">{{
              formatBytes(quotaStats.usedBytes)
            }}</span>
            （未配置月上限）
          </div>

          <!-- Xray vs Physical NIC Comparison -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5 font-mono text-[11px]">
            <div class="p-2.5 rounded-md bg-neutral-950 border border-border/70 space-y-1">
              <div class="text-[10px] text-muted-foreground">本周期 Xray 精确统计</div>
              <div class="text-foreground">
                ↑ {{ formatBytes(node.cycleXrayUpBytes || 0) }} / ↓
                {{ formatBytes(node.cycleXrayDownBytes || 0) }}
              </div>
              <div class="text-[10px] text-muted-foreground">
                历史累计：↑ {{ formatBytes(node.xrayUpBytes || 0) }} / ↓
                {{ formatBytes(node.xrayDownBytes || 0) }}
              </div>
            </div>
            <div class="p-2.5 rounded-md bg-neutral-950 border border-border/70 space-y-1">
              <div class="text-[10px] text-muted-foreground">本周期物理网卡 (NIC) 统计</div>
              <div class="text-foreground">
                ↑ {{ formatBytes(node.cycleNicUpBytes || 0) }} / ↓
                {{ formatBytes(node.cycleNicDownBytes || 0) }}
              </div>
              <div class="text-[10px] text-muted-foreground">
                历史累计：↑ {{ formatBytes(node.nicUpBytes || 0) }} / ↓
                {{ formatBytes(node.nicDownBytes || 0) }}
              </div>
            </div>
          </div>

          <!-- P2-4 Offline Physical NIC Accounting Note -->
          <div
            class="p-2.5 rounded-md bg-amber-500/5 border border-amber-500/20 text-amber-200/90 text-[11px] leading-relaxed"
          >
            <span class="font-semibold text-amber-300">离线流量口径说明：</span>
            当节点离线或 Xray gRPC 统计短暂不可用期间，Agent
            会使用物理网卡增量作为兜底统计口径（可能包含宿主机非 Xray
            外部流量）；恢复上线并重连后自动切回 Xray 精确统计口径。
          </div>

          <!-- Quick Operations -->
          <div class="pt-2 border-t border-border/60 flex flex-wrap items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              :loading="resettingId === node.id"
              @click="emit('reset-traffic', node)"
            >
              <RotateCw class="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
              <span>重置周期流量</span>
            </Button>

            <Button
              v-if="node.desiredState === 'active'"
              variant="outline"
              size="sm"
              :loading="togglingDrain"
              @click="handleToggleDrain"
            >
              <PauseCircle class="w-3.5 h-3.5 mr-1.5 text-amber-400" />
              <span>摘流排空 (draining)</span>
            </Button>
            <Button
              v-else
              variant="outline"
              size="sm"
              :loading="togglingDrain"
              @click="handleToggleDrain"
            >
              <PlayCircle class="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
              <span>恢复活跃 (active)</span>
            </Button>
          </div>
        </div>
      </div>

      <!-- Tab 2: 快速日志 -->
      <div v-else-if="activeTab === 'logs'" class="space-y-3">
        <div
          class="p-3 rounded-lg bg-card border border-border flex flex-wrap items-center justify-between gap-2"
        >
          <div class="flex items-center gap-2 flex-wrap">
            <select
              v-model="logService"
              aria-label="选择远端服务日志"
              class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="xray">xray.service</option>
              <option value="xray-agent">xray-agent.service</option>
            </select>

            <Button
              variant="default"
              size="sm"
              :loading="logLoading || isLogPolling"
              @click="fetchQuickLogs"
            >
              <RefreshCw
                class="w-3.5 h-3.5 mr-1.5"
                :class="{ 'animate-spin': logLoading || isLogPolling }"
              />
              <span>{{ isLogPolling ? '采集中...' : '拉取最近 100 行' }}</span>
            </Button>

            <Button
              variant="secondary"
              size="sm"
              :disabled="!filteredLogLines.length"
              @click="copyQuickLogs"
            >
              <Copy class="w-3.5 h-3.5 mr-1" />
              <span>复制</span>
            </Button>
          </div>

          <Button variant="ghost" size="sm" @click="jumpToFullLogs">
            <ExternalLink class="w-3.5 h-3.5 mr-1" />
            <span>前往完整日志</span>
          </Button>
        </div>

        <div class="relative">
          <input
            v-model="logKeyword"
            type="text"
            placeholder="在已拉取的远端日志中筛选关键词..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md px-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div
          v-if="isLogPolling"
          class="p-2.5 rounded-md bg-neutral-950 border border-sky-500/30 text-sky-300 text-[11px] font-mono flex items-center gap-2"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-ping shrink-0"></span>
          <span>
            {{
              logResult?.status === 'dispatched'
                ? `节点正在执行 journalctl 采集 (${logService})...`
                : `已入队日志采集指令，等待节点下一次心跳领取（${logResult?.commandId || ''}）...`
            }}
          </span>
        </div>

        <div
          v-else-if="logError"
          class="p-2.5 rounded-md bg-rose-500/10 border border-rose-500/30 text-rose-300 text-[11px] font-mono break-all"
        >
          {{ logError }}
        </div>

        <div class="rounded-lg border border-border bg-neutral-950 overflow-hidden">
          <div
            class="px-3 py-1.5 border-b border-border bg-neutral-900/70 flex items-center justify-between text-[10px] font-mono text-muted-foreground"
          >
            <span>{{ logService }} · {{ filteredLogLines.length }} 行</span>
            <span v-if="logResult?.collectedAt">
              采集于 {{ formatDateTime(logResult.collectedAt) }}
            </span>
          </div>
          <div
            v-if="filteredLogLines.length"
            class="p-3 max-h-[420px] overflow-auto font-mono text-[11px] leading-relaxed text-neutral-200 space-y-0.5"
          >
            <div v-for="item in filteredLogLines" :key="item.key" class="whitespace-pre-wrap">
              {{ item.text }}
            </div>
          </div>
          <div v-else class="p-8 text-center text-muted-foreground font-mono text-[11px]">
            {{
              logResult
                ? logKeyword.trim()
                  ? '没有匹配当前关键词的日志行'
                  : '本次未采集到日志输出'
                : '点击「拉取最近 100 行」按需采集远端节点运行日志'
            }}
          </div>
        </div>
      </div>

      <!-- Tab 3: 编译预览 -->
      <div v-else-if="activeTab === 'config'" class="space-y-3">
        <div
          class="p-3 rounded-lg bg-card border border-border flex flex-wrap items-center justify-between gap-2"
        >
          <div class="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              :loading="configLoading"
              @click="fetchCompiledConfig"
            >
              <RefreshCw class="w-3.5 h-3.5 mr-1.5" :class="{ 'animate-spin': configLoading }" />
              <span>刷新预览</span>
            </Button>
            <Button variant="secondary" size="sm" :disabled="!configContent" @click="copyConfig">
              <Copy class="w-3.5 h-3.5 mr-1" />
              <span>复制 JSON</span>
            </Button>
          </div>

          <Button variant="ghost" size="sm" @click="jumpToFullConfig">
            <ExternalLink class="w-3.5 h-3.5 mr-1" />
            <span>前往配置页</span>
          </Button>
        </div>

        <div
          v-if="configError"
          class="p-2.5 rounded-md bg-rose-500/10 border border-rose-500/30 text-rose-300 text-[11px] font-mono break-all"
        >
          {{ configError }}
        </div>

        <div class="rounded-lg border border-border bg-neutral-950 overflow-hidden">
          <div
            class="px-3 py-1.5 border-b border-border bg-neutral-900/70 flex items-center justify-between text-[10px] font-mono text-muted-foreground"
          >
            <span>远端编译配置 (只读预览)</span>
            <span>Hash #{{ formatShortHash(node.expectedConfigHash || node.lastConfigHash) }}</span>
          </div>
          <pre
            v-if="configContent"
            class="p-3 max-h-[460px] overflow-auto font-mono text-[11px] leading-relaxed text-neutral-200"
          ><code>{{ configContent }}</code></pre>
          <div v-else class="p-8 text-center text-muted-foreground font-mono text-[11px]">
            {{ configLoading ? '正在编译远端配置预览...' : '暂无配置预览数据' }}
          </div>
        </div>
      </div>

      <!-- Tab 4: GeoData 与部署 -->
      <div v-else class="space-y-4">
        <!-- Remote GeoData Update -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-2.5">
            <div class="space-y-0.5">
              <div class="font-semibold text-foreground flex items-center gap-1.5">
                <Database class="w-3.5 h-3.5 text-cyan-400" />
                <span>远端 GeoData 规则库更新</span>
              </div>
              <p class="text-[11px] text-muted-foreground font-mono">
                向该节点按需下发最新 geoip.dat /
                geosite.dat，下载后自动校验文件体积并在重载失败时回滚。
              </p>
            </div>
            <Button
              variant="default"
              size="sm"
              :loading="geoUpdating"
              :disabled="geoUpdating"
              @click="triggerRemoteGeoUpdate"
              class="shrink-0"
            >
              <RotateCw class="w-3.5 h-3.5 mr-1.5" :class="{ 'animate-spin': geoUpdating }" />
              <span>{{ geoUpdating ? '节点更新中...' : '下发 GeoData 更新' }}</span>
            </Button>
          </div>

          <div
            v-if="geoUpdating"
            class="p-2.5 bg-neutral-950 rounded-md border border-sky-500/30 text-sky-300 text-[11px] font-mono flex items-center gap-2"
          >
            <span class="w-1.5 h-1.5 rounded-full bg-sky-400 animate-ping shrink-0"></span>
            <span>
              <template v-if="geoResult?.status === 'dispatched'">
                远端节点「{{ node.name }}」正在下载并校验 GeoData（含重载与回滚保护）...
              </template>
              <template v-else-if="geoResult?.status === 'queued'">
                已入队 GeoData 更新指令，等待节点下一次心跳领取（最多约 60s）...
              </template>
              <template v-else>正在向节点下发 GeoData 更新指令...</template>
              <template v-if="geoResult?.commandId">（{{ geoResult.commandId }}）</template>
            </span>
          </div>

          <div
            v-else-if="geoResult && geoResult.nodeId === node.id"
            class="p-2.5 rounded-md border text-[11px] font-mono space-y-1"
            :class="
              geoResult.error
                ? 'bg-rose-500/10 border-rose-500/30 text-rose-300'
                : geoResult.pending
                  ? 'bg-amber-500/10 border-amber-500/30 text-amber-300'
                  : 'bg-emerald-500/10 border-emerald-500/30 text-emerald-300'
            "
          >
            <div v-if="geoResult.error" class="break-all">更新失败：{{ geoResult.error }}</div>
            <div v-else-if="geoResult.pending">
              节点正在后台执行 GeoData 更新，请稍后关注节点同步状态或运行日志。
            </div>
            <div v-else class="flex flex-wrap items-center justify-between gap-2">
              <span>
                更新完成 · GeoIP {{ ((geoResult.geoipSize || 0) / 1048576).toFixed(2) }} MB |
                GeoSite {{ ((geoResult.geositeSize || 0) / 1048576).toFixed(2) }} MB | 耗时
                {{ ((geoResult.durationMs || 0) / 1000).toFixed(1) }} s
              </span>
              <span v-if="geoResult.updatedAt" class="text-emerald-200/80">
                {{ formatDateTime(geoResult.updatedAt) }}
              </span>
            </div>
          </div>
        </div>

        <!-- Deployment & Token Operations -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="font-semibold text-foreground flex items-center gap-1.5">
            <Terminal class="w-3.5 h-3.5 text-amber-400" />
            <span>Agent 部署脚本与认证凭证</span>
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2.5 font-mono text-[11px]">
            <div class="p-2.5 rounded-md bg-neutral-950 border border-border/70">
              <div class="text-[10px] text-muted-foreground">节点接入地址</div>
              <div class="text-foreground mt-0.5 truncate" :title="node.host">{{ node.host }}</div>
            </div>
            <div class="p-2.5 rounded-md bg-neutral-950 border border-border/70">
              <div class="text-[10px] text-muted-foreground">当前 Token 前缀</div>
              <div class="text-foreground mt-0.5">{{ node.tokenPrefix || '-' }}*</div>
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2 pt-1">
            <Button variant="secondary" size="sm" @click="emit('open-install', node)">
              <Terminal class="w-3.5 h-3.5 mr-1.5 text-cyan-400" />
              <span>查看安装脚本</span>
            </Button>
            <Button variant="outline" size="sm" @click="emit('rotate-token', node)">
              <KeyRound class="w-3.5 h-3.5 mr-1.5 text-amber-400" />
              <span>轮换 Agent Token</span>
            </Button>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <Button type="button" variant="outline" size="sm" @click="emit('update:modelValue', false)">
        关闭
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch, onUnmounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  Activity,
  AlertTriangle,
  Copy,
  Database,
  ExternalLink,
  FileCode2,
  KeyRound,
  PauseCircle,
  PlayCircle,
  RefreshCw,
  RotateCw,
  ScrollText,
  Terminal,
} from 'lucide-vue-next'
import Drawer from '../../../components/ui/Drawer.vue'
import Badge from '../../../components/ui/Badge.vue'
import Button from '../../../components/ui/Button.vue'
import api from '../../../api'
import {
  formatShortHash,
  getNodeCommandStatus,
  getNodeSyncStatus,
  type NodeItem,
  type RemoteGeoUpdateResult,
  type RemoteLogResponse,
} from '../../../api/nodes'
import { useActiveNode } from '../../../composables/useActiveNode'
import { copyText } from '../../../utils/clipboard'
import { formatBytes, formatDateTime } from '../../../utils/format'
import { toast } from '../../../utils/toast'
import {
  formatLastSeen,
  getAutoStateReasonBadge,
  getDesiredStateBadge,
  getNodeQuotaStats,
} from '../composables/useNodes'

type OpsTabKey = 'overview' | 'logs' | 'config' | 'geodata'

const props = defineProps<{
  modelValue: boolean
  node: NodeItem | null
  resettingId?: number | null
  togglingDrain?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'open-install', node: NodeItem): void
  (e: 'rotate-token', node: NodeItem): void
  (e: 'reset-traffic', node: NodeItem): void
  (e: 'toggle-drain', node: NodeItem): void
  (e: 'refresh'): void
}>()

const router = useRouter()
const { setActiveNode } = useActiveNode()

const tabs: Array<{ key: OpsTabKey; label: string; icon: any }> = [
  { key: 'overview', label: '状态与配额', icon: Activity },
  { key: 'logs', label: '快速日志', icon: ScrollText },
  { key: 'config', label: '编译预览', icon: FileCode2 },
  { key: 'geodata', label: 'GeoData 与部署', icon: Database },
]

const activeTab = ref<OpsTabKey>('overview')

const nodeSyncStatus = computed(() => getNodeSyncStatus(props.node))
const quotaStats = computed(() =>
  props.node
    ? getNodeQuotaStats(props.node)
    : {
        usedBytes: 0,
        limitBytes: 0,
        hasLimit: false,
        rawPct: 0,
        displayPct: '0',
        barWidth: 0,
        barClass: 'bg-emerald-500',
        textClass: 'text-emerald-400',
      }
)

// Tab 2: Quick Logs state
const logService = ref<'xray' | 'xray-agent'>('xray')
const logKeyword = ref('')
const logLoading = ref(false)
const logError = ref('')
const logResult = ref<RemoteLogResponse | null>(null)
const isLogPolling = ref(false)
let logPollTimer: ReturnType<typeof setInterval> | null = null
let logPollInFlight = false
let logReqSeq = 0

// Tab 3: Compiled Config state
const configLoading = ref(false)
const configError = ref('')
const configContent = ref('')
let configReqSeq = 0

// Tab 4: Remote GeoData state
const geoUpdating = ref(false)
const geoResult = ref<RemoteGeoUpdateResult | null>(null)
let geoPollTimer: ReturnType<typeof setInterval> | null = null
let geoPollInFlight = false
let geoReqSeq = 0

let isUnmounted = false

const stopLogPolling = () => {
  if (logPollTimer) {
    clearInterval(logPollTimer)
    logPollTimer = null
  }
  logPollInFlight = false
  isLogPolling.value = false
}

const stopGeoPolling = () => {
  if (geoPollTimer) {
    clearInterval(geoPollTimer)
    geoPollTimer = null
  }
  geoPollInFlight = false
}

const stopAllTimers = () => {
  stopLogPolling()
  stopGeoPolling()
}

const resetAsyncState = () => {
  stopAllTimers()
  logReqSeq++
  configReqSeq++
  geoReqSeq++

  logLoading.value = false
  logError.value = ''
  logResult.value = null
  logKeyword.value = ''

  configLoading.value = false
  configError.value = ''
  configContent.value = ''

  geoUpdating.value = false
  geoResult.value = null
}

const isCurrentTarget = (nodeId: number) =>
  !isUnmounted && props.modelValue && props.node?.id === nodeId

const handleToggleDrain = () => {
  if (!props.node) return
  emit('toggle-drain', props.node)
}

// --- Tab 2: Quick Logs ---
const filteredLogLines = computed(() => {
  const lines = logResult.value?.lines
  if (!Array.isArray(lines) || lines.length === 0) return []
  const kw = logKeyword.value.trim().toLowerCase()
  const counts = new Map<string, number>()
  return lines
    .filter((line) => !kw || line.toLowerCase().includes(kw))
    .map((text) => {
      const idx = counts.get(text) || 0
      counts.set(text, idx + 1)
      return { text, key: `${text}#${idx}` }
    })
})

const startLogPolling = (nodeId: number, commandId: string, reqSeq: number) => {
  stopLogPolling()
  isLogPolling.value = true

  const pollOnce = async () => {
    if (!isCurrentTarget(nodeId) || reqSeq !== logReqSeq) {
      stopLogPolling()
      return
    }
    if (logPollInFlight) return
    logPollInFlight = true
    try {
      const snap = await getNodeCommandStatus(nodeId, commandId)
      if (!isCurrentTarget(nodeId) || reqSeq !== logReqSeq) return
      if (snap.status === 'queued' || snap.status === 'dispatched') {
        logResult.value = {
          nodeId,
          service: logService.value,
          lines: [],
          cached: false,
          pending: true,
          commandId: snap.id,
          status: snap.status,
        }
        return
      }
      stopLogPolling()
      if (snap.status === 'succeeded') {
        logError.value = ''
        logResult.value = {
          nodeId,
          service: snap.logResult?.service || logService.value,
          lines: Array.isArray(snap.logResult?.lines) ? snap.logResult.lines : [],
          cached: false,
          pending: false,
          commandId: snap.id,
          status: 'succeeded',
          collectedAt: snap.logResult?.collectedAt || snap.updatedAt,
        }
        return
      }
      const errMsg =
        snap.error ||
        (snap.status === 'expired' ? '采集指令已超时过期，请重新拉取' : '远端节点采集日志失败')
      logError.value = errMsg
      logResult.value = {
        nodeId,
        service: logService.value,
        lines: [],
        cached: false,
        pending: false,
        commandId: snap.id,
        status: snap.status,
        error: errMsg,
      }
    } catch (err: any) {
      if (!isCurrentTarget(nodeId) || reqSeq !== logReqSeq) return
      stopLogPolling()
      const msg = typeof err === 'string' ? err : err?.message || '查询远端日志状态失败'
      logError.value = msg
      toast.error('查询远端日志状态失败：' + msg)
    } finally {
      logPollInFlight = false
    }
  }

  logPollTimer = setInterval(() => {
    void pollOnce()
  }, 2000)
}

const fetchQuickLogs = async () => {
  const node = props.node
  if (!node) return
  stopLogPolling()
  const reqSeq = ++logReqSeq
  logLoading.value = true
  logError.value = ''
  try {
    const params = new URLSearchParams()
    params.append('nodeId', String(node.id))
    params.append('service', logService.value)
    params.append('lines', '100')
    params.append('refresh', 'true')
    const res = await api.get<RemoteLogResponse>(`/logs?${params.toString()}`)
    if (!isCurrentTarget(node.id) || reqSeq !== logReqSeq) return
    logResult.value = res || null
    if (res?.error) {
      logError.value = res.error
    } else if (res?.pending && res.commandId) {
      startLogPolling(node.id, res.commandId, reqSeq)
    }
  } catch (err: any) {
    if (!isCurrentTarget(node.id) || reqSeq !== logReqSeq) return
    const msg = typeof err === 'string' ? err : err?.message || '拉取远端日志失败'
    logError.value = msg
    toast.error('拉取远端日志失败：' + msg)
  } finally {
    if (!isUnmounted && reqSeq === logReqSeq) {
      logLoading.value = false
    }
  }
}

const copyQuickLogs = () => {
  if (!filteredLogLines.value.length) return
  const text = filteredLogLines.value.map((item) => item.text).join('\n')
  void copyText(text, '远端日志已复制')
}

const jumpToFullLogs = () => {
  if (!props.node) return
  const targetId = props.node.id
  setActiveNode(targetId)
  emit('update:modelValue', false)
  router.push({ path: '/logs', query: { nodeId: String(targetId) } })
}

// --- Tab 3: Compiled Config Preview ---
const fetchCompiledConfig = async () => {
  const node = props.node
  if (!node) return
  const reqSeq = ++configReqSeq
  configLoading.value = true
  configError.value = ''
  try {
    const res: any = await api.get(`/config/raw?nodeId=${node.id}`)
    if (!isCurrentTarget(node.id) || reqSeq !== configReqSeq) return
    if (res && typeof res === 'object' && typeof res.raw === 'string') {
      try {
        configContent.value = JSON.stringify(JSON.parse(res.raw), null, 2)
      } catch {
        configContent.value = res.raw
      }
    } else {
      configContent.value = typeof res === 'string' ? res : JSON.stringify(res, null, 2)
    }
  } catch (err: any) {
    if (!isCurrentTarget(node.id) || reqSeq !== configReqSeq) return
    const msg = typeof err === 'string' ? err : err?.message || '加载编译配置失败'
    configError.value = msg
    toast.error('加载编译配置失败：' + msg)
  } finally {
    if (!isUnmounted && reqSeq === configReqSeq) {
      configLoading.value = false
    }
  }
}

const copyConfig = () => {
  if (!configContent.value) return
  void copyText(configContent.value, '配置 JSON 已复制')
}

const jumpToFullConfig = () => {
  if (!props.node) return
  setActiveNode(props.node.id)
  emit('update:modelValue', false)
  router.push('/config')
}

// --- Tab 4: Remote GeoData Update ---
const startGeoPolling = (nodeId: number, commandId: string, reqSeq: number) => {
  stopGeoPolling()

  const pollOnce = async () => {
    if (!isCurrentTarget(nodeId) || reqSeq !== geoReqSeq) {
      stopGeoPolling()
      return
    }
    if (geoPollInFlight) return
    geoPollInFlight = true
    try {
      const snap = await getNodeCommandStatus(nodeId, commandId)
      if (!isCurrentTarget(nodeId) || reqSeq !== geoReqSeq) return
      if (snap.status === 'queued' || snap.status === 'dispatched') {
        geoResult.value = {
          nodeId,
          pending: true,
          commandId: snap.id,
          status: snap.status,
        }
        return
      }
      stopGeoPolling()
      geoUpdating.value = false
      if (snap.status === 'succeeded') {
        const geoipSize = snap.geoResult?.geoipSize ?? 0
        const geositeSize = snap.geoResult?.geositeSize ?? 0
        const durationMs = snap.geoResult?.durationMs ?? 0
        const updatedAt = snap.geoResult?.updatedAt || snap.updatedAt
        geoResult.value = {
          nodeId,
          pending: false,
          commandId: snap.id,
          status: 'succeeded',
          geoipSize,
          geositeSize,
          durationMs,
          updatedAt,
        }
        toast.success(
          `远端 GeoData 已更新（GeoIP ${(geoipSize / 1048576).toFixed(2)} MB / GeoSite ${(geositeSize / 1048576).toFixed(2)} MB，耗时 ${(durationMs / 1000).toFixed(1)} s）`
        )
        emit('refresh')
        return
      }
      const errMsg =
        snap.error ||
        (snap.status === 'expired' ? 'GeoData 更新指令已超时过期，请重试' : '远端 GeoData 更新失败')
      geoResult.value = {
        nodeId,
        pending: false,
        commandId: snap.id,
        status: snap.status,
        error: errMsg,
      }
      toast.error(`远端 GeoData 更新失败：${errMsg}`)
      emit('refresh')
    } catch (err: any) {
      if (!isCurrentTarget(nodeId) || reqSeq !== geoReqSeq) return
      stopGeoPolling()
      geoUpdating.value = false
      const errMsg = String(err?.message || err || '查询更新状态失败')
      geoResult.value = {
        nodeId,
        pending: false,
        commandId,
        status: 'failed',
        error: errMsg,
      }
      toast.error('远端 GeoData 更新失败：' + errMsg)
    } finally {
      geoPollInFlight = false
    }
  }

  geoPollTimer = setInterval(() => {
    void pollOnce()
  }, 2000)
}

const triggerRemoteGeoUpdate = async () => {
  const node = props.node
  if (!node) return
  if (
    !confirm(
      `确定向远端节点「${node.name}」按需下发 GeoData 更新吗？节点将校验文件体积并在重载失败时自动回滚。`
    )
  ) {
    return
  }
  stopGeoPolling()
  const reqSeq = ++geoReqSeq
  geoUpdating.value = true
  geoResult.value = null
  try {
    const res = await api.post<RemoteGeoUpdateResult>(`/geodata/update?nodeId=${node.id}`)
    if (!isCurrentTarget(node.id) || reqSeq !== geoReqSeq) return
    geoResult.value = res
    if (res?.error) {
      geoUpdating.value = false
      toast.error(`远端 GeoData 更新失败：${res.error}`)
      emit('refresh')
    } else if (res?.pending && res.commandId) {
      startGeoPolling(node.id, res.commandId, reqSeq)
    } else if (res?.pending) {
      geoUpdating.value = false
      toast.warning('远端节点正在后台执行 GeoData 更新，请稍后关注节点状态')
      emit('refresh')
    } else {
      geoUpdating.value = false
      toast.success(
        `远端 GeoData 已更新（GeoIP ${((res?.geoipSize || 0) / 1048576).toFixed(2)} MB / GeoSite ${((res?.geositeSize || 0) / 1048576).toFixed(2)} MB，耗时 ${((res?.durationMs || 0) / 1000).toFixed(1)} s）`
      )
      emit('refresh')
    }
  } catch (err: any) {
    if (!isCurrentTarget(node.id) || reqSeq !== geoReqSeq) return
    geoUpdating.value = false
    const errMsg = String(err?.message || err || '请求失败')
    geoResult.value = {
      nodeId: node.id,
      pending: false,
      error: errMsg,
    }
    toast.error('远端 GeoData 更新失败：' + errMsg)
  }
}

// Watchers & Lifecycle Guards
watch(logService, () => {
  stopLogPolling()
  logReqSeq++
  logLoading.value = false
  logError.value = ''
  logResult.value = null
})

watch(activeTab, (tab) => {
  if (
    tab === 'config' &&
    props.modelValue &&
    props.node &&
    !configContent.value &&
    !configLoading.value
  ) {
    void fetchCompiledConfig()
  }
})

watch(
  () => props.modelValue,
  (open) => {
    if (!open) {
      resetAsyncState()
    } else if (activeTab.value === 'config' && props.node && !configContent.value) {
      void fetchCompiledConfig()
    }
  }
)

watch(
  () => props.node?.id,
  () => {
    resetAsyncState()
    activeTab.value = 'overview'
  }
)

onUnmounted(() => {
  isUnmounted = true
  resetAsyncState()
})
</script>
