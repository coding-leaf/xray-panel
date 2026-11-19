import { ref, computed } from 'vue'
import {
  listNodes,
  deleteNode,
  rotateNodeToken,
  updateNode,
  type NodeDesiredState,
  type NodeItem,
  type RotateTokenResult,
} from '../../../api/nodes'
import { toast } from '../../../utils/toast'
import { formatDateTime } from '../../../utils/format'

export function getDesiredStateBadge(state: NodeDesiredState) {
  switch (state) {
    case 'active':
      return {
        label: '活跃',
        variant: 'success' as const,
      }
    case 'draining':
      return {
        label: '排空',
        variant: 'warning' as const,
      }
    case 'circuit_broken':
      return {
        label: '熔断',
        variant: 'destructive' as const,
      }
    default:
      return {
        label: state || '未知',
        variant: 'secondary' as const,
      }
  }
}

export function formatLastSeen(lastSeenAt: string | null): string {
  if (!lastSeenAt) return '从未上报'
  const ts = new Date(lastSeenAt).getTime()
  if (Number.isNaN(ts)) return '-'
  const diffSec = Math.max(0, Math.floor((Date.now() - ts) / 1000))
  if (diffSec < 15) return '刚刚'
  if (diffSec < 60) return `${diffSec}s 前`
  const diffMin = Math.floor(diffSec / 60)
  if (diffMin < 60) return `${diffMin}m 前`
  const diffHour = Math.floor(diffMin / 60)
  if (diffHour < 24) return `${diffHour}h 前`
  return formatDateTime(lastSeenAt)
}

export function isBoundInboundsError(err: unknown): boolean {
  const msg = String(err || '').toLowerCase()
  return (
    msg.includes('bound inbound') ||
    msg.includes('still has') ||
    msg.includes('409') ||
    msg.includes('关联入站')
  )
}

export function useNodes() {
  const nodes = ref<NodeItem[]>([])
  const loading = ref(false)
  const searchQuery = ref('')
  const stateFilter = ref<'all' | NodeDesiredState>('all')
  const onlineFilter = ref<'all' | 'online' | 'offline'>('all')

  const summary = computed(() => {
    const list = nodes.value
    const total = list.length
    const onlineCount = list.filter((n) => n.online).length
    const activeCount = list.filter((n) => n.enabled && n.desiredState === 'active').length
    const errorCount = list.filter((n) => Boolean(n.lastSyncError?.trim())).length
    const totalXrayBytes = list.reduce(
      (sum, n) => sum + (n.xrayUpBytes || 0) + (n.xrayDownBytes || 0),
      0
    )
    const totalNicBytes = list.reduce(
      (sum, n) => sum + (n.nicUpBytes || 0) + (n.nicDownBytes || 0),
      0
    )
    return {
      total,
      onlineCount,
      activeCount,
      errorCount,
      totalXrayBytes,
      totalNicBytes,
    }
  })

  const filteredNodes = computed(() => {
    const q = searchQuery.value.trim().toLowerCase()
    return nodes.value.filter((n) => {
      if (stateFilter.value !== 'all' && n.desiredState !== stateFilter.value) {
        return false
      }
      if (onlineFilter.value === 'online' && !n.online) {
        return false
      }
      if (onlineFilter.value === 'offline' && n.online) {
        return false
      }
      if (!q) return true
      return (
        n.name.toLowerCase().includes(q) ||
        n.host.toLowerCase().includes(q) ||
        (n.region || '').toLowerCase().includes(q) ||
        (n.tokenPrefix || '').toLowerCase().includes(q) ||
        (n.remark || '').toLowerCase().includes(q)
      )
    })
  })

  const fetchNodes = async (silent = false) => {
    if (!silent) {
      loading.value = true
    }
    try {
      nodes.value = await listNodes()
    } catch (err: any) {
      if (!silent) {
        toast.error('加载节点列表失败：' + (err?.message || err))
      }
    } finally {
      if (!silent) {
        loading.value = false
      }
    }
  }

  const removeNode = async (node: NodeItem): Promise<boolean> => {
    if (!confirm(`确定删除节点「${node.name}」吗？删除后对应 Agent 将立即失去同步权限。`)) {
      return false
    }
    try {
      await deleteNode(node.id)
      toast.success(`节点「${node.name}」已删除`)
      await fetchNodes(true)
      return true
    } catch (err: any) {
      if (isBoundInboundsError(err)) {
        toast.error(
          `无法删除节点「${node.name}」：仍存在关联的入站规则，请先在「入站网关」中解绑或删除对应入站`
        )
      } else {
        toast.error('删除失败：' + (err?.message || err))
      }
      return false
    }
  }

  const toggleNodeEnabled = async (node: NodeItem): Promise<boolean> => {
    const nextEnabled = !node.enabled
    try {
      await updateNode(node.id, { enabled: nextEnabled })
      toast.success(nextEnabled ? `已启用节点「${node.name}」` : `已停用节点「${node.name}」`)
      await fetchNodes(true)
      return true
    } catch (err: any) {
      toast.error('状态切换失败：' + (err?.message || err))
      return false
    }
  }

  const rotateToken = async (node: NodeItem): Promise<RotateTokenResult | null> => {
    if (
      !confirm(
        `确定为节点「${node.name}」轮换 Token 吗？旧 Token 将立即失效，需在对应服务器更新 Agent 服务配置。`
      )
    ) {
      return null
    }
    try {
      const res = await rotateNodeToken(node.id)
      toast.success(`节点「${node.name}」Token 已轮换`)
      await fetchNodes(true)
      return res
    } catch (err: any) {
      toast.error('轮换 Token 失败：' + (err?.message || err))
      return null
    }
  }

  return {
    nodes,
    loading,
    searchQuery,
    stateFilter,
    onlineFilter,
    summary,
    filteredNodes,
    fetchNodes,
    removeNode,
    toggleNodeEnabled,
    rotateToken,
  }
}
