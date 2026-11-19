import { ref, computed, watch, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import { getNodeSyncStatus, listNodes, type NodeItem } from '../api/nodes'

const ACTIVE_NODE_STORAGE_KEY = 'xray_panel_active_node_id'
const PENDING_SYNC_POLL_MS = 5000

function readStoredNodeId(): number {
  if (typeof window === 'undefined') return 0
  try {
    const raw = window.localStorage.getItem(ACTIVE_NODE_STORAGE_KEY)
    if (!raw) return 0
    const parsed = Number(raw)
    if (Number.isInteger(parsed) && parsed > 0) {
      return parsed
    }
  } catch {
    // ignore storage access errors
  }
  return 0
}

function writeStoredNodeId(id: number) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(ACTIVE_NODE_STORAGE_KEY, String(id))
  } catch {
    // ignore storage access errors
  }
}

const activeNodeId = ref<number>(readStoredNodeId())
const nodesList = ref<NodeItem[]>([])
const nodesLoading = ref<boolean>(false)
const subscriberCount = ref<number>(0)
const isDocumentHidden = ref<boolean>(typeof document !== 'undefined' ? document.hidden : false)

let inflightLoad: Promise<NodeItem[]> | null = null
let loadSeq = 0
let syncPollTimer: ReturnType<typeof setInterval> | null = null
let syncPollInFlight = false
let visibilityBound = false

const activeNode = computed<NodeItem | null>(() => {
  if (activeNodeId.value <= 0) return null
  return nodesList.value.find((n) => n.id === activeNodeId.value) || null
})

const isRemoteNode = computed<boolean>(() => activeNodeId.value > 0)

const shouldPollPendingSync = computed<boolean>(() => {
  if (subscriberCount.value <= 0 || isDocumentHidden.value) return false
  if (typeof window !== 'undefined' && !window.localStorage.getItem('token')) return false
  if (activeNodeId.value <= 0 || !activeNode.value) return false
  return getNodeSyncStatus(activeNode.value) === 'pending'
})

function setActiveNode(id: number) {
  const normalized = Number.isInteger(id) && id > 0 ? id : 0
  const changed = activeNodeId.value !== normalized
  activeNodeId.value = normalized
  writeStoredNodeId(normalized)
  if (changed && normalized > 0) {
    void refreshNodesNow()
  }
}

async function loadNodes(force = false): Promise<NodeItem[]> {
  if (typeof window !== 'undefined' && !window.localStorage.getItem('token')) {
    return []
  }
  if (!force && inflightLoad) {
    return inflightLoad
  }
  const seq = ++loadSeq
  nodesLoading.value = true
  const request = listNodes()
    .then((list) => {
      if (seq === loadSeq) {
        nodesList.value = Array.isArray(list) ? list : []
        if (activeNodeId.value > 0 && !nodesList.value.some((n) => n.id === activeNodeId.value)) {
          setActiveNode(0)
        }
      }
      return nodesList.value
    })
    .catch((err) => {
      console.error('Failed to load nodes:', err)
      return nodesList.value
    })
    .finally(() => {
      if (seq === loadSeq) {
        nodesLoading.value = false
        inflightLoad = null
      }
    })
  inflightLoad = request
  return request
}

async function refreshNodesNow(): Promise<NodeItem[]> {
  return loadNodes(true)
}

function stopSyncPolling() {
  if (syncPollTimer) {
    clearInterval(syncPollTimer)
    syncPollTimer = null
  }
}

async function runSyncPollTick() {
  if (syncPollInFlight || !shouldPollPendingSync.value) return
  syncPollInFlight = true
  try {
    await refreshNodesNow()
  } finally {
    syncPollInFlight = false
  }
}

function startSyncPolling() {
  if (syncPollTimer || !shouldPollPendingSync.value) return
  syncPollTimer = setInterval(() => {
    void runSyncPollTick()
  }, PENDING_SYNC_POLL_MS)
}

function handleVisibilityChange() {
  isDocumentHidden.value = typeof document !== 'undefined' ? document.hidden : false
  if (!isDocumentHidden.value && shouldPollPendingSync.value) {
    void runSyncPollTick()
  }
}

watch(shouldPollPendingSync, (active) => {
  if (active) {
    startSyncPolling()
  } else {
    stopSyncPolling()
  }
})

export function useActiveNode() {
  if (getCurrentInstance()) {
    onMounted(() => {
      subscriberCount.value += 1
      if (!visibilityBound && typeof document !== 'undefined') {
        isDocumentHidden.value = document.hidden
        document.addEventListener('visibilitychange', handleVisibilityChange)
        visibilityBound = true
      }
    })

    onUnmounted(() => {
      subscriberCount.value = Math.max(0, subscriberCount.value - 1)
      if (subscriberCount.value === 0) {
        stopSyncPolling()
        if (visibilityBound && typeof document !== 'undefined') {
          document.removeEventListener('visibilitychange', handleVisibilityChange)
          visibilityBound = false
        }
      }
    })
  }

  return {
    activeNodeId,
    nodesList,
    nodesLoading,
    activeNode,
    isRemoteNode,
    loadNodes,
    refreshNodesNow,
    setActiveNode,
  }
}
