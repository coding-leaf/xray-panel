export const CLIENT_FINGERPRINT_OPTIONS = [
  { label: '跟随系统默认设置 (推荐)', value: '' },
  { label: 'chrome', value: 'chrome' },
  { label: 'firefox', value: 'firefox' },
  { label: 'safari', value: 'safari' },
  { label: 'ios', value: 'ios' },
  { label: 'android', value: 'android' },
  { label: 'edge', value: 'edge' },
  { label: '360', value: '360' },
  { label: 'qq', value: 'qq' },
  { label: 'random', value: 'random' },
  { label: 'randomized', value: 'randomized' },
]

export interface SubRouteItem {
  id: string
  name: string
  routeId: number
  sortOrder?: number
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
  sortOrder?: number
  subRoutesJson?: string
  subRoutes?: SubRouteItem[]
  boundUserEmail?: string
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
  remark: string
  listen: string
  port: number
  externalPort: number
  externalHost: string
  routeId: number
  sortOrder: number
  subRoutes: SubRouteItem[]
  boundUserEmail: string
  protocol: string
  vlessFlow: string
  network: string
  security: string
  selectedUserEmails: string[]
  xhttpPath: string
  xhttpMode: string
  wsPath: string
  grpcService: string
  httpupgradePath: string
  httpupgradeHost: string
  kcpHeaderType: string
  kcpSeed: string
  kcpMtu: number
  kcpTti: number
  kcpUplinkCapacity: number
  kcpDownlinkCapacity: number
  kcpCongestion: boolean
  realityTarget: string
  realityServerNames: string
  realityPrivateKey: string
  realityPublicKey: string
  realityShortIds: string
  tlsServerName: string
  tlsCertFile: string
  tlsKeyFile: string
  tlsAlpn: string
  fallbacksEnabled: boolean
  fallbackDest: string
  fallbackXver: number
  fallbackAlpn: string
  fallbackPath: string
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
  sniffingDestOverride: string[]
  clientFingerprint: string
  vmessAlterId: number
  vmessDisableInsecure: boolean
  trojanPassword: string
  ssNetwork: 'tcp,udp' | 'tcp' | 'udp'
  ssIvCheck: boolean
  socksIp: string
  httpAllowTransparent: boolean
  httpTimeout: number
  dokoFollowRedirect: boolean
  dokoTimeout: number
}

export interface InboundPayload {
  id: number
  tag: string
  remark?: string
  port: number
  externalPort: number
  externalHost: string
  routeId: number
  sortOrder: number
  subRoutesJson: string
  boundUserEmail: string
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
    remark: '',
    listen: '0.0.0.0',
    port: 443,
    externalPort: 443,
    externalHost: '',
    routeId: 0,
    sortOrder: 0,
    subRoutes: [
      { id: '1', name: '🇯🇵 日本原生直连', routeId: 1, sortOrder: 0, outboundTag: 'direct', enabled: true, allowedUsers: [] },
    ],
    boundUserEmail: '',
    protocol: 'vless',
    vlessFlow: 'xtls-rprx-vision',
    network: 'tcp',
    security: 'reality',
    selectedUserEmails: usersList.map((u) => u.email),
    xhttpPath: '/' + Math.random().toString(36).substring(2, 12),
    xhttpMode: 'auto',
    wsPath: '/ws',
    grpcService: 'xray-grpc',
    httpupgradePath: '/httpupgrade',
    httpupgradeHost: '',
    kcpHeaderType: 'none',
    kcpSeed: '',
    kcpMtu: 1350,
    kcpTti: 50,
    kcpUplinkCapacity: 5,
    kcpDownlinkCapacity: 20,
    kcpCongestion: false,
    realityTarget: '',
    realityServerNames: '',
    realityPrivateKey: '',
    realityPublicKey: '',
    realityShortIds: '0123456789abcdef',
    tlsServerName: '',
    tlsCertFile: '',
    tlsKeyFile: '',
    tlsAlpn: '',
    fallbacksEnabled: false,
    fallbackDest: '80',
    fallbackXver: 0,
    fallbackAlpn: '',
    fallbackPath: '',
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
    sniffingDestOverride: ['http', 'tls', 'quic'],
    clientFingerprint: '',
    vmessAlterId: 0,
    vmessDisableInsecure: false,
    trojanPassword: '',
    ssNetwork: 'tcp,udp',
    ssIvCheck: false,
    socksIp: '127.0.0.1',
    httpAllowTransparent: false,
    httpTimeout: 300,
    dokoFollowRedirect: false,
    dokoTimeout: 300,
  }
}

