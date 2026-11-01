<template>
  <div
    :data-node-id="node.id"
    class="relative rounded-xl border bg-card px-3.5 py-2.5 transition-all duration-200 select-none group"
    :class="[
      isActive ? 'border-emerald-400/80 shadow-[0_0_15px_rgba(52,211,153,0.2)] ring-1 ring-emerald-400/50' : 'border-border hover:border-border/80',
      isDimmed ? 'opacity-25 grayscale-[30%]' : 'opacity-100',
      node.metrics?.isIdle ? 'ring-1 ring-amber-500/40 bg-amber-500/[0.02]' : '',
    ]"
    @mouseenter="$emit('hover', node.id)"
    @mouseleave="$emit('leave')"
  >
    <!-- Left Receiving Anchor Dot -->
    <div
      data-anchor="left"
      class="absolute -left-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-emerald-400 border-2 border-background shadow-[0_0_8px_rgba(52,211,153,0.8)] z-10 transition-transform group-hover:scale-125 pointer-events-none"
      title="接收来自通道的流量"
    />

    <!-- Line 1: Status Dot + Outbound Tag + Protocol Capsule + Idle Button / Hover Actions -->
    <div class="flex items-center justify-between gap-2">
      <!-- Left part -->
      <div class="flex items-center gap-1.5 min-w-0">
        <!-- Status Dot -->
        <span
          class="w-2 h-2 rounded-full shrink-0"
          :class="statusDotClass"
        />

        <!-- Outbound Tag -->
        <span class="font-bold text-xs sm:text-sm font-mono text-foreground truncate max-w-[110px]" :title="node.tag">
          {{ node.tag }}
        </span>

        <!-- Protocol Badge -->
        <span
          class="px-1.5 py-0.5 rounded text-[10px] font-mono font-bold uppercase shrink-0 border"
          :class="protocolBadgeColor(node.protocol)"
        >
          {{ node.protocol }}
        </span>
      </div>

      <!-- Right part: Idle Mount Button or Hover Actions -->
      <div class="flex items-center gap-1 shrink-0">
        <!-- Prominent Quick Mount Button if Idle -->
        <button
          v-if="node.metrics?.isIdle"
          type="button"
          @click.stop="$emit('publish', node.tag)"
          class="px-2 py-0.5 rounded bg-amber-500/20 hover:bg-amber-500/30 border border-amber-500/40 text-amber-300 font-semibold text-[10px] flex items-center gap-1 transition-colors shadow-sm"
          title="将闲置出站快速挂载为新线路"
        >
          <Zap class="w-3 h-3" />
          <span>⚡ 挂载</span>
        </button>

        <!-- Hover Actions if not idle -->
        <div v-else class="opacity-0 group-hover:opacity-100 transition-opacity flex items-center gap-0.5">
          <button
            type="button"
            @click.stop="$emit('edit', node.raw)"
            class="p-1 rounded text-muted-foreground hover:text-foreground hover:bg-muted/80 transition-colors"
            title="编辑出站设置"
          >
            <Pencil class="w-3.5 h-3.5" />
          </button>
          <button
            v-if="node.tag !== 'direct' && node.tag !== 'block' && node.tag !== 'blackhole'"
            type="button"
            @click.stop="$emit('delete', node.tag)"
            class="p-1 rounded text-muted-foreground hover:text-rose-400 hover:bg-muted/80 transition-colors"
            title="删除出站"
          >
            <Trash2 class="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
    </div>

    <!-- Line 2: Single-line Address Summary + Reference Count -->
    <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground mt-1.5 pt-1 border-t border-border/40">
      <span class="truncate max-w-[150px]" :title="formatSettingsSummary(node.raw)">
        {{ formatSettingsSummary(node.raw) }}
      </span>

      <span
        v-if="!node.metrics?.isIdle"
        class="text-[10px] text-muted-foreground shrink-0"
      >
        引用: {{ node.metrics?.referenceCount || 0 }}
      </span>
      <span
        v-else
        class="text-[10px] text-amber-400/90 font-medium shrink-0 flex items-center gap-0.5"
      >
        <span>未挂载</span>
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Zap, Pencil, Trash2 } from 'lucide-vue-next'
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
  (e: 'edit', ob: any): void
  (e: 'delete', tag: string): void
  (e: 'publish', tag: string): void
}>()

const statusDotClass = computed(() => {
  if (props.node.metrics?.isIdle) return 'bg-amber-400'
  if (props.node.status === 'blocked') return 'bg-rose-500'
  return 'bg-emerald-400'
})

const protocolBadgeColor = (p: string) => {
  switch ((p || '').toLowerCase()) {
    case 'vless':
      return 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30'
    case 'trojan':
      return 'bg-purple-500/15 text-purple-400 border-purple-500/30'
    case 'freedom':
      return 'bg-emerald-500/15 text-emerald-400 border-emerald-500/30'
    case 'wireguard':
      return 'bg-amber-500/15 text-amber-400 border-amber-500/30'
    case 'shadowsocks':
    case 'ss':
      return 'bg-blue-500/15 text-blue-400 border-blue-500/30'
    case 'blackhole':
      return 'bg-rose-500/15 text-rose-400 border-rose-500/30'
    default:
      return 'bg-muted text-muted-foreground border-border'
  }
}

const formatSettingsSummary = (ob: any) => {
  if (!ob) return ''
  if (ob.protocol === 'freedom') return '直连公网 (Freedom)'
  if (ob.protocol === 'blackhole') return '黑洞拦截 (Blackhole)'
  if (ob.protocol === 'wireguard') return 'Cloudflare WARP'
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
