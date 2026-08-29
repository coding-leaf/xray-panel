<template>
  <div class="space-y-6">
    <!-- Header 区域 -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-white tracking-tight">线路与拓扑中心 (Topology)</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-indigo-500/15 text-indigo-400 border border-indigo-500/30 flex items-center gap-1">
            <span class="w-1.5 h-1.5 rounded-full bg-indigo-400 animate-pulse"></span>
            <span>强类型编译器实时拓扑</span>
          </span>
        </div>
        <p class="text-xs text-gray-400 mt-0.5">一体化接入网关、分流通道与落地出口，支持单端口多出口向导式 1 步发布</p>
      </div>

      <!-- Action Buttons -->
      <div class="flex flex-wrap items-center gap-2.5">
        <button
          @click="openWizard"
          class="px-4 py-2 bg-gradient-to-r from-brand-600 via-indigo-600 to-cyan-500 hover:from-brand-500 hover:to-cyan-400 text-white text-xs font-bold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5 animate-pulse hover:animate-none"
        >
          <Zap class="w-4 h-4 text-amber-300" />
          <span>⚡ 1步发布落地线路</span>
        </button>

        <button
          @click="openCreateGatewayModal"
          class="px-3.5 py-2 bg-gray-900/80 hover:bg-gray-800 text-gray-200 hover:text-white border border-gray-700 hover:border-gray-600 text-xs font-semibold rounded-xl transition-all flex items-center gap-1.5"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>新建接入网关</span>
        </button>

        <button
          @click="openCreateExitModal"
          class="px-3.5 py-2 bg-gray-900/80 hover:bg-gray-800 text-gray-200 hover:text-white border border-gray-700 hover:border-gray-600 text-xs font-semibold rounded-xl transition-all flex items-center gap-1.5"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>新建落地出口</span>
        </button>
      </div>
    </div>

    <!-- Navigation Tabs -->
    <div class="flex items-center gap-2 border-b border-gray-800/80 pb-3">
      <button
        @click="activeTab = 'channels'"
        class="px-4 py-2 rounded-xl text-xs font-semibold transition-all flex items-center gap-2"
        :class="activeTab === 'channels' ? 'bg-brand-600/20 text-brand-300 border border-brand-500/30' : 'text-gray-400 hover:text-gray-200 hover:bg-gray-900/50'"
      >
        <Network class="w-4 h-4" />
        <span>分流通道全景 (Channels: {{ allChannels.length }})</span>
      </button>

      <button
        @click="activeTab = 'gateways'"
        class="px-4 py-2 rounded-xl text-xs font-semibold transition-all flex items-center gap-2"
        :class="activeTab === 'gateways' ? 'bg-brand-600/20 text-brand-300 border border-brand-500/30' : 'text-gray-400 hover:text-gray-200 hover:bg-gray-900/50'"
      >
        <Radio class="w-4 h-4" />
        <span>接入网关池 (Gateways: {{ inbounds.length }})</span>
      </button>

      <button
        @click="activeTab = 'exits'"
        class="px-4 py-2 rounded-xl text-xs font-semibold transition-all flex items-center gap-2"
        :class="activeTab === 'exits' ? 'bg-brand-600/20 text-brand-300 border border-brand-500/30' : 'text-gray-400 hover:text-gray-200 hover:bg-gray-900/50'"
      >
        <Send class="w-4 h-4" />
        <span>落地出口池 (Exit Nodes: {{ outbounds.length }})</span>
      </button>
    </div>

    <!-- TAB 1: 分流通道全景流水线 (Channels Pipeline) -->
    <div v-if="activeTab === 'channels'" class="space-y-4">
      <div v-if="allChannels.length > 0" class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="ch in allChannels"
          :key="ch.id"
          class="glass-panel p-5 rounded-2xl border border-gray-800/80 hover:border-brand-500/40 transition-all flex flex-col justify-between group"
        >
          <div>
            <!-- Channel Title & RouteID Badge -->
            <div class="flex items-center justify-between mb-3">
              <div class="flex items-center gap-2">
                <span class="text-base font-bold text-white tracking-tight">{{ ch.name }}</span>
              </div>
              <span
                v-if="ch.routeId > 0"
                class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold bg-indigo-500/20 text-indigo-300 border border-indigo-500/30"
              >
                Route #{{ ch.routeId }}
              </span>
              <span v-else class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold bg-gray-800 text-gray-400 border border-gray-700">
                直通
              </span>
            </div>

            <!-- Pipeline Flow Topology Card -->
            <div class="bg-gray-950/60 p-3.5 rounded-xl border border-gray-800/60 space-y-2 text-xs font-mono">
              <!-- Gateway Info -->
              <div class="flex items-center justify-between text-gray-400">
                <span class="text-gray-500">接入网关</span>
                <span class="text-cyan-400 font-medium">{{ ch.gatewayTag }} (:{{ ch.externalPort || ch.gatewayPort }})</span>
              </div>

              <!-- Flow Arrow -->
              <div class="flex items-center justify-center text-gray-600 text-[10px]">
                <span>↓ Scoped VLESS 分流 (0x{{ (ch.routeId || 0).toString(16).padStart(4, '0') }})</span>
              </div>

              <!-- Exit Node Info -->
              <div class="flex items-center justify-between text-gray-400">
                <span class="text-gray-500">落地出口</span>
                <span class="text-brand-300 font-bold flex items-center gap-1">
                  <span>{{ ch.outboundTag }}</span>
                  <span class="text-[10px] px-1 py-0.2 rounded bg-gray-800 text-gray-300 font-normal uppercase">
                    {{ getOutboundProtocol(ch.outboundTag) }}
                  </span>
                </span>
              </div>
            </div>

            <!-- Status & Meta -->
            <div class="mt-3 flex items-center justify-between text-[11px] text-gray-500">
              <span>状态: <span :class="ch.enabled ? 'text-emerald-400 font-medium' : 'text-gray-500 line-through'">{{ ch.enabled ? '已发布' : '已暂停' }}</span></span>
              <span>单端口订阅就绪</span>
            </div>
          </div>

          <!-- Actions -->
          <div class="mt-4 pt-3 border-t border-gray-800/60 flex items-center justify-between text-xs">
            <button
              @click="toggleChannelStatus(ch)"
              class="text-gray-400 hover:text-white transition-colors"
            >
              {{ ch.enabled ? '暂停线路' : '启用线路' }}
            </button>
            <button
              @click="deleteChannel(ch)"
              class="text-rose-400 hover:text-rose-300 transition-colors"
            >
              删除线路
            </button>
          </div>
        </div>
      </div>

      <!-- Empty State -->
      <div v-else class="glass-panel p-12 text-center rounded-2xl border border-gray-800">
        <Network class="w-12 h-12 mx-auto text-gray-600 mb-3" />
        <h3 class="text-sm font-semibold text-gray-300">暂未发布任何分流线路</h3>
        <p class="text-xs text-gray-500 mt-1">点击右上角“⚡ 1步发布落地线路”即可一键将落地节点发布为订阅线路</p>
      </div>
    </div>

    <!-- TAB 2: 接入网关池 (Gateways / Inbounds) -->
    <div v-if="activeTab === 'gateways'" class="space-y-4">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="inb in inbounds"
          :key="inb.id"
          class="glass-panel p-5 rounded-2xl border border-gray-800/80 hover:border-brand-500/40 transition-all flex flex-col justify-between"
        >
          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-base font-bold text-white tracking-tight font-mono">{{ inb.tag }}</span>
              <div class="flex items-center gap-1.5">
                <span class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold uppercase bg-brand-500/20 text-brand-300 border border-brand-500/30">
                  {{ inb.protocol }}
                </span>
                <span class="px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-gray-800 text-cyan-400 border border-gray-700">
                  {{ getStreamNetwork(inb) }}
                </span>
              </div>
            </div>

            <div class="space-y-2 text-xs text-gray-400">
              <div class="flex justify-between py-1 border-b border-gray-800/60">
                <span>端口映射 (内部 ➔ 外部)</span>
                <span class="text-brand-300 font-mono font-bold">:{{ inb.port }} ➔ :{{ inb.externalPort || 443 }}</span>
              </div>
              <div class="flex justify-between py-1 border-b border-gray-800/60">
                <span>安全协议 (Security)</span>
                <span class="text-gray-200 font-mono uppercase font-semibold">{{ getSecurityType(inb) }}</span>
              </div>
              <div class="flex justify-between py-1 border-b border-gray-800/60">
                <span>挂载分流线路</span>
                <span class="text-indigo-300 font-mono font-bold">{{ (inb.subRoutes || []).length }} 条</span>
              </div>
              <div class="flex justify-between py-1">
                <span>端口连通性</span>
                <span :class="inb.isAlive ? 'text-emerald-400' : 'text-rose-400'" class="font-semibold flex items-center gap-1">
                  <span class="w-1.5 h-1.5 rounded-full" :class="inb.isAlive ? 'bg-emerald-400' : 'bg-rose-400'"></span>
                  <span>{{ inb.isAlive ? `正常 (${inb.latencyMs || 1}ms)` : '未连通' }}</span>
                </span>
              </div>
            </div>
          </div>

          <div class="mt-4 pt-3 border-t border-gray-800/60 flex items-center justify-between">
            <button @click="editGateway(inb)" class="text-xs text-brand-400 hover:text-brand-300">
              编辑网关参数
            </button>
            <button @click="deleteGateway(inb.id)" class="text-xs text-rose-400 hover:text-rose-300">
              删除网关
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- TAB 3: 落地出口池 (Exit Nodes / Outbounds) -->
    <div v-if="activeTab === 'exits'" class="space-y-4">
      <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
        <div
          v-for="ob in outbounds"
          :key="ob.tag"
          class="glass-panel p-5 rounded-2xl border border-gray-800/80 hover:border-brand-500/40 transition-all flex flex-col justify-between"
        >
          <div>
            <div class="flex items-center justify-between mb-3">
              <span class="text-base font-bold text-white tracking-tight font-mono">{{ ob.tag }}</span>
              <span class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold uppercase" :class="protocolBadgeColor(ob.protocol)">
                {{ ob.protocol }}
              </span>
            </div>

            <div class="space-y-2 text-xs text-gray-400">
              <div class="flex justify-between py-1 border-b border-gray-800/60">
                <span>出站协议</span>
                <span class="text-gray-200 font-mono font-semibold">{{ ob.protocol }}</span>
              </div>
              <div class="py-1">
                <span class="block mb-1 text-gray-500 text-[11px]">设置摘要:</span>
                <div class="bg-gray-900/80 p-2 rounded-xl text-[11px] font-mono text-gray-300 truncate">
                  {{ formatSettingsSummary(ob) }}
                </div>
              </div>
            </div>
          </div>

          <div class="mt-4 pt-3 border-t border-gray-800/60 flex items-center justify-between">
            <button @click="editOutbound(ob)" class="text-xs text-brand-400 hover:text-brand-300">
              编辑设置
            </button>
            <button
              v-if="ob.tag !== 'direct' && ob.tag !== 'block'"
              @click="deleteOutbound(ob.tag)"
              class="text-xs text-rose-400 hover:text-rose-300"
            >
              删除
            </button>
            <span v-else class="text-xs text-gray-600 font-mono">系统保留</span>
          </div>
        </div>
      </div>
    </div>

    <!-- ⚡ 1步发布落地线路向导弹窗 (Quick Publish Wizard Modal) -->
    <div v-if="showWizardModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-lg p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-5 my-8 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white flex items-center gap-2">
              <span>⚡ 1步发布落地线路向导</span>
            </h2>
            <p class="text-xs text-gray-400 mt-0.5">自动创建出口节点、绑定接入网关、分配 RouteID 并下发订阅</p>
          </div>
          <button @click="showWizardModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <form @submit.prevent="submitWizard" class="space-y-4 text-xs">
          <!-- 录入模式切换 -->
          <div class="flex gap-2 p-1 bg-gray-900/80 rounded-xl border border-gray-800">
            <button
              type="button"
              :class="wizardMode === 'link' ? 'bg-brand-600 text-white font-bold' : 'text-gray-400 hover:text-white'"
              class="flex-1 py-2 text-xs rounded-lg transition-all"
              @click="wizardMode = 'link'"
            >
              粘贴节点链接解析
            </button>
            <button
              type="button"
              :class="wizardMode === 'manual' ? 'bg-brand-600 text-white font-bold' : 'text-gray-400 hover:text-white'"
              class="flex-1 py-2 text-xs rounded-lg transition-all"
              @click="wizardMode = 'manual'"
            >
              从现有出口选择 / 自定义
            </button>
          </div>

          <!-- 模式 1: 节点链接解析 -->
          <div v-if="wizardMode === 'link'" class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800">
            <div>
              <label class="block text-gray-300 mb-1 font-medium">节点链接 (支持 vless://, vmess://, trojan://, ss://)</label>
              <textarea
                v-model="wizardLink"
                @input="parseImportLink"
                rows="3"
                placeholder="vless://uuid@server:443?type=tcp&security=reality...#香港节点"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl p-3 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
              ></textarea>
            </div>
            <div v-if="parsedExitTag" class="text-[11px] text-cyan-400 font-mono">
              ✓ 解析成功: 出口标识将设为 <span class="font-bold text-white">{{ parsedExitTag }}</span> ({{ parsedProtocol }})
            </div>
          </div>

          <!-- 模式 2: 从现有出口选择 -->
          <div v-else class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800">
            <div>
              <label class="block text-gray-300 mb-1 font-medium">选择目标出口 (Outbound)</label>
              <select
                v-model="wizardForm.exitTag"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              >
                <option v-for="ob in outbounds" :key="ob.tag" :value="ob.tag">
                  {{ ob.tag }} ({{ ob.protocol }})
                </option>
              </select>
            </div>
          </div>

          <!-- 线路基本参数 -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-gray-300 mb-1 font-medium">订阅线路名称</label>
              <input
                v-model="wizardForm.channelName"
                type="text"
                required
                placeholder="如 🇯🇵 日本原生直连"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
              />
            </div>

            <div>
              <label class="block text-gray-300 mb-1 font-medium">挂载接入网关</label>
              <select
                v-model="wizardForm.gatewayTag"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              >
                <option v-for="inb in inbounds" :key="inb.tag" :value="inb.tag">
                  {{ inb.tag }} (:{{ inb.externalPort || inb.port }})
                </option>
              </select>
            </div>
          </div>

          <!-- 自动分配 RouteID 预览卡片 -->
          <div class="p-3.5 bg-indigo-950/30 border border-indigo-500/30 rounded-2xl flex items-center justify-between text-xs">
            <div class="space-y-0.5">
              <span class="text-indigo-300 font-semibold block">协议级单端口分流</span>
              <span class="text-[10px] text-gray-400">系统已自动分配下一个空闲 RouteID</span>
            </div>
            <span class="px-3 py-1 bg-indigo-500/20 text-indigo-300 font-mono font-bold rounded-lg border border-indigo-500/30">
              Route #{{ nextRouteId }}
            </span>
          </div>

          <div class="pt-3 border-t border-gray-800 flex items-center justify-end gap-3">
            <button
              type="button"
              @click="showWizardModal = false"
              class="px-4 py-2 rounded-xl text-gray-400 hover:text-white"
            >
              取消
            </button>
            <button
              type="submit"
              :disabled="wizardSubmitting"
              class="px-5 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-bold rounded-xl shadow-lg shadow-brand-500/25 disabled:opacity-50"
            >
              {{ wizardSubmitting ? '正在发布...' : '立即发布并生效' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  Plus,
  Zap,
  Radio,
  Send,
  Network,
  Trash2,
} from 'lucide-vue-next'
import api from '../api'
import { toast } from '../utils/toast'

const activeTab = ref<'channels' | 'gateways' | 'exits'>('channels')

const inbounds = ref<any[]>([])
const outbounds = ref<any[]>([])
const usersList = ref<any[]>([])

// 聚合所有通道 (Channels)
const allChannels = computed(() => {
  const list: any[] = []
  for (const inb of inbounds.value) {
    const srs = inb.subRoutes || []
    if (srs.length > 0) {
      for (const sr of srs) {
        list.push({
          id: `${inb.tag}_${sr.routeId}`,
          name: sr.name || `线路 #${sr.routeId}`,
          gatewayTag: inb.tag,
          gatewayPort: inb.port,
          externalPort: inb.externalPort,
          routeId: sr.routeId,
          outboundTag: sr.outboundTag || 'direct',
          enabled: sr.enabled !== false,
          inboundId: inb.id,
        })
      }
    } else {
      // 独立直出通道
      list.push({
        id: `${inb.tag}_direct`,
        name: inb.remark || inb.tag,
        gatewayTag: inb.tag,
        gatewayPort: inb.port,
        externalPort: inb.externalPort,
        routeId: inb.routeId || 0,
        outboundTag: 'direct',
        enabled: inb.enabled,
        inboundId: inb.id,
      })
    }
  }
  return list
})

const nextRouteId = computed(() => {
  const used = new Set(allChannels.value.map((c) => Number(c.routeId) || 0))
  let id = 1
  while (used.has(id)) {
    id++
  }
  return id
})

// === 1步向导状态 ===
const showWizardModal = ref(false)
const wizardMode = ref<'link' | 'manual'>('link')
const wizardLink = ref('')
const wizardSubmitting = ref(false)
const parsedExitTag = ref('')
const parsedProtocol = ref('')
const parsedOutboundPayload = ref<any>(null)

const wizardForm = ref({
  channelName: '',
  gatewayTag: '',
  exitTag: 'direct',
})

const openWizard = () => {
  wizardLink.value = ''
  parsedExitTag.value = ''
  parsedProtocol.value = ''
  parsedOutboundPayload.value = null
  wizardForm.value = {
    channelName: `分流线路 #${nextRouteId.value}`,
    gatewayTag: inbounds.value[0]?.tag || '',
    exitTag: outbounds.value[0]?.tag || 'direct',
  }
  showWizardModal.value = true
}

const parseImportLink = () => {
  const link = wizardLink.value.trim()
  if (!link) {
    parsedExitTag.value = ''
    return
  }
  try {
    if (link.startsWith('vless://')) {
      const u = new URL(link)
      const tag = `out-vless-${u.hostname.replace(/[^a-zA-Z0-9]/g, '') || 'node'}`
      parsedExitTag.value = tag
      parsedProtocol.value = 'vless'
      if (u.hash) {
        wizardForm.value.channelName = decodeURIComponent(u.hash.replace('#', ''))
      }
      parsedOutboundPayload.value = {
        tag,
        protocol: 'vless',
        settingsJson: JSON.stringify({
          vnext: [
            {
              address: u.hostname,
              port: parseInt(u.port) || 443,
              users: [{ id: u.username, encryption: 'none' }],
            },
          ],
        }),
      }
    } else if (link.startsWith('trojan://')) {
      const u = new URL(link)
      const tag = `out-trojan-${u.hostname.replace(/[^a-zA-Z0-9]/g, '') || 'node'}`
      parsedExitTag.value = tag
      parsedProtocol.value = 'trojan'
      if (u.hash) {
        wizardForm.value.channelName = decodeURIComponent(u.hash.replace('#', ''))
      }
      parsedOutboundPayload.value = {
        tag,
        protocol: 'trojan',
        settingsJson: JSON.stringify({
          servers: [
            {
              address: u.hostname,
              port: parseInt(u.port) || 443,
              password: u.username,
            },
          ],
        }),
      }
    }
  } catch (e) {
    console.error('Parse link failed', e)
  }
}

const submitWizard = async () => {
  wizardSubmitting.value = true
  try {
    let targetOutboundTag = wizardForm.value.exitTag

    // 如果是通过链接导入的新出口，先创建出站
    if (wizardMode.value === 'link' && parsedOutboundPayload.value) {
      targetOutboundTag = parsedOutboundPayload.value.tag
      await api.post('/outbounds', parsedOutboundPayload.value)
    }

    // 将 SubRoute 挂载到选定的网关
    const targetGateway = inbounds.value.find((i) => i.tag === wizardForm.value.gatewayTag)
    if (!targetGateway) {
      toast.error('未找到目标接入网关')
      return
    }

    const currentSubRoutes = targetGateway.subRoutes || []
    currentSubRoutes.push({
      id: Math.random().toString(36).substring(2, 9),
      name: wizardForm.value.channelName,
      routeId: nextRouteId.value,
      outboundTag: targetOutboundTag,
      enabled: true,
    })

    const payload = {
      ...targetGateway,
      subRoutesJson: JSON.stringify(currentSubRoutes),
    }

    await api.put(`/inbounds/${targetGateway.id}`, payload)
    toast.success('🎉 线路发布成功！已自动注入 Xray 路由表并就绪订阅')
    showWizardModal.value = false
    await fetchAll()
  } catch (err: any) {
    toast.error(err.response?.data?.error || '发布线路失败')
  } finally {
    wizardSubmitting.value = false
  }
}

const toggleChannelStatus = async (ch: any) => {
  try {
    const targetGateway = inbounds.value.find((i) => i.id === ch.inboundId)
    if (!targetGateway) return

    if (ch.routeId > 0) {
      const srs = targetGateway.subRoutes || []
      for (const sr of srs) {
        if (sr.routeId === ch.routeId) {
          sr.enabled = !sr.enabled
        }
      }
      await api.put(`/inbounds/${targetGateway.id}`, {
        ...targetGateway,
        subRoutesJson: JSON.stringify(srs),
      })
    } else {
      // 切换网关整体启用状态
      await api.put(`/inbounds/${targetGateway.id}`, {
        ...targetGateway,
        enabled: !targetGateway.enabled,
      })
    }
    toast.success('状态已更新')
    await fetchAll()
  } catch (err) {
    toast.error('更新状态失败')
  }
}

const deleteChannel = async (ch: any) => {
  if (!confirm(`确定删除线路 "${ch.name}" 吗？`)) return
  try {
    const targetGateway = inbounds.value.find((i) => i.id === ch.inboundId)
    if (!targetGateway) return

    if (ch.routeId > 0) {
      const srs = (targetGateway.subRoutes || []).filter((s: any) => s.routeId !== ch.routeId)
      await api.put(`/inbounds/${targetGateway.id}`, {
        ...targetGateway,
        subRoutesJson: JSON.stringify(srs),
      })
      toast.success('线路已删除')
    } else {
      // 独立网关直出线路，提示并删除网关
      await api.delete(`/inbounds/${targetGateway.id}`)
      toast.success('网关及线路已删除')
    }
    await fetchAll()
  } catch (err) {
    toast.error('删除线路失败')
  }
}

// 辅助方法
const getOutboundProtocol = (tag: string) => {
  const ob = outbounds.value.find((o) => o.tag === tag)
  return ob ? ob.protocol : 'freedom'
}

const getStreamNetwork = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.network || 'tcp'
  } catch (e) {
    return 'tcp'
  }
}

