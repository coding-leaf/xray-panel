export interface OutboundItem {
  tag: string
  protocol: string // freedom, blackhole, wireguard, vless, vmess, trojan, shadowsocks, socks, http, dns
  sendThrough?: string
  targetStrategy?: string
  settingsJson?: string
  streamSettings?: string
  mux?: string
}

export interface OutboundFormState {
  tag: string
  protocol: string
  freedomDomainStrategy: string
  blackholeResponse: string
  wgSecretKey: string
  wgPeerPublicKey: string
  wgEndpoint: string
  wgAddress: string
  proxyHost: string
  proxyPort: number
  proxyPassword: string
  proxyUsername?: string
  vmessSecurity?: string
  vmessAlterId?: number
  ssMethod?: string
  vlessFlow: string
  streamNetwork: string
  streamSecurity: string
  xhttpPath: string
  xhttpMode: string
  wsPath: string
  wsHost: string
  grpcServiceName: string
  realityServerName: string
  realityFingerprint: string
  realityPublicKey: string
  realityShortId: string
  tlsServerName: string
  tlsFingerprint: string
  realitySpiderX: string
  wgMtu: number
  wgNoKernelTun: boolean
  httpupgradePath: string
  httpupgradeHost: string
  muxEnabled: boolean
  muxConcurrency: number
  muxXudpConcurrency: number
  muxXudpProxyUDP443: 'reject' | 'allow' | 'skip'
  sendThrough: string
  targetStrategy: string
  dialerProxy: string
  rawSockopt?: Record<string, any>
}

export interface OutboundPayload {
  tag: string
  protocol: string
  settingsJson: string
  streamSettings: string
  sendThrough?: string
  targetStrategy?: string
  mux?: string
}

export interface MuxConfig {
  enabled: boolean
  concurrency: number
  xudpConcurrency: number
  xudpProxyUDP443: 'reject' | 'allow' | 'skip'
}

export function parseConcurrency(val: unknown, fallback: number): number {
  if (val === undefined || val === null || val === '') return fallback
  const n = Number(val)
  return isNaN(n) ? fallback : n
}

export function parseMuxConfig(muxStr?: string): MuxConfig | null {
  if (!muxStr || muxStr === '{}' || muxStr === 'null') return null
  try {
    const m = typeof muxStr === 'string' ? JSON.parse(muxStr) : muxStr
    if (!m || m.enabled !== true) return null
    let concurrency = parseConcurrency(m.concurrency, 8)
    if (concurrency === 0) concurrency = 8
    const xudpConcurrency = parseConcurrency(m.xudpConcurrency, 16)
    return {
      enabled: true,
      concurrency,
      xudpConcurrency,
      xudpProxyUDP443: m.xudpProxyUDP443 || 'reject',
    }
  } catch {
    return null
  }
}

export function parseMuxSummary(muxStr?: string): string {
  const m = parseMuxConfig(muxStr)
  if (!m) return ''
  const tcpText = m.concurrency === -1 ? 'TCP 关闭' : `TCP ${m.concurrency}`
  let xudpText = `XUDP ${m.xudpConcurrency}`
  if (m.xudpConcurrency === -1) {
    xudpText = 'XUDP 关闭'
  } else if (m.xudpConcurrency === 0) {
    xudpText = 'XUDP 共享 TCP'
  }
  return `${tcpText} / ${xudpText}`
}

export function parseDialerProxy(streamSettingsStr?: string): string {
  if (!streamSettingsStr) return ''
  try {
    const s =
      typeof streamSettingsStr === 'string' ? JSON.parse(streamSettingsStr) : streamSettingsStr
    return s.sockopt?.dialerProxy || ''
  } catch {
    return ''
  }
}

export function createDefaultOutboundForm(): OutboundFormState {
  return {
    tag: '',
    protocol: 'vless',
    freedomDomainStrategy: 'UseIPv4',
    blackholeResponse: 'none',
    wgSecretKey: '',
    wgPeerPublicKey: '',
    wgEndpoint: '162.159.192.1:2408',
    wgAddress: '172.16.0.2/32',
    proxyHost: '',
    proxyPort: 443,
    proxyPassword: '',
    proxyUsername: '',
    vmessSecurity: 'auto',
    vmessAlterId: 0,
    ssMethod: '2022-blake3-aes-128-gcm',
    vlessFlow: 'xtls-rprx-vision',
    streamNetwork: 'xhttp',
    streamSecurity: 'reality',
    xhttpPath: '/mbqyfa4grswh5ntz',
    xhttpMode: 'auto',
    wsPath: '/ws',
    wsHost: '',
    grpcServiceName: 'xray-grpc',
    realityServerName: '',
    realityFingerprint: 'chrome',
    realityPublicKey: '',
    realityShortId: '0123456789abcdef',
    tlsServerName: '',
    tlsFingerprint: 'chrome',
    realitySpiderX: '',
    wgMtu: 1280,
    wgNoKernelTun: true,
    httpupgradePath: '/httpupgrade',
    httpupgradeHost: '',
    muxEnabled: false,
    muxConcurrency: 8,
    muxXudpConcurrency: 16,
    muxXudpProxyUDP443: 'reject',
    sendThrough: '',
    targetStrategy: '',
    dialerProxy: '',
    rawSockopt: undefined,
  }
}

