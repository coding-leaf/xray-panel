<template>
  <div class="space-y-6 max-w-4xl">
    <div>
      <h1 class="text-2xl font-extrabold text-white tracking-tight">系统与通知设置</h1>
      <p class="text-xs text-gray-400 mt-0.5">配置 Telegram 运维告警机器人、订阅分发域名与管理员安全密钥</p>
    </div>

    <!-- Telegram Bot Settings -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-base font-bold text-white flex items-center gap-2">
        <Send class="w-4 h-4 text-cyan-400" />
        <span>Telegram 机器人运维与告警配置</span>
      </h2>
      <p class="text-xs text-gray-400">
        配置 Telegram Bot Token 与管理员 Chat ID，可接收节点异常、流量超额、系统高负载告警，并支持在 Telegram 中使用 <code>/status</code>、<code>/traffic</code>、<code>/sub</code> 等交互指令。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
        <div>
          <label class="block text-gray-300 font-semibold mb-1">Bot Token (从 @BotFather 获取)</label>
          <input
            v-model="settings.tg_bot_token"
            type="text"
            placeholder="123456789:ABCdefGhIJKlmNoPQRstuVWXyz"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
        </div>

        <div>
          <label class="block text-gray-300 font-semibold mb-1">管理员 Chat ID (从 @userinfobot 获取)</label>
          <input
            v-model="settings.tg_admin_chat_id"
            type="text"
            placeholder="12345678"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
        </div>
      </div>
    </div>

    <!-- Subscription Domain Settings -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-base font-bold text-white flex items-center gap-2">
        <Globe class="w-4 h-4 text-brand-400" />
        <span>订阅与节点连接域名</span>
      </h2>
      <p class="text-xs text-gray-400">
        指定生成 <code>vless://</code> 等分享链接及订阅中的默认服务器节点地址或域名（若为空则默认使用入站监听地址）。
      </p>

      <div class="text-xs">
        <label class="block text-gray-300 font-semibold mb-1">节点连接地址 / 域名</label>
        <input
          v-model="settings.sub_domain"
          type="text"
          placeholder="node1.yourdomain.com 或 服务器外网IP"
          class="w-full max-w-md bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
        />
      </div>

      <div class="pt-2">
        <button
          @click="saveSystemSettings"
          :disabled="saving"
          class="px-5 py-2.5 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 disabled:opacity-50"
        >
          <span>{{ saving ? '保存中...' : '保存系统设置' }}</span>
        </button>
      </div>
    </div>

    <!-- Admin Password Settings -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-base font-bold text-white flex items-center gap-2">
        <Lock class="w-4 h-4 text-purple-400" />
        <span>管理员密码修改</span>
      </h2>

      <form @submit.prevent="changePassword" class="space-y-3 max-w-md text-xs">
        <div>
          <label class="block text-gray-300 mb-1">当前旧密码</label>
          <input
            v-model="oldPassword"
            type="password"
            required
            placeholder="••••••••"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
          />
        </div>

        <div>
          <label class="block text-gray-300 mb-1">新密码</label>
          <input
            v-model="newPassword"
            type="password"
            required
            placeholder="••••••••"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
          />
        </div>

        <button
          type="submit"
          :disabled="pwdLoading"
          class="px-5 py-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-white font-medium transition-colors border border-gray-700"
        >
          <span>{{ pwdLoading ? '更新中...' : '更新密码' }}</span>
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Send, Globe, Lock } from 'lucide-vue-next'
import api from '../api'

const settings = ref<any>({
  tg_bot_token: '',
  tg_admin_chat_id: '',
  sub_domain: '',
})
const saving = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const pwdLoading = ref(false)

const fetchSettings = async () => {
  try {
    const res: any = await api.get('/settings')
    settings.value = { ...settings.value, ...res }
  } catch (err) {
    console.error(err)
  }
}

const saveSystemSettings = async () => {
  saving.value = true
  try {
    await api.post('/settings', settings.value)
    alert('系统设置已成功保存！')
  } catch (err: any) {
    alert('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const changePassword = async () => {
  pwdLoading.value = true
  try {
    await api.post('/auth/password', {
      oldPassword: oldPassword.value,
      newPassword: newPassword.value,
    })
    alert('密码修改成功，请重新登录！')
    localStorage.removeItem('token')
    window.location.href = '/login'
  } catch (err: any) {
    alert('修改失败: ' + err)
  } finally {
    pwdLoading.value = false
  }
}

onMounted(() => {
  fetchSettings()
})
</script>
