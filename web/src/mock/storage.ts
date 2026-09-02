// 本地 LocalStorage 演示环境数据持久化引擎 (v2)

export interface MockState {
  inbounds: any[]
  outbounds: any[]
  routing: any
  dns: any
  users: any[]
  logs: string[]
}

const STORAGE_KEY = 'xray_panel_demo_state_v2'

const now = Date.now()

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
      subRoutesJson: JSON.stringify([
        {
          routeId: 1,
          remark: '🇯🇵 日本东京原生 (直连出口)',
          outboundTag: 'direct',
          enabled: true,
        },
        {
          routeId: 2,
          remark: '🇭🇰 香港低延迟 (落地节点)',
          outboundTag: 'hk-landing',
          enabled: true,
        },
        {
          routeId: 3,
          remark: '🇺🇸 美国西海岸 (WARP 解锁)',
          outboundTag: 'warp-out',
          enabled: true,
        },
      ]),
      enabled: true,
      isAlive: true,
      latencyMs: 18,
    },
  ],
  outbounds: [
    {
      id: 1,
      tag: 'direct',
      protocol: 'freedom',
      settingsJson: JSON.stringify({ domainStrategy: 'UseIPv4' }),
      streamSettings: '{}',
    },
    {
      id: 2,
      tag: 'block',
      protocol: 'blackhole',
      settingsJson: JSON.stringify({ response: { type: 'none' } }),
      streamSettings: '{}',
    },
    {
      id: 3,
      tag: 'warp-out',
      protocol: 'wireguard',
      settingsJson: JSON.stringify({
        address: ['172.16.0.2/32'],
        mtu: 1280,
        noKernelTun: true,
        peers: [
          {
            endpoint: 'engage.cloudflareclient.com:2408',
            publicKey: 'bmXOC+F1FxEMF9dyiK2H5/1SUtzH0JuVo51h2wPfgyo=',
          },
        ],
        secretKey: 'APHekUQCpV3neCk1BnXOXgte7eLCBNfB2iZ8yVyQs0s=',
      }),
      streamSettings: '{}',
    },
    {
      id: 4,
      tag: 'hk-landing',
      protocol: 'vless',
      settingsJson: JSON.stringify({
        vnext: [
          {
            address: 'hk-node.example.com',
            port: 443,
            users: [
              {
                id: 'a1a030bb-9b90-4d07-b54c-52a412598b66',
                encryption: 'none',
              },
            ],
          },
        ],
      }),
      streamSettings: JSON.stringify({
        network: 'xhttp',
        security: 'reality',
        realitySettings: {
          serverName: 'speed.cloudflare.com',
          publicKey: 'vI3J4JxuMsq2oIqmxbkptvR0HDYnveA0STiNbh7TpGo',
          fingerprint: 'chrome',
        },
        xhttpSettings: {
          path: '/show_imglikeyou',
          mode: 'stream-one',
        },
      }),
    },
    {
      id: 5,
      tag: 'jp-landing',
      protocol: 'vless',
      settingsJson: JSON.stringify({
        vnext: [
          {
            address: 'jp-node.example.com',
            port: 443,
            users: [
              {
                id: 'b2b040cc-8c80-4e08-a55d-63b523609c77',
                encryption: 'none',
              },
            ],
          },
        ],
      }),
      streamSettings: JSON.stringify({
        network: 'tcp',
        security: 'reality',
        realitySettings: {
          serverName: 'gateway.icloud.com',
          publicKey: 'FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww',
          fingerprint: 'chrome',
        },
      }),
    },
  ],
  routing: {
    domainStrategy: 'IPIfNonMatch',
    rules: [
      {
        type: 'field',
        outboundTag: 'block',
        ip: ['geoip:private'],
      },
      {
        type: 'field',
        outboundTag: 'block',
        protocol: ['bittorrent'],
      },
      {
        type: 'field',
        outboundTag: 'block',
        port: '25',
        network: 'tcp',
      },
      {
        type: 'field',
        outboundTag: 'warp-out',
        domain: ['geosite:category-ads-all', 'geosite:cn'],
      },
      {
        type: 'field',
        outboundTag: 'warp-out',
        ip: ['geoip:cn'],
      },
    ],
  },
  dns: {
    servers: ['https+local://1.1.1.1/dns-query', '8.8.8.8', 'localhost'],
    queryStrategy: 'UseIPv4',
  },
  users: [
    {
      id: 1,
      email: 'master@yezineko.top',
      uuid: '7117295b-4362-4260-a133-b969344dfcd5',
      inboundTags: 'vless-reality',
      inboundTag: 'vless-reality',
      flow: 'xtls-rprx-vision',
      upBytes: 15200000000,
      downBytes: 30000000000,
      totalBytes: 200 * 1024 * 1024 * 1024, // 200 GB
      expireTime: now + 60 * 86400000, // 60 天后
      resetDay: 1,
      ipLimit: 3,
      enabled: true,
      isOnline: true,
      upSpeed: 1250000, // 1.2 MB/s
      downSpeed: 8450000, // 8.4 MB/s
      subToken: '7117295ba1334dfc',
      createdAt: new Date(now - 30 * 86400000).toISOString(),
    },
    {
      id: 2,
      email: 'user_tokyo@demo.local',
      uuid: '90da5e0c-5a0a-4f19-9932-2d492f2096d3',
      inboundTags: 'vless-reality',
      inboundTag: 'vless-reality',
      flow: 'xtls-rprx-vision',
      upBytes: 2600000000,
      downBytes: 6000000000,
      totalBytes: 100 * 1024 * 1024 * 1024, // 100 GB
      expireTime: now + 180 * 86400000, // 180 天后
      resetDay: 15,
      ipLimit: 0,
      enabled: true,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      subToken: '90da5e0c99322d49',
      createdAt: new Date(now - 15 * 86400000).toISOString(),
    },
    {
      id: 3,
      email: 'expired_guest@demo.local',
      uuid: '1636960f-a826-474f-ae2a-f47a1de4b6b9',
      inboundTags: 'vless-reality',
      inboundTag: 'vless-reality',
      flow: '',
      upBytes: 4000000000,
      downBytes: 11000000000,
      totalBytes: 50 * 1024 * 1024 * 1024, // 50 GB
      expireTime: now - 5 * 86400000, // 5 天前已过期
      resetDay: 0,
      ipLimit: 1,
      enabled: true,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      subToken: '1636960fae2af47a',
      createdAt: new Date(now - 60 * 86400000).toISOString(),
    },
    {
      id: 4,
      email: 'overquota_tester@demo.local',
      uuid: 'f0f0fd99-f013-479b-9f03-0dd5c13f6de1',
      inboundTags: 'vless-reality',
      inboundTag: 'vless-reality',
      flow: '',
      upBytes: 15100000000,
      downBytes: 35000000000, // 50.1 GB
      totalBytes: 50 * 1024 * 1024 * 1024, // 50 GB 封顶
      expireTime: now + 25 * 86400000,
      resetDay: 1,
      ipLimit: 2,
      enabled: true,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      subToken: 'f0f0fd999f030dd5',
      createdAt: new Date(now - 10 * 86400000).toISOString(),
    },
  ],
  logs: [
    '2026/09/03 07:20:01 127.0.0.1:54321 accepted tcp:www.youtube.com:443 [vless-reality -> direct] email: master@yezineko.top',
    '2026/09/03 07:20:15 127.0.0.1:54322 accepted tcp:api.openai.com:443 [vless-reality -> warp-out] email: master@yezineko.top',
    '2026/09/03 07:20:30 127.0.0.1:54323 accepted tcp:hk-node.example.com:443 [vless-reality -> hk-landing] email: master@yezineko.top',
    '2026/09/03 07:21:05 [Info] infra/conf/serial: Reading config: &{Name:/usr/local/etc/xray/config.json Format:json}',
    '2026/09/03 07:21:08 [Info] app/proxyman/inbound: Dynamic gRPC AlterInbound: AddUser master@yezineko.top success',
  ],
}

export function loadMockState(): MockState {
  try {
    // 检查是否有 v1 旧缓存，若存在则清理并迁移至 v2
    if (localStorage.getItem('xray_panel_demo_state_v1')) {
      localStorage.removeItem('xray_panel_demo_state_v1')
    }

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
