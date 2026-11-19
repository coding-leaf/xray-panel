import { ref, computed } from 'vue'
import { listNodes, type NodeItem } from '../api/nodes'

const ACTIVE_NODE_STORAGE_KEY = 'xray_panel_active_node_id'

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
let inflightLoad: Promise<NodeItem[]> | null = null

const activeNode = computed<NodeItem | null>(() => {
  if (activeNodeId.value <= 0) return null
  return nodesList.value.find((n) => n.id === activeNodeId.value) || null
})

const isRemoteNode = computed<boolean>(() => activeNodeId.value > 0)

function setActiveNode(id: number) {
  const normalized = Number.isInteger(id) && id > 0 ? id : 0
  activeNodeId.value = normalized
  writeStoredNodeId(normalized)
}

async function loadNodes(): Promise<NodeItem[]> {
  if (typeof window !== 'undefined' && !window.localStorage.getItem('token')) {
    return []
  }
  if (inflightLoad) {
    return inflightLoad
  }
  nodesLoading.value = true
  inflightLoad = listNodes()
    .then((list) => {
      nodesList.value = Array.isArray(list) ? list : []
      if (activeNodeId.value > 0 && !nodesList.value.some((n) => n.id === activeNodeId.value)) {
        setActiveNode(0)
      }
      return nodesList.value
    })
    .catch((err) => {
      console.error('Failed to load nodes:', err)
      return nodesList.value
    })
    .finally(() => {
      nodesLoading.value = false
      inflightLoad = null
    })
  return inflightLoad
}

export function useActiveNode() {
  return {
    activeNodeId,
    nodesList,
    nodesLoading,
    activeNode,
    isRemoteNode,
    loadNodes,
    setActiveNode,
  }
}
