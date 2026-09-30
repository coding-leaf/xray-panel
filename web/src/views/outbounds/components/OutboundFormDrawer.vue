<template>
  <Drawer
    v-model="isOpen"
    :title="isEditing ? `编辑出站节点: ${form.tag}` : '添加新出站节点'"
    description="配置将同步回写磁盘 config.json 并平滑重启核心应用"
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <form id="outbound-form" @submit.prevent="handleSubmit" class="space-y-4 text-xs">
      <SectionCard title="基本标识与协议">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="出站标识 (Tag)" required hint="节点唯一英文标识，如 warp-out 或 proxy-jp">
            <input
              v-model="form.tag"
              type="text"
              required
              :disabled="isEditing"
              placeholder="例如: warp-out 或 proxy-jp"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
            />
          </FormField>

          <FormField label="出站协议 (Protocol)" required>
            <select
              v-model="form.protocol"
              @change="onOutboundProtocolChange"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="freedom">freedom (直连访问)</option>
              <option value="blackhole">blackhole (黑洞丢弃/拦截)</option>
              <option value="wireguard">wireguard (Cloudflare WARP 出站)</option>
              <option value="dns">dns (内置 DNS 路由分流)</option>
              <option value="vless">vless (上游链式代理)</option>
              <option value="vmess">vmess (上游链式代理)</option>
              <option value="trojan">trojan (上游链式代理)</option>
              <option value="shadowsocks">shadowsocks (上游链式代理)</option>
              <option value="socks">socks 代理出站</option>
              <option value="http">http 代理出站</option>
            </select>
          </FormField>
        </div>
      </SectionCard>

      <!-- Freedom 专属配置 -->
      <SectionCard v-if="form.protocol === 'freedom'" title="Freedom 直连策略">
        <FormField label="域名解析策略 (domainStrategy)" hint="控制客户端域名在本地的解析行为">
          <select
            v-model="form.freedomDomainStrategy"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
          >
            <option value="UseIP">UseIP</option>
            <option value="UseIPv4">UseIPv4 (强制IPv4 - 推荐)</option>
            <option value="UseIPv6">UseIPv6 (强制IPv6)</option>
            <option value="AsIs">AsIs (保持原样)</option>
          </select>
        </FormField>
      </SectionCard>

      <!-- Blackhole 专属配置 -->
      <SectionCard v-else-if="form.protocol === 'blackhole'" title="Blackhole 拦截响应">
        <FormField label="响应类型 (Response Type)" hint="none 为静默丢弃连接，http 为响应 HTTP 403 阻断">
          <select
            v-model="form.blackholeResponse"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
          >
            <option value="none">none (直接静默丢弃)</option>
            <option value="http">http (返回 HTTP 403 阻断页面)</option>
          </select>
        </FormField>
      </SectionCard>

      <!-- WireGuard / WARP 专属配置 -->
      <SectionCard v-else-if="form.protocol === 'wireguard'" title="WireGuard (Cloudflare WARP) 详情">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="Secret Key (私钥)" required>
            <input
              v-model="form.wgSecretKey"
              type="text"
              placeholder="填写 WireGuard Private Key (Base64)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="Peer Public Key (对端公钥)" required>
            <input
              v-model="form.wgPeerPublicKey"
              type="text"
              placeholder="填写 Cloudflare WARP 对端公钥"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="Endpoint (对端服务器)">
            <input
              v-model="form.wgEndpoint"
              type="text"
              placeholder="162.159.192.1:2408"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="本地地址 (Address)">
            <input
              v-model="form.wgAddress"
              type="text"
              placeholder="172.16.0.2/32"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>
      </SectionCard>

      <!-- DNS 专属配置 -->
      <SectionCard v-else-if="form.protocol === 'dns'" title="DNS 路由出站">
        <div class="p-3 rounded bg-neutral-950 border border-border text-muted-foreground text-xs leading-relaxed">
          由 Xray 内置 DNS 模块发起查询出站，无需额外参数。通常与路由规则 (Routing) 配合，将 DNS 流量定向路由至此 Tag (例如 <code class="text-cyan-400 font-mono">dns-out</code>)。
        </div>
      </SectionCard>

      <!-- Proxy outbounds (VLESS / VMess / Trojan / Shadowsocks / Socks / HTTP) -->
      <SectionCard v-else title="上游代理服务器连接信息">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="服务器地址 (Host/Address)" required>
            <input
              v-model="form.proxyHost"
              type="text"
              required
              placeholder="127.0.0.1 或 proxy.example.com"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="服务器端口 (Port)" required>
            <input
              v-model.number="form.proxyPort"
              type="number"
              required
              placeholder="443"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <div v-if="['vless', 'vmess', 'trojan'].includes(form.protocol)">
          <FormField label="UUID / 密码 (Password)" required>
            <input
              v-model="form.proxyPassword"
              type="text"
              required
              placeholder="UUID 或密码"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- VMess 专属安全加密 -->
        <div v-if="form.protocol === 'vmess'" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="VMess 加密 (Security)">
            <select
              v-model="form.vmessSecurity"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="auto">auto (自动匹配)</option>
              <option value="aes-128-gcm">aes-128-gcm</option>
              <option value="chacha20-poly1305">chacha20-poly1305</option>
              <option value="none">none</option>
            </select>
          </FormField>
          <FormField label="AlterID (额外ID)">
            <input
              v-model.number="form.vmessAlterId"
              type="number"
              placeholder="0"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- Shadowsocks 专属密码与加密方式 -->
        <div v-if="form.protocol === 'shadowsocks'" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="SS 密码 (Password)" required>
            <input
              v-model="form.proxyPassword"
              type="text"
              required
              placeholder="Shadowsocks 密码"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="加密算法 (Method)">
            <select
              v-model="form.ssMethod"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
              <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm</option>
              <option value="aes-256-gcm">aes-256-gcm</option>
              <option value="aes-128-gcm">aes-128-gcm</option>
              <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
              <option value="none">none</option>
            </select>
          </FormField>
        </div>

        <!-- Socks / HTTP 专属可选用户名密码 -->
        <div v-if="['socks', 'http'].includes(form.protocol)" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="认证用户名 (选填)">
            <input
              v-model="form.proxyUsername"
              type="text"
              placeholder="用户名 (如无需认证可留空)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="认证密码 (选填)">
            <input
              v-model="form.proxyPassword"
              type="password"
              placeholder="密码 (如无需认证可留空)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- 传输层与安全层联动 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol)" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border">
          <FormField label="传输协议 (Network)">
            <select
              v-model="form.streamNetwork"
              @change="onOutboundNetworkChange"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="tcp">TCP</option>
              <option value="xhttp">XHTTP</option>
              <option value="grpc">gRPC</option>
              <option value="ws">WebSocket</option>
              <option value="httpupgrade">HTTPUpgrade</option>
            </select>
          </FormField>
          <FormField
            label="安全协议 (Security)"
            :hint="['vmess', 'shadowsocks'].includes(form.protocol) ? '面板策略当前仅开放 VLESS / Trojan 协议搭配 REALITY 伪装出站' : undefined"
          >
            <select
              v-model="form.streamSecurity"
              @change="onOutboundSecurityChange"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option v-if="['tcp', 'xhttp', 'grpc'].includes(form.streamNetwork) && !['vmess', 'shadowsocks'].includes(form.protocol)" value="reality">REALITY</option>
              <option value="tls">TLS</option>
              <option value="none">None</option>
            </select>
          </FormField>
        </div>

        <!-- VLESS 出站流控选项 (XTLS Vision) -->
        <div v-if="form.protocol === 'vless'" class="pt-2 border-t border-border">
          <FormField
            label="流控策略 (Flow / XTLS Vision)"
            :hint="form.streamNetwork !== 'tcp' || !['reality', 'tls'].includes(form.streamSecurity) ? '* Vision 流控仅适用于 TCP + TLS / REALITY 架构。' : undefined"
          >
            <template #extra>
              <span class="text-[11px] text-brand-400 font-mono">XTLS / REALITY 极速流控</span>
            </template>
            <select
              v-model="form.vlessFlow"
              :disabled="form.streamNetwork !== 'tcp' || !['reality', 'tls'].includes(form.streamSecurity)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-40 font-mono"
            >
              <option value="xtls-rprx-vision">xtls-rprx-vision (XTLS Vision 极速流控 - 推荐)</option>
              <option value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option>
              <option value="">none (无流控 - 适用于 XHTTP / gRPC / WS 等)</option>
            </select>
          </FormField>
        </div>

        <!-- XHTTP 专用参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamNetwork === 'xhttp'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
          <FormField label="XHTTP 路径 (Path)">
            <input
              v-model="form.xhttpPath"
              type="text"
              placeholder="/mbqyfa4grswh5ntz"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="XHTTP 模式 (Mode)">
            <select
              v-model="form.xhttpMode"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="auto">auto</option>
              <option value="stream-up">stream-up</option>
              <option value="stream-one">stream-one</option>
            </select>
          </FormField>
        </div>

        <!-- WS 专用参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamNetwork === 'ws'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
          <FormField label="WebSocket 路径 (Path)">
            <input
              v-model="form.wsPath"
              type="text"
              placeholder="/ws"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="WebSocket Host 头部 (选填)">
            <input
              v-model="form.wsHost"
              type="text"
              placeholder="例如: example.com"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- REALITY 专属连接参数 -->
        <div v-if="['vless', 'trojan'].includes(form.protocol) && form.streamSecurity === 'reality'" class="space-y-3 pt-2 border-t border-border/60 bg-neutral-950/60 p-3 rounded-md">
          <h4 class="text-xs font-bold text-cyan-400 font-mono">REALITY 握手伪装参数</h4>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="SNI 伪装域名 (ServerName)">
              <input
                v-model="form.realityServerName"
                type="text"
                placeholder="例如: apple.com"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>
            <FormField label="指纹伪装 (Fingerprint)">
              <select
                v-model="form.realityFingerprint"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="chrome">chrome (推荐)</option>
                <option value="firefox">firefox</option>
                <option value="safari">safari</option>
                <option value="ios">ios</option>
                <option value="edge">edge</option>
              </select>
            </FormField>
          </div>
          <FormField label="Public Key (对端公钥 / pbk)">
            <input
              v-model="form.realityPublicKey"
              type="text"
              placeholder="例如: FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="Short ID (短ID / sid)">
            <input
              v-model="form.realityShortId"
              type="text"
              placeholder="0123456789abcdef"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- TLS 专属连接参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamSecurity === 'tls'" class="pt-2 border-t border-border/60 bg-neutral-950/60 p-3 rounded-md">
          <FormField label="TLS SNI (ServerName)">
            <input
              v-model="form.tlsServerName"
              type="text"
              placeholder="example.com"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>
      </SectionCard>
    </form>

    <template #footer>
      <Button variant="secondary" size="sm" @click="isOpen = false">
        取消
      </Button>
      <Button
        type="submit"
        form="outbound-form"
        variant="default"
        size="sm"
        :loading="saving"
      >
        <span>{{ saving ? '保存中...' : '保存出站配置' }}</span>
      </Button>
    </template>
  </Drawer>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { toast } from '../../../utils/toast'
