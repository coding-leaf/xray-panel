<template>
  <div class="space-y-4">
    <!-- Top Action & Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">用户与多节点归属管理</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ filteredUsers.length }} / {{ users.length }} 位用户
          </Badge>
          <Badge variant="success" :dot="true" class="text-[10px] hidden sm:inline-flex">
            gRPC 毫秒内存热生效
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          支持批量延期与流量重置、独立安全订阅 Token、每月周期自动重置与并发设备限制
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          variant="default"
          size="sm"
          @click="emit('add')"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加用户</span>
        </Button>
      </div>
    </div>

    <!-- Quick Stats Cards (KPI Metrics) -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>总用户数</span>
          <Users class="w-3.5 h-3.5" />
        </div>
        <div class="text-base font-bold font-mono text-foreground">
          {{ users.length }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>在线传输</span>
          <Activity class="w-3.5 h-3.5 text-emerald-400" />
        </div>
        <div class="text-base font-bold font-mono text-emerald-400">
          {{ onlineUsersCount }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>正常启用</span>
          <Check class="w-3.5 h-3.5 text-foreground" />
        </div>
        <div class="text-base font-bold font-mono text-foreground">
          {{ enabledUsersCount }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>临期/超额/停用</span>
          <AlertCircle class="w-3.5 h-3.5 text-amber-400" />
        </div>
        <div class="text-base font-bold font-mono text-amber-400">
          {{ attentionUsersCount }}
        </div>
      </div>
    </div>

    <!-- Filter & Search Bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
      <div class="flex flex-1 items-center gap-2 max-w-lg">
        <div class="relative w-full">
          <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-muted-foreground" />
          <input
            :value="searchQuery"
            @input="onSearchInput"
            type="text"
            placeholder="搜索用户名 / 邮箱 / UUID / Token / 节点..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <select
          :value="statusFilter"
          @change="onStatusFilterChange"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部状态</option>
          <option value="online">在线传输</option>
          <option value="enabled">已启用</option>
          <option value="disabled">已停用</option>
          <option value="expired">已到期</option>
          <option value="overquota">已超额</option>
        </select>
      </div>

      <div v-if="selectedUserIds.length > 0" class="flex items-center gap-2 text-xs">
        <span class="text-muted-foreground font-mono">已选 {{ selectedUserIds.length }} 位</span>
      </div>
    </div>

    <!-- Batch Action Floating Bar -->
    <div
      v-if="selectedUserIds.length > 0"
      class="rounded-lg border border-border bg-card p-3 flex flex-wrap items-center justify-between gap-3 shadow-sm transition-all"
    >
      <div class="flex items-center gap-2 text-xs">
        <Badge variant="default" class="font-mono">
          已勾选 {{ selectedUserIds.length }} 位用户
        </Badge>
        <span class="text-muted-foreground hidden sm:inline">批量执行运维指令：</span>
      </div>

      <div class="flex flex-wrap items-center gap-1.5">
        <Button
          variant="secondary"
          size="sm"
          @click="emit('batch-renew', 30)"
          class="text-xs font-mono"
        >
          <CalendarPlus class="w-3.5 h-3.5 mr-1 text-emerald-400" />
          <span>+30天</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="emit('batch-renew', 90)"
          class="text-xs font-mono"
        >
          <CalendarPlus class="w-3.5 h-3.5 mr-1 text-emerald-400" />
          <span>+90天</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="emit('batch-renew', 365)"
          class="text-xs font-mono"
        >
          <CalendarPlus class="w-3.5 h-3.5 mr-1 text-emerald-400" />
          <span>+365天</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="emit('batch-reset-traffic')"
          class="text-xs font-mono"
        >
          <RotateCcw class="w-3.5 h-3.5 mr-1 text-cyan-400" />
          <span>重置流量</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="emit('batch-set-status', true)"
          class="text-xs font-mono"
        >
          启用
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="emit('batch-set-status', false)"
          class="text-xs font-mono"
        >
          停用
        </Button>
        <Button
          variant="destructive"
          size="sm"
          @click="emit('batch-delete')"
          class="text-xs font-mono"
        >
          <Trash2 class="w-3.5 h-3.5 mr-1" />
          <span>批量删除</span>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          @click="emit('clear-selection')"
          class="text-xs text-muted-foreground hover:text-foreground"
        >
          取消
        </Button>
      </div>
    </div>

    <!-- Main Table View -->
    <Table>
      <TableHeader>
        <TableRow class="hover:bg-transparent cursor-default">
          <TableHead class="w-10 text-center">
            <input
              type="checkbox"
              :checked="isAllUsersSelected"
              @change="emit('toggle-select-all')"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
          </TableHead>
          <TableHead class="min-w-[180px]">用户名 / 标识</TableHead>
          <TableHead class="min-w-[140px]">授权入站节点</TableHead>
          <TableHead class="min-w-[160px]">流量配额</TableHead>
          <TableHead class="min-w-[130px]">有效期与重置</TableHead>
          <TableHead class="min-w-[120px]">状态与实时速率</TableHead>
          <TableHead class="text-right min-w-[170px]">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow
          v-for="user in filteredUsers"
          :key="user.id"
          @click="emit('inspect', user)"
          :class="{ 'bg-muted/30': currentInspectorUserId === user.id }"
        >
          <!-- Checkbox -->
          <TableCell class="text-center" @click.stop>
            <input
              type="checkbox"
              :checked="selectedUserIds.includes(user.id)"
              @change="onUserCheck(user.id)"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
          </TableCell>

          <!-- Username & Email & SubToken -->
          <TableCell>
            <div class="font-mono font-medium text-foreground text-xs truncate max-w-[220px]">
              {{ user.email }}
            </div>
            <div class="text-[10px] text-muted-foreground font-mono flex items-center gap-1.5 mt-0.5">
              <span v-if="user.ipLimit > 0" class="text-amber-400 font-semibold">限 {{ user.ipLimit }} IP</span>
              <span v-else>无IP限制</span>
              <span>•</span>
              <span class="truncate max-w-[120px]" :title="user.subToken || '未生成'">
                Token: {{ user.subToken ? user.subToken.substring(0, 8) + '...' : '未生成' }}
              </span>
            </div>
          </TableCell>

          <!-- Inbound Tags -->
          <TableCell>
            <div class="flex flex-wrap gap-1 max-w-[200px]">
              <Badge
                v-for="tag in getNodeTags(user).slice(0, 3)"
                :key="tag"
                variant="outline"
                class="text-[10px] px-1.5 py-0 font-mono"
              >
                <span>{{ tag }}</span>
                <span v-if="getSubRoutesCount(tag)" class="text-cyan-400 font-bold ml-0.5">({{ getSubRoutesCount(tag) }}线)</span>
              </Badge>
              <Badge
                v-if="getNodeTags(user).length > 3"
                variant="secondary"
                class="text-[10px] px-1 py-0 font-mono"
                :title="getNodeTags(user).slice(3).join(', ')"
              >
                +{{ getNodeTags(user).length - 3 }}
              </Badge>
              <span v-if="!getNodeTags(user).length" class="text-[11px] text-muted-foreground font-mono">
                未分配节点
              </span>
            </div>
          </TableCell>

          <!-- Traffic Usage & Progress -->
          <TableCell class="font-mono">
            <div class="space-y-1">
              <div class="text-xs text-foreground flex items-center justify-between">
                <span>{{ formatBytes(user.upBytes + user.downBytes) }}</span>
                <span class="text-muted-foreground text-[11px]">/ {{ user.totalBytes > 0 ? formatBytes(user.totalBytes) : '无限制' }}</span>
              </div>
              <div v-if="user.totalBytes > 0" class="w-full max-w-[140px] h-1.5 bg-neutral-900 rounded-full overflow-hidden border border-border/40">
                <div
                  class="h-full rounded-full transition-all"
                  :class="getTrafficProgressClass(user)"
                  :style="{ width: `${Math.min(100, getTrafficPercent(user))}%` }"
                />
              </div>
            </div>
          </TableCell>

          <!-- Expire Date & Reset -->
          <TableCell class="font-mono text-[11px]">
            <div :class="isUserExpired(user) ? 'text-amber-400 font-semibold' : 'text-foreground'">
              {{ user.expireTime > 0 ? formatDate(user.expireTime) : '永久有效' }}
              <span v-if="isUserExpired(user)" class="text-[10px] text-rose-400 ml-1">(已到期)</span>
            </div>
            <div v-if="user.resetDay > 0" class="text-[10px] text-cyan-400/90 mt-0.5">
              每月 {{ user.resetDay }} 日自动重置
            </div>
          </TableCell>

          <!-- Status & Live Speeds -->
          <TableCell>
            <div class="space-y-1">
              <div class="flex items-center gap-1.5">
                <Badge
                  :variant="getUserBadgeVariant(user)"
                  :dot="true"
                  class="text-[10px]"
                >
                  {{ getUserStatusText(user) }}
                </Badge>
              </div>
              <!-- Real-time transfer speed -->
              <div v-if="user.isOnline && (user.upSpeed > 0 || user.downSpeed > 0)" class="text-[10px] font-mono text-cyan-400 flex items-center gap-1">
                <span>↑{{ formatBytes(user.upSpeed) }}/s</span>
                <span>↓{{ formatBytes(user.downSpeed) }}/s</span>
              </div>
            </div>
          </TableCell>

          <!-- Quick Action Buttons -->
          <TableCell class="text-right" @click.stop>
            <div class="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="icon"
                @click="emit('inspect', user)"
                title="打开巡检抽屉"
              >
                <SlidersHorizontal class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="copyText(UserSubscriptionService.getDirectTokenSubUrl(user))"
                title="复制聚合订阅链接"
              >
                <Copy class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="emit('share', user)"
                title="二维码与安全提取码"
              >
                <QrCode class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="emit('history', user)"
                title="查看流量历史趋势"
              >
                <BarChart2 class="w-3.5 h-3.5 text-cyan-400" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="emit('edit', user)"
                title="编辑用户配置"
              >
                <Edit class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                class="text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                @click="emit('delete', user.id)"
                title="删除用户"
              >
                <Trash2 class="w-3.5 h-3.5" />
              </Button>
            </div>
          </TableCell>
        </TableRow>

        <!-- Empty State -->
        <TableRow v-if="!filteredUsers.length" class="hover:bg-transparent">
          <TableCell colspan="7" class="p-10 text-center text-muted-foreground">
            <div class="flex flex-col items-center justify-center gap-2">
              <Users class="w-8 h-8 text-neutral-600 mb-1" />
              <p class="text-xs font-medium text-neutral-300">没有匹配的用户记录</p>
              <p class="text-[11px] text-muted-foreground">可尝试调整搜索关键字或筛选条件，或点击右上角添加新用户</p>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>
  </div>
</template>

<script setup lang="ts">
import {
  Plus,
  Search,
  Edit,
  Trash2,
  RotateCcw,
  Copy,
  BarChart2,
  CalendarPlus,
  QrCode,
  SlidersHorizontal,
  Users,
  Activity,
  Check,
  AlertCircle,
} from 'lucide-vue-next'
import { copyText } from '../../../utils/clipboard'
import { formatBytes, formatDate } from '../../../utils/format'
import { UserSubscriptionService } from '../services/subscription'
import type { UserItem } from '../types'
import type { StatusFilterType } from '../composables/useUserList'

import Table from '../../../components/ui/Table.vue'
import TableHeader from '../../../components/ui/TableHeader.vue'
import TableBody from '../../../components/ui/TableBody.vue'
import TableHead from '../../../components/ui/TableHead.vue'
import TableRow from '../../../components/ui/TableRow.vue'
import TableCell from '../../../components/ui/TableCell.vue'
import Button from '../../../components/ui/Button.vue'
import Badge from '../../../components/ui/Badge.vue'

interface Props {
  users: UserItem[]
  filteredUsers: UserItem[]
  selectedUserIds: number[]
  availableInbounds: any[]
  currentInspectorUserId?: number | null
  onlineUsersCount: number
  enabledUsersCount: number
  attentionUsersCount: number
  isAllUsersSelected: boolean
  searchQuery: string
  statusFilter: StatusFilterType
  isUserExpired: (user: UserItem) => boolean
  getUserBadgeVariant: (user: UserItem) => 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive'
  getUserStatusText: (user: UserItem) => string
  getTrafficPercent: (user: UserItem) => number
  getTrafficProgressClass: (user: UserItem) => string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:searchQuery', val: string): void
  (e: 'update:statusFilter', val: StatusFilterType): void
  (e: 'update:selectedUserIds', val: number[]): void
  (e: 'toggle-select-all'): void
  (e: 'clear-selection'): void
  (e: 'add'): void
  (e: 'inspect', user: UserItem): void
  (e: 'edit', user: UserItem): void
  (e: 'share', user: UserItem): void
  (e: 'history', user: UserItem): void
  (e: 'delete', id: number): void
  (e: 'batch-renew', days: number): void
  (e: 'batch-reset-traffic'): void
  (e: 'batch-set-status', enabled: boolean): void
  (e: 'batch-delete'): void
}>()

const onSearchInput = (e: Event) => {
  const target = e.target as HTMLInputElement
  emit('update:searchQuery', target.value)
}

const onStatusFilterChange = (e: Event) => {
  const target = e.target as HTMLSelectElement
  emit('update:statusFilter', target.value as StatusFilterType)
}

const onUserCheck = (userId: number) => {
  const ids = [...props.selectedUserIds]
  const idx = ids.indexOf(userId)
  if (idx >= 0) {
    ids.splice(idx, 1)
  } else {
    ids.push(userId)
  }
  emit('update:selectedUserIds', ids)
}

const getNodeTags = (user: UserItem): string[] => {
  return UserSubscriptionService.getNodeTags(user, props.availableInbounds)
}

const getSubRoutesCount = (tag: string): number => {
  const inb = props.availableInbounds.find((i: any) => i.tag === tag)
  if (!inb) return 0
  try {
    const srs = JSON.parse(inb.subRoutesJson || '[]')
    return srs.filter((s: any) => s.enabled !== false).length
  } catch (_e) {
    return 0
  }
}
</script>
