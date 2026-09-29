import { ref, computed } from 'vue'
import api from '../../../api'
import { toast } from '../../../utils/toast'
import type { UserItem } from '../types'
import { UserSubscriptionService } from '../services/subscription'

export type StatusFilterType = 'all' | 'online' | 'enabled' | 'disabled' | 'expired' | 'overquota'

export function useUserList() {
  const users = ref<UserItem[]>([])
  const availableInbounds = ref<any[]>([])
  const selectedUserIds = ref<number[]>([])
  const loading = ref(false)

  // Search & Filter
  const searchQuery = ref('')
  const statusFilter = ref<StatusFilterType>('all')

  // KPI Metrics
  const onlineUsersCount = computed(() => users.value.filter((u) => u.isOnline).length)
  const enabledUsersCount = computed(() => users.value.filter((u) => u.enabled).length)
  const attentionUsersCount = computed(() => {
    const now = Date.now()
    return users.value.filter((u) => {
      const expired = u.expireTime > 0 && u.expireTime < now
      const overquota = u.totalBytes > 0 && (u.upBytes + u.downBytes) >= u.totalBytes
      const disabled = !u.enabled
      return expired || overquota || disabled
    }).length
  })

  // Status Checkers
  const isUserExpired = (user: UserItem): boolean => {
    return user.expireTime > 0 && user.expireTime < Date.now()
  }

  const isUserOverQuota = (user: UserItem): boolean => {
    return user.totalBytes > 0 && (user.upBytes + user.downBytes) >= user.totalBytes
  }

  const getUserBadgeVariant = (user: UserItem): 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive' => {
    if (!user.enabled) return 'destructive'
    if (isUserExpired(user)) return 'warning'
    if (isUserOverQuota(user)) return 'destructive'
    if (user.isOnline) return 'success'
    return 'secondary'
  }

  const getUserStatusText = (user: UserItem): string => {
    if (!user.enabled) return '已停用'
    if (isUserExpired(user)) return '已到期'
    if (isUserOverQuota(user)) return '已超额'
    if (user.isOnline) return '在线传输'
    return '正常运行'
  }

  const getTrafficPercent = (user?: UserItem | null): number => {
    if (!user || !user.totalBytes) return 0
    const used = user.upBytes + user.downBytes
    return (used / user.totalBytes) * 100
  }

  const getTrafficProgressClass = (user: UserItem): string => {
    const percent = getTrafficPercent(user)
    if (percent >= 90) return 'bg-rose-500'
    if (percent >= 75) return 'bg-amber-400'
    return 'bg-foreground'
  }

  // Filtered List
  const filteredUsers = computed(() => {
    const query = searchQuery.value.trim().toLowerCase()
    const now = Date.now()

    return users.value.filter((user) => {
      // 状态过滤
      if (statusFilter.value === 'online' && !user.isOnline) return false
      if (statusFilter.value === 'enabled' && !user.enabled) return false
      if (statusFilter.value === 'disabled' && user.enabled) return false
      if (statusFilter.value === 'expired') {
        const isExpired = user.expireTime > 0 && user.expireTime < now
        if (!isExpired) return false
      }
      if (statusFilter.value === 'overquota') {
        const isOver = user.totalBytes > 0 && (user.upBytes + user.downBytes) >= user.totalBytes
        if (!isOver) return false
      }

      // 搜索过滤
      if (!query) return true
      const emailMatch = user.email?.toLowerCase().includes(query)
      const uuidMatch = user.uuid?.toLowerCase().includes(query)
      const subTokenMatch = user.subToken?.toLowerCase().includes(query)
      const inboundMatch = UserSubscriptionService.getNodeTags(user, availableInbounds.value).some((tag) =>
        tag.toLowerCase().includes(query)
      )
      return emailMatch || uuidMatch || subTokenMatch || inboundMatch
    })
  })

  // Selection
  const isAllUsersSelected = computed(() => {
    if (!filteredUsers.value.length) return false
    return filteredUsers.value.every((u) => selectedUserIds.value.includes(u.id))
  })

  const toggleSelectAllUsers = () => {
    if (isAllUsersSelected.value) {
      const currentIds = new Set(filteredUsers.value.map((u) => u.id))
      selectedUserIds.value = selectedUserIds.value.filter((id) => !currentIds.has(id))
    } else {
      const currentIds = new Set(selectedUserIds.value)
      for (const u of filteredUsers.value) {
        currentIds.add(u.id)
      }
      selectedUserIds.value = Array.from(currentIds)
    }
  }

  // API Methods
  const fetchAll = async () => {
    loading.value = true
    try {
      const [uRes, inbRes]: any = await Promise.all([api.get('/users'), api.get('/inbounds')])
      users.value = uRes || []
      availableInbounds.value = inbRes || []
    } catch (err) {
      console.error(err)
    } finally {
      loading.value = false
    }
  }

  const fetchSpeeds = async () => {
    if (document.hidden) return
    try {
      const res: any = await api.get('/users/speeds')
      if (res && users.value.length) {
        for (const u of users.value) {
          const s = res[u.email]
          if (s) {
            u.upSpeed = s.upSpeed || 0
            u.downSpeed = s.downSpeed || 0
            u.isOnline = !!s.isOnline
          } else {
            u.upSpeed = 0
            u.downSpeed = 0
            u.isOnline = false
          }
        }
      }
    } catch (_err) {
      // 保证轮询稳定性
    }
  }

  // Single / Batch Operations
  const batchRenew = async (days: number) => {
    if (!selectedUserIds.value.length) return
    if (!confirm(`确定为选中的 ${selectedUserIds.value.length} 位用户统一延期 ${days} 天吗？`)) return
    try {
      await api.post('/users/batch-renew', {
        ids: selectedUserIds.value,
        days,
      })
      toast.success(`成功为选中用户批量延期 ${days} 天！`)
      selectedUserIds.value = []
      await fetchAll()
    } catch (err: any) {
      toast.error('批量延期失败: ' + err)
    }
  }

  const batchRenewOne = async (userId: number, days: number) => {
    try {
      await api.post('/users/batch-renew', {
        ids: [userId],
        days,
      })
      toast.success(`已成功延期 ${days} 天！`)
      await fetchAll()
    } catch (err: any) {
      toast.error('延期失败: ' + err)
    }
  }

  const batchResetTraffic = async () => {
    if (!selectedUserIds.value.length) return
    if (!confirm(`确定重置选中的 ${selectedUserIds.value.length} 位用户的已用上下行流量吗？`)) return
    try {
      await api.post('/users/batch-reset-traffic', {
        ids: selectedUserIds.value,
      })
      toast.success('已成功重置选中用户的已用流量！')
      selectedUserIds.value = []
      await fetchAll()
    } catch (err: any) {
      toast.error('批量重置流量失败: ' + err)
    }
  }

  const batchSetStatus = async (enabled: boolean) => {
    if (!selectedUserIds.value.length) return
    const action = enabled ? '启用' : '禁用'
    if (!confirm(`确定批量${action}选中的 ${selectedUserIds.value.length} 位用户吗？`)) return
    try {
      await api.post('/users/batch-status', {
        ids: selectedUserIds.value,
        enabled,
      })
      toast.success(`已成功批量${action}选中用户！`)
      selectedUserIds.value = []
      await fetchAll()
    } catch (err: any) {
      toast.error(`批量${action}失败: ` + err)
    }
  }

  const batchDeleteUsers = async () => {
    if (!selectedUserIds.value.length) return
    if (!confirm(`确定批量删除选中的 ${selectedUserIds.value.length} 位用户吗？此操作不可逆！`)) return
    try {
      for (const id of selectedUserIds.value) {
        await api.delete(`/users/${id}`)
      }
      toast.success('已成功批量删除选中用户！')
      selectedUserIds.value = []
      await fetchAll()
    } catch (err: any) {
      toast.error('批量删除失败: ' + err)
    }
  }

  const toggleUserEnabled = async (user: UserItem) => {
    try {
      await api.post('/users/batch-status', {
        ids: [user.id],
        enabled: !user.enabled,
      })
      toast.success(`用户已${!user.enabled ? '启用' : '停用'}！`)
      await fetchAll()
    } catch (err: any) {
      toast.error('切换状态失败: ' + err)
    }
  }

  const deleteUser = async (id: number) => {
    if (!confirm('确定删除该用户并将其从所有节点下线吗？')) return false
    try {
      await api.delete(`/users/${id}`)
      toast.success('用户已成功删除！')
      await fetchAll()
      return true
    } catch (err: any) {
      toast.error('删除失败: ' + err)
      return false
    }
  }

  const resetTraffic = async (id: number) => {
    if (!confirm('确定重置该用户的上下行流量吗？')) return
    try {
      await api.post(`/users/${id}/reset-traffic`)
      toast.success('用户已用流量已重置为 0！')
      await fetchAll()
    } catch (err: any) {
      toast.error('重置失败: ' + err)
    }
  }

  const resetUserSubToken = async (userId: number): Promise<{ subToken?: string; uuid?: string } | null> => {
    if (!confirm('确定重置该用户的订阅与连接密钥吗？所有旧设备将立即断开连接，旧订阅链接也将失效！')) return null
    try {
      const res: any = await api.post(`/users/${userId}/reset-token`)
      toast.success('订阅与密钥重置成功，旧设备已断开！')
      await fetchAll()
      return res
    } catch (err: any) {
      toast.error('重置失败: ' + err)
      return null
    }
  }

  return {
    users,
    availableInbounds,
    selectedUserIds,
    loading,
    searchQuery,
    statusFilter,
    filteredUsers,
    isAllUsersSelected,
    onlineUsersCount,
    enabledUsersCount,
    attentionUsersCount,
    isUserExpired,
    isUserOverQuota,
    getUserBadgeVariant,
    getUserStatusText,
    getTrafficPercent,
    getTrafficProgressClass,
    toggleSelectAllUsers,
    fetchAll,
    fetchSpeeds,
    batchRenew,
    batchRenewOne,
    batchResetTraffic,
    batchSetStatus,
    batchDeleteUsers,
    toggleUserEnabled,
    deleteUser,
    resetTraffic,
    resetUserSubToken,
  }
}
