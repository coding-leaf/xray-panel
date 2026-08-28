<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-white tracking-tight">DNS 模块显式配置 (DNS)</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-amber-500/15 text-amber-400 border border-amber-500/30 flex items-center gap-1">
            <span>🟠 修改配置自动重启核心 (初始化上游 DNS 池)</span>
          </span>
        </div>
        <p class="text-xs text-gray-400 mt-0.5">配置 Xray 内置 DNS 解析器、DoH 安全域名查询与静态 Hosts，保存后自动落盘并重启核心生效</p>
      </div>

      <button
        @click="saveDNS"
        :disabled="saving"
        class="px-5 py-2 bg-emerald-600 hover:bg-emerald-500 text-white text-xs font-bold rounded-xl transition-all shadow-lg shadow-emerald-600/25 flex items-center gap-1.5"
      >
        <Check class="w-4 h-4" />
        <span>{{ saving ? '保存中...' : '保存并应用' }}</span>
      </button>
    </div>

    <!-- Strategy & Basic Settings -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-2">
        <Globe class="w-4 h-4 text-brand-400" />
        <span>① DNS 解析全局策略</span>
      </h2>

      <div class="grid grid-cols-1 sm:grid-cols-3 gap-4 text-xs">
        <div>
          <label class="block text-gray-300 mb-1 font-medium">查询策略 (queryStrategy)</label>
          <select
            v-model="dnsConfig.queryStrategy"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          >
            <option value="UseIP">UseIP (双栈根据网络自动选择)</option>
            <option value="UseIPv4">UseIPv4 (仅查询 IPv4 A 记录 - 推荐)</option>
            <option value="UseIPv6">UseIPv6 (优先/仅查询 IPv6 AAAA 记录)</option>
          </select>
        </div>

        <div>
          <label class="block text-gray-300 mb-1 font-medium">客户端 ECS IP (clientIp 可选)</label>
          <input
            v-model="dnsConfig.clientIp"
            type="text"
            placeholder="例如 1.2.3.4 (用于加速海外 CDN 分配)"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
        </div>

        <div>
          <label class="block text-gray-300 mb-1 font-medium">DNS 缓存机制</label>
          <div class="flex items-center gap-4 mt-2 text-gray-300">
            <label class="flex items-center gap-1.5 cursor-pointer">
              <input type="checkbox" v-model="dnsConfig.disableCache" class="rounded bg-gray-900 border-gray-700 text-brand-600" />
              <span>禁用内存缓存 (disableCache)</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Preset DNS Providers -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-2">
          <Server class="w-4 h-4 text-cyan-400" />
          <span>② DNS 上游服务器列表 (Servers)</span>
        </h2>
        <span class="text-[11px] text-gray-500 font-mono">从上往下优先级依次降低</span>
      </div>

      <!-- Quick Add Buttons -->
      <div class="flex flex-wrap gap-2 pt-1 border-t border-gray-800">
        <span class="text-xs text-gray-400 self-center mr-1">快捷添加:</span>
        <button
          @click="addServer('https://1.1.1.1/dns-query')"
          class="px-2.5 py-1 rounded-lg bg-gray-900/80 hover:bg-brand-500/20 text-gray-300 hover:text-brand-300 border border-gray-700 text-[11px] font-mono transition-colors"
        >
          + Cloudflare DoH (1.1.1.1)
        </button>
        <button
          @click="addServer('https://dns.google/dns-query')"
          class="px-2.5 py-1 rounded-lg bg-gray-900/80 hover:bg-brand-500/20 text-gray-300 hover:text-brand-300 border border-gray-700 text-[11px] font-mono transition-colors"
        >
          + Google DoH (8.8.8.8)
        </button>
        <button
          @click="addServer('223.5.5.5')"
          class="px-2.5 py-1 rounded-lg bg-gray-900/80 hover:bg-brand-500/20 text-gray-300 hover:text-brand-300 border border-gray-700 text-[11px] font-mono transition-colors"
        >
          + 阿里 DNS (223.5.5.5)
        </button>
        <button
          @click="addServer('119.29.29.29')"
          class="px-2.5 py-1 rounded-lg bg-gray-900/80 hover:bg-brand-500/20 text-gray-300 hover:text-brand-300 border border-gray-700 text-[11px] font-mono transition-colors"
        >
          + 腾讯 DNSPod (119.29.29.29)
        </button>
        <button
          @click="addServer('localhost')"
          class="px-2.5 py-1 rounded-lg bg-gray-900/80 hover:bg-brand-500/20 text-gray-300 hover:text-brand-300 border border-gray-700 text-[11px] font-mono transition-colors"
        >
          + 本地系统 DNS (localhost)
        </button>
      </div>

      <!-- Current Servers List -->
      <div class="space-y-2 pt-2 text-xs">
        <div
          v-for="(srv, idx) in dnsConfig.servers"
          :key="idx"
          class="p-3 bg-gray-900/80 rounded-xl border border-gray-800 flex items-center justify-between gap-3"
        >
          <div class="flex items-center gap-3 flex-1">
            <span class="w-5 h-5 rounded bg-gray-800 flex items-center justify-center font-mono text-[10px] text-gray-400 font-bold">
              {{ Number(idx) + 1 }}
            </span>
            <input
              v-model="dnsConfig.servers[idx]"
              type="text"
              class="w-full bg-transparent font-mono text-white text-xs focus:outline-none"
            />
          </div>

          <div class="flex items-center gap-1.5">
            <button
              @click="moveServer(Number(idx), -1)"
              :disabled="Number(idx) === 0"
              class="p-1 rounded bg-gray-800 text-gray-400 hover:text-white disabled:opacity-30"
              title="上移"
            >
              <ArrowUp class="w-3.5 h-3.5" />
            </button>
            <button
              @click="moveServer(Number(idx), 1)"
              :disabled="Number(idx) === dnsConfig.servers.length - 1"
              class="p-1 rounded bg-gray-800 text-gray-400 hover:text-white disabled:opacity-30"
              title="下移"
            >
              <ArrowDown class="w-3.5 h-3.5" />
            </button>
            <button
              @click="deleteServer(Number(idx))"
              class="p-1 rounded bg-gray-800 text-rose-400 hover:bg-rose-500/20"
              title="删除"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>

        <button
          @click="addCustomServer"
          class="w-full py-2 rounded-xl border border-dashed border-gray-700 text-gray-400 hover:text-white hover:border-gray-500 transition-colors flex items-center justify-center gap-1 text-xs"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>添加自定义 DNS 服务器</span>
        </button>
      </div>
    </div>

    <!-- Static Hosts Mapping -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-xs font-bold text-white uppercase tracking-wider flex items-center gap-2">
        <Layers class="w-4 h-4 text-purple-400" />
        <span>③ 静态 Hosts 域名映射 (可选)</span>
      </h2>
      <p class="text-[11px] text-gray-500 leading-relaxed">
        可将特定域名强制重定向到指定 IP 或别名（格式如 <code>domain.com: 127.0.0.1</code> 或 <code>geosite:category-ads-all: 127.0.0.1</code>）
      </p>

      <textarea
        v-model="hostsText"
        rows="4"
        placeholder="example.com: 127.0.0.1&#10;domain:google.com: 1.1.1.1"
        class="w-full bg-gray-900 border border-gray-700 rounded-xl p-3 text-white font-mono text-xs focus:outline-none focus:border-brand-500"
      ></textarea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Globe, Server, Layers, Check, Plus, ArrowUp, ArrowDown, Trash2 } from 'lucide-vue-next'
