import { ref, computed } from 'vue'
import {
  deleteNode,
  resetNodeTraffic,
  rotateNodeToken,
  updateNode,
  type NodeDesiredState,
  type NodeItem,
  type RotateTokenResult,
} from '../../../api/nodes'
import { useActiveNode } from '../../../composables/useActiveNode'
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

export function getAutoStateReasonBadge(reason?: string) {
  switch (reason) {
    case 'auto_drain_quota':
      return {
        label: '配额自动摘流',
        variant: 'warning' as const,
      }
    case 'auto_break_quota':
      return {
        label: '配额自动熔断',
        variant: 'destructive' as const,
      }
    case 'manual':
      return {
        label: '手动调度',
        variant: 'secondary' as const,
      }
    default:
      return null
  }
}

export interface NodeQuotaStats {
  usedBytes: number
  limitBytes: number
  hasLimit: boolean
  rawPct: number
  displayPct: string
  barWidth: number
  barClass: string
  textClass: string
}

export function getNodeQuotaStats(node: NodeItem): NodeQuotaStats {
  const usedBytes = Math.max(0, node.cycleUsedBytes || 0)
  const limitBytes = Math.max(0, node.trafficLimitBytes || 0)
  const hasLimit = limitBytes > 0
  if (!hasLimit) {
    return {
      usedBytes,
      limitBytes: 0,
      hasLimit: false,
      rawPct: 0,
      displayPct: '0',
      barWidth: 0,
      barClass: 'bg-emerald-500',
      textClass: 'text-emerald-400',
    }
  }

  const warnPct = node.warnThresholdPct > 0 ? node.warnThresholdPct : 80
  const drainPct = node.drainThresholdPct > 0 ? node.drainThresholdPct : 90
  const breakPct = node.breakThresholdPct > 0 ? node.breakThresholdPct : 95
  const rawPct = (usedBytes * 100) / limitBytes
  const roundedPct = Math.round(rawPct * 10) / 10
  const barWidth = Math.min(100, Math.max(0, rawPct))

  let barClass = 'bg-emerald-500'
  let textClass = 'text-emerald-400'
  if (rawPct >= breakPct) {
    barClass = 'bg-rose-500'
    textClass = 'text-rose-400'
  } else if (rawPct >= drainPct) {
    barClass = 'bg-orange-500'
    textClass = 'text-orange-400'
  } else if (rawPct >= warnPct) {
    barClass = 'bg-amber-400'
    textClass = 'text-amber-400'
  }

  return {
    usedBytes,
    limitBytes,
    hasLimit: true,
    rawPct,
    displayPct: roundedPct.toFixed(1).replace(/\.0$/, ''),
    barWidth,
    barClass,
    textClass,
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
  const {
    nodesList: nodes,
    nodesLoading: loading,
    loadNodes,
    refreshNodesNow,
    startListPolling,
    stopListPolling,
  } = useActiveNode()

  const resettingId = ref<number | null>(null)
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
    try {
      await loadNodes(true, { silent, rethrow: !silent })
    } catch (err: any) {
      if (!silent) {
        toast.error('加载节点列表失败：' + (err?.message || err))
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
      await refreshNodesNow()
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
      await refreshNodesNow()
      return true
    } catch (err: any) {
      toast.error('状态切换失败：' + (err?.message || err))
      return false
    }
  }

  const setNodeDesiredState = async (
    node: NodeItem,
    desiredState: NodeDesiredState
  ): Promise<boolean> => {
    const labelMap: Record<NodeDesiredState, string> = {
      active: '活跃分发 (active)',
      draining: '排空摘流 (draining)',
      circuit_broken: '熔断隔离 (circuit_broken)',
    }
    try {
      await updateNode(node.id, { desiredState })
      toast.success(`节点「${node.name}」已切换为 ${labelMap[desiredState]}`)
      await refreshNodesNow()
      return true
    } catch (err: any) {
      toast.error('调度状态更新失败：' + (err?.message || err))
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
      await refreshNodesNow()
      return res
    } catch (err: any) {
      toast.error('轮换 Token 失败：' + (err?.message || err))
      return null
    }
  }

  const handleResetTraffic = async (node: NodeItem): Promise<boolean> => {
    if (resettingId.value === node.id) return false
    if (
      !confirm(
        `确定重置节点「${node.name}」的本周期流量计数吗？若节点当前因配额触发自动摘流或熔断，重置后将自动恢复为 active。`
      )
    ) {
      return false
    }
    resettingId.value = node.id
    try {
      await resetNodeTraffic(node.id)
      toast.success(`节点「${node.name}」周期流量已重置`)
      await refreshNodesNow()
      return true
    } catch (err: any) {
      toast.error('重置周期流量失败：' + (err?.message || err))
      return false
    } finally {
      resettingId.value = null
    }
  }

  return {
    nodes,
    loading,
    resettingId,
    searchQuery,
    stateFilter,
    onlineFilter,
    summary,
    filteredNodes,
    fetchNodes,
    refreshNodesNow,
    startListPolling,
    stopListPolling,
    removeNode,
    toggleNodeEnabled,
    setNodeDesiredState,
    rotateToken,
    handleResetTraffic,
  }
}
