<template>
  <div class="space-y-4">
    <!-- Main User Table -->
    <UserTable
      :users="users" :filtered-users="filteredUsers"
      :selected-user-ids="selectedUserIds" :available-inbounds="availableInbounds"
      :current-inspector-user-id="currentInspectorUser?.id"
      :online-users-count="onlineUsersCount" :enabled-users-count="enabledUsersCount"
      :attention-users-count="attentionUsersCount" :is-all-users-selected="isAllUsersSelected"
      v-model:search-query="searchQuery" v-model:status-filter="statusFilter"
      :is-user-expired="isUserExpired" :get-user-badge-variant="getUserBadgeVariant"
      :get-user-status-text="getUserStatusText" :get-traffic-percent="getTrafficPercent"
      :get-traffic-progress-class="getTrafficProgressClass"
      @update:selected-user-ids="selectedUserIds = $event"
      @toggle-select-all="toggleSelectAllUsers" @clear-selection="selectedUserIds = []"
      @add="openAddModal" @inspect="openInspectDrawer" @edit="openEditModal"
      @share="openShareModal" @history="openHistoryModal" @delete="handleDeleteUser"
      @batch-renew="batchRenew" @batch-reset-traffic="batchResetTraffic"
      @batch-set-status="batchSetStatus" @batch-delete="batchDeleteUsers"
    />

    <!-- User Inspector Drawer (Synchronous) -->
    <UserDetailDrawer
      v-model="showInspectorDrawer" :user="currentInspectorUser" :available-inbounds="availableInbounds"
      :get-user-badge-variant="getUserBadgeVariant" :get-user-status-text="getUserStatusText"
      :get-traffic-percent="getTrafficPercent" :get-traffic-progress-class="getTrafficProgressClass"
      @edit="openEditModal" @delete="handleDeleteUser" @share="openShareModal"
      @history="openHistoryModal" @toggle-enabled="toggleUserEnabled"
      @reset-traffic="resetTraffic" @renew-one="batchRenewOne" @reset-token="handleResetToken"
    />

    <!-- Async Lazy Loaded Form Drawer & Modals -->
    <UserFormDrawer
      v-if="showFormDrawer" v-model="showFormDrawer"
      :is-editing="isEditingForm" :initial-data="formUser"
      :available-inbounds="availableInbounds" @saved="fetchAll"
    />
    <UserShareModal
      v-if="showShareModal" v-model="showShareModal"
      :share-data="currentShareData" @reset-token="handleResetToken"
    />
    <UserTrafficModal
      v-if="showHistoryModal" v-model="showHistoryModal"
      :user="currentHistoryUser"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, defineAsyncComponent } from 'vue'
import api from '../api'
import { toast } from '../utils/toast'
import type { UserItem, UserShareData } from './users/types'
import { useUserList } from './users/composables/useUserList'

// Synchronous components
import UserTable from './users/components/UserTable.vue'
import UserDetailDrawer from './users/components/UserDetailDrawer.vue'

// Asynchronous lazy-loaded heavy components
const UserFormDrawer = defineAsyncComponent(() => import('./users/components/UserFormDrawer.vue'))
const UserShareModal = defineAsyncComponent(() => import('./users/components/UserShareModal.vue'))
const UserTrafficModal = defineAsyncComponent(() => import('./users/components/UserTrafficModal.vue'))

const {
  users, availableInbounds, selectedUserIds, searchQuery, statusFilter,
  filteredUsers, isAllUsersSelected, onlineUsersCount, enabledUsersCount,
  attentionUsersCount, isUserExpired, getUserBadgeVariant, getUserStatusText,
  getTrafficPercent, getTrafficProgressClass, toggleSelectAllUsers,
  fetchAll, fetchSpeeds, batchRenew, batchRenewOne, batchResetTraffic,
  batchSetStatus, batchDeleteUsers, toggleUserEnabled, deleteUser,
  resetTraffic, resetUserSubToken,
} = useUserList()

// Modal & Drawer States
const showInspectorDrawer = ref(false)
const selectedInspectorUser = ref<UserItem | null>(null)
const currentInspectorUser = computed(() => {
  if (!selectedInspectorUser.value) return null
  return users.value.find((u) => u.id === selectedInspectorUser.value?.id) || selectedInspectorUser.value
})

const showFormDrawer = ref(false)
const isEditingForm = ref(false)
const formUser = ref<UserItem | null>(null)

const showShareModal = ref(false)
const currentShareData = ref<UserShareData | null>(null)

const showHistoryModal = ref(false)
const currentHistoryUser = ref<UserItem | null>(null)

// Action Handlers
const openInspectDrawer = (user: UserItem) => {
  selectedInspectorUser.value = user
  showInspectorDrawer.value = true
}

const openAddModal = () => {
  isEditingForm.value = false
  formUser.value = null
  showFormDrawer.value = true
}

const openEditModal = (user: UserItem) => {
  isEditingForm.value = true
  formUser.value = user
  showFormDrawer.value = true
}

const openShareModal = async (user: UserItem) => {
  try {
    const res: any = await api.get(`/users/${user.id}/share`)
    currentShareData.value = { ...res, user }
    showShareModal.value = true
  } catch (err: any) {
    toast.error('获取订阅链接失败: ' + (err.message || err))
  }
}

const openHistoryModal = (user: UserItem) => {
  currentHistoryUser.value = user
  showHistoryModal.value = true
}

const handleDeleteUser = async (id: number) => {
  const ok = await deleteUser(id)
  if (ok && showInspectorDrawer.value && selectedInspectorUser.value?.id === id) {
    showInspectorDrawer.value = false
  }
}

const handleResetToken = async (userId: number) => {
  const res = await resetUserSubToken(userId)
  if (res && currentShareData.value?.user && currentShareData.value.user.id === userId) {
    currentShareData.value.user.subToken = res.subToken
    if (res.uuid) currentShareData.value.user.uuid = res.uuid
  }
}

// Speeds Polling & Lifecycle
let speedTimer: ReturnType<typeof setInterval> | null = null

const handleVisibilityChange = () => {
  if (!document.hidden) fetchSpeeds()
}

onMounted(async () => {
  await fetchAll()
  document.addEventListener('visibilitychange', handleVisibilityChange)
  speedTimer = setInterval(fetchSpeeds, 3000)
})

onUnmounted(() => {
  if (speedTimer) {
    clearInterval(speedTimer)
    speedTimer = null
  }
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
