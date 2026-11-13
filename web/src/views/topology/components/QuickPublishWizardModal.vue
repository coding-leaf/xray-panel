<template>
  <Teleport to="body">
    <div
      v-if="modelValue"
      class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 overflow-y-auto"
    >
      <div
        class="w-full max-w-lg p-5 rounded-lg border border-border bg-card shadow-2xl space-y-4 my-8 max-h-[90vh] overflow-y-auto"
      >
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <div>
            <h2 class="text-sm font-bold text-foreground flex items-center gap-2">
              <span>⚡ 1步发布落地线路向导</span>
            </h2>
            <p class="text-xs text-muted-foreground mt-0.5">
              自动创建出口节点、绑定接入网关、分配 RouteID 并下发订阅
            </p>
          </div>
          <button
            type="button"
            @click="closeModal"
            class="text-muted-foreground hover:text-foreground text-base transition-colors"
          >
            ✕
          </button>
        </div>

        <form @submit.prevent="submitWizard" class="space-y-4 text-xs">
          <!-- 录入模式切换 -->
          <div class="flex gap-2 p-1 bg-muted/60 rounded-lg border border-border">
            <button
              type="button"
              :class="
                wizardMode === 'link'
                  ? 'bg-foreground text-background font-semibold shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              "
              class="flex-1 py-1.5 text-xs rounded-md transition-all font-medium"
              @click="wizardMode = 'link'"
            >
              粘贴节点链接解析
            </button>
            <button
              type="button"
              :class="
                wizardMode === 'manual'
                  ? 'bg-foreground text-background font-semibold shadow-sm'
                  : 'text-muted-foreground hover:text-foreground'
              "
              class="flex-1 py-1.5 text-xs rounded-md transition-all font-medium"
              @click="wizardMode = 'manual'"
            >
              从现有出口选择 / 自定义
            </button>
          </div>

          <!-- 模式 1: 节点链接解析 -->
          <div
            v-if="wizardMode === 'link'"
            class="space-y-3 bg-muted/30 p-3.5 rounded-lg border border-border"
          >
            <div>
              <label class="block text-foreground mb-1 font-medium"
                >节点链接 (支持 vless://, trojan://, ss://)</label
              >
              <textarea
                v-model="wizardLink"
                @input="parseImportLink"
                rows="3"
                placeholder="vless://uuid@server:443?type=tcp&security=reality...#香港节点"
                class="w-full bg-background border border-border rounded-md p-2.5 text-foreground font-mono text-[11px] focus:outline-none focus:border-ring transition-colors"
              ></textarea>
            </div>
            <div v-if="parsedExitTag" class="text-[11px] text-cyan-400 font-mono">
              ✓ 解析成功：出口标识将设为
              <span class="font-bold text-foreground">{{ parsedExitTag }}</span> ({{
                parsedProtocol
              }})
            </div>
            <div v-if="parsedExitTag" class="pt-2 border-t border-border/40">
              <label
                class="flex items-center gap-2 cursor-pointer text-xs"
                :class="{ 'opacity-60 cursor-not-allowed': isVisionNode }"
              >
                <input
                  type="checkbox"
                  v-model="enableMux"
                  :disabled="isVisionNode"
                  class="rounded bg-background border-border text-foreground focus:ring-ring"
                />
                <span class="font-medium text-foreground">启用跨洋连接复用 (Mux + XUDP)</span>
                <span v-if="isVisionNode" class="text-[10px] text-amber-400 font-mono"
                  >(与 XTLS Vision 互斥，已自动禁用)</span
                >
                <span v-else class="text-[10px] text-muted-foreground font-mono"
                  >(加速高延迟网络并发，默认并发 8 / XUDP 16)</span
                >
              </label>
            </div>
          </div>

          <!-- 模式 2: 从现有出口选择 -->
          <div v-else class="space-y-3 bg-muted/30 p-3.5 rounded-lg border border-border">
            <div>
              <label class="block text-foreground mb-1 font-medium">选择目标出口 (Outbound)</label>
              <select
                v-model="wizardForm.exitTag"
                class="w-full bg-background border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:border-ring transition-colors"
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
              <label class="block text-foreground mb-1 font-medium">订阅线路名称</label>
              <input
                v-model="wizardForm.channelName"
                type="text"
                required
                placeholder="如 🇯🇵 日本原生直连"
                class="w-full bg-background border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:border-ring transition-colors"
              />
            </div>

            <div>
              <label class="block text-foreground mb-1 font-medium">挂载接入网关</label>
              <select
                v-model="wizardForm.gatewayTag"
                class="w-full bg-background border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:border-ring transition-colors"
              >
                <option v-for="inb in inbounds" :key="inb.tag" :value="inb.tag">
                  {{ inb.tag }} (:{{ inb.externalPort || inb.port }})
                </option>
              </select>
            </div>
          </div>

          <!-- 自动分配 RouteID 预览卡片 -->
          <div
            class="p-3 bg-indigo-950/30 border border-indigo-500/30 rounded-lg flex items-center justify-between text-xs"
          >
            <div class="space-y-0.5">
              <span class="text-indigo-300 font-semibold block">协议级单端口分流</span>
              <span class="text-[10px] text-muted-foreground"
                >系统已自动分配下一个空闲 RouteID</span
              >
            </div>
            <span
              class="px-2.5 py-1 bg-indigo-500/20 text-indigo-300 font-mono font-bold rounded-md border border-indigo-500/30"
            >
              Route #{{ nextRouteId }}
            </span>
          </div>

          <div class="pt-3 border-t border-border flex items-center justify-end gap-2.5">
            <button
              type="button"
              @click="closeModal"
              class="px-3.5 py-1.5 rounded-md text-xs font-medium text-muted-foreground hover:text-foreground transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              :disabled="wizardSubmitting"
              class="px-4 py-2 bg-foreground text-background hover:bg-foreground/90 font-medium text-xs rounded-md shadow-sm transition-colors disabled:opacity-50"
            >
              {{ wizardSubmitting ? '正在发布...' : '立即发布并生效' }}
            </button>
          </div>
        </form>
      </div>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import api from '../../../api'
import { toast } from '../../../utils/toast'

interface Props {
  modelValue: boolean
  inbounds: any[]
  outbounds: any[]
  nextRouteId: number
  initialExitTag?: string
}

const props = defineProps<Props>()
const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'success'): void
}>()