/**
 * 自动裁剪当前协议与安全设置不适用的无关字段，杜绝脏数据入库
 */
export function sanitizeInboundPayload(raw: InboundFormData): InboundFormData {
  const clean: InboundFormData = {
    id: raw.id,
    tag: (raw.tag || '').trim(),
    remark: (raw.remark || '').trim(),
    listen: (raw.listen || '0.0.0.0').trim(),
    port: raw.port || 443,
    externalPort: raw.externalPort || 0,
    externalHost: (raw.externalHost || '').trim(),
    routeId: raw.routeId || 0,
    sortOrder: Number(raw.sortOrder || 0),
    boundUserEmail: (raw.boundUserEmail || '').trim(),
    protocol: raw.protocol || 'vless',
    network: raw.network || 'tcp',
    security: raw.security || 'none',
    sniffingEnabled: raw.sniffingEnabled !== false,
    sniffingRouteOnly: raw.sniffingRouteOnly === true,
    sniffingDestOverride: Array.isArray(raw.sniffingDestOverride) && raw.sniffingDestOverride.length > 0
      ? [...raw.sniffingDestOverride]
      : ['http', 'tls', 'quic'],
    subRoutes: [],
    vlessFlow: '',
    selectedUserEmails: [],
    xhttpPath: '',
    xhttpMode: 'auto',
    wsPath: '',
    grpcService: '',
    httpupgradePath: '',
    httpupgradeHost: '',
    kcpHeaderType: 'none',
    kcpSeed: '',
    kcpMtu: 1350,
    kcpTti: 50,
    kcpUplinkCapacity: 5,
    kcpDownlinkCapacity: 20,
    kcpCongestion: false,
    realityTarget: '',
    realityServerNames: '',
    realityPrivateKey: '',
    realityPublicKey: '',
    realityShortIds: '',
    tlsServerName: '',
    tlsCertFile: '',
    tlsKeyFile: '',
    tlsAlpn: '',
    fallbacksEnabled: false,
    fallbackDest: '',
    fallbackXver: 0,
    fallbackAlpn: '',
    fallbackPath: '',
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
    clientFingerprint: '',
    vmessAlterId: 0,
    vmessDisableInsecure: false,
    trojanPassword: '',
    ssNetwork: 'tcp,udp',
    ssIvCheck: false,
    socksIp: '127.0.0.1',
    httpAllowTransparent: false,
    httpTimeout: 300,
    dokoFollowRedirect: false,
    dokoTimeout: 300,
  }

  // 1. 协议特定清洗与安全/传输约束
  if (clean.protocol === 'vless') {
    clean.subRoutes = (Array.isArray(raw.subRoutes) ? raw.subRoutes : []).map((sr: any) => ({
      ...sr,
      sortOrder: Number(sr.sortOrder || 0),
    }))
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.fallbacksEnabled = Boolean(raw.fallbacksEnabled)
    clean.fallbackDest = raw.fallbackDest || '80'
    clean.fallbackXver = raw.fallbackXver || 0
    if (clean.fallbacksEnabled) {
      clean.fallbackAlpn = (raw.fallbackAlpn || '').trim()
      clean.fallbackPath = (raw.fallbackPath || '').trim()
    }
    if (clean.network === 'tcp' && ['reality', 'tls'].includes(clean.security)) {
      if (raw.vlessFlow === 'none' || raw.vlessFlow === '') {
        clean.vlessFlow = 'none'
      } else if (raw.vlessFlow) {
        clean.vlessFlow = raw.vlessFlow
      } else {
        clean.vlessFlow = 'xtls-rprx-vision'
      }
    } else {
      clean.vlessFlow = ''
    }
  } else if (clean.protocol === 'vmess') {
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.vlessFlow = ''
    clean.vmessAlterId = Number(raw.vmessAlterId) || 0
    clean.vmessDisableInsecure = Boolean(raw.vmessDisableInsecure)
    if (clean.security === 'reality') {
      clean.security = 'tls'
    }
  } else if (clean.protocol === 'trojan') {
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.trojanPassword = (raw.trojanPassword || '').trim()
    clean.fallbacksEnabled = Boolean(raw.fallbacksEnabled)
    clean.fallbackDest = raw.fallbackDest || '80'
    clean.fallbackXver = raw.fallbackXver || 0
    if (clean.fallbacksEnabled) {
      clean.fallbackAlpn = (raw.fallbackAlpn || '').trim()
      clean.fallbackPath = (raw.fallbackPath || '').trim()
    }
    clean.vlessFlow = ''
  } else if (clean.protocol === 'shadowsocks') {
    clean.ssMethod = raw.ssMethod || '2022-blake3-aes-128-gcm'
    clean.ssPassword = raw.ssPassword || ''
    clean.ssNetwork = raw.ssNetwork || 'tcp,udp'
    clean.ssIvCheck = Boolean(raw.ssIvCheck)
    clean.selectedUserEmails = Array.isArray(raw.selectedUserEmails) ? [...raw.selectedUserEmails] : []
    clean.security = 'none'
    clean.vlessFlow = ''
  } else if (clean.protocol === 'socks') {
    clean.socksAuth = raw.socksAuth || 'noauth'
    clean.socksUdp = raw.socksUdp !== false
    clean.socksUsername = raw.socksUsername || ''
    clean.socksPassword = raw.socksPassword || ''
    clean.socksIp = (raw.socksIp || '127.0.0.1').trim()
    clean.network = 'tcp'
    clean.security = 'none'
    clean.vlessFlow = ''
  } else if (clean.protocol === 'http') {
    clean.httpUsername = raw.httpUsername || ''
    clean.httpPassword = raw.httpPassword || ''
    clean.httpAllowTransparent = Boolean(raw.httpAllowTransparent)
    clean.httpTimeout = Number(raw.httpTimeout) || 300
    clean.network = 'tcp'
    clean.security = 'none'
    clean.vlessFlow = ''
  } else if (clean.protocol === 'dokodemo-door') {
    clean.dokoAddress = raw.dokoAddress || '127.0.0.1'
    clean.dokoPort = raw.dokoPort || 53
    clean.dokoNetwork = raw.dokoNetwork || 'tcp,udp'
    clean.dokoFollowRedirect = Boolean(raw.dokoFollowRedirect)
    clean.dokoTimeout = Number(raw.dokoTimeout) || 300
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
    } else if (clean.network === 'httpupgrade') {
      clean.httpupgradePath = raw.httpupgradePath || '/httpupgrade'
      clean.httpupgradeHost = (raw.httpupgradeHost || '').trim()
    } else if (clean.network === 'mkcp') {
      clean.kcpHeaderType = raw.kcpHeaderType || 'none'
      clean.kcpSeed = (raw.kcpSeed || '').trim()
      clean.kcpMtu = raw.kcpMtu || 1350
      clean.kcpTti = raw.kcpTti || 50
      clean.kcpUplinkCapacity = raw.kcpUplinkCapacity || 5
      clean.kcpDownlinkCapacity = raw.kcpDownlinkCapacity || 20
      clean.kcpCongestion = Boolean(raw.kcpCongestion)
    }
  }

  // 3. 安全协议清洗 (仅在支持安全层的协议下生效)
  if (!['socks', 'http', 'dokodemo-door', 'shadowsocks'].includes(clean.protocol)) {
    if (clean.security === 'reality') {
      clean.realityTarget = raw.realityTarget || ''
      clean.realityServerNames = raw.realityServerNames || ''
      clean.realityPrivateKey = raw.realityPrivateKey || ''
      clean.realityPublicKey = raw.realityPublicKey || ''
      clean.realityShortIds = raw.realityShortIds || '0123456789abcdef'
      clean.clientFingerprint = (raw.clientFingerprint || '').trim()
    } else if (clean.security === 'tls') {
      clean.tlsServerName = raw.tlsServerName || ''
      clean.tlsCertFile = raw.tlsCertFile || ''
      clean.tlsKeyFile = raw.tlsKeyFile || ''
      clean.tlsAlpn = (raw.tlsAlpn || '').trim()
      clean.clientFingerprint = (raw.clientFingerprint || '').trim()
    }
  }

  return clean
}

