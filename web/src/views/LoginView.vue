<template>
  <div class="min-h-screen flex items-center justify-center p-4 bg-[radial-gradient(ellipse_at_top,_var(--tw-gradient-stops))] from-gray-900 via-[#0B0F19] to-black">
    <div class="w-full max-w-md">
      <!-- Glow effect -->
      <div class="relative">
        <div class="absolute -top-12 left-1/2 -translate-x-1/2 w-48 h-48 bg-brand-500/20 rounded-full blur-3xl pointer-events-none"></div>

        <div class="glass-panel p-8 rounded-3xl relative z-10 shadow-2xl border border-gray-800/80">
          <div class="text-center mb-8">
            <div class="w-14 h-14 mx-auto mb-4 rounded-2xl bg-gradient-to-tr from-brand-600 to-cyan-400 flex items-center justify-center shadow-lg shadow-brand-500/30">
              <Radio class="w-8 h-8 text-white" />
            </div>
            <h1 class="text-2xl font-extrabold text-white tracking-tight">Xray Decoupled Panel</h1>
            <p class="text-sm text-gray-400 mt-1">独立运维监控与订阅分发系统</p>
          </div>

          <form @submit.prevent="handleLogin" class="space-y-4">
            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">管理员账号</label>
              <div class="relative">
                <input
                  v-model="username"
                  type="text"
                  required
                  placeholder="admin"
                  class="w-full bg-gray-900/80 border border-gray-700/60 rounded-xl px-4 py-2.5 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-brand-500 focus:ring-1 focus:ring-brand-500 transition-colors"
                />
              </div>
            </div>

            <div>
              <label class="block text-xs font-semibold uppercase tracking-wider text-gray-400 mb-1.5">登录密码</label>
              <div class="relative">
                <input
                  v-model="password"
                  type="password"
                  required
                  placeholder="••••••••"
                  class="w-full bg-gray-900/80 border border-gray-700/60 rounded-xl px-4 py-2.5 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-brand-500 focus:ring-1 focus:ring-brand-500 transition-colors"
                />
              </div>
            </div>

            <div v-if="require2FA">
              <label class="block text-xs font-semibold uppercase tracking-wider text-cyan-400 mb-1.5">2FA 动态验证码</label>
              <input
                v-model="passcode"
                type="text"
                maxlength="6"
                placeholder="6 位 TOTP 动态码"
                class="w-full bg-gray-900/80 border border-cyan-500/60 rounded-xl px-4 py-2.5 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-cyan-400 focus:ring-1 focus:ring-cyan-400 transition-colors font-mono tracking-widest text-center"
              />
            </div>

            <div v-if="errorMsg" class="p-3 rounded-xl bg-red-500/10 border border-red-500/20 text-red-400 text-xs flex items-center gap-2">
              <AlertCircle class="w-4 h-4 shrink-0" />
              <span>{{ errorMsg }}</span>
            </div>

            <button
              type="submit"
              :disabled="loading"
              class="w-full mt-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-semibold py-2.5 rounded-xl text-sm transition-all shadow-lg shadow-brand-500/25 disabled:opacity-50 flex items-center justify-center gap-2"
            >
              <span v-if="loading" class="animate-spin w-4 h-4 border-2 border-white border-t-transparent rounded-full"></span>
              <span>{{ loading ? '登录中...' : '立即登录' }}</span>
            </button>
          </form>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { Radio, AlertCircle } from 'lucide-vue-next'
import api from '../api'

const router = useRouter()
const username = ref('admin')
const password = ref('')
const passcode = ref('')
const require2FA = ref(false)
const loading = ref(false)
const errorMsg = ref('')

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
    if (errorMsg.value.includes('2fa')) {
      require2FA.value = true
    }
  } finally {
    loading.value = false
  }
}
</script>
