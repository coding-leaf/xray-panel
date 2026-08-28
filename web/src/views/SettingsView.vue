<template>
  <div class="space-y-6 max-w-4xl">
    <div>
      <div class="flex items-center gap-2.5">
        <h1 class="text-2xl font-extrabold text-white tracking-tight">系统设置与核心解耦配置</h1>
        <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 flex items-center gap-1">
          <span>🟢 零硬编码 / 官方标准托管</span>
        </span>
      </div>
      <p class="text-xs text-gray-400 mt-0.5">配置 Xray Systemd 服务路径、订阅公共域名、Telegram 告警与管理员安全凭证</p>
    </div>

    <!-- 1. Xray 核心运行环境与 Systemd 托管设置 (零硬编码) -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-base font-bold text-white flex items-center gap-2">
          <Cpu class="w-4 h-4 text-brand-400" />
          <span>① Xray 核心解耦与 Systemd 托管环境</span>
        </h2>
        <span class="text-[11px] text-gray-500 font-mono">遵循 Linux 官方标准默认规范</span>
      </div>
      <p class="text-xs text-gray-400">
        面板与 Xray 核心完全解耦。在生产环境下面板通过 systemctl 管理 Xray 守护进程，在此可动态修改核心路径与服务名。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
        <div>
          <label class="block text-gray-300 font-semibold mb-1">Systemd 服务名称 (Service Name)</label>
          <input
            v-model="settings.xray_service_name"
            type="text"
            placeholder="xray"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">例如: <code>xray</code> 或 <code>xray.service</code></p>
        </div>

        <div>
          <label class="block text-gray-300 font-semibold mb-1">Xray gRPC API 监听地址</label>
          <input
            v-model="settings.xray_grpc_addr"
            type="text"
            placeholder="127.0.0.1:8080"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">需与 config.json 中 api.listen 保持一致</p>
        </div>

        <div>
          <label class="block text-gray-300 font-semibold mb-1">Xray 配置文件路径 (Config Path)</label>
          <input
            v-model="settings.xray_config_path"
            type="text"
            placeholder="/usr/local/etc/xray/config.json"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">官方路径: <code>/usr/local/etc/xray/config.json</code></p>
        </div>

        <div>
          <label class="block text-gray-300 font-semibold mb-1">Xray 二进制可执行文件 (Binary Path)</label>
          <input
            v-model="settings.xray_bin_path"
            type="text"
            placeholder="/usr/local/bin/xray"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">官方路径: <code>/usr/local/bin/xray</code></p>
        </div>

        <div class="sm:col-span-2">
          <label class="block text-gray-300 font-semibold mb-1">GeoData 规则库存储目录</label>
          <input
            v-model="settings.xray_geodata_dir"
            type="text"
            placeholder="/usr/local/share/xray"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">官方路径: <code>/usr/local/share/xray</code>（存放 geoip.dat 与 geosite.dat）</p>
        </div>
      </div>
    </div>

    <!-- 2. 面板公共访问与订阅分发设置 -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-base font-bold text-white flex items-center gap-2">
        <Globe class="w-4 h-4 text-brand-400" />
        <span>② 面板公网访问与订阅地址配置</span>
      </h2>
      <p class="text-xs text-gray-400">
        指定用户获取订阅链接时使用的基础域名以及节点分享链接中的默认服务器外网连接地址。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4 text-xs">
        <div>
          <label class="block text-gray-300 font-semibold mb-1">面板公网访问 URL (Public URL)</label>
          <input
            v-model="settings.public_url"
            type="text"
            placeholder="https://panel.yourdomain.com"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">用于生成聚合订阅 URL 前缀</p>
        </div>

        <div>
          <label class="block text-gray-300 font-semibold mb-1">节点连接地址 / 默认外网 IP</label>
          <input
            v-model="settings.sub_domain"
            type="text"
            placeholder="node1.yourdomain.com 或 服务器外网IP"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">若节点监听 0.0.0.0 时以此地址作为分享地址</p>
        </div>

        <div class="sm:col-span-2">
          <label class="block text-gray-300 font-semibold mb-1">全局默认外部公网连接端口 (Public Port)</label>
          <input
            v-model.number="settings.public_port"
            type="number"
            placeholder="443"
            class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
          />
          <p class="text-[10px] text-gray-500 mt-1">Nginx 前置 443 分流反代时，订阅链接默认下发的外部连接端口（默认 443）</p>
        </div>
      </div>
    </div>

    <!-- 3. Telegram 机器人运维与告警设置 -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <div class="flex items-center justify-between">
        <h2 class="text-base font-bold text-white flex items-center gap-2">
          <Send class="w-4 h-4 text-cyan-400" />
          <span>③ Telegram 机器人运维与告警配置</span>
        </h2>
        <button
          @click="testTelegram"
          :disabled="testingTG"
          class="px-3 py-1.5 bg-cyan-600/20 hover:bg-cyan-600/30 text-cyan-400 rounded-xl text-xs font-semibold border border-cyan-500/30 transition-colors flex items-center gap-1 disabled:opacity-50"
        >
          <Send class="w-3 h-3" />
          <span>{{ testingTG ? '发送中...' : '📨 发送测试通知' }}</span>
        </button>
      </div>

      <p class="text-xs text-gray-400">
        配置 Telegram Bot Token 与管理员 Chat ID，实时接收节点异常、流量超额、系统高负载告警，并支持在 TG 中使用 <code>/status</code> 等指令。
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

    <!-- 保存按钮 -->
    <div class="flex justify-end pt-2">
      <button
        @click="saveSystemSettings"
        :disabled="saving"
        class="px-6 py-3 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-bold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-2 disabled:opacity-50"
      >
        <Save class="w-4 h-4" />
        <span>{{ saving ? '保存中...' : '保存全部系统与解耦设置' }}</span>
      </button>
    </div>

    <!-- 4. 管理员密码修改 -->
    <div class="glass-panel p-6 rounded-2xl border border-gray-800/80 space-y-4">
      <h2 class="text-base font-bold text-white flex items-center gap-2">
        <Lock class="w-4 h-4 text-purple-400" />
        <span>④ 管理员密码安全修改</span>
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
          :disabled="changingPwd"
          class="px-4 py-2 bg-purple-600 hover:bg-purple-500 text-white rounded-xl font-semibold transition-colors disabled:opacity-50"
        >
          {{ changingPwd ? '更新中...' : '修改管理员密码' }}
        </button>
      </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Send, Globe, Lock, Cpu, Save } from 'lucide-vue-next'
