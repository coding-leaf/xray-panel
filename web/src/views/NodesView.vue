<template>
  <div class="space-y-4">
    <!-- Top Header -->
    <div
      class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60"
    >
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">远端直连节点 (Nodes)</h1>
          <Badge variant="outline" class="text-[10px]"> {{ filteredNodes.length }} 个节点 </Badge>
          <Badge
            v-if="summary.errorCount > 0"
            variant="destructive"
            :dot="true"
            class="text-[10px]"
          >
            {{ summary.errorCount }} 个同步异常
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          管理分布式 Xray Agent 直连节点、一次性认证 Token 与多机直连订阅状态
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button variant="secondary" size="sm" :loading="loading" @click="fetchNodes(false)">
          <RefreshCw class="w-3.5 h-3.5 mr-1.5" />
          <span>刷新</span>
        </Button>
        <Button variant="default" size="sm" @click="openCreateDrawer">
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>新增节点</span>
        </Button>
      </div>
    </div>

    <!-- Summary KPI Cards -->
    <div class="grid grid-cols-2 lg:grid-cols-4 gap-3">
      <div class="rounded-lg border border-border bg-card p-3.5 space-y-1">
        <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
          <span>注册节点总数</span>
          <Server class="w-3.5 h-3.5 text-neutral-400" />
        </div>
        <div class="text-xl font-semibold font-mono text-foreground">{{ summary.total }}</div>
        <div class="text-[10px] font-mono text-muted-foreground">
          主控 Master (ID 0) 不计入远端列表
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3.5 space-y-1">
        <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
          <span>在线心跳 (90s 内)</span>
          <Activity class="w-3.5 h-3.5 text-emerald-400" />
        </div>
        <div class="text-xl font-semibold font-mono text-foreground">
          <span class="text-emerald-400">{{ summary.onlineCount }}</span>
          <span class="text-xs text-muted-foreground font-normal"> / {{ summary.total }}</span>
        </div>
        <div class="text-[10px] font-mono text-muted-foreground">
          离线节点：{{ Math.max(0, summary.total - summary.onlineCount) }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3.5 space-y-1">
        <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
          <span>活跃可分发</span>
          <CheckCircle2 class="w-3.5 h-3.5 text-cyan-400" />
        </div>
        <div class="text-xl font-semibold font-mono text-foreground">{{ summary.activeCount }}</div>
        <div class="text-[10px] font-mono text-muted-foreground">需同时满足 已启用 + active</div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3.5 space-y-1">
        <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
          <span>累计直连流量</span>
          <ArrowUpDown class="w-3.5 h-3.5 text-neutral-400" />
        </div>
        <div class="text-xl font-semibold font-mono text-foreground">
          {{ formatBytes(summary.totalXrayBytes) }}
        </div>
        <div class="text-[10px] font-mono text-muted-foreground">
          网卡累计：{{ formatBytes(summary.totalNicBytes) }}
        </div>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
      <div class="flex flex-1 flex-wrap sm:flex-nowrap items-center gap-2 max-w-2xl">
        <div class="relative flex-1 min-w-[200px]">
          <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-muted-foreground" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="搜索节点名 / 公网 Host / 地区 / Token 前缀..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <select
          v-model="onlineFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部在线状态</option>
          <option value="online">仅在线</option>
          <option value="offline">仅离线</option>
        </select>

        <select
          v-model="stateFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部调度状态</option>
          <option value="active">active (活跃)</option>
          <option value="draining">draining (排空)</option>
          <option value="circuit_broken">circuit_broken (熔断)</option>
        </select>
      </div>
    </div>

    <!-- Desktop Table View -->
    <div
      class="hidden sm:block relative w-full overflow-x-auto rounded-lg border border-border bg-neutral-950"
    >
      <table class="w-full caption-bottom text-xs border-collapse">
        <thead class="border-b border-border bg-neutral-900">
          <tr>
            <th
              class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              节点与地区
            </th>
            <th
              class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              公网 Host
            </th>
            <th
              class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              在线与调度状态
            </th>
            <th
              class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              版本与负载
            </th>
            <th
              class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              周期配额与流量
            </th>
            <th
              class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              同步状态
            </th>
            <th
              class="h-9 px-3.5 text-right align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider"
            >
              操作
            </th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr
            v-for="node in filteredNodes"
            :key="node.id"
            class="border-b border-border/30 transition-colors hover:bg-muted/25"
          >
            <!-- 1. Name, Region, TokenPrefix -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-1.5 flex-wrap">
                <span class="font-mono font-semibold text-foreground text-xs">{{ node.name }}</span>
                <Badge v-if="node.region" variant="outline" class="text-[10px]">
                  {{ node.region }}
                </Badge>
                <Badge v-if="!node.enabled" variant="secondary" class="text-[10px]"> 已停用 </Badge>
              </div>
              <div
                class="text-[11px] text-muted-foreground mt-0.5 font-mono flex items-center gap-2"
              >
                <span>ID #{{ node.id }}</span>
                <span>·</span>
                <span>Token: {{ node.tokenPrefix || '-' }}*</span>
              </div>
              <div
                v-if="node.remark"
                class="text-[10px] text-muted-foreground/80 mt-0.5 truncate max-w-[220px]"
                :title="node.remark"
              >
                {{ node.remark }}
              </div>
            </td>

            <!-- 2. Host -->
            <td class="p-3.5 align-middle font-mono">
              <div class="text-foreground text-xs truncate max-w-[180px]" :title="node.host">
                {{ node.host }}
              </div>
            </td>

            <!-- 3. Online + LastSeen + DesiredState -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-1.5 flex-wrap">
                <Badge
                  :variant="node.online ? 'success' : 'secondary'"
                  :dot="true"
                  :title="node.lastSeenAt ? formatDateTime(node.lastSeenAt) : '尚未收到心跳'"
                >
                  {{ node.online ? '在线' : '离线' }} · {{ formatLastSeen(node.lastSeenAt) }}
                </Badge>
                <Badge :variant="getDesiredStateBadge(node.desiredState).variant">
                  {{ getDesiredStateBadge(node.desiredState).label }}
                </Badge>
                <Badge
                  v-if="getAutoStateReasonBadge(node.autoStateReason)"
                  :variant="getAutoStateReasonBadge(node.autoStateReason)!.variant"
                  class="text-[10px]"
                >
                  {{ getAutoStateReasonBadge(node.autoStateReason)!.label }}
                </Badge>
              </div>
            </td>

            <!-- 4. Versions + CPU/Mem -->
            <td class="p-3.5 align-middle font-mono text-[11px]">
              <div class="flex items-center gap-2 text-foreground">
                <span>CPU {{ (node.cpuPercent || 0).toFixed(1) }}%</span>
                <span class="text-neutral-600">/</span>
                <span>MEM {{ (node.memPercent || 0).toFixed(1) }}%</span>
              </div>
              <div class="text-[10px] text-muted-foreground mt-0.5 truncate max-w-[190px]">
                Agent {{ node.agentVersion || '-' }} · Xray {{ node.xrayVersion || '-' }}
              </div>
            </td>

            <!-- 5. Traffic & Quota -->
            <td class="p-3.5 align-middle font-mono text-[11px] min-w-[190px]">
              <div v-if="getNodeQuotaStats(node).hasLimit" class="mb-1.5">
                <div class="flex items-center justify-between gap-2 text-[10px]">
                  <span class="text-neutral-200">
                    周期 {{ formatBytes(getNodeQuotaStats(node).usedBytes) }} /
                    {{ formatBytes(getNodeQuotaStats(node).limitBytes) }}
                  </span>
                  <span :class="['font-semibold', getNodeQuotaStats(node).textClass]">
                    {{ getNodeQuotaStats(node).displayPct }}%
                  </span>
                </div>
                <div class="w-full h-1.5 rounded-full bg-neutral-800 overflow-hidden mt-1">
                  <div
                    :class="[
                      'h-full rounded-full transition-all',
                      getNodeQuotaStats(node).barClass,
                    ]"
                    :style="{ width: `${getNodeQuotaStats(node).barWidth}%` }"
                  />
                </div>
              </div>
              <div v-else class="text-[10px] text-neutral-300 mb-1">
                周期已用：{{ formatBytes(getNodeQuotaStats(node).usedBytes) }} · 不限
              </div>
              <div class="text-[10px] text-muted-foreground">
                Xray: ↑ {{ formatBytes(node.xrayUpBytes || 0) }} / ↓
                {{ formatBytes(node.xrayDownBytes || 0) }}
              </div>
              <div class="text-[10px] text-muted-foreground mt-0.5">
                NIC: ↑ {{ formatBytes(node.nicUpBytes || 0) }} / ↓
                {{ formatBytes(node.nicDownBytes || 0) }}
              </div>
            </td>

            <!-- 6. Sync Status -->
            <td class="p-3.5 align-middle max-w-[220px]">
              <div v-if="getNodeSyncStatus(node) === 'error'" class="space-y-1">
                <Badge
                  variant="destructive"
                  :dot="true"
                  class="text-[10px]"
                  :title="node.lastSyncError || '配置同步失败'"
                >
                  同步失败
                </Badge>
                <div
                  v-if="node.lastSyncError"
                  class="inline-flex items-start gap-1 px-2 py-0.5 rounded bg-rose-500/10 border border-rose-500/20 text-rose-300 font-mono text-[10px] max-w-full"
                  :title="node.lastSyncError"
                >
                  <AlertTriangle class="w-3 h-3 text-rose-400 shrink-0 mt-0.5" />
                  <span class="truncate">{{ node.lastSyncError }}</span>
                </div>
              </div>
              <div v-else-if="getNodeSyncStatus(node) === 'synced'" class="space-y-1">
                <Badge
                  variant="success"
                  :dot="true"
                  class="text-[10px]"
                  :title="`配置已同步生效：${node.lastConfigHash || node.expectedConfigHash || '-'}`"
                >
                  已同步
                </Badge>
                <div
                  class="text-[10px] font-mono text-muted-foreground"
                  :title="`配置 Hash：${node.lastConfigHash || node.expectedConfigHash || '-'}`"
                >
                  #{{ formatShortHash(node.lastConfigHash || node.expectedConfigHash) }}
                </div>
              </div>
              <div v-else class="space-y-1">
                <Badge
                  variant="warning"
                  :dot="true"
                  dot-color="bg-amber-400 animate-pulse"
                  class="text-[10px]"
                  :title="`配置待同步 · 期望 ${node.expectedConfigHash || '-'} / 当前 ${node.lastConfigHash || '-'}`"
                >
                  待同步
                </Badge>
                <div
                  class="text-[10px] font-mono text-muted-foreground truncate"
                  :title="`期望 ${node.expectedConfigHash || '-'} / 当前 ${node.lastConfigHash || '-'}`"
                >
                  期望 {{ formatShortHash(node.expectedConfigHash) }} / 当前
                  {{ formatShortHash(node.lastConfigHash) }}
                </div>
              </div>
            </td>

            <!-- 7. Actions -->
            <td class="p-3.5 align-middle text-right" @click.stop>
              <div class="flex items-center justify-end gap-1.5 flex-wrap">
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px] text-cyan-400 hover:text-cyan-300 hover:bg-cyan-500/10"
                  @click="openInstallScriptModal(node)"
                >
                  安装脚本
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px] text-amber-400 hover:text-amber-300 hover:bg-amber-500/10"
                  @click="handleRotateToken(node)"
                >
                  轮换 Token
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px] text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10"
                  :loading="resettingId === node.id"
                  @click="handleResetTraffic(node)"
                >
                  重置周期流量
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  class="h-7 px-2 text-[11px]"
                  @click="openEditDrawer(node)"
                >
                  编辑
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px]"
                  @click="toggleNodeEnabled(node)"
                >
                  {{ node.enabled ? '停用' : '启用' }}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px] text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                  @click="removeNode(node)"
                >
                  删除
                </Button>
              </div>
            </td>
          </tr>

          <!-- Empty State -->
          <tr v-if="!filteredNodes.length">
            <td colspan="7" class="p-10 text-center text-muted-foreground">
              <div class="flex flex-col items-center justify-center gap-2">
                <Server class="w-8 h-8 text-neutral-600 mb-1" />
                <p class="text-xs font-medium text-neutral-300">
                  {{ nodes.length ? '没有匹配的节点记录' : '暂未注册远端直连节点' }}
                </p>
                <p class="text-[11px] text-muted-foreground">
                  {{
                    nodes.length
                      ? '可尝试调整搜索关键字或状态筛选条件'
                      : '点击右上角「新增节点」生成 Token 并部署 xray-agent'
                  }}
                </p>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Mobile Card Stream View (< sm) -->
    <div class="sm:hidden space-y-2.5">
      <div
        v-for="node in filteredNodes"
        :key="node.id"
        class="rounded-lg border border-border bg-neutral-950 p-3 space-y-2.5"
      >
        <div class="flex items-start justify-between gap-2 pb-2 border-b border-border/50">
          <div class="min-w-0 flex-1 space-y-1">
            <div class="flex items-center gap-1.5 flex-wrap">
              <span class="font-mono font-semibold text-foreground text-xs truncate">{{
                node.name
              }}</span>
              <Badge v-if="node.region" variant="outline" class="text-[10px]">
                {{ node.region }}
              </Badge>
              <Badge v-if="!node.enabled" variant="secondary" class="text-[10px]"> 已停用 </Badge>
            </div>
            <div class="font-mono text-[11px] text-muted-foreground truncate">
              {{ node.host }} · #{{ node.id }}
            </div>
          </div>

          <div class="flex flex-col items-end gap-1 shrink-0">
            <Badge :variant="node.online ? 'success' : 'secondary'" :dot="true" class="text-[10px]">
              {{ node.online ? '在线' : '离线' }}
            </Badge>
            <Badge :variant="getDesiredStateBadge(node.desiredState).variant" class="text-[10px]">
              {{ getDesiredStateBadge(node.desiredState).label }}
            </Badge>
            <Badge
              v-if="getAutoStateReasonBadge(node.autoStateReason)"
              :variant="getAutoStateReasonBadge(node.autoStateReason)!.variant"
              class="text-[10px]"
            >
              {{ getAutoStateReasonBadge(node.autoStateReason)!.label }}
            </Badge>
          </div>
        </div>

        <div class="text-[11px] font-mono">
          <div v-if="getNodeQuotaStats(node).hasLimit" class="space-y-1">
            <div class="flex items-center justify-between gap-2 text-[10px]">
              <span class="text-muted-foreground">
                周期流量 {{ formatBytes(getNodeQuotaStats(node).usedBytes) }} /
                {{ formatBytes(getNodeQuotaStats(node).limitBytes) }}
              </span>
              <span :class="['font-semibold', getNodeQuotaStats(node).textClass]">
                {{ getNodeQuotaStats(node).displayPct }}%
              </span>
            </div>
            <div class="w-full h-1.5 rounded-full bg-neutral-800 overflow-hidden">
              <div
                :class="['h-full rounded-full transition-all', getNodeQuotaStats(node).barClass]"
                :style="{ width: `${getNodeQuotaStats(node).barWidth}%` }"
              />
            </div>
          </div>
          <div v-else class="text-[10px] text-muted-foreground">
            周期已用：{{ formatBytes(getNodeQuotaStats(node).usedBytes) }} · 不限
          </div>
        </div>

        <div class="grid grid-cols-2 gap-2 text-[11px] font-mono">
          <div>
            <span class="text-muted-foreground block text-[10px]">系统负载 / 心跳</span>
            <span class="text-foreground">
              CPU {{ (node.cpuPercent || 0).toFixed(1) }}% · MEM
              {{ (node.memPercent || 0).toFixed(1) }}%
            </span>
            <span class="block text-[10px] text-muted-foreground">
              {{ formatLastSeen(node.lastSeenAt) }}
            </span>
          </div>
          <div>
            <span class="text-muted-foreground block text-[10px]">累计 Xray 流量</span>
            <span class="text-foreground block">↑ {{ formatBytes(node.xrayUpBytes || 0) }}</span>
            <span class="text-muted-foreground block"
              >↓ {{ formatBytes(node.xrayDownBytes || 0) }}</span
            >
          </div>
        </div>

        <div
          v-if="getNodeSyncStatus(node) === 'error'"
          class="p-2 rounded bg-rose-500/10 border border-rose-500/20 text-rose-300 font-mono text-[10px] space-y-1"
        >
          <div class="flex items-center gap-1.5">
            <Badge variant="destructive" :dot="true" class="text-[10px]">同步失败</Badge>
          </div>
          <div v-if="node.lastSyncError" class="flex items-start gap-1.5">
            <AlertTriangle class="w-3.5 h-3.5 text-rose-400 shrink-0 mt-0.5" />
            <span class="break-all">{{ node.lastSyncError }}</span>
          </div>
        </div>
        <div
          v-else-if="getNodeSyncStatus(node) === 'synced'"
          class="flex items-center justify-between gap-2 px-2 py-1.5 rounded bg-emerald-500/5 border border-emerald-500/20 font-mono text-[10px]"
        >
          <Badge variant="success" :dot="true" class="text-[10px]">已同步</Badge>
          <span class="text-muted-foreground">
            #{{ formatShortHash(node.lastConfigHash || node.expectedConfigHash) }}
          </span>
        </div>
        <div
          v-else
          class="flex items-center justify-between gap-2 px-2 py-1.5 rounded bg-amber-500/5 border border-amber-500/20 font-mono text-[10px]"
        >
          <Badge
            variant="warning"
            :dot="true"
            dot-color="bg-amber-400 animate-pulse"
            class="text-[10px]"
          >
            待同步
          </Badge>
          <span class="text-muted-foreground truncate">
            期望 {{ formatShortHash(node.expectedConfigHash) }} / 当前
            {{ formatShortHash(node.lastConfigHash) }}
          </span>
        </div>

        <div
          class="pt-2 border-t border-border/40 flex flex-wrap items-center justify-end gap-1.5"
          @click.stop
        >
          <Button
            variant="ghost"
            size="sm"
            class="h-7 px-2 text-[11px] text-cyan-400 hover:text-cyan-300 hover:bg-cyan-500/10"
            @click="openInstallScriptModal(node)"
          >
            安装脚本
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="h-7 px-2 text-[11px] text-amber-400 hover:text-amber-300 hover:bg-amber-500/10"
            @click="handleRotateToken(node)"
          >
            轮换 Token
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="h-7 px-2 text-[11px] text-emerald-400 hover:text-emerald-300 hover:bg-emerald-500/10"
            :loading="resettingId === node.id"
            @click="handleResetTraffic(node)"
          >
            重置周期流量
          </Button>
          <Button
            variant="secondary"
            size="sm"
            class="h-7 px-2 text-[11px]"
            @click="openEditDrawer(node)"
          >
            编辑
          </Button>
          <Button
            variant="ghost"
            size="sm"
            class="h-7 px-2 text-[11px] text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
            @click="removeNode(node)"
          >
            删除
          </Button>
        </div>
      </div>

      <div
        v-if="!filteredNodes.length"
        class="p-8 text-center rounded-lg border border-border bg-neutral-950 text-muted-foreground"
      >
        <Server class="w-8 h-8 text-neutral-600 mx-auto mb-1.5" />
        <p class="text-xs font-medium text-neutral-300">
          {{ nodes.length ? '没有匹配的节点记录' : '暂未注册远端直连节点' }}
        </p>
        <p class="text-[11px] text-muted-foreground mt-0.5">
          {{ nodes.length ? '可尝试调整筛选条件' : '点击右上角「新增节点」开始部署' }}
        </p>
      </div>
    </div>

    <!-- Node Form Drawer -->
    <NodeFormDrawer
      v-if="showFormDrawer"
      v-model="showFormDrawer"
      :is-editing="isEditing"
      :initial-data="editingNode"
      @created="onNodeCreated"
      @saved="onNodeSaved"
    />

    <!-- One-time Token & Install Script Modal -->
    <NodeTokenModal v-if="showTokenModal" v-model="showTokenModal" :modal-state="tokenModalState" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted, defineAsyncComponent } from 'vue'
