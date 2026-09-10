<template>
  <div class="relative min-h-screen flex items-center justify-center p-4 sm:p-6 lg:p-8 overflow-hidden bg-[#07090E] text-slate-100 selection:bg-indigo-500/30 selection:text-indigo-200">
    <!-- Ambient Cyber Glows -->
    <div class="absolute -top-32 left-1/2 -translate-x-1/2 w-[550px] h-[350px] bg-gradient-to-tr from-indigo-600/20 via-cyan-500/10 to-transparent rounded-full blur-[110px] pointer-events-none"></div>
    <div class="absolute -bottom-24 -left-20 w-[380px] h-[380px] bg-purple-900/15 rounded-full blur-[120px] pointer-events-none"></div>
    <div class="absolute -bottom-24 -right-20 w-[380px] h-[380px] bg-cyan-600/10 rounded-full blur-[120px] pointer-events-none"></div>

    <!-- Fine Grid Background Overlay -->
    <div class="absolute inset-0 bg-[linear-gradient(to_right,#1e293b12_1px,transparent_1px),linear-gradient(to_bottom,#1e293b12_1px,transparent_1px)] bg-[size:3.5rem_3.5rem] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_50%,#000_70%,transparent_100%)] pointer-events-none"></div>

    <div class="w-full max-w-xl relative z-10 my-auto py-8">
      <!-- Status Badge -->
      <div class="flex justify-center mb-5">
        <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-indigo-500/10 border border-indigo-500/20 text-indigo-300 text-xs font-medium tracking-wide shadow-sm">
          <Shield class="w-3.5 h-3.5 text-indigo-400" />
          <span>分布式网管接入点 · 节点运维受保护</span>
        </div>
      </div>

      <!-- Glass Frame Container -->
      <div class="relative rounded-3xl p-[1px] bg-gradient-to-b from-white/15 via-white/[0.05] to-transparent shadow-[0_20px_60px_-15px_rgba(0,0,0,0.85)]">
        <div class="absolute top-0 left-1/2 -translate-x-1/2 w-48 h-[2px] bg-gradient-to-r from-transparent via-indigo-500 to-transparent"></div>

        <div class="relative rounded-[23px] bg-[#0c101b]/95 backdrop-blur-2xl p-6 sm:p-9 border border-white/[0.04]">
          <!-- Header -->
          <div class="text-center mb-7">
            <div class="w-13 h-13 mx-auto mb-3.5 rounded-2xl p-[1px] bg-gradient-to-tr from-indigo-500 via-cyan-400 to-indigo-600 shadow-lg shadow-indigo-500/20 flex items-center justify-center">
              <div class="w-12 h-12 rounded-[15px] bg-[#0b0f19] flex items-center justify-center">
                <KeyRound class="w-6 h-6 text-indigo-400" />
              </div>
            </div>
            <h1 class="text-xl sm:text-2xl font-bold tracking-tight text-white">分布式网管接入点</h1>
            <p class="text-xs text-slate-400 mt-1.5 font-mono">Distributed Network Operations Point</p>
          </div>

          <!-- Phase 1: Input Ticket Form -->
          <div v-if="!payload" class="space-y-5">
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-slate-400 mb-2 text-center">
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
                  class="w-full bg-[#111624]/90 border border-slate-700/80 rounded-xl px-4 py-3.5 text-center text-2xl font-mono font-bold tracking-[0.4em] text-indigo-300 placeholder-slate-600 focus:outline-none focus:border-indigo-500 focus:ring-2 focus:ring-indigo-500/30 transition-all uppercase"
                />
              </div>
              <p class="text-[11px] text-slate-500 text-center mt-2">
                不区分大小写，自动忽略混淆字符 (O/0、I/L/1)
              </p>
            </div>

            <!-- Error Notification Alert -->
            <div v-if="errorMessage" class="p-3.5 rounded-xl bg-rose-500/10 border border-rose-500/20 flex items-start gap-3 text-rose-300 text-xs">
              <AlertCircle class="w-4 h-4 shrink-0 mt-0.5 text-rose-400" />
              <div class="flex-1">
                <span class="font-medium">{{ errorMessage }}</span>
              </div>
            </div>

            <button
              @click="handleClaim"
              :disabled="loading || !inputCode.trim()"
              class="w-full relative group overflow-hidden rounded-xl bg-gradient-to-r from-indigo-600 via-indigo-500 to-cyan-500 p-[1px] font-semibold text-white shadow-lg shadow-indigo-500/20 disabled:opacity-50 disabled:cursor-not-allowed transition-all duration-200"
            >
              <div class="relative flex items-center justify-center gap-2 rounded-xl bg-[#0c101b] px-4 py-3 group-hover:bg-opacity-0 transition-all duration-200">
                <Loader2 v-if="loading" class="w-4 h-4 animate-spin text-indigo-400" />
                <ArrowRight v-else class="w-4 h-4 text-indigo-400 group-hover:translate-x-0.5 transition-transform" />
                <span class="text-sm tracking-wide">{{ loading ? '正在验证凭据...' : '立即提取配置' }}</span>
              </div>
            </button>
          </div>

          <!-- Phase 2: Dual-Track Delivery Display -->
          <div v-else class="space-y-6">
            <!-- Auto-burn Countdown Bar -->
            <div class="p-3 rounded-xl bg-amber-500/10 border border-amber-500/20 flex items-center justify-between text-xs text-amber-300">
              <div class="flex items-center gap-2">
                <Flame class="w-4 h-4 text-amber-400 animate-pulse" />
                <span>内存安全自毁倒计时</span>
              </div>
              <span class="font-mono font-bold text-amber-400">{{ countdownSeconds }} 秒后清除</span>
            </div>

            <!-- Track 1: Emergency Cold-start Nodes -->
            <div class="rounded-2xl bg-[#111624]/70 border border-slate-800 p-4 space-y-3">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <div class="w-2 h-2 rounded-full bg-emerald-400"></div>
                  <h3 class="text-sm font-bold text-white tracking-wide">轨道 1：急救连接节点</h3>
                </div>
                <span class="text-[11px] font-mono px-2 py-0.5 rounded bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                  {{ payload.emergency_nodes?.length || 0 }} 个节点
                </span>
              </div>

              <p class="text-xs text-slate-400 leading-relaxed">
                用于首次配网或建立高可用专属加密通道。
              </p>

              <!-- Main Action: Copy all emergency nodes -->
              <button
                @click="copyEmergencyNodes"
                class="w-full flex items-center justify-center gap-2 py-2.5 px-4 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white text-xs font-semibold shadow-md shadow-indigo-600/20 transition-colors"
              >
                <Check v-if="copiedNodes" class="w-4 h-4 text-emerald-300" />
                <Copy v-else class="w-4 h-4" />
                <span>{{ copiedNodes ? '已成功复制全部节点链接！' : '一键复制全部急救节点' }}</span>
              </button>

              <!-- Fallback Textarea for Mobile/WeChat -->
              <div class="pt-1">
                <div class="flex items-center justify-between text-[11px] text-slate-400 mb-1">
                  <span>长按下方区域手动全选复制（手机端兜底）：</span>
                </div>
                <textarea
                  readonly
                  rows="3"
                  @click="selectTarget"
                  :value="joinedNodes"
                  class="w-full bg-black/40 border border-slate-800 rounded-lg p-2 font-mono text-[11px] text-slate-300 focus:outline-none focus:border-indigo-500/60 select-all"
                ></textarea>
              </div>

              <!-- Quick Client Instructions Accordion -->
              <div class="pt-2 border-t border-white/[0.05]">
                <button
                  @click="showGuide = !showGuide"
                  class="w-full flex items-center justify-between text-xs text-slate-400 hover:text-slate-200 transition-colors py-1"
                >
                  <span class="flex items-center gap-1.5">
                    <HelpCircle class="w-3.5 h-3.5 text-indigo-400" />
                    <span>各客户端一键导入指引</span>
                  </span>
                  <ChevronDown class="w-3.5 h-3.5 transition-transform" :class="{ 'rotate-180': showGuide }" />
                </button>

                <div v-if="showGuide" class="mt-2 space-y-2 text-[11px] text-slate-400 bg-black/30 p-3 rounded-xl border border-white/[0.04]">
                  <div>
                    <strong class="text-slate-200">Android 客户端：</strong>
                    点击右上角「+」号 ➔ 选择「从剪贴板导入」➔ 启动连接。
                  </div>
                  <div>
                    <strong class="text-slate-200">iOS 客户端：</strong>
                    打开客户端即会弹出提示「从剪贴板添加」➔ 点击允许并启动连接。
                  </div>
                  <div>
                    <strong class="text-slate-200">PC / Mac 客户端：</strong>
                    在节点配置中选择从剪贴板粘贴节点 ➔ 选中后开启系统代理。
                  </div>
                </div>
              </div>
            </div>

            <!-- Track 2: In-Tunnel Auto-Update Subscription -->
            <div class="rounded-2xl bg-[#111624]/70 border border-slate-800 p-4 space-y-3">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <div class="w-2 h-2 rounded-full bg-indigo-400"></div>
                  <h3 class="text-sm font-bold text-white tracking-wide">轨道 2：长效自动更新订阅</h3>
                </div>
                <span class="text-[11px] font-mono px-2 py-0.5 rounded bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
                  长效保障
                </span>
              </div>

              <p class="text-xs text-slate-400 leading-relaxed">
                在连接急救节点成功后，将订阅链接填入客户端。后续节点网络调整均可自动同步更新。
              </p>

              <div class="flex items-center gap-2">
                <input
                  readonly
                  :value="payload.subscription_url"
                  @click="selectTarget"
                  class="flex-1 bg-black/40 border border-slate-800 rounded-xl px-3 py-2 text-xs font-mono text-slate-300 focus:outline-none select-all"
                />
                <button
                  @click="copySubscriptionURL"
                  class="shrink-0 px-3 py-2 rounded-xl bg-white/[0.08] hover:bg-white/[0.12] text-white text-xs font-semibold transition-colors flex items-center gap-1.5"
                >
                  <Check v-if="copiedSub" class="w-3.5 h-3.5 text-emerald-300" />
                  <Copy v-else class="w-3.5 h-3.5" />
                  <span>{{ copiedSub ? '已复制' : '复制' }}</span>
                </button>
              </div>

              <div class="p-2.5 rounded-xl bg-indigo-500/5 border border-indigo-500/15 text-[11px] text-indigo-300/90 leading-normal">
                💡 <span class="font-semibold">使用建议：</span>添加订阅后，建议在客户端配置中开启<strong>「通过代理更新订阅」</strong>以保证在复杂网络下持续稳定同步。
              </div>
            </div>

            <!-- Manual Reset / Destroy Button -->
            <div class="pt-2 text-center">
              <button
                @click="resetPortal"
                class="text-xs text-slate-500 hover:text-rose-400 transition-colors inline-flex items-center gap-1"
              >
                <Trash2 class="w-3.5 h-3.5" />
                <span>立即销毁当前数据并返回</span>
              </button>
            </div>
          </div>
        </div>
      </div>

      <!-- Footer Info -->
      <div class="text-center mt-6 text-xs text-slate-600 space-y-1">
        <p>端到端加密与访问控制已启用 · 阅后即焚保护机制</p>
        <p class="text-[10px] font-mono text-slate-700">NetOps Version 1.3 · Distributed Network Operations Hub</p>
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
  Loader2,
  AlertCircle,
  Check,
  Copy,
  Flame,
  ChevronDown,
  HelpCircle,
  Trash2,
} from 'lucide-vue-next'
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
