import { ref, computed, type Ref } from 'vue'
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
    parts.push(
      `域名：${list.slice(0, 2).join(', ')}${list.length > 2 ? ` (+${list.length - 2})` : ''}`
    )
  }
  if (rule.ip && Array.isArray(rule.ip) && rule.ip.length > 0) {
    const list = rule.ip.map((i: string) => i.replace(/^geoip:/, ''))
    parts.push(
      `IP：${list.slice(0, 2).join(', ')}${list.length > 2 ? ` (+${list.length - 2})` : ''}`
    )
  }
  if (rule.port) {
    parts.push(`端口：${rule.port}`)
  }
  if (rule.protocol && Array.isArray(rule.protocol) && rule.protocol.length > 0) {
    parts.push(`协议：${rule.protocol.join(',')}`)
  }
  if (rule.network) {
    parts.push(`网络：${rule.network}`)
  }
  return parts.length > 0 ? parts.join(' | ') : '默认规则'
}

export function useTopologyGraph(
  inbounds: Ref<any[]>,
  outbounds: Ref<any[]>,
  routing: Ref<any>,
  users: Ref<any[]>,
  showGlobalRulesRef?: Ref<boolean>
) {
  const innerShowGlobalRules = showGlobalRulesRef || ref(false)
  const hiddenGlobalGroupsCount = ref(0)
  const totalGlobalGroupsCount = ref(0)

  const graph = computed<TopologyGraph>(() => {
    // 1. Thoroughly filter inbounds: remove internal 'api'
    const inbList = (inbounds.value || []).filter((inb) => inb && inb.tag !== 'api')
    const obList = outbounds.value || []

    // 2. Filter routing rules: exclude 'api' outbound and SubRoute shadow rules (vlessRoute)
    const rawRules = routing.value?.rules || []
    const routingRules = rawRules.filter((r: any) => r && r.outboundTag !== 'api' && !r.vlessRoute)
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

      let status: TopologyNode['status']
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
      label?: string
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

    // Path 1: VLESS SubRoute (Smart aggregation of multi-inbound identical routeId & outboundTag)
    interface SubRouteGroupItem {
      inb: any
      sr: any
    }
    const subRouteGroupMap = new Map<string, SubRouteGroupItem[]>()
    const subRouteTargetTags = new Set<string>()

    for (const inb of inbList) {
      const subRoutes: any[] = inb.subRoutes || []
      for (const sr of subRoutes) {
        const targetOutTag = sr.outboundTag || 'direct'
        subRouteTargetTags.add(targetOutTag)
        const key = `${sr.routeId}__${targetOutTag}`
        if (!subRouteGroupMap.has(key)) {
          subRouteGroupMap.set(key, [])
        }
        subRouteGroupMap.get(key)!.push({ inb, sr })
      }
    }

    for (const [, items] of subRouteGroupMap.entries()) {
      const first = items[0]
      const routeId = first.sr.routeId
      const targetOutTag = first.sr.outboundTag || 'direct'
      const chId = `ch-sr-${routeId}-${targetOutTag}`
      const targetOutId = `out-${targetOutTag}`
      const isBlocked = targetOutTag === 'block' || targetOutTag === 'blackhole'

      // Always count references for each bound subroute
      refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + items.length)

      // Clean trailing _v6, _v4 suffix (e.g. 日本直出_v6 & 日本直出 -> 日本直出)
      let rawName = first.sr.name || `线路 #${routeId}`
      for (const item of items) {
        if (item.sr.name && !item.sr.name.match(/[_ -]?(v6|v4)$/i)) {
          rawName = item.sr.name
          break
        }
      }
      const cleanedTitle = rawName.replace(/[_ -]?(v6|v4)$/i, '').trim() || `线路 #${routeId}`

      const boundRoutes = items.map((item) => ({
        inboundId: item.inb.id,
        gatewayTag: item.inb.tag,
        subRoute: item.sr,
      }))
      const gatewayTagsStr = items.map((item) => item.inb.tag).join(', ')

      const isAnyActive = items.some(
        (item) => item.sr.enabled !== false && item.inb.enabled !== false
      )
      const channelStatus: TopologyNode['status'] = !isAnyActive
        ? 'paused'
        : isBlocked
          ? 'blocked'
          : 'normal'

      let totalClientCount = 0
      for (const item of items) {
        if (
          item.sr.allowedUsers &&
          Array.isArray(item.sr.allowedUsers) &&
          item.sr.allowedUsers.length > 0
        ) {
          totalClientCount += item.sr.allowedUsers.length
        } else {
          const inbNodeId = `inb-${item.inb.id || item.inb.tag}`
          const matched = inboundNodes.find((n) => n.id === inbNodeId)
          totalClientCount += matched?.metrics?.clientCount || 0
        }
      }

      channelNodes.push({
        id: chId,
        type: 'channel',
        category: 'subroute',
        title: cleanedTitle,
        tag: targetOutTag,
        protocol: 'subroute',
        status: channelStatus,
        routeId,
        gatewayTag: gatewayTagsStr,
        outboundTag: targetOutTag,
        inboundId: first.inb.id,
        metrics: {
          clientCount: totalClientCount,
        },
        raw: {
          ...first.sr,
          name: cleanedTitle,
          boundRoutes,
          inboundId: first.inb.id,
          gatewayTag: gatewayTagsStr,
        },
      })

      // Inbound -> Channel edges
      for (const item of items) {
        const inbNodeId = `inb-${item.inb.id || item.inb.tag}`
        const isItemEnabled = item.sr.enabled !== false && item.inb.enabled !== false
        const linkStatus: LinkStatus = !isItemEnabled ? 'paused' : isBlocked ? 'blocked' : 'active'
        addEdge(inbNodeId, chId, linkStatus, 'subroute', `Route #${routeId}`)
      }

      // Channel -> Outbound edge
      const outLinkStatus: LinkStatus = !isAnyActive ? 'paused' : isBlocked ? 'blocked' : 'active'
      addEdge(chId, targetOutId, outLinkStatus, 'subroute', targetOutTag)
    }

    // Path 2: Routing rules (Scoped rules and Global rules)
    const inboundsWithWholeSiteRule = new Set<string>()
    const globalRulesList: any[] = []

    routingRules.forEach((rule: any, idx: number) => {
      const targetOutTag = rule.outboundTag || 'direct'
      const targetOutId = `out-${targetOutTag}`
      const isBlocked = targetOutTag === 'block' || targetOutTag === 'blackhole'

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
        const hasSpecificConditions =
          (rule.domain && Array.isArray(rule.domain) && rule.domain.length > 0) ||
          (rule.ip && Array.isArray(rule.ip) && rule.ip.length > 0) ||
          !!rule.port ||
          (rule.protocol && Array.isArray(rule.protocol) && rule.protocol.length > 0) ||
          !!rule.network

        for (const inTag of ruleInboundTags) {
          const matchedInb = inbList.find((i) => i.tag === inTag)
          if (matchedInb) {
            if (!hasSpecificConditions) {
              inboundsWithWholeSiteRule.add(inTag)
            }

            const inbNodeId = `inb-${matchedInb.id || matchedInb.tag}`
            const chId = `ch-rule-scoped-${matchedInb.tag}-${idx}`
            const linkStatus: LinkStatus =
              matchedInb.enabled === false ? 'paused' : isBlocked ? 'blocked' : 'active'

            refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + 1)

            const ruleTitle = !hasSpecificConditions
              ? `${inTag} 全流量直通`
              : formatRuleSummary(rule)

            channelNodes.push({
              id: chId,
              type: 'channel',
              category: 'scoped_rule',
              title: ruleTitle,
              subtitle: !hasSpecificConditions
                ? `专属直通 ➔ ${targetOutTag}`
                : `入站 [${inTag}] 专属规则`,
              tag: targetOutTag,
              protocol: 'rule',
              status: matchedInb.enabled === false ? 'paused' : isBlocked ? 'blocked' : 'normal',
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
        // Global Rule (collect for aggregated grouping)
        globalRulesList.push(rule)
      }
    })

    // Path 2b: Aggregate Global Rules by targetOutTag
    const globalRulesByTarget = new Map<string, any[]>()
    for (const rule of globalRulesList) {
      const targetOutTag = rule.outboundTag || 'direct'
      refCountMap.set(targetOutTag, (refCountMap.get(targetOutTag) || 0) + 1)
      if (!globalRulesByTarget.has(targetOutTag)) {
        globalRulesByTarget.set(targetOutTag, [])
      }
      globalRulesByTarget.get(targetOutTag)!.push(rule)
    }

    let hiddenCount = 0
    let totalCount = 0

    for (const [targetOutTag, groupRules] of globalRulesByTarget.entries()) {
      totalCount++
      const isSystem =
        targetOutTag === 'direct' || targetOutTag === 'block' || targetOutTag === 'blackhole'
      const isReferencedBySubRoute = subRouteTargetTags.has(targetOutTag)
      // Auto-retain 1 aggregated capsule for non-system outbounds (e.g. warp-out) not referenced by SubRoute
      const isAutoRetained = !isSystem && !isReferencedBySubRoute
      const shouldDisplay = innerShowGlobalRules.value || isAutoRetained

      if (!shouldDisplay) {
        hiddenCount++
        continue
      }

      const isBlocked = targetOutTag === 'block' || targetOutTag === 'blackhole'
      const targetOutId = `out-${targetOutTag}`
      const chId = `ch-global-agg-${targetOutTag}`
      const linkStatus: LinkStatus = isBlocked ? 'blocked' : 'active'

      // Summarize conditions
      const tagsSet = new Set<string>()
      for (const r of groupRules) {
        if (Array.isArray(r.domain)) {
          r.domain.forEach((d: string) => tagsSet.add(d.replace(/^(geosite:|domain:)/, '')))
        }
        if (Array.isArray(r.ip)) {
          r.ip.forEach((i: string) => tagsSet.add(i.replace(/^geoip:/, '')))
        }
        if (r.port) tagsSet.add(String(r.port))
        if (Array.isArray(r.protocol)) {
          r.protocol.forEach((p: string) => tagsSet.add(p))
        }
      }
      const tagsSummary = Array.from(tagsSet).slice(0, 3).join(', ')

      let title: string
      let subtitle: string
      if (isBlocked) {
        title = `全局拦截`
        subtitle = `${groupRules.length}条拦截规则${tagsSummary ? ': ' + tagsSummary : ''}`
      } else if (!isSystem) {
        const hasCn = tagsSet.has('cn')
        title = hasCn ? `CN 分流策略` : `全局分流`
        subtitle = `${groupRules.length}条规则${tagsSummary ? ': ' + tagsSummary : ''}`
      } else {
        title = `全局直连`
        subtitle = `${groupRules.length}条规则${tagsSummary ? ': ' + tagsSummary : ''}`
      }

      channelNodes.push({
        id: chId,
        type: 'channel',
        category: 'global_rule',
        title,
        subtitle,
        tag: targetOutTag,
        protocol: 'global-rule',
        status: isBlocked ? 'blocked' : 'normal',
        outboundTag: targetOutTag,
        metrics: {
          referenceCount: groupRules.length,
        },
        raw: { rules: groupRules, targetOutTag },
      })

      const nonSubInbounds = inbList.filter((i) => !i.subRoutes || i.subRoutes.length === 0)
      const relevantInbounds = nonSubInbounds.length > 0 ? nonSubInbounds : inbList.slice(0, 2)
      for (const inb of relevantInbounds) {
        const inbNodeId = `inb-${inb.id || inb.tag}`
        addEdge(inbNodeId, chId, linkStatus, 'global_rule', '全局规则')
      }
      addEdge(chId, targetOutId, linkStatus, 'global_rule', targetOutTag)
    }

    hiddenGlobalGroupsCount.value = hiddenCount
    totalGlobalGroupsCount.value = totalCount

    // Path 3: Default Fallback for Inbounds without subRoutes AND without whole-site scoped rules
    for (const inb of inbList) {
      const srs: any[] = inb.subRoutes || []
      if (srs.length === 0 && !inboundsWithWholeSiteRule.has(inb.tag)) {
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
          title: inb.remark || `${inb.tag} 直通出站`,
          subtitle: `缺省直通`,
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
      const isSystemReserved = tag === 'direct' || tag === 'block' || tag === 'blackhole'
      const isIdle = !isSystemReserved && refCount === 0
      const isBlocked = ob.protocol === 'blackhole' || tag === 'block' || tag === 'blackhole'

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
    targetEdgeId: string | null
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
    showGlobalRules: innerShowGlobalRules,
    hiddenGlobalGroupsCount,
    totalGlobalGroupsCount,
  }
}
