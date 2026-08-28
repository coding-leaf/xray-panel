<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">出站代理管理 (Outbounds)</h1>
        <p class="text-xs text-gray-400 mt-0.5">配置直连、黑洞拦截、Cloudflare WARP (WireGuard) 与链式上游代理</p>
      </div>
      <button
        @click="openCreateModal"
        class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
      >
        <Plus class="w-4 h-4" />
        <span>添加出站节点</span>
      </button>
    </div>

    <!-- Outbounds Cards -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
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
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>用途说明</span>
              <span class="text-gray-300">{{ getUsageDesc(ob) }}</span>
            </div>
            <div class="py-1">
              <span class="block mb-1 text-gray-500 text-[11px]">设置详情摘要:</span>
              <div class="bg-gray-900/80 p-2 rounded-xl text-[11px] font-mono text-gray-300 truncate">
                {{ formatSettingsSummary(ob) }}
              </div>
            </div>
          </div>
        </div>

        <div class="mt-4 pt-3 border-t border-gray-800/60 flex items-center justify-between">
          <button
            @click="editOutbound(ob)"
            class="text-xs text-brand-400 hover:text-brand-300 font-medium transition-colors"
          >
            编辑设置
          </button>
          <button
            v-if="ob.tag !== 'direct' && ob.tag !== 'block'"
            @click="deleteOutbound(ob.tag)"
            class="text-xs text-red-400 hover:text-red-300 transition-colors"
          >
            删除
          </button>
          <span v-else class="text-xs text-gray-600 font-mono">系统保留</span>
        </div>
      </div>
    </div>

    <!-- Outbound Edit / Create Modal -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-xl p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-5 my-8">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white">{{ isEditing ? '编辑出站节点' : '添加新出站节点' }}</h2>
            <p class="text-xs text-gray-400 mt-0.5">配置将同步回写磁盘 config.json 并平滑应用</p>
          </div>
          <button @click="showModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <form @submit.prevent="saveOutbound" class="space-y-4 text-xs">
          <div>
            <label class="block text-gray-300 mb-1 font-medium">出站标识 (Tag)</label>
            <input
              v-model="form.tag"
              type="text"
              required
              :disabled="isEditing"
              placeholder="warp-out 或 proxy-jp"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500 disabled:opacity-50"
            />
          </div>

          <div>
            <label class="block text-gray-300 mb-1 font-medium">出站协议 (Protocol)</label>
            <select
              v-model="form.protocol"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
            >
              <option value="freedom">Freedom (直连访问)</option>
              <option value="blackhole">Blackhole (黑洞丢弃/拦截)</option>
              <option value="wireguard">WireGuard (Cloudflare WARP 出站)</option>
              <option value="socks">Socks 代理出站</option>
              <option value="http">HTTP 代理出站</option>
              <option value="shadowsocks">Shadowsocks 出站</option>
              <option value="trojan">Trojan 出站</option>
            </select>
          </div>

          <!-- Freedom 专属配置 -->
          <div v-if="form.protocol === 'freedom'" class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800">
            <h3 class="font-bold text-emerald-400 text-xs">Freedom 策略</h3>
            <div>
              <label class="block text-gray-400 mb-1">域名解析策略 (domainStrategy)</label>
              <select
                v-model="form.freedomDomainStrategy"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
              >
                <option value="UseIP">UseIP</option>
                <option value="UseIPv4">UseIPv4 (强制IPv4)</option>
                <option value="UseIPv6">UseIPv6 (强制IPv6)</option>
                <option value="AsIs">AsIs (保持原样)</option>
              </select>
            </div>
          </div>

          <!-- Blackhole 专属配置 -->
          <div v-else-if="form.protocol === 'blackhole'" class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800">
            <h3 class="font-bold text-rose-400 text-xs">Blackhole 拦截响应</h3>
            <div>
              <label class="block text-gray-400 mb-1">响应类型 (Response Type)</label>
              <select
                v-model="form.blackholeResponse"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
              >
                <option value="none">none (直接静默丢弃)</option>
                <option value="http">http (返回 HTTP 403 阻断页面)</option>
              </select>
            </div>
          </div>

          <!-- WireGuard / WARP 专属配置 -->
          <div v-else-if="form.protocol === 'wireguard'" class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800">
            <h3 class="font-bold text-cyan-400 text-xs">WireGuard (Cloudflare WARP) 详情</h3>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-gray-400 mb-1">Secret Key (私钥)</label>
                <input
                  v-model="form.wgSecretKey"
                  type="text"
                  placeholder="APHekUQCpV3neCk1BnXOXgte7eLCBNfB2iZ8yVyQs0s="
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
                />
              </div>
              <div>
                <label class="block text-gray-400 mb-1">Peer Public Key (对端公钥)</label>
                <input
                  v-model="form.wgPeerPublicKey"
                  type="text"
                  placeholder="bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo="
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-gray-400 mb-1">Endpoint (对端服务器)</label>
                <input
                  v-model="form.wgEndpoint"
                  type="text"
                  placeholder="162.159.192.1:2408"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
              <div>
                <label class="block text-gray-400 mb-1">本地地址 (Address)</label>
                <input
                  v-model="form.wgAddress"
                  type="text"
                  placeholder="172.16.0.2/32"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>
          </div>

          <!-- Proxy outbounds (Socks / HTTP) -->
          <div v-else class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800">
            <h3 class="font-bold text-indigo-400 text-xs">代理服务器连接信息</h3>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-gray-400 mb-1">服务器地址 (Host)</label>
                <input
                  v-model="form.proxyHost"
                  type="text"
                  placeholder="127.0.0.1 或 proxy.com"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
              <div>
                <label class="block text-gray-400 mb-1">服务器端口 (Port)</label>
                <input
                  v-model.number="form.proxyPort"
                  type="number"
                  placeholder="1080"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>
          </div>

          <!-- Modal Action Buttons -->
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
              :disabled="saving"
              class="px-5 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-semibold text-xs rounded-xl transition-all shadow-lg shadow-brand-500/25"
            >
              <span>{{ saving ? '保存中...' : '保存出站配置' }}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus } from 'lucide-vue-next'
