import type { UserItem } from '../types'

function safeBtoa(str: string): string {
  try {
    return btoa(encodeURIComponent(str).replace(/%([0-9A-F]{2})/g, (_, p1) => String.fromCharCode(parseInt(p1, 16))))
  } catch (_e) {
    return btoa(str)
  }
}

export class UserSubscriptionService {
  /** 生成独立安全 Token 聚合订阅链接 */
  static getDirectTokenSubUrl(user?: UserItem | null): string {
    if (!user || !user.subToken) return ''
    return `${window.location.origin}/sub/${user.subToken}`
  }

  /** 解析用户授权的入站 Tags 列表，支持基于当前可用节点进行有效性校验与回退 */
  static getNodeTags(user?: UserItem | null, availableInbounds: any[] = []): string[] {
    if (!user) return []
    let tags: string[] = []
    if (user.inboundTags) {
      tags = user.inboundTags.split(',').map((s: string) => s.trim()).filter(Boolean)
    } else if (user.inboundTag) {
      tags = [user.inboundTag]
    }

    if (availableInbounds && availableInbounds.length > 0) {
      const validTags = new Set(availableInbounds.map((i: any) => i.tag))
      const filtered = tags.filter((t) => validTags.has(t))
      if (filtered.length > 0) {
        return filtered
      }
      return availableInbounds.map((i: any) => i.tag)
    }
    return tags
  }

  /**
   * 生成单个节点的直连链接
   * 支持协议: vless, vmess, trojan, shadowsocks / ss
   */
  static generateNodeLink(inb: any, user?: UserItem | null): string {
    if (!inb) return ''
    const proto = (inb.protocol || 'vless').toLowerCase()
    const extHost = inb.listen || window.location.hostname || '127.0.0.1'
    const port = inb.port || 443
    const uuid = user?.uuid || 'uuid'
    const remark = inb.remark || inb.tag || 'node'
    const stream = inb.streamSettings || {}
    const network = stream.network || inb.network || 'tcp'
    const security = stream.security || (inb.tls ? 'tls' : 'none')
    const reality = stream.realitySettings || {}
    const tlsSettings = stream.tlsSettings || {}
    const sni = reality.serverName || tlsSettings.serverName || extHost

    if (proto === 'vless') {
      const pbk = reality.publicKey || ''
      const sid = reality.shortIds?.[0] || reality.shortId || ''
      const fp = reality.fingerprint || 'chrome'
      const flow = user?.flow || inb.flow || ''
      const query = new URLSearchParams()
      if (security && security !== 'none') query.set('security', security)
      if (sni) query.set('sni', sni)
      if (fp) query.set('fp', fp)
      if (pbk) query.set('pbk', pbk)
      if (sid) query.set('sid', sid)
      query.set('type', network)
      if (flow) query.set('flow', flow)

      return `vless://${uuid}@${extHost}:${port}?${query.toString()}#${encodeURIComponent(remark)}`
    }

    if (proto === 'vmess') {
      const wsSettings = stream.wsSettings || {}
      const vmessObj = {
        v: '2',
        ps: remark,
        add: extHost,
        port: port,
        id: uuid,
        aid: 0,
        net: network,
        type: 'none',
        host: wsSettings.headers?.Host || extHost,
        path: wsSettings.path || '/',
        tls: security === 'tls' ? 'tls' : 'none',
        sni: sni,
      }
      return `vmess://${safeBtoa(JSON.stringify(vmessObj))}`
    }

    if (proto === 'trojan') {
      const query = new URLSearchParams()
      if (security && security !== 'none') query.set('security', security)
      if (sni) query.set('sni', sni)
      query.set('type', network)
      return `trojan://${uuid}@${extHost}:${port}?${query.toString()}#${encodeURIComponent(remark)}`
    }

    if (proto === 'shadowsocks' || proto === 'ss') {
      const method = inb.settings?.method || '2022-blake3-aes-128-gcm'
      const password = user?.uuid || inb.settings?.password || 'password'
      const ssAuth = safeBtoa(`${method}:${password}`)
      return `ss://${ssAuth}@${extHost}:${port}#${encodeURIComponent(remark)}`
    }

    return `vless://${uuid}@${extHost}:${port}#${encodeURIComponent(remark)}`
  }
}
