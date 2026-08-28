<template>
  <div v-if="isLoginPage" class="min-h-screen bg-[#090D16]">
    <router-view />
  </div>

  <div v-else class="min-h-screen flex bg-[#090D16] text-gray-100">
    <!-- Sidebar -->
    <aside class="w-64 glass-panel border-r border-gray-800/80 flex flex-col justify-between hidden md:flex">
      <div>
        <!-- Logo -->
        <div class="h-16 flex items-center px-6 gap-3 border-b border-gray-800/60">
          <div class="w-9 h-9 rounded-xl bg-gradient-to-tr from-brand-600 to-cyan-400 flex items-center justify-center shadow-lg shadow-brand-500/20">
            <Radio class="w-5 h-5 text-white" />
          </div>
          <div>
            <div class="font-bold tracking-tight text-white flex items-center gap-1.5">
              <span>XRAY</span>
              <span class="text-xs px-1.5 py-0.5 rounded bg-brand-500/20 text-brand-400 font-mono">CORE</span>
            </div>
            <p class="text-[11px] text-gray-400">Decoupled Panel</p>
          </div>
        </div>

        <!-- Navigation Links -->
        <nav class="p-4 space-y-1.5">
          <router-link
            v-for="item in navItems"
            :key="item.path"
            :to="item.path"
            class="flex items-center gap-3 px-3.5 py-2.5 rounded-xl text-sm font-medium transition-all duration-200"
            :class="[
              $route.path === item.path
                ? 'bg-brand-600/20 text-brand-300 border border-brand-500/30 shadow-sm'
                : 'text-gray-400 hover:text-gray-200 hover:bg-gray-800/40'
            ]"
          >
            <component :is="item.icon" class="w-4 h-4" />
            <span>{{ item.name }}</span>
          </router-link>
        </nav>
      </div>

      <!-- User footer -->
      <div class="p-4 border-t border-gray-800/60">
        <button
          @click="logout"
          class="w-full flex items-center justify-between px-3.5 py-2.5 rounded-xl text-sm text-gray-400 hover:text-red-400 hover:bg-red-500/10 transition-colors"
        >
          <span class="flex items-center gap-2">
            <LogOut class="w-4 h-4" />
            <span>退出登录</span>
          </span>
          <span class="text-xs text-gray-500 font-mono">{{ username }}</span>
        </button>
      </div>
    </aside>

    <!-- Main Content Area -->
    <div class="flex-1 flex flex-col min-w-0 overflow-hidden">
      <!-- Mobile Top Navbar -->
      <header class="h-16 md:hidden glass-panel border-b border-gray-800/80 flex items-center justify-between px-4">
        <div class="flex items-center gap-2">
          <Radio class="w-5 h-5 text-brand-400" />
          <span class="font-bold text-sm">Xray Panel</span>
        </div>
        <div class="flex items-center gap-2">
          <button @click="logout" class="p-2 text-gray-400 hover:text-red-400">
            <LogOut class="w-4 h-4" />
          </button>
        </div>
      </header>

      <!-- Router View -->
      <main class="flex-1 overflow-y-auto p-4 sm:p-6 lg:p-8">
        <router-view />
      </main>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
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
} from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()

const isLoginPage = computed(() => route.path === '/login')
const username = computed(() => localStorage.getItem('username') || 'admin')

const navItems = [
  { name: '运行监控', path: '/', icon: LayoutDashboard },
  { name: '入站节点', path: '/inbounds', icon: Radio },
  { name: '出站代理', path: '/outbounds', icon: Send },
  { name: '路由分流', path: '/routing', icon: RouteIcon },
  { name: 'DNS 设置', path: '/dns', icon: Globe },
  { name: '用户与订阅', path: '/users', icon: Users },
  { name: '配置编辑', path: '/config', icon: FileCode2 },
  { name: '运行日志', path: '/logs', icon: ScrollText },
  { name: '系统设置', path: '/settings', icon: Settings },
]

const logout = () => {
  localStorage.removeItem('token')
  localStorage.removeItem('username')
  router.push('/login')
}
</script>