import api from '../api'

const outbounds = ref<any[]>([])
const showModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)

const form = ref<any>({
  tag: '',
  protocol: 'freedom',
  freedomDomainStrategy: 'UseIPv4',
  blackholeResponse: 'none',
  wgSecretKey: '',
  wgPeerPublicKey: '',
  wgEndpoint: '162.159.192.1:2408',
  wgAddress: '172.16.0.2/32',
  proxyHost: '',
  proxyPort: 1080,
})

const fetchOutbounds = async () => {
  try {
    outbounds.value = await api.get('/outbounds')
  } catch (err) {
    console.error(err)
  }
}

const openCreateModal = () => {
  isEditing.value = false
  form.value = {
    tag: '',
    protocol: 'wireguard',
    freedomDomainStrategy: 'UseIPv4',
    blackholeResponse: 'none',
    wgSecretKey: '',
    wgPeerPublicKey: '',
    wgEndpoint: '162.159.192.1:2408',
    wgAddress: '172.16.0.2/32',
    proxyHost: '',
    proxyPort: 1080,
  }
  showModal.value = true
}

const editOutbound = (ob: any) => {
  isEditing.value = true
  form.value.tag = ob.tag
  form.value.protocol = ob.protocol

  let s: any = {}
  try {
    s = JSON.parse(ob.settingsJson || '{}')
  } catch (e) {}

  form.value.freedomDomainStrategy = s.domainStrategy || 'UseIPv4'
  form.value.blackholeResponse = s.response?.type || 'none'

  if (ob.protocol === 'wireguard') {
    form.value.wgSecretKey = s.secretKey || ''
    form.value.wgAddress = (s.address || ['172.16.0.2/32'])[0]
    if (s.peers?.length > 0) {
      form.value.wgEndpoint = s.peers[0].endpoint || ''
      form.value.wgPeerPublicKey = s.peers[0].publicKey || ''
    }
  }

  showModal.value = true
}

const buildSettingsJSON = () => {
  if (form.value.protocol === 'freedom') {
    return JSON.stringify({
      domainStrategy: form.value.freedomDomainStrategy || 'UseIPv4',
    })
  } else if (form.value.protocol === 'blackhole') {
    return JSON.stringify({
      response: {
        type: form.value.blackholeResponse || 'none',
      },
    })
  } else if (form.value.protocol === 'wireguard') {
    return JSON.stringify({
      secretKey: form.value.wgSecretKey,
      address: [form.value.wgAddress || '172.16.0.2/32'],
      noKernelTun: true,
      mtu: 1280,
      peers: [
        {
          endpoint: form.value.wgEndpoint,
          publicKey: form.value.wgPeerPublicKey,
        },
      ],
    })
  } else if (form.value.protocol === 'socks' || form.value.protocol === 'http') {
    return JSON.stringify({
      servers: [
        {
          address: form.value.proxyHost,
          port: form.value.proxyPort,
        },
      ],
    })
  }
  return '{}'
}

const saveOutbound = async () => {
  saving.value = true
  try {
    const payload = {
      tag: form.value.tag,
      protocol: form.value.protocol,
      settingsJson: buildSettingsJSON(),
    }
    await api.post('/outbounds', payload)
    showModal.value = false
    await fetchOutbounds()
  } catch (err: any) {
    alert('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteOutbound = async (tag: string) => {
  if (!confirm(`确定删除出站节点 ${tag} 吗？`)) return
  try {
    await api.delete(`/outbounds/${tag}`)
    await fetchOutbounds()
  } catch (err: any) {
    alert('删除失败: ' + err)
  }
}

const protocolBadgeColor = (proto: string) => {
  switch (proto?.toLowerCase()) {
    case 'freedom':
      return 'bg-emerald-500/15 text-emerald-300 border border-emerald-500/20'
    case 'blackhole':
      return 'bg-rose-500/15 text-rose-300 border border-rose-500/20'
    case 'wireguard':
      return 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/20'
    default:
      return 'bg-indigo-500/15 text-indigo-300 border border-indigo-500/20'
  }
}

const getUsageDesc = (ob: any) => {
  switch (ob.protocol?.toLowerCase()) {
    case 'freedom':
      return '直接向目标发起网络连接'
    case 'blackhole':
      return '静默丢弃或拦截阻断连接'
    case 'wireguard':
      return 'Cloudflare WARP 清洁 IP 出站'
    default:
      return '转发至上游代理'
  }
}

const formatSettingsSummary = (ob: any) => {
  if (!ob.settingsJson || ob.settingsJson === '{}') return '默认系统策略'
  try {
    const s = JSON.parse(ob.settingsJson)
    if (s.domainStrategy) return `解析策略: ${s.domainStrategy}`
    if (s.peers?.length > 0) return `WARP Endpoint: ${s.peers[0].endpoint}`
    if (s.response?.type) return `拦截类型: ${s.response.type}`
    if (s.servers?.length > 0) return `代理服务器: ${s.servers[0].address}:${s.servers[0].port}`
    return JSON.stringify(s)
  } catch (e) {
    return ob.settingsJson
  }
}

onMounted(() => {
  fetchOutbounds()
})
</script>