const getSecurityType = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.security || 'none'
  } catch (e) {
    return 'none'
  }
}

const protocolBadgeColor = (p: string) => {
  switch ((p || '').toLowerCase()) {
    case 'vless': return 'bg-cyan-500/20 text-cyan-300 border border-cyan-500/30'
    case 'trojan': return 'bg-purple-500/20 text-purple-300 border border-purple-500/30'
    case 'freedom': return 'bg-emerald-500/20 text-emerald-300 border border-emerald-500/30'
    case 'wireguard': return 'bg-amber-500/20 text-amber-300 border border-amber-500/30'
    default: return 'bg-gray-800 text-gray-300'
  }
}

const formatSettingsSummary = (ob: any) => {
  if (ob.protocol === 'freedom') return '直连公网 (Freedom)'
  if (ob.protocol === 'blackhole') return '黑洞拦截 (Blackhole)'
  if (ob.protocol === 'wireguard') return 'Cloudflare WARP (WireGuard)'
  return ob.settingsJson || '{}'
}

const openCreateGatewayModal = () => {
  toast.info('请进入网关池标签页进行深度参数配置')
  activeTab.value = 'gateways'
}

const openCreateExitModal = () => {
  toast.info('请进入出口池标签页进行出站参数配置')
  activeTab.value = 'exits'
}

