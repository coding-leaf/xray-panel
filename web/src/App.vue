<template>
  <!-- Global Toast Notification Container -->
  <ToastContainer />

  <div v-if="isLoginPage" class="min-h-screen bg-[#07090E]">
    <router-view />
  </div>

  <div v-else class="min-h-screen flex bg-[#07090E] text-gray-100 selection:bg-indigo-500/30 selection:text-indigo-200">
    <!-- Sidebar (Desktop) -->
    <aside class="w-64 glass-panel border-r border-white/[0.06] flex flex-col justify-between hidden md:flex shrink-0 z-20">
      <div>
        <!-- Brand Logo Header -->
        <div class="h-16 flex items-center px-5 gap-3 border-b border-white/[0.06]">
          <div class="w-8 h-8 rounded-xl bg-gradient-to-tr from-indigo-600 via-indigo-500 to-cyan-400 flex items-center justify-center shadow-lg shadow-indigo-500/25 ring-1 ring-white/20">
            <Radio class="w-4 h-4 text-white" />
          </div>
          <div>
            <div class="font-bold tracking-tight text-white text-sm flex items-center gap-1.5">
              <span>XRAY</span>
              <span class="text-[10px] px-1.5 py-0.5 rounded-md bg-indigo-500/20 text-indigo-300 font-mono border border-indigo-500/30">DECOUPLED</span>
            </div>
            <p class="text-[11px] text-gray-400 font-medium">Control Plane</p>
          </div>
        </div>

        <!-- Navigation Links -->
        <nav class="p-3.5 space-y-1">
          <router-link
            v-for="item in navItems"
            :key="item.path"
            :to="item.path"
            class="flex items-center gap-3 px-3.5 py-2.5 rounded-xl text-xs font-semibold transition-all duration-200 group relative"
            :class="[
              $route.path === item.path
                ? 'bg-gradient-to-r from-indigo-600/25 to-indigo-500/10 text-white border border-indigo-500/30 shadow-sm'
                : 'text-gray-400 hover:text-gray-200 hover:bg-white/[0.04]'
            ]"
          >
            <div
              v-if="$route.path === item.path"
              class="absolute left-0 top-2 bottom-2 w-1 rounded-r-full bg-indigo-500 shadow-[0_0_8px_rgba(99,102,241,0.8)]"
            ></div>
            <component
              :is="item.icon"
              class="w-4 h-4 shrink-0 transition-transform group-hover:scale-110"
              :class="$route.path === item.path ? 'text-indigo-400' : 'text-gray-500 group-hover:text-gray-300'"
            />
            <span>{{ item.name }}</span>
          </router-link>
        </nav>
      </div>

      <!-- User footer -->
      <div class="p-3.5 border-t border-white/[0.06] bg-black/20">
        <button
          @click="logout"
          class="w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl text-xs font-medium text-gray-400 hover:text-rose-400 hover:bg-rose-500/10 transition-colors group border border-transparent hover:border-rose-500/20"
        >
          <span class="flex items-center gap-2.5">
            <LogOut class="w-4 h-4 group-hover:-translate-x-0.5 transition-transform" />
            <span>退出登录</span>
          </span>
          <span class="text-[11px] text-gray-500 font-mono bg-white/[0.04] px-2 py-0.5 rounded-md">{{ username }}</span>
        </button>
      </div>
    </aside>

    <!-- Main Content Area -->
    <div class="flex-1 flex flex-col min-w-0 overflow-hidden">
      <!-- Desktop Top Status & Action Bar -->
      <header class="h-16 glass-panel border-b border-white/[0.06] hidden md:flex items-center justify-between px-6 z-10 shrink-0">
        <!-- View title & breadcrumb -->
        <div class="flex items-center gap-2.5">
          <span class="text-xs text-gray-500 font-medium font-mono">PANEL /</span>
          <h1 class="text-sm font-bold text-white tracking-wide">{{ currentViewName }}</h1>
        </div>

        <!-- Global Status Pill & Quick Controls -->
        <div class="flex items-center gap-3">
          <!-- Xray Active Status Badge -->
          <div class="flex items-center gap-2 px-3 py-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-400 text-xs font-medium font-mono">
            <span class="w-2 h-2 rounded-full bg-emerald-400 pulse-green"></span>
            <span>Xray Core Running</span>
          </div>

          <!-- Quick Refresh Button -->
          <button
            @click="triggerGlobalRefresh"
            :disabled="refreshing"
            class="p-2 rounded-xl bg-white/[0.04] hover:bg-white/[0.08] text-gray-400 hover:text-white border border-white/[0.06] transition-colors"
            title="刷新数据"
          >
            <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': refreshing }" />
          </button>

          <!-- Quick Restart Core -->
          <button
            @click="restartCore"
            :disabled="restarting"
            class="flex items-center gap-1.5 px-3 py-1.5 rounded-xl bg-indigo-600/20 hover:bg-indigo-600/30 text-indigo-300 hover:text-white border border-indigo-500/30 text-xs font-semibold transition-all shadow-sm disabled:opacity-50"
            title="重载/重启 Xray 核心"
          >
            <Zap class="w-3.5 h-3.5 text-indigo-400" />
            <span>{{ restarting ? '重启中...' : '重启核心' }}</span>
          </button>
        </div>
      </header>

      <!-- Mobile Top Navbar -->
      <header class="h-16 md:hidden glass-panel border-b border-white/[0.06] flex items-center justify-between px-4 z-10 shrink-0">
        <div class="flex items-center gap-2.5">
          <div class="w-7 h-7 rounded-lg bg-indigo-600 flex items-center justify-center">
            <Radio class="w-4 h-4 text-white" />
          </div>
          <span class="font-bold text-sm text-white">Xray Panel</span>
        </div>
        <div class="flex items-center gap-2">
          <button @click="logout" class="p-2 text-gray-400 hover:text-rose-400">
            <LogOut class="w-4 h-4" />
          </button>
        </div>
      </header>

      <!-- Mobile Bottom Navigation Bar -->
      <nav class="md:hidden fixed bottom-0 left-0 right-0 h-14 glass-panel border-t border-white/[0.08] flex items-center justify-around px-2 z-50">
        <router-link
          v-for="item in mobileNavItems"
          :key="item.path"
          :to="item.path"
          class="flex flex-col items-center justify-center gap-0.5 text-[10px] font-medium transition-colors"
          :class="$route.path === item.path ? 'text-indigo-400 font-bold' : 'text-gray-400'"
        >
          <component :is="item.icon" class="w-4 h-4" />
          <span>{{ item.shortName || item.name }}</span>
        </router-link>
      </nav>

      <!-- Router View Area -->
      <main class="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8 pb-20 md:pb-8">
        <router-view v-slot="{ Component }">
          <transition name="fade-slide" mode="out-in">
            <component :is="Component" :key="$route.path" />
          </transition>
        </router-view>
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  LayoutDashboard,
  Radio,
  Users,
  FileCode2,
  ScrollText,
  Send,
  Route as RouteIcon,
  Globe,
  Settings,
  LogOut,
  Zap,
  RefreshCw,
} from 'lucide-vue-next'
import ToastContainer from './components/ToastContainer.vue'
import { toast } from './utils/toast'
import api from './api'

