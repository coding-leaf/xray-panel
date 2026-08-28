<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-white tracking-tight">路由与分流策略 (Routing)</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-amber-500/15 text-amber-400 border border-amber-500/30 flex items-center gap-1">
            <span>🟠 修改规则自动重启核心 (加载新路由表)</span>
          </span>
        </div>
        <p class="text-xs text-gray-400 mt-0.5">自上而下匹配分流规则，出站与入站标签动态绑定，Xray 官方不支持平滑重载，规则落盘后自动全量重启核心生效</p>
      </div>

      <div class="flex items-center gap-3">
        <button
          @click="openAddRuleModal"
          class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
        >
          <Plus class="w-4 h-4" />
          <span>添加分流规则</span>
        </button>

        <button
          @click="saveAllRouting"
          :disabled="saving"
          class="px-5 py-2 bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold rounded-xl transition-all shadow-lg shadow-emerald-600/25 flex items-center gap-1.5"
        >
          <Check class="w-4 h-4" />
          <span>{{ saving ? '保存中...' : '保存并应用' }}</span>
        </button>
      </div>
    </div>

    <!-- GeoData 规则库在线升级状态卡片 -->
    <div class="glass-panel p-4 sm:p-5 rounded-2xl border border-gray-800/80 flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4 bg-gradient-to-r from-gray-900/60 to-brand-950/20">
      <div class="flex items-center gap-3">
        <div class="p-2.5 rounded-xl bg-brand-500/10 border border-brand-500/20 text-brand-400">
          <Database class="w-5 h-5" />
        </div>
        <div class="space-y-0.5 text-xs">
          <div class="flex items-center gap-2">
            <span class="font-bold text-white">GeoData 分流规则库 (geoip.dat & geosite.dat)</span>
            <span class="px-2 py-0.2 rounded text-[10px] font-mono bg-gray-800 text-cyan-400">
              {{ geodataStatus?.platform || 'Xray Core' }}
            </span>
          </div>
          <p class="text-[11px] text-gray-400 font-mono">
            GeoIP: {{ geodataStatus?.geoipExists ? `${(geodataStatus.geoipSize / 1048576).toFixed(2)} MB` : '未找到' }} | 
            GeoSite: {{ geodataStatus?.geositeExists ? `${(geodataStatus.geositeSize / 1048576).toFixed(2)} MB` : '未找到' }} | 
            路径: {{ geodataStatus?.targetDirectory || './' }}
          </p>
        </div>
      </div>

      <button
        @click="updateGeoData"
        :disabled="updatingGeo"
        class="px-4 py-2 bg-gradient-to-r from-cyan-600 to-blue-600 hover:from-cyan-500 hover:to-blue-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-cyan-500/25 flex items-center gap-1.5 shrink-0 disabled:opacity-50"
      >
        <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': updatingGeo }" />
        <span>{{ updatingGeo ? '正在更新规则库...' : '⚡ 一键拉取最新规则库' }}</span>
      </button>
    </div>

    <!-- Domain Strategy & Presets Bar -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-5">
      <!-- Strategy Selector -->
      <div class="glass-panel p-5 rounded-2xl border border-gray-800/80 space-y-2">
        <label class="block text-xs font-bold text-white uppercase tracking-wider">
          🌐 域名解析策略 (domainStrategy)
        </label>
        <select
          v-model="routingConfig.domainStrategy"
          class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-xs text-white focus:outline-none focus:border-brand-500 font-mono"
        >
          <option value="IPIfNonMatch">IPIfNonMatch (推荐：域名未匹配时解析为 IP 再次匹配)</option>
          <option value="AsIs">AsIs (保持原样：仅匹配客户端直发域名)</option>
          <option value="IPOnDemand">IPOnDemand (强制实时解析 IP 匹配)</option>
        </select>
        <p class="text-[11px] text-gray-500 leading-relaxed">
          配合 Inbound 的 Sniffing（域名嗅探），可精准识别 TLS/HTTP 连接真实域名。
        </p>
      </div>

      <!-- Presets quick buttons (Interactive Wizard) -->
      <div class="glass-panel p-5 rounded-2xl border border-gray-800/80 lg:col-span-2 space-y-3">
        <div class="flex items-center justify-between">
          <span class="text-xs font-bold text-gray-300 uppercase tracking-wider block">
            ⚡ 智能预设分流向导 (动态探测可用出站)
          </span>
          <span class="text-[11px] text-gray-500 font-mono">点击唤起配置向导</span>
        </div>

        <div class="flex flex-wrap gap-2">
          <button
            @click="openPresetWizard('ads')"
            class="px-3 py-1.5 rounded-xl bg-gray-900/80 hover:bg-rose-500/10 border border-gray-700 hover:border-rose-500/40 text-xs text-gray-300 hover:text-rose-300 transition-all flex items-center gap-1.5"
          >
            <span>🛡️ 拦截广告 (geosite:category-ads-all)</span>
          </button>

          <button
            @click="openPresetWizard('private_ip')"
            class="px-3 py-1.5 rounded-xl bg-gray-900/80 hover:bg-rose-500/10 border border-gray-700 hover:border-rose-500/40 text-xs text-gray-300 hover:text-rose-300 transition-all flex items-center gap-1.5"
          >
            <span>🔒 屏蔽私有局域网 (geoip:private)</span>
          </button>

          <button
            @click="openPresetWizard('bt')"
            class="px-3 py-1.5 rounded-xl bg-gray-900/80 hover:bg-rose-500/10 border border-gray-700 hover:border-rose-500/40 text-xs text-gray-300 hover:text-rose-300 transition-all flex items-center gap-1.5"
          >
            <span>🚫 拦截 BT 下载 (bittorrent)</span>
          </button>

          <button
            @click="openPresetWizard('smtp')"
            class="px-3 py-1.5 rounded-xl bg-gray-900/80 hover:bg-rose-500/10 border border-gray-700 hover:border-rose-500/40 text-xs text-gray-300 hover:text-rose-300 transition-all flex items-center gap-1.5"
          >
            <span>📧 封禁邮件 25 端口 (port: 25)</span>
          </button>

          <button
            @click="openPresetWizard('cn')"
            class="px-3 py-1.5 rounded-xl bg-gray-900/80 hover:bg-cyan-500/10 border border-gray-700 hover:border-cyan-500/40 text-xs text-gray-300 hover:text-cyan-300 transition-all flex items-center gap-1.5"
          >
            <span>⚡ 国内流量分流 (geosite:cn / geoip:cn)</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Rules List -->
    <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden">
      <div class="px-5 py-3.5 bg-gray-900/80 border-b border-gray-800 flex items-center justify-between text-xs font-bold text-gray-400">
        <span>规则列表（按从上至下顺序优先匹配）</span>
        <span>共 {{ routingConfig.rules?.length || 0 }} 条规则</span>
      </div>

      <div class="divide-y divide-gray-800/60 text-xs">
        <div
          v-for="(rule, idx) in routingConfig.rules"
          :key="idx"
          class="p-4 hover:bg-white/[0.02] transition-colors flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3"
        >
          <div class="flex items-start gap-3">
            <span class="w-6 h-6 rounded-lg bg-gray-800 flex items-center justify-center font-mono font-bold text-gray-400 text-[11px] shrink-0 mt-0.5">
              {{ Number(idx) + 1 }}
            </span>

            <div class="space-y-1">
              <div class="flex flex-wrap items-center gap-2">
                <span class="font-bold text-white">目标出站:</span>
                <span class="px-2.5 py-0.5 rounded-md font-mono font-bold text-xs" :class="outboundBadgeColor(rule.outboundTag)">
                  {{ rule.outboundTag }}
                </span>
                <span v-if="rule.inboundTag?.length" class="text-gray-500 font-mono text-[11px]">
                  (来源入站: {{ rule.inboundTag.join(', ') }})
                </span>
                <span v-if="!isKnownOutbound(rule.outboundTag)" class="text-amber-400 text-[10px] font-semibold" title="系统中未配置该出站标签">
                  ⚠️ 未知出站
                </span>
              </div>

              <!-- Rule conditions preview -->
              <div class="flex flex-wrap gap-1.5 pt-1">
                <span v-if="rule.domain?.length" class="px-2 py-0.5 rounded bg-blue-500/15 text-blue-300 font-mono text-[11px] border border-blue-500/20">
                  域名: {{ rule.domain.join(', ') }}
                </span>
                <span v-if="rule.ip?.length" class="px-2 py-0.5 rounded bg-purple-500/15 text-purple-300 font-mono text-[11px] border border-purple-500/20">
                  IP: {{ rule.ip.join(', ') }}
                </span>
                <span v-if="rule.protocol?.length" class="px-2 py-0.5 rounded bg-amber-500/15 text-amber-300 font-mono text-[11px] border border-amber-500/20">
                  协议: {{ rule.protocol.join(', ') }}
                </span>
                <span v-if="rule.port" class="px-2 py-0.5 rounded bg-rose-500/15 text-rose-300 font-mono text-[11px] border border-rose-500/20">
                  端口: {{ rule.port }}
                </span>
                <span v-if="rule.network" class="px-2 py-0.5 rounded bg-gray-800 text-gray-300 font-mono text-[11px]">
                  网络: {{ rule.network }}
                </span>
              </div>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex items-center gap-1.5 self-end sm:self-center shrink-0">
            <button
              @click="moveRule(Number(idx), -1)"
              :disabled="Number(idx) === 0"
              class="p-1.5 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 disabled:opacity-30 transition-colors"
              title="上移优先级"
            >
              <ArrowUp class="w-3.5 h-3.5" />
            </button>
            <button
              @click="moveRule(Number(idx), 1)"
              :disabled="Number(idx) === (routingConfig.rules?.length || 0) - 1"
              class="p-1.5 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 disabled:opacity-30 transition-colors"
              title="下移优先级"
            >
              <ArrowDown class="w-3.5 h-3.5" />
            </button>
            <button
              @click="editRule(Number(idx))"
              class="p-1.5 rounded-lg bg-gray-800 hover:bg-gray-700 text-brand-400 transition-colors"
              title="编辑规则"
            >
              <Edit3 class="w-3.5 h-3.5" />
            </button>
            <button
              @click="deleteRule(Number(idx))"
              class="p-1.5 rounded-lg bg-gray-800 hover:bg-rose-500/20 text-rose-400 transition-colors"
              title="删除规则"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Rule Edit Modal (Fully Decoupled Dropdowns) -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-xl p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-5 my-8">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white">{{ isEditingRule ? '编辑分流规则' : '添加分流规则' }}</h2>
            <p class="text-xs text-gray-400 mt-0.5">从已有的入站与出站标签中动态选择绑定</p>
          </div>
          <button @click="showModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <form @submit.prevent="saveRuleInModal" class="space-y-4 text-xs">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <!-- 动态 OutboundTag 下拉选择 -->
            <div>
              <label class="block text-gray-300 mb-1 font-medium">目标出站 (OutboundTag)</label>
              <div class="space-y-1.5">
                <select
                  v-model="ruleForm.outboundTag"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                >
                  <option v-for="tag in availableOutboundTags" :key="tag" :value="tag">
                    {{ tag }}
                  </option>
                  <option value="__custom__">+ 手动输入自定义 Tag</option>
                </select>

                <input
                  v-if="ruleForm.outboundTag === '__custom__' || !availableOutboundTags.includes(ruleForm.outboundTag)"
                  v-model="ruleForm.customOutboundTag"
                  type="text"
                  placeholder="输入自定义 OutboundTag"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-1.5 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>

            <!-- 动态 InboundTag 匹配选择 -->
            <div>
              <label class="block text-gray-300 mb-1 font-medium">来源入站 (InboundTag 可选)</label>
              <select
                v-model="ruleForm.selectedInboundTag"
                @change="onSelectInboundTag"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              >
                <option value="">全部入站 (默认不限制)</option>
                <option v-for="tag in availableInboundTags" :key="tag" :value="tag">
                  {{ tag }}
                </option>
              </select>
              <input
                v-model="ruleForm.inboundTagsStr"
                type="text"
                placeholder="或用逗号隔开多个入站Tag (如 api, vless-reality)"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-1.5 mt-1.5 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <div>
            <label class="block text-gray-300 mb-1 font-medium">域名匹配列表 (Domain，多个用逗号或换行隔开)</label>
            <textarea
              v-model="ruleForm.domainStr"
              rows="3"
              placeholder="geosite:category-ads-all, geosite:cn, domain:google.com"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl p-3 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
            ></textarea>
          </div>

          <div>
            <label class="block text-gray-300 mb-1 font-medium">IP 匹配列表 (IP，多个用逗号或换行隔开)</label>
            <textarea
              v-model="ruleForm.ipStr"
              rows="3"
              placeholder="geoip:cn, geoip:private, 192.168.0.0/16"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl p-3 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
            ></textarea>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label class="block text-gray-300 mb-1 font-medium">端口 (Port)</label>
              <input
                v-model="ruleForm.port"
                type="text"
                placeholder="25 或 80,443 或 1-1024"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div>
              <label class="block text-gray-300 mb-1 font-medium">传输层协议 (Network)</label>
              <select
                v-model="ruleForm.network"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
              >
                <option value="">全部 (TCP + UDP)</option>
                <option value="tcp">仅 TCP</option>
                <option value="udp">仅 UDP</option>
              </select>
            </div>

            <div>
              <label class="block text-gray-300 mb-1 font-medium">应用协议 (Protocol)</label>
              <input
                v-model="ruleForm.protocolStr"
                type="text"
                placeholder="bittorrent, http, tls"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <div class="flex justify-end gap-3 pt-2 border-t border-gray-800">
            <button
              type="button"
              @click="showModal = false"
              class="px-5 py-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 font-medium text-xs transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              class="px-5 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-semibold text-xs rounded-xl transition-all shadow-lg shadow-brand-500/25"
            >
              确认并暂存
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Smart Preset Wizard Modal -->
    <div v-if="showPresetModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-md p-6 sm:p-7 rounded-3xl border border-gray-800 shadow-2xl space-y-5">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-base font-bold text-white">{{ presetData.title }}</h2>
            <p class="text-xs text-gray-400 mt-0.5">{{ presetData.description }}</p>
          </div>
          <button @click="showPresetModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <div class="space-y-3 text-xs">
          <div>
            <label class="block text-gray-300 mb-1 font-medium">选择目标出站 (OutboundTag)</label>
            <select
              v-model="presetData.selectedOutbound"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
            >
              <option v-for="tag in availableOutboundTags" :key="tag" :value="tag">
                {{ tag }}
              </option>
            </select>
          </div>

          <div v-if="!availableOutboundTags.includes(presetData.selectedOutbound)" class="p-2.5 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-300 text-[11px]">
            ⚠️ 当前系统中未检测到 <code>{{ presetData.selectedOutbound }}</code> 出站，请选择现有出站或先在「出站代理」页面创建。
          </div>

          <div class="p-3 rounded-xl bg-gray-900 border border-gray-800 space-y-1 font-mono text-[11px] text-gray-400">
            <span class="block text-gray-300 font-bold">即将注入的匹配条件:</span>
            <div v-for="(rule, i) in presetData.rules" :key="i">
              - {{ formatRuleSummary(rule) }}
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-3 pt-2 border-t border-gray-800">
          <button
            type="button"
            @click="showPresetModal = false"
            class="px-4 py-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs transition-colors"
          >
            取消
          </button>
          <button
            type="button"
            @click="confirmInjectPreset"
            class="px-5 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-bold text-xs rounded-xl transition-all shadow-lg shadow-brand-500/25"
          >
            确认注入规则
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Plus, Check, ArrowUp, ArrowDown, Edit3, Trash2, Database, RefreshCw } from 'lucide-vue-next'
import { toast } from '../utils/toast'
import api from '../api'

