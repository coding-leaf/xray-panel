<template>
  <Modal
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="`流量历史趋势 — ${user?.email || ''}`"
    description="每日聚合流量归档与可视化统计"
    size="lg"
  >
    <template #icon>
      <BarChart2 class="w-4 h-4 text-cyan-400" />
    </template>

    <div class="space-y-4">
      <!-- Time Range Selector & Metrics -->
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-1 bg-neutral-900 p-1 rounded-md border border-border">
          <button
            v-for="d in [7, 14, 30]"
            :key="d"
            @click="setHistoryDays(d)"
            class="px-2.5 py-1 rounded text-xs font-mono transition-colors"
            :class="historyDays === d ? 'bg-foreground text-background font-semibold' : 'text-muted-foreground hover:text-foreground'"
          >
            近 {{ d }} 天
          </button>
        </div>

        <div class="text-[11px] font-mono text-muted-foreground">
          区间总计: <span class="text-foreground font-bold">{{ formatBytes(historySummary.totalAll) }}</span>
        </div>
      </div>

      <!-- Metric Cards -->
      <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 font-mono">
        <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
          <span class="text-[10px] text-muted-foreground uppercase">总上行流量</span>
          <div class="text-xs sm:text-sm font-bold text-emerald-400 truncate">{{ formatBytes(historySummary.totalUp) }}</div>
        </div>
        <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
          <span class="text-[10px] text-muted-foreground uppercase">总下行流量</span>
          <div class="text-xs sm:text-sm font-bold text-cyan-400 truncate">{{ formatBytes(historySummary.totalDown) }}</div>
        </div>
        <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
          <span class="text-[10px] text-muted-foreground uppercase">单日峰值</span>
          <div class="text-xs sm:text-sm font-bold text-amber-400 truncate">{{ formatBytes(maxDayBytes) }}</div>
        </div>
        <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
          <span class="text-[10px] text-muted-foreground uppercase">日均消耗</span>
          <div class="text-xs sm:text-sm font-bold text-foreground truncate">{{ formatBytes(historySummary.avgDay) }}</div>
        </div>
      </div>

      <!-- Bar Chart Visualizer -->
      <div class="space-y-2">
        <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
          <span class="font-medium text-foreground">每日用量走势</span>
          <div class="flex items-center gap-3">
            <span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-emerald-500"></span> 上行</span>
            <span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-cyan-500"></span> 下行</span>
          </div>
        </div>

        <!-- Active log inspection strip -->
        <div class="min-h-[36px] px-3 py-1.5 bg-neutral-900 rounded-md border border-border flex flex-wrap items-center justify-between gap-2 text-xs font-mono">
          <div v-if="activeLog" class="flex flex-wrap items-center gap-x-3 gap-y-1 text-foreground">
            <span class="font-bold text-foreground">{{ activeLog.date }}</span>
            <span class="text-emerald-400">↑ {{ formatBytes(activeLog.upBytes) }}</span>
            <span class="text-cyan-400">↓ {{ formatBytes(activeLog.downBytes) }}</span>
            <span class="font-bold">总计: {{ formatBytes(activeLog.upBytes + activeLog.downBytes) }}</span>
          </div>
          <div v-else class="text-muted-foreground text-[11px]">
            悬停或点击柱形查看单日用量详情
          </div>
        </div>

        <!-- Bar visualizer -->
        <div v-if="sortedHistoryLogs.length" class="bg-card rounded-md border border-border p-3 overflow-x-auto pb-2">
          <div class="h-36 flex items-end gap-2 pt-2 min-w-full w-max">
            <div
              v-for="log in sortedHistoryLogs"
              :key="log.date"
              @mouseenter="hoveredLog = log"
              @mouseleave="hoveredLog = null"
              @click="toggleSelectLog(log)"
              class="flex-1 min-w-[32px] max-w-[48px] flex flex-col items-center gap-1.5 group relative h-full justify-end cursor-pointer select-none"
            >
              <div class="w-full flex-1 flex flex-col justify-end items-center relative">
                <div
                  class="w-full max-w-[20px] rounded-t-sm overflow-hidden flex flex-col justify-end bg-neutral-900 transition-all duration-200"
                  :class="{ 'ring-2 ring-foreground scale-105': activeLog?.date === log.date }"
                  :style="{ height: `${getTotalBarHeight(log)}%` }"
                >
                  <!-- Up Bytes (Emerald) -->
                  <div
                    v-if="log.upBytes > 0"
                    class="w-full bg-emerald-500 hover:bg-emerald-400 transition-all"
                    :style="{ height: `${getSegmentPercent(log.upBytes, log)}%` }"
                  />
                  <!-- Down Bytes (Cyan) -->
                  <div
                    v-if="log.downBytes > 0"
                    class="w-full bg-cyan-500 hover:bg-cyan-400 transition-all"
                    :style="{ height: `${getSegmentPercent(log.downBytes, log)}%` }"
                  />
                </div>
              </div>

              <!-- Date Label -->
              <span
                class="text-[10px] font-mono transition-colors truncate w-full text-center shrink-0"
                :class="activeLog?.date === log.date ? 'text-foreground font-bold' : 'text-muted-foreground group-hover:text-foreground'"
              >
                {{ log.date.substring(5) }}
              </span>
            </div>
          </div>
        </div>

        <div v-else class="text-center py-8 text-xs text-muted-foreground font-mono">
          暂无历史流量记录
        </div>
      </div>

      <!-- Table breakdown -->
      <div class="space-y-1.5">
        <div class="max-h-44 overflow-y-auto rounded-md border border-border bg-neutral-950">
          <table class="w-full text-left text-xs">
            <thead class="border-b border-border bg-neutral-900 sticky top-0">
              <tr class="font-mono text-[11px] text-muted-foreground">
                <th class="py-2 px-3">日期</th>
                <th class="py-2 px-3">上行 (Up)</th>
                <th class="py-2 px-3">下行 (Down)</th>
                <th class="py-2 px-3">单日总计</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/40 font-mono text-[11px]">
              <tr v-for="log in historyLogs" :key="log.id" class="hover:bg-muted/30">
                <td class="py-2 px-3 text-foreground font-medium">{{ log.date }}</td>
                <td class="py-2 px-3 text-emerald-400">{{ formatBytes(log.upBytes) }}</td>
                <td class="py-2 px-3 text-cyan-400">{{ formatBytes(log.downBytes) }}</td>
                <td class="py-2 px-3 text-foreground font-bold">{{ formatBytes(log.upBytes + log.downBytes) }}</td>
              </tr>
              <tr v-if="!historyLogs.length">
                <td colspan="4" class="text-center py-4 text-muted-foreground">暂无记录</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <template #footer>
      <Button variant="outline" size="sm" @click="emit('update:modelValue', false)">
        关闭
      </Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { BarChart2 } from 'lucide-vue-next'
