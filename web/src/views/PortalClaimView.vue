<template>
  <div class="relative min-h-screen flex items-center justify-center p-4 sm:p-6 lg:p-8 bg-background text-foreground selection:bg-neutral-800 selection:text-foreground">
    <div class="w-full max-w-xl relative z-10 my-auto py-8">
      <!-- Status Badge -->
      <div class="flex justify-center mb-5">
        <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-neutral-900 border border-border text-neutral-300 text-xs font-mono">
          <Shield class="w-3.5 h-3.5 text-neutral-400" />
          <span>分布式网管接入点 · 节点运维受保护</span>
        </div>
      </div>

      <!-- Card Container -->
      <div class="rounded-lg bg-card border border-border p-6 sm:p-8 shadow-xl">
        <!-- Header -->
        <div class="text-center mb-6">
          <div class="w-12 h-12 mx-auto mb-3.5 rounded-md bg-neutral-900 border border-border flex items-center justify-center">
            <KeyRound class="w-6 h-6 text-foreground" />
          </div>
          <h1 class="text-lg font-semibold tracking-tight text-foreground font-mono">分布式网管接入点</h1>
          <p class="text-xs text-muted-foreground mt-1 font-mono">Distributed Network Operations Point</p>
        </div>

        <!-- Phase 1: Input Ticket Form -->
        <div v-if="!payload" class="space-y-4">
          <div>
            <label class="block text-xs font-medium text-muted-foreground mb-2 text-center font-mono">
              请输入 6 位提取凭据 (一次性安全码)
            </label>
            <div class="relative max-w-xs mx-auto">
              <input
                ref="codeInputRef"
                v-model="inputCode"
                type="text"
                maxlength="8"
                placeholder="如: 7K9X2P"
                @input="handleInputFormat"
                @keydown.enter="handleClaim"
                class="w-full bg-neutral-950 border border-border rounded-md px-4 py-3 text-center text-2xl font-mono font-bold tracking-[0.4em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring focus:border-neutral-500 transition-colors uppercase"
              />
            </div>
            <p class="text-[11px] text-muted-foreground text-center mt-2 font-mono">
              不区分大小写，自动忽略混淆字符 (O/0、I/L/1)
            </p>
          </div>

          <!-- Error Notification Alert -->
          <div v-if="errorMessage" class="p-2.5 rounded-md bg-rose-500/10 border border-rose-500/20 flex items-start gap-2.5 text-rose-400 text-xs">
            <AlertCircle class="w-4 h-4 shrink-0 mt-0.5 text-rose-400" />
            <div class="flex-1 font-mono leading-snug">
              <span>{{ errorMessage }}</span>
            </div>
          </div>

          <Button
            @click="handleClaim"
            :disabled="loading || !inputCode.trim()"
            :loading="loading"
            class="w-full h-10 text-xs font-medium"
          >
            <span>{{ loading ? '正在验证凭据...' : '立即提取配置' }}</span>
            <ArrowRight v-if="!loading" class="w-3.5 h-3.5 ml-1.5" />
          </Button>
        </div>

        <!-- Phase 2: Dual-Track Delivery Display -->
        <div v-else class="space-y-5">
          <!-- Auto-burn Countdown Bar -->
          <div class="p-3 rounded-md bg-amber-500/10 border border-amber-500/20 flex items-center justify-between text-xs text-amber-400 font-mono">
            <div class="flex items-center gap-2">
              <Flame class="w-4 h-4 text-amber-400 animate-pulse" />
              <span>内存安全自毁倒计时</span>
            </div>
            <span class="font-bold">{{ countdownSeconds }} 秒后清除</span>
          </div>

          <!-- Track 1: Emergency Cold-start Nodes -->
          <div class="rounded-md bg-neutral-950 border border-border p-4 space-y-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
                <h3 class="text-xs font-semibold text-foreground tracking-wide font-mono">轨道 1：急救连接节点</h3>
              </div>
              <span class="text-[11px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                {{ payload.emergency_nodes?.length || 0 }} 个节点
              </span>
            </div>

            <p class="text-xs text-muted-foreground leading-relaxed">
              用于首次配网或建立高可用专属加密通道。
            </p>

            <!-- Main Action: Copy all emergency nodes -->
            <Button
              @click="copyEmergencyNodes"
              class="w-full h-9 text-xs"
            >
              <Check v-if="copiedNodes" class="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
              <Copy v-else class="w-3.5 h-3.5 mr-1.5" />
              <span>{{ copiedNodes ? '已成功复制全部节点链接！' : '一键复制全部急救节点' }}</span>
            </Button>

            <!-- Fallback Textarea for Mobile/WeChat -->
            <div class="pt-1">
              <div class="text-[11px] text-muted-foreground mb-1 font-mono">
                长按下方区域手动全选复制（手机端兜底）：
              </div>
              <textarea
                readonly
                rows="3"
                @click="selectTarget"
                :value="joinedNodes"
                class="w-full bg-neutral-900 border border-border rounded-md p-2 font-mono text-[11px] text-neutral-300 focus:outline-none focus:border-neutral-500 select-all"
              ></textarea>
            </div>

            <!-- Quick Client Instructions Accordion -->
            <div class="pt-2 border-t border-border">
              <button
                @click="showGuide = !showGuide"
                class="w-full flex items-center justify-between text-xs text-muted-foreground hover:text-foreground transition-colors py-1"
              >
                <span class="flex items-center gap-1.5">
                  <HelpCircle class="w-3.5 h-3.5 text-muted-foreground" />
                  <span>各客户端一键导入指引</span>
                </span>
                <ChevronDown class="w-3.5 h-3.5 transition-transform" :class="{ 'rotate-180': showGuide }" />
              </button>

              <div v-if="showGuide" class="mt-2 space-y-2 text-[11px] text-muted-foreground bg-neutral-900 p-3 rounded-md border border-border">
                <div>
                  <strong class="text-neutral-200">Android 客户端：</strong>
                  点击右上角「+」号 ➔ 选择「从剪贴板导入」➔ 启动连接。
                </div>
                <div>
                  <strong class="text-neutral-200">iOS 客户端：</strong>
                  打开客户端即会弹出提示「从剪贴板添加」➔ 点击允许并启动连接。
                </div>
                <div>
                  <strong class="text-neutral-200">PC / Mac 客户端：</strong>
                  在节点配置中选择从剪贴板粘贴节点 ➔ 选中后开启系统代理。
                </div>
              </div>
            </div>
          </div>

          <!-- Track 2: In-Tunnel Auto-Update Subscription -->
          <div class="rounded-md bg-neutral-950 border border-border p-4 space-y-3">
            <div class="flex items-center justify-between">
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full bg-blue-400"></span>
                <h3 class="text-xs font-semibold text-foreground tracking-wide font-mono">轨道 2：长效自动更新订阅</h3>
              </div>
              <span class="text-[11px] font-mono px-2 py-0.5 rounded bg-neutral-800 text-neutral-300 border border-border">
                长效保障
              </span>
            </div>

            <p class="text-xs text-muted-foreground leading-relaxed">
              在连接急救节点成功后，将订阅链接填入客户端。后续节点网络调整均可自动同步更新。
            </p>

            <div class="flex items-center gap-2">
              <input
                readonly
                :value="payload.subscription_url"
                @click="selectTarget"
                class="flex-1 h-9 bg-neutral-900 border border-border rounded-md px-3 text-xs font-mono text-neutral-300 focus:outline-none focus:border-neutral-500 select-all"
              />
              <Button
                variant="secondary"
                size="sm"
                @click="copySubscriptionURL"
                class="shrink-0 h-9 px-3"
              >
                <Check v-if="copiedSub" class="w-3.5 h-3.5 mr-1 text-emerald-400" />
                <Copy v-else class="w-3.5 h-3.5 mr-1" />
                <span>{{ copiedSub ? '已复制' : '复制' }}</span>
              </Button>
            </div>

            <div class="p-2.5 rounded-md bg-neutral-900 border border-border text-[11px] text-muted-foreground leading-normal">
              💡 <span class="font-medium text-foreground">使用建议：</span>添加订阅后，建议在客户端配置中开启<strong>「通过代理更新订阅」</strong>以保证在复杂网络下持续稳定同步。
            </div>
          </div>

          <!-- Manual Reset / Destroy Button -->
          <div class="pt-2 text-center">
            <button
              @click="resetPortal"
              class="text-xs text-muted-foreground hover:text-rose-400 transition-colors inline-flex items-center gap-1 font-mono"
            >
              <Trash2 class="w-3.5 h-3.5" />
              <span>立即销毁当前数据并返回</span>
            </button>
          </div>
        </div>
      </div>

      <!-- Footer Info -->
      <div class="text-center mt-6 text-xs text-muted-foreground space-y-1 font-mono">
        <p>端到端加密与访问控制已启用 · 阅后即焚保护机制</p>
        <p class="text-[10px] text-neutral-600">NetOps Version 1.3 · Distributed Network Operations Hub</p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import {
  Shield,
  KeyRound,
  ArrowRight,
  AlertCircle,
  Check,
  Copy,
  Flame,
  ChevronDown,
  HelpCircle,
  Trash2,
} from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import api from '../api'