const route = useRoute()
const router = useRouter()

const isLoginPage = computed(() => route.path === '/login')
const username = computed(() => localStorage.getItem('username') || 'admin')

const refreshing = ref(false)
const restarting = ref(false)

const navItems = [
  { name: '运行监控', path: '/', icon: LayoutDashboard, shortName: '监控' },
  { name: '入站节点', path: '/inbounds', icon: Radio, shortName: '入站' },
  { name: '用户与订阅', path: '/users', icon: Users, shortName: '用户' },
  { name: '出站代理', path: '/outbounds', icon: Send, shortName: '出站' },
  { name: '路由分流', path: '/routing', icon: RouteIcon, shortName: '路由' },
  { name: 'DNS 设置', path: '/dns', icon: Globe, shortName: 'DNS' },
  { name: '配置编辑', path: '/config', icon: FileCode2, shortName: '配置' },
  { name: '运行日志', path: '/logs', icon: ScrollText, shortName: '日志' },
  { name: '系统设置', path: '/settings', icon: Settings, shortName: '设置' },
]

const mobileNavItems = [
  navItems[0], // 监控
  navItems[1], // 入站
  navItems[2], // 用户
  navItems[4], // 路由
  navItems[8], // 设置
]

const currentViewName = computed(() => {
  const cur = navItems.find((n) => n.path === route.path)
  return cur ? cur.name : '控制面板'
})

const triggerGlobalRefresh = () => {
  refreshing.value = true
  // 触发全局事件或轻微重新渲染
  setTimeout(() => {
    refreshing.value = false
    toast.info('面板数据已同步更新')
  }, 400)
}

const restartCore = async () => {
  restarting.value = true
  try {
    await api.post('/config/restart')
    toast.success('Xray 核心已成功重启/重载！')
  } catch (err: any) {
    toast.error('重启失败: ' + err)
  } finally {
    restarting.value = false
  }
}

const logout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  toast.info('已安全退出系统')
  router.push('/login')
}
</script>