import Drawer from '../../../components/ui/Drawer.vue'
import Button from '../../../components/ui/Button.vue'
import FormField from '../../../components/ui/FormField.vue'
import SectionCard from '../../../components/ui/SectionCard.vue'
import type { OutboundItem, OutboundFormState, OutboundPayload } from '../types'
import {
  createDefaultOutboundForm,
  populateOutboundForm,
  sanitizeOutboundPayload,
} from '../types'

const props = defineProps<{
  modelValue: boolean
  editingOutbound: OutboundItem | null
  saving: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'save', payload: OutboundPayload): void
}>()

const form = ref<OutboundFormState>(createDefaultOutboundForm())

const isOpen = computed({
  get: () => props.modelValue,
  set: (val: boolean) => emit('update:modelValue', val),
})

const isEditing = computed(() => !!props.editingOutbound)

const onOutboundProtocolChange = () => {
  if (['freedom', 'blackhole', 'wireguard', 'socks', 'http', 'dns'].includes(form.value.protocol)) {
    form.value.streamNetwork = 'tcp'
    form.value.streamSecurity = 'none'
    form.value.vlessFlow = ''
  } else if (['vmess', 'shadowsocks'].includes(form.value.protocol)) {
    if (form.value.streamSecurity === 'reality') {
      form.value.streamSecurity = 'none'
    }
    form.value.vlessFlow = ''
  } else if (form.value.protocol === 'trojan') {
    form.value.vlessFlow = ''
  } else if (form.value.protocol === 'vless') {
    if (form.value.streamNetwork === 'tcp' && ['reality', 'tls'].includes(form.value.streamSecurity)) {
      form.value.vlessFlow = 'xtls-rprx-vision'
    } else {
      form.value.vlessFlow = ''
    }
  }
}