const wizardMode = ref<'link' | 'manual'>('link')
const wizardLink = ref('')
const wizardSubmitting = ref(false)
const parsedExitTag = ref('')
const parsedProtocol = ref('')
const parsedOutboundPayload = ref<any>(null)
const enableMux = ref(true)
const isVisionNode = ref(false)

const wizardForm = ref({
  channelName: '',
  gatewayTag: '',
  exitTag: 'direct',
})

const closeModal = () => {
  emit('update:modelValue', false)
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      wizardLink.value = ''
      parsedExitTag.value = ''
      parsedProtocol.value = ''
      parsedOutboundPayload.value = null
      enableMux.value = true
      isVisionNode.value = false

      if (props.initialExitTag) {
        wizardMode.value = 'manual'
        wizardForm.value = {
          channelName: `落地线路 #${props.nextRouteId}`,
          gatewayTag: props.inbounds[0]?.tag || '',
          exitTag: props.initialExitTag,
        }
      } else {
        wizardMode.value = 'link'
        wizardForm.value = {
          channelName: `分流线路 #${props.nextRouteId}`,
          gatewayTag: props.inbounds[0]?.tag || '',
          exitTag: props.outbounds[0]?.tag || 'direct',
        }
      }
    }
  },
  { immediate: true }
)

