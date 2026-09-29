<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">路由与分流策略 (Routing)</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ routingConfig.rules?.length || 0 }} 条规则
          </Badge>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20">
            修改自动重启核心生效
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          自上而下优先匹配分流规则，出站与入站标签动态绑定，保存落盘后自动全量重启核心生效
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          @click="openAddRuleDrawer"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加规则</span>
        </Button>

        <Button
          variant="default"
          size="sm"
          :loading="saving"
          @click="saveAllRouting"
        >
          <Check class="w-3.5 h-3.5 mr-1" />
          <span>{{ saving ? '保存中...' : '保存并应用' }}</span>
        </Button>
      </div>
    </div>

    <!-- GeoData 规则库在线升级状态卡片 -->
    <div class="p-3.5 rounded-lg bg-card border border-border flex flex-col gap-3">
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <div class="p-2 rounded bg-neutral-900 border border-border text-foreground">
            <Database class="w-4 h-4 text-cyan-400" />
          </div>
          <div class="space-y-0.5 text-xs">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground">GeoData 分流规则库 (geoip.dat & geosite.dat)</span>
              <span class="px-1.5 py-0.2 rounded text-[10px] font-mono bg-neutral-900 border border-border text-cyan-400">
                {{ geodataStatus?.platform || 'Xray Core' }}
              </span>
            </div>
            <p class="text-[11px] text-muted-foreground font-mono">
              GeoIP: {{ geodataStatus?.geoipExists ? `${(geodataStatus.geoipSize / 1048576).toFixed(2)} MB` : '未找到' }} | 
              GeoSite: {{ geodataStatus?.geositeExists ? `${(geodataStatus.geositeSize / 1048576).toFixed(2)} MB` : '未找到' }} | 
              路径: {{ geodataStatus?.targetDirectory || './' }}
            </p>
          </div>
        </div>

        <Button
          variant="secondary"
          size="sm"
          :loading="updatingGeo"
          @click="updateGeoData"
          class="shrink-0"
        >
          <RotateCw class="w-3.5 h-3.5 mr-1.5" :class="{ 'animate-spin': updatingGeo }" />
          <span>{{ updatingGeo ? '规则库升级中...' : '一键升级规则库' }}</span>
        </Button>
      </div>

      <!-- 实时下载进度条 -->
      <div v-if="updatingGeo || (geoProgress.percentage > 0 && geoProgress.percentage < 100)" class="p-2.5 bg-neutral-950 rounded-md border border-border space-y-1.5 transition-all">
        <div class="flex items-center justify-between text-xs font-mono">
          <div class="flex items-center gap-2">
            <span class="w-1.5 h-1.5 rounded-full" :class="updatingGeo ? 'bg-cyan-400 animate-ping' : 'bg-emerald-400'"></span>
            <span class="text-muted-foreground text-[11px]">
              {{ geoProgress.message || '正在准备下载...' }}
            </span>
          </div>
          <span class="font-bold text-cyan-400 text-xs">
            {{ geoProgress.percentage }}%
          </span>
        </div>

        <div class="w-full h-1.5 bg-neutral-900 rounded-full overflow-hidden border border-border/60">
          <div
            class="h-full rounded-full bg-cyan-500 transition-all duration-300"
            :style="{ width: `${geoProgress.percentage}%` }"
          ></div>
        </div>
      </div>
    </div>

    <!-- Domain Strategy & Presets Bar -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">
      <!-- Strategy Selector -->
      <div class="p-3.5 rounded-lg bg-card border border-border space-y-2">
        <label class="block text-xs font-semibold text-foreground uppercase tracking-wider font-mono">
          域名解析策略 (domainStrategy)
        </label>
        <select
          v-model="routingConfig.domainStrategy"
          class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
        >
          <option value="IPIfNonMatch">IPIfNonMatch (推荐: 未命中则解析为IP再次匹配)</option>
          <option value="AsIs">AsIs (保持原样: 仅匹配客户端直发域名)</option>
          <option value="IPOnDemand">IPOnDemand (强制实时解析IP匹配)</option>
        </select>
        <p class="text-[11px] text-muted-foreground leading-relaxed">
          配合 Inbound 域名嗅探 (Sniffing)，精准识别 TLS/HTTP 连接真实域名。
        </p>
      </div>

      <!-- Presets quick buttons (Interactive Wizard) -->
      <div class="p-3.5 rounded-lg bg-card border border-border lg:col-span-2 space-y-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono">
            智能预设向导 (快速注入常用分流规则)
          </span>
          <span class="text-[11px] text-muted-foreground font-mono">点击唤起配置向导</span>
        </div>

        <div class="flex flex-wrap gap-2 pt-1">
          <button
            @click="openPresetWizard('ads')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>🛡️ 拦截广告 (category-ads)</span>
          </button>

          <button
            @click="openPresetWizard('private_ip')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>🔒 屏蔽私有局域网 (private)</span>
          </button>

          <button
            @click="openPresetWizard('bt')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>🚫 拦截 BT 下载 (bittorrent)</span>
          </button>

          <button
            @click="openPresetWizard('smtp')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>📧 封禁邮件 25 端口 (port: 25)</span>
          </button>

          <button
            @click="openPresetWizard('cn')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>⚡ 国内流量分流 (geosite:cn)</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Rules Table-First List -->
    <div class="relative w-full overflow-hidden rounded-lg border border-border bg-neutral-950">
      <table class="w-full caption-bottom text-xs border-collapse">
        <thead class="border-b border-border bg-neutral-900 font-mono text-[11px]">
          <tr>
            <th class="h-9 px-3 text-center align-middle font-medium text-muted-foreground uppercase tracking-wider w-12">次序</th>
            <th class="h-9 px-3 text-left align-middle font-medium text-muted-foreground uppercase tracking-wider">目标出站 (OutboundTag)</th>
            <th class="h-9 px-3 text-left align-middle font-medium text-muted-foreground uppercase tracking-wider">来源入站 (Inbounds)</th>
            <th class="h-9 px-3 text-left align-middle font-medium text-muted-foreground uppercase tracking-wider">匹配条件矩阵</th>
            <th class="h-9 px-3 text-right align-middle font-medium text-muted-foreground uppercase tracking-wider w-36">优先级与操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr
            v-for="(rule, idx) in routingConfig.rules"
            :key="idx"
            class="hover:bg-muted/20 transition-colors"
          >
            <!-- 1. Index / Priority -->
            <td class="p-3 text-center align-middle font-mono text-muted-foreground font-semibold text-xs">
              #{{ Number(idx) + 1 }}
            </td>

            <!-- 2. Target Outbound -->
            <td class="p-3 align-middle font-mono">
              <div class="flex items-center gap-1.5">
                <Badge :variant="getOutboundBadgeVariant(rule.outboundTag)">
                  {{ rule.outboundTag }}
                </Badge>
                <span v-if="!isKnownOutbound(rule.outboundTag)" class="text-amber-400 text-[10px] font-semibold" title="系统中未配置该出站标签">
                  ⚠️ 未知出站
                </span>
              </div>
            </td>

            <!-- 3. Source Inbounds -->
            <td class="p-3 align-middle font-mono text-[11px]">
              <span v-if="rule.inboundTag?.length" class="text-foreground">
                {{ rule.inboundTag.join(', ') }}
              </span>
              <span v-else class="text-muted-foreground">全部入站</span>
            </td>

            <!-- 4. Matching Conditions -->
            <td class="p-3 align-middle">
              <div class="flex flex-wrap items-center gap-1.5 font-mono text-[11px]">
                <span v-if="rule.vlessRoute" class="px-1.5 py-0.5 rounded bg-indigo-950/60 text-indigo-300 border border-indigo-800/50">
                  vlessRoute: {{ rule.vlessRoute }}
                </span>
                <span v-if="rule.domain?.length" class="px-1.5 py-0.5 rounded bg-blue-950/60 text-blue-300 border border-blue-800/50">
                  域名: {{ rule.domain.join(', ') }}
                </span>
                <span v-if="rule.ip?.length" class="px-1.5 py-0.5 rounded bg-purple-950/60 text-purple-300 border border-purple-800/50">
                  IP: {{ rule.ip.join(', ') }}
                </span>
                <span v-if="rule.protocol?.length" class="px-1.5 py-0.5 rounded bg-amber-950/60 text-amber-300 border border-amber-800/50">
                  协议: {{ rule.protocol.join(', ') }}
                </span>
                <span v-if="rule.port" class="px-1.5 py-0.5 rounded bg-rose-950/60 text-rose-300 border border-rose-800/50">
                  端口: {{ rule.port }}
                </span>
                <span v-if="rule.network" class="px-1.5 py-0.5 rounded bg-neutral-900 text-neutral-300 border border-border">
                  网络: {{ rule.network }}
                </span>
                <span v-if="!rule.vlessRoute && !rule.domain?.length && !rule.ip?.length && !rule.protocol?.length && !rule.port && !rule.network" class="text-muted-foreground text-xs font-sans">
                  无额外限制 (匹配任意流量)
                </span>
              </div>
            </td>

            <!-- 5. Actions -->
            <td class="p-3 align-middle text-right">
              <div class="flex items-center justify-end gap-1">
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 w-7 p-0"
                  :disabled="Number(idx) === 0"
                  @click="moveRule(Number(idx), -1)"
                  title="上移优先级"
                >
                  <ArrowUp class="w-3.5 h-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 w-7 p-0"
                  :disabled="Number(idx) === (routingConfig.rules?.length || 0) - 1"
                  @click="moveRule(Number(idx), 1)"
                  title="下移优先级"
                >
                  <ArrowDown class="w-3.5 h-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 w-7 p-0 text-brand-400 hover:text-brand-300"
                  @click="editRule(Number(idx))"
                  title="编辑规则"
                >
                  <Edit3 class="w-3.5 h-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 w-7 p-0 text-rose-400 hover:text-rose-300 hover:bg-rose-950/40"
                  @click="deleteRule(Number(idx))"
                  title="删除规则"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </Button>
              </div>
            </td>
          </tr>

          <!-- Empty State -->
          <tr v-if="!routingConfig.rules?.length">
            <td colspan="5" class="p-8 text-center text-muted-foreground text-xs">
              暂无路由分流规则，点击上方「添加规则」或使用智能预设
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Rule Edit Drawer (Replacing old centered modal) -->
    <Drawer
      v-model="showRuleDrawer"
      :title="isEditingRule ? `编辑分流规则 #${editingIndex + 1}` : '添加分流规则'"
      description="从已有的入站与出站标签中动态绑定，配置即时暂存"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <form id="rule-form" @submit.prevent="saveRuleInDrawer" class="space-y-4 text-xs">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <!-- 动态 OutboundTag 下拉选择 -->
          <div>
            <label class="block text-foreground mb-1 font-medium">目标出站 (OutboundTag)</label>
            <div class="space-y-1.5">
              <select
                v-model="ruleForm.outboundTag"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
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
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-1.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <!-- 动态 InboundTag 匹配选择 -->
          <div>
            <label class="block text-foreground mb-1 font-medium">来源入站 (InboundTag 可选)</label>
            <select
              v-model="ruleForm.selectedInboundTag"
              @change="onSelectInboundTag"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
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
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-1.5 mt-1.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">
            VLESS 协议路由匹配 (vlessRoute，可选)
          </label>
          <input
            v-model="ruleForm.vlessRoute"
            type="text"
            placeholder="例如: 1 或 2 或 1,2 或 100-200 (匹配客户端 VLESS UUID 中携带的 16 位路由编号)"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-0.5">Xray 官方 VLESS 协议级多节点路由分流条件，支持单值、列表或范围匹配</p>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">域名匹配列表 (Domain，多个用逗号或换行隔开)</label>
          <textarea
            v-model="ruleForm.domainStr"
            rows="3"
            placeholder="geosite:category-ads-all, geosite:cn, domain:google.com"
            class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring leading-relaxed"
          ></textarea>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">IP 匹配列表 (IP，多个用逗号或换行隔开)</label>
          <textarea
            v-model="ruleForm.ipStr"
            rows="3"
            placeholder="geoip:cn, geoip:private, 192.168.0.0/16"
            class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring leading-relaxed"
          ></textarea>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label class="block text-foreground mb-1 font-medium">端口 (Port)</label>
            <input
              v-model="ruleForm.port"
              type="text"
              placeholder="25 或 80,443 或 1-1024"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          <div>
            <label class="block text-foreground mb-1 font-medium">传输层协议 (Network)</label>
            <select
              v-model="ruleForm.network"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">全部 (TCP + UDP)</option>
              <option value="tcp">仅 TCP</option>
              <option value="udp">仅 UDP</option>
            </select>
          </div>

          <div>
            <label class="block text-foreground mb-1 font-medium">应用协议 (Protocol)</label>
            <input
              v-model="ruleForm.protocolStr"
              type="text"
              placeholder="bittorrent, http, tls"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>
      </form>

      <template #footer>
        <Button variant="secondary" size="sm" @click="showRuleDrawer = false">
          取消
        </Button>
        <Button
          type="submit"
          form="rule-form"
          variant="default"
          size="sm"
        >
          <span>确认并暂存</span>
        </Button>
      </template>
    </Drawer>

    <!-- Smart Preset Wizard Modal -->
    <div v-if="showPresetModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75">
      <div class="w-full max-w-md p-5 rounded-lg border border-border bg-card shadow-2xl space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <div>
            <h2 class="text-sm font-bold text-foreground">{{ presetData.title }}</h2>
            <p class="text-xs text-muted-foreground mt-0.5">{{ presetData.description }}</p>
          </div>
          <button @click="showPresetModal = false" class="text-muted-foreground hover:text-foreground text-base">✕</button>
        </div>

        <div class="space-y-3 text-xs">
          <div>
            <label class="block text-foreground mb-1 font-medium">选择目标出站 (OutboundTag)</label>
            <select
              v-model="presetData.selectedOutbound"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option v-for="tag in availableOutboundTags" :key="tag" :value="tag">
                {{ tag }}
              </option>
            </select>
          </div>

          <div v-if="!availableOutboundTags.includes(presetData.selectedOutbound)" class="p-2.5 rounded-md bg-amber-950/40 border border-amber-800/50 text-amber-300 text-[11px]">
            ⚠️ 当前系统中未检测到 <code>{{ presetData.selectedOutbound }}</code> 出站，请选择现有出站或先在「出站代理」页面创建。
          </div>

          <div class="p-3 rounded-md bg-neutral-950 border border-border space-y-1 font-mono text-[11px] text-muted-foreground">
            <span class="block text-foreground font-bold">即将注入的匹配条件:</span>
            <div v-for="(rule, i) in presetData.rules" :key="i">
              - {{ formatRuleSummary(rule) }}
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-2.5 pt-2 border-t border-border">
          <Button
            variant="secondary"
            size="sm"
            @click="showPresetModal = false"
          >
            取消
          </Button>
          <Button
            variant="default"
            size="sm"
            @click="confirmInjectPreset"
          >
            确认注入规则
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Plus, Check, ArrowUp, ArrowDown, Edit3, Trash2, Database, RotateCw } from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import Drawer from '../components/ui/Drawer.vue'
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

