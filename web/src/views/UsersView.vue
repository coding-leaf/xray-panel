<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">用户与订阅管理</h1>
        <p class="text-xs text-gray-400 mt-0.5">动态热添加/删除 Xray 节点用户、独立 Token 订阅分发与流量限额管理</p>
      </div>
      <button
        @click="openAddModal"
        class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
      >
        <UserPlus class="w-4 h-4" />
        <span>添加用户</span>
      </button>
    </div>

    <!-- Users Table -->
    <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="text-gray-400 bg-gray-900/60 border-b border-gray-800">
            <tr>
              <th class="py-3.5 px-4 font-semibold">用户邮箱</th>
              <th class="py-3.5 px-4 font-semibold">绑定节点</th>
              <th class="py-3.5 px-4 font-semibold">流量使用进度</th>
              <th class="py-3.5 px-4 font-semibold">到期时间</th>
              <th class="py-3.5 px-4 font-semibold">状态</th>
              <th class="py-3.5 px-4 font-semibold text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-800/60">
            <tr v-for="u in users" :key="u.id" class="hover:bg-gray-800/30 transition-colors">
              <!-- Email & UUID -->
              <td class="py-3.5 px-4">
                <div class="font-bold text-white flex items-center gap-1.5">
                  <span>{{ u.email }}</span>
                </div>
                <div class="text-[11px] font-mono text-gray-500 truncate max-w-[200px]" :title="u.uuid">
                  {{ u.uuid }}
                </div>
              </td>

              <!-- Inbound Tag -->
              <td class="py-3.5 px-4">
                <span class="px-2.5 py-1 rounded-lg bg-gray-800 border border-gray-700 text-gray-300 font-mono text-[11px]">
                  {{ u.inboundTag }}
                </span>
              </td>

              <!-- Traffic Progress Bar -->
              <td class="py-3.5 px-4 min-w-[180px]">
                <div class="flex justify-between text-[11px] mb-1 font-mono">
                  <span class="text-gray-300">{{ formatBytes(u.upBytes + u.downBytes) }}</span>
                  <span class="text-gray-500">{{ u.totalBytes > 0 ? formatBytes(u.totalBytes) : '无限制' }}</span>
                </div>
                <div class="w-full bg-gray-800 rounded-full h-1.5 overflow-hidden">
                  <div
                    class="h-1.5 rounded-full transition-all duration-300"
                    :class="trafficBarColor(u)"
                    :style="{ width: `${trafficPercent(u)}%` }"
                  ></div>
                </div>
              </td>

              <!-- Expire Time -->
              <td class="py-3.5 px-4 font-mono text-gray-400">
                {{ formatExpiry(u.expireTime) }}
              </td>

              <!-- Status Switch/Badge -->
              <td class="py-3.5 px-4">
                <span
                  class="inline-flex items-center gap-1 px-2.5 py-0.5 rounded-full text-[10px] font-semibold"
                  :class="u.enabled && !isExpired(u.expireTime) ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-red-500/10 text-red-400 border border-red-500/20'"
                >
                  <span class="w-1.5 h-1.5 rounded-full" :class="u.enabled && !isExpired(u.expireTime) ? 'bg-emerald-400' : 'bg-red-400'"></span>
                  <span>{{ u.enabled && !isExpired(u.expireTime) ? '正常' : '已禁用/到期' }}</span>
                </span>
              </td>

              <!-- Actions -->
              <td class="py-3.5 px-4 text-right space-x-2">
                <button
                  @click="openShareModal(u)"
                  class="px-2.5 py-1 rounded-lg bg-brand-500/10 hover:bg-brand-500/20 text-brand-400 transition-colors"
                  title="获取订阅/节点链接与二维码"
                >
                  <QrCode class="w-3.5 h-3.5 inline" />
                </button>
                <button
                  @click="resetTraffic(u.id)"
                  class="px-2.5 py-1 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 transition-colors"
                  title="重置已用流量"
                >
                  <RotateCcw class="w-3.5 h-3.5 inline" />
                </button>
                <button
                  @click="deleteUser(u.id)"
                  class="px-2.5 py-1 rounded-lg bg-red-500/10 hover:bg-red-500/20 text-red-400 transition-colors"
                  title="删除用户"
                >
                  <Trash2 class="w-3.5 h-3.5 inline" />
                </button>
              </td>
            </tr>
            <tr v-if="!users.length">
              <td colspan="6" class="py-12 text-center text-gray-500">
                暂无用户数据，点击上方“添加用户”新增
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add User Modal -->
    <div v-if="showAddModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-md p-6 rounded-3xl border border-gray-800 shadow-2xl space-y-4">
        <h2 class="text-lg font-bold text-white">添加新用户</h2>

        <form @submit.prevent="createUser" class="space-y-3 text-xs">
          <div>
            <label class="block text-gray-400 mb-1">用户邮箱标识</label>
            <input
              v-model="addForm.email"
              type="email"
              required
              placeholder="user@example.com"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
            />
          </div>

          <div>
            <label class="block text-gray-400 mb-1">绑定入站节点</label>
            <select
              v-model="addForm.inboundTag"
              required
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
            >
              <option v-for="inb in inbounds" :key="inb.id" :value="inb.tag">
                {{ inb.tag }} ({{ inb.protocol }}:{{ inb.port }})
              </option>
            </select>
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-gray-400 mb-1">流量限额 (GB, 0为无限)</label>
              <input
                v-model.number="addForm.totalGB"
                type="number"
                min="0"
                placeholder="100"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
            <div>
              <label class="block text-gray-400 mb-1">有效期天数 (0为永久)</label>
              <input
                v-model.number="addForm.expireDays"
                type="number"
                min="0"
                placeholder="30"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <div class="flex justify-end gap-2 pt-2">
            <button
              type="button"
              @click="showAddModal = false"
              class="px-4 py-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs"
            >
              取消
            </button>
            <button
              type="submit"
              class="px-4 py-2 rounded-xl bg-brand-600 hover:bg-brand-500 text-white text-xs font-semibold"
            >
              创建
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Share & Subscription Modal -->
    <div v-if="showShareModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-lg p-6 rounded-3xl border border-gray-800 shadow-2xl space-y-4">
        <div class="flex items-center justify-between">
          <h2 class="text-base font-bold text-white">用户专属订阅与节点</h2>
          <button @click="showShareModal = false" class="text-gray-400 hover:text-white">✕</button>
        </div>

        <div class="flex justify-center p-4 bg-white rounded-2xl w-fit mx-auto shadow-inner">
          <qrcode-vue :value="subUrl" :size="160" level="H" />
        </div>

        <div class="space-y-3 text-xs">
          <div>
            <label class="block text-gray-400 mb-1 font-semibold">独立 Token 订阅分发链接</label>
            <div class="flex gap-2">
              <input
                type="text"
                readonly
                :value="subUrl"
                class="flex-1 bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-gray-200 font-mono text-[11px]"
              />
              <button
                @click="copyText(subUrl)"
                class="px-3 py-2 bg-brand-600 hover:bg-brand-500 text-white rounded-xl font-medium shrink-0"
              >
                复制
              </button>
            </div>
          </div>

          <div v-if="nodeShareLink">
            <label class="block text-gray-400 mb-1 font-semibold">直连节点分享链接 (Xray 标准格式)</label>
            <div class="flex gap-2">
              <input
                type="text"
                readonly
                :value="nodeShareLink"
                class="flex-1 bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-gray-200 font-mono text-[11px]"
              />
              <button
                @click="copyText(nodeShareLink)"
                class="px-3 py-2 bg-gray-800 hover:bg-gray-700 text-white rounded-xl font-medium shrink-0"
              >
                复制
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { UserPlus, QrCode, RotateCcw, Trash2 } from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import api from '../api'