import api from '../../../api'
import { formatBytes } from '../../../utils/format'
import type { UserItem, TrafficRecord } from '../types'

import Modal from '../../../components/ui/Modal.vue'
import Button from '../../../components/ui/Button.vue'

interface Props {
  modelValue: boolean
  user?: UserItem | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const historyLogs = ref<TrafficRecord[]>([])
const historyDays = ref(14)
const hoveredLog = ref<TrafficRecord | null>(null)
const selectedLog = ref<TrafficRecord | null>(null)
const activeLog = computed(() => hoveredLog.value || selectedLog.value)

const toggleSelectLog = (log: TrafficRecord) => {
  if (selectedLog.value?.date === log.date) {
    selectedLog.value = null
  } else {
    selectedLog.value = log
  }
}

const fetchUserHistory = async (userId: number, days: number) => {
  try {
    const res: any = await api.get(`/users/${userId}/traffic-history?days=${days}`)
    historyLogs.value = res || []
  } catch (err) {
    console.error(err)
    historyLogs.value = []
  }
}

const setHistoryDays = async (days: number) => {
  historyDays.value = days
  hoveredLog.value = null
  selectedLog.value = null
  if (props.user?.id) {
    await fetchUserHistory(props.user.id, days)
  }
}

watch(
  () => [props.modelValue, props.user?.id],
  async ([open, userId]) => {
    if (open && userId) {
      hoveredLog.value = null
      selectedLog.value = null
      historyDays.value = 14
      await fetchUserHistory(userId as number, 14)
    }
  }
)

const sortedHistoryLogs = computed(() => {
  return [...historyLogs.value].reverse()
})

const maxDayBytes = computed(() => {
  let max = 1
  for (const log of historyLogs.value) {
    const total = log.upBytes + log.downBytes
    if (total > max) max = total
  }
  return max
})

const getTotalBarHeight = (log: TrafficRecord) => {
  const total = (log.upBytes || 0) + (log.downBytes || 0)
  if (!total || maxDayBytes.value <= 0) return 4
  return Math.min(90, Math.max(6, (total / maxDayBytes.value) * 90))
}

const getSegmentPercent = (segmentBytes: number, log: TrafficRecord) => {
  const total = (log.upBytes || 0) + (log.downBytes || 0)
  if (!total || !segmentBytes) return 0
  return Math.round((segmentBytes / total) * 100)
}

const historySummary = computed(() => {
  let up = 0
  let down = 0
  for (const log of historyLogs.value) {
    up += log.upBytes || 0
    down += log.downBytes || 0
  }
  const all = up + down
  const count = historyLogs.value.length || 1
  return {
    totalUp: up,
    totalDown: down,
    totalAll: all,
    avgDay: Math.round(all / count),
  }
})
</script>
