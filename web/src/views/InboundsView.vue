<template>
  <div class="space-y-4">
    <!-- Top Action & Filter Header -->
    <div
      class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60"
    >
      <div>
        <div class="flex items-center gap-2 flex-wrap">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">入站网关 (Inbounds)</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ filteredInbounds.length }} 个节点
          </Badge>
          <span
            v-if="nodeFilter === 'all'"
            class="inline-flex items-center gap-1.5 px-2 py-0.5 rounded text-[10px] font-mono bg-neutral-900 text-muted-foreground border border-border"
          >
            <span>全局筛选：全部节点</span>
            <button
              type="button"
              @click="restoreActiveNodeFilter"
              class="text-sky-400 hover:text-sky-300 underline"
            >
              切回当前节点
            </button>
          </span>
          <span
            v-else-if="isRemoteNode"
            class="px-2 py-0.5 rounded text-[10px] font-mono bg-sky-500/10 text-sky-300 border border-sky-500/30"
          >
            远程节点：{{ activeNode?.name || `#${activeNodeId}` }}
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          管理 Xray 入站代理节点，支持 VLESS REALITY 伪装巡检与单端口多出口分流
        </p>
      </div>

      <!-- Action Buttons -->
      <div class="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          :loading="checkingReality"
          @click="triggerRealityCheck"
          title="巡检所有 REALITY 伪装域名证书与可达性"
        >
          <ShieldCheck class="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
          <span>{{ checkingReality ? '检测中...' : '检测 REALITY' }}</span>
        </Button>
        <Button variant="default" size="sm" @click="openCreateDrawer">
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加新入站</span>
        </Button>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
      <div class="flex flex-1 flex-wrap sm:flex-nowrap items-center gap-2 max-w-2xl">
        <!-- Search Input -->
        <div class="relative w-full">
          <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-muted-foreground" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="搜索节点 Tag / 端口 / 协议 / 域名..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <!-- Node Filter -->
        <select
          v-model="nodeFilter"
          @change="onNodeFilterChange"
          aria-label="按节点筛选入站"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部节点</option>
          <option value="0">本机 Master</option>
          <option v-for="n in nodesList" :key="n.id" :value="String(n.id)">
            {{ n.name }}{{ n.region ? ` (${n.region})` : '' }}
          </option>
        </select>

        <!-- Protocol Filter -->
        <select
          v-model="protocolFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部协议</option>
          <option value="vless">VLESS</option>
          <option value="vmess">VMess</option>
          <option value="trojan">Trojan</option>
          <option value="shadowsocks">Shadowsocks</option>
          <option value="socks">Socks</option>
          <option value="http">HTTP</option>
          <option value="dokodemo-door">dokodemo-door</option>
        </select>
      </div>

      <!-- Reality Alert Summary Banner if any warning/error -->
      <div
        v-if="realityAlertCount > 0"
        class="flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-amber-500/10 border border-amber-500/20 text-amber-300 text-xs font-mono"
      >
        <AlertTriangle class="w-3.5 h-3.5 text-amber-400 shrink-0" />
        <span>发现 {{ realityAlertCount }} 个 REALITY 目标需关注</span>
      </div>
    </div>

    <!-- Main Table View -->
    <InboundTable
      :inbounds="filteredInbounds"
      :selected-inbound="selectedInbound"
      :users-list="usersList"
      :node-map="nodeMap"
      :reality-summary="realitySummary"
      @inspect="inspectInbound"
      @edit="editInbound"
      @delete="handleDelete"
      @logs="viewLogs"
    />

    <!-- Inspector Drawer -->
    <InboundDetailDrawer
      v-model="showInspectorDrawer"
      :inbound="selectedInbound"
      :users-list="usersList"
      :reality-summary="realitySummary"
      :checking-reality="checkingReality"
      @edit="editInbound"
      @delete="handleDelete"
      @check-reality="triggerRealityCheck"
      @logs="viewLogs"
    />

    <!-- Create / Edit Inbound Drawer (Lazy Loaded) -->
    <InboundFormDrawer
      v-if="showFormDrawer"
      v-model="showFormDrawer"
      :is-editing="isEditing"
      :initial-data="selectedInbound"
      :default-node-id="activeNodeId"
      :users-list="usersList"
      :available-outbounds="availableOutbounds"
      :nodes-list="nodesList"
      @saved="onSaved"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, defineAsyncComponent } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, AlertTriangle, ShieldCheck, Search } from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { toast } from '../utils/toast'
