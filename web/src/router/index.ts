import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { isMockMode } from '../mock'

const LoginView = () => import('../views/LoginView.vue')
const PortalClaimView = () => import('../views/PortalClaimView.vue')
const DashboardView = () => import('../views/DashboardView.vue')
const TopologyView = () => import('../views/TopologyView.vue')
const InboundsView = () => import('../views/InboundsView.vue')
const OutboundsView = () => import('../views/OutboundsView.vue')
const RoutingView = () => import('../views/RoutingView.vue')
const DNSView = () => import('../views/DNSView.vue')
const UsersView = () => import('../views/UsersView.vue')
const ConfigView = () => import('../views/ConfigView.vue')
const LogsView = () => import('../views/LogsView.vue')
const SettingsView = () => import('../views/SettingsView.vue')

const routes: RouteRecordRaw[] = [
  { path: '/login', component: LoginView, meta: { layout: 'blank' } },
  { path: '/portal', component: PortalClaimView, meta: { layout: 'blank' } },
  { path: '/', component: DashboardView, meta: { requiresAuth: true } },
  { path: '/topology', component: TopologyView, meta: { requiresAuth: true } },
  { path: '/inbounds', component: InboundsView, meta: { requiresAuth: true } },
  { path: '/outbounds', component: OutboundsView, meta: { requiresAuth: true } },
  { path: '/routing', component: RoutingView, meta: { requiresAuth: true } },
  { path: '/dns', component: DNSView, meta: { requiresAuth: true } },
  { path: '/users', component: UsersView, meta: { requiresAuth: true } },
  { path: '/config', component: ConfigView, meta: { requiresAuth: true } },
  { path: '/logs', component: LogsView, meta: { requiresAuth: true } },
  { path: '/settings', component: SettingsView, meta: { requiresAuth: true } },
  { path: '/:pathMatch(.*)*', redirect: '/' },
]

const router = createRouter({
  history: createWebHistory((import.meta as any).env?.BASE_URL || '/'),
  routes,
})

router.beforeEach((to, from, next) => {
  if (isMockMode() && !localStorage.getItem('token')) {
    localStorage.setItem('token', 'demo-mock-jwt-token')
  }
  const token = localStorage.getItem('token')
  if (to.meta.requiresAuth && !token) {
    next('/login')
  } else if (to.path === '/login' && token && !isMockMode()) {
    next('/')
  } else {
    next()
  }
})

router.afterEach((to) => {
  if (to.path === '/portal') {
    document.title = '分布式网管接入点'
  } else if (to.path === '/login') {
    document.title = '系统登录 - System Management Console'
  } else {
    document.title = 'System Management Console'
  }
})

export default router
