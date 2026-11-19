import api from './index'

export type NodeDesiredState = 'active' | 'draining' | 'circuit_broken'

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
  lastUsersHash: string
  lastSyncError: string
  cpuPercent: number
  memPercent: number
  nicUpBytes: number
  nicDownBytes: number
  xrayUpBytes: number
  xrayDownBytes: number
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

export const getNodeInstallScript = async (
  id: number,
  masterUrl?: string
): Promise<InstallScriptResult> => {
  const trimmed = (masterUrl || '').trim()
  const query = trimmed ? `?masterUrl=${encodeURIComponent(trimmed)}` : ''
  return api.get<InstallScriptResult>(`/admin/nodes/${id}/install-script${query}`)
}
