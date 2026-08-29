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
            <p class="text-[11px] text-gray-400 font-medium flex items-center gap-1.5">
              <span>Control Plane</span>
              <span class="text-[9px] text-indigo-300 font-mono bg-indigo-500/20 px-1 py-0.5 rounded border border-indigo-500/30">v1.3.0</span>
            </p>
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

    <!-- Mobile Drawer (滑出式全量菜单) -->
    <div
      v-if="isMobileDrawerOpen"
      class="fixed inset-0 z-50 md:hidden flex"
    >
      <!-- 背景遮罩 -->
      <div
        @click="isMobileDrawerOpen = false"
        class="fixed inset-0 bg-black/75 backdrop-blur-sm transition-opacity duration-300"
      ></div>

      <!-- 抽屉菜单本体 -->
      <div class="relative w-4/5 max-w-xs bg-[#090D16] border-r border-white/10 h-full flex flex-col justify-between z-10 shadow-2xl animate-fade-in">
        <div class="overflow-y-auto">
          <!-- 抽屉顶部 -->
          <div class="h-16 flex items-center justify-between px-5 border-b border-white/[0.08]">
            <div class="flex items-center gap-3">
              <div class="w-8 h-8 rounded-xl bg-gradient-to-tr from-indigo-600 via-indigo-500 to-cyan-400 flex items-center justify-center shadow-lg shadow-indigo-500/25">
                <Radio class="w-4 h-4 text-white" />
              </div>
              <div>
                <div class="font-bold text-white text-sm">XRAY PANEL</div>
                <div class="text-[10px] text-gray-400">全部功能导航</div>
              </div>
            </div>
            <button
              @click="isMobileDrawerOpen = false"
              class="p-1.5 rounded-lg text-gray-400 hover:text-white hover:bg-white/10"
            >
              <X class="w-5 h-5" />
            </button>
          </div>

          <!-- Xray 状态卡片 (手机抽屉内) -->
          <div class="p-3.5">
            <div
              class="flex items-center justify-between px-3 py-2 rounded-xl border text-xs font-mono"
              :class="coreStatus.active ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400' : 'bg-rose-500/10 border-rose-500/20 text-rose-400'"
            >
              <div class="flex items-center gap-2">
                <span class="w-2 h-2 rounded-full" :class="coreStatus.active ? 'bg-emerald-400 pulse-green' : 'bg-rose-500 animate-pulse'"></span>
                <span>{{ coreStatus.active ? 'Xray Core 运行中' : 'Xray Core 已停止' }}</span>
              </div>
              <button
                @click="restartCore"
                :disabled="restarting"
                class="px-2 py-1 rounded bg-indigo-600/30 text-indigo-300 hover:text-white text-[11px]"
              >
                {{ restarting ? '...' : '重启' }}
              </button>
            </div>
          </div>

          <!-- 导航菜单列表 -->
          <nav class="px-3 pb-6 space-y-1">
            <router-link
              v-for="item in navItems"
              :key="item.path"
              :to="item.path"
              @click="isMobileDrawerOpen = false"
              class="flex items-center gap-3 px-3.5 py-3 rounded-xl text-xs font-semibold transition-all group"
              :class="[
                $route.path === item.path
                  ? 'bg-indigo-600/25 text-white border border-indigo-500/30'
                  : 'text-gray-300 hover:text-white hover:bg-white/[0.05]'
              ]"
            >
              <component
                :is="item.icon"
                class="w-4 h-4 shrink-0"
                :class="$route.path === item.path ? 'text-indigo-400' : 'text-gray-400 group-hover:text-gray-200'"
              />
              <span>{{ item.name }}</span>
            </router-link>
          </nav>
        </div>

        <!-- 抽屉底部用户退出 -->
        <div class="p-3.5 border-t border-white/[0.08] bg-black/30">
          <button
            @click="logout"
            class="w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl text-xs font-medium text-gray-400 hover:text-rose-400 hover:bg-rose-500/10 transition-colors"
          >
            <span class="flex items-center gap-2.5">
              <LogOut class="w-4 h-4" />
              <span>退出登录</span>
            </span>
            <span class="text-[11px] text-gray-500 font-mono bg-white/[0.05] px-2 py-0.5 rounded-md">{{ username }}</span>
          </button>
        </div>
      </div>
    </div>

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
          <div
            class="flex items-center gap-2 px-3 py-1.5 rounded-full border text-xs font-medium font-mono transition-colors shadow-sm"
            :class="coreStatus.active ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400' : 'bg-rose-500/10 border-rose-500/20 text-rose-400'"
          >
            <span
              class="w-2 h-2 rounded-full"
              :class="coreStatus.active ? 'bg-emerald-400 pulse-green' : 'bg-rose-500 animate-pulse'"
            ></span>
            <span>{{ coreStatus.active ? (coreStatus.version ? `Xray Core 运行中 (${coreStatus.version})` : 'Xray Core 运行中') : 'Xray Core 已停止' }}</span>
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
      <header class="h-14 md:hidden glass-panel border-b border-white/[0.08] flex items-center justify-between px-3.5 z-10 shrink-0">
        <div class="flex items-center gap-2.5">
          <!-- 汉堡按钮 -->
          <button
            @click="isMobileDrawerOpen = true"
            class="p-1.5 -ml-1 rounded-lg bg-white/[0.04] text-gray-300 hover:text-white border border-white/[0.08]"
            title="展开导航菜单"
          >
            <Menu class="w-5 h-5" />
          </button>

          <div class="flex items-center gap-1.5">
            <span class="font-bold text-xs text-white tracking-wide">{{ currentViewName }}</span>
          </div>
        </div>

        <!-- 手机顶栏右侧状态与快捷键 -->
        <div class="flex items-center gap-1.5">
          <!-- 核心状态小胶囊 -->
          <div
            class="flex items-center gap-1 px-2 py-1 rounded-full border text-[10px] font-mono"
            :class="coreStatus.active ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400' : 'bg-rose-500/10 border-rose-500/20 text-rose-400'"
          >
            <span class="w-1.5 h-1.5 rounded-full" :class="coreStatus.active ? 'bg-emerald-400' : 'bg-rose-500 animate-pulse'"></span>
            <span>{{ coreStatus.active ? '运行中' : '停止' }}</span>
          </div>

          <!-- 一键重启 -->
          <button
            @click="restartCore"
            :disabled="restarting"
            class="p-1.5 rounded-lg bg-indigo-600/20 text-indigo-300 border border-indigo-500/30 text-xs disabled:opacity-50"
            title="重启核心"
          >
            <Zap class="w-3.5 h-3.5" :class="{ 'animate-spin': restarting }" />
          </button>

          <!-- 刷新 -->
          <button
            @click="triggerGlobalRefresh"
            :disabled="refreshing"
            class="p-1.5 rounded-lg bg-white/[0.04] text-gray-300 border border-white/[0.08]"
            title="刷新"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': refreshing }" />
          </button>
        </div>
      </header>

      <!-- Mobile Bottom Navigation Bar (4个高频快捷页) -->
      <nav class="md:hidden fixed bottom-0 left-0 right-0 h-14 bg-[#070A11]/90 backdrop-blur-xl border-t border-white/[0.08] flex items-center justify-around px-2 z-40">
        <router-link
          v-for="item in mobileNavItems"
          :key="item.path"
          :to="item.path"
          class="flex flex-col items-center justify-center gap-0.5 text-[10px] font-medium transition-all px-3 py-1 rounded-xl"
          :class="$route.path === item.path ? 'text-indigo-400 font-bold bg-indigo-500/10' : 'text-gray-400 hover:text-gray-200'"
        >
          <component :is="item.icon" class="w-4 h-4" />
          <span>{{ item.shortName || item.name }}</span>
        </router-link>
      </nav>

      <!-- Router View Area -->
      <main class="flex-1 overflow-y-auto p-3.5 sm:p-6 lg:p-8 pb-20 md:pb-8">
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
import { ref, computed, onMounted, onUnmounted } from 'vue'
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
  Menu,
  X,
  Network,
} from 'lucide-vue-next'
import ToastContainer from './components/ToastContainer.vue'
import { toast } from './utils/toast'
import api from './api'

