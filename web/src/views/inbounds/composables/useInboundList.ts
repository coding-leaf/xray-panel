import { ref, computed } from 'vue'
import api from '../../../api'
import { toast } from '../../../utils/toast'
import {
  getRealityStatus,
  checkRealityStatus,
  type RealitySummaryStatus,
} from '../../../api/reality'
import type { InboundItem } from '../types'

export function getRealityBadgeClass(status: 'ok' | 'warning' | 'error') {
  switch (status) {
    case 'ok':
      return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
    case 'warning':
      return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'error':
      return 'bg-rose-500/10 text-rose-400 border-rose-500/20'
  }
}

export function getRealityDotClass(status: 'ok' | 'warning' | 'error') {
  switch (status) {
    case 'ok':
      return 'bg-emerald-400'
    case 'warning':
      return 'bg-amber-400'
    case 'error':
      return 'bg-rose-400'
  }
}

export function getProtocolBadgeVariant(proto?: string) {
  switch (proto?.toLowerCase()) {
    case 'vless':
    case 'trojan':
    case 'vmess':
      return 'default' as const
    case 'shadowsocks':
    case 'socks':
      return 'secondary' as const
    case 'http':
    case 'dokodemo-door':
      return 'outline' as const
    default:
      return 'secondary' as const
  }
}

export function getSecurityBadgeVariant(sec?: string) {
  switch (sec?.toLowerCase()) {
    case 'reality':
      return 'default' as const
    case 'tls':
      return 'secondary' as const
    case 'none':
      return 'outline' as const
    default:
      return 'outline' as const
  }
}

export function getNodeFlow(inb: any): string {
  try {
    const s = JSON.parse(inb.settingsJson || '{}')
    if (s.flow !== undefined) return s.flow || 'none'
    return 'none'
  } catch {
    return 'none'
  }
}

export function getStreamNetwork(inb: any): string {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.network || 'tcp'
  } catch {
    return 'tcp'
  }
}

export function getSecurityType(inb: any): string {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.security || 'none'
  } catch {
    return 'none'
  }
}

export function isReality(inb: any): boolean {
  return getSecurityType(inb) === 'reality'
}

export function getClientCount(inb: any, usersList: any[] = []): number {
  if (['socks', 'http', 'dokodemo-door'].includes(inb.protocol)) {
    return 0
  }
  if (Array.isArray(usersList) && usersList.length > 0) {
    return usersList.filter((u: any) => {
      const tags = (u.inboundTags || u.inboundTag || '').split(',').map((s: string) => s.trim())
      return tags.includes(inb.tag)
    }).length
  }
  try {
    const s = JSON.parse(inb.settingsJson || '{}')
    return s.clients?.length || 0
  } catch {
    return 0
  }
}

export function getInboundRealityOverallStatus(tag: string, summary: RealitySummaryStatus | null) {
  const items = summary?.items?.filter((item) => item.inboundTag === tag) || []
  if (!items.length) return null
  if (items.some((i) => i.status === 'error')) {
    const firstErr = items.find((i) => i.status === 'error')!
    return {
      status: 'error' as const,
      badgeText: `Reality 异常: ${firstErr.details || firstErr.errorType || '合规异常'}`,
      item: firstErr,
      items,
    }
  }
  if (items.some((i) => i.status === 'warning')) {
    const firstWarn = items.find((i) => i.status === 'warning')!
    return {
      status: 'warning' as const,
      badgeText: `Reality 预警: ${firstWarn.details || firstWarn.errorType || '临期或预警'}`,
      item: firstWarn,
      items,
    }
  }
  const firstOk = items[0]
  const tls = firstOk.tlsVersion || 'TLS 1.3'
  const alpn = firstOk.alpn ? ` / ${firstOk.alpn}` : ''
  return {
    status: 'ok' as const,
    badgeText: `Reality 正常 (${tls}${alpn})`,
    item: firstOk,
    items,
  }
}

export function getInboundRealityField(inb: any, field: string): string {
  try {
    const stream = JSON.parse(inb.streamSettingsJson || inb.streamSettings || '{}')
    if (stream.realitySettings) {
      if (field === 'serverNames' && Array.isArray(stream.realitySettings.serverNames)) {
        return stream.realitySettings.serverNames.join(', ')
      }
      if (field === 'shortIds' && Array.isArray(stream.realitySettings.shortIds)) {
        return stream.realitySettings.shortIds.join(', ')
      }
      return stream.realitySettings[field] || ''
    }
    return ''
  } catch {
    return ''
  }
}

export function getAssignedUserEmails(inb: any, usersList: any[] = []): string[] {
  const result: string[] = []
  try {
    const s = JSON.parse(inb.settingsJson || '{}')
    if (Array.isArray(s.clients)) {
      for (const c of s.clients) {
        if (c.email) result.push(c.email)
      }
    }
  } catch {}
  if (Array.isArray(usersList)) {
    for (const u of usersList) {
      const tags = (u.inboundTags || u.inboundTag || '').split(',').map((s: string) => s.trim())
      if (tags.includes(inb.tag) && !result.includes(u.email)) {
        result.push(u.email)
      }
    }
  }
  return result
}

