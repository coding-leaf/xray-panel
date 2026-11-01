<template>
  <div
    :data-node-id="node.id"
    class="relative rounded-xl border bg-card px-3.5 py-2.5 transition-all duration-200 select-none group"
    :class="[
      isActive ? 'border-cyan-400/80 shadow-[0_0_15px_rgba(34,211,238,0.15)] ring-1 ring-cyan-400/50' : 'border-border hover:border-border/80',
      isDimmed ? 'opacity-25 grayscale-[30%]' : 'opacity-100',
    ]"
    @mouseenter="$emit('hover', node.id)"
    @mouseleave="$emit('leave')"
  >
    <!-- Right Port Anchor Dot -->
    <div
      data-anchor="right"
      class="absolute -right-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-cyan-400 border-2 border-background shadow-[0_0_8px_rgba(34,211,238,0.8)] z-10 transition-transform group-hover:scale-125 pointer-events-none"
      title="流量流向出口"
    />

    <!-- Line 1: Status Dot + Gateway Name + Protocol/Security Badge + Hover Actions -->
    <div class="flex items-center justify-between gap-2">
      <!-- Left part -->
      <div class="flex items-center gap-1.5 min-w-0">
        <!-- Breathing Status Dot -->
        <span class="relative flex h-2 w-2 shrink-0">
          <span
            v-if="node.status === 'online'"
            class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
          />
          <span
            class="relative inline-flex rounded-full h-2 w-2"
            :class="statusDotClass(node.status)"
          />
        </span>

        <!-- Gateway Name -->
        <span class="font-bold text-xs sm:text-sm font-mono text-foreground truncate max-w-[110px]" :title="node.tag">
          {{ node.tag }}
        </span>

        <!-- Protocol & Security Badge -->
        <span
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold uppercase shrink-0 border"
          :class="protocolClass(node.protocol)"
        >
          {{ protocolLabel }}
        </span>
      </div>

      <!-- Right part: Hover Quick Action Icons or Latency -->
      <div class="flex items-center gap-1 shrink-0">
        <!-- Action Buttons (visible on hover) -->
        <div class="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-0.5">
          <button
            type="button"
            @click.stop="$emit('logs', node.tag)"
            class="p-1 rounded text-muted-foreground hover:text-cyan-400 hover:bg-muted/80 transition-colors"
            title="查看访问日志"
          >
            <FileText class="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            @click.stop="$emit('edit', node.raw)"
            class="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/80 transition-colors"
            title="编辑网关"
          >
            <Pencil class="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            @click.stop="$emit('delete', node.inboundId || node.raw.id)"
            class="p-1 rounded text-muted-foreground hover:text-rose-400 hover:bg-muted/80 transition-colors"
            title="删除网关"
          >
            <Trash2 class="w-3.5 h-3.5" />
          </button>
        </div>

        <!-- Latency badge (visible when not hovered) -->
        <span class="text-[10px] font-mono text-muted-foreground/70 group-hover:hidden">
          {{ node.latencyMs || 1 }}ms
        </span>
      </div>
    </div>

    <!-- Line 2: Port · Users · Traffic -->
    <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground mt-1.5 pt-1 border-t border-border/40">
      <div class="flex items-center gap-1 truncate">
        <span class="text-cyan-400/90 font-semibold">:{{ node.externalPort || node.listenPort }}</span>
        <span class="text-neutral-600">·</span>
        <span>{{ node.metrics?.clientCount || 0 }} 用户</span>
      </div>

      <div class="flex items-center gap-1 text-[10px] shrink-0">
        <span class="text-emerald-400/90">↑{{ formatBytes(node.metrics?.upBytes || 0) }}</span>
        <span class="text-cyan-400/90">↓{{ formatBytes(node.metrics?.downBytes || 0) }}</span>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { FileText, Pencil, Trash2 } from 'lucide-vue-next'
import type { TopologyNode } from '../types'
import { formatBytes } from '../../../utils/format'

interface Props {
  node: TopologyNode
  isActive?: boolean
  isDimmed?: boolean
}

const props = defineProps<Props>()

defineEmits<{
  (e: 'hover', id: string): void
  (e: 'leave'): void
  (e: 'edit', inb: any): void
  (e: 'delete', id: number): void
  (e: 'logs', tag: string): void
}>()

const securityType = computed(() => {
  try {
    const s = JSON.parse(props.node.raw?.streamSettings || '{}')
    return s.security || 'none'
  } catch {
    return 'none'
  }
})

const protocolLabel = computed(() => {
  const p = (props.node.protocol || 'vless').toUpperCase()
  if (p === 'VLESS') {
    if (securityType.value === 'reality') return 'VLESS · REALITY'
    if (securityType.value === 'tls') return 'VLESS · TLS'
  }
  return p
})

const protocolClass = (p: string) => {
  switch ((p || '').toLowerCase()) {
    case 'vless':
      return 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30'
    case 'trojan':
      return 'bg-purple-500/15 text-purple-400 border-purple-500/30'
    case 'shadowsocks':
    case 'ss':
      return 'bg-blue-500/15 text-blue-400 border-blue-500/30'
    case 'vmess':
      return 'bg-indigo-500/15 text-indigo-400 border-indigo-500/30'
    case 'socks':
      return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30'
    default:
      return 'bg-muted text-muted-foreground border-border'
  }
}

const statusDotClass = (status: TopologyNode['status']) => {
  switch (status) {
    case 'online':
      return 'bg-emerald-400'
    case 'offline':
      return 'bg-rose-400'
    case 'paused':
      return 'bg-neutral-500'
    default:
      return 'bg-emerald-400'
  }
}
</script>
