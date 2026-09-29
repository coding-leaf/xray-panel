<template>
  <div class="relative w-full overflow-hidden rounded-lg border border-border bg-neutral-950">
    <table class="w-full caption-bottom text-xs border-collapse">
      <thead class="border-b border-border bg-neutral-900">
        <tr>
          <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">节点标识 (Tag)</th>
          <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">协议与流控</th>
          <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">端口映射与网络</th>
          <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">传输安全 / 伪装</th>
          <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">用户与线路</th>
          <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">端口状态</th>
          <th class="h-9 px-3.5 text-right align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">操作</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-border/40">
        <tr
          v-for="inb in inbounds"
          :key="inb.id"
          @click="emit('inspect', inb)"
          class="border-b border-border/30 transition-colors hover:bg-muted/30 cursor-pointer"
          :class="{ 'bg-muted/20': selectedInbound?.id === inb.id }"
        >
          <!-- 1. Tag & Remarks -->
          <td class="p-3.5 align-middle">
            <div class="flex items-center gap-2">
              <span class="font-mono font-semibold text-foreground text-xs">{{ inb.tag }}</span>
              <span v-if="inb.routeId && inb.routeId > 0 && (!inb.subRoutes || inb.subRoutes.length === 0)" class="text-[10px] font-mono px-1 py-0.2 rounded bg-neutral-800 text-neutral-300 border border-neutral-700">
                #{{ inb.routeId }}
              </span>
            </div>
            <div class="text-[11px] text-muted-foreground mt-0.5 font-mono truncate max-w-[200px]">
              {{ inb.listen || '0.0.0.0' }}:{{ inb.port }}
            </div>
          </td>

          <!-- 2. Protocol & Flow -->
          <td class="p-3.5 align-middle">
            <div class="flex items-center gap-1.5">
              <Badge :variant="getProtocolBadgeVariant(inb.protocol)">
                {{ inb.protocol?.toUpperCase() }}
              </Badge>
              <span v-if="getNodeFlow(inb) !== 'none'" class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-neutral-900 text-neutral-300 border border-neutral-800">
                {{ getNodeFlow(inb) }}
              </span>
            </div>
          </td>

          <!-- 3. Port & Network -->
          <td class="p-3.5 align-middle font-mono">
            <div class="flex items-center gap-1.5 text-xs">
              <span class="text-foreground">:{{ inb.externalPort || inb.port }}</span>
              <span class="text-neutral-600">/</span>
              <span class="text-neutral-400 text-[11px] uppercase">{{ getStreamNetwork(inb) }}</span>
              <span v-if="(inb.externalPort || inb.port) !== 443 && isReality(inb)" class="text-amber-400 text-[10px]" title="非443端口Reality存在阻断风险">
                ⚠️
              </span>
            </div>
            <div v-if="inb.externalHost" class="text-[10px] text-muted-foreground truncate max-w-[150px]">
              {{ inb.externalHost }}
            </div>
          </td>

          <!-- 4. Security & Reality -->
          <td class="p-3.5 align-middle">
            <div class="flex items-center gap-1.5">
              <Badge :variant="getSecurityBadgeVariant(getSecurityType(inb))">
                {{ getSecurityType(inb)?.toUpperCase() }}
              </Badge>
              <!-- Reality Status Indicator -->
              <div v-if="isReality(inb) && getInboundRealityStatus(inb.tag)">
                <span
                  class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono border"
                  :class="getRealityBadgeClass(getInboundRealityStatus(inb.tag)!.status)"
                  :title="getInboundRealityStatus(inb.tag)!.badgeText"
                >
                  <span class="w-1 h-1 rounded-full" :class="getRealityDotClass(getInboundRealityStatus(inb.tag)!.status)"></span>
                  <span>{{ getInboundRealityStatus(inb.tag)!.status.toUpperCase() }}</span>
                </span>
              </div>
            </div>
          </td>

          <!-- 5. Users & SubRoutes -->
          <td class="p-3.5 align-middle font-mono text-xs">
            <div class="flex items-center gap-2">
              <span class="text-foreground">{{ getClientCount(inb, usersList) }} 用户</span>
              <span v-if="inb.subRoutes?.length" class="text-[10px] px-1.5 py-0.5 rounded bg-muted/60 text-muted-foreground border border-border/40">
                {{ inb.subRoutes.length }} 线路
              </span>
            </div>
          </td>

          <!-- 6. Alive & Latency -->
          <td class="p-3.5 align-middle font-mono text-xs">
            <div class="flex items-center gap-1.5">
              <span
                class="w-1.5 h-1.5 rounded-full"
                :class="inb.isAlive ? 'bg-emerald-400' : 'bg-rose-500'"
              />
              <span :class="inb.isAlive ? 'text-neutral-300' : 'text-rose-400'">
                {{ inb.isAlive ? `${inb.latencyMs || 1}ms` : '未响应' }}
              </span>
            </div>
          </td>

          <!-- 7. Actions -->
          <td class="p-3.5 align-middle text-right" @click.stop>
            <div class="flex items-center justify-end gap-1.5">
              <Button
                variant="ghost"
                size="sm"
                class="h-7 px-2 text-[11px]"
                @click="emit('inspect', inb)"
              >
                详情
              </Button>
              <Button
                variant="secondary"
                size="sm"
                class="h-7 px-2 text-[11px]"
                @click="emit('edit', inb)"
              >
                编辑
              </Button>
              <Button
                variant="ghost"
                size="sm"
                class="h-7 px-2 text-[11px] text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                @click="emit('delete', inb.id)"
              >
                删除
              </Button>
            </div>
          </td>
        </tr>

        <!-- Empty State -->
        <tr v-if="!inbounds.length">
          <td colspan="7" class="p-10 text-center text-muted-foreground">
            <div class="flex flex-col items-center justify-center gap-2">
              <Radio class="w-8 h-8 text-neutral-600 mb-1" />
              <p class="text-xs font-medium text-neutral-300">没有匹配的入站节点</p>
              <p class="text-[11px] text-muted-foreground">可尝试调整搜索条件，或点击右上角添加新节点</p>
            </div>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { Radio } from 'lucide-vue-next'
import Badge from '../../../components/ui/Badge.vue'
import Button from '../../../components/ui/Button.vue'
import type { RealitySummaryStatus } from '../../../api/reality'
import type { InboundItem } from '../types'
import {
  getProtocolBadgeVariant,
  getSecurityBadgeVariant,
  getNodeFlow,
  getStreamNetwork,
  getSecurityType,
  isReality,
  getClientCount,
  getInboundRealityOverallStatus,
  getRealityBadgeClass,
  getRealityDotClass,
} from '../composables/useInboundList'

const props = defineProps<{
  inbounds: InboundItem[]
  selectedInbound?: InboundItem | null
  usersList?: any[]
  realitySummary?: RealitySummaryStatus | null
}>()

const emit = defineEmits<{
  (e: 'inspect', inb: InboundItem): void
  (e: 'edit', inb: InboundItem): void
  (e: 'delete', id: number): void
}>()

const getInboundRealityStatus = (tag: string) => {
  return getInboundRealityOverallStatus(tag, props.realitySummary || null)
}
</script>
