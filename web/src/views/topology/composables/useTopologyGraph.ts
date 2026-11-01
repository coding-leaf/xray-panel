import { computed, type Ref } from 'vue'
import type {
  TopologyGraph,
  TopologyNode,
  TopologyEdge,
  HoverHighlightState,
  StreamPipelineUnit,
  ChannelCategory,
  LinkStatus,
} from '../types'

export function formatRuleSummary(rule: any): string {
  if (!rule) return '分流规则'
  const parts: string[] = []
  if (rule.domain && Array.isArray(rule.domain) && rule.domain.length > 0) {
    const list = rule.domain.map((d: string) => d.replace(/^(geosite:|domain:)/, ''))
    parts.push(`域名: ${list.slice(0, 2).join(', ')}${list.length > 2 ? ` (+${list.length - 2})` : ''}`)
  }
  if (rule.ip && Array.isArray(rule.ip) && rule.ip.length > 0) {
    const list = rule.ip.map((i: string) => i.replace(/^geoip:/, ''))
    parts.push(`IP: ${list.slice(0, 2).join(', ')}${list.length > 2 ? ` (+${list.length - 2})` : ''}`)
  }
  if (rule.port) {
    parts.push(`端口: ${rule.port}`)
  }
  if (rule.protocol && Array.isArray(rule.protocol) && rule.protocol.length > 0) {
    parts.push(`协议: ${rule.protocol.join(',')}`)
  }
  if (rule.network) {
    parts.push(`网络: ${rule.network}`)
  }
  return parts.length > 0 ? parts.join(' | ') : '默认规则'
}

