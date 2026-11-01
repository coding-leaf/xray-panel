<template>
  <div
    :data-node-id="node.id"
    class="relative rounded-lg border bg-card p-4 transition-all duration-200 select-none group"
    :class="[
      isActive ? 'border-emerald-400/80 shadow-[0_0_15px_rgba(52,211,153,0.2)] ring-1 ring-emerald-400/50' : 'border-border hover:border-border/80',
      isDimmed ? 'opacity-25 grayscale-[30%]' : 'opacity-100',
      node.metrics?.isIdle ? 'ring-1 ring-amber-500/30' : '',
    ]"
    @mouseenter="$emit('hover', node.id)"
    @mouseleave="$emit('leave')"
  >
    <!-- Left Receiving Anchor Dot -->
    <div
      data-anchor="left"
      class="absolute -left-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-emerald-400 border-2 border-background shadow-[0_0_8px_rgba(52,211,153,0.8)] z-10 transition-transform group-hover:scale-125"
      title="接收来自通道的流量"
    />

    <!-- Header: Protocol & Reference Count -->
    <div class="flex items-center justify-between gap-2 mb-2">
      <div class="flex items-center gap-1.5 min-w-0">
        <span
          class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold uppercase shrink-0"
          :class="protocolBadgeColor(node.protocol)"
        >
          {{ node.protocol }}
        </span>
        <span class="text-sm font-bold font-mono text-foreground truncate tracking-tight">
          {{ node.tag }}
        </span>
      </div>

      <!-- Ref Count Badge -->
      <span
        v-if="!node.metrics?.isIdle"
        class="px-2 py-0.5 rounded-md text-[10px] font-mono font-medium bg-muted text-muted-foreground border border-border shrink-0"
      >
        引用: {{ node.metrics?.referenceCount || 0 }}
      </span>

      <!-- Idle Warning Badge -->
      <span
        v-else
        class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold bg-amber-500/15 text-amber-400 border border-amber-500/30 flex items-center gap-1 shrink-0 animate-pulse"
      >
        <AlertTriangle class="w-2.5 h-2.5" />
        <span>闲置未挂载</span>
      </span>
    </div>

    <!-- Idle Quick Mount Button -->
    <div v-if="node.metrics?.isIdle" class="mb-2">
      <button
        type="button"
        @click.stop="$emit('publish', node.tag)"
        class="w-full py-1.5 px-3 rounded-md bg-amber-500/15 hover:bg-amber-500/25 border border-amber-500/40 text-amber-300 font-semibold text-xs flex items-center justify-center gap-1.5 transition-colors shadow-sm"
      >
        <Zap class="w-3.5 h-3.5" />
        <span>⚡ 1步挂载为线路</span>
      </button>
    </div>

    <!-- Settings Summary Box -->
    <div class="space-y-1.5 text-xs text-muted-foreground">
      <div class="bg-background p-2 rounded-md border border-border/80 text-[11px] font-mono text-muted-foreground/90 truncate">
        {{ formatSettingsSummary(node.raw) }}
      </div>
    </div>

    <!-- Footer Actions -->
    <div class="mt-3 pt-2.5 border-t border-border/60 flex items-center justify-between text-[11px]">
      <button
        type="button"
        @click.stop="$emit('edit', node.raw)"
        class="text-foreground/80 hover:text-foreground font-medium transition-colors"
      >
        编辑设置
      </button>

      <button
        v-if="node.tag !== 'direct' && node.tag !== 'block'"
        type="button"
        @click.stop="$emit('delete', node.tag)"
        class="text-rose-400/90 hover:text-rose-300 transition-colors"
      >
        删除
      </button>
      <span v-else class="text-[10px] text-muted-foreground/60 font-mono">
        系统保留
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { AlertTriangle, Zap } from 'lucide-vue-next'
import type { TopologyNode } from '../types'

interface Props {
  node: TopologyNode
  isActive?: boolean
  isDimmed?: boolean
}

defineProps<Props>()

defineEmits<{
  (e: 'hover', id: string): void
  (e: 'leave'): void
  (e: 'edit', ob: any): void
  (e: 'delete', tag: string): void
  (e: 'publish', tag: string): void
}>()

const protocolBadgeColor = (p: string) => {
  switch ((p || '').toLowerCase()) {
    case 'vless':
      return 'bg-cyan-500/15 text-cyan-400 border border-cyan-500/30'
    case 'trojan':
      return 'bg-purple-500/15 text-purple-400 border border-purple-500/30'
    case 'freedom':
      return 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30'
    case 'wireguard':
      return 'bg-amber-500/15 text-amber-400 border border-amber-500/30'
    case 'shadowsocks':
      return 'bg-blue-500/15 text-blue-400 border border-blue-500/30'
    case 'blackhole':
      return 'bg-rose-500/15 text-rose-400 border border-rose-500/30'
    default:
      return 'bg-muted text-muted-foreground border border-border'
  }
}

const formatSettingsSummary = (ob: any) => {
  if (!ob) return ''
  if (ob.protocol === 'freedom') return '直连公网 (Freedom)'
  if (ob.protocol === 'blackhole') return '黑洞拦截 (Blackhole)'
  if (ob.protocol === 'wireguard') return 'Cloudflare WARP (WireGuard)'
  try {
    const s = JSON.parse(ob.settingsJson || '{}')
    if (s.vnext && s.vnext[0]) {
      return `${s.vnext[0].address}:${s.vnext[0].port}`
    }
    if (s.servers && s.servers[0]) {
      return `${s.servers[0].address}:${s.servers[0].port}`
    }
  } catch {}
  return ob.settingsJson || '{}'
}
</script>
