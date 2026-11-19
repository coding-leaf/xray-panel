import { ref, computed, watch, onMounted, onUnmounted, getCurrentInstance } from 'vue'
import { getNodeSyncStatus, listNodes, type NodeItem } from '../api/nodes'

const ACTIVE_NODE_STORAGE_KEY = 'xray_panel_active_node_id'
const PENDING_SYNC_POLL_MS = 5000
const LIST_POLL_MS = 15000

export interface LoadNodesOptions {
  silent?: boolean
  rethrow?: boolean
}

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
const listPollSubscribers = ref<number>(0)
const isDocumentHidden = ref<boolean>(typeof document !== 'undefined' ? document.hidden : false)

let inflightLoad: Promise<{ list: NodeItem[]; err: unknown; failed: boolean }> | null = null
let loadSeq = 0
let syncPollTimer: ReturnType<typeof setInterval> | null = null
let syncPollInFlight = false
let listPollTimer: ReturnType<typeof setInterval> | null = null
let listPollInFlight = false
let visibilityBound = false

const activeNode = computed<NodeItem | null>(() => {
  if (activeNodeId.value <= 0) return null
  return nodesList.value.find((n) => n.id === activeNodeId.value) || null
})

const isRemoteNode = computed<boolean>(() => activeNodeId.value > 0)

const hasAuthToken = () =>
  typeof window === 'undefined' || Boolean(window.localStorage.getItem('token'))

const shouldPollPendingSync = computed<boolean>(() => {
  if (subscriberCount.value <= 0 || isDocumentHidden.value) return false
  if (!hasAuthToken()) return false
  if (activeNodeId.value <= 0 || !activeNode.value) return false
  return getNodeSyncStatus(activeNode.value) === 'pending'
})

const shouldPollList = computed<boolean>(() => {
  if (listPollSubscribers.value <= 0 || isDocumentHidden.value) return false
  if (!hasAuthToken()) return false
  return true
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

async function loadNodes(force = false, options?: LoadNodesOptions): Promise<NodeItem[]> {
  if (!hasAuthToken()) {
    return []
  }
  const rethrow = Boolean(options?.rethrow)
  if (!force && inflightLoad) {
    const res = await inflightLoad
    if (rethrow && res.failed) {
      throw res.err
    }
    return res.list
  }
  const silent = Boolean(options?.silent)
  const seq = ++loadSeq
  if (!silent) {
    nodesLoading.value = true
  }
  const request = listNodes()
    .then((list) => {
      if (seq === loadSeq) {
        nodesList.value = Array.isArray(list) ? list : []
        if (activeNodeId.value > 0 && !nodesList.value.some((n) => n.id === activeNodeId.value)) {
          setActiveNode(0)
        }
      }
      return { list: nodesList.value, err: null as unknown, failed: false }
    })
    .catch((err) => {
      console.error('Failed to load nodes:', err)
      return { list: nodesList.value, err, failed: true }
    })
    .finally(() => {
      if (seq === loadSeq) {
        nodesLoading.value = false
        inflightLoad = null
      }
    })
  inflightLoad = request
  const res = await request
  if (rethrow && res.failed) {
    throw res.err
  }
  return res.list
}

async function refreshNodesNow(): Promise<NodeItem[]> {
  return loadNodes(true, { silent: true })
}

function bindVisibilityListener() {
  if (!visibilityBound && typeof document !== 'undefined') {
    isDocumentHidden.value = document.hidden
    document.addEventListener('visibilitychange', handleVisibilityChange)
    visibilityBound = true
  }
}

function maybeUnbindVisibilityListener() {
  if (
    subscriberCount.value === 0 &&
    listPollSubscribers.value === 0 &&
    visibilityBound &&
    typeof document !== 'undefined'
  ) {
    document.removeEventListener('visibilitychange', handleVisibilityChange)
    visibilityBound = false
  }
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

function stopListPollTimer() {
  if (listPollTimer) {
    clearInterval(listPollTimer)
    listPollTimer = null
  }
}

async function runListPollTick() {
  if (listPollInFlight || syncPollInFlight || !shouldPollList.value) return
  listPollInFlight = true
  try {
    await refreshNodesNow()
  } finally {
    listPollInFlight = false
  }
}

function ensureListPollTimer() {
  if (listPollTimer || !shouldPollList.value) return
  listPollTimer = setInterval(() => {
    void runListPollTick()
  }, LIST_POLL_MS)
}

function startListPolling() {
  listPollSubscribers.value += 1
  bindVisibilityListener()
  if (shouldPollList.value) {
    ensureListPollTimer()
  }
}

function stopListPolling() {
  listPollSubscribers.value = Math.max(0, listPollSubscribers.value - 1)
  if (listPollSubscribers.value === 0) {
    stopListPollTimer()
    maybeUnbindVisibilityListener()
  }
}

function handleVisibilityChange() {
  isDocumentHidden.value = typeof document !== 'undefined' ? document.hidden : false
  if (isDocumentHidden.value) {
    stopSyncPolling()
    stopListPollTimer()
    return
  }
  if (shouldPollPendingSync.value) {
    startSyncPolling()
    void runSyncPollTick()
  } else if (shouldPollList.value) {
    ensureListPollTimer()
    void runListPollTick()
  }
  if (shouldPollList.value) {
    ensureListPollTimer()
  }
}

watch(shouldPollPendingSync, (active) => {
  if (active) {
    startSyncPolling()
  } else {
    stopSyncPolling()
  }
})

watch(shouldPollList, (active) => {
  if (active) {
    ensureListPollTimer()
  } else {
    stopListPollTimer()
  }
})

export function useActiveNode() {
  if (getCurrentInstance()) {
    onMounted(() => {
      subscriberCount.value += 1
      bindVisibilityListener()
    })

    onUnmounted(() => {
      subscriberCount.value = Math.max(0, subscriberCount.value - 1)
      if (subscriberCount.value === 0) {
        stopSyncPolling()
      }
      maybeUnbindVisibilityListener()
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
    startListPolling,
    stopListPolling,
  }
}
