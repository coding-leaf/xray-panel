<template>
  <div class="space-y-6">
    <!-- Navigation Tabs -->
    <div class="flex items-center gap-2 border-b border-border pb-3 flex-nowrap overflow-x-auto">
      <button
        type="button"
        @click="activeTab = 'channels'"
        class="px-3.5 py-1.5 rounded-md text-xs font-semibold transition-all flex items-center gap-2 shrink-0"
        :class="
          activeTab === 'channels'
            ? 'bg-muted text-foreground border border-border'
            : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
        "
      >
        <Network class="w-3.5 h-3.5 text-cyan-400" />
        <span>分流通道全景 (Channels: {{ allChannels.length }})</span>
      </button>

      <button
        type="button"
        @click="activeTab = 'gateways'"
        class="px-3.5 py-1.5 rounded-md text-xs font-semibold transition-all flex items-center gap-2 shrink-0"
        :class="
          activeTab === 'gateways'
            ? 'bg-muted text-foreground border border-border'
            : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
        "
      >
        <Radio class="w-3.5 h-3.5 text-emerald-400" />
        <span>接入网关池 (Gateways: {{ inbounds.length }})</span>
      </button>

      <button
        type="button"
        @click="activeTab = 'exits'"
        class="px-3.5 py-1.5 rounded-md text-xs font-semibold transition-all flex items-center gap-2 shrink-0"
        :class="
          activeTab === 'exits'
            ? 'bg-muted text-foreground border border-border'
            : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
        "
      >
        <Send class="w-3.5 h-3.5 text-indigo-400" />
        <span>落地出口池 (Exit Nodes: {{ outbounds.length }})</span>
      </button>
    </div>

    <!-- TAB 1: 分流通道全景流水线 (Channels Pipeline) -->
    <div v-if="activeTab === 'channels'" class="space-y-4">
      <div
        v-if="allChannels.length > 0"
        class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4"
      >
        <div
          v-for="ch in allChannels"
          :key="ch.id"
          class="p-4 rounded-lg bg-card border border-border flex flex-col justify-between group hover:border-border/80 transition-colors"
        >
          <div>
            <!-- Channel Title & RouteID Badge -->
            <div class="flex items-center justify-between mb-3">
              <div class="flex items-center gap-2">
                <span class="text-sm font-bold text-foreground tracking-tight">{{ ch.name }}</span>
              </div>
              <span
                v-if="ch.routeId > 0"
                class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold bg-indigo-950/60 text-indigo-300 border border-indigo-800/50"
              >
                Route #{{ ch.routeId }}
              </span>
              <span
                v-else
                class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold bg-muted/60 text-muted-foreground border border-border"
              >
                直通
              </span>
            </div>

            <!-- Pipeline Flow Topology Card -->
            <div
              class="bg-background p-3 rounded-md border border-border space-y-2 text-xs font-mono"
            >
              <!-- Gateway Info -->
              <div class="flex items-center justify-between text-muted-foreground">
                <span class="text-muted-foreground/80">接入网关</span>
                <span class="text-cyan-400 font-medium"
                  >{{ ch.gatewayTag }} (:{{ ch.externalPort || ch.gatewayPort }})</span
                >
              </div>

              <!-- Flow Arrow -->
              <div class="flex items-center justify-center text-muted-foreground/60 text-[10px]">
                <span
                  >↓ Scoped VLESS 分流 (0x{{
                    (ch.routeId || 0).toString(16).padStart(4, '0')
                  }})</span
                >
              </div>

              <!-- Exit Node Info -->
              <div class="flex items-center justify-between text-muted-foreground">
                <span class="text-muted-foreground/80">落地出口</span>
                <span class="text-indigo-300 font-bold flex items-center gap-1">
                  <span>{{ ch.outboundTag }}</span>
                  <span
                    class="text-[10px] px-1 py-0.2 rounded bg-muted text-foreground font-normal uppercase"
                  >
                    {{ getOutboundProtocol(ch.outboundTag) }}
                  </span>
                </span>
              </div>
            </div>

            <!-- Status & Meta -->
            <div
              class="mt-3 flex items-center justify-between text-[11px] text-muted-foreground font-mono"
            >
              <span
                >状态:
                <span
                  :class="
                    ch.enabled
                      ? 'text-emerald-400 font-medium'
                      : 'text-muted-foreground line-through'
                  "
                  >{{ ch.enabled ? '已发布' : '已暂停' }}</span
                ></span
              >
              <span>单端口订阅就绪</span>
            </div>
          </div>

          <!-- Actions -->
          <div class="mt-4 pt-3 border-t border-border flex items-center justify-between text-xs">
            <button
              type="button"
              @click="$emit('toggleChannelStatus', ch)"
              class="text-muted-foreground hover:text-foreground transition-colors font-medium"
            >
              {{
                ch.routeId > 0
                  ? ch.enabled
                    ? '暂停线路'
                    : '启用线路'
                  : ch.enabled
                    ? '暂停直通'
                    : '启用直通'
              }}
            </button>
            <button
              type="button"
              @click="$emit('deleteChannel', ch)"
              class="text-rose-400 hover:text-rose-300 transition-colors font-medium"
            >
              {{ ch.routeId > 0 ? '删除线路' : '删除网关' }}
            </button>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else class="p-12 text-center rounded-lg bg-card border border-border">
        <Network class="w-12 h-12 mx-auto text-muted-foreground/40 mb-3" />
        <h3 class="text-sm font-semibold text-foreground">暂未发布任何分流线路</h3>
        <p class="text-xs text-muted-foreground mt-1">
          点击右上角“⚡ 1步发布落地线路”即可一键将落地节点发布为订阅线路
        </p>
      </div>
    </div>

    <!-- TAB 2: 接入网关池 (Gateways / Inbounds) -->
    <div v-if="activeTab === 'gateways'" class="space-y-4">
      <div class="flex items-center justify-between">
        <span class="text-xs text-muted-foreground"
          >已部署的入站接入网关节点（共 {{ inbounds.length }} 个）</span
        >
        <button
          type="button"
          @click="$emit('openCreateGateway')"
          class="px-3 py-1.5 bg-muted hover:bg-muted/80 text-foreground border border-border text-xs font-medium rounded-md transition-all flex items-center gap-1.5"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>新建接入网关</span>
        </button>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="inb in inbounds"
          :key="inb.id"
          class="p-4 rounded-lg bg-card border border-border flex flex-col justify-between hover:border-border/80 transition-colors"
        >
          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-foreground tracking-tight font-mono">{{
                inb.tag
              }}</span>
              <div class="flex items-center gap-1.5">
                <span
                  class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold uppercase bg-cyan-500/15 text-cyan-400 border border-cyan-500/30"
                >
                  {{ inb.protocol }}
                </span>
                <span
                  class="px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-muted text-muted-foreground border border-border"
                >
                  {{ getStreamNetwork(inb) }}
                </span>
              </div>
            </div>

            <div class="space-y-2 text-xs text-muted-foreground">
              <div class="flex justify-between py-1 border-b border-border/50">
                <span>端口映射 (内部 ➔ 外部)</span>
                <span class="text-cyan-400 font-mono font-bold"
                  >:{{ inb.port }} ➔ :{{ inb.externalPort || 443 }}</span
                >
              </div>
              <div class="flex justify-between py-1 border-b border-border/50">
                <span>安全协议 (Security)</span>
                <span class="text-foreground font-mono uppercase font-semibold">{{
                  getSecurityType(inb)
                }}</span>
              </div>
              <div class="flex justify-between py-1 border-b border-border/50">
                <span>挂载分流线路</span>
                <span class="text-indigo-300 font-mono font-bold"
                  >{{ (inb.subRoutes || []).length }} 条</span
                >
              </div>
              <div class="flex justify-between py-1">
                <span>端口连通性</span>
                <span
                  :class="inb.isAlive ? 'text-emerald-400' : 'text-rose-400'"
                  class="font-semibold flex items-center gap-1"
                >
                  <span
                    class="w-1.5 h-1.5 rounded-full"
                    :class="inb.isAlive ? 'bg-emerald-400' : 'bg-rose-400'"
                  ></span>
                  <span>{{ inb.isAlive ? `正常 (${inb.latencyMs || 1}ms)` : '未连通' }}</span>
                </span>
              </div>
            </div>
          </div>

          <div class="mt-4 pt-3 border-t border-border flex items-center justify-between">
            <button
              type="button"
              @click="$emit('editGateway', inb)"
              class="text-xs text-foreground/80 hover:text-foreground font-medium"
            >
              编辑网关参数
            </button>
            <button
              type="button"
              @click="$emit('deleteGateway', inb.id)"
              class="text-xs text-rose-400 hover:text-rose-300 font-medium"
            >
              删除网关
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- TAB 3: 落地出口池 (Exit Nodes / Outbounds) -->
    <div v-if="activeTab === 'exits'" class="space-y-4">
      <div class="flex items-center justify-between">
        <span class="text-xs text-muted-foreground"
          >已配置的落地出口节点（共 {{ outbounds.length }} 个）</span
        >
        <button
          type="button"
          @click="$emit('openCreateExit')"
          class="px-3 py-1.5 bg-muted hover:bg-muted/80 text-foreground border border-border text-xs font-medium rounded-md transition-all flex items-center gap-1.5"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>新建落地出口</span>
        </button>
      </div>
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="ob in outbounds"
          :key="ob.tag"
          class="p-4 rounded-lg bg-card border border-border flex flex-col justify-between hover:border-border/80 transition-colors"
        >
          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-sm font-bold text-foreground tracking-tight font-mono">{{
                ob.tag
              }}</span>
              <span
                class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold uppercase"
                :class="protocolBadgeColor(ob.protocol)"
              >
                {{ ob.protocol }}
              </span>
            </div>

            <div class="space-y-2 text-xs text-muted-foreground">
              <div class="flex justify-between py-1 border-b border-border/50">
                <span>出站协议</span>
                <span class="text-foreground font-mono font-semibold">{{ ob.protocol }}</span>
              </div>
              <div class="py-1">
                <span class="block mb-1 text-muted-foreground/80 text-[11px]">设置摘要:</span>
                <div
                  class="bg-background p-2 rounded-md border border-border text-[11px] font-mono text-muted-foreground truncate"
                >
                  {{ formatSettingsSummary(ob) }}
                </div>
              </div>
            </div>
          </div>

          <div class="mt-4 pt-3 border-t border-border flex items-center justify-between">
            <button
              type="button"
              @click="$emit('editOutbound', ob)"
              class="text-xs text-foreground/80 hover:text-foreground font-medium"
            >
              编辑设置
            </button>
            <button
              v-if="ob.tag !== 'direct' && ob.tag !== 'block'"
              type="button"
              @click="$emit('deleteOutbound', ob.tag)"
              class="text-xs text-rose-400 hover:text-rose-300 font-medium"
            >
              删除
            </button>
            <span v-else class="text-xs text-muted-foreground/60 font-mono">系统保留</span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { Plus, Radio, Send, Network } from 'lucide-vue-next'

