<template>
  <div
    :data-node-id="node.id"
    class="relative rounded-lg border bg-card p-4 transition-all duration-200 select-none group"
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
      class="absolute -left-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-indigo-400 border-2 border-background shadow-[0_0_8px_rgba(129,140,248,0.8)] z-10 transition-transform group-hover:scale-125"
      title="接收来自接入网关的流量"
    />

    <!-- Right Emitting Anchor Dot -->
    <div
      data-anchor="right"
      class="absolute -right-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-indigo-400 border-2 border-background shadow-[0_0_8px_rgba(129,140,248,0.8)] z-10 transition-transform group-hover:scale-125"
      title="输出至落地出口"
    />

    <!-- Header: Badge & Status -->
    <div class="flex items-center justify-between gap-2 mb-2">
      <div class="flex items-center gap-1.5">
        <span
          v-if="node.category === 'subroute' && (node.routeId ?? 0) > 0"
          class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-indigo-950/60 text-indigo-300 border border-indigo-800/50"
        >
          Route #{{ node.routeId }}
        </span>
        <span
          v-else-if="node.category === 'scoped_rule'"
          class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-purple-950/60 text-purple-300 border border-purple-800/50"
        >
          专属规则
        </span>
        <span
          v-else-if="node.category === 'global_rule'"
          class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-sky-950/60 text-sky-300 border border-sky-800/50"
        >
          全局规则
        </span>
        <span
          v-else
          class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-muted text-muted-foreground border border-border"
        >
          缺省直连
        </span>

        <span
          v-if="node.status === 'blocked'"
          class="px-1.5 py-0.2 rounded text-[10px] bg-rose-500/15 text-rose-400 border border-rose-500/30 font-medium"
        >
          拦截
        </span>
      </div>

      <span
        class="text-[11px] font-mono font-medium"
        :class="statusTextColor"
      >
        {{ statusText }}
      </span>
    </div>

    <!-- Title -->
    <div class="mb-2">
      <h4 class="text-sm font-bold text-foreground truncate tracking-tight" :title="node.title">
        {{ node.title }}
      </h4>
      <p v-if="node.subtitle" class="text-[10px] text-muted-foreground truncate font-mono mt-0.5">
        {{ node.subtitle }}
      </p>
    </div>

    <!-- Routing Pipeline Flow Badge -->
    <div class="bg-background p-2.5 rounded-md border border-border/80 space-y-1.5 text-[11px] font-mono">
      <div class="flex items-center justify-between text-muted-foreground">
        <span class="text-muted-foreground/70">源网关:</span>
        <span class="text-cyan-400 font-semibold truncate max-w-[130px]">{{ node.gatewayTag || '通用入站' }}</span>
      </div>
      <div class="flex items-center justify-between text-muted-foreground">
        <span class="text-muted-foreground/70">目标出口:</span>
        <span class="text-indigo-300 font-semibold truncate max-w-[130px] flex items-center gap-1">
          <span>{{ node.outboundTag }}</span>
        </span>
      </div>
    </div>

    <!-- Footer Actions (for manageable channels like SubRoute) -->
    <div
      v-if="node.category === 'subroute' || node.category === 'fallback'"
      class="mt-3 pt-2.5 border-t border-border/60 flex items-center justify-between text-[11px]"
    >
      <button
        type="button"
        @click.stop="$emit('toggleStatus', node)"
        class="text-muted-foreground hover:text-foreground font-medium transition-colors"
      >
        {{ node.category === 'fallback' ? (node.status === 'paused' ? '启用直通' : '暂停直通') : (node.status === 'paused' ? '启用线路' : '暂停线路') }}
      </button>
      <button
        type="button"
        @click.stop="$emit('delete', node)"
        class="text-rose-400/90 hover:text-rose-300 transition-colors font-medium"
      >
        {{ node.category === 'fallback' ? '删除网关' : '删除线路' }}
      </button>
    </div>

    <div
      v-else
      class="mt-3 pt-2.5 border-t border-border/60 flex items-center justify-between text-[10px] font-mono text-muted-foreground/80"
    >
      <span>路由分流引擎</span>
      <span class="text-muted-foreground/60">内核规则匹配</span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
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

const statusText = computed(() => {
  if (props.node.status === 'paused') return '已暂停'
  if (props.node.status === 'blocked') return '阻断'
  return '已生效'
})

const statusTextColor = computed(() => {
  if (props.node.status === 'paused') return 'text-muted-foreground line-through'
  if (props.node.status === 'blocked') return 'text-rose-400'
  return 'text-emerald-400'
})
</script>
