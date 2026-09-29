<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">DNS 模块显式配置 (DNS)</h1>
          <Badge variant="outline" class="text-[10px] font-mono">
            {{ dnsConfig.servers?.length || 0 }} 个上游池
          </Badge>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20">
            修改自动重启核心生效
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          配置 Xray 内置 DNS 解析器、DoH 安全域名查询与静态 Hosts，保存后自动落盘并平滑应用
        </p>
      </div>

      <Button
        variant="default"
        size="sm"
        :loading="saving"
        @click="saveDNS"
      >
        <Check class="w-3.5 h-3.5 mr-1" />
        <span>{{ saving ? '保存中...' : '保存并应用' }}</span>
      </Button>
    </div>

    <!-- Strategy & Basic Settings -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
        <Globe class="w-3.5 h-3.5 text-cyan-400" />
        <span>① DNS 解析全局策略</span>
      </h2>

      <div class="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground mb-1 font-medium">查询策略 (queryStrategy)</label>
          <select
            v-model="dnsConfig.queryStrategy"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="UseIP">UseIP (双栈根据网络自动选择)</option>
            <option value="UseIPv4">UseIPv4 (仅查询 IPv4 A 记录 - 推荐)</option>
            <option value="UseIPv6">UseIPv6 (优先/仅查询 IPv6 AAAA 记录)</option>
          </select>
        </div>

        <div>
          <label class="block text-muted-foreground mb-1 font-medium">客户端 ECS IP (clientIp 可选)</label>
          <input
            v-model="dnsConfig.clientIp"
            type="text"
            placeholder="例如 1.2.3.4 (用于加速海外 CDN 分配)"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div>
          <label class="block text-muted-foreground mb-1 font-medium">DNS 缓存机制</label>
          <div class="flex items-center gap-3 mt-2 text-foreground">
            <label class="flex items-center gap-2 cursor-pointer">
              <input type="checkbox" v-model="dnsConfig.disableCache" class="rounded bg-neutral-950 border-border text-brand-500 focus:ring-0" />
              <span class="text-xs">禁用内存缓存 (disableCache)</span>
            </label>
          </div>
        </div>
      </div>
    </div>

    <!-- Quick Preset DNS Providers -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
          <Server class="w-3.5 h-3.5 text-cyan-400" />
          <span>② DNS 上游服务器列表 (Servers)</span>
        </h2>
        <span class="text-[11px] text-muted-foreground font-mono">从上往下优先级依次降低</span>
      </div>

      <!-- Quick Add Buttons -->
      <div class="flex flex-wrap gap-2 pt-1 border-t border-border/60">
        <span class="text-xs text-muted-foreground self-center mr-1">快捷添加:</span>
        <button
          @click="addServer('https://1.1.1.1/dns-query')"
          class="px-2.5 py-1 rounded-md bg-neutral-950 hover:bg-neutral-900 text-muted-foreground hover:text-foreground border border-border text-[11px] font-mono transition-colors"
        >
          + Cloudflare DoH (1.1.1.1)
        </button>
        <button
          @click="addServer('https://dns.google/dns-query')"
          class="px-2.5 py-1 rounded-md bg-neutral-950 hover:bg-neutral-900 text-muted-foreground hover:text-foreground border border-border text-[11px] font-mono transition-colors"
        >
          + Google DoH (8.8.8.8)
        </button>
        <button
          @click="addServer('223.5.5.5')"
          class="px-2.5 py-1 rounded-md bg-neutral-950 hover:bg-neutral-900 text-muted-foreground hover:text-foreground border border-border text-[11px] font-mono transition-colors"
        >
          + 阿里 DNS (223.5.5.5)
        </button>
        <button
          @click="addServer('119.29.29.29')"
          class="px-2.5 py-1 rounded-md bg-neutral-950 hover:bg-neutral-900 text-muted-foreground hover:text-foreground border border-border text-[11px] font-mono transition-colors"
        >
          + 腾讯 DNSPod (119.29.29.29)
        </button>
        <button
          @click="addServer('localhost')"
          class="px-2.5 py-1 rounded-md bg-neutral-950 hover:bg-neutral-900 text-muted-foreground hover:text-foreground border border-border text-[11px] font-mono transition-colors"
        >
          + 本地系统 DNS (localhost)
        </button>
      </div>

      <!-- Current Servers List -->
      <div class="space-y-2 pt-1 text-xs">
        <div
          v-for="(_, idx) in dnsConfig.servers"
          :key="idx"
          class="p-2.5 bg-neutral-950 rounded-md border border-border flex items-center justify-between gap-3"
        >
          <div class="flex items-center gap-2.5 flex-1">
            <span class="w-5 h-5 rounded bg-neutral-900 border border-border flex items-center justify-center font-mono text-[10px] text-muted-foreground font-bold shrink-0">
              {{ Number(idx) + 1 }}
            </span>
            <input
              v-model="dnsConfig.servers[idx]"
              type="text"
              class="w-full bg-transparent font-mono text-foreground text-xs focus:outline-none"
            />
          </div>

          <div class="flex items-center gap-1 shrink-0">
            <Button
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0"
              :disabled="Number(idx) === 0"
              @click="moveServer(Number(idx), -1)"
              title="上移"
            >
              <ArrowUp class="w-3 h-3" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0"
              :disabled="Number(idx) === dnsConfig.servers.length - 1"
              @click="moveServer(Number(idx), 1)"
              title="下移"
            >
              <ArrowDown class="w-3 h-3" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 w-6 p-0 text-rose-400 hover:text-rose-300 hover:bg-rose-950/40"
              @click="deleteServer(Number(idx))"
              title="删除"
            >
              <Trash2 class="w-3 h-3" />
            </Button>
          </div>
        </div>

        <button
          @click="addCustomServer"
          class="w-full py-2 rounded-md border border-dashed border-border hover:border-neutral-500 text-muted-foreground hover:text-foreground transition-colors flex items-center justify-center gap-1 text-xs font-mono"
        >
          <Plus class="w-3.5 h-3.5" />
          <span>添加自定义 DNS 服务器</span>
        </button>
      </div>
    </div>

    <!-- Static Hosts Mapping -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-2.5">
      <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
        <Layers class="w-3.5 h-3.5 text-indigo-400" />
        <span>③ 静态 Hosts 域名映射 (可选)</span>
      </h2>
      <p class="text-[11px] text-muted-foreground leading-relaxed">
        可将特定域名强制重定向到指定 IP 或别名（格式如 <code>domain.com: 127.0.0.1</code> 或 <code>geosite:category-ads-all: 127.0.0.1</code>）
      </p>

      <textarea
        v-model="hostsText"
        rows="4"
        placeholder="example.com: 127.0.0.1&#10;domain:google.com: 1.1.1.1"
        class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-foreground font-mono text-xs focus:outline-none focus:ring-1 focus:ring-ring leading-relaxed"
      ></textarea>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Globe, Server, Layers, Check, Plus, ArrowUp, ArrowDown, Trash2 } from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { toast } from '../utils/toast'
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
        // 优先使用冒号后接空格分割，兼容多冒号规则键（如 geosite:xxx 或 domain:xxx）与 IPv6 值（如 ::1）
        const colonSpaceIdx = trimmed.search(/:\s+/)
        if (colonSpaceIdx > 0) {
          const k = trimmed.slice(0, colonSpaceIdx).trim()
          const v = trimmed.slice(colonSpaceIdx + 1).trim()
          if (k && v) hostsMap[k] = v
          continue
        }
        // 若无空格，检查是否以已知 Xray 规则前缀开头
        const prefixMatch = trimmed.match(/^(?:geosite|domain|full|keyword|regexp):[^:]+:/i)
        if (prefixMatch) {
          const splitIdx = prefixMatch[0].length - 1
          const k = trimmed.slice(0, splitIdx).trim()
          const v = trimmed.slice(splitIdx + 1).trim()
          if (k && v) hostsMap[k] = v
          continue
        }
        // 兜底：若包含 IPv4 目标，按最后冒号分割；否则按首个冒号分割
        const firstColon = trimmed.indexOf(':')
        const lastColon = trimmed.lastIndexOf(':')
        if (lastColon > 0) {
          const valCandidate = trimmed.slice(lastColon + 1).trim()
          if (/^\d{1,3}(\.\d{1,3}){3}$/.test(valCandidate)) {
            const k = trimmed.slice(0, lastColon).trim()
            if (k && valCandidate) hostsMap[k] = valCandidate
            continue
          }
        }
        if (firstColon > 0) {
          const k = trimmed.slice(0, firstColon).trim()
          const v = trimmed.slice(firstColon + 1).trim()
          if (k && v) hostsMap[k] = v
        }
      }
    }
    dnsConfig.value.hosts = hostsMap

    await api.post('/dns', dnsConfig.value)
    toast.success('DNS 模块配置已保存并平滑生效！')
    await fetchDNS()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  fetchDNS()
})
</script>