const editGateway = (inb: any) => {
  window.location.href = `/inbounds?edit=${inb.id}`
}

const deleteGateway = async (id: number) => {
  if (!confirm('确定删除该网关吗？')) return
  try {
    await api.delete(`/inbounds/${id}`)
    toast.success('网关已删除')
    await fetchAll()
  } catch (e) {
    toast.error('删除网关失败')
  }
}

const editOutbound = (ob: any) => {
  window.location.href = `/outbounds?edit=${ob.tag}`
}

const deleteOutbound = async (tag: string) => {
  if (!confirm(`确定删除出站 ${tag} 吗？`)) return
  try {
    await api.delete(`/outbounds/${tag}`)
    toast.success('出站已删除')
    await fetchAll()
  } catch (e) {
    toast.error('删除出站失败')
  }
}

const fetchAll = async () => {
  try {
    const [inbRes, obRes, uRes]: any = await Promise.all([
      api.get('/inbounds'),
      api.get('/outbounds').catch(() => []),
      api.get('/users').catch(() => []),
    ])
    inbounds.value = (inbRes || []).map((ib: any) => {
      let srs: any[] = []
      try {
        srs = JSON.parse(ib.subRoutesJson || '[]')
      } catch (e) {}
      return { ...ib, subRoutes: srs }
    })
    outbounds.value = obRes || []
    usersList.value = uRes || []
  } catch (err) {
    console.error(err)
  }
}

onMounted(() => {
  fetchAll()
})
</script>
