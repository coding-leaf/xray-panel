import api from './index'

export type RealityDomainStatus = 'ok' | 'warning' | 'error'

export interface RealityCheckItem {
  inboundId: number
  inboundTag: string
  dest: string
  serverName: string
  status: RealityDomainStatus
  errorType?: string
  details: string
  tlsVersion?: string
  alpn?: string
  certExpiry?: string
  daysLeft: number
  latencyMs: number
  checkedAt: string
}

export interface RealitySummaryStatus {
  totalChecked: number
  totalCount?: number
  okCount: number
  warningCount: number
  errorCount: number
  items: RealityCheckItem[]
  lastCheckAt?: string
  checkedAt?: string
}

export const getRealityStatus = async (): Promise<RealitySummaryStatus> => {
  const res: any = await api.get('/inbounds/reality-status')
  if (res && res.data && typeof res.data === 'object' && res.data.totalChecked !== undefined) {
    return res.data
  }
  return res
}

export const checkRealityStatus = async (): Promise<RealitySummaryStatus> => {
  const res: any = await api.post('/inbounds/reality-status/check')
  if (res && res.data && typeof res.data === 'object' && res.data.totalChecked !== undefined) {
    return res.data
  }
  return res
}
