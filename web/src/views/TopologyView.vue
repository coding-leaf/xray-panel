<template>
  <div class="space-y-6">
    <!-- Header 区域 -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-foreground tracking-tight">线路与拓扑中心 (Topology)</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-indigo-500/15 text-indigo-400 border border-indigo-500/30 flex items-center gap-1">
            <span class="w-1.5 h-1.5 rounded-full bg-indigo-400 animate-pulse"></span>
            <span>强类型编译器实时拓扑</span>
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">一体化接入网关、分流通道与落地出口，全协议路由流向可视化与 1 步发布</p>
      </div>

      <!-- Action Buttons & View Mode Switcher -->
      <div class="flex flex-wrap items-center gap-2.5">
        <!-- View Switcher (Segmented Control) -->
        <div class="flex items-center p-0.5 bg-muted rounded-md border border-border">
          <button
            type="button"
            @click="viewMode = 'canvas'"
            class="px-3 py-1.5 rounded-[4px] text-xs font-medium transition-all flex items-center gap-1.5"
            :class="viewMode === 'canvas' ? 'bg-background text-foreground shadow-sm font-semibold' : 'text-muted-foreground hover:text-foreground'"
          >
            <Workflow class="w-3.5 h-3.5 text-cyan-400" />
            <span>拓扑画布</span>
          </button>
          <button
            type="button"
            @click="viewMode = 'pools'"
            class="px-3 py-1.5 rounded-[4px] text-xs font-medium transition-all flex items-center gap-1.5"
            :class="viewMode === 'pools' ? 'bg-background text-foreground shadow-sm font-semibold' : 'text-muted-foreground hover:text-foreground'"
          >
            <LayoutGrid class="w-3.5 h-3.5 text-indigo-400" />
            <span>分组卡片池</span>
          </button>
        </div>

        <button
          @click="openWizard()"
          class="px-3.5 py-1.5 bg-foreground text-background hover:bg-foreground/90 text-xs font-medium rounded-md transition-colors flex items-center gap-1.5 shadow-sm"
        >
          <Zap class="w-3.5 h-3.5 text-amber-400" />
          <span>1步发布落地线路</span>
        </button>

        <button
          @click="openCreateGatewayModal"
          class="px-3 py-1.5 bg-muted text-foreground hover:bg-muted/80 border border-border text-xs font-medium rounded-md transition-colors flex items-center gap-1.5"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>新建接入网关</span>
        </button>

        <button
          @click="openCreateExitModal"
          class="px-3 py-1.5 bg-muted text-foreground hover:bg-muted/80 border border-border text-xs font-medium rounded-md transition-colors flex items-center gap-1.5"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>新建落地出口</span>
        </button>
      </div>
    </div>

    <!-- VIEW 1: 动态脉冲连线画布 (Topology Canvas) -->
    <TopologyCanvas
      v-if="viewMode === 'canvas'"
      :graph="graph"
      :stream-pipelines="streamPipelines"
      :compute-active-subgraph="computeActiveSubgraph"
      :show-global-rules="showGlobalRules"
      :hidden-global-rules-count="hiddenGlobalGroupsCount"
      @toggle-global-rules="showGlobalRules = !showGlobalRules"
      @open-wizard="openWizard"
      @open-create-gateway="openCreateGatewayModal"
      @open-create-exit="openCreateExitModal"
      @toggle-channel-status="toggleChannelStatus"
      @delete-channel="deleteChannel"
      @edit-gateway="editGateway"
      @delete-gateway="deleteGateway"
      @logs="viewLogs"
      @edit-outbound="editOutbound"
      @delete-outbound="deleteOutbound"
    />

    <!-- VIEW 2: 传统分组池视图 (Topology Pools View) -->
    <TopologyPoolsView
      v-else
      :inbounds="inbounds"
      :outbounds="outbounds"
      :all-channels="allChannels"
      @open-wizard="openWizard"
      @open-create-gateway="openCreateGatewayModal"
      @open-create-exit="openCreateExitModal"
      @toggle-channel-status="toggleChannelStatus"
      @delete-channel="deleteChannel"
      @edit-gateway="editGateway"
      @delete-gateway="deleteGateway"
      @edit-outbound="editOutbound"
      @delete-outbound="deleteOutbound"
    />

    <!-- ⚡ 1步发布落地线路向导弹窗 -->
    <QuickPublishWizardModal
      v-model="showWizardModal"
      :inbounds="inbounds"
      :outbounds="outbounds"
      :next-route-id="nextRouteId"
      :initial-exit-tag="wizardInitialExitTag"
      @success="fetchAll"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Zap, Workflow, LayoutGrid } from 'lucide-vue-next'