const routingConfig = ref<any>({
  domainStrategy: 'IPIfNonMatch',
  rules: [],
})

const inboundsList = ref<any[]>([])
const outboundsList = ref<any[]>([])
const geodataStatus = ref<any>(null)
const updatingGeo = ref(false)

const showModal = ref(false)
const isEditingRule = ref(false)
const editingIndex = ref(-1)
const saving = ref(false)

const fetchGeoStatus = async () => {
  try {
    const res: any = await api.get('/geodata/status')
    geodataStatus.value = res
  } catch (err) {
    console.error(err)
  }
}

const updateGeoData = async () => {
  if (!confirm('确定在线更新 GeoIP / GeoSite 规则库吗？更新完成后将自动重载 Xray 核心。')) return
  updatingGeo.value = true
  try {
    await api.post('/geodata/update')
    toast.success('GeoData 规则库更新成功！')
    await fetchGeoStatus()
  } catch (err: any) {
    toast.error('更新失败: ' + err)
  } finally {
    updatingGeo.value = false
  }
}

// Preset Wizard State
const showPresetModal = ref(false)
const presetData = ref<any>({
  type: '',
  title: '',
  description: '',
  selectedOutbound: 'block',
  rules: [],
})

const ruleForm = ref<any>({
  outboundTag: 'direct',
  customOutboundTag: '',
  selectedInboundTag: '',
  inboundTagsStr: '',
  domainStr: '',
  ipStr: '',
  port: '',
  network: '',
  protocolStr: '',
})

