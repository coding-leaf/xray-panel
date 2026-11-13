import { ref, watch, onMounted, onUnmounted, nextTick, type Ref } from 'vue'
import type { TopologyEdge, HoverHighlightState, ComputedBezierEdge } from '../types'

export function useSvgBezier(
  containerRef: Ref<HTMLElement | null>,
  edges: Ref<TopologyEdge[]>,
  highlightState: Ref<HoverHighlightState>
) {
  const computedEdges = ref<ComputedBezierEdge[]>([])
  let rafId: number | null = null
  let resizeObserver: ResizeObserver | null = null

  const calculateEdges = () => {
    const container = containerRef.value
    if (!container) {
      computedEdges.value = []
      return
    }

    const containerRect = container.getBoundingClientRect()
    // If container is hidden or detached
    if (containerRect.width === 0 || containerRect.height === 0) {
      return
    }

    const hasHighlight =
      highlightState.value.activeNodeIds.size > 0 || highlightState.value.activeEdgeIds.size > 0
    const activeEdgeIds = highlightState.value.activeEdgeIds

    const results: ComputedBezierEdge[] = []

    for (const edge of edges.value) {
      // Find source right anchor
      let sourceX: number
      let sourceY: number
      const sourceAnchor = container.querySelector(
        `[data-node-id="${edge.sourceId}"] [data-anchor="right"]`
      ) as HTMLElement | null
      const sourceCard = container.querySelector(
        `[data-node-id="${edge.sourceId}"]`
      ) as HTMLElement | null

      if (sourceAnchor) {
        const r = sourceAnchor.getBoundingClientRect()
        sourceX = r.left + r.width / 2 - containerRect.left
        sourceY = r.top + r.height / 2 - containerRect.top
      } else if (sourceCard) {
        const r = sourceCard.getBoundingClientRect()
        sourceX = r.right - containerRect.left
        sourceY = r.top + r.height / 2 - containerRect.top
      } else {
        continue
      }

      // Find target left anchor
      let targetX: number
      let targetY: number
      const targetAnchor = container.querySelector(
        `[data-node-id="${edge.targetId}"] [data-anchor="left"]`
      ) as HTMLElement | null
      const targetCard = container.querySelector(
        `[data-node-id="${edge.targetId}"]`
      ) as HTMLElement | null

      if (targetAnchor) {
        const r = targetAnchor.getBoundingClientRect()
        targetX = r.left + r.width / 2 - containerRect.left
        targetY = r.top + r.height / 2 - containerRect.top
      } else if (targetCard) {
        const r = targetCard.getBoundingClientRect()
        targetX = r.left - containerRect.left
        targetY = r.top + r.height / 2 - containerRect.top
      } else {
        continue
      }

      // Cubic Bezier calculation
      const dx = Math.max(targetX - sourceX, 40)
      const cx1 = sourceX + dx * 0.48
      const cy1 = sourceY
      const cx2 = targetX - dx * 0.48
      const cy2 = targetY
      const pathD = `M ${sourceX.toFixed(1)} ${sourceY.toFixed(1)} C ${cx1.toFixed(1)} ${cy1.toFixed(1)}, ${cx2.toFixed(1)} ${cy2.toFixed(1)}, ${targetX.toFixed(1)} ${targetY.toFixed(1)}`

      const isActive = activeEdgeIds.has(edge.id)
      const isDimmed = hasHighlight && !isActive

      results.push({
        edge,
        pathD,
        sourceX,
        sourceY,
        targetX,
        targetY,
        status: edge.status,
        category: edge.category,
        isActive,
        isDimmed,
      })
    }

    computedEdges.value = results
  }

  const scheduleUpdate = () => {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
    }
    rafId = requestAnimationFrame(() => {
      calculateEdges()
      rafId = null
    })
  }

  // Trigger recalculation on edges or highlight updates
  watch(
    () => edges.value,
    () => {
      nextTick(scheduleUpdate)
    },
    { deep: true }
  )

  watch(
    () => highlightState.value,
    () => {
      // Fast path: just update isActive and isDimmed without re-querying DOM if paths exist
      if (computedEdges.value.length > 0) {
        const hasHighlight =
          highlightState.value.activeNodeIds.size > 0 || highlightState.value.activeEdgeIds.size > 0
        const activeEdgeIds = highlightState.value.activeEdgeIds

        computedEdges.value = computedEdges.value.map((ce) => {
          const isActive = activeEdgeIds.has(ce.edge.id)
          const isDimmed = hasHighlight && !isActive
          return {
            ...ce,
            isActive,
            isDimmed,
          }
        })
      } else {
        scheduleUpdate()
      }
    },
    { deep: true }
  )

  onMounted(() => {
    nextTick(() => {
      scheduleUpdate()

      if (containerRef.value && typeof ResizeObserver !== 'undefined') {
        resizeObserver = new ResizeObserver(() => {
          scheduleUpdate()
        })
        resizeObserver.observe(containerRef.value)
      }

      window.addEventListener('resize', scheduleUpdate, { passive: true })
      window.addEventListener('scroll', scheduleUpdate, { passive: true, capture: true })
    })
  })

  onUnmounted(() => {
    if (rafId !== null) {
      cancelAnimationFrame(rafId)
      rafId = null
    }
    if (resizeObserver) {
      resizeObserver.disconnect()
      resizeObserver = null
    }
    window.removeEventListener('resize', scheduleUpdate)
    window.removeEventListener('scroll', scheduleUpdate, true)
  })

  return {
    computedEdges,
    scheduleUpdate,
  }
}
