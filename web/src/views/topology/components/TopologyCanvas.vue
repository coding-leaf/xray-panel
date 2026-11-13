<template>
  <div class="space-y-6">
    <!-- Top Statistics Bar & Legend -->
    <div
      class="flex flex-col md:flex-row items-start md:items-center justify-between gap-4 p-3.5 rounded-lg bg-card border border-border"
    >
      <!-- Status Summary Chips -->
      <div class="flex flex-wrap items-center gap-2 text-xs font-mono">
        <span
          class="px-2.5 py-1 rounded-md bg-muted text-foreground border border-border flex items-center gap-1.5"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-cyan-400"></span>
          <span
            >网关:
            <strong class="text-cyan-400 font-bold"
              >{{ activeGatewaysCount }}/{{ graph.inboundNodes.length }}</strong
            >
            在线</span
          >
        </span>

        <span
          class="px-2.5 py-1 rounded-md bg-muted text-foreground border border-border flex items-center gap-1.5"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-indigo-400"></span>
          <span
            >分流线路/规则:
            <strong class="text-indigo-300 font-bold">{{ graph.channelNodes.length }}</strong>
            条</span
          >
        </span>

        <span
          class="px-2.5 py-1 rounded-md bg-muted text-foreground border border-border flex items-center gap-1.5"
        >
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span>
          <span
            >落地出口:
            <strong class="text-emerald-400 font-bold">{{ graph.outboundNodes.length }}</strong>
            个</span
          >
          <span
            v-if="idleOutboundsCount > 0"
            class="ml-1 px-1.5 py-0.2 rounded bg-amber-500/20 text-amber-300 text-[10px]"
          >
            {{ idleOutboundsCount }} 闲置
          </span>
        </span>

        <!-- Global Rules Toggle Button -->
        <button
          type="button"
          @click="$emit('toggleGlobalRules')"
          class="px-2.5 py-1 rounded-md text-xs font-mono transition-colors flex items-center gap-1.5 border"
          :class="
            showGlobalRules
              ? 'bg-sky-500/15 text-sky-300 border-sky-500/35 font-semibold'
              : 'bg-muted text-muted-foreground border-border hover:text-foreground'
          "
          :title="showGlobalRules ? '收起全局分流规则' : '展开显示全部底层域名/IP分流规则'"
        >
          <Filter class="w-3 h-3" />
          <span>{{
            showGlobalRules
              ? '收起全局分流规则'
              : hiddenGlobalRulesCount && hiddenGlobalRulesCount > 0
                ? `显示全局分流规则 (${hiddenGlobalRulesCount}组)`
                : '显示全局分流规则'
          }}</span>
        </button>
      </div>

      <!-- Canvas Legend (Visual Guide) -->
      <div class="flex flex-wrap items-center gap-3 text-[11px] text-muted-foreground select-none">
        <span class="flex items-center gap-1">
          <span
            class="w-3 h-0.5 bg-cyan-400 rounded-full shadow-[0_0_6px_rgba(34,211,238,0.8)]"
          ></span>
          <span>脉冲虚线：活跃分流</span>
        </span>
        <span class="flex items-center gap-1">
          <span class="w-3 h-0.5 bg-indigo-400/80 rounded-full"></span>
          <span>规则直通</span>
        </span>
        <span class="flex items-center gap-1">
          <span class="w-3 h-0.5 bg-neutral-600 rounded-full"></span>
          <span>已暂停</span>
        </span>
        <span class="flex items-center gap-1">
          <span class="w-3 h-0.5 bg-rose-500 rounded-full"></span>
          <span>阻断拦截</span>
        </span>
      </div>
    </div>

    <!-- Desktop Three-Column Canvas with Animated SVG Layer -->
    <div
      ref="canvasRef"
      class="relative hidden lg:block rounded-xl border border-border bg-neutral-950/40 p-5 min-h-[360px]"
    >
      <!-- SVG Connecting Curves Layer -->
      <svg
        class="absolute inset-0 w-full h-full pointer-events-none z-0 overflow-visible"
        xmlns="http://www.w3.org/2000/svg"
      >
        <defs>
          <filter id="glow-pulse" x="-100%" y="-100%" width="300%" height="300%">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>

        <!-- Render All Computed Edges -->
        <g v-for="ce in computedEdges" :key="ce.edge.id">
          <!-- 1. Underlying Baseline Path (Soft clear 0.45 when inactive, 1.0 when active, 0.08 dimmed) -->
          <path
            :id="`edge-path-${ce.edge.id}`"
            :d="ce.pathD"
            fill="none"
            :stroke="getEdgeColor(ce)"
            stroke-dasharray="5 5"
            :stroke-width="ce.isActive ? 2.5 : 1.5"
            :opacity="ce.isDimmed ? 0.08 : ce.isActive ? 1 : 0.45"
            class="transition-all duration-200"
          />

          <!-- 2. Active Flowing Dash Layer (Moving dashed stroke) -->
          <path
            v-if="ce.status === 'active'"
            :d="ce.pathD"
            fill="none"
            :stroke="getEdgeColor(ce)"
            stroke-dasharray="6 14"
            :stroke-width="ce.isActive ? 2.8 : 1.8"
            :opacity="ce.isDimmed ? 0.05 : ce.isActive ? 1 : 0.65"
            :class="ce.isActive ? 'flowing-dash-fast' : 'flowing-dash'"
            class="transition-all duration-200"
          />

          <!-- 3. Glowing Pulse Particle (animateMotion dot) -->
          <circle
            v-if="ce.status === 'active' && !ce.isDimmed"
            :key="`${ce.edge.id}-${ce.pathD}-${ce.isActive}`"
            r="3"
            :fill="getEdgeColor(ce)"
            filter="url(#glow-pulse)"
            :opacity="ce.isActive ? 1 : 0.7"
          >
            <animateMotion
              :path="ce.pathD"
              :dur="ce.isActive ? '1.5s' : '2.6s'"
              repeatCount="indefinite"
            >
              <mpath :href="`#edge-path-${ce.edge.id}`" />
            </animateMotion>
          </circle>
        </g>
      </svg>

      <!-- 3-Column Nodes Grid (Centered vertically to prevent slanted noodle lines) -->
      <div class="grid grid-cols-3 gap-10 lg:gap-14 relative z-10 items-center">
        <!-- COLUMN 1: Inbound Gateways -->
        <div class="space-y-3">
          <div class="flex items-center justify-between pb-2 border-b border-border/80">
            <div class="flex items-center gap-2">
              <span class="w-2 h-2 rounded-full bg-cyan-400"></span>
              <h3 class="text-xs font-bold text-foreground uppercase tracking-wider font-mono">
                接入网关 (Gateways)
              </h3>
            </div>
            <button
              type="button"
              @click="$emit('openCreateGateway')"
              class="text-[11px] text-muted-foreground hover:text-foreground font-medium transition-colors"
            >
              + 新增
            </button>
          </div>

          <div class="space-y-2.5">
            <InboundNodeCard
              v-for="inb in graph.inboundNodes"
              :key="inb.id"
              :node="inb"
              :is-active="highlightState.activeNodeIds.has(inb.id)"
              :is-dimmed="isNodeDimmed(inb.id)"
              @hover="onNodeHover"
              @leave="onNodeLeave"
              @edit="$emit('editGateway', $event)"
              @delete="$emit('deleteGateway', $event)"
              @logs="$emit('logs', $event)"
            />
          </div>
        </div>

        <!-- COLUMN 2: Routing Pipeline & Channels -->
        <div class="space-y-3">
          <div class="flex items-center justify-between pb-2 border-b border-border/80">
            <div class="flex items-center gap-2">
              <span class="w-2 h-2 rounded-full bg-indigo-400"></span>
              <h3 class="text-xs font-bold text-foreground uppercase tracking-wider font-mono">
                分流通道与规则 (Pipeline)
              </h3>
            </div>
            <button
              type="button"
              @click="$emit('openWizard')"
              class="text-[11px] text-muted-foreground hover:text-foreground font-medium transition-colors"
            >
              + 发布线路
            </button>
          </div>

          <div class="space-y-2.5">
            <ChannelNodeCard
              v-for="ch in graph.channelNodes"
              :key="ch.id"
              :node="ch"
              :is-active="highlightState.activeNodeIds.has(ch.id)"
              :is-dimmed="isNodeDimmed(ch.id)"
              @hover="onNodeHover"
              @leave="onNodeLeave"
              @toggle-status="$emit('toggleChannelStatus', $event)"
              @delete="$emit('deleteChannel', $event)"
            />
          </div>
        </div>

        <!-- COLUMN 3: Exit Nodes -->
        <div class="space-y-3">
          <div class="flex items-center justify-between pb-2 border-b border-border/80">
            <div class="flex items-center gap-2">
              <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
              <h3 class="text-xs font-bold text-foreground uppercase tracking-wider font-mono">
                落地出口 (Exit Nodes)
              </h3>
            </div>
            <button
              type="button"
              @click="$emit('openCreateExit')"
              class="text-[11px] text-muted-foreground hover:text-foreground font-medium transition-colors"
            >
              + 新增
            </button>
          </div>

          <div class="space-y-2.5">
            <OutboundNodeCard
              v-for="ob in graph.outboundNodes"
              :key="ob.id"
              :node="ob"
              :is-active="highlightState.activeNodeIds.has(ob.id)"
              :is-dimmed="isNodeDimmed(ob.id)"
              @hover="onNodeHover"
              @leave="onNodeLeave"
              @edit="$emit('editOutbound', $event)"
              @delete="$emit('deleteOutbound', $event)"
              @publish="$emit('openWizard', $event)"
            />
          </div>
        </div>
      </div>
    </div>

    <!-- Mobile Responsive Stream Cards (< lg) -->
    <div class="block lg:hidden space-y-4">
      <div class="p-3 bg-muted/40 rounded-lg border border-border text-xs text-muted-foreground">
        小屏模式已自动优化为流式链路流水线，展示入站至落地节点的完整流向：
      </div>

      <div class="space-y-3">
        <div
          v-for="unit in streamPipelines"
          :key="unit.id"
          class="p-4 rounded-lg bg-card border border-border space-y-3"
        >
          <!-- Stream Header -->
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-sm font-bold text-foreground">{{ unit.name }}</span>
              <span
                v-if="unit.routeId && unit.routeId > 0"
                class="px-2 py-0.5 rounded text-[10px] font-mono bg-indigo-950/60 text-indigo-300 border border-indigo-800/40"
              >
                Route #{{ unit.routeId }}
              </span>
            </div>
            <span
              class="text-xs font-mono"
              :class="
                unit.status === 'active'
                  ? 'text-emerald-400'
                  : unit.status === 'blocked'
                    ? 'text-rose-400'
                    : 'text-neutral-500'
              "
            >
              {{
                unit.status === 'active'
                  ? '● 启用'
                  : unit.status === 'blocked'
                    ? '● 拦截'
                    : '○ 暂停'
              }}
            </span>
          </div>

          <!-- Flow Stream: Gateway -> Route -> Exit -->
          <div
            class="bg-background p-3 rounded-md border border-border text-xs font-mono space-y-2"
          >
            <div class="flex items-center justify-between">
              <span class="text-muted-foreground">接入网关:</span>
              <span class="text-cyan-400 font-semibold"
                >{{ unit.inbound.tag }} (:{{
                  unit.inbound.externalPort || unit.inbound.listenPort
                }})</span
              >
            </div>
            <div class="flex items-center justify-center text-neutral-600 text-[10px]">
              <span>↓ {{ unit.channel.title }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-muted-foreground">落地出口:</span>
              <span class="text-emerald-400 font-semibold"
                >{{ unit.outbound.tag }} ({{ unit.outbound.protocol }})</span
              >
            </div>
          </div>

          <!-- Quick Actions on mobile -->
          <div
            v-if="unit.channel.category === 'subroute'"
            class="pt-2 border-t border-border flex items-center justify-between text-xs"
          >
            <button
              type="button"
              @click="$emit('toggleChannelStatus', unit.channel)"
              class="text-muted-foreground hover:text-foreground font-medium"
            >
              {{ unit.channel.status === 'paused' ? '启用线路' : '暂停线路' }}
            </button>
            <button
              type="button"
              @click="$emit('deleteChannel', unit.channel)"
              class="text-rose-400 hover:text-rose-300 font-medium"
            >
              删除线路
            </button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Filter } from 'lucide-vue-next'