import InboundTable from './inbounds/components/InboundTable.vue'
import InboundDetailDrawer from './inbounds/components/InboundDetailDrawer.vue'
import { useInboundList } from './inbounds/composables/useInboundList'
import type { InboundItem } from './inbounds/types'

const InboundFormDrawer = defineAsyncComponent(
  () => import('./inbounds/components/InboundFormDrawer.vue')
)

const route = useRoute()
const router = useRouter()
const showInspectorDrawer = ref(false)
const showFormDrawer = ref(false)
const selectedInbound = ref<InboundItem | null>(null)
const isEditing = ref(false)

const {
  activeNodeId,
  activeNode,
  isRemoteNode,
  setActiveNode,
  inbounds,
  usersList,
  nodesList,
  nodeMap,
  availableOutbounds,
  checkingReality,
  realitySummary,
  searchQuery,
  protocolFilter,
  nodeFilter,
  filteredInbounds,
  realityAlertCount,
  fetchAll,
  triggerRealityCheck,
  deleteInbound,
} = useInboundList()

const onNodeFilterChange = () => {
  if (nodeFilter.value !== 'all') {
    setActiveNode(Number(nodeFilter.value))
  }
}

const restoreActiveNodeFilter = () => {
  nodeFilter.value = String(activeNodeId.value)
}

const inspectInbound = (inb: InboundItem) => {
  selectedInbound.value = inb
  showInspectorDrawer.value = true
}

const viewLogs = (inb: InboundItem) => {
  setActiveNode(Number(inb.nodeId || 0))
  router.push({
    path: '/logs',
    query: {
      type: 'access',
      inbound: inb.tag,
    },
  })
}

const openCreateDrawer = () => {
  isEditing.value = false
  selectedInbound.value = null
  showInspectorDrawer.value = false
  showFormDrawer.value = true
}

const editInbound = (inb: InboundItem) => {
  selectedInbound.value = inb
  isEditing.value = true
  showInspectorDrawer.value = false
  showFormDrawer.value = true
}

const handleDelete = async (id: number) => {
  const success = await deleteInbound(id)
  if (success && selectedInbound.value?.id === id) {
    showInspectorDrawer.value = false
    selectedInbound.value = null
  }
}

const onSaved = async () => {
  toast.success('节点配置已保存并重载核心')
  await fetchAll()
  if (selectedInbound.value) {
    selectedInbound.value = inbounds.value.find((i) => i.id === selectedInbound.value?.id) || null
  }
}

const checkRouteQuery = () => {
  const editQuery = route.query.edit as string
  if (editQuery && inbounds.value.length > 0) {
    const target = inbounds.value.find(
      (ib) => ib.tag === editQuery || String(ib.id) === String(editQuery)
    )
    if (target) {
      editInbound(target)
    } else {
      toast.warning('未找到指定的入站节点：' + editQuery)
    }
  } else if (route.query.action === 'create' || route.query.create) {
    openCreateDrawer()
  }
}

onMounted(async () => {
  await fetchAll()
  checkRouteQuery()
})

watch(
  () => [route.query.edit, route.query.action, route.query.create],
  () => {
    checkRouteQuery()
  }
)

watch(activeNodeId, () => {
  showInspectorDrawer.value = false
  selectedInbound.value = null
})
</script>