import api from '../api'

const settings = ref<any>({
  xray_service_name: 'xray',
  xray_grpc_addr: '127.0.0.1:8080',
  xray_config_path: '/usr/local/etc/xray/config.json',
  xray_bin_path: '/usr/local/bin/xray',
  xray_geodata_dir: '/usr/local/share/xray',
  public_url: 'http://127.0.0.1:9000',
  sub_domain: '',
  tg_bot_token: '',
  tg_admin_chat_id: '',
})

const saving = ref(false)
const testingTG = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const changingPwd = ref(false)

const fetchSettings = async () => {
  try {
    const res: any = await api.get('/settings')
    if (res) {
      settings.value = {
        ...settings.value,
        ...res,
      }
    }
  } catch (err) {
    console.error(err)
  }
}

const saveSystemSettings = async () => {
  saving.value = true
  try {
    await api.post('/settings', settings.value)
    alert('系统与解耦设置已成功保存并即时生效！')
    await fetchSettings()
  } catch (err: any) {
    alert('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const testTelegram = async () => {
  if (!settings.value.tg_bot_token || !settings.value.tg_admin_chat_id) {
    alert('请先填写 Bot Token 与 Admin Chat ID 并保存！')
    return
  }
  testingTG.value = true
  try {
    await api.post('/settings/test-telegram')
    alert('测试消息已成功发送至 Telegram！请在客户端查看。')
  } catch (err: any) {
    alert('发送失败: ' + err)
  } finally {
    testingTG.value = false
  }
}

const changePassword = async () => {
  if (newPassword.value.length < 6) {
    alert('新密码长度不能少于 6 位')
    return
  }
  changingPwd.value = true
  try {
    await api.post('/auth/change-password', {
      oldPassword: oldPassword.value,
      newPassword: newPassword.value,
    })
    alert('管理员密码修改成功，请牢记新密码！')
    oldPassword.value = ''
    newPassword.value = ''
  } catch (err: any) {
    alert('修改失败: ' + err)
  } finally {
    changingPwd.value = false
  }
}

onMounted(() => {
  fetchSettings()
})
</script>