const onOutboundSecurityChange = () => {
  if (form.value.protocol === 'vless') {
    if (form.value.streamNetwork === 'tcp' && ['reality', 'tls'].includes(form.value.streamSecurity)) {
      if (!form.value.vlessFlow) form.value.vlessFlow = 'xtls-rprx-vision'
    } else {
      form.value.vlessFlow = ''
    }
  }
}

const onOutboundNetworkChange = () => {
  if (['vmess', 'shadowsocks'].includes(form.value.protocol)) {
    if (form.value.streamSecurity === 'reality') {
      form.value.streamSecurity = 'none'
    }
    return
  }
  if (!['tcp', 'xhttp', 'grpc'].includes(form.value.streamNetwork) && form.value.streamSecurity === 'reality') {
    form.value.streamSecurity = 'tls'
  }
}

watch(
  () => [props.modelValue, props.editingOutbound] as const,
  ([open, ob]) => {
    if (open) {
      if (ob) {
        form.value = populateOutboundForm(ob)
      } else {
        form.value = createDefaultOutboundForm()
      }
    }
  },
  { immediate: true }
)

const handleSubmit = () => {
  if (['vless', 'trojan'].includes(form.value.protocol) && form.value.streamSecurity === 'reality') {
    if (!form.value.realityServerName?.trim()) {
      toast.error('请填写 REALITY SNI 伪装域名 (ServerName)')
      return
    }
  }
  const payload = sanitizeOutboundPayload(form.value)
  emit('save', payload)
}
</script>
