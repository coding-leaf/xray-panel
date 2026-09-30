export interface SubRouteItem {
  id: string
  name: string
  routeId: number
  outboundTag: string
  enabled: boolean
  allowedUsers?: string[]
}

export interface InboundItem {
  id: number
  tag: string
  port: number
  listen: string
  protocol: string
  settingsJson?: string
  streamSettings?: string
  streamSettingsJson?: string
  sniffingJson?: string
  remark?: string
  externalPort?: number
  externalHost?: string
  routeId?: number
  subRoutesJson?: string
  subRoutes?: SubRouteItem[]
  upBytes?: number
  downBytes?: number
  enabled?: boolean
  createdAt?: string
  updatedAt?: string
  latencyMs?: number
  isAlive?: boolean
}

export interface InboundFormData {
  id: number
  tag: string
  listen: string
  port: number
  externalPort: number
  externalHost: string
  routeId: number
  subRoutes: SubRouteItem[]
  protocol: string
  vlessFlow: string
  network: string
  security: string
  selectedUserEmails: string[]
  xhttpPath: string
  xhttpMode: string
  wsPath: string
  grpcService: string
  realityTarget: string
  realityServerNames: string
  realityPrivateKey: string
  realityPublicKey: string
  realityShortIds: string
  tlsServerName: string
  tlsCertFile: string
  tlsKeyFile: string
  fallbacksEnabled: boolean
  fallbackDest: string
  fallbackXver: number
  socksAuth: string
  socksUdp: boolean
  socksUsername: string
  socksPassword: string
  httpUsername: string
  httpPassword: string
  dokoAddress: string
  dokoPort: number
  dokoNetwork: string
  ssMethod: string
  ssPassword: string
  sniffingEnabled: boolean
  sniffingRouteOnly: boolean
}

export interface InboundPayload {
  id: number
  tag: string
  port: number
  externalPort: number
  externalHost: string
  routeId: number
  subRoutesJson: string
  listen: string
  protocol: string
  settingsJson: string
  streamSettings: string
  sniffingJson: string
  enabled: boolean
}

export function getDefaultInboundFormData(usersList: any[] = []): InboundFormData {
  return {
    id: 0,
    tag: 'vless-tcp',
    listen: '0.0.0.0',
    port: 443,
    externalPort: 443,
    externalHost: '',
    routeId: 0,
    subRoutes: [
      { id: '1', name: '🇯🇵 日本原生直连', routeId: 1, outboundTag: 'direct', enabled: true, allowedUsers: [] },
    ],
    protocol: 'vless',
    vlessFlow: 'xtls-rprx-vision',
    network: 'tcp',
    security: 'reality',
    selectedUserEmails: usersList.map((u) => u.email),
    xhttpPath: '/' + Math.random().toString(36).substring(2, 12),
    xhttpMode: 'auto',
    wsPath: '/ws',
    grpcService: 'xray-grpc',
    realityTarget: 'www.example.com:443',
    realityServerNames: 'www.example.com',
    realityPrivateKey: '',
    realityPublicKey: '',
    realityShortIds: '0123456789abcdef',
    tlsServerName: '',
    tlsCertFile: '',
    tlsKeyFile: '',
    fallbacksEnabled: false,
    fallbackDest: '80',
    fallbackXver: 0,
    socksAuth: 'noauth',
    socksUdp: true,
    socksUsername: '',
    socksPassword: '',
    httpUsername: '',
    httpPassword: '',
    dokoAddress: '127.0.0.1',
    dokoPort: 53,
    dokoNetwork: 'tcp,udp',
    ssMethod: '2022-blake3-aes-128-gcm',
    ssPassword: '',
    sniffingEnabled: true,
    sniffingRouteOnly: true,
  }
}

/**
 * 自动裁剪当前协议与安全设置不适用的无关字段，杜绝脏数据入库
 */