const route = useRoute()
const router = useRouter()

const isLoginPage = computed(() => route.path === '/login')
const username = computed(() => localStorage.getItem('username') || 'admin')

const isMobileDrawerOpen = ref(false)
const refreshing = ref(false)
const restarting = ref(false)

const navItems = [
  { name: '运行监控', path: '/', icon: LayoutDashboard, shortName: '监控' },
  { name: '线路与拓扑', path: '/topology', icon: Network, shortName: '拓扑' },
  { name: '用户与订阅', path: '/users', icon: Users, shortName: '用户' },
  { name: '路由分流', path: '/routing', icon: RouteIcon, shortName: '路由' },
  { name: '入站网关', path: '/inbounds', icon: Radio, shortName: '网关' },
  { name: '出站出口', path: '/outbounds', icon: Send, shortName: '出站' },
  { name: 'DNS 设置', path: '/dns', icon: Globe, shortName: 'DNS' },
  { name: '配置编辑', path: '/config', icon: FileCode2, shortName: '配置' },
  { name: '运行日志', path: '/logs', icon: ScrollText, shortName: '日志' },
  { name: '系统设置', path: '/settings', icon: Settings, shortName: '设置' },
]

// 手机端底部常驻 4 个最常用的核心导航
const mobileNavItems = [
  navItems[0], // 监控
  navItems[1], // 拓扑
  navItems[2], // 用户
  navItems[8], // 日志
]

const currentViewName = computed(() => {
  const cur = navItems.find((n) => n.path === route.path)
  return cur ? cur.name : '控制面板'
})

const coreStatus = ref<{ active: boolean; version?: string }>({
  active: true,
  version: '',
})
let statusTimer: any = null

const fetchCoreStatus = async () => {
  if (isLoginPage.value) return
  try {
    const res: any = await api.get('/dashboard')
    if (res?.service) {
      coreStatus.value = {
        active: !!res.service.active,
        version: res.metrics?.xrayVersion || '',
      }
    }
  } catch (e) {
    coreStatus.value.active = false
  }
}

const triggerGlobalRefresh = () => {
  refreshing.value = true
  fetchCoreStatus()
  setTimeout(() => {
    refreshing.value = false
    toast.info('面板数据已同步更新')
  }, 400)
}

const restartCore = async () => {
  restarting.value = true
  try {
    await api.post('/service/restart')
    toast.success('Xray 核心已成功重启！')
    await fetchCoreStatus()
  } catch (err: any) {
    toast.error('重启失败: ' + err)
  } finally {
    restarting.value = false
  }
}

onMounted(() => {
  fetchCoreStatus()
  statusTimer = setInterval(fetchCoreStatus, 6000)
})

onUnmounted(() => {
  if (statusTimer) clearInterval(statusTimer)
})

const logout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  toast.info('已安全退出系统')
  router.push('/login')
}
</script>