const availableOutboundTags = computed(() => {
  const tags = outboundsList.value.map((o) => o.tag).filter((t) => t)
  if (!tags.includes('direct')) tags.unshift('direct')
  if (!tags.includes('block')) tags.push('block')
  return tags
})

const availableInboundTags = computed(() => {
  return inboundsList.value.map((i) => i.tag).filter((t) => t)
})

const fetchAllDependencies = async () => {
  try {
    const [routeRes, inbRes, outRes]: any = await Promise.all([
      api.get('/routing'),
      api.get('/inbounds'),
      api.get('/outbounds'),
    ])
    routingConfig.value = routeRes || { domainStrategy: 'IPIfNonMatch', rules: [] }
    inboundsList.value = inbRes || []
    outboundsList.value = outRes || []
  } catch (err) {
    console.error(err)
  }
}

const isKnownOutbound = (tag: string) => {
  return availableOutboundTags.value.includes(tag) || tag === 'api'
}

const openAddRuleModal = () => {
  isEditingRule.value = false
  editingIndex.value = -1
  const defaultOutbound = availableOutboundTags.value.includes('block') ? 'block' : (availableOutboundTags.value[0] || 'direct')
  ruleForm.value = {
    outboundTag: defaultOutbound,
    customOutboundTag: '',
    selectedInboundTag: '',
    inboundTagsStr: '',
    domainStr: '',
    ipStr: '',
    port: '',
    network: '',
    protocolStr: '',
  }
  showModal.value = true
}

