<template>
  <Modal
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="modalTitle"
    :description="modalDescription"
    size="lg"
  >
    <template #icon>
      <KeyRound v-if="modalState?.token" class="w-4 h-4 text-amber-400 shrink-0" />
      <Terminal v-else class="w-4 h-4 text-cyan-400 shrink-0" />
    </template>

    <div v-if="modalState" class="space-y-4 text-xs">
      <!-- One-time Plaintext Token Section -->
      <div
        v-if="modalState.token"
        class="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3.5 space-y-2.5"
      >
        <div class="flex items-start gap-2 text-amber-300 text-[11px] leading-relaxed">
          <AlertTriangle class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
          <span>
            此明文 Token 仅在本次弹窗展示一次。关闭后服务端仅保留不可逆 SHA-256
            摘要，无法再次查看；若遗失请使用「轮换 Token」。
          </span>
        </div>

        <div class="flex flex-col sm:flex-row items-stretch sm:items-center gap-2">
          <div
            class="flex-1 min-w-0 px-3 py-2 rounded-md bg-neutral-950 border border-border font-mono text-[11px] text-foreground break-all select-all"
          >
            {{ modalState.token }}
          </div>
          <Button
            type="button"
            variant="default"
            size="sm"
            class="shrink-0"
            @click="copyText(modalState.token || '', '已复制节点明文 Token')"
          >
            <Copy class="w-3.5 h-3.5 mr-1.5" />
            <span>复制 Token</span>
          </Button>
        </div>
      </div>

      <!-- Info Banner when opened from Install Script only -->
      <div
        v-else
        class="rounded-lg border border-border bg-neutral-900/50 p-3 flex items-start gap-2 text-[11px] text-muted-foreground leading-relaxed"
      >
        <Info class="w-4 h-4 text-neutral-400 shrink-0 mt-0.5" />
        <span>
          出于安全原则，服务端不保存明文 Token（前缀：
          <code class="text-foreground font-mono">{{ modalState.tokenPrefix || '-' }}</code>
          ）。运行下方安装脚本前，请将
          <code class="text-foreground font-mono">&lt;YOUR_NODE_TOKEN&gt;</code>
          替换为创建时保存的明文 Token；如已遗失，请先点击「轮换 Token」。
        </span>
      </div>

      <!-- Master URL Configuration -->
      <div class="rounded-lg border border-border bg-card p-3.5 space-y-2.5">
        <FormField
          label="主控面板地址 (Master URL)"
          hint="远端节点访问主控的公网可达地址（推荐使用证书有效的 HTTPS 地址，避免填写 127.0.0.1 或内网地址）"
        >
          <template #default="{ id, describedBy }">
            <div class="flex items-center gap-2">
              <Input
                :id="id"
                :aria-describedby="describedBy"
                v-model="masterUrl"
                type="text"
                placeholder="https://panel.example.com"
                @keyup.enter="loadInstallScript"
              />
              <Button
                type="button"
                variant="secondary"
                size="sm"
                class="shrink-0 h-9"
                :loading="loadingScript"
                @click="loadInstallScript"
              >
                <RefreshCw class="w-3.5 h-3.5 mr-1.5" />
                <span>生成脚本</span>
              </Button>
            </div>
          </template>
        </FormField>
      </div>

      <!-- One-Line Install Command (Primary) -->
      <div class="space-y-2">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <span class="text-[11px] font-mono text-muted-foreground tracking-wide">
            一键安装命令（推荐，复制后直接粘贴到 SSH 终端回车）
            <span v-if="modalState.token" class="text-emerald-400 ml-1">
              (已自动内嵌一次性 Token)
            </span>
          </span>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            class="h-7 px-2.5 text-[11px]"
            @click="copyText(oneLineCommand, '已复制一键安装命令')"
          >
            <Copy class="w-3 h-3 mr-1.5" />
            <span>一键复制安装命令</span>
          </Button>
        </div>

        <pre
          class="p-3.5 rounded-lg bg-neutral-950 border border-border font-mono text-[11px] text-emerald-300 leading-relaxed select-all whitespace-pre-wrap break-all"
          >{{ oneLineCommand }}</pre>

        <p class="text-[11px] text-muted-foreground leading-relaxed">
          适用于 GCP / AWS / Debian / Ubuntu 等非 root 与 root 宿主机（极简容器若无
          <code class="text-foreground font-mono">sudo</code> 可去掉命令中的
          <code class="text-foreground font-mono">sudo</code>）。
        </p>
      </div>

      <!-- Full Systemd Script (Collapsible / Advanced) -->
      <div class="border-t border-border pt-3 space-y-2">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <button
            type="button"
            class="inline-flex items-center gap-1.5 text-[11px] font-mono text-muted-foreground hover:text-foreground transition-colors"
            :aria-expanded="showFullScript"
            @click="showFullScript = !showFullScript"
          >
            <ChevronDown v-if="showFullScript" class="w-3.5 h-3.5 shrink-0" />
            <ChevronRight v-else class="w-3.5 h-3.5 shrink-0" />
            <span>查看完整 systemd 脚本（高级 / 审计）</span>
          </button>

          <Button
            v-if="showFullScript"
            type="button"
            variant="secondary"
            size="sm"
            class="h-7 px-2.5 text-[11px]"
            :disabled="!renderedScript || loadingScript"
            @click="copyText(renderedScript, '已复制完整安装脚本')"
          >
            <Copy class="w-3 h-3 mr-1.5" />
            <span>复制完整脚本</span>
          </Button>
        </div>

        <div v-if="showFullScript" class="space-y-2">
          <div
            v-if="scriptError"
            class="p-3 rounded-md border border-rose-500/30 bg-rose-500/10 text-rose-300 font-mono text-[11px]"
          >
            {{ scriptError }}
          </div>

          <pre
            v-else
            class="p-3.5 rounded-lg bg-neutral-950 border border-border font-mono text-[11px] text-neutral-200 overflow-x-auto max-h-64 leading-relaxed select-all whitespace-pre"
            >{{ loadingScript ? '# 正在向主控生成部署脚本...' : renderedScript }}</pre>
        </div>
      </div>
    </div>

    <template #footer>
      <Button type="button" variant="outline" size="sm" @click="emit('update:modelValue', false)">
        我已妥善保存，关闭
      </Button>
      <Button
        type="button"
        variant="default"
        size="sm"
        @click="copyText(oneLineCommand, '已复制一键安装命令')"
      >
        <Copy class="w-3.5 h-3.5 mr-1.5" />
        <span>复制一键安装命令</span>
      </Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import {
  AlertTriangle,
  ChevronDown,
  ChevronRight,
  Copy,
  Info,
  KeyRound,
  RefreshCw,
  Terminal,
} from 'lucide-vue-next'
import Modal from '../../../components/ui/Modal.vue'
import Button from '../../../components/ui/Button.vue'
import Input from '../../../components/ui/Input.vue'
import FormField from '../../../components/ui/FormField.vue'
import { getNodeInstallScript } from '../../../api/nodes'
import { copyText } from '../../../utils/clipboard'
import type { NodeTokenModalState } from '../types'

