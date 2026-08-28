<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-white tracking-tight">用户与多节点归属管理</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 flex items-center gap-1">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
            <span>gRPC 毫秒内存热生效 (零断网 / 免重启)</span>
          </span>
        </div>
        <p class="text-xs text-gray-400 mt-0.5">按用户聚合多节点归属，支持全局聚合订阅、单节点独立提取与多协议多端分发</p>
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
          <thead>
            <tr class="border-b border-gray-800/80 bg-gray-900/50 text-gray-400 font-semibold">
              <th class="py-3.5 px-4">用户名 / 邮箱</th>
              <th class="py-3.5 px-4">归属节点 (Inbound Tags)</th>
              <th class="py-3.5 px-4">流控模式 (Flow)</th>
              <th class="py-3.5 px-4">已用 / 总限额</th>
              <th class="py-3.5 px-4">到期时间</th>
              <th class="py-3.5 px-4">状态</th>
              <th class="py-3.5 px-4 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-800/40">
            <tr v-for="user in users" :key="user.id" class="hover:bg-white/[0.02] transition-colors">
              <td class="py-3.5 px-4 font-mono font-medium text-white">
                {{ user.email }}
              </td>

              <!-- 归属多节点展示 -->
              <td class="py-3.5 px-4">
                <div class="flex flex-wrap gap-1">
                  <span
                    v-for="tag in getNodeTags(user)"
                    :key="tag"
                    class="px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-brand-500/15 text-brand-300 border border-brand-500/20"
                  >
                    {{ tag }}
                  </span>
                </div>
              </td>

              <td class="py-3.5 px-4 font-mono text-gray-300">
                {{ user.flow || 'none' }}
              </td>

              <td class="py-3.5 px-4 font-mono text-gray-300">
                <div class="space-y-1">
                  <div>{{ formatBytes(user.upBytes + user.downBytes) }} / {{ user.totalBytes > 0 ? formatBytes(user.totalBytes) : '无限制' }}</div>
                  <div v-if="user.totalBytes > 0" class="w-24 h-1.5 bg-gray-800 rounded-full overflow-hidden">
                    <div
                      class="h-full rounded-full transition-all"
                      :class="getTrafficPercent(user) > 90 ? 'bg-rose-500' : 'bg-brand-500'"
                      :style="{ width: `${Math.min(100, getTrafficPercent(user))}%` }"
                    ></div>
                  </div>
                </div>
              </td>

              <td class="py-3.5 px-4 text-gray-400 font-mono text-[11px]">
                {{ user.expireTime > 0 ? formatDate(user.expireTime) : '永久有效' }}
              </td>

              <td class="py-3.5 px-4">
                <span
                  class="px-2 py-0.5 rounded-full text-[10px] font-semibold"
                  :class="user.enabled ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-gray-800 text-gray-400'"
                >
                  {{ user.enabled ? '正常' : '已禁用' }}
                </span>
              </td>

              <td class="py-3.5 px-4 text-right space-x-2">
                <button
                  @click="openShareModal(user)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-brand-600/20 text-brand-400 transition-colors"
                  title="获取聚合与单节点订阅"
                >
                  <Share2 class="w-3.5 h-3.5" />
                </button>
                <button
                  @click="resetTraffic(user.id)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 transition-colors"
                  title="重置流量"
                >
                  <RotateCcw class="w-3.5 h-3.5" />
                </button>
                <button
                  @click="openEditModal(user)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-gray-700 text-indigo-400 transition-colors"
                  title="编辑"
                >
                  <Edit class="w-3.5 h-3.5" />
                </button>
                <button
                  @click="deleteUser(user.id)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-rose-500/20 text-rose-400 transition-colors"
                  title="删除"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add/Edit User Modal (With Multi-Node Checkbox Assignment) -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-lg p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-5 my-8">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white">{{ isEditing ? '编辑用户权限与节点' : '添加新用户' }}</h2>
            <p class="text-xs text-gray-400 mt-0.5">选择该用户归属的节点，将自动同步至 Xray 内存与文件</p>
          </div>
          <button @click="showModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <form @submit.prevent="saveUser" class="space-y-4 text-xs">
          <div>
            <label class="block text-gray-300 mb-1 font-medium">用户名 / 邮箱 (Email)</label>
            <input
              v-model="form.email"
              type="text"
              required
              :disabled="isEditing"
              placeholder="user@example.com"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500 disabled:opacity-50"
            />
          </div>

          <!-- 授权所属节点（多选列表） -->
          <div>
            <div class="flex items-center justify-between mb-1.5">
              <label class="text-gray-300 font-medium">授权所属节点 (多选归属)</label>
              <button
                type="button"
                @click="toggleSelectAllInbounds"
                class="text-brand-400 hover:text-brand-300 text-[11px] font-semibold"
              >
                {{ isAllInboundsSelected ? '取消全选' : '全选所有节点' }}
              </button>
            </div>

            <div class="space-y-2 max-h-48 overflow-y-auto bg-gray-900/80 p-3 rounded-xl border border-gray-800">
              <label
                v-for="inb in availableInbounds"
                :key="inb.tag"
                class="flex items-center justify-between p-2 rounded-lg hover:bg-gray-800/60 cursor-pointer transition-colors"
              >
                <div class="flex items-center gap-2.5">
                  <input
                    type="checkbox"
                    :value="inb.tag"
                    v-model="form.selectedTags"
                    class="rounded bg-gray-800 border-gray-700 text-brand-600 focus:ring-0"
                  />
                  <span class="font-mono text-white text-xs font-semibold">{{ inb.tag }}</span>
                </div>
                <div class="flex items-center gap-1.5 font-mono text-[11px] text-gray-400">
                  <span class="uppercase">{{ inb.protocol }}</span>
                  <span>:{{ inb.port }}</span>
                </div>
              </label>

              <div v-if="!availableInbounds.length" class="text-center py-4 text-gray-500">
                暂无可用的入站节点，请先在「入站节点」中添加
              </div>
            </div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-gray-300 mb-1 font-medium">流控模式 (Flow)</label>
              <select
                v-model="form.flow"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              >
                <option value="xtls-rprx-vision">xtls-rprx-vision (推荐)</option>
                <option value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option>
                <option value="">none (无流控)</option>
              </select>
            </div>

            <div>
              <label class="block text-gray-300 mb-1 font-medium">总流量限制 (GB, 0为不限)</label>
              <input
                v-model.number="form.totalGB"
                type="number"
                min="0"
                placeholder="0"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-gray-300 mb-1 font-medium">有效期 (天数, 0为永不过期)</label>
              <input
                v-model.number="form.expireDays"
                type="number"
                min="0"
                placeholder="0"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div class="flex items-center gap-2 pt-6">
              <label class="flex items-center gap-2 cursor-pointer text-gray-300">
                <input type="checkbox" v-model="form.enabled" class="rounded bg-gray-900 border-gray-700 text-brand-600" />
                <span>启用该用户状态</span>
              </label>
            </div>
          </div>

          <div class="flex justify-end gap-3 pt-3 border-t border-gray-800">
            <button
              type="button"
              @click="showModal = false"
              class="px-5 py-2.5 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 font-medium text-xs transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              :disabled="saving"
              class="px-5 py-2.5 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-semibold text-xs rounded-xl transition-all shadow-lg shadow-brand-500/25 disabled:opacity-50"
            >
              <span>{{ saving ? '保存中...' : '保存并实时热生效' }}</span>
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Aggregated Subscription & Single Node Share Modal -->
    <div v-if="showShareModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-2xl p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-6 my-8 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white">用户订阅与节点分享 ({{ currentShareData?.email }})</h2>
            <p class="text-xs text-gray-400 mt-0.5">支持一键全量订阅与各个节点的独立分享直连</p>
          </div>
          <button @click="showShareModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <!-- 1. 全局聚合订阅 (All Nodes Subscription) -->
        <div class="bg-gradient-to-br from-brand-950/60 to-indigo-950/60 p-5 rounded-2xl border border-brand-500/30 space-y-4">
          <div class="flex items-center justify-between">
            <span class="text-xs font-bold text-brand-300 uppercase tracking-wider flex items-center gap-1.5">
              <span>📦 全节点聚合订阅链接 (包含已授权的所有 {{ currentShareData?.nodes?.length || 0 }} 个节点)</span>
            </span>
          </div>

          <div class="flex flex-col sm:flex-row gap-4 items-center">
            <!-- QR Code -->
            <div class="p-2 bg-white rounded-xl shadow-lg shrink-0">
              <qrcode-vue :value="currentShareData?.allSubUrl || ''" :size="100" level="M" />
            </div>

            <div class="flex-1 space-y-2 w-full text-xs">
              <div class="p-2.5 bg-gray-900/90 rounded-xl border border-gray-800 font-mono text-[11px] text-white break-all select-all">
                {{ currentShareData?.allSubUrl }}
              </div>
              <div class="flex flex-wrap gap-2">
                <button
                  @click="copyText(currentShareData?.allSubUrl)"
                  class="px-4 py-2 bg-brand-600 hover:bg-brand-500 text-white rounded-xl font-semibold text-xs transition-colors flex items-center gap-1.5"
                >
                  <Copy class="w-3.5 h-3.5" />
                  <span>复制全节点聚合订阅</span>
                </button>
              </div>
            </div>
          </div>
        </div>

        <!-- 2. 单节点独立直连与独立订阅列表 (Single Node Breakdowns) -->
        <div class="space-y-3">
          <h3 class="text-xs font-bold text-gray-300 uppercase tracking-wider">
            📍 单节点独立导出与直连 (按节点独立使用)
          </h3>

          <div class="divide-y divide-gray-800 rounded-2xl border border-gray-800 bg-gray-900/50 overflow-hidden text-xs">
            <div
              v-for="node in currentShareData?.nodes"
              :key="node.tag"
              class="p-4 space-y-3 hover:bg-white/[0.01] transition-colors"
            >
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2">
                  <span class="font-bold text-white font-mono text-sm">{{ node.tag }}</span>
                  <span class="px-2 py-0.5 rounded-md text-[10px] font-mono uppercase bg-gray-800 text-cyan-400 border border-gray-700">
                    {{ node.protocol }}
                  </span>
                </div>
              </div>

              <!-- Node direct share link -->
              <div class="space-y-1">
                <span class="text-gray-400 text-[11px]">节点直连分享链接 (vless:// / vmess://):</span>
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    readonly
                    :value="node.shareLink"
                    class="w-full bg-gray-900 border border-gray-800 rounded-xl px-3 py-1.5 text-[11px] font-mono text-gray-300 focus:outline-none"
                  />
                  <button
                    @click="copyText(node.shareLink)"
                    class="px-3 py-1.5 bg-gray-800 hover:bg-gray-700 text-white rounded-xl font-semibold text-xs shrink-0 transition-colors"
                  >
                    复制链接
                  </button>
                </div>
              </div>

              <!-- Node single sub link -->
              <div class="space-y-1">
                <span class="text-gray-400 text-[11px]">单节点专属订阅链接:</span>
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    readonly
                    :value="node.singleSub"
                    class="w-full bg-gray-900 border border-gray-800 rounded-xl px-3 py-1.5 text-[11px] font-mono text-gray-400 focus:outline-none"
                  />
                  <button
                    @click="copyText(node.singleSub)"
                    class="px-3 py-1.5 bg-gray-800 hover:bg-gray-700 text-white rounded-xl font-semibold text-xs shrink-0 transition-colors"
                  >
                    复制单订阅
                  </button>
                </div>
              </div>
            </div>

            <div v-if="!currentShareData?.nodes?.length" class="text-center py-6 text-gray-500">
              该用户尚未绑定任何入站节点
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { UserPlus, Edit, Trash2, RotateCcw, Share2, Copy } from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import api from '../api'

const users = ref<any[]>([])
const availableInbounds = ref<any[]>([])
const showModal = ref(false)
const showShareModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const currentShareData = ref<any>(null)

const form = ref<any>({
  id: 0,
  email: '',
  selectedTags: [] as string[],
  flow: 'xtls-rprx-vision',
  totalGB: 0,
  expireDays: 0,
  enabled: true,
})

const isAllInboundsSelected = computed(() => {
  if (!availableInbounds.value.length) return false
  return form.value.selectedTags.length === availableInbounds.value.length
})

const toggleSelectAllInbounds = () => {
  if (isAllInboundsSelected.value) {
    form.value.selectedTags = []
  } else {
    form.value.selectedTags = availableInbounds.value.map((i) => i.tag)
  }
}

const fetchAll = async () => {
  try {
    const [uRes, inbRes]: any = await Promise.all([api.get('/users'), api.get('/inbounds')])
    users.value = uRes || []
    availableInbounds.value = inbRes || []
  } catch (err) {
    console.error(err)
  }
}

const getNodeTags = (user: any): string[] => {
  if (user.inboundTags) {
    return user.inboundTags.split(',').map((s: string) => s.trim()).filter((s: string) => s)
  }
  if (user.inboundTag) return [user.inboundTag]
  return []
}

const openAddModal = () => {
  isEditing.value = false
  form.value = {
    id: 0,
    email: '',
    selectedTags: availableInbounds.value.map((i) => i.tag), // 默认选中全部节点
    flow: 'xtls-rprx-vision',
    totalGB: 0,
    expireDays: 0,
    enabled: true,
  }
  showModal.value = true
}

const openEditModal = (user: any) => {
  isEditing.value = true
  form.value = {
    id: user.id,
    email: user.email,
    selectedTags: getNodeTags(user),
    flow: user.flow || '',
    totalGB: user.totalBytes > 0 ? Math.round(user.totalBytes / 1073741824) : 0,
    expireDays: 0,
    enabled: user.enabled,
  }
  showModal.value = true
}

const saveUser = async () => {
  if (!form.value.selectedTags.length) {
    alert('请至少选择一个归属的入站节点！')
    return
  }

  saving.value = true
  try {
    const payload: any = {
      email: form.value.email,
      inboundTags: form.value.selectedTags,
      inboundTag: form.value.selectedTags[0],
      flow: form.value.flow,
      totalBytes: form.value.totalGB > 0 ? form.value.totalGB * 1073741824 : 0,
      expireDays: form.value.expireDays,
      enabled: form.value.enabled,
    }

    if (isEditing.value) {
      const existingUser = users.value.find((u) => u.id === form.value.id)
      await api.put(`/users/${form.value.id}`, {
        ...existingUser,
        inboundTags: form.value.selectedTags.join(','),
        inboundTag: form.value.selectedTags[0],
        flow: form.value.flow,
        totalBytes: payload.totalBytes,
        enabled: form.value.enabled,
      })
    } else {
      await api.post('/users', payload)
    }

    showModal.value = false
    await fetchAll()
  } catch (err: any) {
    alert('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteUser = async (id: number) => {
  if (!confirm('确定删除该用户并将其从所有节点下线吗？')) return
  try {
    await api.delete(`/users/${id}`)
    await fetchAll()
  } catch (err: any) {
    alert('删除失败: ' + err)
  }
}

const resetTraffic = async (id: number) => {
  if (!confirm('确定重置该用户的上下行流量吗？')) return
  try {
    await api.post(`/users/${id}/reset-traffic`)
    await fetchAll()
  } catch (err: any) {
    alert('重置失败: ' + err)
  }
}

const openShareModal = async (user: any) => {
  try {
    const res: any = await api.get(`/users/${user.id}/share`)
    currentShareData.value = res
    showShareModal.value = true
  } catch (err: any) {
    alert('获取订阅链接失败: ' + err)
  }
}

const copyText = (text: string) => {
  if (!text) return
  navigator.clipboard.writeText(text)
  alert('已成功复制到剪贴板！')
}

const getTrafficPercent = (user: any) => {
  if (!user.totalBytes) return 0
  const used = user.upBytes + user.downBytes
  return (used / user.totalBytes) * 100
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

const formatDate = (ms: number) => {
  return new Date(ms).toLocaleDateString()
}

onMounted(() => {
  fetchAll()
})
</script>
