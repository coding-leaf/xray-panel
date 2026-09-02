// Mock 拦截请求调度中心 (v2)

import { loadMockState, saveMockState, resetMockState, MockState } from './storage'

export { resetMockState }

export function isMockMode(): boolean {
  const env = (import.meta as any).env || {}
  return (
    env.VITE_MOCK_MODE === 'true' ||
    env.MODE === 'demo' ||
    window.location.search.includes('mock=true') ||
    window.location.hostname.includes('github.io')
  )
}

function delay<T>(data: T, ms = 120): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(data), ms))
}

// 辅助函数：根据 VLESS Route ID 动态置换 UUID 的第 3 节 (bytes 6:8)
function applyRouteIdToUuid(uuid: string, routeId: number): string {
  if (!routeId || routeId <= 0) return uuid
  const parts = uuid.split('-')
  if (parts.length === 5) {
    parts[2] = routeId.toString(16).padStart(4, '0')
    return parts.join('-')
  }
  return uuid
}

export async function handleMockRequest(url: string, method: string, data?: any): Promise<any> {
  const cleanUrl = url.split('?')[0]
  const state = loadMockState()

  // 1. Auth 登录与状态
  if (cleanUrl.endsWith('/auth/login') && method === 'POST') {
    return delay({
      token: 'demo-jwt-token-mock-mode',
      username: 'admin',
    })
  }

  // 2. Dashboard 仪表盘
  if (cleanUrl.endsWith('/dashboard') && method === 'GET') {
    return delay({
      xrayStatus: {
        running: true,
        version: 'Xray 26.3.27 (Demo Engine) Linux/amd64',
        uptimeSecs: 48600,
        xrayPid: 12345,
      },
      hostStatus: {
        cpuPercent: Math.round(18 + Math.random() * 8),
        memUsedBytes: 384 * 1024 * 1024,
        memTotalBytes: 1024 * 1024 * 1024,
        diskUsedBytes: 12 * 1024 * 1024 * 1024,
        diskTotalBytes: 40 * 1024 * 1024 * 1024,
        netInSpeed: Math.round(1024 * (1200 + Math.random() * 300)),
        netOutSpeed: Math.round(1024 * (8450 + Math.random() * 1200)),
      },
      stats: {
        totalUsers: state.users.length,
        onlineUsers: state.users.filter((u) => u.isOnline).length,
        totalInbounds: state.inbounds.length,
        totalOutbounds: state.outbounds.length,
        totalTrafficBytes: state.users.reduce((acc, u) => acc + (u.upBytes || 0) + (u.downBytes || 0), 0),
      },
    })
  }

  // 3. Inbounds 入站网关
  if (cleanUrl.endsWith('/inbounds') && method === 'GET') {
    return delay(state.inbounds)
  }

  if (cleanUrl.endsWith('/inbounds') && method === 'POST') {
    const newId = state.inbounds.reduce((max, i) => Math.max(max, i.id || 0), 0) + 1
    const newInbound = { ...data, id: newId, isAlive: true, latencyMs: 15 }
    state.inbounds.push(newInbound)
    saveMockState(state)
    return delay(newInbound)
  }

  const inboundIdMatch = cleanUrl.match(/\/inbounds\/(\d+)$/)
  if (inboundIdMatch) {
    const id = parseInt(inboundIdMatch[1], 10)
    if (method === 'PUT') {
      const idx = state.inbounds.findIndex((i) => i.id === id)
      if (idx !== -1) {
        state.inbounds[idx] = { ...state.inbounds[idx], ...data }
        saveMockState(state)
        return delay(state.inbounds[idx])
      }
    }
    if (method === 'DELETE') {
      state.inbounds = state.inbounds.filter((i) => i.id !== id)
      saveMockState(state)
      return delay({ success: true })
    }
  }

  if (cleanUrl.endsWith('/inbounds/reality-keypair')) {
    return delay({
      privateKey: 'OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_' + Math.random().toString(36).substring(2, 10),
      publicKey: 'FMdWD0uS9lrXUAoMmTP5e2LLD-' + Math.random().toString(36).substring(2, 10),
      shortId: Math.random().toString(16).substring(2, 18),
    })
  }

  // 4. Outbounds 落地出口
  if (cleanUrl.endsWith('/outbounds') && method === 'GET') {
    return delay(state.outbounds)
  }

  if (cleanUrl.endsWith('/outbounds') && method === 'POST') {
    const idx = state.outbounds.findIndex((o) => o.tag === data.tag)
    if (idx !== -1) {
      state.outbounds[idx] = { ...state.outbounds[idx], ...data }
    } else {
      state.outbounds.push(data)
    }
    saveMockState(state)
    return delay(data)
  }

  const outboundTagMatch = cleanUrl.match(/\/outbounds\/([^/]+)$/)
  if (outboundTagMatch && method === 'DELETE') {
    const tag = decodeURIComponent(outboundTagMatch[1])
    state.outbounds = state.outbounds.filter((o) => o.tag !== tag)
    saveMockState(state)
    return delay({ success: true })
  }

  // 5. Routing 路由规则
  if (cleanUrl.endsWith('/routing') && method === 'GET') {
    return delay(state.routing)
  }

  if (cleanUrl.endsWith('/routing') && method === 'PUT') {
    state.routing = data
    saveMockState(state)
    return delay({ success: true })
  }

  // 6. Users 用户管理
  if (cleanUrl.endsWith('/users/speeds') && method === 'GET') {
    const speeds: Record<string, any> = {}
    for (const u of state.users) {
      speeds[u.email] = {
        email: u.email,
        upSpeed: u.isOnline ? Math.floor(800000 + Math.random() * 500000) : 0,
        downSpeed: u.isOnline ? Math.floor(6000000 + Math.random() * 3000000) : 0,
        lastActive: Date.now(),
        isOnline: u.isOnline,
      }
    }
    return delay(speeds, 40)
  }

  if (cleanUrl.endsWith('/users') && method === 'GET') {
    return delay(state.users)
  }

  if (cleanUrl.endsWith('/users') && method === 'POST') {
    const newId = state.users.reduce((max, u) => Math.max(max, u.id || 0), 0) + 1
    const subToken = Math.random().toString(36).substring(2, 18) + Math.random().toString(36).substring(2, 18)
    const user = {
      id: newId,
      email: data.email,
      uuid: '7117295b-4362-4260-a133-' + Math.random().toString(16).substring(2, 14),
      inboundTag: Array.isArray(data.inboundTags) ? data.inboundTags[0] : data.inboundTag,
      inboundTags: Array.isArray(data.inboundTags) ? data.inboundTags.join(',') : (data.inboundTags || data.inboundTag),
      flow: data.flow || '',
      subToken,
      upBytes: 1024 * 1024 * Math.floor(Math.random() * 50),
      downBytes: 1024 * 1024 * Math.floor(Math.random() * 200),
      totalBytes: data.totalBytes || 0,
      expireTime: data.expireDays > 0 ? Date.now() + data.expireDays * 86400000 : 0,
      resetDay: data.resetDay || 0,
      ipLimit: data.ipLimit || 0,
      enabled: data.enabled !== false,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      createdAt: new Date().toISOString(),
    }
    state.users.push(user)
    saveMockState(state)
    return delay(user)
  }

  // 批量延期
  if (cleanUrl.endsWith('/users/batch-renew') && method === 'POST') {
    const { ids = [], days = 30 } = data || {}
    for (const u of state.users) {
      if (ids.includes(u.id)) {
        const currentExpire = u.expireTime > Date.now() ? u.expireTime : Date.now()
        u.expireTime = currentExpire + days * 86400000
      }
    }
    saveMockState(state)
    return delay({ success: true, count: ids.length })
  }

  // 批量重置流量
  if (cleanUrl.endsWith('/users/batch-reset-traffic') && method === 'POST') {
    const { ids = [] } = data || {}
    for (const u of state.users) {
      if (ids.includes(u.id)) {
        u.upBytes = 0
        u.downBytes = 0
      }
    }
    saveMockState(state)
    return delay({ success: true, count: ids.length })
  }

  // 批量设置状态
  if (cleanUrl.endsWith('/users/batch-status') && method === 'POST') {
    const { ids = [], enabled = true } = data || {}
    for (const u of state.users) {
      if (ids.includes(u.id)) {
        u.enabled = enabled
      }
    }
    saveMockState(state)
    return delay({ success: true, count: ids.length })
  }

  // 用户详情更新与删除
  const userIdMatch = cleanUrl.match(/\/users\/(\d+)$/)
  if (userIdMatch) {
    const id = parseInt(userIdMatch[1], 10)
    if (method === 'PUT') {
      const idx = state.users.findIndex((u) => u.id === id)
      if (idx !== -1) {
        // 模拟 v1.5.0 UpdateUserDTO：绝不覆盖已累加的流量与关键系统字段
        const current = state.users[idx]
        state.users[idx] = {
          ...current,
          ...data,
          upBytes: current.upBytes,
          downBytes: current.downBytes,
          uuid: current.uuid,
          subToken: current.subToken,
          createdAt: current.createdAt,
        }
        saveMockState(state)
        return delay(state.users[idx])
      }
    }
    if (method === 'DELETE') {
      state.users = state.users.filter((u) => u.id !== id)
      saveMockState(state)
      return delay({ success: true })
    }
  }

  // 历史流量趋势 (Traffic History)
  const trafficHistoryMatch = cleanUrl.match(/\/users\/(\d+)\/traffic-history$/)
  if (trafficHistoryMatch) {
    const days = parseInt(new URL(url, 'http://localhost').searchParams.get('days') || '14', 10)
    const history: any[] = []
    const today = new Date()
    for (let i = 0; i < days; i++) {
      const d = new Date(today.getTime() - i * 86400000)
      const dateStr = d.toISOString().split('T')[0]
      const baseDown = Math.floor((120 + (i % 5) * 80 + Math.sin(i * 1.5) * 60) * 1024 * 1024)
      const baseUp = Math.floor((25 + (i % 3) * 20 + Math.cos(i * 1.2) * 15) * 1024 * 1024)
      history.push({
        id: i + 1,
        date: dateStr,
        upBytes: baseUp,
        downBytes: baseDown,
        totalBytes: baseUp + baseDown,
      })
    }
    return delay(history)
  }

  // 获取多节点订阅与分享链接 (基于 VLESS Route ID 动态 UUID)
  const userSubMatch = cleanUrl.match(/\/users\/(\d+)\/share$/)
  if (userSubMatch) {
    const id = parseInt(userSubMatch[1], 10)
    const user = state.users.find((u) => u.id === id)
    const links: string[] = []

    const userInboundTags = (user?.inboundTags || user?.inboundTag || '').split(',').map((s: string) => s.trim())
    const assignedInbounds = state.inbounds.filter((inb) => userInboundTags.includes(inb.tag))

    for (const inb of assignedInbounds) {
      let subRoutes: any[] = []
      try {
        subRoutes = JSON.parse(inb.subRoutesJson || '[]')
      } catch {}

      const enabledSubRoutes = subRoutes.filter((sr: any) => sr.enabled && sr.routeId > 0)
      if (enabledSubRoutes.length > 0) {
        for (const sr of enabledSubRoutes) {
          const routeUuid = applyRouteIdToUuid(user?.uuid || 'uuid', sr.routeId)
          const remark = sr.remark || `${inb.tag}-${sr.routeId}`
          links.push(
            `vless://${routeUuid}@demo.example.com:${inb.externalPort || inb.port || 443}?security=reality&sni=www.titech.ac.jp&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=tcp&flow=${user?.flow || 'xtls-rprx-vision'}#${encodeURIComponent(remark)}`
          )
        }
      } else {
        links.push(
          `vless://${user?.uuid || 'uuid'}@demo.example.com:${inb.externalPort || inb.port || 443}?security=reality&sni=www.titech.ac.jp&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=tcp&flow=${user?.flow || 'xtls-rprx-vision'}#${encodeURIComponent(inb.tag)}`
        )
      }
    }

    if (!links.length) {
      links.push(
        `vless://${user?.uuid || 'uuid'}@demo.example.com:443?security=reality&sni=www.titech.ac.jp&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=tcp&flow=xtls-rprx-vision#🇯🇵+日本东京原生+(直连出口)`
      )
    }

    return delay({
      token: user?.subToken || 'mock-token',
      subUrl: `${window.location.origin}${window.location.pathname}#/sub/${user?.subToken || 'mock-token'}`,
      links,
    })
  }

  if (cleanUrl.match(/\/users\/(\d+)\/reset-token$/)) {
    return delay({ subToken: Math.random().toString(36).substring(2, 18) })
  }

  if (cleanUrl.match(/\/users\/(\d+)\/reset-traffic$/)) {
    const id = parseInt(cleanUrl.match(/\/users\/(\d+)\/reset-traffic$/)![1], 10)
    const user = state.users.find((u) => u.id === id)
    if (user) {
      user.upBytes = 0
      user.downBytes = 0
      saveMockState(state)
    }
    return delay({ success: true })
  }

  // 7. DNS
  if (cleanUrl.endsWith('/dns') && method === 'GET') {
    return delay(state.dns)
  }
  if (cleanUrl.endsWith('/dns') && method === 'PUT') {
    state.dns = data
    saveMockState(state)
    return delay({ success: true })
  }

  // 8. Logs
  if (cleanUrl.endsWith('/logs') && method === 'GET') {
    const nowStr = new Date().toISOString().replace('T', ' ').substring(0, 19).replace(/-/g, '/')
    const randomLogs = [
      `${nowStr} 127.0.0.1:4${Math.floor(1000 + Math.random() * 9000)} accepted tcp:www.youtube.com:443 [vless-reality -> direct] email: master@yezineko.top`,
      `${nowStr} 127.0.0.1:4${Math.floor(1000 + Math.random() * 9000)} accepted tcp:api.openai.com:443 [vless-reality -> warp-out] email: master@yezineko.top`,
      `${nowStr} 127.0.0.1:4${Math.floor(1000 + Math.random() * 9000)} accepted tcp:hk-node.example.com:443 [vless-reality -> hk-landing] email: master@yezineko.top`,
      `${nowStr} [Info] app/proxyman/inbound: inbound connection accepted on port 4434`,
    ]
    return delay({
      lines: [...randomLogs, ...state.logs],
    })
  }

  // 9. Config JSON
  if (cleanUrl.endsWith('/config/raw') && method === 'GET') {
    return delay({
      raw: JSON.stringify(
        {
          inbounds: state.inbounds,
          outbounds: state.outbounds,
          routing: state.routing,
          dns: state.dns,
        },
        null,
        2
      ),
    })
  }

  // 默认返回成功
  return delay({ success: true })
}
