// 本地 LocalStorage 演示环境数据持久化引擎 (v3 - gRPC 运行时架构)

export interface MockState {
  inbounds: any[]
  outbounds: any[]
  routing: any
  dns: any
  users: any[]
  logs: string[]
  auditLogs?: any[]
  settings?: any
  snapshots?: any[]
  geodata?: any
}

const STORAGE_KEY = 'xray_panel_demo_state_v3'

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
          dest: 'www.example.com:443',
          serverNames: ['www.example.com'],
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
          name: '🇯🇵 日本东京原生 (直连出口)',
          outboundTag: 'direct',
          enabled: true,
        },
        {
          routeId: 2,
          name: '🇭🇰 香港低延迟 (落地节点)',
          outboundTag: 'hk-landing',
          enabled: true,
        },
        {
          routeId: 3,
          name: '🇺🇸 美国西海岸 (WARP 解锁)',
          outboundTag: 'warp-out',
          enabled: true,
        },
      ]),
      enabled: true,
      isAlive: true,
      latencyMs: 18,
    },
    {
      id: 2,
      tag: 'vmess-ws',
      listen: '0.0.0.0',
      port: 8080,
      externalPort: 80,
      externalHost: 'demo.example.com',
      routeId: 0,
      protocol: 'vmess',
      settingsJson: JSON.stringify({
        clients: [],
      }),
      streamSettings: JSON.stringify({
        network: 'ws',
        security: 'none',
        wsSettings: {
          path: '/vmess',
          headers: {
            Host: 'demo.example.com',
          },
        },
      }),
      sniffingJson: JSON.stringify({
        enabled: true,
        destOverride: ['http', 'tls'],
        routeOnly: false,
      }),
      subRoutesJson: '[]',
      enabled: true,
      isAlive: true,
      latencyMs: 24,
    },
    {
      id: 3,
      tag: 'trojan-reality',
      listen: '0.0.0.0',
      port: 8443,
      externalPort: 8443,
      externalHost: 'demo.example.com',
      routeId: 0,
      protocol: 'trojan',
      settingsJson: JSON.stringify({
        clients: [],
      }),
      streamSettings: JSON.stringify({
        network: 'tcp',
        security: 'reality',
        realitySettings: {
          dest: 'gateway.icloud.com:443',
          serverNames: ['gateway.icloud.com'],
          privateKey: 'OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc',
          publicKey: 'FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww',
          shortIds: ['0123456789abcdef'],
        },
      }),
      sniffingJson: JSON.stringify({
        enabled: true,
        destOverride: ['http', 'tls'],
        routeOnly: false,
      }),
      subRoutesJson: '[]',
      enabled: true,
      isAlive: true,
      latencyMs: 21,
    },
    {
      id: 4,
      tag: 'ss-in',
      listen: '0.0.0.0',
      port: 8388,
      externalPort: 8388,
      externalHost: 'demo.example.com',
      routeId: 0,
      protocol: 'shadowsocks',
      settingsJson: JSON.stringify({
        method: '2022-blake3-aes-128-gcm',
        password: 'xray-panel-demo-password-2026',
        network: 'tcp,udp',
      }),
      streamSettings: '{}',
      sniffingJson: JSON.stringify({
        enabled: true,
        destOverride: ['http', 'tls'],
        routeOnly: false,
      }),
      subRoutesJson: '[]',
      enabled: true,
      isAlive: true,
      latencyMs: 19,
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
      email: 'master@example.com',
      uuid: '7117295b-4362-4260-a133-b969344dfcd5',
      inboundTags: 'vless-reality,vmess-ws,trojan-reality,ss-in',
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
      email: 'alice_vision@demo.local',
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
      isOnline: true,
      upSpeed: 450000,
      downSpeed: 2100000,
      subToken: '90da5e0c99322d49',
      createdAt: new Date(now - 15 * 86400000).toISOString(),
    },
    {
      id: 3,
      email: 'bob_vmess@demo.local',
      uuid: 'a8e14652-325b-4a57-b08e-5b1b4d0811e2',
      inboundTags: 'vmess-ws',
      inboundTag: 'vmess-ws',
      flow: '',
      upBytes: 4200000000,
      downBytes: 12800000000,
      totalBytes: 50 * 1024 * 1024 * 1024, // 50 GB
      expireTime: now + 90 * 86400000,
      resetDay: 1,
      ipLimit: 2,
      enabled: true,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      subToken: 'a8e146525b1b4d08',
      createdAt: new Date(now - 12 * 86400000).toISOString(),
    },
    {
      id: 4,
      email: 'carol_trojan@demo.local',
      uuid: 'e79b2940-d983-4927-9964-b63487f8723c',
      inboundTags: 'trojan-reality',
      inboundTag: 'trojan-reality',
      flow: '',
      upBytes: 1100000000,
      downBytes: 3400000000,
      totalBytes: 50 * 1024 * 1024 * 1024, // 50 GB
      expireTime: now + 45 * 86400000,
      resetDay: 5,
      ipLimit: 2,
      enabled: true,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      subToken: 'e79b2940b63487f8',
      createdAt: new Date(now - 8 * 86400000).toISOString(),
    },
    {
      id: 5,
      email: 'dave_shadowsocks@demo.local',
      uuid: 'c52df85e-e478-43d7-84e1-70bf81d11394',
      inboundTags: 'ss-in',
      inboundTag: 'ss-in',
      flow: '',
      upBytes: 520000000,
      downBytes: 1200000000,
      totalBytes: 30 * 1024 * 1024 * 1024, // 30 GB
      expireTime: now + 30 * 86400000,
      resetDay: 1,
      ipLimit: 1,
      enabled: true,
      isOnline: false,
      upSpeed: 0,
      downSpeed: 0,
      subToken: 'c52df85e70bf81d1',
      createdAt: new Date(now - 5 * 86400000).toISOString(),
    },
    {
      id: 6,
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
      id: 7,
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
    '2026/09/07 22:45:01 [Info] app/proxyman/command: Dynamic gRPC HandlerService AlterInbound: AddUser master@example.com (VLESS Vision) into inbound [vless-reality] success',
    '2026/09/07 22:45:01 [Info] storage/boltdb: ACID transaction committed user master@example.com into bucket "users"',
    '2026/09/07 22:45:02 [Info] app/proxyman/command: Dynamic gRPC HandlerService AlterInbound: AddUser bob_vmess@demo.local (VMess) into inbound [vmess-ws] success',
    '2026/09/07 22:45:03 [Info] app/proxyman/command: Dynamic gRPC HandlerService AlterInbound: AddUser carol_trojan@demo.local (Trojan) into inbound [trojan-reality] success',
    '2026/09/07 22:45:04 [Info] app/proxyman/command: Dynamic gRPC HandlerService AlterInbound: AddUser dave_shadowsocks@demo.local (Shadowsocks) into inbound [ss-in] success',
    '2026/09/07 22:46:10 [Info] app/stats/command: StatsService QueryStats pattern "user>>>master@example.com>>>traffic>>>downlink" -> 30000000000 bytes',
    '2026/09/07 22:47:00 [Info] app/sync: Cold-boot SyncToDiskConfig fallback verified with zero config drift',
    '2026/09/07 22:48:01 127.0.0.1:54321 accepted tcp:www.youtube.com:443 [vless-reality -> direct] email: master@example.com',
    '2026/09/07 22:48:15 127.0.0.1:54322 accepted tcp:api.openai.com:443 [vless-reality -> warp-out] email: master@example.com',
    '2026/09/07 22:48:30 127.0.0.1:54323 accepted tcp:hk-node.example.com:443 [vless-reality -> hk-landing] email: master@example.com',
  ],
  settings: {
    xray_service_name: 'xray',
    xray_grpc_addr: '127.0.0.1:8080',
    xray_config_path: '/usr/local/etc/xray/config.json',
    xray_bin_path: '/usr/local/bin/xray',
    xray_geodata_dir: '/usr/local/share/xray',
    public_url: 'https://demo.example.com:9000',
    portal_url: '',
    sub_domain: 'sub.example.com',
    public_port: 443,
    tg_bot_token: '123456789:ABCdefGHIjklMNOpqrSTUvwxYZ',
    tg_admin_chat_id: '987654321',
    totpEnabled: false,
  },
  snapshots: [
    {
      id: 1,
      remark: 'v1.6.0 gRPC 架构初始化快照',
      createdAt: new Date(now - 3600000).toISOString(),
      content: JSON.stringify(
        {
          inbounds: [
            {
              tag: 'vless-reality',
              port: 4434,
              protocol: 'vless',
            },
          ],
        },
        null,
        2
      ),
    },
  ],
  geodata: {
    platform: 'Xray Core (gRPC Runtime)',
    geoipExists: true,
    geoipSize: 8941200,
    geositeExists: true,
    geositeSize: 23518400,
    targetDirectory: '/usr/local/share/xray',
  },
  auditLogs: [
    {
      id: 1,
      createdAt: new Date(now - 7200000).toISOString(),
      operator: 'admin',
      clientIp: '127.0.0.1',
      action: 'AUTH_LOGIN',
      target: 'admin',
      details: '管理员成功登录控制台',
      status: 'SUCCESS',
    },
    {
      id: 2,
      createdAt: new Date(now - 3600000).toISOString(),
      operator: 'admin',
      clientIp: '127.0.0.1',
      action: 'INBOUND_UPDATE',
      target: 'vless-reality',
      details: '更新入站节点配置: 端口 4434',
      status: 'SUCCESS',
    },
    {
      id: 3,
      createdAt: new Date(now - 1800000).toISOString(),
      operator: 'admin',
      clientIp: '127.0.0.1',
      action: 'CONFIG_RELOAD',
      target: 'xray-core',
      details: '触发 Xray 配置热重载',
      status: 'SUCCESS',
    },
  ],
}

export function loadMockState(): MockState {
  try {
    // 检查是否有 v1/v2 旧缓存，若存在则清理并迁移至 v3
    if (localStorage.getItem('xray_panel_demo_state_v1')) {
      localStorage.removeItem('xray_panel_demo_state_v1')
    }
    if (localStorage.getItem('xray_panel_demo_state_v2')) {
      localStorage.removeItem('xray_panel_demo_state_v2')
    }

    const raw = localStorage.getItem(STORAGE_KEY)
    if (raw) {
      const parsed = JSON.parse(raw)
      return {
        ...DEFAULT_RAW_STATE,
        ...parsed,
        settings: { ...DEFAULT_RAW_STATE.settings, ...(parsed.settings || {}) },
        geodata: { ...DEFAULT_RAW_STATE.geodata, ...(parsed.geodata || {}) },
        snapshots: parsed.snapshots && parsed.snapshots.length ? parsed.snapshots : DEFAULT_RAW_STATE.snapshots,
        auditLogs: parsed.auditLogs && parsed.auditLogs.length ? parsed.auditLogs : DEFAULT_RAW_STATE.auditLogs,
      }
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