export function populateOutboundForm(ob: OutboundItem): OutboundFormState {
  const form = createDefaultOutboundForm()
  form.tag = ob.tag || ''
  form.protocol = ob.protocol || 'vless'
  form.sendThrough = ob.sendThrough || ''
  form.targetStrategy = ob.targetStrategy === 'AsIs' ? '' : ob.targetStrategy || ''

  if (ob.mux) {
    try {
      const m = typeof ob.mux === 'string' ? JSON.parse(ob.mux) : ob.mux
      if (m && typeof m === 'object') {
        form.muxEnabled = m.enabled === true
        if (m.concurrency !== undefined) {
          const c = parseConcurrency(m.concurrency, 8)
          form.muxConcurrency = c === 0 ? 8 : c
        }
        if (m.xudpConcurrency !== undefined) {
          form.muxXudpConcurrency = parseConcurrency(m.xudpConcurrency, 16)
        }
        if (m.xudpProxyUDP443) form.muxXudpProxyUDP443 = m.xudpProxyUDP443
      }
    } catch {
      form.muxEnabled = false
    }
  }

  let s: Record<string, any>
  try {
    s = JSON.parse(ob.settingsJson || '{}')
  } catch {
    s = {}
  }

  let str: Record<string, any>
  try {
    str = JSON.parse(ob.streamSettings || '{}')
  } catch {
    str = {}
  }

  if (str.sockopt && typeof str.sockopt === 'object') {
    const { dialerProxy, ...restSockopt } = str.sockopt
    form.dialerProxy = dialerProxy || ''
    if (ob.protocol === 'freedom' && 'domainStrategy' in restSockopt) {
      delete restSockopt.domainStrategy
    }
    form.rawSockopt = Object.keys(restSockopt).length > 0 ? restSockopt : undefined
  } else {
    form.dialerProxy = ''
    form.rawSockopt = undefined
  }

  form.streamNetwork = str.network || 'tcp'
  form.streamSecurity = str.security || 'none'
  if (ob.protocol === 'freedom') {
    form.freedomDomainStrategy =
      s.domainStrategy ||
      str.sockopt?.domainStrategy ||
      (ob.targetStrategy && ob.targetStrategy !== 'AsIs' ? ob.targetStrategy : '') ||
      'UseIPv4'
    form.targetStrategy = ''
  } else {
    form.freedomDomainStrategy = s.domainStrategy || 'UseIPv4'
  }
  form.blackholeResponse = s.response?.type || 'none'

  if (['freedom', 'blackhole', 'wireguard', 'socks', 'http', 'dns'].includes(ob.protocol)) {
    form.streamNetwork = 'tcp'
    form.streamSecurity = 'none'
    form.vlessFlow = ''
  } else if (['vmess', 'shadowsocks'].includes(ob.protocol) && form.streamSecurity === 'reality') {
    form.streamSecurity = 'none'
  }

  if (str.xhttpSettings) {
    form.xhttpPath = str.xhttpSettings.path || '/mbqyfa4grswh5ntz'
    form.xhttpMode = str.xhttpSettings.mode || 'auto'
  }
  if (str.wsSettings) {
    form.wsPath = str.wsSettings.path || '/ws'
    form.wsHost = str.wsSettings.headers?.Host || ''
  }
  if (str.grpcSettings) {
    form.grpcServiceName = str.grpcSettings.serviceName || 'xray-grpc'
  }
  if (str.realitySettings) {
    form.realityServerName = str.realitySettings.serverName || ''
    form.realityPublicKey = str.realitySettings.publicKey || ''
    form.realityShortId = str.realitySettings.shortId || ''
    form.realityFingerprint = str.realitySettings.fingerprint || 'chrome'
    form.realitySpiderX = str.realitySettings.spiderX || ''
  }
  if (str.tlsSettings) {
    form.tlsServerName = str.tlsSettings.serverName || ''
    form.tlsFingerprint = str.tlsSettings.fingerprint || 'chrome'
  }
  if (str.httpupgradeSettings) {
    form.httpupgradePath = str.httpupgradeSettings.path || '/httpupgrade'
    form.httpupgradeHost = str.httpupgradeSettings.host || ''
  }

  if (ob.protocol === 'wireguard') {
    form.wgSecretKey = s.secretKey || ''
    form.wgAddress = (s.address || ['172.16.0.2/32'])[0]
    form.wgMtu = s.mtu !== undefined ? s.mtu : 1280
    form.wgNoKernelTun = s.noKernelTun !== undefined ? s.noKernelTun : true
    if (s.peers?.length > 0) {
      form.wgEndpoint = s.peers[0].endpoint || ''
      form.wgPeerPublicKey = s.peers[0].publicKey || ''
    }
  } else if (['vless', 'vmess'].includes(ob.protocol)) {
    if (s.vnext?.length > 0) {
      form.proxyHost = s.vnext[0].address || ''
      form.proxyPort = s.vnext[0].port || 443
      if (s.vnext[0].users?.length > 0) {
        const u = s.vnext[0].users[0]
        form.proxyPassword = u.id || u.password || ''
        if (ob.protocol === 'vless') {
          const flowCandidate = s.flow !== undefined ? s.flow : u.flow
          if (flowCandidate !== undefined) {
            form.vlessFlow =
              flowCandidate === '' || flowCandidate === 'none' ? 'none' : flowCandidate
          } else {
            const isTcp = form.streamNetwork === 'tcp'
            const isTlsOrReality = ['reality', 'tls'].includes(form.streamSecurity)
            form.vlessFlow = isTcp && isTlsOrReality ? 'xtls-rprx-vision' : 'none'
          }
        } else if (ob.protocol === 'vmess') {
          form.vmessSecurity = u.security || 'auto'
          form.vmessAlterId = u.alterId || 0
        }
      }
    }
  } else if (ob.protocol === 'trojan') {
    if (s.servers?.length > 0) {
      form.proxyHost = s.servers[0].address || ''
      form.proxyPort = s.servers[0].port || 443
      form.proxyPassword = s.servers[0].password || ''
    }
  } else if (ob.protocol === 'shadowsocks') {
    if (s.servers?.length > 0) {
      form.proxyHost = s.servers[0].address || ''
      form.proxyPort = s.servers[0].port || 443
      form.proxyPassword = s.servers[0].password || ''
      form.ssMethod = s.servers[0].method || '2022-blake3-aes-128-gcm'
    }
  } else if (['socks', 'http'].includes(ob.protocol)) {
    if (s.servers?.length > 0) {
      form.proxyHost = s.servers[0].address || ''
      form.proxyPort = s.servers[0].port || 1080
      if (s.servers[0].users?.length > 0) {
        form.proxyUsername = s.servers[0].users[0].user || ''
        form.proxyPassword = s.servers[0].users[0].pass || ''
      }
    }
  }

  return form
}