import {
  Activity,
  AlertTriangle,
  ArrowUpDown,
  CheckCircle2,
  Plus,
  RefreshCw,
  Search,
  Server,
} from 'lucide-vue-next'
import Badge from '../components/ui/Badge.vue'
import Button from '../components/ui/Button.vue'
import { formatBytes, formatDateTime } from '../utils/format'
import {
  formatShortHash,
  getNodeSyncStatus,
  type CreateNodeResult,
  type NodeItem,
} from '../api/nodes'
import type { NodeTokenModalState } from './nodes/types'
import {
  formatLastSeen,
  getAutoStateReasonBadge,
  getDesiredStateBadge,
  getNodeQuotaStats,
  useNodes,
} from './nodes/composables/useNodes'

const NodeFormDrawer = defineAsyncComponent(() => import('./nodes/components/NodeFormDrawer.vue'))
const NodeTokenModal = defineAsyncComponent(() => import('./nodes/components/NodeTokenModal.vue'))

const {
  nodes,
  loading,
  resettingId,
  searchQuery,
  stateFilter,
  onlineFilter,
  summary,
  filteredNodes,
  fetchNodes,
  removeNode,
  toggleNodeEnabled,
  rotateToken,
  handleResetTraffic,
} = useNodes()

const showFormDrawer = ref(false)
const isEditing = ref(false)
const editingNode = ref<NodeItem | null>(null)