const showRuleDrawer = ref(false)
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

const geoProgress = ref<any>({
  isUpdating: false,
  percentage: 0,
  message: '',
  speedBps: 0,
})
let progressTimer: any = null

const pollGeoProgress = async () => {
  try {
    const res: any = await api.get('/geodata/progress')
    if (res) {
      geoProgress.value = res
      if (!res.isUpdating) {
        if (progressTimer) {
          clearInterval(progressTimer)
          progressTimer = null
        }
        updatingGeo.value = false
        if (res.step === 'done') {
          toast.success(res.message || 'GeoData 规则库更新成功！')
          await fetchGeoStatus()
          setTimeout(() => {
            geoProgress.value.percentage = 0
          }, 4000)
        } else if (res.step === 'error') {
          toast.error(res.message || '更新规则库失败')
        }
      }
    }
  } catch (e) {
    console.error('poll progress failed', e)
  }
}

const updateGeoData = async () => {
  if (!confirm('确定在线更新 GeoIP / GeoSite 规则库吗？更新完成后将自动重载 Xray 核心。')) return
  updatingGeo.value = true
  geoProgress.value = { isUpdating: true, percentage: 5, message: '正在启动后台下载引擎...' }
  try {
    await api.post('/geodata/update')
    if (progressTimer) clearInterval(progressTimer)
    progressTimer = setInterval(pollGeoProgress, 500)
  } catch (err: any) {
    updatingGeo.value = false
    toast.error('触发更新失败: ' + err)
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

const openAddRuleDrawer = () => {
  isEditingRule.value = false
  editingIndex.value = -1
  const defaultOutbound = availableOutboundTags.value.includes('block') ? 'block' : (availableOutboundTags.value[0] || 'direct')
  ruleForm.value = {
    outboundTag: defaultOutbound,
    customOutboundTag: '',
    selectedInboundTag: '',
    inboundTagsStr: '',
    vlessRoute: '',
    domainStr: '',
    ipStr: '',
    port: '',
    network: '',
    protocolStr: '',
  }
  showRuleDrawer.value = true
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
    vlessRoute: r.vlessRoute || '',
    domainStr: (r.domain || []).join(',\n'),
    ipStr: (r.ip || []).join(',\n'),
    port: r.port || '',
    network: r.network || '',
    protocolStr: (r.protocol || []).join(', '),
  }
  showRuleDrawer.value = true
}

const parseArray = (str: string) => {
  if (!str) return undefined
  const items = str
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s)
  return items.length > 0 ? items : undefined
}

const saveRuleInDrawer = () => {
  let targetOutbound = ruleForm.value.outboundTag
  if (targetOutbound === '__custom__') {
    targetOutbound = ruleForm.value.customOutboundTag.trim() || 'direct'
  }

  const newRule: any = {
    outboundTag: targetOutbound,
  }
  if (ruleForm.value.vlessRoute && ruleForm.value.vlessRoute.trim()) {
    newRule.vlessRoute = ruleForm.value.vlessRoute.trim()
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
  showRuleDrawer.value = false
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
  if (rule.vlessRoute) return `vlessRoute: ${rule.vlessRoute}`
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

const getOutboundBadgeVariant = (tag: string): 'default' | 'secondary' | 'outline' | 'destructive' => {
  switch (tag?.toLowerCase()) {
    case 'direct':
      return 'default'
    case 'block':
      return 'destructive'
    case 'warp-out':
      return 'secondary'
    default:
      return 'outline'
  }
}

onMounted(() => {
  fetchAllDependencies()
  fetchGeoStatus()
})
</script>