import type {
  TopologyGraph,
  TopologyNode,
  StreamPipelineUnit,
  HoverHighlightState,
  ComputedBezierEdge,
} from '../types'
import { useSvgBezier } from '../composables/useSvgBezier'
import InboundNodeCard from './InboundNodeCard.vue'
import ChannelNodeCard from './ChannelNodeCard.vue'
import OutboundNodeCard from './OutboundNodeCard.vue'

interface Props {
  graph: TopologyGraph
  streamPipelines: StreamPipelineUnit[]
  computeActiveSubgraph: (nodeId: string | null, edgeId: string | null) => HoverHighlightState
  showGlobalRules?: boolean
  hiddenGlobalRulesCount?: number
}

const props = defineProps<Props>()

defineEmits<{
  (e: 'openWizard', initialExitTag?: string): void
  (e: 'openCreateGateway'): void
  (e: 'openCreateExit'): void
  (e: 'toggleChannelStatus', node: TopologyNode): void
  (e: 'deleteChannel', node: TopologyNode): void
  (e: 'editGateway', inb: any): void
  (e: 'deleteGateway', id: number): void
  (e: 'logs', tag: string): void
  (e: 'editOutbound', ob: any): void
  (e: 'deleteOutbound', tag: string): void
  (e: 'toggleGlobalRules'): void
}>()

