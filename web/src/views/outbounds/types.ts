export interface OutboundItem {
  tag: string
  protocol: string // freedom, blackhole, wireguard, vless, vmess, trojan, shadowsocks, socks, http, dns
  settingsJson?: string
  streamSettings?: string
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
  tlsAllowInsecure: boolean
  tlsFingerprint: string
}

export interface OutboundPayload {
  tag: string
  protocol: string
  settingsJson: string
  streamSettings: string
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
    realityServerName: 'www.example.com',
    realityFingerprint: 'chrome',
    realityPublicKey: '',
    realityShortId: '0123456789abcdef',
    tlsServerName: '',
    tlsAllowInsecure: false,
    tlsFingerprint: 'chrome',
  }
}

export function populateOutboundForm(ob: OutboundItem): OutboundFormState {
  const form = createDefaultOutboundForm()
  form.tag = ob.tag || ''
  form.protocol = ob.protocol || 'vless'

  let s: Record<string, any> = {}
  try {
    s = JSON.parse(ob.settingsJson || '{}')
  } catch {
    s = {}
  }

  let str: Record<string, any> = {}
  try {
    str = JSON.parse(ob.streamSettings || '{}')
  } catch {
    str = {}
  }

  form.streamNetwork = str.network || 'tcp'
  form.streamSecurity = str.security || 'none'
  form.freedomDomainStrategy = s.domainStrategy || 'UseIPv4'
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
    form.realityServerName = str.realitySettings.serverName || 'www.example.com'
    form.realityPublicKey = str.realitySettings.publicKey || ''
    form.realityShortId = str.realitySettings.shortId || ''
    form.realityFingerprint = str.realitySettings.fingerprint || 'chrome'
  }
  if (str.tlsSettings) {
    form.tlsServerName = str.tlsSettings.serverName || ''
    form.tlsAllowInsecure = str.tlsSettings.allowInsecure === true
    form.tlsFingerprint = str.tlsSettings.fingerprint || 'chrome'
  }

  if (ob.protocol === 'wireguard') {
    form.wgSecretKey = s.secretKey || ''
    form.wgAddress = (s.address || ['172.16.0.2/32'])[0]
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
          form.vlessFlow = u.flow !== undefined ? u.flow : 'xtls-rprx-vision'
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
      noKernelTun: true,
      mtu: 1280,
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
    const userObj: Record<string, any> = {
      id: form.proxyPassword,
      encryption: 'none',
    }
    if (isTcp && isTlsOrReality && form.vlessFlow) {
      userObj.flow = form.vlessFlow
    }
    return JSON.stringify({
      vnext: [
        {
          address: form.proxyHost,
          port: form.proxyPort,
          users: [userObj],
        },
      ],
    })
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
  if (['freedom', 'blackhole', 'wireguard', 'socks', 'http', 'dns'].includes(form.protocol)) {
    return ''
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
  }

  if (form.streamSecurity === 'reality') {
    stream.realitySettings = {
      serverName: form.realityServerName || 'www.example.com',
      publicKey: form.realityPublicKey || '',
      shortId: form.realityShortId || '',
      fingerprint: form.realityFingerprint || 'chrome',
    }
  } else if (form.streamSecurity === 'tls') {
    stream.tlsSettings = {
      serverName: form.tlsServerName || '',
      allowInsecure: form.tlsAllowInsecure === true,
      fingerprint: form.tlsFingerprint || 'chrome',
    }
  }

  return JSON.stringify(stream, null, 2)
}

export function sanitizeOutboundPayload(form: OutboundFormState): OutboundPayload {
  return {
    tag: form.tag.trim(),
    protocol: form.protocol,
    settingsJson: buildSettingsJSON(form),
    streamSettings: buildStreamSettingsJSON(form),
  }
}

export function getProtocolBadgeVariant(proto?: string): 'default' | 'secondary' | 'outline' | 'destructive' {
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
  } catch {}
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
  if (ob.protocol === 'dns') return '内置 DNS 路由解析'
  if (!ob.settingsJson || ob.settingsJson === '{}') return '默认系统策略'
  try {
    const s = JSON.parse(ob.settingsJson)
    if (s.domainStrategy) return `解析策略: ${s.domainStrategy}`
    if (s.peers?.length > 0) return `WARP Endpoint: ${s.peers[0].endpoint}`
    if (s.response?.type) return `拦截类型: ${s.response.type}`
    if (s.vnext?.length > 0) return `上游代理: ${s.vnext[0].address}:${s.vnext[0].port}`
    if (s.servers?.length > 0) return `代理服务器: ${s.servers[0].address}:${s.servers[0].port}`
    return JSON.stringify(s)
  } catch {
    return ob.settingsJson
  }
}

export function formatJsonPretty(str?: string): string {
  if (!str) return '{}'
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}
