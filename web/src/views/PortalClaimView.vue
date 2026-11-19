<template>
  <main
    class="relative min-h-screen flex items-center justify-center p-4 sm:p-6 lg:p-8 bg-background text-foreground selection:bg-neutral-800 selection:text-foreground"
  >
    <div class="w-full max-w-xl relative z-10 my-auto py-8">
      <!-- Status Badge -->
      <div class="flex justify-center mb-5">
        <div
          class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-neutral-900 border border-border text-neutral-300 text-xs font-mono"
        >
          <Shield class="w-3.5 h-3.5 text-neutral-400" />
          <span>分布式网管接入点 · 节点运维受保护</span>
        </div>
      </div>

      <!-- Card Container -->
      <div class="rounded-lg bg-card border border-border p-6 sm:p-8 shadow-xl">
        <!-- Header -->
        <div class="text-center mb-6">
          <div
            class="w-12 h-12 mx-auto mb-3.5 rounded-md bg-neutral-900 border border-border flex items-center justify-center"
          >
            <KeyRound class="w-6 h-6 text-foreground" />
          </div>
          <h1 class="text-lg font-semibold tracking-tight text-foreground font-mono">
            分布式网管接入点
          </h1>
          <p class="text-xs text-muted-foreground mt-1 font-mono">
            Distributed Network Operations Point
          </p>
        </div>

        <!-- Phase 1: Input Ticket Form -->
        <form v-if="!payload" @submit.prevent="handleClaim" class="space-y-4">
          <div>
            <label
              for="portal-claim-code"
              class="block text-xs font-medium text-muted-foreground mb-2 text-center font-mono"
            >
              请输入 6 位提取凭据 (一次性安全码)
            </label>
            <div class="relative max-w-xs mx-auto">
              <input
                id="portal-claim-code"
                ref="codeInputRef"
                v-model="inputCode"
                name="code"
                type="text"
                maxlength="8"
                autocomplete="one-time-code"
                autocorrect="off"
                autocapitalize="characters"
                spellcheck="false"
                enterkeyhint="done"
                placeholder="如：7K9X2P"
                @input="handleInputFormat"
                class="w-full bg-neutral-950 border border-border rounded-md px-4 py-3 text-center text-2xl font-mono font-bold tracking-[0.4em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring focus:border-neutral-500 transition-colors uppercase"
              />
            </div>
            <p class="text-[11px] text-muted-foreground text-center mt-2 font-mono">
              不区分大小写，自动忽略混淆字符 (O/0、I/L/1)
            </p>
          </div>

          <!-- Error Notification Alert -->
          <div
            v-if="errorMessage"
            role="alert"
            class="p-2.5 rounded-md bg-rose-500/10 border border-rose-500/20 flex items-start gap-2.5 text-rose-400 text-xs"
          >
            <AlertCircle class="w-4 h-4 shrink-0 mt-0.5 text-rose-400" />
            <div class="flex-1 font-mono leading-snug">
              <span>{{ errorMessage }}</span>
            </div>
          </div>

          <Button
            type="submit"
            :disabled="loading || !inputCode.trim()"
            :loading="loading"
            class="w-full h-10 text-xs font-medium"
          >
            <span>{{ loading ? '正在验证凭据...' : '立即提取配置' }}</span>
            <ArrowRight v-if="!loading" class="w-3.5 h-3.5 ml-1.5" />
          </Button>
        </form>

        <!-- Phase 2: Subscription-First Delivery Display -->
        <div v-else class="space-y-4">
          <!-- Auto-burn Countdown Bar -->
          <div
            class="p-3 rounded-md bg-amber-500/10 border border-amber-500/20 flex items-center justify-between text-xs text-amber-400 font-mono"
          >
            <div class="flex items-center gap-2">
              <Flame class="w-4 h-4 text-amber-400 animate-pulse" />
              <span>内存安全自毁倒计时</span>
            </div>
            <span class="font-bold">{{ countdownSeconds }} 秒后清除</span>
          </div>

          <!-- Primary Section: Auto-Update Subscription (Recommended) -->
          <section
            aria-labelledby="portal-sub-heading"
            class="rounded-lg bg-neutral-950 border border-emerald-500/40 p-4 sm:p-5 space-y-3.5"
          >
            <div class="flex items-center justify-between gap-2 flex-wrap">
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full bg-emerald-400 shrink-0"></span>
                <h2
                  id="portal-sub-heading"
                  class="text-sm font-semibold text-foreground tracking-wide font-mono"
                >
                  首选：长效订阅链接
                </h2>
              </div>
              <span
                class="text-[11px] font-mono px-2 py-0.5 rounded bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 font-medium"
              >
                强烈推荐 · 自动更新
              </span>
            </div>

            <p class="text-xs text-neutral-300 leading-relaxed">
              请优先复制此<strong class="text-foreground">订阅链接</strong
              >导入客户端。添加一次即可长期使用，后续节点维护或扩容只需在客户端点击「更新订阅」，<strong
                class="text-emerald-400 font-medium"
                >无需再次索取提取码</strong
              >。
            </p>

            <!-- Primary CTA Button -->
            <Button
              type="button"
              :disabled="!payload.subscription_url"
              @click="copySubscriptionURL"
              class="w-full h-10 text-xs font-semibold"
            >
              <Check v-if="copiedSub" class="w-4 h-4 mr-1.5 text-emerald-600" />
              <Copy v-else class="w-4 h-4 mr-1.5" />
              <span>{{
                copiedSub ? '已复制订阅链接，请前往客户端添加订阅' : '一键复制长效订阅链接'
              }}</span>
            </Button>

            <!-- Readonly URL Input (Click-to-select fallback) -->
            <div class="space-y-1">
              <div class="text-[11px] text-muted-foreground font-mono">
                订阅地址（点击输入框可手动全选复制）：
              </div>
              <input
                readonly
                aria-label="长效订阅链接"
                :value="payload.subscription_url"
                @click="selectTarget"
                class="w-full h-9 bg-neutral-900 border border-border rounded-md px-3 text-xs font-mono text-neutral-300 focus:outline-none focus:border-neutral-500 select-all"
              />
            </div>

            <!-- Important Client Setting Tip -->
            <div
              class="p-2.5 rounded-md bg-emerald-500/5 border border-emerald-500/20 text-[11px] text-neutral-300 leading-relaxed flex items-start gap-2"
            >
              <RefreshCw class="w-3.5 h-3.5 text-emerald-400 shrink-0 mt-0.5" />
              <div>
                <span class="font-medium text-foreground">关键设置：</span>
                导入订阅后，建议在客户端开启<strong class="text-foreground">「自动更新」</strong
                >及<strong class="text-foreground">「通过代理更新订阅」</strong
                >，确保复杂网络下也能持续同步最新节点。
              </div>
            </div>

            <!-- Subscription Import Guide (Native Details Disclosure) -->
            <details class="portal-details pt-3 border-t border-border">
              <summary
                class="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground transition-colors py-1 select-none"
              >
                <ChevronDown class="portal-chevron w-3.5 h-3.5 text-muted-foreground" />
                <BookOpen class="w-3.5 h-3.5 text-emerald-400 shrink-0" />
                <span class="font-medium text-neutral-200"
                  >如何添加订阅链接？（iOS / Android / 电脑端步骤）</span
                >
              </summary>

              <div
                class="mt-2.5 space-y-2.5 text-[11px] text-muted-foreground bg-neutral-900 p-3 rounded-md border border-border leading-relaxed"
              >
                <div>
                  <strong class="text-neutral-200">iOS（Shadowrocket / 小火箭等）：</strong>
                  点击右上角「<code class="text-neutral-200">+</code>」➔ 类型选择
                  <strong class="text-emerald-400">「Subscribe / 订阅」</strong>
                  （请勿直接点普通节点）➔ 在 URL 栏粘贴订阅链接并保存 ➔ 返回首页选中节点连接。
                </div>
                <div>
                  <strong class="text-neutral-200"
                    >Android（v2rayNG / Clash Meta / Nekobox）：</strong
                  >
                  打开菜单进入「<strong class="text-neutral-200">订阅分组设置</strong>」或「<strong
                    class="text-neutral-200"
                    >配置</strong
                  >」➔ 点击「<code class="text-neutral-200">+</code>」新建订阅并粘贴链接 ➔
                  保存后点击「<strong class="text-emerald-400">更新订阅</strong>」拉取全部节点。
                </div>
                <div>
                  <strong class="text-neutral-200"
                    >Windows / macOS（v2rayN / Clash Verge / FlClash）：</strong
                  >
                  进入「<strong class="text-neutral-200">订阅设置 / 配置</strong>」➔
                  新建订阅并粘贴链接 ➔ 点击「<strong class="text-emerald-400">更新订阅</strong
                  >」获取最新节点。
                </div>
                <div class="pt-2 border-t border-border text-amber-300/90">
                  <strong>如果直接拉取订阅超时或失败：</strong>
                  请展开下方「临时过渡节点」先临时连上网络，再回来更新上方订阅链接即可。
                </div>
              </div>
            </details>
          </section>

          <!-- Secondary Section: Emergency Static Nodes (Collapsed by Default) -->
          <details class="portal-details rounded-lg bg-neutral-950/70 border border-border">
            <summary
              class="flex items-center justify-between gap-2 p-3.5 text-xs text-muted-foreground hover:text-foreground transition-colors select-none"
            >
              <span class="flex items-center gap-2 min-w-0">
                <ChevronDown class="portal-chevron w-3.5 h-3.5 text-muted-foreground" />
                <span class="w-2 h-2 rounded-full bg-amber-400 shrink-0"></span>
                <span class="font-medium text-neutral-300 truncate font-mono">
                  无法拉取订阅？展开临时过渡节点（{{ payload.emergency_nodes?.length || 0 }} 个）
                </span>
              </span>
              <span
                class="shrink-0 text-[10px] font-mono px-2 py-0.5 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20"
              >
                不会自动更新
              </span>
            </summary>

            <div class="px-4 pb-4 pt-1 space-y-3 border-t border-border/60">
              <!-- Strong Warning Banner -->
              <div
                class="mt-3 p-3 rounded-md bg-amber-500/10 border border-amber-500/25 text-xs text-amber-200/90 space-y-1.5 leading-relaxed"
              >
                <div class="flex items-center gap-1.5 font-semibold text-amber-400">
                  <AlertTriangle class="w-3.5 h-3.5 shrink-0" />
                  <span>请勿把临时节点当作日常长期节点使用</span>
                </div>
                <p class="text-[11px] text-amber-200/80">
                  以下仅为当前时刻的静态节点快照，<strong>后续维护调整后将直接失效且不会自动更新</strong>。仅在当前网络无法直接拉取订阅时用于冷启动过渡：
                </p>
                <ol class="list-decimal list-inside text-[11px] text-amber-200/90 space-y-1 pt-0.5">
                  <li>复制下方临时节点，在客户端选择「从剪贴板导入」并先开启连接；</li>
                  <li>
                    联网后，<strong>立即回到上方复制「长效订阅链接」</strong>并更新订阅，之后切换至订阅节点并删除临时节点。
                  </li>
                </ol>
              </div>

              <!-- Secondary Action: Copy emergency nodes -->
              <Button
                type="button"
                variant="secondary"
                :disabled="!joinedNodes"
                @click="copyEmergencyNodes"
                class="w-full h-9 text-xs"
              >
                <Check v-if="copiedNodes" class="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
                <Copy v-else class="w-3.5 h-3.5 mr-1.5" />
                <span>{{
                  copiedNodes
                    ? '已复制临时节点（联网后请务必添加上方订阅）'
                    : '复制临时过渡节点（仅作首次联网）'
                }}</span>
              </Button>

              <!-- Fallback Textarea for Mobile/WeChat -->
              <div>
                <div class="text-[11px] text-muted-foreground mb-1 font-mono">
                  手机端手动全选兜底（长按下方区域复制）：
                </div>
                <textarea
                  readonly
                  rows="3"
                  aria-label="临时过渡节点链接列表"
                  @click="selectTarget"
                  :value="joinedNodes"
                  class="w-full bg-neutral-900 border border-border rounded-md p-2 font-mono text-[11px] text-neutral-300 focus:outline-none focus:border-neutral-500 select-all"
                ></textarea>
              </div>
            </div>
          </details>

          <!-- Manual Reset / Destroy Button -->
          <div class="pt-2 text-center">
            <button
              type="button"
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
        <p class="text-[10px] text-neutral-600">
          NetOps Version 1.3 · Distributed Network Operations Hub
        </p>
      </div>
    </div>
  </main>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick } from 'vue'
