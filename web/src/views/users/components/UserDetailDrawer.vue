<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="`用户巡检: ${user?.email || ''}`"
    :description="`UUID: ${user?.uuid || '未分配'} · 独立 Token 聚合订阅`"
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <div v-if="user" class="space-y-4 text-xs">
      <!-- 1. Quick Action & Status Header Card -->
      <div class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">实时状态与快捷操作</span>
          <div class="flex items-center gap-1.5">
            <Badge :variant="getUserBadgeVariant(user)" :dot="true">
              {{ getUserStatusText(user) }}
            </Badge>
            <Badge v-if="user.isOnline" variant="success" class="text-[10px]">
              ↑{{ formatBytes(user.upSpeed || 0) }}/s ↓{{ formatBytes(user.downSpeed || 0) }}/s
            </Badge>
          </div>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1">
          <Button
            variant="outline"
            size="sm"
            @click="copyText(UserSubscriptionService.getDirectTokenSubUrl(user))"
            class="w-full justify-center"
          >
            <Copy class="w-3.5 h-3.5 mr-1" />
            <span>复制订阅</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="emit('share', user)"
            class="w-full justify-center"
          >
            <QrCode class="w-3.5 h-3.5 mr-1" />
            <span>二维码/凭据</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="emit('history', user)"
            class="w-full justify-center"
          >
            <BarChart2 class="w-3.5 h-3.5 mr-1 text-cyan-400" />
            <span>流量趋势</span>
          </Button>
          <Button
            variant="outline"
            size="sm"
            @click="emit('toggle-enabled', user)"
            :class="user.enabled ? 'hover:text-rose-400' : 'hover:text-emerald-400'"
            class="w-full justify-center"
          >
            <span>{{ user.enabled ? '停用账号' : '启用账号' }}</span>
          </Button>
        </div>
      </div>

      <!-- 2. Quota & Bandwidth Diagnostics -->
      <div class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">流量配额与消耗</span>
          <Button
            variant="ghost"
            size="sm"
            class="h-6 text-[11px] px-2 text-cyan-400"
            @click="emit('reset-traffic', user.id)"
          >
            <RotateCcw class="w-3 h-3 mr-1" /> 重置流量
          </Button>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 font-mono">
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">已用总计</div>
            <div class="text-sm font-semibold text-foreground mt-0.5">
              {{ formatBytes(user.upBytes + user.downBytes) }}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">配额限额</div>
            <div class="text-sm font-semibold text-foreground mt-0.5">
              {{ user.totalBytes > 0 ? formatBytes(user.totalBytes) : '无限制' }}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">上行上传</div>
            <div class="text-sm font-semibold text-emerald-400 mt-0.5">
              {{ formatBytes(user.upBytes) }}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">下行下载</div>
            <div class="text-sm font-semibold text-cyan-400 mt-0.5">
              {{ formatBytes(user.downBytes) }}
            </div>
          </div>
        </div>

        <div v-if="user.totalBytes > 0" class="space-y-1.5 pt-1">
          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-muted-foreground">配额使用率</span>
            <span :class="getTrafficPercent(user) > 90 ? 'text-rose-400 font-bold' : 'text-foreground'">
              {{ getTrafficPercent(user).toFixed(1) }}%
            </span>
          </div>
          <div class="w-full h-2 bg-neutral-900 rounded-full overflow-hidden border border-border/40">
            <div
              class="h-full rounded-full transition-all"
              :class="getTrafficProgressClass(user)"
              :style="{ width: `${Math.min(100, getTrafficPercent(user))}%` }"
            />
          </div>
        </div>
      </div>

      <!-- 3. Lifecycle & Cycle Settings -->
      <div class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">有效期与重置周期</span>
          <div class="flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              class="h-6 text-[11px] px-2 text-emerald-400"
              @click="emit('renew-one', user.id, 30)"
            >
              +30天
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 text-[11px] px-2 text-emerald-400"
              @click="emit('renew-one', user.id, 90)"
            >
              +90天
            </Button>
          </div>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 font-mono">
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">账号到期日</div>
            <div class="text-xs font-semibold text-foreground mt-0.5">
              {{ user.expireTime > 0 ? formatDate(user.expireTime) : '永久有效' }}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">每月自动重置</div>
            <div class="text-xs font-semibold text-cyan-400 mt-0.5">
              {{ user.resetDay > 0 ? `每月 ${user.resetDay} 日` : '不自动重置' }}
            </div>
          </div>
          <div>
            <div class="text-[10px] text-muted-foreground uppercase">并发设备限额</div>
            <div class="text-xs font-semibold text-foreground mt-0.5">
              {{ user.ipLimit > 0 ? `限制 ${user.ipLimit} IP` : '无限制' }}
            </div>
          </div>
        </div>
      </div>

      <!-- 4. Security & Credentials -->
      <div class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">认证凭据与密钥</span>
          <Button
            variant="ghost"
            size="sm"
            class="h-6 text-[11px] px-2 text-rose-400"
            @click="emit('reset-token', user.id)"
          >
            <RotateCcw class="w-3 h-3 mr-1" /> 重置 Token/密钥
          </Button>
        </div>

        <div class="space-y-2.5 font-mono">
          <div>
            <label class="text-[10px] text-muted-foreground uppercase block mb-1">UUID / 密码</label>
            <div class="flex items-center gap-1.5">
              <input
                :value="user.uuid"
                readonly
                class="flex-1 bg-neutral-950 border border-border rounded-md px-2.5 py-1 text-xs text-foreground font-mono select-all focus:outline-none"
              />
              <Button variant="secondary" size="sm" class="h-7 px-2" @click="copyText(user.uuid || '')">
                <Copy class="w-3 h-3" />
              </Button>
            </div>
          </div>

          <div>
            <label class="text-[10px] text-muted-foreground uppercase block mb-1">订阅安全 Token</label>
            <div class="flex items-center gap-1.5">
              <input
                :value="user.subToken || '未生成'"
                readonly
                class="flex-1 bg-neutral-950 border border-border rounded-md px-2.5 py-1 text-xs text-foreground font-mono select-all focus:outline-none"
              />
              <Button
                v-if="user.subToken"
                variant="secondary"
                size="sm"
                class="h-7 px-2"
                @click="copyText(user.subToken)"
              >
                <Copy class="w-3 h-3" />
              </Button>
            </div>
          </div>

          <div>
            <label class="text-[10px] text-muted-foreground uppercase block mb-1">通用聚合订阅 URL</label>
            <div class="flex items-center gap-1.5">
              <input
                :value="UserSubscriptionService.getDirectTokenSubUrl(user)"
                readonly
                class="flex-1 bg-neutral-950 border border-border rounded-md px-2.5 py-1 text-xs text-foreground font-mono select-all focus:outline-none"
              />
              <Button variant="secondary" size="sm" class="h-7 px-2" @click="copyText(UserSubscriptionService.getDirectTokenSubUrl(user))">
                <Copy class="w-3 h-3" />
              </Button>
            </div>
          </div>
        </div>
      </div>

      <!-- 5. Associated Inbounds Matrix -->
      <div class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">
            已授权入站网关 ({{ getNodeTags(user).length }})
          </span>
          <Button
            variant="ghost"
            size="sm"
            class="h-6 text-[11px] px-2 text-foreground"
            @click="emit('edit', user)"
          >
            <Edit class="w-3 h-3 mr-1" /> 变更授权节点
          </Button>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
          <div
            v-for="tag in getNodeTags(user)"
            :key="tag"
            class="p-2.5 rounded-md border border-border bg-neutral-950 font-mono flex items-center justify-between"
          >
            <div class="min-w-0 pr-2">
              <div class="text-xs font-semibold text-foreground truncate">{{ tag }}</div>
              <div class="text-[10px] text-muted-foreground mt-0.5">
                <span v-if="getInboundMeta(tag)">{{ getInboundMeta(tag)?.protocol?.toUpperCase() }} :{{ getInboundMeta(tag)?.port }}</span>
                <span v-else>自定义接入点</span>
                <span v-if="getSubRoutesCount(tag)" class="text-cyan-400 font-bold ml-1.5">({{ getSubRoutesCount(tag) }}条线路)</span>
              </div>
            </div>
            <Badge variant="outline" class="text-[10px] shrink-0 font-mono">
              已授权
            </Badge>
          </div>
          <div v-if="!getNodeTags(user).length" class="col-span-2 text-center py-4 text-muted-foreground text-xs font-mono">
            暂未绑定入站节点，请点击编辑进行关联
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <div v-if="user" class="flex items-center justify-between w-full">
        <Button
          variant="destructive"
          size="sm"
          @click="emit('delete', user.id)"
        >
          <Trash2 class="w-3.5 h-3.5 mr-1" />
          <span>删除用户</span>
        </Button>
        <div class="flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            @click="emit('update:modelValue', false)"
          >
            关闭
          </Button>
          <Button
            variant="default"
            size="sm"
            @click="emit('edit', user)"
          >
            <Edit class="w-3.5 h-3.5 mr-1" />
            <span>编辑配置</span>
          </Button>
        </div>
      </div>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import {
  Copy,
  QrCode,
  BarChart2,
  RotateCcw,
  Edit,
  Trash2,
} from 'lucide-vue-next'
import { copyText } from '../../../utils/clipboard'
import { formatBytes, formatDate } from '../../../utils/format'
import { UserSubscriptionService } from '../services/subscription'
import type { UserItem } from '../types'

import Drawer from '../../../components/ui/Drawer.vue'
import Button from '../../../components/ui/Button.vue'
import Badge from '../../../components/ui/Badge.vue'

interface Props {
  modelValue: boolean
  user?: UserItem | null
  availableInbounds: any[]
  getUserBadgeVariant: (user: UserItem) => 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive'
  getUserStatusText: (user: UserItem) => string
  getTrafficPercent: (user: UserItem) => number
  getTrafficProgressClass: (user: UserItem) => string
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'edit', user: UserItem): void
  (e: 'delete', id: number): void
  (e: 'share', user: UserItem): void
  (e: 'history', user: UserItem): void
  (e: 'toggle-enabled', user: UserItem): void
  (e: 'reset-traffic', id: number): void
  (e: 'renew-one', id: number, days: number): void
  (e: 'reset-token', id: number): void
}>()

const getNodeTags = (user: UserItem): string[] => {
  return UserSubscriptionService.getNodeTags(user, props.availableInbounds)
}

const getInboundMeta = (tag: string) => {
  return props.availableInbounds.find((i: any) => i.tag === tag)
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
