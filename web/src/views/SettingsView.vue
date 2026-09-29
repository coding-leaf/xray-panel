<template>
  <div class="space-y-4 max-w-4xl">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">系统设置与核心解耦配置</h1>
          <Badge variant="outline" class="text-[10px] font-mono">
            零硬编码 / 官方标准托管
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">配置 Xray Systemd 服务路径、订阅公共域名、Telegram 告警与管理员安全凭证</p>
      </div>

      <Button
        variant="default"
        size="sm"
        :loading="saving"
        @click="saveSystemSettings"
      >
        <Save class="w-3.5 h-3.5 mr-1" />
        <span>{{ saving ? '保存中...' : '保存全部设置' }}</span>
      </Button>
    </div>

    <!-- 1. Xray 核心运行环境与 Systemd 托管设置 (零硬编码) -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
          <Cpu class="w-3.5 h-3.5 text-brand-400" />
          <span>① Xray 核心解耦与 Systemd 托管环境</span>
        </h2>
        <span class="text-[11px] text-muted-foreground font-mono">遵循 Linux 官方标准默认规范</span>
      </div>
      <p class="text-xs text-muted-foreground">
        面板与 Xray 核心完全解耦。在生产环境下面板通过 systemctl 管理 Xray 守护进程，在此可动态修改核心路径与服务名。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground font-medium mb-1">Systemd 服务名称 (Service Name)</label>
          <input
            v-model="settings.xray_service_name"
            type="text"
            placeholder="xray"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">例如: <code>xray</code> 或 <code>xray.service</code></p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">Xray gRPC API 监听地址</label>
          <input
            v-model="settings.xray_grpc_addr"
            type="text"
            placeholder="127.0.0.1:8080"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">需与 config.json 中 api.listen 保持一致</p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">Xray 配置文件路径 (Config Path)</label>
          <input
            v-model="settings.xray_config_path"
            type="text"
            placeholder="/usr/local/etc/xray/config.json"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">官方路径: <code>/usr/local/etc/xray/config.json</code></p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">Xray 二进制可执行文件 (Binary Path)</label>
          <input
            v-model="settings.xray_bin_path"
            type="text"
            placeholder="/usr/local/bin/xray"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">官方路径: <code>/usr/local/bin/xray</code></p>
        </div>

        <div class="sm:col-span-2">
          <label class="block text-muted-foreground font-medium mb-1">GeoData 规则库存储目录</label>
          <input
            v-model="settings.xray_geodata_dir"
            type="text"
            placeholder="/usr/local/share/xray"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">官方路径: <code>/usr/local/share/xray</code>（存放 geoip.dat 与 geosite.dat）</p>
        </div>
      </div>
    </div>

    <!-- 2. 面板公共访问与订阅分发设置 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
        <Globe class="w-3.5 h-3.5 text-cyan-400" />
        <span>② 面板公网访问与订阅地址配置</span>
      </h2>
      <p class="text-xs text-muted-foreground">
        指定用户获取订阅链接时使用的基础域名以及节点分享链接中的默认服务器外网连接地址。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground font-medium mb-1">面板公网访问 URL (Public URL)</label>
          <input
            v-model="settings.public_url"
            type="text"
            placeholder="http://IP:9000 或 https://panel.yourdomain.com"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">用于管理面板外网访问及默认聚合订阅 URL 前缀</p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">中立提取门户 URL (Portal URL / Worker 代理)</label>
          <input
            v-model="settings.portal_url"
            type="text"
            placeholder="https://sub.yourworker.workers.dev"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">安全提取码专属分发入口（推荐填写 Cloudflare Worker 地址以防封锁；留空自动跟随公网地址）</p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">节点连接地址 / 默认外网 IP</label>
          <input
            v-model="settings.sub_domain"
            type="text"
            placeholder="node1.yourdomain.com 或 服务器外网IP"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">若节点监听 0.0.0.0 时以此地址作为分享地址</p>
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">全局默认外部公网连接端口 (Public Port)</label>
          <input
            v-model.number="settings.public_port"
            type="number"
            placeholder="443"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-1">Nginx 前置 443 分流反代时，订阅链接默认下发的外部连接端口（默认 443）</p>
        </div>
      </div>
    </div>

    <!-- 3. Telegram 机器人运维与告警设置 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
          <Send class="w-3.5 h-3.5 text-indigo-400" />
          <span>③ Telegram 机器人运维与告警配置</span>
        </h2>
        <Button
          variant="secondary"
          size="sm"
          :loading="testingTG"
          @click="testTelegram"
        >
          <Send class="w-3 h-3 mr-1" />
          <span>{{ testingTG ? '发送中...' : '发送测试通知' }}</span>
        </Button>
      </div>

      <p class="text-xs text-muted-foreground">
        配置 Telegram Bot Token 与管理员 Chat ID，实时接收节点异常、流量超额、系统高负载告警，并支持在 TG 中使用 <code>/status</code> 等指令。
      </p>

      <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs">
        <div>
          <label class="block text-muted-foreground font-medium mb-1">Bot Token (从 @BotFather 获取)</label>
          <input
            v-model="settings.tg_bot_token"
            type="text"
            placeholder="123456789:ABCdefGhIJKlmNoPQRstuVWXyz"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div>
          <label class="block text-muted-foreground font-medium mb-1">管理员 Chat ID (从 @userinfobot 获取)</label>
          <input
            v-model="settings.tg_admin_chat_id"
            type="text"
            placeholder="12345678"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>
      </div>
    </div>

    <!-- 4. 管理员密码修改 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
        <Lock class="w-3.5 h-3.5 text-purple-400" />
        <span>④ 管理员密码安全修改</span>
      </h2>

      <form @submit.prevent="changePassword" class="space-y-3 max-w-md text-xs">
        <div>
          <label class="block text-muted-foreground mb-1 font-medium">当前旧密码</label>
          <input
            v-model="oldPassword"
            type="password"
            required
            placeholder="••••••••"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div>
          <label class="block text-muted-foreground mb-1 font-medium">新密码</label>
          <input
            v-model="newPassword"
            type="password"
            required
            placeholder="••••••••"
            class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <Button
          type="submit"
          variant="secondary"
          size="sm"
          :loading="changingPwd"
        >
          {{ changingPwd ? '更新中...' : '修改管理员密码' }}
        </Button>
      </form>
    </div>

    <!-- 5. TOTP 双因素认证 (2FA) 安全加固 -->
    <div class="p-4 rounded-lg bg-card border border-border space-y-3">
      <div class="flex items-center justify-between">
        <h2 class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono flex items-center gap-1.5">
          <ShieldCheck class="w-3.5 h-3.5 text-emerald-400" />
          <span>⑤ TOTP 双因素身份认证 (2FA)</span>
        </h2>
        <Badge
          :variant="totpEnabled ? 'default' : 'secondary'"
          class="text-[10px] font-mono"
        >
          {{ totpEnabled ? '已启用 2FA 保护' : '未启用 2FA' }}
        </Badge>
      </div>

      <p class="text-xs text-muted-foreground">
        启用双因素认证后，每次登录除了需要输入密码外，还必须输入 Google Authenticator 或 1Password 等应用生成的 6 位动态验证码，有效防范凭据泄露风险。
      </p>

      <div class="pt-1">
        <Button
          v-if="!totpEnabled"
          variant="default"
          size="sm"
          :loading="loading2FA"
          @click="startSetup2FA"
        >
          <QrCode class="w-3.5 h-3.5 mr-1" />
          <span>{{ loading2FA ? '加载中...' : '扫码绑定并开启 2FA' }}</span>
        </Button>

        <Button
          v-else
          variant="destructive"
          size="sm"
          @click="showDisableModal = true"
        >
          <ShieldAlert class="w-3.5 h-3.5 mr-1" />
          <span>关闭 2FA 双因素保护</span>
        </Button>
      </div>
    </div>

    <!-- 2FA 绑定弹窗 -->
    <div v-if="showSetupModal" class="fixed inset-0 bg-black/75 z-50 flex items-center justify-center p-4">
      <div class="bg-card border border-border rounded-lg p-5 max-w-sm w-full space-y-4 shadow-2xl">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <h3 class="text-sm font-bold text-foreground flex items-center gap-1.5">
            <QrCode class="w-4 h-4 text-emerald-400" />
            <span>绑定 Google 验证码 (2FA)</span>
          </h3>
          <button @click="showSetupModal = false" class="text-muted-foreground hover:text-foreground text-xs">✕</button>
        </div>

        <p class="text-[11px] text-muted-foreground">
          请使用手机 Authenticator 扫描下方二维码，或手动输入 Secret 密钥：
        </p>

        <!-- QR Code -->
        <div class="bg-white p-3 rounded-md w-fit mx-auto shadow-inner">
          <qrcode-vue :value="totpSetupData.otpauthUrl" :size="160" level="M" />
        </div>

        <div class="bg-neutral-950 border border-border rounded-md p-2 text-center">
          <span class="text-[10px] uppercase text-muted-foreground block mb-0.5 font-mono">Base32 Secret 密钥</span>
          <code class="text-xs font-mono font-bold text-emerald-400 select-all">{{ totpSetupData.secret }}</code>
        </div>

        <div class="space-y-1.5">
          <label class="block text-xs text-foreground font-medium">输入 App 生成的 6 位动态码确认：</label>
          <input
            v-model="setupPasscode"
            type="text"
            maxlength="6"
            placeholder="例如: 123456"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-center text-sm font-mono text-foreground tracking-widest focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <div class="flex justify-end gap-2 pt-2 border-t border-border">
          <Button variant="secondary" size="sm" @click="showSetupModal = false">取消</Button>
          <Button
            variant="default"
            size="sm"
            :loading="enabling2FA"
            :disabled="setupPasscode.length !== 6"
            @click="confirmEnable2FA"
          >
            {{ enabling2FA ? '验证中...' : '确认开启 2FA' }}
          </Button>
        </div>
      </div>
    </div>

    <!-- 2FA 关闭确认弹窗 -->
    <div v-if="showDisableModal" class="fixed inset-0 bg-black/75 z-50 flex items-center justify-center p-4">
      <div class="bg-card border border-border rounded-lg p-5 max-w-sm w-full space-y-4 shadow-2xl">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <h3 class="text-sm font-bold text-foreground flex items-center gap-1.5">
            <ShieldAlert class="w-4 h-4 text-rose-400" />
            <span>关闭 2FA 保护</span>
          </h3>
          <button @click="showDisableModal = false" class="text-muted-foreground hover:text-foreground text-xs">✕</button>
        </div>

        <p class="text-xs text-muted-foreground">关闭 2FA 后登录将只需密码，请输入管理员密码以确认操作：</p>

        <div class="space-y-3 text-xs">
          <div>
            <label class="block text-muted-foreground mb-1 font-medium">管理员密码</label>
            <input
              v-model="disablePassword"
              type="password"
              placeholder="••••••••"
              class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
          <div>
            <label class="block text-muted-foreground mb-1 font-medium">当前 6 位动态码 (必填)</label>
            <input
              v-model="disablePasscode"
              type="text"
              maxlength="6"
              placeholder="6 位动态码"
              class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-foreground font-mono text-center tracking-widest focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>

        <div class="flex justify-end gap-2 pt-2 border-t border-border">
          <Button variant="secondary" size="sm" @click="showDisableModal = false">取消</Button>
          <Button
            variant="destructive"
            size="sm"
            :loading="disabling2FA"
            :disabled="!disablePassword || disablePasscode.length !== 6"
            @click="confirmDisable2FA"
          >
            {{ disabling2FA ? '关闭中...' : '确认关闭' }}
          </Button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Send, Globe, Lock, Cpu, Save, ShieldCheck, ShieldAlert, QrCode } from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import { toast } from '../utils/toast'