const inputCode = ref('')
const codeInputRef = ref<HTMLInputElement | null>(null)
const loading = ref(false)
const errorMessage = ref('')
const payload = ref<any>(null)

const copiedNodes = ref(false)
const copiedSub = ref(false)
const showGuide = ref(false)

const countdownSeconds = ref(120)
let timer: any = null
let expireAtTimestamp = 0

const joinedNodes = computed(() => {
  if (!payload.value?.emergency_nodes) return ''
  return payload.value.emergency_nodes.join('\n')
})

const handleInputFormat = () => {
  let val = inputCode.value
  // 全角转半角
  val = val.replace(/[\uFF01-\uFF5E]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) - 0xFEE0))
  // 替换全角空格
  val = val.replace(/\u3000/g, ' ')
  // 移除所有空格并转为大写
  val = val.replace(/\s+/g, '').toUpperCase()
  inputCode.value = val
  errorMessage.value = ''
}

const copyToClipboard = async (text: string): Promise<boolean> => {
  try {
    if (navigator.clipboard && window.isSecureContext) {
      await navigator.clipboard.writeText(text)
      return true
    }
  } catch {
    // ignore and fallback
  }

  try {
    const textArea = document.createElement('textarea')
    textArea.value = text
    textArea.style.position = 'fixed'
    textArea.style.left = '-9999px'
    textArea.style.top = '0'
    document.body.appendChild(textArea)
    textArea.focus()
    textArea.select()
    const successful = document.execCommand('copy')
    document.body.removeChild(textArea)
    if (successful) return true
  } catch {
    // ignore
  }
  return false
}

