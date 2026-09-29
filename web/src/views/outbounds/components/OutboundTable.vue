<template>
  <div class="space-y-4">
    <!-- Top Action & Filter Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">出站代理管理 (Outbounds)</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ filteredOutbounds.length }} 个节点
          </Badge>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20">
            修改自动重启核心生效
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          配置直连、黑洞拦截、Cloudflare WARP (WireGuard) 与链式上游代理，保存后自动落盘并平滑应用
        </p>
      </div>

      <!-- Action Buttons -->
      <div class="flex items-center gap-2">
        <Button
          variant="default"
          size="sm"
          @click="emit('create')"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加出站节点</span>
        </Button>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
      <div class="flex flex-1 items-center gap-2 max-w-md">
        <!-- Search Input -->
        <div class="relative w-full">
          <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-muted-foreground" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="搜索节点 Tag / 协议 / 目标地址..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <!-- Protocol Filter -->
        <select
          v-model="protocolFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部协议</option>
          <option value="freedom">freedom (直连)</option>
          <option value="wireguard">wireguard (WARP)</option>
          <option value="vless">vless</option>
          <option value="vmess">vmess</option>
          <option value="trojan">trojan</option>
          <option value="shadowsocks">shadowsocks</option>
          <option value="socks">socks</option>
          <option value="http">http</option>
          <option value="blackhole">blackhole (黑洞)</option>
        </select>
      </div>
    </div>

    <!-- Main Table View -->
    <div class="relative w-full overflow-hidden rounded-lg border border-border bg-neutral-950">
      <table class="w-full caption-bottom text-xs border-collapse">
        <thead class="border-b border-border bg-neutral-900">
          <tr>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">节点标识 (Tag)</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">出站协议</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">传输与安全</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">目标端点 / 地址</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">配置摘要</th>
            <th class="h-9 px-3.5 text-right align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr
            v-for="ob in filteredOutbounds"
            :key="ob.tag"
            @click="emit('inspect', ob)"
            class="border-b border-border/30 transition-colors hover:bg-muted/30 cursor-pointer"
            :class="{ 'bg-muted/20': selectedTag === ob.tag }"
          >
            <!-- 1. Tag & Description -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-2">
                <span class="font-mono font-semibold text-foreground text-xs">{{ ob.tag }}</span>
                <span v-if="ob.tag === 'direct' || ob.tag === 'block'" class="text-[10px] font-mono px-1 py-0.2 rounded bg-neutral-800 text-neutral-400 border border-neutral-700">
                  系统默认
                </span>
              </div>
              <div class="text-[11px] text-muted-foreground mt-0.5 truncate max-w-[200px]">
                {{ getUsageDesc(ob) }}
              </div>
            </td>

            <!-- 2. Protocol -->
            <td class="p-3.5 align-middle">
              <Badge :variant="getProtocolBadgeVariant(ob.protocol)">
                {{ ob.protocol?.toUpperCase() }}
              </Badge>
            </td>

            <!-- 3. Network & Security -->
            <td class="p-3.5 align-middle font-mono">
              <div v-if="getOutboundStreamInfo(ob).network || getOutboundStreamInfo(ob).security" class="flex items-center gap-1.5">
                <span v-if="getOutboundStreamInfo(ob).network" class="px-1.5 py-0.5 rounded text-[10px] bg-neutral-900 border border-border text-neutral-300">
                  {{ getOutboundStreamInfo(ob).network.toUpperCase() }}
                </span>
                <span v-if="getOutboundStreamInfo(ob).security && getOutboundStreamInfo(ob).security !== 'none'" class="px-1.5 py-0.5 rounded text-[10px] bg-cyan-950/60 border border-cyan-800/50 text-cyan-300">
                  {{ getOutboundStreamInfo(ob).security.toUpperCase() }}
                </span>
              </div>
              <span v-else class="text-neutral-500 text-[11px]">-</span>
            </td>

            <!-- 4. Target Endpoint / Host -->
            <td class="p-3.5 align-middle font-mono text-[11px] text-foreground">
              {{ getTargetEndpoint(ob) }}
            </td>

            <!-- 5. Settings Summary -->
            <td class="p-3.5 align-middle">
              <div class="text-[11px] font-mono text-muted-foreground max-w-[240px] truncate" :title="formatSettingsSummary(ob)">
                {{ formatSettingsSummary(ob) }}
              </div>
            </td>

            <!-- 6. Actions -->
            <td class="p-3.5 align-middle text-right" @click.stop>
              <div class="flex items-center justify-end gap-1.5">
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-xs"
                  @click="emit('inspect', ob)"
                  title="查看详情与参数"
                >
                  <Eye class="w-3.5 h-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-xs text-brand-400 hover:text-brand-300"
                  @click="emit('edit', ob)"
                  title="编辑配置"
                >
                  <Edit3 class="w-3.5 h-3.5" />
                </Button>
                <Button
                  v-if="ob.tag !== 'direct' && ob.tag !== 'block'"
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-950/40"
                  @click="emit('delete', ob.tag)"
                  title="删除节点"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </Button>
              </div>
            </td>
          </tr>

          <!-- Empty State -->
          <tr v-if="filteredOutbounds.length === 0">
            <td colspan="6" class="p-8 text-center text-muted-foreground">
              <p class="text-xs">未找到符合条件的出站节点</p>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { Plus, Search, Edit3, Trash2, Eye } from 'lucide-vue-next'
import Button from '../../../components/ui/Button.vue'
import Badge from '../../../components/ui/Badge.vue'
import type { OutboundItem } from '../types'
import {
  getProtocolBadgeVariant,
  getUsageDesc,
  getTargetEndpoint,
  getOutboundStreamInfo,
  formatSettingsSummary,
} from '../types'

const props = defineProps<{
  outbounds: OutboundItem[]
  selectedTag?: string
}>()

const emit = defineEmits<{
  (e: 'inspect', ob: OutboundItem): void
  (e: 'edit', ob: OutboundItem): void
  (e: 'delete', tag: string): void
  (e: 'create'): void
}>()

const searchQuery = ref('')
const protocolFilter = ref('all')

const filteredOutbounds = computed(() => {
  return props.outbounds.filter((ob) => {
    if (protocolFilter.value !== 'all' && ob.protocol?.toLowerCase() !== protocolFilter.value.toLowerCase()) {
      return false
    }
    if (!searchQuery.value) return true
    const q = searchQuery.value.toLowerCase()
    return (
      ob.tag?.toLowerCase().includes(q) ||
      ob.protocol?.toLowerCase().includes(q) ||
      getTargetEndpoint(ob).toLowerCase().includes(q) ||
      formatSettingsSummary(ob).toLowerCase().includes(q)
    )
  })
})
</script>