import {
  Shield,
  KeyRound,
  ArrowRight,
  AlertCircle,
  AlertTriangle,
  BookOpen,
  Check,
  Copy,
  Flame,
  ChevronDown,
  RefreshCw,
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

const countdownSeconds = ref(120)
let timer: ReturnType<typeof setInterval> | null = null
let copiedNodesTimer: ReturnType<typeof setTimeout> | null = null
let copiedSubTimer: ReturnType<typeof setTimeout> | null = null
let expireAtTimestamp = 0

const joinedNodes = computed(() => {
  if (!payload.value?.emergency_nodes) return ''
  return payload.value.emergency_nodes.join('\n')
})

const handleInputFormat = () => {
  let val = inputCode.value
  // 全角转半角
  val = val.replace(/[\uFF01-\uFF5E]/g, (ch) => String.fromCharCode(ch.charCodeAt(0) - 0xfee0))
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
    errorMessage.value = typeof err === 'string' ? err : err.message || '凭据无效、已过期或已被销毁'
  } finally {
    loading.value = false
  }
}

const copyEmergencyNodes = async () => {
  if (!joinedNodes.value) return
  const ok = await copyToClipboard(joinedNodes.value)
  if (ok) {
    copiedNodes.value = true
    if (copiedNodesTimer) clearTimeout(copiedNodesTimer)
    copiedNodesTimer = setTimeout(() => {
      copiedNodes.value = false
      copiedNodesTimer = null
    }, 2500)
  }
}

const copySubscriptionURL = async () => {
  if (!payload.value?.subscription_url) return
  const ok = await copyToClipboard(payload.value.subscription_url)
  if (ok) {
    copiedSub.value = true
    if (copiedSubTimer) clearTimeout(copiedSubTimer)
    copiedSubTimer = setTimeout(() => {
      copiedSub.value = false
      copiedSubTimer = null
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
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  if (copiedNodesTimer) {
    clearTimeout(copiedNodesTimer)
    copiedNodesTimer = null
  }
  if (copiedSubTimer) {
    clearTimeout(copiedSubTimer)
    copiedSubTimer = null
  }
  copiedNodes.value = false
  copiedSub.value = false
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
  if (copiedNodesTimer) clearTimeout(copiedNodesTimer)
  if (copiedSubTimer) clearTimeout(copiedSubTimer)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
