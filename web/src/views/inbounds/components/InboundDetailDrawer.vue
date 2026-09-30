<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="`入站节点: ${inbound?.tag || ''}`"
    :description="`协议 ${inbound?.protocol?.toUpperCase()} / 端口 :${inbound?.port}`"
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <div v-if="inbound" class="space-y-4 text-xs">
      <!-- 1. Node Overview Card -->
      <div class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">节点概览 (Overview)</span>
          <div class="flex items-center gap-1.5">
            <Badge :variant="inbound.isAlive ? 'success' : 'destructive'" :dot="true">
              {{ inbound.isAlive ? `运行正常 (${inbound.latencyMs || 1}ms)` : '端口未响应' }}
            </Badge>
            <Badge :variant="getProtocolBadgeVariant(inbound.protocol)">
              {{ inbound.protocol?.toUpperCase() }}
            </Badge>
          </div>
        </div>

        <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 font-mono">
          <div>
            <span class="block text-[10px] text-muted-foreground">Tag 标识</span>
            <span class="text-foreground font-semibold">{{ inbound.tag }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">内部监听</span>
            <span class="text-neutral-300">{{ inbound.listen || '0.0.0.0' }}:{{ inbound.port }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">公网外部端口</span>
            <span class="text-foreground font-semibold">:{{ inbound.externalPort || inbound.port }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">传输协议 (Network)</span>
            <span class="text-neutral-300 uppercase">{{ getStreamNetwork(inbound) }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">安全协议 (Security)</span>
            <span class="text-neutral-300 uppercase">{{ getSecurityType(inbound) }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">节点流控 (Flow)</span>
            <span class="text-neutral-300">{{ getNodeFlow(inbound) }}</span>
          </div>
        </div>

        <div v-if="inbound.externalHost" class="pt-2 border-t border-border/40 font-mono text-[11px]">
          <span class="text-muted-foreground">自定义外部域名: </span>
          <span class="text-neutral-300">{{ inbound.externalHost }}</span>
        </div>
      </div>

      <!-- Socks 专属参数 -->
      <div v-if="inbound.protocol === 'socks'" class="rounded-lg border border-border bg-card p-4 space-y-3 font-mono text-xs">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-semibold text-foreground uppercase tracking-wider">Socks 代理配置</span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-[11px]">
          <div>
            <span class="block text-[10px] text-muted-foreground">认证方式</span>
            <span class="text-foreground">{{ parsedSettings.auth === 'password' ? '账号密码认证' : '无认证 (noauth)' }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">UDP 支持</span>
            <span class="text-foreground">{{ parsedSettings.udp !== false ? '已启用' : '已关闭' }}</span>
          </div>
          <div v-if="parsedSettings.accounts?.length > 0" class="col-span-2">
            <span class="block text-[10px] text-muted-foreground">认证账号</span>
            <span class="text-foreground">{{ parsedSettings.accounts[0].user }}</span>
          </div>
        </div>
      </div>

      <!-- HTTP 专属参数 -->
      <div v-if="inbound.protocol === 'http'" class="rounded-lg border border-border bg-card p-4 space-y-3 font-mono text-xs">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-semibold text-foreground uppercase tracking-wider">HTTP 代理配置</span>
        </div>
        <div class="text-[11px]">
          <span class="block text-[10px] text-muted-foreground">认证配置</span>
          <span class="text-foreground">
            {{ parsedSettings.accounts?.length > 0 ? `用户名: ${parsedSettings.accounts[0].user}` : '无认证 (匿名代理)' }}
          </span>
        </div>
      </div>

      <!-- dokodemo-door 专属参数 -->
      <div v-if="inbound.protocol === 'dokodemo-door'" class="rounded-lg border border-border bg-card p-4 space-y-3 font-mono text-xs">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-semibold text-foreground uppercase tracking-wider">dokodemo-door 转发目标</span>
        </div>
        <div class="grid grid-cols-3 gap-2 text-[11px]">
          <div>
            <span class="block text-[10px] text-muted-foreground">目标地址</span>
            <span class="text-foreground font-semibold">{{ parsedSettings.address || '127.0.0.1' }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">目标端口</span>
            <span class="text-foreground font-semibold">:{{ parsedSettings.port || 53 }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">转发协议</span>
            <span class="text-foreground uppercase">{{ parsedSettings.network || 'TCP,UDP' }}</span>
          </div>
        </div>
      </div>

      <!-- Shadowsocks 专属参数 -->
      <div v-if="inbound.protocol === 'shadowsocks'" class="rounded-lg border border-border bg-card p-4 space-y-3 font-mono text-xs">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-semibold text-foreground uppercase tracking-wider">Shadowsocks 节点参数</span>
        </div>
        <div class="grid grid-cols-2 gap-2 text-[11px]">
          <div>
            <span class="block text-[10px] text-muted-foreground">加密算法 (Method)</span>
            <span class="text-cyan-400 font-semibold">{{ parsedSettings.method || '2022-blake3-aes-128-gcm' }}</span>
          </div>
          <div>
            <span class="block text-[10px] text-muted-foreground">运行模式</span>
            <span class="text-foreground">{{ parsedSettings.password ? '单用户密码模式' : '多用户 Sub-Key 派生模式' }}</span>
          </div>
        </div>
      </div>

      <!-- 2. Reality / TLS Inspection Card -->
      <div v-if="isReality(inbound)" class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <div class="flex items-center gap-2">
            <Shield class="w-3.5 h-3.5 text-neutral-300" />
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">REALITY 伪装配置与合规状态</span>
          </div>
          <Button
            variant="outline"
            size="sm"
            class="h-6 px-2 text-[10px]"
            :loading="checkingReality"
            @click="emit('checkReality')"
          >
            刷新检测
          </Button>
        </div>

        <!-- Reality Target & SNI -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 font-mono text-[11px]">
          <div class="p-2 rounded bg-neutral-900/80 border border-border/40">
            <span class="block text-[10px] text-muted-foreground">回落伪装目标 (Dest)</span>
            <span class="text-foreground">{{ getInboundRealityField(inbound, 'dest') || '-' }}</span>
          </div>
          <div class="p-2 rounded bg-neutral-900/80 border border-border/40">
            <span class="block text-[10px] text-muted-foreground">SNI 域名列表</span>
            <span class="text-foreground">{{ getInboundRealityField(inbound, 'serverNames') || '-' }}</span>
          </div>
        </div>

        <!-- Public Key & ShortIDs -->
        <div class="space-y-1.5 font-mono text-[11px]">
          <div class="p-2 rounded bg-neutral-900/80 border border-border/40 flex items-center justify-between">
            <div class="min-w-0 pr-2">
              <span class="block text-[10px] text-muted-foreground">Short ID</span>
              <span class="text-foreground truncate">{{ getInboundRealityField(inbound, 'shortIds') || '0123456789abcdef' }}</span>
            </div>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 px-2 text-[10px] shrink-0"
              @click="copyText(getInboundRealityField(inbound, 'shortIds'))"
            >
              <Copy class="w-3 h-3 mr-1" />
              复制
            </Button>
          </div>
        </div>

        <!-- Reality Detailed Check Items -->
        <div v-if="inboundRealityStatus" class="pt-2 border-t border-border/40 space-y-2">
          <span class="text-[10px] font-mono uppercase text-muted-foreground tracking-wider">巡检明细 (Inspection Results)</span>
          <div
            v-for="(item, idx) in inboundRealityStatus.items"
            :key="idx"
            class="p-2.5 rounded-md border text-[11px] font-mono space-y-1"
            :class="item.status === 'ok' ? 'bg-neutral-900/60 border-neutral-800' : item.status === 'warning' ? 'bg-amber-500/5 border-amber-500/20' : 'bg-rose-500/5 border-rose-500/20'"
          >
            <div class="flex items-center justify-between">
              <span class="text-foreground font-semibold">{{ item.serverName }}</span>
              <span
                class="px-1.5 py-0.2 rounded text-[10px] font-semibold"
                :class="item.status === 'ok' ? 'text-emerald-400 bg-emerald-500/10' : item.status === 'warning' ? 'text-amber-400 bg-amber-500/10' : 'text-rose-400 bg-rose-500/10'"
              >
                {{ item.status.toUpperCase() }}
              </span>
            </div>
            <div class="flex items-center justify-between text-[10px] text-muted-foreground">
              <span>目标: {{ item.dest }}</span>
              <span v-if="item.daysLeft >= 0">证书剩 {{ item.daysLeft }} 天</span>
              <span>{{ item.tlsVersion || 'TLS' }} / {{ item.latencyMs || 0 }}ms</span>
            </div>
            <div v-if="item.details" class="text-[10px] text-neutral-400">
              {{ item.details }}
            </div>
          </div>
        </div>
        <div v-else class="text-[11px] text-muted-foreground font-mono">
          暂无巡检缓存，请点击“刷新检测”执行实时探测。
        </div>
      </div>

      <!-- 3. SubRoutes (单端口多出口) -->
      <div v-if="inbound.subRoutes?.length" class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">分流订阅线路 (Sub-Routes)</span>
          <span class="text-[10px] font-mono text-muted-foreground">共 {{ inbound.subRoutes.length }} 条</span>
        </div>

        <div class="space-y-2">
          <div
            v-for="sr in inbound.subRoutes"
            :key="sr.id || sr.routeId"
            class="p-2.5 rounded-md border border-border/60 bg-neutral-900/60 font-mono text-[11px] flex items-center justify-between"
          >
            <div class="flex items-center gap-2">
              <span class="px-1.5 py-0.5 rounded bg-neutral-800 text-neutral-300 font-bold">#{{ sr.routeId }}</span>
              <span class="text-foreground font-sans font-medium">{{ sr.name || ('线路 #' + sr.routeId) }}</span>
              <span class="text-neutral-500 text-[10px]">➔ {{ sr.outboundTag || 'direct' }}</span>
            </div>
            <div class="flex items-center gap-1.5">
              <Badge :variant="sr.enabled ? 'success' : 'secondary'">
                {{ sr.enabled ? '已启用' : '已停用' }}
              </Badge>
              <Badge variant="outline" class="text-[10px]">
                {{ sr.allowedUsers?.length ? `${sr.allowedUsers.length} 人授权` : '全员' }}
              </Badge>
            </div>
          </div>
        </div>
      </div>

      <!-- 4. Authorized Users -->
      <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(inbound.protocol)" class="rounded-lg border border-border bg-card p-4 space-y-3">
        <div class="flex items-center justify-between border-b border-border/60 pb-2">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">已关联授权用户</span>
          <span class="text-[10px] font-mono text-muted-foreground">{{ getClientCount(inbound, usersList) }} 位用户</span>
        </div>

        <div class="flex flex-wrap gap-1.5 max-h-36 overflow-y-auto">
          <span
            v-for="email in getAssignedUserEmails(inbound, usersList)"
            :key="email"
            class="px-2 py-1 rounded bg-neutral-900 border border-border/60 text-neutral-300 font-mono text-[11px] flex items-center gap-1.5"
          >
            <span class="w-1.5 h-1.5 rounded-full bg-neutral-500"></span>
            <span>{{ email }}</span>
          </span>
          <span v-if="!getAssignedUserEmails(inbound, usersList).length" class="text-[11px] text-muted-foreground font-mono">
            暂无绑定用户
          </span>
        </div>
      </div>
    </div>

    <template #footer>
      <Button
        variant="outline"
        size="sm"
        @click="emit('update:modelValue', false)"
      >
        关闭
      </Button>
      <Button
        v-if="inbound"
        variant="destructive"
        size="sm"
        @click="emit('delete', inbound.id)"
      >
        删除节点
      </Button>
      <Button
        v-if="inbound"
        variant="default"
        size="sm"
        @click="inbound && emit('edit', inbound)"
      >
        编辑配置
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Shield, Copy } from 'lucide-vue-next'
import Drawer from '../../../components/ui/Drawer.vue'
import Badge from '../../../components/ui/Badge.vue'
import Button from '../../../components/ui/Button.vue'
import { copyText } from '../../../utils/clipboard'
import type { RealitySummaryStatus } from '../../../api/reality'
import type { InboundItem } from '../types'
import {
  getProtocolBadgeVariant,
  getNodeFlow,
  getStreamNetwork,
  getSecurityType,
  isReality,
  getClientCount,
  getInboundRealityOverallStatus,
  getInboundRealityField,
  getAssignedUserEmails,
} from '../composables/useInboundList'

const props = defineProps<{
  modelValue: boolean
  inbound: InboundItem | null
  usersList?: any[]
  realitySummary?: RealitySummaryStatus | null
  checkingReality?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'edit', inb: InboundItem): void
  (e: 'delete', id: number): void
  (e: 'checkReality'): void
}>()

const inboundRealityStatus = computed(() => {
  if (!props.inbound) return null
  return getInboundRealityOverallStatus(props.inbound.tag, props.realitySummary || null)
})

const parsedSettings = computed(() => {
  if (!props.inbound?.settingsJson) return {}
  try {
    return JSON.parse(props.inbound.settingsJson)
  } catch {
    return {}
  }
})
</script>
