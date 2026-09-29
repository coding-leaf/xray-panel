<template>
  <!-- Global Toast Notification Container -->
  <ToastContainer />

  <div v-if="isBlankLayout" class="min-h-screen bg-background">
    <router-view />
  </div>

  <div v-else class="min-h-screen flex bg-background text-foreground font-sans selection:bg-neutral-800 selection:text-neutral-200">
    <!-- Compact Desktop Sidebar (Vercel / Linear Style) -->
    <aside class="w-56 bg-neutral-950 border-r border-border flex flex-col justify-between hidden md:flex shrink-0 z-20">
      <div class="flex-1 overflow-y-auto py-3">
        <!-- Brand Header -->
        <div class="h-10 flex items-center px-4 gap-2.5 mb-2">
          <div class="w-6 h-6 rounded-md bg-neutral-900 border border-border flex items-center justify-center text-foreground">
            <Radio class="w-3.5 h-3.5 text-neutral-200" />
          </div>
          <div class="flex items-center gap-1.5">
            <span class="font-mono font-semibold tracking-tight text-xs text-foreground">XRAY PANEL</span>
            <span class="text-[9px] font-mono px-1 py-0.2 bg-muted text-muted-foreground rounded border border-border/60">v2.5</span>
          </div>
        </div>

        <!-- Categorized Navigation -->
        <nav class="px-2 space-y-4">
          <div v-for="section in navSections" :key="section.title" class="space-y-0.5">
            <div class="text-[10px] font-mono uppercase tracking-wider text-muted-foreground/60 px-2 py-1">
              {{ section.title }}
            </div>
            <router-link
              v-for="item in section.items"
              :key="item.path"
              :to="item.path"
              class="flex items-center gap-2.5 px-2.5 py-1.5 rounded-md text-xs font-medium transition-colors"
              :class="[
                $route.path === item.path
                  ? 'bg-muted text-foreground border border-border/60 shadow-xs'
                  : 'text-muted-foreground hover:text-foreground hover:bg-muted/40 border border-transparent'
              ]"
            >
              <component
                :is="item.icon"
                class="w-3.5 h-3.5 shrink-0"
                :class="$route.path === item.path ? 'text-foreground' : 'text-muted-foreground'"
              />
              <span>{{ item.name }}</span>
            </router-link>
          </div>
        </nav>
      </div>

      <!-- User footer -->
      <div class="p-2.5 border-t border-border bg-neutral-950">
        <button
          @click="logout"
          class="w-full flex items-center justify-between px-2.5 py-1.5 rounded-md text-xs font-medium text-muted-foreground hover:text-rose-400 hover:bg-rose-500/10 transition-colors group border border-transparent hover:border-rose-500/20"
        >
          <span class="flex items-center gap-2">
            <LogOut class="w-3.5 h-3.5 group-hover:-translate-x-0.5 transition-transform" />
            <span>退出系统</span>
          </span>
          <span class="text-[10px] text-muted-foreground font-mono bg-muted/60 px-1.5 py-0.5 rounded border border-border/40">{{ username }}</span>
        </button>
      </div>
    </aside>

    <!-- Mobile Drawer -->
    <div
      v-if="isMobileDrawerOpen"
      class="fixed inset-0 z-50 md:hidden flex"
    >
      <!-- Backdrop -->
      <div
        @click="isMobileDrawerOpen = false"
        class="fixed inset-0 bg-black/70 backdrop-blur-[2px] transition-opacity"
      ></div>

      <!-- Drawer Content -->
      <div class="relative w-72 max-w-xs bg-neutral-950 border-r border-border h-full flex flex-col justify-between z-10 shadow-2xl">
        <div class="overflow-y-auto py-3">
          <!-- Drawer Header -->
          <div class="h-12 flex items-center justify-between px-4 border-b border-border mb-2">
            <div class="flex items-center gap-2">
              <div class="w-6 h-6 rounded-md bg-neutral-900 border border-border flex items-center justify-center">
                <Radio class="w-3.5 h-3.5 text-neutral-200" />
              </div>
              <span class="font-mono font-semibold text-xs text-foreground">XRAY PANEL</span>
            </div>
            <button
              @click="isMobileDrawerOpen = false"
              class="p-1 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <!-- Xray Status -->
          <div class="px-3 py-2">
            <div
              class="flex items-center justify-between px-2.5 py-1.5 rounded-md border text-xs font-mono"
              :class="coreStatus.active ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400' : 'bg-rose-500/10 border-rose-500/20 text-rose-400'"
            >
              <div class="flex items-center gap-1.5">
                <span class="w-1.5 h-1.5 rounded-full" :class="coreStatus.active ? 'bg-emerald-400' : 'bg-rose-500'"></span>
                <span class="text-[11px]">{{ coreStatus.active ? 'Xray Core 运行中' : 'Core 已停止' }}</span>
              </div>
              <button
                @click="restartCore"
                :disabled="restarting"
                class="px-2 py-0.5 rounded bg-neutral-800 text-neutral-300 hover:text-white text-[10px] border border-border"
              >
                {{ restarting ? '...' : '重启' }}
              </button>
            </div>
          </div>

          <!-- Navigation items -->
          <nav class="px-2 space-y-4 pb-6">
            <div v-for="section in navSections" :key="section.title" class="space-y-0.5">
              <div class="text-[10px] font-mono uppercase tracking-wider text-muted-foreground/60 px-2 py-1">
                {{ section.title }}
              </div>
              <router-link
                v-for="item in section.items"
                :key="item.path"
                :to="item.path"
                @click="isMobileDrawerOpen = false"
                class="flex items-center gap-2.5 px-2.5 py-2 rounded-md text-xs font-medium transition-colors"
                :class="[
                  $route.path === item.path
                    ? 'bg-muted text-foreground border border-border/60'
                    : 'text-muted-foreground hover:text-foreground hover:bg-muted/40'
                ]"
              >
                <component
                  :is="item.icon"
                  class="w-3.5 h-3.5 shrink-0"
                  :class="$route.path === item.path ? 'text-foreground' : 'text-muted-foreground'"
                />
                <span>{{ item.name }}</span>
              </router-link>
            </div>
          </nav>
        </div>

        <!-- Drawer Footer -->
        <div class="p-3 border-t border-border bg-neutral-950">
          <button
            @click="logout"
            class="w-full flex items-center justify-between px-2.5 py-1.5 rounded-md text-xs font-medium text-muted-foreground hover:text-rose-400 transition-colors"
          >
            <span class="flex items-center gap-2">
              <LogOut class="w-3.5 h-3.5" />
              <span>退出系统</span>
            </span>
            <span class="text-[10px] font-mono text-muted-foreground bg-muted/60 px-1.5 py-0.5 rounded">{{ username }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Main Shell Area -->
    <div class="flex-1 flex flex-col min-w-0 overflow-hidden">
      <!-- Desktop Top Console Bar -->
      <header class="h-12 bg-neutral-950 border-b border-border hidden md:flex items-center justify-between px-6 z-10 shrink-0">
        <!-- Breadcrumb -->
        <div class="flex items-center gap-2">
          <span class="text-[11px] text-muted-foreground font-mono">{{ currentSectionTitle }}</span>
          <span class="text-neutral-600 text-xs">/</span>
          <span class="text-xs font-medium text-foreground">{{ currentViewName }}</span>
        </div>

        <!-- Global Status Pill & Quick Controls -->
        <div class="flex items-center gap-2.5">
          <!-- Mock Demo Indicator -->
          <div v-if="isMock" class="flex items-center gap-1.5 bg-amber-500/10 border border-amber-500/20 px-2 py-0.5 rounded-md text-amber-300 text-[11px] font-mono">
            <span class="w-1.5 h-1.5 rounded-full bg-amber-400 animate-pulse"></span>
            <span>DEMO</span>
            <button
              @click="handleResetDemo"
              class="ml-1 px-1.5 py-0.2 rounded bg-amber-500/20 hover:bg-amber-500/30 text-[10px] text-amber-200 transition-colors border border-amber-500/30"
              title="重置演示状态"
            >
              重置
            </button>
          </div>

          <!-- Xray Active Status Badge -->
          <div
            class="flex items-center gap-1.5 px-2.5 py-1 rounded-md border text-[11px] font-mono transition-colors"
            :class="coreStatus.active ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400' : 'bg-rose-500/10 border-rose-500/20 text-rose-400'"
          >
            <span
              class="w-1.5 h-1.5 rounded-full"
              :class="coreStatus.active ? 'bg-emerald-400 pulse-green' : 'bg-rose-500 animate-pulse'"
            ></span>
            <span>{{ coreStatus.active ? (coreStatus.version ? `Core ${coreStatus.version}` : 'Core Active') : 'Core Stopped' }}</span>
          </div>

          <!-- Quick Refresh Button -->
          <button
            @click="triggerGlobalRefresh"
            :disabled="refreshing"
            class="h-7 w-7 rounded-md bg-muted/50 hover:bg-muted text-muted-foreground hover:text-foreground border border-border/60 flex items-center justify-center transition-colors"
            title="刷新数据"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': refreshing }" />
          </button>

          <!-- Quick Restart Core -->
          <button
            @click="restartCore"
            :disabled="restarting"
            class="h-7 px-2.5 rounded-md bg-muted/60 hover:bg-muted text-muted-foreground hover:text-foreground border border-border/80 text-[11px] font-mono flex items-center gap-1.5 transition-colors disabled:opacity-50"
            title="重启 Xray 核心"
          >
            <Zap class="w-3 h-3 text-amber-400" />
            <span>{{ restarting ? '重启中...' : '重启核心' }}</span>
          </button>
        </div>
      </header>

      <!-- Mobile Top Navbar -->
      <header class="h-12 md:hidden bg-neutral-950 border-b border-border flex items-center justify-between px-3.5 z-10 shrink-0">
        <div class="flex items-center gap-2">
          <button
            @click="isMobileDrawerOpen = true"
            class="p-1.5 rounded-md bg-muted text-muted-foreground hover:text-foreground border border-border"
            title="展开导航菜单"
          >
            <Menu class="w-4 h-4" />
          </button>
          <span class="font-medium text-xs text-foreground">{{ currentViewName }}</span>
        </div>

        <div class="flex items-center gap-1.5">
          <div
            class="flex items-center gap-1 px-2 py-0.5 rounded-md border text-[10px] font-mono"
            :class="coreStatus.active ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-400' : 'bg-rose-500/10 border-rose-500/20 text-rose-400'"
          >
            <span class="w-1.5 h-1.5 rounded-full" :class="coreStatus.active ? 'bg-emerald-400' : 'bg-rose-500'"></span>
            <span>{{ coreStatus.active ? '运行中' : '停止' }}</span>
          </div>

          <button
            @click="restartCore"
            :disabled="restarting"
            class="p-1.5 rounded-md bg-muted text-muted-foreground border border-border text-xs"
            title="重启核心"
          >
            <Zap class="w-3.5 h-3.5" :class="{ 'animate-spin': restarting }" />
          </button>

          <button
            @click="triggerGlobalRefresh"
            :disabled="refreshing"
            class="p-1.5 rounded-md bg-muted text-muted-foreground border border-border text-xs"
            title="刷新"
          >
            <RefreshCw class="w-3.5 h-3.5" :class="{ 'animate-spin': refreshing }" />
          </button>
        </div>
      </header>

      <!-- Mobile Bottom Navigation Bar (4 high-frequency items) -->
      <nav class="md:hidden fixed bottom-0 left-0 right-0 h-12 pb-[env(safe-area-inset-bottom)] box-content bg-neutral-950/95 border-t border-border flex items-center justify-around px-2 z-40">
        <router-link
          v-for="item in mobileNavItems"
          :key="item.path"
          :to="item.path"
          class="flex flex-col items-center justify-center gap-0.5 text-[10px] font-medium transition-colors px-3 py-1 rounded-md"
          :class="$route.path === item.path ? 'text-foreground font-semibold bg-muted' : 'text-muted-foreground hover:text-foreground'"
        >
          <component :is="item.icon" class="w-3.5 h-3.5" />
          <span>{{ item.shortName || item.name }}</span>
        </router-link>
      </nav>

      <!-- Main Router Content View Area -->
      <main class="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-7 pb-20 md:pb-8 min-w-0">
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
import { isMockMode, resetMockState } from './mock'

const route = useRoute()
const router = useRouter()

const isMock = computed(() => isMockMode())
const handleResetDemo = () => {
  if (confirm('确认将演示数据重置为初始状态吗？')) {
    resetMockState()
    toast.success('已恢复初始状态！')
    window.location.reload()
  }
}

const isBlankLayout = computed(() => {
  const p = route.path || (typeof window !== 'undefined' ? window.location.pathname : '')
  return ['/login', '/portal'].includes(p) || p.startsWith('/portal') || p.startsWith('/login') || route.meta?.layout === 'blank'
})
const username = computed(() => localStorage.getItem('username') || 'admin')

const isMobileDrawerOpen = ref(false)
const refreshing = ref(false)
const restarting = ref(false)

interface NavItem {
  name: string
  path: string
  icon: any
  shortName?: string
}

interface NavSection {
  title: string
  items: NavItem[]
}

const navSections: NavSection[] = [
  {
    title: 'OVERVIEW',
    items: [
      { name: '运行监控', path: '/', icon: LayoutDashboard, shortName: '监控' },
    ],
  },
  {
    title: 'RESOURCES',
    items: [
      { name: '入站网关', path: '/inbounds', icon: Radio, shortName: '网关' },
      { name: '出站出口', path: '/outbounds', icon: Send, shortName: '出站' },
      { name: '路由分流', path: '/routing', icon: RouteIcon, shortName: '路由' },
      { name: '用户与订阅', path: '/users', icon: Users, shortName: '用户' },
    ],
  },
  {
    title: 'OPERATIONS',
    items: [
      { name: '线路与拓扑', path: '/topology', icon: Network, shortName: '拓扑' },
      { name: '运行日志', path: '/logs', icon: ScrollText, shortName: '日志' },
    ],
  },
  {
    title: 'SYSTEM',
    items: [
      { name: 'DNS 设置', path: '/dns', icon: Globe, shortName: 'DNS' },
      { name: '配置编辑', path: '/config', icon: FileCode2, shortName: '配置' },
      { name: '系统设置', path: '/settings', icon: Settings, shortName: '设置' },
    ],
  },
]

const allNavItems = computed(() => navSections.flatMap((s) => s.items))

const mobileNavItems = [
  navSections[0].items[0], // 监控
  navSections[1].items[0], // 入站
  navSections[1].items[3], // 用户
  navSections[2].items[1], // 日志
]

const currentSectionTitle = computed(() => {
  for (const s of navSections) {
    if (s.items.some((i) => i.path === route.path)) {
      return s.title
    }
  }
  return 'PANEL'
})

const currentViewName = computed(() => {
  const cur = allNavItems.value.find((n) => n.path === route.path)
  return cur ? cur.name : '控制面板'
})

const coreStatus = ref<{ active: boolean; version?: string }>({
  active: true,
  version: '',
})
let statusTimer: any = null

const fetchCoreStatus = async () => {
  if (isBlankLayout.value) return
  try {
    const res: any = await api.get('/service/status')
    if (res) {
      coreStatus.value = {
        active: !!res.active,
        version: res.version || '',
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
    toast.info('面板数据已同步')
  }, 400)
}

const restartCore = async () => {
  restarting.value = true
  try {
    await api.post('/service/restart')
    toast.success('Xray 核心已成功重启')
    await fetchCoreStatus()
  } catch (err: any) {
    toast.error('重启失败: ' + err)
  } finally {
    restarting.value = false
  }
}

const handleVisibilityChange = () => {
  if (document.hidden) {
    if (statusTimer) {
      clearInterval(statusTimer)
      statusTimer = null
    }
  } else {
    fetchCoreStatus()
    if (!statusTimer) {
      statusTimer = setInterval(fetchCoreStatus, 6000)
    }
  }
}

onMounted(() => {
  fetchCoreStatus()
  statusTimer = setInterval(fetchCoreStatus, 6000)
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  if (statusTimer) clearInterval(statusTimer)
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})

const logout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  toast.info('已安全退出系统')
  router.push('/login')
}
</script>
