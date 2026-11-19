import api from './index'

export type NodeDesiredState = 'active' | 'draining' | 'circuit_broken'
export type NodeTrafficCountMode = 'both' | 'outbound' | 'max'
export type NodeSyncStatus = 'synced' | 'pending' | 'error'

export interface NodeItem {
  id: number
  name: string
  host: string
  region: string
  tokenPrefix: string
  enabled: boolean
  desiredState: NodeDesiredState
  lastSeenAt: string | null
  agentVersion: string
  xrayVersion: string
  lastConfigHash: string
  expectedConfigHash?: string
  configSyncStatus?: NodeSyncStatus
  lastUsersHash: string
  lastSyncError: string
  cpuPercent: number
  memPercent: number
  nicUpBytes: number
  nicDownBytes: number
  xrayUpBytes: number
  xrayDownBytes: number
  trafficLimitBytes: number
  billingResetDay: number
  lastResetMonth: number
  trafficCountMode: NodeTrafficCountMode
  trafficMultiplier: number
  warnThresholdPct: number
  drainThresholdPct: number
  breakThresholdPct: number
  cycleXrayUpBytes: number
  cycleXrayDownBytes: number
  cycleNicUpBytes: number
  cycleNicDownBytes: number
  cycleUsedBytes: number
  autoStateReason: string
  remark: string
  createdAt: string
  updatedAt: string
  online: boolean
}

export interface CreateNodePayload {
  name: string
  host: string
  region?: string
  enabled?: boolean
  desiredState?: NodeDesiredState
  trafficLimitBytes?: number
  billingResetDay?: number
  trafficCountMode?: NodeTrafficCountMode
  trafficMultiplier?: number
  warnThresholdPct?: number
  drainThresholdPct?: number
  breakThresholdPct?: number
  remark?: string
}

export interface CreateNodeResult extends NodeItem {
  token: string
}

export interface UpdateNodePayload {
  name?: string
  host?: string
  region?: string
  enabled?: boolean
  desiredState?: NodeDesiredState
  trafficLimitBytes?: number
  billingResetDay?: number
  trafficCountMode?: NodeTrafficCountMode
  trafficMultiplier?: number
  warnThresholdPct?: number
  drainThresholdPct?: number
  breakThresholdPct?: number
  remark?: string
}

export interface RotateTokenResult {
  nodeId: number
  tokenPrefix: string
  token: string
}

export interface InstallScriptResult {
  nodeId: number
  script: string
}

export interface RemoteGeoUpdateResult {
  nodeId: number
  pending: boolean
  geoipSize: number
  geositeSize: number
  durationMs: number
  updatedAt: number
  error: string
}

export function getNodeSyncStatus(
  node?: Pick<
    NodeItem,
    'lastSyncError' | 'lastConfigHash' | 'expectedConfigHash' | 'configSyncStatus'
  > | null
): NodeSyncStatus {
  if (!node) return 'pending'
  if (
    node.configSyncStatus === 'synced' ||
    node.configSyncStatus === 'pending' ||
    node.configSyncStatus === 'error'
  ) {
    return node.configSyncStatus
  }
  if ((node.lastSyncError || '').trim() !== '') {
    return 'error'
  }
  if (
    node.lastConfigHash &&
    node.expectedConfigHash &&
    node.lastConfigHash === node.expectedConfigHash
  ) {
    return 'synced'
  }
  return 'pending'
}

export function formatShortHash(hash?: string): string {
  const trimmed = (hash || '').trim()
  return trimmed ? trimmed.slice(0, 8) : '-'
}

export const listNodes = async (): Promise<NodeItem[]> => {
  const res = await api.get<NodeItem[]>('/admin/nodes')
  return Array.isArray(res) ? res : []
}

export const getNode = async (id: number): Promise<NodeItem> => {
  return api.get<NodeItem>(`/admin/nodes/${id}`)
}

export const createNode = async (payload: CreateNodePayload): Promise<CreateNodeResult> => {
  return api.post<CreateNodeResult>('/admin/nodes', payload)
}

export const updateNode = async (id: number, payload: UpdateNodePayload): Promise<NodeItem> => {
  return api.put<NodeItem>(`/admin/nodes/${id}`, payload)
}

export const deleteNode = async (id: number): Promise<{ message: string }> => {
  return api.delete<{ message: string }>(`/admin/nodes/${id}`)
}

export const rotateNodeToken = async (id: number): Promise<RotateTokenResult> => {
  return api.post<RotateTokenResult>(`/admin/nodes/${id}/rotate-token`)
}

export const resetNodeTraffic = async (id: number): Promise<NodeItem> => {
  return api.post<NodeItem>(`/admin/nodes/${id}/reset-traffic`)
}

export const getNodeInstallScript = async (
  id: number,
  masterUrl?: string
): Promise<InstallScriptResult> => {
  const trimmed = (masterUrl || '').trim()
  const query = trimmed ? `?masterUrl=${encodeURIComponent(trimmed)}` : ''
  return api.get<InstallScriptResult>(`/admin/nodes/${id}/install-script${query}`)
}
