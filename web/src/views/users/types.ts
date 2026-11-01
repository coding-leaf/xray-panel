export interface UserItem {
  id: number
  email: string
  uuid?: string
  subToken?: string
  inboundTags?: string
  inboundTag?: string
  flow?: string
  remark?: string
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
  uuid?: string
  subToken?: string
  remark?: string
  expireTime?: number
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

  let expireTime = form.expireTime ?? (existingUser ? existingUser.expireTime || 0 : 0)
  if (isEditing) {
    if (form.extendDays > 0) {
      const now = Date.now()
      const base = (!expireTime || expireTime < now) ? now : expireTime
      expireTime = base + form.extendDays * 86400000
    }
  }

  const payload: Record<string, any> = {
    email: form.email,
    uuid: form.uuid ? form.uuid.trim() : undefined,
    subToken: form.subToken ? form.subToken.trim() : undefined,
    remark: form.remark ? form.remark.trim() : '',
    inboundTags: isEditing ? form.selectedTags.join(',') : form.selectedTags,
    inboundTag: form.selectedTags[0] || '',
    flow: form.flow,
    totalBytes,
    expireTime,
    resetDay: form.resetDay,
    ipLimit: form.ipLimit,
    enabled: form.enabled,
  }

  if (!isEditing && form.expireDays > 0 && (!form.expireTime || form.expireTime <= 0)) {
    payload.expireDays = form.expireDays
  }

  return payload
}