export function useInboundList() {
  const inbounds = ref<InboundItem[]>([])
  const usersList = ref<any[]>([])
  const availableOutbounds = ref<string[]>(['direct', 'block'])
  const checkingReality = ref(false)
  const realitySummary = ref<RealitySummaryStatus | null>(null)
  const searchQuery = ref('')
  const protocolFilter = ref('all')

  const filteredInbounds = computed(() => {
    return inbounds.value.filter((ib) => {
      // Protocol filter
      if (protocolFilter.value !== 'all' && ib.protocol?.toLowerCase() !== protocolFilter.value.toLowerCase()) {
        return false
      }
      // Search query
      if (!searchQuery.value) return true
      const q = searchQuery.value.toLowerCase().trim()
      const tagMatch = ib.tag?.toLowerCase().includes(q)
      const portMatch = String(ib.port).includes(q) || String(ib.externalPort || '').includes(q)
      const protoMatch = ib.protocol?.toLowerCase().includes(q)
      const hostMatch = ib.externalHost?.toLowerCase().includes(q)
      return tagMatch || portMatch || protoMatch || hostMatch
    })
  })

  const realityAlertCount = computed(() => {
    return (realitySummary.value?.errorCount || 0) + (realitySummary.value?.warningCount || 0)
  })

  const fetchRealityStatus = async () => {
    try {
      const rawRes: any = await getRealityStatus()
      realitySummary.value =
        rawRes?.data && typeof rawRes.data === 'object' && rawRes.data.totalChecked !== undefined
          ? rawRes.data
          : rawRes
    } catch (err) {
      console.error('Failed to fetch reality status:', err)
    }
  }

  const triggerRealityCheck = async () => {
    checkingReality.value = true
    try {
      const rawRes: any = await checkRealityStatus()
      const res: RealitySummaryStatus =
        rawRes?.data && typeof rawRes.data === 'object' && rawRes.data.totalChecked !== undefined
          ? rawRes.data
          : rawRes
      realitySummary.value = res
      const count = res?.totalChecked ?? res?.totalCount ?? (res?.items?.length || 0)
      const errCount = res?.errorCount ?? 0
      const warnCount = res?.warningCount ?? 0
      if (errCount > 0) {
        toast.warning(`检测完成：发现 ${errCount} 个 Reality 异常域名`)
      } else if (warnCount > 0) {
        toast.warning(`检测完成：发现 ${warnCount} 个 Reality 预警域名`)
      } else {
        toast.success(`Reality 检测完成：共巡检 ${count} 个目标，全部合规`)
      }
    } catch (err: any) {
      toast.error('检测失败: ' + (err?.message || err))
    } finally {
      checkingReality.value = false
    }
  }

  const fetchAll = async () => {
    try {
      const [inbRes, uRes, obRes]: any = await Promise.all([
        api.get('/inbounds'),
        api.get('/users'),
        api.get('/outbounds').catch(() => []),
      ])
      inbounds.value = (inbRes || []).map((ib: any) => {
        let srs: any[] = []
        try {
          srs = JSON.parse(ib.subRoutesJson || '[]')
        } catch (e) {}
        return {
          ...ib,
          subRoutes: srs.map((r: any) => ({
            ...r,
            name: r.name || r.remark || '',
            allowedUsers: Array.isArray(r.allowedUsers) ? r.allowedUsers : [],
          })),
        }
      })
      usersList.value = uRes || []
      if (Array.isArray(obRes) && obRes.length > 0) {
        availableOutbounds.value = obRes.map((o: any) => o.tag).filter((t: string) => t)
      }
      if (!availableOutbounds.value.includes('direct')) {
        availableOutbounds.value.unshift('direct')
      }
      fetchRealityStatus()
    } catch (err) {
      console.error(err)
    }
  }

  const deleteInbound = async (id: number): Promise<boolean> => {
    if (!confirm('确定删除该入站节点吗？')) return false
    try {
      await api.delete(`/inbounds/${id}`)
      toast.success('入站节点已成功删除')
      await fetchAll()
      return true
    } catch (err: any) {
      toast.error('删除失败: ' + err)
      return false
    }
  }

  return {
    inbounds,
    usersList,
    availableOutbounds,
    checkingReality,
    realitySummary,
    searchQuery,
    protocolFilter,
    filteredInbounds,
    realityAlertCount,
    fetchAll,
    fetchRealityStatus,
    triggerRealityCheck,
    deleteInbound,
    getInboundRealityOverallStatus: (tag: string) => getInboundRealityOverallStatus(tag, realitySummary.value),
    getInboundRealityField,
    getAssignedUserEmails: (inb: any) => getAssignedUserEmails(inb, usersList.value),
    getRealityBadgeClass,
    getRealityDotClass,
    getProtocolBadgeVariant,
    getSecurityBadgeVariant,
    getNodeFlow,
    getClientCount: (inb: any) => getClientCount(inb, usersList.value),
    getStreamNetwork,
    getSecurityType,
    isReality,
  }
}
