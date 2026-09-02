// 本地 LocalStorage 毛坯房数据持久化引擎

export interface MockState {
  inbounds: any[]
  outbounds: any[]
  routing: any
  dns: any
  users: any[]
  logs: string[]
}

const STORAGE_KEY = 'xray_panel_demo_state_v1'

const DEFAULT_RAW_STATE: MockState = {
  inbounds: [
    {
      id: 1,
      tag: 'vless-reality',
      listen: '0.0.0.0',
      port: 4434,
      externalPort: 443,
      externalHost: 'demo.example.com',
      routeId: 0,
      protocol: 'vless',
      settingsJson: JSON.stringify({
        decryption: 'none',
        clients: [],
      }),
      streamSettings: JSON.stringify({
        network: 'tcp',
        security: 'reality',
        realitySettings: {
          dest: 'www.titech.ac.jp:443',
          serverNames: ['www.titech.ac.jp'],
          privateKey: 'OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc',
          publicKey: 'FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww',
          shortIds: ['0123456789abcdef'],
        },
      }),
      sniffingJson: JSON.stringify({
        enabled: true,
        destOverride: ['http', 'tls', 'quic'],
        routeOnly: true,
      }),
      subRoutesJson: '[]',
      enabled: true,
      isAlive: true,
      latencyMs: 12,
    },
  ],
  outbounds: [
    {
      tag: 'direct',
      protocol: 'freedom',
      settingsJson: JSON.stringify({ domainStrategy: 'UseIPv4' }),
      streamSettings: '{}',
    },
    {
      tag: 'block',
      protocol: 'blackhole',
      settingsJson: '{}',
      streamSettings: '{}',
    },
  ],
  routing: {
    domainStrategy: 'IPIfNonMatch',
    rules: [
      {
        type: 'field',
        outboundTag: 'block',
        domain: ['geosite:category-ads-all'],
      },
      {
        type: 'field',
        outboundTag: 'direct',
        ip: ['geoip:private', 'geoip:cn'],
      },
      {
        type: 'field',
        outboundTag: 'direct',
        domain: ['geosite:cn'],
      },
    ],
  },
  dns: {
    servers: ['8.8.8.8', 'https+local://1.1.1.1/dns-query', 'localhost'],
    queryStrategy: 'UseIPv4',
  },
  users: [],
  logs: [
    '2026/08/29 19:40:01 127.0.0.1:54321 accepted tcp:www.google.com:443 [vless-reality -> direct] email: demo@user.com',
    '2026/08/29 19:40:15 127.0.0.1:54322 accepted tcp:github.com:443 [vless-reality -> direct] email: demo@user.com',
    '2026/08/29 19:40:30 [Info] infra/conf/serial: Reading config: &{Name:/usr/local/etc/xray/config.json Format:json}',
    '2026/08/29 19:40:32 [Warning] app/proxyman/inbound: connection ends > vless: reading connection: EOF',
  ],
}

export function loadMockState(): MockState {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      return JSON.parse(raw)
    }
  } catch (e) {
    console.error('Failed to load mock state from localStorage', e)
  }
  saveMockState(DEFAULT_RAW_STATE)
  return JSON.parse(JSON.stringify(DEFAULT_RAW_STATE))
}

export function saveMockState(state: MockState): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  } catch (e) {
    console.error('Failed to save mock state to localStorage', e)
  }
}

export function resetMockState(): MockState {
  const fresh = JSON.parse(JSON.stringify(DEFAULT_RAW_STATE))
  saveMockState(fresh)
  return fresh
}