import api from '../api'

const settings = ref<any>({
  xray_service_name: 'xray',
  xray_grpc_addr: '127.0.0.1:8080',
  xray_config_path: '/usr/local/etc/xray/config.json',
  xray_bin_path: '/usr/local/bin/xray',
  xray_geodata_dir: '/usr/local/share/xray',
  public_url: '',
  portal_url: '',
  sub_domain: '',
  public_port: 443,
  tg_bot_token: '',
  tg_admin_chat_id: '',
})

const saving = ref(false)
const testingTG = ref(false)
const oldPassword = ref('')
const newPassword = ref('')
const changingPwd = ref(false)

// 2FA 状态
const totpEnabled = ref(false)
const showSetupModal = ref(false)
const showDisableModal = ref(false)
const loading2FA = ref(false)
const enabling2FA = ref(false)
const disabling2FA = ref(false)
const totpSetupData = ref({ secret: '', otpauthUrl: '' })
const setupPasscode = ref('')
const disablePassword = ref('')
const disablePasscode = ref('')

const fetchAdminInfo = async () => {
  try {
    const res: any = await api.get('/auth/info')
    if (res) {
      totpEnabled.value = !!res.totpEnabled
    }
  } catch (err) {
    console.error(err)
  }
}

const fetchSettings = async () => {
  try {
    const res: any = await api.get('/settings')
    if (res) {
      settings.value = {
        ...settings.value,
        ...res,
      }
    }
    // 若未配置 public_url 或仍为默认 127.0.0.1，自动根据当前浏览器地址推荐
    if (!settings.value.public_url || settings.value.public_url.includes('127.0.0.1')) {
      settings.value.public_url = `${window.location.protocol}//${window.location.host}`
    }
  } catch (err) {
    console.error(err)
  }
}

