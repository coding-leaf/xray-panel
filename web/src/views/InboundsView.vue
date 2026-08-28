<template>
  <div class="space-y-6">
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight">节点与入站管理</h1>
        <p class="text-xs text-gray-400 mt-0.5">查看与管理 Xray 入站代理端口、传输层与 Reality 安全设置</p>
      </div>
      <button
        @click="openCreateModal"
        class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
      >
        <Plus class="w-4 h-4" />
        <span>添加节点</span>
      </button>
    </div>

    <!-- Inbounds List Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
      <div
        v-for="inb in inbounds"
        :key="inb.id"
        class="glass-panel p-5 rounded-2xl border border-gray-800/80 hover:border-brand-500/40 transition-all flex flex-col justify-between"
      >
        <div>
          <div class="flex items-center justify-between mb-3">
            <span class="text-base font-bold text-white tracking-tight">{{ inb.tag }}</span>
            <span class="px-2.5 py-0.5 rounded-full text-xs font-mono font-bold uppercase" :class="protocolBadgeColor(inb.protocol)">
              {{ inb.protocol }}
            </span>
          </div>

          <div class="space-y-2 text-xs text-gray-400">
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>监听端口</span>
              <span class="text-brand-400 font-mono font-semibold">{{ inb.port }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>监听地址</span>
              <span class="text-gray-200 font-mono">{{ inb.listen || '0.0.0.0' }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>累计上下行</span>
              <span class="text-gray-200 font-mono">{{ formatBytes(inb.upBytes + inb.downBytes) }}</span>
            </div>
            <div class="flex justify-between py-1">
              <span>运行状态</span>
              <span class="text-emerald-400 font-semibold flex items-center gap-1">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span> 已启用
              </span>
            </div>
          </div>
        </div>

        <div class="mt-4 pt-3 border-t border-gray-800/60 flex items-center justify-between">
          <button
            @click="editInbound(inb)"
            class="text-xs text-brand-400 hover:text-brand-300 font-medium transition-colors"
          >
            编辑参数
          </button>
          <button
            @click="deleteInbound(inb.id)"
            class="text-xs text-red-400 hover:text-red-300 transition-colors"
          >
            删除
          </button>
        </div>
      </div>
    </div>

    <!-- Empty state -->
    <div v-if="!inbounds.length" class="glass-panel p-12 text-center rounded-2xl">
      <Radio class="w-12 h-12 mx-auto text-gray-600 mb-3" />
      <h3 class="text-sm font-semibold text-gray-300">暂无入站节点</h3>
      <p class="text-xs text-gray-500 mt-1">点击右上角“添加节点”或在配置页中导入 config.json</p>
    </div>

    <!-- Create/Edit Modal -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-lg p-6 rounded-3xl border border-gray-800 shadow-2xl space-y-4">
        <h2 class="text-lg font-bold text-white">{{ isEditing ? '编辑节点' : '添加新节点' }}</h2>

        <form @submit.prevent="saveInbound" class="space-y-3 text-xs">
          <div>
            <label class="block text-gray-400 mb-1">节点标识 Tag</label>
            <input
              v-model="form.tag"
              type="text"
              required
              placeholder="vless-reality"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
            />
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="block text-gray-400 mb-1">端口</label>
              <input
                v-model.number="form.port"
                type="number"
                required
                placeholder="443"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
            <div>
              <label class="block text-gray-400 mb-1">协议</label>
              <select
                v-model="form.protocol"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
              >
                <option value="vless">VLESS</option>
                <option value="vmess">VMess</option>
                <option value="trojan">Trojan</option>
                <option value="shadowsocks">Shadowsocks</option>
              </select>
            </div>
          </div>

          <div>
            <label class="block text-gray-400 mb-1">传输配置 (streamSettings JSON)</label>
            <textarea
              v-model="form.streamSettings"
              rows="4"
              placeholder='{"network":"xhttp","security":"reality",...}'
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
            ></textarea>
          </div>

          <div class="flex justify-end gap-2 pt-2">
            <button
              type="button"
              @click="showModal = false"
              class="px-4 py-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 text-xs"
            >
              取消
            </button>
            <button
              type="submit"
              class="px-4 py-2 rounded-xl bg-brand-600 hover:bg-brand-500 text-white text-xs font-semibold"
            >
              保存
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { Plus, Radio } from 'lucide-vue-next'
import api from '../api'

const inbounds = ref<any[]>([])
const showModal = ref(false)
const isEditing = ref(false)
const form = ref<any>({
  tag: '',
  port: 443,
  protocol: 'vless',
  listen: '0.0.0.0',
  streamSettings: '{\n  "network": "xhttp",\n  "security": "reality"\n}',
})

const fetchInbounds = async () => {
  try {
    inbounds.value = await api.get('/inbounds')
  } catch (err) {
    console.error(err)
  }
}

const openCreateModal = () => {
  isEditing.value = false
  form.value = {
    tag: 'vless-reality',
    port: 4434,
    protocol: 'vless',
    listen: '0.0.0.0',
    streamSettings: '{\n  "network": "xhttp",\n  "security": "reality"\n}',
  }
  showModal.value = true
}

const editInbound = (inb: any) => {
  isEditing.value = true
  form.value = { ...inb }
  showModal.value = true
}

const saveInbound = async () => {
  try {
    if (isEditing.value) {
      await api.put(`/inbounds/${form.value.id}`, form.value)
    } else {
      await api.post('/inbounds', form.value)
    }
    showModal.value = false
    await fetchInbounds()
  } catch (err: any) {
    alert('保存失败: ' + err)
  }
}

const deleteInbound = async (id: number) => {
  if (!confirm('确定删除该入站节点吗？')) return
  try {
    await api.delete(`/inbounds/${id}`)
    await fetchInbounds()
  } catch (err: any) {
    alert('删除失败: ' + err)
  }
}

const protocolBadgeColor = (proto: string) => {
  switch (proto?.toLowerCase()) {
    case 'vless':
      return 'bg-brand-500/15 text-brand-300 border border-brand-500/20'
    case 'vmess':
      return 'bg-purple-500/15 text-purple-300 border border-purple-500/20'
    case 'trojan':
      return 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/20'
    default:
      return 'bg-gray-700 text-gray-300'
  }
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

onMounted(() => {
  fetchInbounds()
})
</script>
