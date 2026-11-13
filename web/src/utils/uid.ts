let uid = 0

export function nextUid(prefix = 'uid'): string {
  uid += 1
  return `${prefix}-${uid}`
}