const saveSystemSettings = async () => {
  saving.value = true
  try {
    await api.post('/settings', settings.value)
    toast.success('系统与解耦设置已成功保存并即时生效！')
    await fetchSettings()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const testTelegram = async () => {
  if (!settings.value.tg_bot_token || !settings.value.tg_admin_chat_id) {
    toast.warning('请先填写 Bot Token 与 Admin Chat ID 并保存！')
    return
  }
  testingTG.value = true
  try {
    await api.post('/settings/test-telegram')
    toast.success('测试消息已成功发送至 Telegram！请在客户端查看。')
  } catch (err: any) {
    toast.error('发送失败: ' + err)
  } finally {
    testingTG.value = false
  }
}

const changePassword = async () => {
  if (newPassword.value.length < 6) {
    toast.warning('新密码长度不能少于 6 位')
    return
  }
  changingPwd.value = true
  try {
    await api.post('/auth/change-password', {
      oldPassword: oldPassword.value,
      newPassword: newPassword.value,
    })
    toast.success('管理员密码修改成功，请牢记新密码！')
    oldPassword.value = ''
    newPassword.value = ''
  } catch (err: any) {
    toast.error('修改失败: ' + err)
  } finally {
    changingPwd.value = false
  }
}

const startSetup2FA = async () => {
  loading2FA.value = true
  try {
    const res: any = await api.get('/auth/2fa/setup')
    totpSetupData.value = {
      secret: res.secret,
      otpauthUrl: res.otpauthUrl,
    }
    setupPasscode.value = ''
    showSetupModal.value = true
  } catch (err: any) {
    toast.error('获取 2FA 密钥失败: ' + err)
  } finally {
    loading2FA.value = false
  }
}

const confirmEnable2FA = async () => {
  if (setupPasscode.value.length !== 6) {
    toast.warning('请输入完整的 6 位动态验证码')
    return
  }
  enabling2FA.value = true
  try {
    const res: any = await api.post('/auth/2fa/enable', {
      secret: totpSetupData.value.secret,
      passcode: setupPasscode.value,
    })
    toast.success(res.message || '2FA 双因素认证已成功开启！')
    totpEnabled.value = true
    showSetupModal.value = false
  } catch (err: any) {
    toast.error('开启失败: ' + (typeof err === 'string' ? err : '验证码不正确'))
  } finally {
    enabling2FA.value = false
  }
}

const confirmDisable2FA = async () => {
  if (!disablePassword.value) {
    toast.warning('请输入密码确认')
    return
  }
  if (!disablePasscode.value || disablePasscode.value.length !== 6) {
    toast.warning('请输入 6 位 2FA 动态码')
    return
  }
  disabling2FA.value = true
  try {
    const res: any = await api.post('/auth/2fa/disable', {
      password: disablePassword.value,
      passcode: disablePasscode.value,
    })
    toast.success(res.message || '2FA 已关闭')
    totpEnabled.value = false
    showDisableModal.value = false
    disablePassword.value = ''
    disablePasscode.value = ''
  } catch (err: any) {
    toast.error('关闭失败: ' + (typeof err === 'string' ? err : '密码或验证码错误'))
  } finally {
    disabling2FA.value = false
  }
}

onMounted(() => {
  fetchSettings()
  fetchAdminInfo()
})
</script>
