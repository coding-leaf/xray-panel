<template>
  <div class="space-y-4">
    <!-- Remote Node Context Banner -->
    <div
      v-if="isRemoteNode"
      class="px-3 py-2 rounded-lg bg-sky-500/10 border border-sky-500/30 text-sky-200 text-xs font-mono flex items-center justify-between gap-2"
    >
      <div class="flex items-center gap-2">
        <Server class="w-3.5 h-3.5 text-sky-400 shrink-0" />
        <span>
          当前托管远端节点：<strong>{{ activeNode?.name || `#${activeNodeId}` }}</strong>
          <span v-if="activeNode?.region" class="text-sky-300/80"> ({{ activeNode.region }})</span>
        </span>
      </div>
      <span class="text-[11px] text-sky-300/80 hidden sm:inline">保存后自动编译并同步至该节点</span>
    </div>

    <!-- Main Outbound Table -->
    <OutboundTable
      :outbounds="outbounds"
      :selected-tag="selectedOutbound?.tag"
      @inspect="openInspectDrawer"
      @edit="openEditDrawer"
      @delete="deleteOutbound"
      @create="openCreateDrawer"
    />

    <!-- Outbound Detail Drawer (Inspector) -->
    <OutboundDetailDrawer
      v-model="showInspectorDrawer"
      :outbound="selectedOutbound"
      @edit="openEditDrawer"
      @delete="deleteOutbound"
    />

    <!-- Outbound Form Drawer (Async Lazy Loaded) -->
    <OutboundFormDrawer
      v-if="showFormDrawer"
      v-model="showFormDrawer"
      :editing-outbound="editingOutbound"
      :outbounds="outbounds"
      :saving="saving"
      @save="saveOutbound"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, defineAsyncComponent } from 'vue'
import { useRoute } from 'vue-router'
import { Server } from 'lucide-vue-next'
import { toast } from '../utils/toast'
import api from '../api'
import { useActiveNode } from '../composables/useActiveNode'
import type { OutboundItem, OutboundPayload } from './outbounds/types'
import OutboundTable from './outbounds/components/OutboundTable.vue'
import OutboundDetailDrawer from './outbounds/components/OutboundDetailDrawer.vue'

const OutboundFormDrawer = defineAsyncComponent(
  () => import('./outbounds/components/OutboundFormDrawer.vue')
)

const route = useRoute()
const { activeNodeId, activeNode, isRemoteNode } = useActiveNode()
const outbounds = ref<OutboundItem[]>([])
const showInspectorDrawer = ref(false)
const showFormDrawer = ref(false)
const selectedOutbound = ref<OutboundItem | null>(null)
const editingOutbound = ref<OutboundItem | null>(null)
const saving = ref(false)
let fetchSeq = 0

const fetchOutbounds = async () => {
  const seq = ++fetchSeq
  const nodeId = activeNodeId.value
  try {
    const res = await api.get(`/outbounds?nodeId=${nodeId}`)
    if (seq !== fetchSeq) return
    outbounds.value = Array.isArray(res) ? res : []
    if (selectedOutbound.value) {
      const updated = outbounds.value.find((o) => o.tag === selectedOutbound.value?.tag)
      if (updated) selectedOutbound.value = updated
    }
  } catch (err) {
    console.error(err)
  }
}

const openInspectDrawer = (ob: OutboundItem) => {
  selectedOutbound.value = ob
  showInspectorDrawer.value = true
}

const openCreateDrawer = () => {
  editingOutbound.value = null
  showFormDrawer.value = true
}

const openEditDrawer = (ob: OutboundItem) => {
  editingOutbound.value = ob
  showInspectorDrawer.value = false
  showFormDrawer.value = true
}

const saveOutbound = async (payload: OutboundPayload) => {
  saving.value = true
  try {
    await api.post(`/outbounds?nodeId=${activeNodeId.value}`, payload)
    showFormDrawer.value = false
    toast.success(isRemoteNode.value ? '远端节点出站配置已保存' : '出站配置已保存')
    await fetchOutbounds()
  } catch (err: any) {
    toast.error('保存失败：' + err)
  } finally {
    saving.value = false
  }
}

const deleteOutbound = async (tag: string) => {
  if (!confirm(`确定删除出站节点 ${tag} 吗？`)) return
  try {
    await api.delete(`/outbounds/${encodeURIComponent(tag)}?nodeId=${activeNodeId.value}`)
    toast.success('出站节点已删除')
    if (selectedOutbound.value?.tag === tag) {
      showInspectorDrawer.value = false
      selectedOutbound.value = null
    }
    await fetchOutbounds()
  } catch (err: any) {
    toast.error('删除失败：' + err)
  }
}

const checkRouteQuery = () => {
  const editQuery = route.query.edit as string
  if (editQuery && outbounds.value.length > 0) {
    const target = outbounds.value.find((ob) => ob.tag === editQuery)
    if (target) {
      openEditDrawer(target)
    } else {
      toast.warning('未找到指定的出站节点：' + editQuery)
    }
  } else if (route.query.action === 'create' || route.query.create) {
    openCreateDrawer()
  }
}

onMounted(async () => {
  await fetchOutbounds()
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
  showFormDrawer.value = false
  selectedOutbound.value = null
  editingOutbound.value = null
  fetchOutbounds()
})
</script>