export function buildSettingsJSON(form: OutboundFormState): string {
  if (form.protocol === 'freedom') {
    return JSON.stringify({
      domainStrategy: form.freedomDomainStrategy || 'UseIPv4',
    })
  } else if (form.protocol === 'blackhole') {
    return JSON.stringify({
      response: {
        type: form.blackholeResponse || 'none',
      },
    })
  } else if (form.protocol === 'wireguard') {
    return JSON.stringify({
      secretKey: form.wgSecretKey,
      address: [form.wgAddress || '172.16.0.2/32'],
      noKernelTun: form.wgNoKernelTun !== false,
      mtu: form.wgMtu || 1280,
      peers: [
        {
          endpoint: form.wgEndpoint,
          publicKey: form.wgPeerPublicKey,
        },
      ],
    })
  } else if (form.protocol === 'vless') {
    const isTcp = form.streamNetwork === 'tcp'
    const isTlsOrReality = ['reality', 'tls'].includes(form.streamSecurity)
    const flowVal = isTcp && isTlsOrReality ? form.vlessFlow || '' : ''
    const userObj: Record<string, any> = {
      id: form.proxyPassword,
      encryption: 'none',
    }
    if (flowVal && flowVal !== 'none') {
      userObj.flow = flowVal
    } else {
      userObj.flow = ''
    }
    const settings: Record<string, any> = {
      vnext: [
        {
          address: form.proxyHost,
          port: form.proxyPort,
          users: [userObj],
        },
      ],
    }
    if (flowVal && flowVal !== 'none') {
      settings.flow = flowVal
    }
    return JSON.stringify(settings)
  } else if (form.protocol === 'vmess') {
    return JSON.stringify({
      vnext: [
        {
          address: form.proxyHost,
          port: form.proxyPort,
          users: [
            {
              id: form.proxyPassword,
              security: form.vmessSecurity || 'auto',
              alterId: form.vmessAlterId || 0,
            },
          ],
        },
      ],
    })
  } else if (form.protocol === 'shadowsocks') {
    return JSON.stringify({
      servers: [
        {
          address: form.proxyHost,
          port: form.proxyPort,
          method: form.ssMethod || '2022-blake3-aes-128-gcm',
          password: form.proxyPassword,
        },
      ],
    })
  } else if (form.protocol === 'trojan') {
    return JSON.stringify({
      servers: [
        {
          address: form.proxyHost,
          port: form.proxyPort,
          password: form.proxyPassword,
        },
      ],
    })
  } else if (['socks', 'http'].includes(form.protocol)) {
    const srv: Record<string, any> = {
      address: form.proxyHost,
      port: form.proxyPort,
    }
    if (form.proxyUsername || form.proxyPassword) {
      srv.users = [
        {
          user: form.proxyUsername || '',
          pass: form.proxyPassword || '',
        },
      ]
    }
    return JSON.stringify({
      servers: [srv],
    })
  } else if (form.protocol === 'dns') {
    return '{}'
  }
  return '{}'
}