export function useTopologyGraph(
  inbounds: Ref<any[]>,
  outbounds: Ref<any[]>,
  routing: Ref<any>,
  users: Ref<any[]>,
) {
  const graph = computed<TopologyGraph>(() => {
    const inbList = inbounds.value || []
    const obList = outbounds.value || []
    const routingRules = (routing.value?.rules || []).filter((r: any) => r && r.outboundTag !== 'api')
    const userList = users.value || []

    const refCountMap = new Map<string, number>()
    for (const ob of obList) {
      if (ob && ob.tag) {
        refCountMap.set(ob.tag, 0)
      }
    }

    // 1. Inbound Nodes
    const inboundNodes: TopologyNode[] = inbList.map((inb) => {
      let clientCount = 0
      for (const u of userList) {
        if (!u) continue
        const tags = (u.inboundTags || '').split(',').map((t: string) => t.trim())
        if (tags.includes(inb.tag) || u.inboundTag === inb.tag) {
          clientCount++
        }
      }

      let status: TopologyNode['status'] = 'normal'
      if (inb.enabled === false) {
        status = 'paused'
      } else if (inb.isAlive === false) {
        status = 'offline'
      } else {
        status = 'online'
      }

      return {
        id: `inb-${inb.id || inb.tag}`,
        type: 'inbound',
        title: inb.tag,
        tag: inb.tag,
        protocol: (inb.protocol || 'vless').toLowerCase(),
        status,
        isAlive: inb.isAlive !== false,
        latencyMs: inb.latencyMs ?? 1,
        listenPort: inb.port,
        externalPort: inb.externalPort || inb.port,
        inboundId: inb.id,
        metrics: {
          upBytes: inb.upBytes || 0,
          downBytes: inb.downBytes || 0,
          clientCount,
        },
        raw: inb,
      }
    })

    const channelNodes: TopologyNode[] = []
    const edges: TopologyEdge[] = []
    const edgeIdSet = new Set<string>()

    const addEdge = (
      sourceId: string,
      targetId: string,
      status: LinkStatus,
      category: ChannelCategory,
      label?: string,
    ) => {
      const edgeId = `e-${sourceId}-${targetId}`
      if (edgeIdSet.has(edgeId)) return
      edgeIdSet.add(edgeId)
      edges.push({
        id: edgeId,
        sourceId,
        targetId,
        status,
        category,
        label,
      })
    }

    // Path 1: VLESS SubRoute (Scoped SubRoute 线路)
    for (const inb of inbList) {
      const inbNodeId = `inb-${inb.id || inb.tag}`
      const matchedInbNode = inboundNodes.find((n) => n.id === inbNodeId)
      const inbClientCount = matchedInbNode?.metrics?.clientCount || 0
      const subRoutes: any[] = inb.subRoutes || []
      if (subRoutes.length > 0) {
        for (const sr of subRoutes) {
          const chId = `ch-${inb.tag}-sr-${sr.routeId}`
          const targetOutTag = sr.outboundTag || 'direct'
          const targetOutId = `out-${targetOutTag}`
          const isEnabled = sr.enabled !== false && inb.enabled !== false
          const isBlocked = targetOutTag === 'block' || targetOutTag === 'blackhole'
          const linkStatus: LinkStatus = !isEnabled ? 'paused' : (isBlocked ? 'blocked' : 'active')

          refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + 1)

          const routeClientCount =
            sr.allowedUsers && Array.isArray(sr.allowedUsers) && sr.allowedUsers.length > 0
              ? sr.allowedUsers.length
              : inbClientCount

          channelNodes.push({
            id: chId,
            type: 'channel',
            category: 'subroute',
            title: sr.name || `线路 #${sr.routeId}`,
            tag: targetOutTag,
            protocol: 'subroute',
            status: !isEnabled ? 'paused' : (isBlocked ? 'blocked' : 'normal'),
            routeId: sr.routeId,
            gatewayTag: inb.tag,
            outboundTag: targetOutTag,
            inboundId: inb.id,
            metrics: {
              clientCount: routeClientCount,
            },
            raw: { ...sr, inboundId: inb.id, gatewayTag: inb.tag },
          })

          addEdge(inbNodeId, chId, linkStatus, 'subroute', `Route #${sr.routeId}`)
          addEdge(chId, targetOutId, linkStatus, 'subroute', targetOutTag)
        }
      }
    }

    // Path 2: Routing rules
    routingRules.forEach((rule: any, idx: number) => {
      const targetOutTag = rule.outboundTag || 'direct'
      const targetOutId = `out-${targetOutTag}`
      const isBlocked = targetOutTag === 'block' || targetOutTag === 'blackhole'

      // Check for scoped inboundTag
      let ruleInboundTags: string[] = []
      if (rule.inboundTag) {
        if (Array.isArray(rule.inboundTag)) {
          ruleInboundTags = rule.inboundTag
        } else if (typeof rule.inboundTag === 'string') {
          ruleInboundTags = rule.inboundTag.split(',').map((s: string) => s.trim())
        }
      }

      if (ruleInboundTags.length > 0) {
        // Scoped Rule
        for (const inTag of ruleInboundTags) {
          const matchedInb = inbList.find((i) => i.tag === inTag)
          if (matchedInb) {
            const inbNodeId = `inb-${matchedInb.id || matchedInb.tag}`
            const chId = `ch-rule-scoped-${matchedInb.tag}-${idx}`
            const linkStatus: LinkStatus = matchedInb.enabled === false ? 'paused' : (isBlocked ? 'blocked' : 'active')

            refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + 1)

            channelNodes.push({
              id: chId,
              type: 'channel',
              category: 'scoped_rule',
              title: formatRuleSummary(rule),
              subtitle: `入站 [${inTag}] 专属规则`,
              tag: targetOutTag,
              protocol: 'rule',
              status: matchedInb.enabled === false ? 'paused' : (isBlocked ? 'blocked' : 'normal'),
              gatewayTag: inTag,
              outboundTag: targetOutTag,
              inboundId: matchedInb.id,
              raw: rule,
            })

            addEdge(inbNodeId, chId, linkStatus, 'scoped_rule', `专属规则`)
            addEdge(chId, targetOutId, linkStatus, 'scoped_rule', targetOutTag)
          }
        }
      } else {
        // Global Rule (e.g. adblock / WARP / geoip routing)
        // Connect to inbounds that don't have subroutes or general non-subroute inbounds
        const nonSubInbounds = inbList.filter((i) => !i.subRoutes || i.subRoutes.length === 0)
        const relevantInbounds = nonSubInbounds.length > 0 ? nonSubInbounds : inbList.slice(0, 2)

        if (relevantInbounds.length > 0) {
          const chId = `ch-rule-global-${idx}`
          const linkStatus: LinkStatus = isBlocked ? 'blocked' : 'active'

          refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + 1)

          channelNodes.push({
            id: chId,
            type: 'channel',
            category: 'global_rule',
            title: formatRuleSummary(rule),
            subtitle: `全局分流策略`,
            tag: targetOutTag,
            protocol: 'global-rule',
            status: isBlocked ? 'blocked' : 'normal',
            outboundTag: targetOutTag,
            raw: rule,
          })

          for (const inb of relevantInbounds) {
            const inbNodeId = `inb-${inb.id || inb.tag}`
            addEdge(inbNodeId, chId, linkStatus, 'global_rule', '全局规则')
          }
          addEdge(chId, targetOutId, linkStatus, 'global_rule', targetOutTag)
        }
      }
    })

    // Path 3: Default Fallback for Inbounds without subRoutes (ensures zero isolated inbounds)
    for (const inb of inbList) {
      const srs: any[] = inb.subRoutes || []
      if (srs.length === 0) {
        const inbNodeId = `inb-${inb.id || inb.tag}`
        const chId = `ch-${inb.tag}-fallback`
        const targetOutTag = obList[0]?.tag || 'direct'
        const targetOutId = `out-${targetOutTag}`
        const isEnabled = inb.enabled !== false
        const linkStatus: LinkStatus = !isEnabled ? 'paused' : 'active'

        refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + 1)

        channelNodes.push({
          id: chId,
          type: 'channel',
          category: 'fallback',
          title: inb.remark || `${inb.tag} 直连出站`,
          subtitle: `缺省兜底直通`,
          tag: targetOutTag,
          protocol: 'direct',
          status: !isEnabled ? 'paused' : 'normal',
          routeId: inb.routeId || 0,
          gatewayTag: inb.tag,
          outboundTag: targetOutTag,
          inboundId: inb.id,
          raw: inb,
        })

        addEdge(inbNodeId, chId, linkStatus, 'fallback', '直通直出')
        addEdge(chId, targetOutId, linkStatus, 'fallback', targetOutTag)
      }
    }

    // 2. Outbound Nodes (with accurate referenceCount and isIdle)
    const outboundNodes: TopologyNode[] = obList.map((ob) => {
      const tag = ob.tag
      const refCount = refCountMap.get(tag) || 0
      const isSystemReserved = tag === 'direct' || tag === 'block'
      const isIdle = !isSystemReserved && refCount === 0
      const isBlocked = ob.protocol === 'blackhole' || tag === 'block'

      return {
        id: `out-${tag}`,
        type: 'outbound',
        title: tag,
        tag,
        protocol: (ob.protocol || 'freedom').toLowerCase(),
        status: isBlocked ? 'blocked' : 'normal',
        metrics: {
          referenceCount: refCount,
          isIdle,
        },
        raw: ob,
      }
    })

    // Build Adjacency Maps
    const upstreamMap = new Map<string, Set<string>>()
    const downstreamMap = new Map<string, Set<string>>()
    const nodeEdgesMap = new Map<string, Set<string>>()

    const allNodeIds = [
      ...inboundNodes.map((n) => n.id),
      ...channelNodes.map((n) => n.id),
      ...outboundNodes.map((n) => n.id),
    ]

    for (const nid of allNodeIds) {
      upstreamMap.set(nid, new Set())
      downstreamMap.set(nid, new Set())
      nodeEdgesMap.set(nid, new Set())
    }

    for (const edge of edges) {
      if (!upstreamMap.has(edge.targetId)) upstreamMap.set(edge.targetId, new Set())
      if (!downstreamMap.has(edge.sourceId)) downstreamMap.set(edge.sourceId, new Set())
      if (!nodeEdgesMap.has(edge.sourceId)) nodeEdgesMap.set(edge.sourceId, new Set())
      if (!nodeEdgesMap.has(edge.targetId)) nodeEdgesMap.set(edge.targetId, new Set())

      upstreamMap.get(edge.targetId)!.add(edge.sourceId)
      downstreamMap.get(edge.sourceId)!.add(edge.targetId)
      nodeEdgesMap.get(edge.sourceId)!.add(edge.id)
      nodeEdgesMap.get(edge.targetId)!.add(edge.id)
    }

    return {
      inboundNodes,
      channelNodes,
      outboundNodes,
      edges,
      upstreamMap,
      downstreamMap,
      nodeEdgesMap,
    }
  })

  // Streamlined Pipeline Units for Mobile / Responsive Stream
  const streamPipelines = computed<StreamPipelineUnit[]>(() => {
    const { inboundNodes, channelNodes, outboundNodes, edges } = graph.value
    const inbMap = new Map<string, TopologyNode>(inboundNodes.map((n) => [n.id, n]))
    const obMap = new Map<string, TopologyNode>(outboundNodes.map((n) => [n.id, n]))

    const units: StreamPipelineUnit[] = []

    for (const ch of channelNodes) {
      const inEdges = edges.filter((e) => e.targetId === ch.id)
      const outEdge = edges.find((e) => e.sourceId === ch.id)
      const outbound = outEdge ? obMap.get(outEdge.targetId) : undefined

      if (outbound) {
        for (const inEdge of inEdges) {
          const inbound = inbMap.get(inEdge.sourceId)
          if (inbound) {
            units.push({
              id: `unit-${ch.id}-${inbound.id}`,
              name: ch.title,
              routeId: ch.routeId,
              category: ch.category || 'subroute',
              status: inEdge.status,
              inbound,
              channel: ch,
              outbound,
            })
          }
        }
      }
    }

    return units
  })

  // Bidirectional Traversal for Hover Highlighting
  const computeActiveSubgraph = (
    targetNodeId: string | null,
    targetEdgeId: string | null,
  ): HoverHighlightState => {
    if (!targetNodeId && !targetEdgeId) {
      return {
        hoveredNodeId: null,
        hoveredEdgeId: null,
        activeNodeIds: new Set(),
        activeEdgeIds: new Set(),
      }
    }

    const { upstreamMap, downstreamMap, edges } = graph.value
    const activeNodeIds = new Set<string>()
    const activeEdgeIds = new Set<string>()

    const traverseUp = (startId: string) => {
      const queue = [startId]
      while (queue.length > 0) {
        const cur = queue.shift()!
        activeNodeIds.add(cur)
        const parents = upstreamMap.get(cur)
        if (parents) {
          for (const p of parents) {
            if (!activeNodeIds.has(p)) {
              activeNodeIds.add(p)
              queue.push(p)
            }
          }
        }
      }
    }

    const traverseDown = (startId: string) => {
      const queue = [startId]
      while (queue.length > 0) {
        const cur = queue.shift()!
        activeNodeIds.add(cur)
        const children = downstreamMap.get(cur)
        if (children) {
          for (const c of children) {
            if (!activeNodeIds.has(c)) {
              activeNodeIds.add(c)
              queue.push(c)
            }
          }
        }
      }
    }

    if (targetNodeId) {
      activeNodeIds.add(targetNodeId)
      traverseUp(targetNodeId)
      traverseDown(targetNodeId)
    } else if (targetEdgeId) {
      const e = edges.find((item) => item.id === targetEdgeId)
      if (e) {
        activeEdgeIds.add(e.id)
        traverseUp(e.sourceId)
        traverseDown(e.targetId)
      }
    }

    // Include all edges between active nodes
    for (const e of edges) {
      if (activeNodeIds.has(e.sourceId) && activeNodeIds.has(e.targetId)) {
        activeEdgeIds.add(e.id)
      }
    }

    return {
      hoveredNodeId: targetNodeId,
      hoveredEdgeId: targetEdgeId,
      activeNodeIds,
      activeEdgeIds,
    }
  }

  return {
    graph,
    streamPipelines,
    computeActiveSubgraph,
  }
}