const canvasRef = ref<HTMLElement | null>(null)

const highlightState = ref<HoverHighlightState>({
  hoveredNodeId: null,
  hoveredEdgeId: null,
  activeNodeIds: new Set(),
  activeEdgeIds: new Set(),
})

const edgesRef = computed(() => props.graph.edges)

const { computedEdges } = useSvgBezier(canvasRef, edgesRef, highlightState)

const activeGatewaysCount = computed(() => {
  return props.graph.inboundNodes.filter((i) => i.status === 'online').length
})

const idleOutboundsCount = computed(() => {
  return props.graph.outboundNodes.filter((o) => o.metrics?.isIdle).length
})

const onNodeHover = (nodeId: string) => {
  highlightState.value = props.computeActiveSubgraph(nodeId, null)
}

const onNodeLeave = () => {
  highlightState.value = props.computeActiveSubgraph(null, null)
}

const isNodeDimmed = (nodeId: string) => {
  const hasActive =
    highlightState.value.activeNodeIds.size > 0 || highlightState.value.activeEdgeIds.size > 0
  if (!hasActive) return false
  return !highlightState.value.activeNodeIds.has(nodeId)
}

const getEdgeColor = (ce: ComputedBezierEdge) => {
  if (ce.status === 'paused') {
    return '#525252'
  }
  if (ce.status === 'blocked') {
    return '#f43f5e'
  }
  if (ce.category === 'subroute') {
    return '#38bdf8'
  }
  if (ce.category === 'scoped_rule') {
    return '#a78bfa'
  }
  if (ce.category === 'global_rule') {
    return '#38bdf8'
  }
  return '#34d399'
}
</script>

<style scoped>
@keyframes flowDash {
  to {
    stroke-dashoffset: -40;
  }
}

.flowing-dash {
  animation: flowDash 2.4s linear infinite;
}

.flowing-dash-fast {
  animation: flowDash 1.2s linear infinite;
}
</style>
