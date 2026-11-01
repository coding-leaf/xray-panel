<template>
  <div
    :data-node-id="node.id"
    class="relative rounded-lg border bg-card p-4 transition-all duration-200 select-none group"
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
      class="absolute -right-1.5 top-1/2 -translate-y-1/2 w-3 h-3 rounded-full bg-cyan-400 border-2 border-background shadow-[0_0_8px_rgba(34,211,238,0.8)] z-10 transition-transform group-hover:scale-125"
      title="流量流向出口"
    />

    <!-- Header: Protocol, Status Lamp & Latency -->
    <div class="flex items-center justify-between gap-2 mb-2.5">
      <div class="flex items-center gap-1.5 min-w-0">
        <span
          class="px-2 py-0.5 rounded-md text-[10px] font-mono font-bold uppercase shrink-0"
          :class="protocolClass(node.protocol)"
        >
          {{ node.protocol }}
        </span>
        <span class="text-sm font-bold font-mono text-foreground truncate tracking-tight">
          {{ node.tag }}
        </span>
      </div>

      <div class="flex items-center gap-1.5 shrink-0">
        <!-- Latency -->
        <span class="text-[10px] font-mono text-muted-foreground/80">
          {{ node.latencyMs || 1 }}ms
        </span>
        <!-- Breathing Status Dot -->
        <span class="relative flex h-2 w-2">
          <span
            v-if="node.status === 'online'"
            class="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
          />
          <span
            class="relative inline-flex rounded-full h-2 w-2"
            :class="statusDotClass(node.status)"
          />
        </span>
      </div>
    </div>

    <!-- Body Info -->
    <div class="space-y-2 text-xs">
      <!-- Port Mapping -->
      <div class="flex items-center justify-between py-1 border-b border-border/50 text-[11px] font-mono">
        <span class="text-muted-foreground">端口映射</span>
        <span class="text-cyan-400 font-semibold">:{{ node.listenPort }} ➔ :{{ node.externalPort }}</span>
      </div>

      <!-- Network & Security Badges -->
      <div class="flex items-center justify-between py-1 border-b border-border/50 text-[11px]">
        <span class="text-muted-foreground">传输 / 安全</span>
        <div class="flex items-center gap-1 font-mono text-[10px]">
          <span class="px-1.5 py-0.2 rounded bg-muted text-muted-foreground uppercase border border-border/40">
            {{ streamNetwork }}
          </span>
          <span class="px-1.5 py-0.2 rounded bg-muted text-foreground uppercase border border-border/40 font-semibold">
            {{ securityType }}
          </span>
        </div>
      </div>

      <!-- Traffic & User Metrics -->
      <div class="flex items-center justify-between pt-0.5 text-[11px] font-mono text-muted-foreground">
        <div class="flex items-center gap-1">
          <Users class="w-3 h-3 text-muted-foreground/80" />
          <span>{{ node.metrics?.clientCount || 0 }} 用户</span>
        </div>
        <div class="flex items-center gap-1.5 text-[10px]">
          <span class="text-emerald-400/90">↑{{ formatBytes(node.metrics?.upBytes || 0) }}</span>
          <span class="text-cyan-400/90">↓{{ formatBytes(node.metrics?.downBytes || 0) }}</span>
        </div>
      </div>
    </div>

    <!-- Footer Actions -->
    <div class="mt-3 pt-2.5 border-t border-border/60 flex items-center justify-between text-[11px]">
      <div class="flex items-center gap-2">
        <button
          type="button"
          @click.stop="$emit('edit', node.raw)"
          class="text-foreground/80 hover:text-foreground font-medium transition-colors"
        >
          编辑
        </button>
        <button
          type="button"
          @click.stop="$emit('logs', node.tag)"
          class="text-muted-foreground hover:text-cyan-400 transition-colors flex items-center gap-0.5"
          title="查看访问日志"
        >
          <FileText class="w-3 h-3" />
          <span>日志</span>
        </button>
      </div>

      <button
        type="button"
        @click.stop="$emit('delete', node.inboundId || node.raw.id)"
        class="text-rose-400/90 hover:text-rose-300 transition-colors"
      >
        删除
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Users, FileText } from 'lucide-vue-next'
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

const streamNetwork = computed(() => {
  try {
    const s = JSON.parse(props.node.raw?.streamSettings || '{}')
    return s.network || 'tcp'
  } catch {
    return 'tcp'
  }
})

const securityType = computed(() => {
  try {
    const s = JSON.parse(props.node.raw?.streamSettings || '{}')
    return s.security || 'none'
  } catch {
    return 'none'
  }
})

const protocolClass = (p: string) => {
  switch ((p || '').toLowerCase()) {
    case 'vless':
      return 'bg-cyan-500/15 text-cyan-400 border border-cyan-500/30'
    case 'trojan':
      return 'bg-purple-500/15 text-purple-400 border border-purple-500/30'
    case 'shadowsocks':
    case 'ss':
      return 'bg-blue-500/15 text-blue-400 border border-blue-500/30'
    case 'vmess':
      return 'bg-indigo-500/15 text-indigo-400 border border-indigo-500/30'
    case 'socks':
      return 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30'
    default:
      return 'bg-muted text-muted-foreground border border-border'
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