const onSelectInboundTag = () => {
  if (ruleForm.value.selectedInboundTag) {
    const current = parseArray(ruleForm.value.inboundTagsStr) || []
    if (!current.includes(ruleForm.value.selectedInboundTag)) {
      current.push(ruleForm.value.selectedInboundTag)
      ruleForm.value.inboundTagsStr = current.join(', ')
    }
  }
}

const editRule = (idx: number) => {
  isEditingRule.value = true
  editingIndex.value = idx
  const r = routingConfig.value.rules[idx]
  const isCustom = !availableOutboundTags.value.includes(r.outboundTag)

  ruleForm.value = {
    outboundTag: isCustom ? '__custom__' : r.outboundTag,
    customOutboundTag: isCustom ? r.outboundTag : '',
    selectedInboundTag: '',
    inboundTagsStr: (r.inboundTag || []).join(', '),
    domainStr: (r.domain || []).join(',\n'),
    ipStr: (r.ip || []).join(',\n'),
    port: r.port || '',
    network: r.network || '',
    protocolStr: (r.protocol || []).join(', '),
  }
  showModal.value = true
}

const parseArray = (str: string) => {
  if (!str) return undefined
  const items = str
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s)
  return items.length > 0 ? items : undefined
}

const saveRuleInModal = () => {
  let targetOutbound = ruleForm.value.outboundTag
  if (targetOutbound === '__custom__') {
    targetOutbound = ruleForm.value.customOutboundTag.trim() || 'direct'
  }

  const newRule: any = {
    outboundTag: targetOutbound,
  }
  const inbounds = parseArray(ruleForm.value.inboundTagsStr)
  if (inbounds) newRule.inboundTag = inbounds

  const domains = parseArray(ruleForm.value.domainStr)
  if (domains) newRule.domain = domains

  const ips = parseArray(ruleForm.value.ipStr)
  if (ips) newRule.ip = ips

  if (ruleForm.value.port) newRule.port = ruleForm.value.port
  if (ruleForm.value.network) newRule.network = ruleForm.value.network

  const protos = parseArray(ruleForm.value.protocolStr)
  if (protos) newRule.protocol = protos

  if (isEditingRule.value && editingIndex.value >= 0) {
    routingConfig.value.rules[editingIndex.value] = newRule
  } else {
    routingConfig.value.rules.push(newRule)
  }
  showModal.value = false
}