const showTokenModal = ref(false)
const tokenModalState = ref<NodeTokenModalState | null>(null)

const openCreateDrawer = () => {
  isEditing.value = false
  editingNode.value = null
  showFormDrawer.value = true
}

const openEditDrawer = (node: NodeItem) => {
  isEditing.value = true
  editingNode.value = node
  showFormDrawer.value = true
}

const openInstallScriptModal = (node: NodeItem) => {
  tokenModalState.value = {
    nodeId: node.id,
    nodeName: node.name,
    tokenPrefix: node.tokenPrefix,
    mode: 'install',
  }
  showTokenModal.value = true
}

const onNodeCreated = async (created: CreateNodeResult) => {
  await fetchNodes(true)
  tokenModalState.value = {
    nodeId: created.id,
    nodeName: created.name,
    token: created.token,
    tokenPrefix: created.tokenPrefix,
    mode: 'created',
  }
  showTokenModal.value = true
}

const onNodeSaved = async () => {
  await fetchNodes(true)
}

const handleRotateToken = async (node: NodeItem) => {
  const res = await rotateToken(node)
  if (!res) return
  tokenModalState.value = {
    nodeId: res.nodeId,
    nodeName: node.name,
    token: res.token,
    tokenPrefix: res.tokenPrefix,
    mode: 'rotated',
  }
  showTokenModal.value = true
}

let pollTimer: ReturnType<typeof setInterval> | null = null
let pollInFlight = false

const pollNodes = async () => {
  if (pollInFlight) return
  pollInFlight = true
  try {
    await fetchNodes(true)
  } finally {
    pollInFlight = false
  }
}

const handleVisibilityChange = () => {
  if (document.hidden) {
    if (pollTimer) {
      clearInterval(pollTimer)
      pollTimer = null
    }
  } else {
    pollNodes()
    if (!pollTimer) {
      pollTimer = setInterval(pollNodes, 15000)
    }
  }
}

onMounted(() => {
  fetchNodes(false)
  pollTimer = setInterval(pollNodes, 15000)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