export function buildStreamSettingsJSON(form: OutboundFormState): string {
  const sockopt: Record<string, any> = { ...(form.rawSockopt || {}) }
  const trimmedDialer = form.dialerProxy?.trim()
  if (trimmedDialer) {
    sockopt.dialerProxy = trimmedDialer
  } else {
    delete sockopt.dialerProxy
  }
  const hasSockopt = Object.keys(sockopt).length > 0

  if (['blackhole', 'dns'].includes(form.protocol)) {
    return ''
  }

  if (['freedom', 'wireguard', 'socks', 'http'].includes(form.protocol)) {
    if (!hasSockopt) {
      return ''
    }
    return JSON.stringify({ sockopt }, null, 2)
  }

  const stream: Record<string, any> = {
    network: form.streamNetwork,
    security: form.streamSecurity,
  }

  // vmess 与 shadowsocks 禁止 REALITY
  if (['vmess', 'shadowsocks'].includes(form.protocol) && stream.security === 'reality') {
    stream.security = 'none'
  }

  if (form.streamNetwork === 'xhttp') {
    stream.xhttpSettings = {
      path: form.xhttpPath || '/',
      mode: form.xhttpMode || 'auto',
    }
  } else if (form.streamNetwork === 'ws') {
    stream.wsSettings = {
      path: form.wsPath || '/',
      headers: form.wsHost ? { Host: form.wsHost } : undefined,
    }
  } else if (form.streamNetwork === 'grpc') {
    stream.grpcSettings = {
      serviceName: form.grpcServiceName || 'xray-grpc',
    }
  } else if (form.streamNetwork === 'httpupgrade') {
    const hu: Record<string, any> = {
      path: form.httpupgradePath || '/httpupgrade',
    }
    if (form.httpupgradeHost) {
      hu.host = form.httpupgradeHost
    }
    stream.httpupgradeSettings = hu
  }

  if (form.streamSecurity === 'reality') {
    const r: Record<string, any> = {
      serverName: form.realityServerName || '',
      publicKey: form.realityPublicKey || '',
      shortId: form.realityShortId || '',
      fingerprint: form.realityFingerprint || 'chrome',
    }
    if (form.realitySpiderX) {
      r.spiderX = form.realitySpiderX
    }
    stream.realitySettings = r
  } else if (form.streamSecurity === 'tls') {
    stream.tlsSettings = {
      serverName: form.tlsServerName || '',
      fingerprint: form.tlsFingerprint || 'chrome',
    }
  }

  if (hasSockopt) {
    stream.sockopt = sockopt
  }

  return JSON.stringify(stream, null, 2)
}

