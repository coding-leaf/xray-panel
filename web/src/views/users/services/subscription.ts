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
      const filtered = tags.filter((t) => validTags.has(t))
      if (filtered.length > 0) {
        return filtered
      }
      return availableInbounds.map((i: any) => i.tag)
    }
    return tags
  }
}