const parseImportLink = () => {
  const link = wizardLink.value.trim()
  if (!link) {
    parsedExitTag.value = ''
    parsedOutboundPayload.value = null
    isVisionNode.value = false
    enableMux.value = true
    return
  }

  try {
    if (link.startsWith('vless://')) {
      const u = new URL(link)
      const cleanHost = u.hostname.replace(/[^a-zA-Z0-9]/g, '') || 'node'
      const tag = `out-vless-${cleanHost}-${Math.random().toString(36).substring(2, 6)}`
      parsedExitTag.value = tag
      parsedProtocol.value = 'vless'
      if (u.hash) {
        wizardForm.value.channelName = decodeURIComponent(u.hash.replace('#', ''))
      }

      const params = u.searchParams
      const network = params.get('type') || 'tcp'
      const security = params.get('security') || 'none'
      const flow = params.get('flow') || ''
      if (flow.includes('vision')) {
        isVisionNode.value = true
        enableMux.value = false
      } else {
        isVisionNode.value = false
        enableMux.value = true
      }
      const sni = params.get('sni') || ''
      const pbk = params.get('pbk') || ''
      const sid = params.get('sid') || ''
      const fp = params.get('fp') || 'chrome'
      const spx = params.get('spx') || ''
      const path = params.get('path') || ''
      const mode = params.get('mode') || 'auto'
      const serviceName = params.get('serviceName') || ''

      const userObj: any = {
        id: u.username,
        encryption: 'none',
      }
      if (flow) {
        userObj.flow = flow
      }

      const stream: any = {
        network,
        security,
      }

      if (security === 'reality') {
        stream.realitySettings = {
          serverName: sni || u.hostname,
          fingerprint: fp,
          publicKey: pbk,
          shortId: sid,
          spiderX: spx,
        }
      } else if (security === 'tls') {
        stream.tlsSettings = {
          serverName: sni || u.hostname,
          fingerprint: fp,
        }
      }

      if (network === 'xhttp') {
        stream.xhttpSettings = { path: path || '/', mode }
      } else if (network === 'ws') {
        stream.wsSettings = { path: path || '/' }
      } else if (network === 'grpc') {
        stream.grpcSettings = { serviceName }
      }

      parsedOutboundPayload.value = {
        tag,
        protocol: 'vless',
        settingsJson: JSON.stringify({
          vnext: [
            {
              address: u.hostname,
              port: parseInt(u.port) || 443,
              users: [userObj],
            },
          ],
        }),
        streamSettings: JSON.stringify(stream),
      }
    } else if (link.startsWith('trojan://')) {
      isVisionNode.value = false
      enableMux.value = true
      const u = new URL(link)
      const cleanHost = u.hostname.replace(/[^a-zA-Z0-9]/g, '') || 'node'
      const tag = `out-trojan-${cleanHost}-${Math.random().toString(36).substring(2, 6)}`
      parsedExitTag.value = tag
      parsedProtocol.value = 'trojan'
      if (u.hash) {
        wizardForm.value.channelName = decodeURIComponent(u.hash.replace('#', ''))
      }

      const params = u.searchParams
      const network = params.get('type') || 'tcp'
      const security = params.get('security') || 'tls'
      const sni = params.get('sni') || ''

      const stream: any = {
        network,
        security,
      }
      if (security === 'tls') {
        stream.tlsSettings = {
          serverName: sni || u.hostname,
        }
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
        streamSettings: JSON.stringify(stream),
      }
    } else if (link.startsWith('ss://')) {
      isVisionNode.value = false
      enableMux.value = true
      const clean = link.replace('ss://', '')
      const parts = clean.split('#')
      const mainPart = parts[0]
      if (parts[1]) {
        wizardForm.value.channelName = decodeURIComponent(parts[1])
      }

      let method = 'aes-128-gcm'
      let password = ''
      let server = ''
      let port = 443

      if (mainPart.includes('@')) {
        const [authB64, hostPort] = mainPart.split('@')
        try {
          const auth = atob(authB64)
          const authParts = auth.split(':')
          method = authParts[0]
          password = authParts[1]
        } catch {
          /* ignore */
        }
        const hp = hostPort.split(':')
        server = hp[0]
        port = parseInt(hp[1]) || 443
      } else {
        try {
          const decoded = atob(mainPart)
          const [auth, hp] = decoded.split('@')
          const authParts = auth.split(':')
          method = authParts[0]
          password = authParts[1]
          const hpParts = hp.split(':')
          server = hpParts[0]
          port = parseInt(hpParts[1]) || 443
        } catch {
          /* ignore */
        }
      }

      const tag = `out-ss-${server.replace(/[^a-zA-Z0-9]/g, '') || 'node'}-${Math.random().toString(36).substring(2, 6)}`
      parsedExitTag.value = tag
      parsedProtocol.value = 'shadowsocks'
      parsedOutboundPayload.value = {
        tag,
        protocol: 'shadowsocks',
        settingsJson: JSON.stringify({
          servers: [
            {
              address: server,
              port: port,
              method: method,
              password: password,
            },
          ],
        }),
      }
    }
  } catch (e) {
    console.error('Parse link failed', e)
    parsedExitTag.value = ''
    parsedProtocol.value = ''
    parsedOutboundPayload.value = null
    isVisionNode.value = false
    enableMux.value = true
  }
}

const submitWizard = async () => {
  wizardSubmitting.value = true
  try {
    let targetOutboundTag = wizardForm.value.exitTag

    if (wizardMode.value === 'link') {
      if (!parsedOutboundPayload.value) {
        toast.error('未检测到有效的节点链接，请检查格式')
        return
      }
      targetOutboundTag = parsedOutboundPayload.value.tag
      const existingOb = props.outbounds.find((o) => o.tag === targetOutboundTag)
      if (!existingOb) {
        const payload = { ...parsedOutboundPayload.value }
        if (enableMux.value && !isVisionNode.value) {
          payload.mux = JSON.stringify({
            enabled: true,
            concurrency: 8,
            xudpConcurrency: 16,
            xudpProxyUDP443: 'reject',
          })
        } else {
          payload.mux = '{}'
        }
        await api.post('/outbounds', payload)
      }
    }

    const targetGateway = props.inbounds.find((i) => i.tag === wizardForm.value.gatewayTag)
    if (!targetGateway) {
      toast.error('未找到目标接入网关')
      return
    }

    const currentSubRoutes = targetGateway.subRoutes ? [...targetGateway.subRoutes] : []
    currentSubRoutes.push({
      id: Math.random().toString(36).substring(2, 9),
      name: wizardForm.value.channelName,
      routeId: props.nextRouteId,
      outboundTag: targetOutboundTag,
      enabled: true,
    })

    const payload = {
      ...targetGateway,
      subRoutesJson: JSON.stringify(currentSubRoutes),
    }

    await api.put(`/inbounds/${targetGateway.id}`, payload)
    toast.success('线路已发布，已注入 Xray 路由表并就绪订阅')
    emit('update:modelValue', false)
    emit('success')
  } catch (err: any) {
    toast.error(err.response?.data?.error || '发布线路失败')
  } finally {
    wizardSubmitting.value = false
  }
}
</script>