const deleteRule = (idx: number) => {
  routingConfig.value.rules.splice(idx, 1)
}

const moveRule = (idx: number, step: number) => {
  const target = idx + step
  if (target < 0 || target >= routingConfig.value.rules.length) return
  const temp = routingConfig.value.rules[idx]
  routingConfig.value.rules[idx] = routingConfig.value.rules[target]
  routingConfig.value.rules[target] = temp
}

// 智能预设向导 (Smart Preset Wizard)
const openPresetWizard = (type: string) => {
  let defOutbound = 'block'
  if (type === 'cn') {
    defOutbound = availableOutboundTags.value.includes('warp-out')
      ? 'warp-out'
      : (availableOutboundTags.value.includes('direct') ? 'direct' : availableOutboundTags.value[0] || 'direct')
  } else {
    defOutbound = availableOutboundTags.value.includes('block')
      ? 'block'
      : (availableOutboundTags.value[0] || 'block')
  }

  if (type === 'ads') {
    presetData.value = {
      type: 'ads',
      title: '🛡️ 广告拦截预设向导',
      description: '拦截常见广告联盟与追踪域名',
      selectedOutbound: defOutbound,
      rules: [{ domain: ['geosite:category-ads-all'] }],
    }
  } else if (type === 'private_ip') {
    presetData.value = {
      type: 'private_ip',
      title: '🔒 局域网私有 IP 屏蔽向导',
      description: '防止穿透访问服务器内网私有地址',
      selectedOutbound: defOutbound,
      rules: [{ ip: ['geoip:private'] }],
    }
  } else if (type === 'bt') {
    presetData.value = {
      type: 'bt',
      title: '🚫 BT/BitTorrent 拦截向导',
      description: '防止服务器遭遇版权版权投诉 (DMCA)',
      selectedOutbound: defOutbound,
      rules: [{ protocol: ['bittorrent'] }],
    }
  } else if (type === 'smtp') {
    presetData.value = {
      type: 'smtp',
      title: '📧 邮件 25 端口封锁向导',
      description: '防止滥用服务器发送垃圾邮件',
      selectedOutbound: defOutbound,
      rules: [{ port: '25', network: 'tcp' }],
    }
  } else if (type === 'cn') {
    presetData.value = {
      type: 'cn',
      title: '⚡ 国内流量分流向导',
      description: '将国内域名与 IP 路由至指定出站（如 WARP 或直连）',
      selectedOutbound: defOutbound,
      rules: [{ domain: ['geosite:cn'] }, { ip: ['geoip:cn'] }],
    }
  }

  showPresetModal.value = true
}