const users = ref<any[]>([])
const inbounds = ref<any[]>([])
const showAddModal = ref(false)
const showShareModal = ref(false)
const currentUser = ref<any>(null)
const subUrl = ref('')
const nodeShareLink = ref('')

const addForm = ref({
  email: '',
  inboundTag: '',
  totalGB: 100,
  expireDays: 30,
})

const fetchUsers = async () => {
  try {
    users.value = await api.get('/users')
  } catch (err) {
    console.error(err)
  }
}

const fetchInbounds = async () => {
  try {
    inbounds.value = await api.get('/inbounds')
    if (inbounds.value.length > 0 && !addForm.value.inboundTag) {
      addForm.value.inboundTag = inbounds.value[0].tag
    }
  } catch (err) {
    console.error(err)
  }
}

const openAddModal = () => {
  addForm.value = {
    email: '',
    inboundTag: inbounds.value[0]?.tag || '',
    totalGB: 100,
    expireDays: 30,
  }
  showAddModal.value = true
}

const createUser = async () => {
  try {
    await api.post('/users', {
      email: addForm.value.email,
      inboundTag: addForm.value.inboundTag,
      totalBytes: addForm.value.totalGB * 1024 * 1024 * 1024,
      expireDays: addForm.value.expireDays,
    })
    showAddModal.value = false
    await fetchUsers()
  } catch (err: any) {
    alert('创建失败: ' + err)
  }
}

const deleteUser = async (id: number) => {
  if (!confirm('确定删除该用户吗？')) return
  try {
    await api.delete(`/users/${id}`)
    await fetchUsers()
  } catch (err: any) {
    alert('删除失败: ' + err)
  }
}

const resetTraffic = async (id: number) => {
  if (!confirm('确认重置该用户的已用流量吗？')) return
  try {
    await api.post(`/users/${id}/reset`)
    await fetchUsers()
  } catch (err: any) {
    alert('重置失败: ' + err)
  }
}

const openShareModal = async (u: any) => {
  currentUser.value = u
  subUrl.value = `${window.location.origin}/api/sub/${u.subToken}`
  try {
    const res: any = await api.get(`/users/${u.id}/share`)
    nodeShareLink.value = res.shareLink || ''
  } catch (err) {
    nodeShareLink.value = ''
  }
  showShareModal.value = true
}

const copyText = (text: string) => {
  navigator.clipboard.writeText(text)
  alert('已复制到剪贴板')
}

const trafficPercent = (u: any) => {
  if (!u.totalBytes || u.totalBytes <= 0) return 0
  const pct = ((u.upBytes + u.downBytes) / u.totalBytes) * 100
  return Math.min(Math.round(pct), 100)
}

const trafficBarColor = (u: any) => {
  const pct = trafficPercent(u)
  if (pct > 90) return 'bg-red-500'
  if (pct > 75) return 'bg-amber-500'
  return 'bg-brand-500'
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

const formatExpiry = (ms: number) => {
  if (!ms || ms <= 0) return '永久有效'
  return new Date(ms).toLocaleDateString()
}

const isExpired = (ms: number) => {
  if (!ms || ms <= 0) return false
  return Date.now() > ms
}

onMounted(() => {
  fetchUsers()
  fetchInbounds()
})
</script>