export function sanitizeInboundPayload(raw: InboundFormData): InboundFormData {
  const clean: InboundFormData = {
    id: raw.id,
    tag: (raw.tag || '').trim(),
    listen: (raw.listen || '0.0.0.0').trim(),
    port: raw.port || 443,
    externalPort: raw.externalPort || 0,
    externalHost: (raw.externalHost || '').trim(),
    routeId: raw.routeId || 0,
    protocol: raw.protocol || 'vless',
    network: raw.network || 'tcp',
    security: raw.security || 'none',
    sniffingEnabled: raw.sniffingEnabled !== false,
    sniffingRouteOnly: raw.sniffingRouteOnly === true,
    subRoutes: [],
    vlessFlow: '',
    selectedUserEmails: [],
    xhttpPath: '',
    xhttpMode: 'auto',
    wsPath: '',
    grpcService: '',
    realityTarget: '',
    realityServerNames: '',
    realityPrivateKey: '',
    realityPublicKey: '',
    realityShortIds: '',
    tlsServerName: '',
    tlsCertFile: '',
    tlsKeyFile: '',
    fallbacksEnabled: false,
    fallbackDest: '',
    fallbackXver: 0,
    socksAuth: 'noauth',
    socksUdp: true,
    socksUsername: '',
    socksPassword: '',
    httpUsername: '',
    httpPassword: '',
    dokoAddress: '127.0.0.1',
    dokoPort: 53,
    dokoNetwork: 'tcp,udp',
    ssMethod: '2022-blake3-aes-128-gcm',
    ssPassword: '',
  }

  // 1. 协议特定清洗与安全/传输约束
  if (clean.protocol === 'vless') {
    clean.subRoutes = Array.isArray(raw.subRoutes) ? [...raw.subRoutes] : []
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.fallbacksEnabled = Boolean(raw.fallbacksEnabled)
    clean.fallbackDest = raw.fallbackDest || '80'
    clean.fallbackXver = raw.fallbackXver || 0
    if (clean.network === 'tcp' && ['reality', 'tls'].includes(clean.security)) {
      clean.vlessFlow = raw.vlessFlow || 'xtls-rprx-vision'
    } else {
      clean.vlessFlow = ''
    }
  } else if (clean.protocol === 'vmess') {
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.vlessFlow = ''
    if (clean.security === 'reality') {
      clean.security = 'tls'
    }
  } else if (clean.protocol === 'trojan') {
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.fallbacksEnabled = Boolean(raw.fallbacksEnabled)
    clean.fallbackDest = raw.fallbackDest || '80'
    clean.fallbackXver = raw.fallbackXver || 0
    clean.vlessFlow = ''
  } else if (clean.protocol === 'shadowsocks') {
    clean.ssMethod = raw.ssMethod || '2022-blake3-aes-128-gcm'
    clean.ssPassword = raw.ssPassword || ''
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.security = 'none'
    clean.vlessFlow = ''
  } else if (clean.protocol === 'socks') {
    clean.socksAuth = raw.socksAuth || 'noauth'
    clean.socksUdp = raw.socksUdp !== false
    clean.socksUsername = raw.socksUsername || ''
    clean.socksPassword = raw.socksPassword || ''
    clean.network = 'tcp'
    clean.security = 'none'
    clean.vlessFlow = ''
  } else if (clean.protocol === 'http') {
    clean.httpUsername = raw.httpUsername || ''
    clean.httpPassword = raw.httpPassword || ''
    clean.network = 'tcp'
    clean.security = 'none'
    clean.vlessFlow = ''
  } else if (clean.protocol === 'dokodemo-door') {
    clean.dokoAddress = raw.dokoAddress || '127.0.0.1'
    clean.dokoPort = raw.dokoPort || 53
    clean.dokoNetwork = raw.dokoNetwork || 'tcp,udp'
    clean.network = 'tcp'
    clean.security = 'none'
    clean.vlessFlow = ''
  }

  // 2. 传输层网络清洗 (仅在支持流传输的协议下生效)
  if (!['socks', 'http', 'dokodemo-door', 'shadowsocks'].includes(clean.protocol)) {
    if (clean.network === 'xhttp') {
      clean.xhttpPath = raw.xhttpPath || '/split'
      clean.xhttpMode = raw.xhttpMode || 'auto'
    } else if (clean.network === 'ws') {
      clean.wsPath = raw.wsPath || '/ws'
    } else if (clean.network === 'grpc') {
      clean.grpcService = raw.grpcService || 'xray-grpc'
    }
  }

  // 3. 安全协议清洗 (仅在支持安全层的协议下生效)
  if (!['socks', 'http', 'dokodemo-door', 'shadowsocks'].includes(clean.protocol)) {
    if (clean.security === 'reality') {
      clean.realityTarget = raw.realityTarget || 'www.example.com:443'
      clean.realityServerNames = raw.realityServerNames || 'www.example.com'
      clean.realityPrivateKey = raw.realityPrivateKey || ''
      clean.realityPublicKey = raw.realityPublicKey || ''
      clean.realityShortIds = raw.realityShortIds || '0123456789abcdef'
    } else if (clean.security === 'tls') {
      clean.tlsServerName = raw.tlsServerName || ''
      clean.tlsCertFile = raw.tlsCertFile || ''
      clean.tlsKeyFile = raw.tlsKeyFile || ''
    }
  }

  return clean
}

