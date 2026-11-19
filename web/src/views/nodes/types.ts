import type { NodeDesiredState, NodeItem, NodeTrafficCountMode } from '../../api/nodes'

export type { NodeDesiredState, NodeItem, NodeTrafficCountMode }

export interface NodeFormData {
  id: number
  name: string
  host: string
  region: string
  enabled: boolean
  desiredState: NodeDesiredState
  trafficLimitGB: number
  billingResetDay: number
  trafficCountMode: NodeTrafficCountMode
  trafficMultiplier: number
  warnThresholdPct: number
  drainThresholdPct: number
  breakThresholdPct: number
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

export const NODE_TRAFFIC_COUNT_MODE_OPTIONS: Array<{
  value: NodeTrafficCountMode
  label: string
  description: string
}> = [
  {
    value: 'both',
    label: 'both (双向累加：上行 + 下行)',
    description: '标准双向计费，周期已用 = (上行 + 下行) × 安全系数',
  },
  {
    value: 'outbound',
    label: 'outbound (仅出站上行)',
    description: '仅统计出站上行流量，适用于单向计费机房',
  },
  {
    value: 'max',
    label: 'max (取上行 / 下行最大值)',
    description: '取上行与下行较大值，适用于双向独立峰值计费',
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
    trafficLimitGB: 0,
    billingResetDay: 0,
    trafficCountMode: 'both',
    trafficMultiplier: 1.0,
    warnThresholdPct: 80,
    drainThresholdPct: 90,
    breakThresholdPct: 95,
    remark: '',
  }
}
