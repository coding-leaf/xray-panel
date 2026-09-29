<template>
  <div class="space-y-4">
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
      :saving="saving"
      @save="saveOutbound"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch, defineAsyncComponent } from 'vue'
import { useRoute } from 'vue-router'
import { toast } from '../utils/toast'
import api from '../api'
import type { OutboundItem, OutboundPayload } from './outbounds/types'
import OutboundTable from './outbounds/components/OutboundTable.vue'
import OutboundDetailDrawer from './outbounds/components/OutboundDetailDrawer.vue'

const OutboundFormDrawer = defineAsyncComponent(
  () => import('./outbounds/components/OutboundFormDrawer.vue')
)

const route = useRoute()
const outbounds = ref<OutboundItem[]>([])
const showInspectorDrawer = ref(false)
const showFormDrawer = ref(false)
const selectedOutbound = ref<OutboundItem | null>(null)
const editingOutbound = ref<OutboundItem | null>(null)
const saving = ref(false)

const fetchOutbounds = async () => {
  try {
    outbounds.value = await api.get('/outbounds')
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
    await api.post('/outbounds', payload)
    showFormDrawer.value = false
    toast.success('出站配置已保存成功！')
    await fetchOutbounds()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteOutbound = async (tag: string) => {
  if (!confirm(`确定删除出站节点 ${tag} 吗？`)) return
  try {
    await api.delete(`/outbounds/${tag}`)
    toast.success('出站节点已成功删除！')
    if (selectedOutbound.value?.tag === tag) {
      showInspectorDrawer.value = false
      selectedOutbound.value = null
    }
    await fetchOutbounds()
  } catch (err: any) {
    toast.error('删除失败: ' + err)
  }
}

const checkRouteQuery = () => {
  const editQuery = route.query.edit as string
  if (editQuery && outbounds.value.length > 0) {
    const target = outbounds.value.find((ob) => ob.tag === editQuery)
    if (target) {
      openEditDrawer(target)
    } else {
      toast.warning('未找到指定的出站节点: ' + editQuery)
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
</script>
