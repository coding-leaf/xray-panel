export type NodeType = 'inbound' | 'channel' | 'outbound'
export type ChannelCategory = 'subroute' | 'scoped_rule' | 'global_rule' | 'fallback'
export type LinkStatus = 'active' | 'paused' | 'blocked'

export interface TopologyNodeMetrics {
  upBytes?: number
  downBytes?: number
  clientCount?: number
  referenceCount?: number
  isIdle?: boolean
}

export interface TopologyNode {
  id: string
  type: NodeType
  category?: ChannelCategory
  title: string
  subtitle?: string
  tag: string
  protocol: string
  status: 'online' | 'offline' | 'paused' | 'normal' | 'blocked'
  isAlive?: boolean
  latencyMs?: number
  listenPort?: number
  externalPort?: number
  routeId?: number
  gatewayTag?: string
  outboundTag?: string
  inboundId?: number
  metrics?: TopologyNodeMetrics
  raw: any
}

export interface TopologyEdge {
  id: string
  sourceId: string
  targetId: string
  status: LinkStatus
  category: ChannelCategory
  label?: string
}

export interface TopologyGraph {
  inboundNodes: TopologyNode[]
  channelNodes: TopologyNode[]
  outboundNodes: TopologyNode[]
  edges: TopologyEdge[]
  upstreamMap: Map<string, Set<string>>
  downstreamMap: Map<string, Set<string>>
  nodeEdgesMap: Map<string, Set<string>>
}

export interface HoverHighlightState {
  hoveredNodeId: string | null
  hoveredEdgeId: string | null
  activeNodeIds: Set<string>
  activeEdgeIds: Set<string>
}

export interface ComputedBezierEdge {
  edge: TopologyEdge
  pathD: string
  sourceX: number
  sourceY: number
  targetX: number
  targetY: number
  status: LinkStatus
  category: ChannelCategory
  isActive: boolean
  isDimmed: boolean
}

export interface StreamPipelineUnit {
  id: string
  name: string
  routeId?: number
  category: ChannelCategory
  status: LinkStatus
  inbound: TopologyNode
  channel: TopologyNode
  outbound: TopologyNode
}