import api from '../api'
import { toast } from '../utils/toast'
import { useTopologyGraph } from './topology/composables/useTopologyGraph'
import TopologyCanvas from './topology/components/TopologyCanvas.vue'
import TopologyPoolsView from './topology/components/TopologyPoolsView.vue'
import QuickPublishWizardModal from './topology/components/QuickPublishWizardModal.vue'

const router = useRouter()
const viewMode = ref<'canvas' | 'pools'>('canvas')

const inbounds = ref<any[]>([])
const outbounds = ref<any[]>([])
const routing = ref<any>({ rules: [] })
const usersList = ref<any[]>([])
const showGlobalRules = ref(false)

// Topology Graph Derivation Engine
const {
  graph,
  streamPipelines,
  computeActiveSubgraph,
  hiddenGlobalGroupsCount,
} = useTopologyGraph(
  inbounds,
  outbounds,
  routing,
  usersList,
  showGlobalRules,
)

// Aggregate channels for traditional pools view and nextRouteId
const allChannels = computed(() => {
  const list: any[] = []
  for (const inb of inbounds.value) {
    const srs = inb.subRoutes || []
    if (srs.length > 0) {
      for (const sr of srs) {
        list.push({
          id: `${inb.tag}_${sr.routeId}`,
          name: sr.name || sr.remark || `线路 #${sr.routeId}`,
          gatewayTag: inb.tag,
          gatewayPort: inb.port,
          externalPort: inb.externalPort,
          routeId: sr.routeId,
          outboundTag: sr.outboundTag || 'direct',
          enabled: sr.enabled !== false,
          inboundId: inb.id,
        })
      }
    } else {
      list.push({
        id: `${inb.tag}_direct`,
        name: inb.remark || inb.tag,
        gatewayTag: inb.tag,
        gatewayPort: inb.port,
        externalPort: inb.externalPort,
        routeId: inb.routeId || 0,
        outboundTag: 'direct',
        enabled: inb.enabled,
        inboundId: inb.id,
      })
    }
  }
  return list
})

const nextRouteId = computed(() => {
  const used = new Set(allChannels.value.map((c) => Number(c.routeId) || 0))
  let id = 1
  while (used.has(id)) {
    id++
  }
  return id
})

// Wizard Modal State
const showWizardModal = ref(false)
const wizardInitialExitTag = ref('')

const openWizard = (initialExitTag?: string) => {
  wizardInitialExitTag.value = initialExitTag || ''
  showWizardModal.value = true
}

const openCreateGatewayModal = () => {
  router.push({ path: '/inbounds', query: { action: 'create' } })
}

const openCreateExitModal = () => {
  router.push({ path: '/outbounds', query: { action: 'create' } })
}

const editGateway = (inb: any) => {
  router.push({ path: '/inbounds', query: { edit: inb.tag || inb.id } })
}

const deleteGateway = async (id: number) => {
  if (!confirm('确定删除该网关吗？')) return
  try {
    await api.delete(`/inbounds/${id}`)
    toast.success('网关已删除')
    await fetchAll()
  } catch (e) {
    toast.error('删除网关失败')
  }
}

const viewLogs = (tag: string) => {
  router.push({ path: '/logs', query: { type: 'access', inbound: tag } })
}

const editOutbound = (ob: any) => {
  router.push({ path: '/outbounds', query: { edit: ob.tag } })
}

const deleteOutbound = async (tag: string) => {
  if (!confirm(`确定删除出站 ${tag} 吗？`)) return
  try {
    await api.delete(`/outbounds/${tag}`)
    toast.success('出站已删除')
    await fetchAll()
  } catch (e) {
    toast.error('删除出站失败')
  }
}