const handleClaim = async () => {
  const code = inputCode.value.trim()
  if (!code) {
    errorMessage.value = '请输入提取码'
    return
  }

  loading.value = true
  errorMessage.value = ''

  try {
    const res: any = await api.post('/portal/claim', { code })
    payload.value = res
    startCountdown()
  } catch (err: any) {
    errorMessage.value = typeof err === 'string' ? err : (err.message || '凭据无效、已过期或已被销毁')
  } finally {
    loading.value = false
  }
}

const copyEmergencyNodes = async () => {
  if (!joinedNodes.value) return
  const ok = await copyToClipboard(joinedNodes.value)
  if (ok) {
    copiedNodes.value = true
    setTimeout(() => {
      copiedNodes.value = false
    }, 2500)
  }
}

const copySubscriptionURL = async () => {
  if (!payload.value?.subscription_url) return
  const ok = await copyToClipboard(payload.value.subscription_url)
  if (ok) {
    copiedSub.value = true
    setTimeout(() => {
      copiedSub.value = false
    }, 2500)
  }
}

const startCountdown = () => {
  expireAtTimestamp = Date.now() + 120 * 1000
  countdownSeconds.value = 120
  if (timer) clearInterval(timer)
  timer = setInterval(() => {
    const remaining = Math.ceil((expireAtTimestamp - Date.now()) / 1000)
    countdownSeconds.value = Math.max(0, remaining)
    if (countdownSeconds.value <= 0) {
      resetPortal()
    }
  }, 1000)
}

const handleVisibilityChange = () => {
  if (payload.value && expireAtTimestamp > 0) {
    if (Date.now() >= expireAtTimestamp) {
      resetPortal()
    } else {
      countdownSeconds.value = Math.max(0, Math.ceil((expireAtTimestamp - Date.now()) / 1000))
    }
  }
}

const resetPortal = () => {
  if (timer) clearInterval(timer)
  expireAtTimestamp = 0
  payload.value = null
  inputCode.value = ''
  errorMessage.value = ''
  countdownSeconds.value = 120
  nextTick(() => {
    codeInputRef.value?.focus()
  })
}

const selectTarget = (e: MouseEvent) => {
  const target = e.target as HTMLInputElement | HTMLTextAreaElement | null
  target?.select()
}

onMounted(() => {
  codeInputRef.value?.focus()
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