export function buildSettingsJSON(clean: InboundFormData, usersList: any[] = []): string {
  const settings: any = {}

  if (clean.protocol === 'socks') {
    settings.auth = clean.socksAuth || 'noauth'
    settings.udp = clean.socksUdp !== false
    if (clean.socksAuth === 'password' && (clean.socksUsername || clean.socksPassword)) {
      settings.accounts = [
        {
          user: clean.socksUsername || '',
          pass: clean.socksPassword || '',
        },
      ]
    }
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'http') {
    if (clean.httpUsername || clean.httpPassword) {
      settings.accounts = [
        {
          user: clean.httpUsername || '',
          pass: clean.httpPassword || '',
        },
      ]
    }
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'dokodemo-door') {
    settings.address = clean.dokoAddress || '127.0.0.1'
    settings.port = clean.dokoPort || 53
    settings.network = clean.dokoNetwork || 'tcp,udp'
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'shadowsocks') {
    settings.method = clean.ssMethod || '2022-blake3-aes-128-gcm'
    settings.network = 'tcp,udp'
    if (clean.ssPassword) {
      settings.password = clean.ssPassword
    }
    const is2022 = (clean.ssMethod || '').includes('2022-blake3')
    const clients: any[] = []
    for (const email of clean.selectedUserEmails) {
      const userObj = usersList.find((u: any) => u.email === email)
      if (userObj) {
        const c: any = {
          password: userObj.uuid,
          email: userObj.email,
          level: 0,
        }
        if (!is2022) {
          c.method = clean.ssMethod || 'aes-128-gcm'
        }
        clients.push(c)
      }
    }
    if (clients.length > 0) {
      settings.clients = clients
    }
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'trojan') {
    const clients: any[] = []
    for (const email of clean.selectedUserEmails) {
      const userObj = usersList.find((u: any) => u.email === email)
      if (userObj) {
        clients.push({
          password: userObj.uuid,
          email: userObj.email,
          level: 0,
        })
      }
    }
    settings.clients = clients
    if (clean.fallbacksEnabled && clean.fallbackDest) {
      settings.fallbacks = [
        {
          dest: clean.fallbackDest,
          xver: clean.fallbackXver || 0,
        },
      ]
    }
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'vmess') {
    const clients: any[] = []
    for (const email of clean.selectedUserEmails) {
      const userObj = usersList.find((u: any) => u.email === email)
      if (userObj) {
        clients.push({
          id: userObj.uuid,
          email: userObj.email,
          level: 0,
        })
      }
    }
    settings.clients = clients
    return JSON.stringify(settings, null, 2)
  }

  // VLESS
  const isTcp = clean.network === 'tcp'
  const isTlsOrReality = clean.security === 'reality' || clean.security === 'tls'
  const flowVal = (isTcp && isTlsOrReality) ? (clean.vlessFlow || '') : ''
  settings.decryption = 'none'
  if (flowVal) {
    settings.flow = flowVal
  }

  const clients: any[] = []
  for (const email of clean.selectedUserEmails) {
    const userObj = usersList.find((u: any) => u.email === email)
    if (userObj) {
      const c: any = {
        id: userObj.uuid,
        email: userObj.email,
        level: 0,
      }
      if (flowVal) {
        c.flow = flowVal
      }
      clients.push(c)
    }
  }
  settings.clients = clients

  if (clean.fallbacksEnabled && clean.fallbackDest) {
    settings.fallbacks = [
      {
        dest: clean.fallbackDest,
        xver: clean.fallbackXver || 0,
      },
    ]
  }

  return JSON.stringify(settings, null, 2)
}

