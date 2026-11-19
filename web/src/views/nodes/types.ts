import type { NodeDesiredState, NodeItem } from '../../api/nodes'

export type { NodeDesiredState, NodeItem }

export interface NodeFormData {
  id: number
  name: string
  host: string
  region: string
  enabled: boolean
  desiredState: NodeDesiredState
  remark: string
}

export interface NodeTokenModalState {
  nodeId: number
  nodeName: string
  token?: string
  tokenPrefix?: string
  mode: 'created' | 'rotated' | 'install'
}

export const NODE_DESIRED_STATE_OPTIONS: Array<{
  value: NodeDesiredState
  label: string
  description: string
}> = [
  {
    value: 'active',
    label: 'active (正常分发)',
    description: '纳入用户订阅并正常同步配置',
  },
  {
    value: 'draining',
    label: 'draining (排空停发)',
    description: '保留运行中的连接，但从新拉取的订阅中摘除',
  },
  {
    value: 'circuit_broken',
    label: 'circuit_broken (熔断隔离)',
    description: '异常隔离状态，停止从订阅下发',
  },
]

export function getDefaultNodeFormData(): NodeFormData {
  return {
    id: 0,
    name: '',
    host: '',
    region: '',
    enabled: true,
    desiredState: 'active',
    remark: '',
  }
}
