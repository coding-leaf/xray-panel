<template>
  <div
    :data-node-id="node.id"
    class="relative rounded-xl border bg-card px-3.5 py-2.5 transition-all duration-200 select-none group"
    :class="[
      isActive ? 'border-indigo-400/80 shadow-[0_0_15px_rgba(129,140,248,0.2)] ring-1 ring-indigo-400/50' : 'border-border hover:border-border/80',
      isDimmed ? 'opacity-25 grayscale-[30%]' : 'opacity-100',
    ]"
    @mouseenter="$emit('hover', node.id)"
    @mouseleave="$emit('leave')"
  >
    <!-- Left Receiving Anchor Dot -->
    <div
      data-anchor="left"
      class="absolute -left-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-indigo-400 border-2 border-background shadow-[0_0_8px_rgba(129,140,248,0.8)] z-10 transition-transform group-hover:scale-125 pointer-events-none"
      title="接收来自接入网关的流量"
    />

    <!-- Right Emitting Anchor Dot -->
    <div
      data-anchor="right"
      class="absolute -right-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-indigo-400 border-2 border-background shadow-[0_0_8px_rgba(129,140,248,0.8)] z-10 transition-transform group-hover:scale-125 pointer-events-none"
      title="输出至落地出口"
    />

    <!-- Line 1: Route Badge + Channel Name + Status Dot / Hover Actions -->
    <div class="flex items-center justify-between gap-2">
      <!-- Left part -->
      <div class="flex items-center gap-1.5 min-w-0">
        <!-- Badge -->
        <span
          v-if="node.category === 'subroute' && (node.routeId ?? 0) > 0"
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-indigo-950/70 text-indigo-300 border border-indigo-800/60 shrink-0"
        >
          Route #{{ node.routeId }}
        </span>
        <span
          v-else-if="node.category === 'scoped_rule'"
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-purple-950/70 text-purple-300 border border-purple-800/60 shrink-0"
        >
          规则分流
        </span>
        <span
          v-else-if="node.category === 'global_rule'"
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-sky-950/70 text-sky-300 border border-sky-800/60 shrink-0"
        >
          {{ node.status === 'blocked' ? '全局拦截' : '全局分流' }}
        </span>
        <span
          v-else
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold bg-muted text-muted-foreground border border-border shrink-0"
        >
          直通
        </span>

        <!-- Channel Title -->
        <span
          class="font-bold text-xs sm:text-sm text-foreground truncate max-w-[130px]"
          :class="node.status === 'paused' ? 'line-through text-muted-foreground' : ''"
          :title="node.title"
        >
          {{ node.title }}
        </span>
      </div>

      <!-- Right part: Status Dot or Hover Action Buttons -->
      <div class="flex items-center gap-1 shrink-0">
        <!-- Hover Quick Actions for SubRoute & Fallback -->
        <div
          v-if="node.category === 'subroute' || node.category === 'fallback'"
          class="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-0.5"
        >
          <button
            type="button"
            @click.stop="$emit('toggleStatus', node)"
            class="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/80 transition-colors"
            :title="node.status === 'paused' ? '启用线路' : '暂停线路'"
          >
            <Play v-if="node.status === 'paused'" class="w-3.5 h-3.5 text-emerald-400" />
            <Pause v-else class="w-3.5 h-3.5 text-amber-400" />
          </button>
          <button
            type="button"
            @click.stop="$emit('delete', node)"
            class="p-1 rounded text-muted-foreground hover:text-rose-400 hover:bg-muted/80 transition-colors"
            :title="node.category === 'fallback' ? '删除网关' : '删除线路'"
          >
            <Trash2 class="w-3.5 h-3.5" />
          </button>
        </div>

        <!-- Status Dot (visible when not hovered) -->
        <span
          class="w-2 h-2 rounded-full shrink-0 group-hover:hidden"
          :class="statusDotClass"
          :title="statusText"
        />
      </div>
    </div>

    <!-- Line 2: Single-line Flow Summary (e.g. ipv4入口, ipv6入口 ➔ out-us2) -->
    <div class="flex items-center gap-1.5 text-[11px] font-mono text-muted-foreground mt-1.5 pt-1 border-t border-border/40 truncate">
      <span class="text-cyan-400/90 font-medium truncate max-w-[120px]" :title="node.gatewayTag || '通用入口'">
        {{ node.gatewayTag || '通用入口' }}
      </span>
      <span class="text-neutral-500 shrink-0">➔</span>
      <span class="text-indigo-300 font-semibold truncate max-w-[100px]" :title="node.outboundTag">
        {{ node.outboundTag }}
      </span>
      <span v-if="node.subtitle" class="text-neutral-500 text-[10px] truncate max-w-[100px]" :title="node.subtitle">
        ({{ node.subtitle }})
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Play, Pause, Trash2 } from 'lucide-vue-next'
import type { TopologyNode } from '../types'

interface Props {
  node: TopologyNode
  isActive?: boolean
  isDimmed?: boolean
}

const props = defineProps<Props>()

defineEmits<{
  (e: 'hover', id: string): void
  (e: 'leave'): void
  (e: 'toggleStatus', node: TopologyNode): void
  (e: 'delete', node: TopologyNode): void
}>()

const statusDotClass = computed(() => {
  if (props.node.status === 'paused') return 'bg-neutral-500'
  if (props.node.status === 'blocked') return 'bg-rose-500'
  return 'bg-emerald-400'
})

const statusText = computed(() => {
  if (props.node.status === 'paused') return '已暂停'
  if (props.node.status === 'blocked') return '阻断'
  return '正常运行'
})
</script>