interface Props {
  inbounds: any[]
  outbounds: any[]
  allChannels: any[]
}

const props = defineProps<Props>()

defineEmits<{
  (e: 'openWizard'): void
  (e: 'openCreateGateway'): void
  (e: 'openCreateExit'): void
  (e: 'toggleChannelStatus', ch: any): void
  (e: 'deleteChannel', ch: any): void
  (e: 'editGateway', inb: any): void
  (e: 'deleteGateway', id: number): void
  (e: 'editOutbound', ob: any): void
  (e: 'deleteOutbound', tag: string): void
}>()

const activeTab = ref<'channels' | 'gateways' | 'exits'>('channels')

const getOutboundProtocol = (tag: string) => {
  const ob = props.outbounds.find((o) => o.tag === tag)
  return ob ? ob.protocol : 'freedom'
}

const getStreamNetwork = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.network || 'tcp'
  } catch {
    return 'tcp'
  }
}

const getSecurityType = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.security || 'none'
  } catch {
    return 'none'
  }
}

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
    default:
      return 'bg-muted text-muted-foreground border border-border'
  }
}

const formatSettingsSummary = (ob: any) => {
  if (ob.protocol === 'freedom') return '直连公网 (Freedom)'
  if (ob.protocol === 'blackhole') return '黑洞拦截 (Blackhole)'
  if (ob.protocol === 'wireguard') return 'Cloudflare WARP (WireGuard)'
  return ob.settingsJson || '{}'
}
</script>