const INSTALL_AGENT_SCRIPT_URL =
  'https://raw.githubusercontent.com/coding-leaf/xray-panel/master/deploy/install-agent.sh'

const shellSingleQuote = (val: string) => `'${val.replace(/'/g, `'\\''`)}'`

const props = defineProps<{
  modelValue: boolean
  modalState: NodeTokenModalState | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const masterUrl = ref('')
const rawScript = ref('')
const loadingScript = ref(false)
const scriptError = ref('')
const showFullScript = ref(false)

const modalTitle = computed(() => {
  if (!props.modalState) return '节点凭据与部署脚本'
  if (props.modalState.mode === 'created') {
    return `节点已创建：${props.modalState.nodeName}`
  }
  if (props.modalState.mode === 'rotated') {
    return `Token 已轮换：${props.modalState.nodeName}`
  }
  return `节点部署脚本：${props.modalState.nodeName}`
})

const modalDescription = computed(() => {
  if (!props.modalState) return ''
  if (props.modalState.token) {
    return '请立即复制一次性明文 Token 或直接复制下方一键安装命令到 SSH 终端执行'
  }
  return `节点 ID #${props.modalState.nodeId} · 生成一键安装命令与 systemd 守护进程脚本`
})

const oneLineCommand = computed(() => {
  const normalizedMasterUrl =
    masterUrl.value.trim().replace(/\/+$/, '') || 'https://panel.example.com'
  const tokenValue = props.modalState?.token?.trim() || '<YOUR_NODE_TOKEN>'
  return `curl -fsSL ${INSTALL_AGENT_SCRIPT_URL} | sudo MASTER_URL=${shellSingleQuote(normalizedMasterUrl)} AGENT_TOKEN=${shellSingleQuote(tokenValue)} bash`
})

const renderedScript = computed(() => {
  if (!rawScript.value) return ''
  const token = props.modalState?.token?.trim()
  if (!token) return rawScript.value
  return rawScript.value.replace(
    'NODE_TOKEN="${NODE_TOKEN:-<YOUR_NODE_TOKEN>}"',
    `NODE_TOKEN="\${NODE_TOKEN:-${token}}"`
  )
})

const loadInstallScript = async () => {
  if (!props.modalState?.nodeId) return
  loadingScript.value = true
  scriptError.value = ''
  try {
    const res = await getNodeInstallScript(props.modalState.nodeId, masterUrl.value)
    rawScript.value = res.script || ''
  } catch (err: any) {
    rawScript.value = ''
    scriptError.value = '生成安装脚本失败：' + (err?.message || err)
  } finally {
    loadingScript.value = false
  }
}

watch(
  () => [props.modelValue, props.modalState?.nodeId, props.modalState?.mode],
  ([open]) => {
    if (open && props.modalState) {
      if (!masterUrl.value && typeof window !== 'undefined') {
        masterUrl.value = window.location.origin
      }
      loadInstallScript()
    } else if (!open) {
      rawScript.value = ''
      scriptError.value = ''
      showFullScript.value = false
    }
  },
  { immediate: true }
)
</script>