import api from '../api'

const dnsConfig = ref<any>({
  queryStrategy: 'UseIPv4',
  disableCache: false,
  servers: ['https://1.1.1.1/dns-query', '8.8.8.8', 'localhost'],
  hosts: {},
})

const hostsText = ref('')
const saving = ref(false)

const fetchDNS = async () => {
  try {
    const res: any = await api.get('/dns')
    if (res) {
      dnsConfig.value = res
      if (!dnsConfig.value.servers?.length) {
        dnsConfig.value.servers = ['https://1.1.1.1/dns-query', '8.8.8.8', 'localhost']
      }
      // 转换 hosts map 为换行文本
      if (res.hosts) {
        const lines: string[] = []
        for (const [k, v] of Object.entries(res.hosts)) {
          lines.push(`${k}: ${v}`)
        }
        hostsText.value = lines.join('\n')
      }
    }
  } catch (err) {
    console.error(err)
  }
}

const addServer = (address: string) => {
  if (!dnsConfig.value.servers.includes(address)) {
    dnsConfig.value.servers.push(address)
  }
}

const addCustomServer = () => {
  dnsConfig.value.servers.push('1.1.1.1')
}

const deleteServer = (idx: number) => {
  dnsConfig.value.servers.splice(idx, 1)
}

const moveServer = (idx: number, step: number) => {
  const target = idx + step
  if (target < 0 || target >= dnsConfig.value.servers.length) return
  const temp = dnsConfig.value.servers[idx]
  dnsConfig.value.servers[idx] = dnsConfig.value.servers[target]
  dnsConfig.value.servers[target] = temp
}

const saveDNS = async () => {
  saving.value = true
  try {
    // 解析 hostsText 为 map
    const hostsMap: any = {}
    if (hostsText.value) {
      const lines = hostsText.value.split('\n')
      for (const line of lines) {
        const trimmed = line.trim()
        if (!trimmed || trimmed.startsWith('#')) continue
        const parts = trimmed.split(':')
        if (parts.length >= 2) {
          const k = parts[0].trim()
          const v = parts.slice(1).join(':').trim()
          if (k && v) hostsMap[k] = v
        }
      }
    }
    dnsConfig.value.hosts = hostsMap

    await api.post('/dns', dnsConfig.value)
    alert('DNS 模块配置已保存并平滑生效！')
    await fetchDNS()
  } catch (err: any) {
    alert('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  fetchDNS()
})
</script>
