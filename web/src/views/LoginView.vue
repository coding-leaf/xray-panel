<template>
  <div class="relative min-h-screen flex items-center justify-center p-4 sm:p-6 lg:p-8 bg-background text-foreground selection:bg-neutral-800 selection:text-foreground">
    <!-- Login Container -->
    <div class="w-full max-w-[400px] relative z-10 my-auto">
      <!-- Status Pill -->
      <div class="flex justify-center mb-5">
        <div class="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-mono">
          <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
          <span>系统服务已就绪 · Ready</span>
        </div>
      </div>

      <!-- Card Frame -->
      <div class="rounded-lg bg-card border border-border p-6 sm:p-8 shadow-xl">
        <!-- Brand Logo Header -->
        <div class="text-center mb-6">
          <div class="w-12 h-12 mx-auto mb-3.5 rounded-md bg-neutral-900 border border-border flex items-center justify-center">
            <ShieldCheck class="w-6 h-6 text-foreground" />
          </div>
          <h1 class="text-lg font-semibold tracking-tight text-foreground font-mono">System Management Console</h1>
          <p class="text-xs text-muted-foreground mt-1">云网控制面板 · 分布式运维调度中心</p>
        </div>

        <!-- Login Form -->
        <form @submit.prevent="handleLogin" class="space-y-4">
          <!-- Username Field -->
          <div>
            <label class="block text-xs font-medium text-muted-foreground mb-1.5 font-mono">
              管理员账号
            </label>
            <div class="relative flex items-center">
              <div class="absolute left-3 flex items-center pointer-events-none text-muted-foreground">
                <User class="w-4 h-4" />
              </div>
              <input
                v-model="username"
                type="text"
                required
                autocomplete="username"
                placeholder="admin"
                class="w-full h-9 pl-9 pr-3 bg-neutral-950 border border-border rounded-md text-xs font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring focus:border-neutral-500 transition-colors"
              />
            </div>
          </div>

          <!-- Password Field -->
          <div>
            <div class="flex items-center justify-between mb-1.5">
              <label class="block text-xs font-medium text-muted-foreground font-mono">
                登录密码
              </label>
              <span v-if="capsLockOn" class="text-[11px] text-amber-400 font-mono">
                大写锁定已开启
              </span>
            </div>
            <div class="relative flex items-center">
              <div class="absolute left-3 flex items-center pointer-events-none text-muted-foreground">
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
                class="w-full h-9 pl-9 pr-9 bg-neutral-950 border border-border rounded-md text-xs font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring focus:border-neutral-500 transition-colors"
              />
              <button
                type="button"
                @click="showPassword = !showPassword"
                class="absolute right-2.5 text-muted-foreground hover:text-foreground transition-colors p-1"
                :title="showPassword ? '隐藏密码' : '显示密码'"
              >
                <EyeOff v-if="showPassword" class="w-3.5 h-3.5" />
                <Eye v-else class="w-3.5 h-3.5" />
              </button>
            </div>
          </div>

          <!-- 2FA TOTP Dynamic Code Field -->
          <div v-if="require2FA" class="pt-1">
            <div class="flex items-center justify-between mb-1.5">
              <label class="block text-xs font-medium text-foreground font-mono">
                两步验证动态口令 (2FA)
              </label>
              <span class="text-[11px] text-muted-foreground font-mono">6 位动态码</span>
            </div>
            <div class="relative flex items-center">
              <div class="absolute left-3 flex items-center pointer-events-none text-muted-foreground">
                <KeyRound class="w-4 h-4" />
              </div>
              <input
                v-model="passcode"
                type="text"
                maxlength="6"
                placeholder="000000"
                class="w-full h-9 pl-9 pr-3 bg-neutral-950 border border-border rounded-md text-xs font-mono tracking-[0.25em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring focus:border-neutral-500 transition-colors"
              />
            </div>
            <p class="text-[11px] text-muted-foreground mt-1">请打开身份验证器 (Google Authenticator) 输入动态码</p>
          </div>

          <!-- Error Notification Banner -->
          <div
            v-if="errorMsg"
            class="p-2.5 rounded-md bg-rose-500/10 border border-rose-500/20 text-rose-400 text-xs flex items-center gap-2"
          >
            <AlertCircle class="w-4 h-4 shrink-0 text-rose-400" />
            <span class="leading-snug">{{ errorMsg }}</span>
          </div>

          <!-- Demo Helper Button (if demo mode or empty) -->
          <div v-if="isDemo" class="flex items-center justify-between pt-1">
            <span class="text-[11px] text-muted-foreground">演示环境免密体验</span>
            <button
              type="button"
              @click="fillDemoAccount"
              class="text-[11px] text-foreground hover:underline transition-colors font-mono"
            >
              一键填入演示凭据
            </button>
          </div>

          <!-- Submit Button -->
          <Button
            type="submit"
            :loading="loading"
            class="w-full mt-2 h-9 text-xs"
          >
            <span>立即登录控制台</span>
            <ArrowRight v-if="!loading" class="w-3.5 h-3.5 ml-1.5" />
          </Button>
        </form>

        <!-- Security Trust Badges -->
        <div class="mt-6 pt-4 border-t border-border flex items-center justify-around text-[11px] text-muted-foreground font-mono">
          <span class="flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-neutral-500"></span>
            TLS 1.3 加密
          </span>
          <span class="text-neutral-700">|</span>
          <span class="flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-neutral-500"></span>
            RBAC 访问防护
          </span>
          <span class="text-neutral-700">|</span>
          <span class="flex items-center gap-1.5">
            <span class="w-1.5 h-1.5 rounded-full bg-neutral-500"></span>
            分布式协同
          </span>
        </div>
      </div>

      <!-- Copyright Notice -->
      <p class="text-center text-[11px] text-muted-foreground mt-6 tracking-wide font-mono">
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
import Button from '../components/ui/Button.vue'
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