export function buildStreamSettingsJSON(clean: InboundFormData): string {
  if (['socks', 'http', 'dokodemo-door'].includes(clean.protocol)) {
    return ''
  }

  const stream: any = {
    network: clean.network,
    security: clean.security,
  }

  if (clean.network === 'xhttp') {
    stream.xhttpSettings = {
      path: clean.xhttpPath || '/',
      mode: clean.xhttpMode || 'auto',
      extra: {
        xPaddingBytes: '100-1000',
        scMaxEachPostBytes: '500000-1000000',
        scStreamUpServerSecs: '20-80',
      },
    }
  } else if (clean.network === 'ws') {
    stream.wsSettings = {
      path: clean.wsPath || '/',
    }
  } else if (clean.network === 'grpc') {
    stream.grpcSettings = {
      serviceName: clean.grpcService || 'xray-grpc',
    }
  }

  if (clean.security === 'reality') {
    const sNames = (clean.realityServerNames || '')
      .split(',')
      .map((s: string) => s.trim())
      .filter((s: string) => s)
    const sIds = (clean.realityShortIds || '')
      .split(',')
      .map((s: string) => s.trim())

    stream.realitySettings = {
      dest: clean.realityTarget || 'www.example.com:443',
      serverNames: sNames.length > 0 ? sNames : ['www.example.com'],
      privateKey: clean.realityPrivateKey,
      shortIds: sIds,
    }
  } else if (clean.security === 'tls') {
    const certs: any[] = []
    if (clean.tlsCertFile?.trim() || clean.tlsKeyFile?.trim()) {
      certs.push({
        certificateFile: clean.tlsCertFile?.trim() || '',
        keyFile: clean.tlsKeyFile?.trim() || '',
      })
    }
    stream.tlsSettings = {
      serverName: clean.tlsServerName || '',
      ...(certs.length > 0 ? { certificates: certs } : {}),
    }
  }

  return JSON.stringify(stream, null, 2)
}

export function buildSniffingJSON(clean: InboundFormData): string {
  return JSON.stringify(
    {
      enabled: clean.sniffingEnabled,
      destOverride: ['http', 'tls', 'quic'],
      routeOnly: clean.sniffingRouteOnly,
    },
    null,
    2
  )
}

export function buildInboundPayload(raw: InboundFormData, usersList: any[] = []): InboundPayload {
  const clean = sanitizeInboundPayload(raw)
  return {
    id: clean.id,
    tag: clean.tag,
    port: clean.port,
    externalPort: clean.externalPort || 0,
    externalHost: clean.externalHost || '',
    routeId: clean.routeId || 0,
    subRoutesJson: JSON.stringify(
      (clean.subRoutes || []).map((sr: any) => ({
        id: String(sr.id || Math.random().toString(36).substring(2, 9)),
        name: sr.name || '',
        routeId: Number(sr.routeId || 0),
        outboundTag: sr.outboundTag || 'direct',
        enabled: Boolean(sr.enabled),
        allowedUsers: Array.isArray(sr.allowedUsers) && sr.allowedUsers.length > 0 ? sr.allowedUsers : undefined,
      }))
    ),
    listen: clean.listen || '0.0.0.0',
    protocol: clean.protocol,
    settingsJson: buildSettingsJSON(clean, usersList),
    streamSettings: buildStreamSettingsJSON(clean),
    sniffingJson: buildSniffingJSON(clean),
    enabled: true,
  }
}
