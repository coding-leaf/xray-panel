import type { UserItem } from '../types'

/**
 * 用户订阅辅助服务 (前端仅保留展示与标签过滤相关的必要辅助，所有真实节点链接由后端聚合订阅与分享 API 统一提供)
 */
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
      return tags.filter((t) => validTags.has(t))
    }
    return tags
  }

  /** 计算指定 Inbound 标签下用户可见的有效分流线路数量 */
  static getSubRoutesCount(tag: string, availableInbounds: any[] = [], user?: UserItem | null): number {
    const inb = availableInbounds.find((i: any) => i.tag === tag)
    if (!inb) return 0
    const isVless = !inb.protocol || inb.protocol.toLowerCase() === 'vless'
    if (!isVless) return 0
    try {
      const srs = JSON.parse(inb.subRoutesJson || '[]')
      return srs.filter((s: any) => {
        if (s.enabled === false) return false
        if (user && s.allowedUsers && s.allowedUsers.length > 0) {
          return s.allowedUsers.includes(user.email)
        }
        return true
      }).length
    } catch (_e) {
      return 0
    }
  }
}
