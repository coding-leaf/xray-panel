<template>
  <div class="relative min-h-screen flex items-center justify-center p-4 sm:p-6 lg:p-8 overflow-hidden bg-[#07090E] text-slate-100 selection:bg-indigo-500/30 selection:text-indigo-200">
    <!-- Ambient Cyber Glows -->
    <div class="absolute -top-32 left-1/2 -translate-x-1/2 w-[550px] h-[350px] bg-gradient-to-tr from-indigo-600/20 via-brand-500/15 to-cyan-500/10 rounded-full blur-[110px] pointer-events-none"></div>
    <div class="absolute -bottom-24 -left-20 w-[380px] h-[380px] bg-purple-900/15 rounded-full blur-[120px] pointer-events-none"></div>
    <div class="absolute -bottom-24 -right-20 w-[380px] h-[380px] bg-cyan-600/10 rounded-full blur-[120px] pointer-events-none"></div>

    <!-- Fine Grid Background Overlay -->
    <div class="absolute inset-0 bg-[linear-gradient(to_right,#1e293b12_1px,transparent_1px),linear-gradient(to_bottom,#1e293b12_1px,transparent_1px)] bg-[size:3.5rem_3.5rem] [mask-image:radial-gradient(ellipse_60%_50%_at_50%_50%,#000_70%,transparent_100%)] pointer-events-none"></div>

    <!-- Login Container -->
    <div class="w-full max-w-[440px] relative z-10 my-auto">
      <!-- Status Pill -->
      <div class="flex justify-center mb-5">
        <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-medium tracking-wide shadow-sm">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>系统服务已就绪 · Ready</span>
        </div>
      </div>

      <!-- Glass Card Frame -->
      <div class="relative rounded-3xl p-[1px] bg-gradient-to-b from-white/15 via-white/[0.05] to-transparent shadow-[0_20px_60px_-15px_rgba(0,0,0,0.85)]">
        <!-- Top Accent Line -->
        <div class="absolute top-0 left-1/2 -translate-x-1/2 w-48 h-[2px] bg-gradient-to-r from-transparent via-indigo-500 to-transparent"></div>

        <div class="relative rounded-[23px] bg-[#0c101b]/90 backdrop-blur-2xl p-7 sm:p-9 border border-white/[0.04]">
          <!-- Brand Logo Header -->
          <div class="text-center mb-7">
            <div class="w-14 h-14 mx-auto mb-4 rounded-2xl p-[1px] bg-gradient-to-tr from-indigo-500 via-cyan-400 to-indigo-600 shadow-lg shadow-indigo-500/25 flex items-center justify-center">
              <div class="w-full h-full rounded-[15px] bg-[#0b0f19] flex items-center justify-center">
                <ShieldCheck class="w-7 h-7 text-indigo-400" />
              </div>
            </div>
            <h1 class="text-2xl font-bold tracking-tight text-white">System Management Console</h1>
            <p class="text-xs text-slate-400 mt-1.5">云网控制面板 · 分布式运维调度中心</p>
          </div>

          <!-- Login Form -->
          <form @submit.prevent="handleLogin" class="space-y-4">
            <!-- Username Field -->
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-slate-400 mb-1.5">
                管理员账号
              </label>
              <div class="relative group">
                <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-500 group-focus-within:text-indigo-400 transition-colors">
                  <User class="w-4 h-4" />
                </div>
                <input
                  v-model="username"
                  type="text"
                  required
                  autocomplete="username"
                  placeholder="admin"
                  class="w-full bg-[#111624]/80 border border-slate-700/60 rounded-xl pl-10 pr-4 py-2.5 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-all duration-200"
                />
              </div>
            </div>

            <!-- Password Field -->
            <div>
              <div class="flex items-center justify-between mb-1.5">
                <label class="block text-xs font-semibold uppercase tracking-wider text-slate-400">
                  登录密码
                </label>
                <span v-if="capsLockOn" class="text-[10px] text-amber-400 font-medium animate-pulse">
                  大写锁定已开启
                </span>
              </div>
              <div class="relative group">
                <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-slate-500 group-focus-within:text-indigo-400 transition-colors">
                  <Lock class="w-4 h-4" />
                </div>
                <input
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  required
                  autocomplete="current-password"
                  placeholder="••••••••"
                  @keyup="checkCapsLock"
                  @keydown="checkCapsLock"
                  class="w-full bg-[#111624]/80 border border-slate-700/60 rounded-xl pl-10 pr-10 py-2.5 text-sm text-white placeholder-slate-500 focus:outline-none focus:border-indigo-500 focus:ring-1 focus:ring-indigo-500 transition-all duration-200"
                />
                <button
                  type="button"
                  @click="showPassword = !showPassword"
                  class="absolute inset-y-0 right-0 pr-3 flex items-center text-slate-500 hover:text-slate-300 transition-colors focus:outline-none"
                  :title="showPassword ? '隐藏密码' : '显示密码'"
                >
                  <EyeOff v-if="showPassword" class="w-4 h-4" />
                  <Eye v-else class="w-4 h-4" />
                </button>
              </div>
            </div>

            <!-- 2FA TOTP Dynamic Code Field -->
            <div v-if="require2FA" class="pt-1">
              <div class="flex items-center justify-between mb-1.5">
                <label class="block text-xs font-semibold uppercase tracking-wider text-cyan-400">
                  两步验证动态口令 (2FA)
                </label>
                <span class="text-[10px] text-cyan-400/80">6 位动态码</span>
              </div>
              <div class="relative group">
                <div class="absolute inset-y-0 left-0 pl-3.5 flex items-center pointer-events-none text-cyan-500">
                  <KeyRound class="w-4 h-4" />
                </div>
                <input
                  v-model="passcode"
                  type="text"
                  maxlength="6"
                  placeholder="000000"
                  class="w-full bg-[#111624]/80 border border-cyan-500/50 rounded-xl pl-10 pr-4 py-2.5 text-sm text-cyan-300 placeholder-slate-600 focus:outline-none focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400 transition-all duration-200 font-mono tracking-[0.25em]"
                />
              </div>
              <p class="text-[11px] text-slate-500 mt-1">请打开身份验证器 (Google Authenticator) 输入动态码</p>
            </div>

            <!-- Error Notification Banner -->
            <div
              v-if="errorMsg"
              class="p-3 rounded-xl bg-rose-500/10 border border-rose-500/25 text-rose-300 text-xs flex items-center gap-2.5 transition-all duration-200 animate-shake"
            >
              <AlertCircle class="w-4 h-4 shrink-0 text-rose-400" />
              <span class="leading-snug">{{ errorMsg }}</span>
            </div>

            <!-- Demo Helper Button (if demo mode or empty) -->
            <div v-if="isDemo" class="flex items-center justify-between pt-1">
              <span class="text-[11px] text-slate-500">演示环境免密体验</span>
              <button
                type="button"
                @click="fillDemoAccount"
                class="text-[11px] text-indigo-400 hover:text-indigo-300 transition-colors underline underline-offset-2"
              >
                一键填入演示凭据
              </button>
            </div>

            <!-- Submit Button -->
            <button
              type="submit"
              :disabled="loading"
              class="group relative w-full mt-2 bg-gradient-to-r from-indigo-600 via-indigo-500 to-cyan-500 hover:from-indigo-500 hover:to-cyan-400 text-white font-semibold py-2.5 rounded-xl text-sm transition-all duration-200 shadow-lg shadow-indigo-500/25 hover:shadow-indigo-500/40 disabled:opacity-50 disabled:pointer-events-none flex items-center justify-center gap-2 overflow-hidden"
            >
              <span
                v-if="loading"
                class="animate-spin w-4 h-4 border-2 border-white border-t-transparent rounded-full"
              ></span>
              <template v-else>
                <span>立即登录控制台</span>
                <ArrowRight class="w-4 h-4 group-hover:translate-x-0.5 transition-transform duration-200" />
              </template>
            </button>
          </form>

          <!-- Security Trust Badges -->
          <div class="mt-8 pt-6 border-t border-white/[0.06] flex items-center justify-around text-[11px] text-slate-400">
            <span class="flex items-center gap-1 hover:text-slate-300 transition-colors">
              <span class="w-1 h-1 rounded-full bg-indigo-400"></span>
              TLS 1.3 加密
            </span>
            <span class="text-slate-700">|</span>
            <span class="flex items-center gap-1 hover:text-slate-300 transition-colors">
              <span class="w-1 h-1 rounded-full bg-cyan-400"></span>
              RBAC 访问防护
            </span>
            <span class="text-slate-700">|</span>
            <span class="flex items-center gap-1 hover:text-slate-300 transition-colors">
              <span class="w-1 h-1 rounded-full bg-indigo-400"></span>
              分布式协同
            </span>
          </div>
        </div>
      </div>

      <!-- Copyright Notice -->
      <p class="text-center text-[11px] text-slate-400 mt-6 tracking-wide">
        © 2026 System Management Console. All rights reserved.
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import {
  ShieldCheck,
  User,
  Lock,
  Eye,
  EyeOff,
  KeyRound,
  ArrowRight,
  AlertCircle,
} from 'lucide-vue-next'
import api from '../api'
import { isMockMode } from '../mock'

const router = useRouter()
const username = ref('admin')
const password = ref('')
const passcode = ref('')
const require2FA = ref(false)
const loading = ref(false)
const errorMsg = ref('')
const showPassword = ref(false)
const capsLockOn = ref(false)
const isDemo = ref(false)

onMounted(() => {
  isDemo.value = isMockMode()
})

const checkCapsLock = (e: KeyboardEvent) => {
  capsLockOn.value = e.getModifierState && e.getModifierState('CapsLock')
}

const fillDemoAccount = () => {
  username.value = 'admin'
  password.value = 'admin123'
  errorMsg.value = ''
}

const handleLogin = async () => {
  loading.value = true
  errorMsg.value = ''
  try {
    const res: any = await api.post('/auth/login', {
      username: username.value,
      password: password.value,
      passcode: passcode.value,
    })
    localStorage.setItem('token', res.token)
    localStorage.setItem('username', res.username)
    router.push('/')
  } catch (err: any) {
    errorMsg.value = typeof err === 'string' ? err : '登录失败，请检查账号密码'
    if (errorMsg.value.includes('2fa') || errorMsg.value.includes('passcode')) {
      require2FA.value = true
    }
  } finally {
    loading.value = false
  }
}
</script>