export function sanitizeOutboundPayload(form: OutboundFormState): OutboundPayload {
  const supportsMux = ['vless', 'vmess', 'trojan', 'shadowsocks', 'socks', 'http'].includes(
    form.protocol
  )
  const isVision =
    form.protocol === 'vless' &&
    (form.vlessFlow === 'xtls-rprx-vision' || form.vlessFlow === 'xtls-rprx-vision-udp443')

  let muxStr = '{}'
  if (supportsMux && form.muxEnabled && !isVision) {
    let concurrency = parseConcurrency(form.muxConcurrency, 8)
    if (concurrency === 0) concurrency = 8
    const xudpConcurrency = parseConcurrency(form.muxXudpConcurrency, 16)
    muxStr = JSON.stringify({
      enabled: true,
      concurrency,
      xudpConcurrency,
      xudpProxyUDP443: form.muxXudpProxyUDP443 || 'reject',
    })
  }

  const normalizedTargetStrategy =
    form.protocol === 'freedom' || form.targetStrategy?.trim() === 'AsIs'
      ? ''
      : form.targetStrategy?.trim() || ''

  return {
    tag: form.tag.trim(),
    protocol: form.protocol,
    sendThrough: form.sendThrough?.trim() || '',
    targetStrategy: normalizedTargetStrategy,
    settingsJson: buildSettingsJSON(form),
    streamSettings: buildStreamSettingsJSON(form),
    mux: muxStr,
  }
}

export function getProtocolBadgeVariant(
  proto?: string
): 'default' | 'secondary' | 'outline' | 'destructive' {
  switch (proto?.toLowerCase()) {
    case 'freedom':
      return 'default'
    case 'blackhole':
      return 'destructive'
    case 'wireguard':
    case 'dns':
      return 'secondary'
    default:
      return 'outline'
  }
}

export function getUsageDesc(ob: OutboundItem): string {
  switch (ob.protocol?.toLowerCase()) {
    case 'freedom':
      return '直接向目标发起网络连接'
    case 'blackhole':
      return '静默丢弃或拦截阻断连接'
    case 'wireguard':
      return 'Cloudflare WARP 清洁 IP 出站'
    case 'dns':
      return '内置 DNS 路由分流出站'
    default:
      return '转发至上游代理'
  }
}

export function getTargetEndpoint(ob: OutboundItem): string {
  if (ob.protocol === 'wireguard') {
    try {
      const s = JSON.parse(ob.settingsJson || '{}')
      return s.peers?.[0]?.endpoint || 'engage.cloudflareclient.com:2408'
    } catch {
      return '-'
    }
  }
  if (['freedom', 'blackhole', 'dns'].includes(ob.protocol)) {
    return '内置策略'
  }
  try {
    const s = JSON.parse(ob.settingsJson || '{}')
    if (s.vnext?.[0]) return `${s.vnext[0].address}:${s.vnext[0].port}`
    if (s.servers?.[0]) return `${s.servers[0].address}:${s.servers[0].port}`
  } catch {
    /* ignore */
  }
  return '-'
}

export function getOutboundStreamInfo(ob: OutboundItem): Record<string, any> {
  if (!ob.streamSettings) return {}
  try {
    return JSON.parse(ob.streamSettings)
  } catch {
    return {}
  }
}

export function getParsedSettings(ob: OutboundItem): Record<string, any> {
  if (!ob.settingsJson) return {}
  try {
    return JSON.parse(ob.settingsJson)
  } catch {
    return {}
  }
}

export function formatSettingsSummary(ob: OutboundItem): string {
  let baseSummary: string
  if (ob.protocol === 'dns') baseSummary = '内置 DNS 路由解析'
  else if (!ob.settingsJson || ob.settingsJson === '{}') baseSummary = '默认系统策略'
  else {
    try {
      const s = JSON.parse(ob.settingsJson)
      if (s.domainStrategy) baseSummary = `解析策略：${s.domainStrategy}`
      else if (s.peers?.length > 0) baseSummary = `WARP Endpoint：${s.peers[0].endpoint}`
      else if (s.response?.type) baseSummary = `拦截类型：${s.response.type}`
      else if (s.vnext?.length > 0)
        baseSummary = `上游代理：${s.vnext[0].address}:${s.vnext[0].port}`
      else if (s.servers?.length > 0)
        baseSummary = `代理服务器：${s.servers[0].address}:${s.servers[0].port}`
      else baseSummary = JSON.stringify(s)
    } catch {
      baseSummary = ob.settingsJson
    }
  }

  const extras: string[] = []
  const dialer = parseDialerProxy(ob.streamSettings)
  if (dialer) {
    extras.push(`前置：${dialer}`)
  }
  const muxSummary = parseMuxSummary(ob.mux)
  if (muxSummary) {
    extras.push(`Mux (${muxSummary})`)
  }
  if (ob.targetStrategy) {
    extras.push(`解析：${ob.targetStrategy}`)
  }
  if (ob.sendThrough) {
    extras.push(`绑定 IP：${ob.sendThrough}`)
  }

  if (extras.length > 0) {
    return `${baseSummary} [${extras.join(' | ')}]`
  }
  return baseSummary
}

export function formatJsonPretty(str?: string): string {
  if (!str) return '{}'
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}