export function buildSettingsJSON(clean: InboundFormData, usersList: any[] = []): string {
  const settings: any = {}

  if (clean.protocol === 'socks') {
    settings.auth = clean.socksAuth || 'noauth'
    settings.udp = clean.socksUdp !== false
    settings.ip = (clean.socksIp || '').trim() || '127.0.0.1'
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
    settings.allowTransparent = !!clean.httpAllowTransparent
    settings.timeout = Number(clean.httpTimeout) || 300
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
    settings.followRedirect = !!clean.dokoFollowRedirect
    settings.timeout = Number(clean.dokoTimeout) || 300
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'shadowsocks') {
    settings.method = clean.ssMethod || '2022-blake3-aes-128-gcm'
    settings.network = clean.ssNetwork || 'tcp,udp'
    settings.ivCheck = !!clean.ssIvCheck
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
    if (clean.trojanPassword?.trim()) {
      settings.password = clean.trojanPassword.trim()
    }
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
      const fb: any = {
        dest: clean.fallbackDest,
        xver: clean.fallbackXver || 0,
      }
      if (clean.fallbackAlpn) {
        fb.alpn = clean.fallbackAlpn
      }
      if (clean.fallbackPath) {
        fb.path = clean.fallbackPath
      }
      settings.fallbacks = [fb]
    }
    return JSON.stringify(settings, null, 2)
  }

  if (clean.protocol === 'vmess') {
    settings.default = { alterId: Number(clean.vmessAlterId) || 0 }
    settings.disableInsecureEncryption = !!clean.vmessDisableInsecure
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
      if (flowVal && flowVal !== 'none') {
        c.flow = flowVal
      }
      clients.push(c)
    }
  }
  settings.clients = clients

  if (clean.fallbacksEnabled && clean.fallbackDest) {
    const fb: any = {
      dest: clean.fallbackDest,
      xver: clean.fallbackXver || 0,
    }
    if (clean.fallbackAlpn) {
      fb.alpn = clean.fallbackAlpn
    }
    if (clean.fallbackPath) {
      fb.path = clean.fallbackPath
    }
    settings.fallbacks = [fb]
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
  } else if (clean.network === 'httpupgrade') {
    const hu: any = {
      path: clean.httpupgradePath || '/httpupgrade',
    }
    if (clean.httpupgradeHost) {
      hu.host = clean.httpupgradeHost
    }
    stream.httpupgradeSettings = hu
  } else if (clean.network === 'mkcp') {
    const kcp: any = {
      mtu: clean.kcpMtu || 1350,
      tti: clean.kcpTti || 50,
      uplinkCapacity: clean.kcpUplinkCapacity || 5,
      downlinkCapacity: clean.kcpDownlinkCapacity || 20,
      congestion: Boolean(clean.kcpCongestion),
      header: {
        type: clean.kcpHeaderType || 'none',
      },
    }
    if (clean.kcpSeed) {
      kcp.seed = clean.kcpSeed
    }
    stream.kcpSettings = kcp
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
      dest: clean.realityTarget || '',
      serverNames: sNames,
      privateKey: clean.realityPrivateKey,
      shortIds: sIds,
      ...(clean.clientFingerprint ? { fingerprint: clean.clientFingerprint } : {}),
    }
  } else if (clean.security === 'tls') {
    const certs: any[] = []
    if (clean.tlsCertFile?.trim() || clean.tlsKeyFile?.trim()) {
      certs.push({
        certificateFile: clean.tlsCertFile?.trim() || '',
        keyFile: clean.tlsKeyFile?.trim() || '',
      })
    }
    const tlsObj: any = {
      serverName: clean.tlsServerName || '',
      ...(certs.length > 0 ? { certificates: certs } : {}),
      ...(clean.clientFingerprint ? { fingerprint: clean.clientFingerprint } : {}),
    }
    if (clean.tlsAlpn) {
      const alpnList = clean.tlsAlpn.split(',').map((s: string) => s.trim()).filter(Boolean)
      if (alpnList.length > 0) {
        tlsObj.alpn = alpnList
      }
    }
    stream.tlsSettings = tlsObj
  }

  return JSON.stringify(stream, null, 2)
}

