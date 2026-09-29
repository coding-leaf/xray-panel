<template>
  <Drawer
    v-model="isOpen"
    :title="`出站节点详情: ${outbound?.tag || ''}`"
    description="查看节点结构化参数、流控策略与底层 JSON 配置"
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <div v-if="outbound" class="space-y-5">
      <!-- Overview Card -->
      <div class="p-4 rounded-lg bg-card border border-border space-y-3">
        <div class="flex items-center justify-between">
          <div class="flex items-center gap-2">
            <span class="text-base font-bold font-mono text-foreground">{{ outbound.tag }}</span>
            <Badge :variant="getProtocolBadgeVariant(outbound.protocol)">
              {{ outbound.protocol?.toUpperCase() }}
            </Badge>
          </div>
          <span class="text-xs text-muted-foreground font-mono">
            {{ getUsageDesc(outbound) }}
          </span>
        </div>

        <div class="grid grid-cols-2 gap-2 text-xs font-mono pt-2 border-t border-border/60">
          <div>
            <span class="text-muted-foreground block text-[11px]">出站协议</span>
            <span class="text-foreground font-semibold">{{ outbound.protocol }}</span>
          </div>
          <div>
            <span class="text-muted-foreground block text-[11px]">目标端点</span>
            <span class="text-foreground font-semibold">{{ getTargetEndpoint(outbound) }}</span>
          </div>
        </div>
      </div>

      <!-- WireGuard / WARP details if wireguard -->
      <div v-if="outbound.protocol === 'wireguard'" class="p-4 rounded-lg bg-card border border-border space-y-3">
        <h4 class="text-xs font-semibold text-cyan-400 font-mono flex items-center gap-1.5">
          <ShieldCheck class="w-3.5 h-3.5" />
          <span>WireGuard / Cloudflare WARP 参数</span>
        </h4>
        <div class="space-y-2 text-xs font-mono">
          <div class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">Endpoint (对端服务器)</span>
            <span class="text-foreground">{{ getParsedSettings(outbound).peers?.[0]?.endpoint || '-' }}</span>
          </div>
          <div class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">Address (本地隧道分配IP)</span>
            <span class="text-foreground">{{ (getParsedSettings(outbound).address || []).join(', ') || '-' }}</span>
          </div>
          <div class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">Peer Public Key</span>
            <span class="text-foreground truncate max-w-[280px]" :title="getParsedSettings(outbound).peers?.[0]?.publicKey">
              {{ getParsedSettings(outbound).peers?.[0]?.publicKey || '-' }}
            </span>
          </div>
        </div>
      </div>

      <!-- Stream Security / TLS / Reality details -->
      <div v-if="getOutboundStreamInfo(outbound).network" class="p-4 rounded-lg bg-card border border-border space-y-3">
        <h4 class="text-xs font-semibold text-foreground font-mono flex items-center gap-1.5">
          <Layers class="w-3.5 h-3.5 text-brand-400" />
          <span>传输与安全层 (StreamSettings)</span>
        </h4>
        <div class="space-y-2 text-xs font-mono">
          <div class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">传输协议 (Network)</span>
            <span class="text-foreground">{{ getOutboundStreamInfo(outbound).network?.toUpperCase() }}</span>
          </div>
          <div class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">安全协议 (Security)</span>
            <span class="text-foreground">{{ getOutboundStreamInfo(outbound).security?.toUpperCase() || 'NONE' }}</span>
          </div>
          <div v-if="getOutboundStreamInfo(outbound).realitySettings?.serverName" class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">REALITY SNI 伪装</span>
            <span class="text-cyan-400 font-semibold">{{ getOutboundStreamInfo(outbound).realitySettings?.serverName }}</span>
          </div>
          <div v-if="getOutboundStreamInfo(outbound).realitySettings?.publicKey" class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">REALITY Public Key</span>
            <span class="text-foreground truncate max-w-[280px]">{{ getOutboundStreamInfo(outbound).realitySettings?.publicKey }}</span>
          </div>
          <div v-if="getOutboundStreamInfo(outbound).tlsSettings?.serverName" class="flex justify-between py-1 border-b border-border/40">
            <span class="text-muted-foreground">TLS ServerName</span>
            <span class="text-foreground">{{ getOutboundStreamInfo(outbound).tlsSettings?.serverName }}</span>
          </div>
        </div>
      </div>

      <!-- Raw Settings JSON -->
      <div class="p-4 rounded-lg bg-card border border-border space-y-2">
        <div class="flex items-center justify-between">
          <h4 class="text-xs font-semibold text-muted-foreground font-mono">底盘配置 JSON (settingsJson)</h4>
          <Button variant="ghost" size="sm" class="h-6 px-2 text-[11px]" @click="copyText(outbound.settingsJson || '')">
            <Copy class="w-3 h-3 mr-1" />
            <span>复制</span>
          </Button>
        </div>
        <pre class="bg-neutral-950 p-3 rounded border border-border text-[11px] font-mono text-muted-foreground overflow-x-auto max-h-48 leading-relaxed">{{ formatJsonPretty(outbound.settingsJson) }}</pre>
      </div>

      <!-- Raw StreamSettings JSON if exists -->
      <div v-if="outbound.streamSettings" class="p-4 rounded-lg bg-card border border-border space-y-2">
        <div class="flex items-center justify-between">
          <h4 class="text-xs font-semibold text-muted-foreground font-mono">传输配置 JSON (streamSettings)</h4>
          <Button variant="ghost" size="sm" class="h-6 px-2 text-[11px]" @click="copyText(outbound.streamSettings || '')">
            <Copy class="w-3 h-3 mr-1" />
            <span>复制</span>
          </Button>
        </div>
        <pre class="bg-neutral-950 p-3 rounded border border-border text-[11px] font-mono text-muted-foreground overflow-x-auto max-h-48 leading-relaxed">{{ formatJsonPretty(outbound.streamSettings) }}</pre>
      </div>
    </div>

    <template #footer>
      <Button variant="secondary" size="sm" @click="isOpen = false">
        关闭
      </Button>
      <Button
        v-if="outbound && outbound.tag !== 'direct' && outbound.tag !== 'block'"
        variant="destructive"
        size="sm"
        @click="emit('delete', outbound.tag)"
      >
        <Trash2 class="w-3.5 h-3.5 mr-1" />
        <span>删除节点</span>
      </Button>
      <Button
        v-if="outbound"
        variant="default"
        size="sm"
        @click="emit('edit', outbound)"
      >
        <Edit3 class="w-3.5 h-3.5 mr-1" />
        <span>编辑配置</span>
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { ShieldCheck, Layers, Copy, Trash2, Edit3 } from 'lucide-vue-next'
import Drawer from '../../../components/ui/Drawer.vue'
import Button from '../../../components/ui/Button.vue'
import Badge from '../../../components/ui/Badge.vue'
import { copyText } from '../../../utils/clipboard'
import type { OutboundItem } from '../types'
import {
  getProtocolBadgeVariant,
  getUsageDesc,
  getTargetEndpoint,
  getParsedSettings,
  getOutboundStreamInfo,
  formatJsonPretty,
} from '../types'

const props = defineProps<{
  modelValue: boolean
  outbound: OutboundItem | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'edit', ob: OutboundItem): void
  (e: 'delete', tag: string): void
}>()

const isOpen = computed({
  get: () => props.modelValue,
  set: (val: boolean) => emit('update:modelValue', val),
})
</script>