const confirmInjectPreset = () => {
  for (const r of presetData.value.rules) {
    routingConfig.value.rules.push({
      ...r,
      outboundTag: presetData.value.selectedOutbound,
    })
  }
  showPresetModal.value = false
}

const formatRuleSummary = (rule: any) => {
  if (rule.domain) return `域名: ${rule.domain.join(', ')}`
  if (rule.ip) return `IP: ${rule.ip.join(', ')}`
  if (rule.protocol) return `协议: ${rule.protocol.join(', ')}`
  if (rule.port) return `端口: ${rule.port} (${rule.network || '全部'})`
  return JSON.stringify(rule)
}

const saveAllRouting = async () => {
  saving.value = true
  try {
    await api.post('/routing', routingConfig.value)
    toast.success('路由分流配置已保存并平滑生效！')
    await fetchAllDependencies()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const outboundBadgeColor = (tag: string) => {
  switch (tag?.toLowerCase()) {
    case 'direct':
      return 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/20'
    case 'block':
      return 'bg-rose-500/15 text-rose-300 border border-rose-500/20'
    case 'warp-out':
      return 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/20'
    case 'api':
      return 'bg-purple-500/15 text-purple-300 border border-purple-500/20'
    default:
      return 'bg-indigo-500/15 text-indigo-300 border border-indigo-500/20'
  }
}

onMounted(() => {
  fetchAllDependencies()
  fetchGeoStatus()
})
</script>