export function buildSniffingJSON(clean: InboundFormData): string {
  const destOverride = (clean.sniffingDestOverride && clean.sniffingDestOverride.length > 0)
    ? clean.sniffingDestOverride
    : ['http', 'tls', 'quic']
  return JSON.stringify(
    {
      enabled: clean.sniffingEnabled,
      destOverride,
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
    remark: clean.remark || '',
    port: clean.port,
    externalPort: clean.externalPort || 0,
    externalHost: clean.externalHost || '',
    routeId: clean.routeId || 0,
    sortOrder: clean.sortOrder || 0,
    subRoutesJson: JSON.stringify(
      (clean.subRoutes || []).map((sr: any) => ({
        id: String(sr.id || Math.random().toString(36).substring(2, 9)),
        name: sr.name || '',
        routeId: Number(sr.routeId || 0),
        sortOrder: Number(sr.sortOrder || 0),
        outboundTag: sr.outboundTag || 'direct',
        enabled: Boolean(sr.enabled),
        allowedUsers: Array.isArray(sr.allowedUsers) && sr.allowedUsers.length > 0 ? sr.allowedUsers : undefined,
      }))
    ),
    boundUserEmail: clean.boundUserEmail || '',
    listen: clean.listen || '0.0.0.0',
    protocol: clean.protocol,
    settingsJson: buildSettingsJSON(clean, usersList),
    streamSettings: buildStreamSettingsJSON(clean),
    sniffingJson: buildSniffingJSON(clean),
    enabled: true,
  }
}
