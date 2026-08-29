import { createRouter, createWebHistory } from 'vue-router'
import DashboardView from '../views/DashboardView.vue'
import TopologyView from '../views/TopologyView.vue'
import InboundsView from '../views/InboundsView.vue'
import OutboundsView from '../views/OutboundsView.vue'
import RoutingView from '../views/RoutingView.vue'
import DNSView from '../views/DNSView.vue'
import UsersView from '../views/UsersView.vue'
import ConfigView from '../views/ConfigView.vue'
import LogsView from '../views/LogsView.vue'
import SettingsView from '../views/SettingsView.vue'
import LoginView from '../views/LoginView.vue'

import { isMockMode } from '../mock'

const routes = [
  { path: '/login', component: LoginView },
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

export default router
