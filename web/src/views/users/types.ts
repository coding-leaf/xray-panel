export interface UserItem {
  id: number
  email: string
  uuid?: string
  subToken?: string
  inboundTags?: string
  inboundTag?: string
  flow?: string
  totalBytes: number
  upBytes: number
  downBytes: number
  expireTime: number
  resetDay: number
  ipLimit: number
  enabled: boolean
  isOnline?: boolean
  upSpeed?: number
  downSpeed?: number
  [key: string]: any
}

export interface UserFormData {
  id?: number
  email: string
  selectedTags: string[]
  flow: string
  totalGB: number
  expireDays: number
  extendDays: number
  resetDay: number
  ipLimit: number
  enabled: boolean
}

export interface TrafficRecord {
  id: number
  userId: number
  date: string
  upBytes: number
  downBytes: number
}

export interface UserShareNode {
  tag: string
  protocol?: string
  remark?: string
  shareLink?: string
  singleSub?: string
  url?: string
}

export interface UserShareData {
  userId?: number
  email?: string
  subToken?: string
  allSubUrl?: string
  nodes?: UserShareNode[]
  links?: (string | { tag: string; url?: string; shareLink?: string })[]
  user?: UserItem
}

export function sanitizeUserPayload(
  form: UserFormData,
  isEditing: boolean,
  existingUser?: UserItem
): Record<string, any> {
  const totalBytes = form.totalGB > 0 ? Math.round(form.totalGB * 1073741824) : 0

  if (isEditing && existingUser) {
    let expireTime = existingUser.expireTime || 0
    if (form.extendDays > 0) {
      const now = Date.now()
      expireTime = (!expireTime || expireTime < now)
        ? now + form.extendDays * 86400000
        : expireTime + form.extendDays * 86400000
    }
    return {
      email: form.email,
      inboundTags: form.selectedTags.join(','),
      inboundTag: form.selectedTags[0] || '',
      flow: form.flow,
      totalBytes,
      expireTime,
      resetDay: form.resetDay,
      ipLimit: form.ipLimit,
      enabled: form.enabled,
    }
  }

  return {
    email: form.email,
    inboundTags: form.selectedTags,
    inboundTag: form.selectedTags[0] || '',
    flow: form.flow,
    totalBytes,
    expireDays: form.expireDays,
    resetDay: form.resetDay,
    ipLimit: form.ipLimit,
    enabled: form.enabled,
  }
}