const toggleChannelStatus = async (item: any) => {
  try {
    const boundRoutes = item.raw?.boundRoutes
    if (boundRoutes && Array.isArray(boundRoutes) && boundRoutes.length > 0) {
      // Aggregated SubRoute: toggle all bound routes across inbounds
      const newEnabled = item.status === 'paused'
      const routeId = item.routeId ?? item.raw?.routeId
      const promises = []

      for (const bound of boundRoutes) {
        const targetGateway = inbounds.value.find(
          (i) => i.id === bound.inboundId || i.tag === bound.gatewayTag,
        )
        if (targetGateway) {
          const srs = targetGateway.subRoutes || []
          for (const sr of srs) {
            if (sr.routeId === (routeId ?? bound.subRoute?.routeId)) {
              sr.enabled = newEnabled
            }
          }
          promises.push(
            api.put(`/inbounds/${targetGateway.id}`, {
              ...targetGateway,
              subRoutesJson: JSON.stringify(srs),
            }),
          )
        }
      }
      await Promise.all(promises)
      toast.success(newEnabled ? '线路已同步启用' : '线路已同步暂停')
      await fetchAll()
      return
    }

    // Single SubRoute or Fallback
    const routeId = item.routeId ?? item.raw?.routeId
    const inboundId = item.inboundId ?? item.raw?.inboundId
    const gatewayTag = item.gatewayTag ?? item.raw?.gatewayTag

    const targetGateway = inbounds.value.find((i) => i.id === inboundId || i.tag === gatewayTag)
    if (!targetGateway) return

    if (routeId && routeId > 0) {
      const srs = targetGateway.subRoutes || []
      for (const sr of srs) {
        if (sr.routeId === routeId) {
          const currentEnabled = sr.enabled !== false
          sr.enabled = !currentEnabled
        }
      }
      await api.put(`/inbounds/${targetGateway.id}`, {
        ...targetGateway,
        subRoutesJson: JSON.stringify(srs),
      })
    } else {
      const currentEnabled = targetGateway.enabled !== false
      await api.put(`/inbounds/${targetGateway.id}`, {
        ...targetGateway,
        enabled: !currentEnabled,
      })
    }
    toast.success('状态已更新')
    await fetchAll()
  } catch (err) {
    toast.error('更新状态失败')
  }
}

const deleteChannel = async (item: any) => {
  const isFallback = item.category === 'fallback' || !item.routeId || item.routeId <= 0
  const name = item.name || item.title || item.raw?.name || '线路'
  const boundRoutes = item.raw?.boundRoutes
  const isAggregated = boundRoutes && Array.isArray(boundRoutes) && boundRoutes.length > 0

  const confirmMsg = isFallback
    ? `确定删除接入网关 "${item.gatewayTag || name}" 及其直通链路吗？`
    : isAggregated
      ? `确定删除聚合分流线路 "${name}" 吗？将从所有关联入口中同步移除。`
      : `确定删除分流线路 "${name}" 吗？`
  if (!confirm(confirmMsg)) return

  try {
    if (isAggregated) {
      const routeId = item.routeId ?? item.raw?.routeId
      const promises = []

      for (const bound of boundRoutes) {
        const targetGateway = inbounds.value.find(
          (i) => i.id === bound.inboundId || i.tag === bound.gatewayTag,
        )
        if (targetGateway) {
          const srs = (targetGateway.subRoutes || []).filter(
            (s: any) => s.routeId !== (routeId ?? bound.subRoute?.routeId),
          )
          promises.push(
            api.put(`/inbounds/${targetGateway.id}`, {
              ...targetGateway,
              subRoutesJson: JSON.stringify(srs),
            }),
          )
        }
      }
      await Promise.all(promises)
      toast.success('聚合线路已从所有入口同步删除')
      await fetchAll()
      return
    }

    const routeId = item.routeId ?? item.raw?.routeId
    const inboundId = item.inboundId ?? item.raw?.inboundId
    const gatewayTag = item.gatewayTag ?? item.raw?.gatewayTag

    const targetGateway = inbounds.value.find((i) => i.id === inboundId || i.tag === gatewayTag)
    if (!targetGateway) return

    if (routeId && routeId > 0) {
      const srs = (targetGateway.subRoutes || []).filter((s: any) => s.routeId !== routeId)
      await api.put(`/inbounds/${targetGateway.id}`, {
        ...targetGateway,
        subRoutesJson: JSON.stringify(srs),
      })
      toast.success('线路已删除')
    } else {
      await api.delete(`/inbounds/${targetGateway.id}`)
      toast.success('网关及线路已删除')
    }
    await fetchAll()
  } catch (err) {
    toast.error('删除线路失败')
  }
}

const fetchAll = async () => {
  try {
    const [inbRes, obRes, routeRes, uRes]: any = await Promise.all([
      api.get('/inbounds'),
      api.get('/outbounds').catch(() => []),
      api.get('/routing').catch(() => ({ rules: [] })),
      api.get('/users').catch(() => []),
    ])

    inbounds.value = (inbRes || []).map((ib: any) => {
      let srs: any[] = []
      try {
        srs = JSON.parse(ib.subRoutesJson || '[]')
      } catch (e) {}
      return { ...ib, subRoutes: srs }
    })
    outbounds.value = obRes || []
    routing.value = routeRes || { rules: [] }
    usersList.value = uRes || []
  } catch (err) {
    console.error(err)
  }
}

onMounted(() => {
  fetchAll()
})
</script>
