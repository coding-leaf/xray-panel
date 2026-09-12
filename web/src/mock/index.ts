// Mock 拦截请求调度中心 (v3 - gRPC 运行时架构)

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

// 辅助函数：安全 Base64 编码（支持 Unicode 字符串，防止 btoa 异常）
function safeBtoa(str: string): string {
  try {
    return btoa(encodeURIComponent(str).replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16))))
  } catch {
    return btoa(str)
  }
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
    const totalTraffic = state.users.reduce((acc, u) => acc + (u.upBytes || 0) + (u.downBytes || 0), 0)
    const totalUp = state.users.reduce((acc, u) => acc + (u.upBytes || 0), 0)
    const totalDown = state.users.reduce((acc, u) => acc + (u.downBytes || 0), 0)
    const activeUsers = state.users.filter((u) => u.isOnline).length
    const netUpSpeed = Math.round(1024 * (1200 + Math.random() * 300))
    const netDownSpeed = Math.round(1024 * (8450 + Math.random() * 1200))

    return delay({
      metrics: {
        cpuUsagePercent: Math.round(18 + Math.random() * 8),
        memoryUsagePercent: 37.5,
        memoryUsedBytes: 384 * 1024 * 1024,
        memoryTotalBytes: 1024 * 1024 * 1024,
        diskUsagePercent: 30.0,
        diskUsedBytes: 12 * 1024 * 1024 * 1024,
        diskTotalBytes: 40 * 1024 * 1024 * 1024,
        netUpSpeedBps: netUpSpeed,
        netDownSpeedBps: netDownSpeed,
        netTotalSent: totalUp,
        netTotalRecv: totalDown,
        uptimeSeconds: 48600,
        xrayRunning: true,
        xrayVersion: 'Xray 26.3.27 (gRPC Runtime) Linux/amd64',
      },
      service: {
        active: true,
        subState: 'running',
      },
      userCount: state.users.length,
      activeUsers,
      inbounds: state.inbounds,
      totalUp,
      totalDown,
      xrayStatus: {
        running: true,
        version: 'Xray 26.3.27 (gRPC Runtime) Linux/amd64',
        uptimeSecs: 48600,
        xrayPid: 12345,
      },
      hostStatus: {
        cpuPercent: Math.round(18 + Math.random() * 8),
        memUsedBytes: 384 * 1024 * 1024,
        memTotalBytes: 1024 * 1024 * 1024,
        diskUsedBytes: 12 * 1024 * 1024 * 1024,
        diskTotalBytes: 40 * 1024 * 1024 * 1024,
        netInSpeed: netUpSpeed,
        netOutSpeed: netDownSpeed,
      },
      stats: {
        totalUsers: state.users.length,
        onlineUsers: activeUsers,
        totalInbounds: state.inbounds.length,
        totalOutbounds: state.outbounds.length,
        totalTrafficBytes: totalTraffic,
      },
    })
  }

  // 2.1 Service 核心状态与控制
  if (cleanUrl.endsWith('/service/status') && method === 'GET') {
    return delay({
      active: true,
      subState: 'running',
      version: 'Xray 26.3.27 (gRPC Runtime) Linux/amd64',
    })
  }

  if (cleanUrl.endsWith('/service/restart') && method === 'POST') {
    return delay({ success: true, message: 'Core restarted' })
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

  if (cleanUrl.endsWith('/routing') && (method === 'POST' || method === 'PUT')) {
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
        // 模拟 UpdateUserDTO：绝不覆盖已累加的流量与关键系统字段
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

  // 获取多节点订阅与分享链接 (支持 VLESS Vision、VMess、Trojan、Shadowsocks 等多协议)
  const userSubMatch = cleanUrl.match(/\/users\/(\d+)\/share$/)
  if (userSubMatch) {
    const id = parseInt(userSubMatch[1], 10)
    const user = state.users.find((u) => u.id === id)
    const links: string[] = []

    const userInboundTags = (user?.inboundTags || user?.inboundTag || '').split(',').map((s: string) => s.trim())
    const assignedInbounds = state.inbounds.filter((inb) => userInboundTags.includes(inb.tag))

    for (const inb of assignedInbounds) {
      const proto = (inb.protocol || 'vless').toLowerCase()
      const extHost = inb.externalHost || 'demo.example.com'
      const port = inb.externalPort || inb.port || 443

      if (proto === 'vless') {
        let network = 'tcp'
        let security = 'none'
        try {
          const stream = JSON.parse(inb.streamSettings || '{}')
          if (stream.network) network = stream.network.toLowerCase()
          if (stream.security) security = stream.security.toLowerCase()
        } catch {}

        const isTcpVision = (network === 'tcp' || network === '') && (security === 'reality' || security === 'tls')
        const flowParam = isTcpVision ? `&flow=${user?.flow || 'xtls-rprx-vision'}` : ''

        let subRoutes: any[] = []
        try {
          subRoutes = JSON.parse(inb.subRoutesJson || '[]')
        } catch {}

        const enabledSubRoutes = subRoutes.filter((sr: any) => sr.enabled && sr.routeId > 0)
        if (enabledSubRoutes.length > 0) {
          for (const sr of enabledSubRoutes) {
            const routeUuid = applyRouteIdToUuid(user?.uuid || 'uuid', sr.routeId)
            const remark = sr.name || sr.remark || `${inb.tag}-${sr.routeId}`
            links.push(
              `vless://${routeUuid}@${extHost}:${port}?security=reality&sni=www.example.com&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=${network}${flowParam}#${encodeURIComponent(remark)}`
            )
          }
        } else {
          links.push(
            `vless://${user?.uuid || 'uuid'}@${extHost}:${port}?security=reality&sni=www.example.com&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=${network}${flowParam}#${encodeURIComponent(inb.tag)}`
          )
        }
      } else if (proto === 'vmess') {
        const vmessObj = {
          v: '2',
          ps: `${inb.tag} (VMess WS)`,
          add: extHost,
          port: port,
          id: user?.uuid || 'uuid',
          aid: 0,
          net: 'ws',
          type: 'none',
          host: extHost,
          path: '/vmess',
          tls: 'none',
        }
        links.push(`vmess://${safeBtoa(JSON.stringify(vmessObj))}`)
      } else if (proto === 'trojan') {
        links.push(
          `trojan://${user?.uuid || 'password'}@${extHost}:${port}?security=reality&sni=gateway.icloud.com&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=tcp#${encodeURIComponent(inb.tag)}`
        )
      } else if (proto === 'shadowsocks' || proto === 'ss') {
        const ssAuth = safeBtoa(`2022-blake3-aes-128-gcm:${user?.uuid || 'password'}`)
        links.push(`ss://${ssAuth}@${extHost}:${port}#${encodeURIComponent(inb.tag)}`)
      }
    }

    if (!links.length) {
      links.push(
        `vless://${user?.uuid || 'uuid'}@demo.example.com:443?security=reality&sni=www.example.com&fp=chrome&pbk=FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww&sid=0123456789abcdef&type=tcp&flow=xtls-rprx-vision#🇯🇵+日本东京原生+(直连出口)`
      )
    }

    return delay({
      token: user?.subToken || 'mock-token',
      subUrl: `${window.location.origin}${window.location.pathname}#/sub/${user?.subToken || 'mock-token'}`,
      links,
    })
  }

  // 重置订阅 Token
  const resetTokenMatch = cleanUrl.match(/\/users\/(\d+)\/reset-token$/)
  if (resetTokenMatch && method === 'POST') {
    const id = parseInt(resetTokenMatch[1], 10)
    const newToken = Math.random().toString(36).substring(2, 18) + Math.random().toString(36).substring(2, 18)
    const user = state.users.find((u) => u.id === id)
    if (user) {
      user.subToken = newToken
      saveMockState(state)
    }
    return delay({ subToken: newToken })
  }

  // 重置单用户已用流量
  const resetTrafficMatch = cleanUrl.match(/\/users\/(\d+)\/reset-traffic$/)
  if (resetTrafficMatch && method === 'POST') {
    const id = parseInt(resetTrafficMatch[1], 10)
    const user = state.users.find((u) => u.id === id)
    if (user) {
      user.upBytes = 0
      user.downBytes = 0
      saveMockState(state)
    }
    return delay({ success: true })
  }

  // 生成带外提取码 (Ticket)
  const userTicketMatch = cleanUrl.match(/\/users\/(\d+)\/tickets$/)
  if (userTicketMatch && method === 'POST') {
    const id = parseInt(userTicketMatch[1], 10)
    const ttl = data?.ttl_minutes || 15
    const maxUses = data?.max_uses || 2
    const code = '7K9X2P'
    const expiresAt = Math.floor(Date.now() / 1000) + ttl * 60
    const host = window.location.host
    const portalUrl = `${window.location.protocol}//${host}/portal`
    const shareText = `【安全数据交换】\n提取码：${code}\n门户：${portalUrl}\n有效时间：${ttl}分钟（限${maxUses}次提取）`
    return delay({
      code,
      user_id: id,
      remaining_uses: maxUses,
      expires_at: expiresAt,
      share_text: shareText,
    })
  }

  // 提取带外安全凭据 (Claim Ticket)
  if (cleanUrl.endsWith('/portal/claim') && method === 'POST') {
    const code = (data?.code || '').toUpperCase().trim()
    if (!code) {
      throw new Error('提取码不能为空')
    }
    const host = window.location.host
    const protocol = window.location.protocol
    const demoUser = state.users[0] || { email: 'demo@example.com', subToken: 'demo-sub-token' }
    return delay({
      user_email: demoUser.email,
      emergency_nodes: [
        `vless://11111111-2222-3333-4444-555555555555@${host.split(':')[0]}:443?encryption=none&security=reality&sni=www.example.com&fp=chrome&pbk=1111111111111111111111111111111111111111111&sid=12345678&type=tcp#%E6%80%A5%E6%95%91%E8%8A%82%E7%82%B9-01`,
        `vless://11111111-2222-3333-4444-555555555555@${host.split(':')[0]}:443?encryption=none&security=reality&sni=www.example.com&fp=chrome&pbk=1111111111111111111111111111111111111111111&sid=12345678&type=tcp#%E6%80%A5%E6%95%91%E8%8A%82%E7%82%B9-02`,
      ],
      subscription_url: `${protocol}//${host}/sub/${demoUser.subToken}`,
      remaining_uses: 1,
      expires_at: Math.floor(Date.now() / 1000) + 900,
    })
  }

  // 7. DNS
  if (cleanUrl.endsWith('/dns') && method === 'GET') {
    return delay(state.dns)
  }
  if (cleanUrl.endsWith('/dns') && (method === 'POST' || method === 'PUT')) {
    state.dns = data
    saveMockState(state)
    return delay({ success: true })
  }

  // 8. Logs & Audit
  if (cleanUrl.endsWith('/logs/clear') && method === 'POST') {
    state.logs = []
    if (!state.auditLogs) state.auditLogs = []
    const logType = url.includes('type=error') ? 'error' : 'access'
    state.auditLogs.unshift({
      id: Date.now(),
      createdAt: new Date().toISOString(),
      operator: 'admin',
      clientIp: '127.0.0.1',
      action: 'LOG_CLEAR',
      target: logType,
      details: '清空日志文件: ' + logType,
      status: 'SUCCESS',
    })
    saveMockState(state)
    return delay({ success: true })
  }

  if (cleanUrl.endsWith('/logs') && method === 'GET') {
    const urlObj = new URL(url, 'http://localhost')
    const logType = urlObj.searchParams.get('type') || 'access'
    const inbound = urlObj.searchParams.get('inbound') || ''
    const keyword = (urlObj.searchParams.get('keyword') || '').toLowerCase()
    const nowStr = new Date().toISOString().replace('T', ' ').substring(0, 19).replace(/-/g, '/')

    let access = [
      {
        time: nowStr,
        from_ip: '192.168.1.100:54321',
        protocol: 'tcp',
        target: 'www.google.com:443',
        route: 'vless-reality -> direct',
        inbound_tag: 'vless-reality',
        outbound_tag: 'direct',
        email: 'master@example.com',
        action: 'accepted',
        raw: `${nowStr} 192.168.1.100:54321 accepted tcp:www.google.com:443 [vless-reality -> direct] email: master@example.com`,
      },
      {
        time: nowStr,
        from_ip: '192.168.1.101:54322',
        protocol: 'tcp',
        target: 'api.openai.com:443',
        route: 'vless-reality -> warp-out',
        inbound_tag: 'vless-reality',
        outbound_tag: 'warp-out',
        email: 'master@example.com',
        action: 'accepted',
        raw: `${nowStr} 192.168.1.101:54322 accepted tcp:api.openai.com:443 [vless-reality -> warp-out] email: master@example.com`,
      },
      {
        time: nowStr,
        from_ip: '192.168.1.102:54323',
        protocol: 'tcp',
        target: 'speedtest.net:443',
        route: 'vmess-ws -> direct',
        inbound_tag: 'vmess-ws',
        outbound_tag: 'direct',
        email: 'test@example.com',
        action: 'accepted',
        raw: `${nowStr} 192.168.1.102:54323 accepted tcp:speedtest.net:443 [vmess-ws -> direct] email: test@example.com`,
      },
      {
        time: nowStr,
        from_ip: '192.168.1.103:54324',
        protocol: 'tcp',
        target: 'hk-node.example.com:443',
        route: 'vless-reality -> hk-landing',
        inbound_tag: 'vless-reality',
        outbound_tag: 'hk-landing',
        email: 'master@example.com',
        action: 'accepted',
        raw: `${nowStr} 192.168.1.103:54324 accepted tcp:hk-node.example.com:443 [vless-reality -> hk-landing] email: master@example.com`,
      },
    ]

    if (inbound) {
      access = access.filter((a) => a.inbound_tag === inbound)
    }
    if (keyword) {
      access = access.filter(
        (a) =>
          a.target.toLowerCase().includes(keyword) ||
          a.from_ip.includes(keyword) ||
          a.email.toLowerCase().includes(keyword) ||
          a.route.toLowerCase().includes(keyword)
      )
    }

    let errors = [
      {
        time: nowStr,
        level: 'WARN',
        module: 'proxy/vless',
        message: 'client flow is empty',
        smartTip: '客户端未配置 Vision 流控，或当前入站协议不为 TCP',
        raw: `${nowStr} [Warning] proxy/vless: client flow is empty`,
      },
      {
        time: nowStr,
        level: 'INFO',
        module: 'app/dispatcher',
        message: 'default route matched',
        smartTip: '',
        raw: `${nowStr} [Info] app/dispatcher: default route matched`,
      },
      {
        time: nowStr,
        level: 'ERROR',
        module: 'app/dns',
        message: 'dns-query failed: context canceled',
        smartTip: 'DoH 远端解析握手超时，建议优先使用 8.8.8.8 UDP DNS',
        raw: `${nowStr} [Error] app/dns: dns-query failed: context canceled`,
      },
    ]

    if (keyword) {
      errors = errors.filter(
        (e) =>
          e.message.toLowerCase().includes(keyword) ||
          e.module.toLowerCase().includes(keyword) ||
          (e.smartTip && e.smartTip.toLowerCase().includes(keyword))
      )
    }

    const lines = logType === 'access' ? access.map((a) => a.raw) : errors.map((e) => e.raw)

    return delay({
      access,
      errors,
      lines: [...lines, ...state.logs],
    })
  }

  // 8.1 Audit Logs
  if (cleanUrl.endsWith('/audit-logs') && method === 'GET') {
    const urlObj = new URL(url, 'http://localhost')
    const page = parseInt(urlObj.searchParams.get('page') || '1', 10)
    const pageSize = parseInt(urlObj.searchParams.get('pageSize') || '50', 10)
    const action = urlObj.searchParams.get('action') || ''
    const operator = urlObj.searchParams.get('operator') || ''
    const keyword = (urlObj.searchParams.get('keyword') || '').toLowerCase()

    let list = (state.auditLogs || []).slice()
    if (action) {
      list = list.filter((i: any) => i.action === action)
    }
    if (operator) {
      list = list.filter((i: any) => i.operator === operator)
    }
    if (keyword) {
      list = list.filter(
        (i: any) =>
          (i.action && i.action.toLowerCase().includes(keyword)) ||
          (i.operator && i.operator.toLowerCase().includes(keyword)) ||
          (i.target && i.target.toLowerCase().includes(keyword)) ||
          (i.details && i.details.toLowerCase().includes(keyword))
      )
    }

    const total = list.length
    const start = (page - 1) * pageSize
    const items = list.slice(start, start + pageSize)
    return delay({ items, total, page, pageSize })
  }

  if (cleanUrl.endsWith('/audit-logs') && method === 'DELETE') {
    state.auditLogs = [
      {
        id: Date.now(),
        createdAt: new Date().toISOString(),
        operator: 'admin',
        clientIp: '127.0.0.1',
        action: 'AUDIT_CLEAR',
        target: 'all',
        details: '管理员清空历史操作审查日志',
        status: 'SUCCESS',
      },
    ]
    saveMockState(state)
    return delay({ success: true })
  }

  // 9. Config JSON 与快照
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

  if (cleanUrl.endsWith('/config/snapshots') && method === 'GET') {
    return delay(state.snapshots || [])
  }

  const rollbackMatch = cleanUrl.match(/\/config\/snapshots\/(\d+)\/rollback$/)
  if (rollbackMatch && method === 'POST') {
    const id = parseInt(rollbackMatch[1], 10)
    const snap = (state.snapshots || []).find((s: any) => s.id === id)
    if (snap && snap.content) {
      try {
        const parsed = JSON.parse(snap.content)
        if (parsed.inbounds) state.inbounds = parsed.inbounds
        if (parsed.outbounds) state.outbounds = parsed.outbounds
        if (parsed.routing) state.routing = parsed.routing
        if (parsed.dns) state.dns = parsed.dns
        saveMockState(state)
      } catch {}
    }
    return delay({ success: true })
  }

  if (cleanUrl.endsWith('/config/validate') && method === 'POST') {
    try {
      if (typeof data === 'string') {
        JSON.parse(data)
      }
      return delay({ valid: true, message: 'Official Xray-core 26.x syntax pre-check 100% passed.' })
    } catch (e: any) {
      return delay({ valid: false, message: 'JSON 语法错误: ' + (e?.message || 'Invalid syntax') })
    }
  }

  if (cleanUrl.endsWith('/config/save') && method === 'POST') {
    try {
      let parsed = data
      if (typeof data === 'string') {
        parsed = JSON.parse(data)
      }
      if (parsed.inbounds) state.inbounds = parsed.inbounds
      if (parsed.outbounds) state.outbounds = parsed.outbounds
      if (parsed.routing) state.routing = parsed.routing
      if (parsed.dns) state.dns = parsed.dns

      const snaps = state.snapshots || []
      const newSnapId = snaps.reduce((max: number, s: any) => Math.max(max, s.id || 0), 0) + 1
      snaps.unshift({
        id: newSnapId,
        remark: 'Web 面板在线保存快照',
        createdAt: new Date().toISOString(),
        content: typeof data === 'string' ? data : JSON.stringify(data, null, 2),
      })
      state.snapshots = snaps.slice(0, 10)
      saveMockState(state)
      return delay({ success: true })
    } catch (e: any) {
      return delay({ success: false, message: '配置保存失败: ' + e?.message }, 200)
    }
  }

  // 10. Settings 系统设置与 2FA
  if (cleanUrl.endsWith('/settings') && method === 'GET') {
    return delay(state.settings || {})
  }

  if (cleanUrl.endsWith('/settings') && method === 'POST') {
    state.settings = { ...state.settings, ...data }
    saveMockState(state)
    return delay({ success: true })
  }

  if (cleanUrl.endsWith('/settings/test-telegram') && method === 'POST') {
    return delay({ success: true, message: 'Telegram 测试消息已成功发送至管理员客户端' })
  }

  if (cleanUrl.endsWith('/auth/info') && method === 'GET') {
    return delay({
      username: 'admin',
      totpEnabled: !!state.settings?.totpEnabled,
    })
  }

  if (cleanUrl.endsWith('/auth/change-password') && method === 'POST') {
    return delay({ success: true })
  }

  if (cleanUrl.endsWith('/auth/2fa/setup') && method === 'GET') {
    return delay({
      secret: 'JBSWY3DPEHPK3PXP',
      otpauthUrl: 'otpauth://totp/XrayPanel:admin?secret=JBSWY3DPEHPK3PXP&issuer=XrayPanel',
    })
  }

  if (cleanUrl.endsWith('/auth/2fa/enable') && method === 'POST') {
    if (!state.settings) state.settings = {}
    state.settings.totpEnabled = true
    saveMockState(state)
    return delay({ success: true })
  }

  if (cleanUrl.endsWith('/auth/2fa/disable') && method === 'POST') {
    if (!state.settings) state.settings = {}
    state.settings.totpEnabled = false
    saveMockState(state)
    return delay({ success: true })
  }

  // 11. GeoData 规则库
  if (cleanUrl.endsWith('/geodata/status') && method === 'GET') {
    return delay(
      state.geodata || {
        platform: 'Xray Core (gRPC Runtime)',
        geoipExists: true,
        geoipSize: 8941200,
        geositeExists: true,
        geositeSize: 23518400,
        targetDirectory: '/usr/local/share/xray',
      }
    )
  }

  if (cleanUrl.endsWith('/geodata/update') && method === 'POST') {
    return delay({ success: true, message: 'GeoData 规则库更新任务已在后台启动' })
  }

  if (cleanUrl.endsWith('/geodata/progress') && method === 'GET') {
    return delay({
      isUpdating: false,
      percentage: 100,
      step: 'done',
      message: 'GeoData 规则库已成功更新并平滑重载至 Xray-core 运行时！',
      speedBps: 2450000,
    })
  }

  // 12. Service 重启
  if (cleanUrl.endsWith('/service/restart') && method === 'POST') {
    return delay({ success: true, message: 'Xray 核心已成功平滑重启' })
  }

  // 默认返回成功
  return delay({ success: true })
}
