<template>
  <div class="space-y-4">
    <!-- Top Action & Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">用户与多节点归属管理</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ filteredUsers.length }} / {{ users.length }} 位用户
          </Badge>
          <Badge variant="success" :dot="true" class="text-[10px] hidden sm:inline-flex">
            gRPC 毫秒内存热生效
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          支持批量延期与流量重置、独立安全订阅 Token、每月周期自动重置与并发设备限制
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          variant="default"
          size="sm"
          @click="openAddModal"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加用户</span>
        </Button>
      </div>
    </div>

    <!-- Quick Stats Cards (KPI Metrics) -->
    <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5">
      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>总用户数</span>
          <Users class="w-3.5 h-3.5" />
        </div>
        <div class="text-base font-bold font-mono text-foreground">
          {{ users.length }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>在线传输</span>
          <Activity class="w-3.5 h-3.5 text-emerald-400" />
        </div>
        <div class="text-base font-bold font-mono text-emerald-400">
          {{ onlineUsersCount }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>正常启用</span>
          <Check class="w-3.5 h-3.5 text-foreground" />
        </div>
        <div class="text-base font-bold font-mono text-foreground">
          {{ enabledUsersCount }}
        </div>
      </div>

      <div class="rounded-lg border border-border bg-card p-3 space-y-1">
        <div class="flex items-center justify-between text-muted-foreground text-[11px]">
          <span>临期/超额/停用</span>
          <AlertCircle class="w-3.5 h-3.5 text-amber-400" />
        </div>
        <div class="text-base font-bold font-mono text-amber-400">
          {{ attentionUsersCount }}
        </div>
      </div>
    </div>

    <!-- Filter & Search Bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
      <div class="flex flex-1 items-center gap-2 max-w-lg">
        <div class="relative w-full">
          <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-muted-foreground" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="搜索用户名 / 邮箱 / UUID / Token / 节点..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <select
          v-model="statusFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部状态</option>
          <option value="online">在线传输</option>
          <option value="enabled">已启用</option>
          <option value="disabled">已停用</option>
          <option value="expired">已到期</option>
          <option value="overquota">已超额</option>
        </select>
      </div>

      <div v-if="selectedUserIds.length > 0" class="flex items-center gap-2 text-xs">
        <span class="text-muted-foreground font-mono">已选 {{ selectedUserIds.length }} 位</span>
      </div>
    </div>

    <!-- Batch Action Floating / Fixed Bar (Neutral Console Style) -->
    <div
      v-if="selectedUserIds.length > 0"
      class="rounded-lg border border-border bg-card p-3 flex flex-wrap items-center justify-between gap-3 shadow-sm transition-all"
    >
      <div class="flex items-center gap-2 text-xs">
        <Badge variant="default" class="font-mono">
          已勾选 {{ selectedUserIds.length }} 位用户
        </Badge>
        <span class="text-muted-foreground hidden sm:inline">批量执行运维指令：</span>
      </div>

      <div class="flex flex-wrap items-center gap-1.5">
        <Button
          variant="secondary"
          size="sm"
          @click="batchRenew(30)"
          class="text-xs font-mono"
        >
          <CalendarPlus class="w-3.5 h-3.5 mr-1 text-emerald-400" />
          <span>+30天</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="batchRenew(90)"
          class="text-xs font-mono"
        >
          <CalendarPlus class="w-3.5 h-3.5 mr-1 text-emerald-400" />
          <span>+90天</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="batchRenew(365)"
          class="text-xs font-mono"
        >
          <CalendarPlus class="w-3.5 h-3.5 mr-1 text-emerald-400" />
          <span>+365天</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="batchResetTraffic"
          class="text-xs font-mono"
        >
          <RotateCcw class="w-3.5 h-3.5 mr-1 text-cyan-400" />
          <span>重置流量</span>
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="batchSetStatus(true)"
          class="text-xs font-mono"
        >
          启用
        </Button>
        <Button
          variant="secondary"
          size="sm"
          @click="batchSetStatus(false)"
          class="text-xs font-mono"
        >
          停用
        </Button>
        <Button
          variant="destructive"
          size="sm"
          @click="batchDeleteUsers"
          class="text-xs font-mono"
        >
          <Trash2 class="w-3.5 h-3.5 mr-1" />
          <span>批量删除</span>
        </Button>
        <Button
          variant="ghost"
          size="sm"
          @click="selectedUserIds = []"
          class="text-xs text-muted-foreground hover:text-foreground"
        >
          取消
        </Button>
      </div>
    </div>

    <!-- Main Table View (Table-First) -->
    <Table>
      <TableHeader>
        <TableRow class="hover:bg-transparent cursor-default">
          <TableHead class="w-10 text-center">
            <input
              type="checkbox"
              :checked="isAllUsersSelected"
              @change="toggleSelectAllUsers"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
          </TableHead>
          <TableHead class="min-w-[180px]">用户名 / 标识</TableHead>
          <TableHead class="min-w-[140px]">授权入站节点</TableHead>
          <TableHead class="min-w-[160px]">流量配额</TableHead>
          <TableHead class="min-w-[130px]">有效期与重置</TableHead>
          <TableHead class="min-w-[120px]">状态与实时速率</TableHead>
          <TableHead class="text-right min-w-[170px]">操作</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        <TableRow
          v-for="user in filteredUsers"
          :key="user.id"
          @click="inspectUser(user)"
          :class="{ 'bg-muted/30': currentInspectorUser?.id === user.id }"
        >
          <!-- Checkbox -->
          <TableCell class="text-center" @click.stop>
            <input
              type="checkbox"
              :value="user.id"
              v-model="selectedUserIds"
              class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
            />
          </TableCell>

          <!-- Username & Email & SubToken -->
          <TableCell>
            <div class="font-mono font-medium text-foreground text-xs truncate max-w-[220px]">
              {{ user.email }}
            </div>
            <div class="text-[10px] text-muted-foreground font-mono flex items-center gap-1.5 mt-0.5">
              <span v-if="user.ipLimit > 0" class="text-amber-400 font-semibold">限 {{ user.ipLimit }} IP</span>
              <span v-else>无IP限制</span>
              <span>•</span>
              <span class="truncate max-w-[120px]" :title="user.subToken || '未生成'">
                Token: {{ user.subToken ? user.subToken.substring(0, 8) + '...' : '未生成' }}
              </span>
            </div>
          </TableCell>

          <!-- Inbound Tags -->
          <TableCell>
            <div class="flex flex-wrap gap-1 max-w-[200px]">
              <Badge
                v-for="tag in getNodeTags(user).slice(0, 3)"
                :key="tag"
                variant="outline"
                class="text-[10px] px-1.5 py-0 font-mono"
              >
                <span>{{ tag }}</span>
                <span v-if="getSubRoutesCount(tag)" class="text-cyan-400 font-bold ml-0.5">({{ getSubRoutesCount(tag) }}线)</span>
              </Badge>
              <Badge
                v-if="getNodeTags(user).length > 3"
                variant="secondary"
                class="text-[10px] px-1 py-0 font-mono"
                :title="getNodeTags(user).slice(3).join(', ')"
              >
                +{{ getNodeTags(user).length - 3 }}
              </Badge>
              <span v-if="!getNodeTags(user).length" class="text-[11px] text-muted-foreground font-mono">
                未分配节点
              </span>
            </div>
          </TableCell>

          <!-- Traffic Usage & Progress -->
          <TableCell class="font-mono">
            <div class="space-y-1">
              <div class="text-xs text-foreground flex items-center justify-between">
                <span>{{ formatBytes(user.upBytes + user.downBytes) }}</span>
                <span class="text-muted-foreground text-[11px]">/ {{ user.totalBytes > 0 ? formatBytes(user.totalBytes) : '无限制' }}</span>
              </div>
              <div v-if="user.totalBytes > 0" class="w-full max-w-[140px] h-1.5 bg-neutral-900 rounded-full overflow-hidden border border-border/40">
                <div
                  class="h-full rounded-full transition-all"
                  :class="getTrafficProgressClass(user)"
                  :style="{ width: `${Math.min(100, getTrafficPercent(user))}%` }"
                />
              </div>
            </div>
          </TableCell>

          <!-- Expire Date & Reset -->
          <TableCell class="font-mono text-[11px]">
            <div :class="isUserExpired(user) ? 'text-amber-400 font-semibold' : 'text-foreground'">
              {{ user.expireTime > 0 ? formatDate(user.expireTime) : '永久有效' }}
              <span v-if="isUserExpired(user)" class="text-[10px] text-rose-400 ml-1">(已到期)</span>
            </div>
            <div v-if="user.resetDay > 0" class="text-[10px] text-cyan-400/90 mt-0.5">
              每月 {{ user.resetDay }} 日自动重置
            </div>
          </TableCell>

          <!-- Status & Live Speeds -->
          <TableCell>
            <div class="space-y-1">
              <div class="flex items-center gap-1.5">
                <Badge
                  :variant="getUserBadgeVariant(user)"
                  :dot="true"
                  class="text-[10px]"
                >
                  {{ getUserStatusText(user) }}
                </Badge>
              </div>
              <!-- Real-time transfer speed -->
              <div v-if="user.isOnline && (user.upSpeed > 0 || user.downSpeed > 0)" class="text-[10px] font-mono text-cyan-400 flex items-center gap-1">
                <span>↑{{ formatBytes(user.upSpeed) }}/s</span>
                <span>↓{{ formatBytes(user.downSpeed) }}/s</span>
              </div>
            </div>
          </TableCell>

          <!-- Quick Action Buttons -->
          <TableCell class="text-right" @click.stop>
            <div class="flex items-center justify-end gap-1">
              <Button
                variant="ghost"
                size="icon"
                @click="inspectUser(user)"
                title="打开巡检抽屉"
              >
                <SlidersHorizontal class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="copyText(getDirectTokenSubUrl(user))"
                title="复制聚合订阅链接"
              >
                <Copy class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="openShareModal(user)"
                title="二维码与安全提取码"
              >
                <QrCode class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="openHistoryModal(user)"
                title="查看流量历史趋势"
              >
                <BarChart2 class="w-3.5 h-3.5 text-cyan-400" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                @click="openEditModal(user)"
                title="编辑用户配置"
              >
                <Edit class="w-3.5 h-3.5" />
              </Button>
              <Button
                variant="ghost"
                size="icon"
                class="text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                @click="deleteUser(user.id)"
                title="删除用户"
              >
                <Trash2 class="w-3.5 h-3.5" />
              </Button>
            </div>
          </TableCell>
        </TableRow>

        <!-- Empty State -->
        <TableRow v-if="!filteredUsers.length" class="hover:bg-transparent">
          <TableCell colspan="7" class="p-10 text-center text-muted-foreground">
            <div class="flex flex-col items-center justify-center gap-2">
              <Users class="w-8 h-8 text-neutral-600 mb-1" />
              <p class="text-xs font-medium text-neutral-300">没有匹配的用户记录</p>
              <p class="text-[11px] text-muted-foreground">可尝试调整搜索关键字或筛选条件，或点击右上角添加新用户</p>
            </div>
          </TableCell>
        </TableRow>
      </TableBody>
    </Table>

    <!-- User Inspector Drawer (Right Sheet) -->
    <Drawer
      v-model="showInspectorDrawer"
      :title="`用户巡检: ${currentInspectorUser?.email || ''}`"
      :description="`UUID: ${currentInspectorUser?.uuid || '未分配'} · 独立 Token 聚合订阅`"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <div v-if="currentInspectorUser" class="space-y-4 text-xs">
        <!-- 1. Quick Action & Status Header Card -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">实时状态与快捷操作</span>
            <div class="flex items-center gap-1.5">
              <Badge :variant="getUserBadgeVariant(currentInspectorUser)" :dot="true">
                {{ getUserStatusText(currentInspectorUser) }}
              </Badge>
              <Badge v-if="currentInspectorUser.isOnline" variant="success" class="text-[10px]">
                ↑{{ formatBytes(currentInspectorUser.upSpeed) }}/s ↓{{ formatBytes(currentInspectorUser.downSpeed) }}/s
              </Badge>
            </div>
          </div>

          <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1">
            <Button
              variant="outline"
              size="sm"
              @click="copyText(getDirectTokenSubUrl(currentInspectorUser))"
              class="w-full justify-center"
            >
              <Copy class="w-3.5 h-3.5 mr-1" />
              <span>复制订阅</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              @click="openShareModal(currentInspectorUser)"
              class="w-full justify-center"
            >
              <QrCode class="w-3.5 h-3.5 mr-1" />
              <span>二维码/凭据</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              @click="openHistoryModal(currentInspectorUser)"
              class="w-full justify-center"
            >
              <BarChart2 class="w-3.5 h-3.5 mr-1 text-cyan-400" />
              <span>流量趋势</span>
            </Button>
            <Button
              variant="outline"
              size="sm"
              @click="toggleUserEnabled(currentInspectorUser)"
              :class="currentInspectorUser.enabled ? 'hover:text-rose-400' : 'hover:text-emerald-400'"
              class="w-full justify-center"
            >
              <span>{{ currentInspectorUser.enabled ? '停用账号' : '启用账号' }}</span>
            </Button>
          </div>
        </div>

        <!-- 2. Quota & Bandwidth Diagnostics -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">流量配额与消耗</span>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 text-[11px] px-2 text-cyan-400"
              @click="resetTraffic(currentInspectorUser.id)"
            >
              <RotateCcw class="w-3 h-3 mr-1" /> 重置流量
            </Button>
          </div>

          <div class="grid grid-cols-2 sm:grid-cols-4 gap-3 font-mono">
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">已用总计</div>
              <div class="text-sm font-semibold text-foreground mt-0.5">
                {{ formatBytes(currentInspectorUser.upBytes + currentInspectorUser.downBytes) }}
              </div>
            </div>
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">配额限额</div>
              <div class="text-sm font-semibold text-foreground mt-0.5">
                {{ currentInspectorUser.totalBytes > 0 ? formatBytes(currentInspectorUser.totalBytes) : '无限制' }}
              </div>
            </div>
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">上行上传</div>
              <div class="text-sm font-semibold text-emerald-400 mt-0.5">
                {{ formatBytes(currentInspectorUser.upBytes) }}
              </div>
            </div>
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">下行下载</div>
              <div class="text-sm font-semibold text-cyan-400 mt-0.5">
                {{ formatBytes(currentInspectorUser.downBytes) }}
              </div>
            </div>
          </div>

          <div v-if="currentInspectorUser.totalBytes > 0" class="space-y-1.5 pt-1">
            <div class="flex items-center justify-between text-[11px] font-mono">
              <span class="text-muted-foreground">配额使用率</span>
              <span :class="getTrafficPercent(currentInspectorUser) > 90 ? 'text-rose-400 font-bold' : 'text-foreground'">
                {{ getTrafficPercent(currentInspectorUser).toFixed(1) }}%
              </span>
            </div>
            <div class="w-full h-2 bg-neutral-900 rounded-full overflow-hidden border border-border/40">
              <div
                class="h-full rounded-full transition-all"
                :class="getTrafficProgressClass(currentInspectorUser)"
                :style="{ width: `${Math.min(100, getTrafficPercent(currentInspectorUser))}%` }"
              />
            </div>
          </div>
        </div>

        <!-- 3. Lifecycle & Cycle Settings -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">有效期与重置周期</span>
            <div class="flex items-center gap-1">
              <Button
                variant="ghost"
                size="sm"
                class="h-6 text-[11px] px-2 text-emerald-400"
                @click="batchRenewOne(currentInspectorUser.id, 30)"
              >
                +30天
              </Button>
              <Button
                variant="ghost"
                size="sm"
                class="h-6 text-[11px] px-2 text-emerald-400"
                @click="batchRenewOne(currentInspectorUser.id, 90)"
              >
                +90天
              </Button>
            </div>
          </div>

          <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 font-mono">
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">账号到期日</div>
              <div class="text-xs font-semibold text-foreground mt-0.5">
                {{ currentInspectorUser.expireTime > 0 ? formatDate(currentInspectorUser.expireTime) : '永久有效' }}
              </div>
            </div>
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">每月自动重置</div>
              <div class="text-xs font-semibold text-cyan-400 mt-0.5">
                {{ currentInspectorUser.resetDay > 0 ? `每月 ${currentInspectorUser.resetDay} 日` : '不自动重置' }}
              </div>
            </div>
            <div>
              <div class="text-[10px] text-muted-foreground uppercase">并发设备限额</div>
              <div class="text-xs font-semibold text-foreground mt-0.5">
                {{ currentInspectorUser.ipLimit > 0 ? `限制 ${currentInspectorUser.ipLimit} IP` : '无限制' }}
              </div>
            </div>
          </div>
        </div>

        <!-- 4. Security & Credentials -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">认证凭据与密钥</span>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 text-[11px] px-2 text-rose-400"
              @click="resetUserSubToken(currentInspectorUser.id)"
            >
              <RotateCcw class="w-3 h-3 mr-1" /> 重置 Token/密钥
            </Button>
          </div>

          <div class="space-y-2.5 font-mono">
            <div>
              <label class="text-[10px] text-muted-foreground uppercase block mb-1">UUID / 密码</label>
              <div class="flex items-center gap-1.5">
                <input
                  :value="currentInspectorUser.uuid"
                  readonly
                  class="flex-1 bg-neutral-950 border border-border rounded-md px-2.5 py-1 text-xs text-foreground font-mono select-all focus:outline-none"
                />
                <Button variant="secondary" size="sm" class="h-7 px-2" @click="copyText(currentInspectorUser.uuid)">
                  <Copy class="w-3 h-3" />
                </Button>
              </div>
            </div>

            <div>
              <label class="text-[10px] text-muted-foreground uppercase block mb-1">订阅安全 Token</label>
              <div class="flex items-center gap-1.5">
                <input
                  :value="currentInspectorUser.subToken || '未生成'"
                  readonly
                  class="flex-1 bg-neutral-950 border border-border rounded-md px-2.5 py-1 text-xs text-foreground font-mono select-all focus:outline-none"
                />
                <Button
                  v-if="currentInspectorUser.subToken"
                  variant="secondary"
                  size="sm"
                  class="h-7 px-2"
                  @click="copyText(currentInspectorUser.subToken)"
                >
                  <Copy class="w-3 h-3" />
                </Button>
              </div>
            </div>

            <div>
              <label class="text-[10px] text-muted-foreground uppercase block mb-1">通用聚合订阅 URL</label>
              <div class="flex items-center gap-1.5">
                <input
                  :value="getDirectTokenSubUrl(currentInspectorUser)"
                  readonly
                  class="flex-1 bg-neutral-950 border border-border rounded-md px-2.5 py-1 text-xs text-foreground font-mono select-all focus:outline-none"
                />
                <Button variant="secondary" size="sm" class="h-7 px-2" @click="copyText(getDirectTokenSubUrl(currentInspectorUser))">
                  <Copy class="w-3 h-3" />
                </Button>
              </div>
            </div>
          </div>
        </div>

        <!-- 5. Associated Inbounds Matrix -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">
              已授权入站网关 ({{ getNodeTags(currentInspectorUser).length }})
            </span>
            <Button
              variant="ghost"
              size="sm"
              class="h-6 text-[11px] px-2 text-foreground"
              @click="openEditModal(currentInspectorUser)"
            >
              <Edit class="w-3 h-3 mr-1" /> 变更授权节点
            </Button>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <div
              v-for="tag in getNodeTags(currentInspectorUser)"
              :key="tag"
              class="p-2.5 rounded-md border border-border bg-neutral-950 font-mono flex items-center justify-between"
            >
              <div class="min-w-0 pr-2">
                <div class="text-xs font-semibold text-foreground truncate">{{ tag }}</div>
                <div class="text-[10px] text-muted-foreground mt-0.5">
                  <span v-if="getInboundMeta(tag)">{{ getInboundMeta(tag)?.protocol?.toUpperCase() }} :{{ getInboundMeta(tag)?.port }}</span>
                  <span v-else>自定义接入点</span>
                  <span v-if="getSubRoutesCount(tag)" class="text-cyan-400 font-bold ml-1.5">({{ getSubRoutesCount(tag) }}条线路)</span>
                </div>
              </div>
              <Badge variant="outline" class="text-[10px] shrink-0 font-mono">
                已授权
              </Badge>
            </div>
            <div v-if="!getNodeTags(currentInspectorUser).length" class="col-span-2 text-center py-4 text-muted-foreground text-xs font-mono">
              暂未绑定入站节点，请点击编辑进行关联
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="flex items-center justify-between w-full">
          <Button
            variant="destructive"
            size="sm"
            @click="deleteUser(currentInspectorUser.id)"
          >
            <Trash2 class="w-3.5 h-3.5 mr-1" />
            <span>删除用户</span>
          </Button>
          <div class="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              @click="showInspectorDrawer = false"
            >
              关闭
            </Button>
            <Button
              variant="default"
              size="sm"
              @click="openEditModal(currentInspectorUser)"
            >
              <Edit class="w-3.5 h-3.5 mr-1" />
              <span>编辑配置</span>
            </Button>
          </div>
        </div>
      </template>
    </Drawer>

    <!-- User Form Drawer (Add/Edit User) -->
    <Drawer
      v-model="showModal"
      :title="isEditing ? `编辑用户: ${form.email}` : '新建用户'"
      :description="isEditing ? '调整用户的授权节点、配额策略与有效期' : '创建新凭据并自动下发同步至核心节点'"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <form @submit.prevent="saveUser" id="userForm" class="space-y-4 text-xs">
        <!-- 1. Authentication -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider block border-b border-border/60 pb-2">
            1. 基础认证与身份
          </span>

          <div class="space-y-3">
            <div>
              <label class="block text-foreground font-semibold mb-1">
                用户名 / 邮箱 <span class="text-rose-400">*</span>
              </label>
              <input
                v-model="form.email"
                type="text"
                required
                :disabled="isEditing"
                placeholder="user@example.com 或 纯用户名"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-60"
              />
              <p class="text-[10px] text-muted-foreground mt-1">创建后将作为订阅唯一索引与 Xray 客户端身份标记</p>
            </div>

            <div class="flex items-center gap-2 pt-1">
              <input
                type="checkbox"
                id="form-enabled"
                v-model="form.enabled"
                class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
              />
              <label for="form-enabled" class="text-foreground font-medium cursor-pointer select-none">
                账号处于启用状态（禁用后将立即从节点断开）
              </label>
            </div>
          </div>
        </div>

        <!-- 2. Authorized Inbounds -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">
              2. 授权入站节点 (Inbounds) <span class="text-rose-400">*</span>
            </span>
            <button
              type="button"
              @click="toggleSelectAllInbounds"
              class="text-[11px] text-foreground hover:underline font-mono"
            >
              {{ isAllInboundsSelected ? '取消全选' : '全选所有节点' }}
            </button>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-44 overflow-y-auto p-1">
            <label
              v-for="inb in availableInbounds"
              :key="inb.id"
              class="flex items-center gap-2 p-2.5 rounded-md border border-border bg-neutral-950 hover:bg-muted/40 cursor-pointer transition-colors"
              :class="{ 'border-neutral-500 bg-muted/30': form.selectedTags.includes(inb.tag) }"
            >
              <input
                type="checkbox"
                :value="inb.tag"
                v-model="form.selectedTags"
                class="rounded bg-neutral-900 border-border text-foreground focus:ring-0 cursor-pointer"
              />
              <div class="min-w-0 font-mono">
                <div class="text-xs font-semibold text-foreground truncate">{{ inb.tag }}</div>
                <div class="text-[10px] text-muted-foreground uppercase">{{ inb.protocol }} :{{ inb.port }}</div>
              </div>
            </label>
          </div>
        </div>

        <!-- 3. Quota & Lifecycle Policy -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider block border-b border-border/60 pb-2">
            3. 配额限额与计费周期策略
          </span>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-foreground font-semibold mb-1">流量限额 (GB)</label>
              <input
                v-model.number="form.totalGB"
                type="number"
                min="0"
                step="0.01"
                placeholder="0 为无限制"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <p class="text-[10px] text-muted-foreground mt-1">达到配额后自动切断连接</p>
            </div>

            <div v-if="!isEditing">
              <label class="block text-foreground font-semibold mb-1">初始有效天数</label>
              <input
                v-model.number="form.expireDays"
                type="number"
                min="0"
                placeholder="0 为永久有效，如 30"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <p class="text-[10px] text-muted-foreground mt-1">从创建当前时刻起计算</p>
            </div>

            <div v-else>
              <label class="block text-foreground font-semibold mb-1">延长有效天数 (+天)</label>
              <input
                v-model.number="form.extendDays"
                type="number"
                min="0"
                placeholder="如增加 30 天"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <p class="text-[10px] text-muted-foreground mt-1">在当前有效期基础上顺延</p>
            </div>

            <div>
              <label class="block text-foreground font-semibold mb-1">每月重置流量日</label>
              <input
                v-model.number="form.resetDay"
                type="number"
                min="0"
                max="31"
                placeholder="0-31 (如每月1号清零)"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <p class="text-[10px] text-muted-foreground mt-1">0 为不按月重置流量</p>
            </div>

            <div>
              <label class="block text-foreground font-semibold mb-1">并发连接设备数 (IP)</label>
              <input
                v-model.number="form.ipLimit"
                type="number"
                min="0"
                max="100"
                placeholder="0 为不限制"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-xs text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <p class="text-[10px] text-muted-foreground mt-1">限制同时在线客户端 IP 数</p>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <Button
          variant="outline"
          size="sm"
          @click="showModal = false"
        >
          取消
        </Button>
        <Button
          variant="default"
          size="sm"
          type="submit"
          form="userForm"
          :loading="saving"
        >
          {{ saving ? '保存中...' : '确认保存' }}
        </Button>
      </template>
    </Drawer>

    <!-- Share & Subscription Modal (Neutral Console Style) -->
    <Teleport to="body">
      <div v-if="showShareModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75">
        <div class="bg-neutral-950 border border-border rounded-lg shadow-2xl w-full max-w-lg overflow-hidden flex flex-col">
          <!-- Modal Header -->
          <div class="h-14 px-5 border-b border-border flex items-center justify-between shrink-0 bg-card">
            <div>
              <h3 class="text-sm font-semibold text-foreground flex items-center gap-1.5">
                <Zap class="w-4 h-4 text-amber-400" />
                <span>全节点订阅与安全凭据</span>
              </h3>
              <p class="text-[11px] text-muted-foreground mt-0.5 font-mono">
                {{ currentShareData?.user?.email }}
              </p>
            </div>
            <button
              type="button"
              @click="showShareModal = false"
              class="h-7 w-7 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/60 flex items-center justify-center transition-colors"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <!-- Tabs Header -->
          <div class="flex border-b border-border bg-neutral-900 px-4 pt-2 gap-2 text-xs font-mono">
            <button
              type="button"
              @click="activeShareTab = 'link'"
              class="pb-2 px-2.5 font-medium border-b-2 transition-colors flex items-center gap-1.5"
              :class="activeShareTab === 'link' ? 'border-foreground text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
            >
              <Copy class="w-3.5 h-3.5" />
              <span>订阅链接</span>
            </button>
            <button
              type="button"
              @click="activeShareTab = 'qrcode'"
              class="pb-2 px-2.5 font-medium border-b-2 transition-colors flex items-center gap-1.5"
              :class="activeShareTab === 'qrcode' ? 'border-foreground text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
            >
              <QrCode class="w-3.5 h-3.5" />
              <span>手机扫码</span>
            </button>
            <button
              type="button"
              @click="activeShareTab = 'ticket'"
              class="pb-2 px-2.5 font-medium border-b-2 transition-colors flex items-center gap-1.5"
              :class="activeShareTab === 'ticket' ? 'border-foreground text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
            >
              <ShieldCheck class="w-3.5 h-3.5" />
              <span>安全提取码 (Ticket)</span>
            </button>
          </div>

          <!-- Modal Body -->
          <div v-if="currentShareData" class="p-5 space-y-4 text-xs max-h-[75vh] overflow-y-auto">
            <!-- 1. 二维码模式 -->
            <div v-if="activeShareTab === 'qrcode'" class="text-center py-4 space-y-3">
              <div class="inline-block p-4 bg-white rounded-lg shadow-sm border border-border">
                <qrcode-vue
                  :value="getDirectTokenSubUrl(currentShareData.user)"
                  :size="180"
                  level="M"
                  render-as="svg"
                />
              </div>
              <p class="text-[11px] text-muted-foreground">
                支持 Shadowrocket / Clash / V2Ray / Sing-box 等主流客户端相机直接扫码添加
              </p>
            </div>

            <!-- 2. 安全提取码模式 -->
            <div v-else-if="activeShareTab === 'ticket'" class="space-y-4">
              <div class="rounded-lg border border-border bg-card p-4 space-y-3">
                <div class="flex items-center justify-between">
                  <span class="font-bold text-foreground flex items-center gap-1.5">
                    <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
                    <span>带外安全分发 (阅后即焚凭证)</span>
                  </span>
                  <Badge variant="outline" class="text-[10px]">抗审查通道</Badge>
                </div>
                <p class="text-[11px] text-muted-foreground leading-relaxed">
                  生成 6 位高熵中立提取码，支持境内即时通讯安全发送。接收方在分布式网管接入点输入即可获取节点。
                </p>

                <div class="grid grid-cols-2 gap-3 pt-1">
                  <div>
                    <label class="block text-[11px] text-muted-foreground mb-1 font-mono">有效时长</label>
                    <select
                      v-model="ticketTTL"
                      class="w-full bg-neutral-950 border border-border rounded-md px-2.5 h-8 text-xs text-foreground font-mono focus:outline-none"
                    >
                      <option :value="15">15 分钟 (推荐)</option>
                      <option :value="30">30 分钟</option>
                      <option :value="60">1 小时</option>
                    </select>
                  </div>
                  <div>
                    <label class="block text-[11px] text-muted-foreground mb-1 font-mono">允许兑换次数</label>
                    <select
                      v-model="ticketMaxUses"
                      class="w-full bg-neutral-950 border border-border rounded-md px-2.5 h-8 text-xs text-foreground font-mono focus:outline-none"
                    >
                      <option :value="1">1 次 (严格即焚)</option>
                      <option :value="2">2 次 (防误触推荐)</option>
                      <option :value="5">5 次</option>
                    </select>
                  </div>
                </div>

                <Button
                  variant="default"
                  size="sm"
                  class="w-full font-mono mt-1"
                  :loading="generatingTicket"
                  @click="generateUserTicket(currentShareData.user?.id)"
                >
                  <Sparkles class="w-3.5 h-3.5 mr-1.5" />
                  <span>{{ generatingTicket ? '正在生成...' : '生成安全提取码' }}</span>
                </Button>
              </div>

              <!-- Display generated ticket -->
              <div v-if="generatedTicket" class="rounded-lg border border-border bg-card p-4 space-y-3 font-mono">
                <div class="text-center py-2 bg-neutral-950 rounded-md border border-border">
                  <div class="text-[11px] text-muted-foreground mb-1">6 位提取码 (不区分大小写)</div>
                  <div class="text-2xl font-bold tracking-[0.3em] text-foreground">
                    {{ generatedTicket.code }}
                  </div>
                  <div class="text-[10px] text-muted-foreground mt-1">
                    剩余可用 {{ generatedTicket.remaining_uses }} 次 · {{ formatTicketExpires(generatedTicket.expires_at) }}
                  </div>
                </div>

                <div>
                  <label class="block text-[11px] text-muted-foreground mb-1">可直接复制发给用户的分享文案：</label>
                  <textarea
                    readonly
                    rows="3"
                    :value="generatedTicket.share_text"
                    @click="selectTarget"
                    class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-[11px] font-mono text-foreground focus:outline-none select-all"
                  ></textarea>
                </div>

                <div v-if="generatedTicket.share_text?.includes('127.0.0.1') || generatedTicket.share_text?.includes('localhost')" class="p-2.5 bg-amber-500/10 border border-amber-500/20 rounded-md text-[11px] text-amber-300 flex items-start gap-2">
                  <AlertCircle class="w-4 h-4 mt-0.5 shrink-0 text-amber-400" />
                  <div>
                    <span class="font-semibold">提示：当前入口为本地 127.0.0.1 地址</span>
                    <p class="text-amber-300/80 text-[10px] mt-0.5">外部用户无法直接访问本地环回地址。建议前往【系统设置】配置「中立提取门户 URL」或「面板公网访问 URL」。</p>
                  </div>
                </div>

                <Button
                  variant="default"
                  size="sm"
                  class="w-full font-mono"
                  @click="copyText(generatedTicket.share_text)"
                >
                  <Copy class="w-3.5 h-3.5 mr-1" />
                  <span>一键复制中立提取分享文案</span>
                </Button>
              </div>
            </div>

            <!-- 3. 链接模式 -->
            <div v-else class="space-y-4">
              <!-- 核心专属安全订阅链接 (Token-based) -->
              <div class="rounded-lg border border-border bg-card p-4 space-y-2">
                <div class="flex items-center justify-between">
                  <span class="font-bold text-foreground flex items-center gap-1.5 font-mono">
                    <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
                    <span>聚合订阅 (All-In-One Sub URL)</span>
                  </span>
                  <Button
                    variant="ghost"
                    size="sm"
                    class="h-6 text-[10px] text-rose-400 font-mono px-2"
                    @click="resetUserSubToken(currentShareData.user?.id)"
                  >
                    <RotateCcw class="w-3 h-3 mr-1" /> 重置
                  </Button>
                </div>

                <div class="flex items-center gap-2 font-mono">
                  <input
                    :value="getDirectTokenSubUrl(currentShareData.user)"
                    readonly
                    class="w-full bg-neutral-950 border border-border rounded-md px-3 h-8 text-foreground text-[11px] select-all focus:outline-none"
                  />
                  <Button
                    variant="secondary"
                    size="sm"
                    class="shrink-0 h-8 font-mono"
                    @click="copyText(getDirectTokenSubUrl(currentShareData.user))"
                  >
                    <Copy class="w-3.5 h-3.5 mr-1" /> 复制
                  </Button>
                </div>
                <p class="text-[10px] text-muted-foreground">客户端自动识别全协议并定时静默更新节点</p>
              </div>

              <!-- 单节点独立直连链接列表 -->
              <div class="space-y-2">
                <label class="block text-foreground font-semibold text-[11px] font-mono">单个节点独立直连链接</label>
                <div class="max-h-48 overflow-y-auto space-y-2 pr-1">
                  <div
                    v-for="link in currentShareData.links"
                    :key="link.tag"
                    class="p-2.5 bg-neutral-950 rounded-md border border-border flex items-center justify-between gap-2 hover:border-neutral-700 transition-colors"
                  >
                    <div class="overflow-hidden font-mono">
                      <div class="text-foreground text-[11px] font-semibold truncate">{{ link.tag }}</div>
                      <div class="text-[10px] text-muted-foreground truncate">{{ link.url }}</div>
                    </div>
                    <Button
                      variant="ghost"
                      size="icon"
                      class="shrink-0 h-7 w-7"
                      @click="copyText(link.url)"
                      title="复制单个节点直连链接"
                    >
                      <Copy class="w-3 h-3" />
                    </Button>
                  </div>
                  <div v-if="!currentShareData.links?.length" class="text-center py-4 text-muted-foreground text-xs font-mono">
                    该用户暂无直连节点链接
                  </div>
                </div>
              </div>
            </div>
          </div>

          <!-- Modal Footer -->
          <div class="p-3.5 border-t border-border bg-card shrink-0 flex items-center justify-end">
            <Button variant="outline" size="sm" @click="showShareModal = false">
              关闭
            </Button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Traffic History Modal (Neutral Console Style) -->
    <Teleport to="body">
      <div v-if="showHistoryModal" class="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 bg-black/75">
        <div class="bg-neutral-950 border border-border rounded-lg shadow-2xl w-full max-w-2xl max-h-[90vh] flex flex-col overflow-hidden">
          <!-- Header -->
          <div class="h-14 px-5 border-b border-border flex items-center justify-between shrink-0 bg-card">
            <div>
              <h3 class="text-sm font-semibold text-foreground flex items-center gap-2">
                <BarChart2 class="w-4 h-4 text-cyan-400" />
                <span>流量历史趋势 — {{ currentHistoryUser?.email }}</span>
              </h3>
              <p class="text-[11px] text-muted-foreground mt-0.5 font-mono">
                每日聚合流量归档与可视化统计
              </p>
            </div>
            <button
              type="button"
              @click="showHistoryModal = false"
              class="h-7 w-7 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/60 flex items-center justify-center transition-colors"
            >
              <X class="w-4 h-4" />
            </button>
          </div>

          <!-- Scrollable Body -->
          <div class="p-5 space-y-4 overflow-y-auto text-xs">
            <!-- Time Range Selector & Metrics -->
            <div class="flex flex-wrap items-center justify-between gap-2">
              <div class="flex items-center gap-1 bg-neutral-900 p-1 rounded-md border border-border">
                <button
                  v-for="d in [7, 14, 30]"
                  :key="d"
                  @click="setHistoryDays(d)"
                  class="px-2.5 py-1 rounded text-xs font-mono transition-colors"
                  :class="historyDays === d ? 'bg-foreground text-background font-semibold' : 'text-muted-foreground hover:text-foreground'"
                >
                  近 {{ d }} 天
                </button>
              </div>

              <div class="text-[11px] font-mono text-muted-foreground">
                区间总计: <span class="text-foreground font-bold">{{ formatBytes(historySummary.totalAll) }}</span>
              </div>
            </div>

            <!-- Metric Cards -->
            <div class="grid grid-cols-2 sm:grid-cols-4 gap-2 font-mono">
              <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
                <span class="text-[10px] text-muted-foreground uppercase">总上行流量</span>
                <div class="text-xs sm:text-sm font-bold text-emerald-400 truncate">{{ formatBytes(historySummary.totalUp) }}</div>
              </div>
              <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
                <span class="text-[10px] text-muted-foreground uppercase">总下行流量</span>
                <div class="text-xs sm:text-sm font-bold text-cyan-400 truncate">{{ formatBytes(historySummary.totalDown) }}</div>
              </div>
              <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
                <span class="text-[10px] text-muted-foreground uppercase">单日峰值</span>
                <div class="text-xs sm:text-sm font-bold text-amber-400 truncate">{{ formatBytes(maxDayBytes) }}</div>
              </div>
              <div class="p-3 bg-card rounded-md border border-border space-y-0.5">
                <span class="text-[10px] text-muted-foreground uppercase">日均消耗</span>
                <div class="text-xs sm:text-sm font-bold text-foreground truncate">{{ formatBytes(historySummary.avgDay) }}</div>
              </div>
            </div>

            <!-- Bar Chart Visualizer -->
            <div class="space-y-2">
              <div class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
                <span class="font-medium text-foreground">每日用量走势</span>
                <div class="flex items-center gap-3">
                  <span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-emerald-500"></span> 上行</span>
                  <span class="flex items-center gap-1.5"><span class="w-2 h-2 rounded-full bg-cyan-500"></span> 下行</span>
                </div>
              </div>

              <!-- Active log inspection strip -->
              <div class="min-h-[36px] px-3 py-1.5 bg-neutral-900 rounded-md border border-border flex flex-wrap items-center justify-between gap-2 text-xs font-mono">
                <div v-if="activeLog" class="flex flex-wrap items-center gap-x-3 gap-y-1 text-foreground">
                  <span class="font-bold text-foreground">{{ activeLog.date }}</span>
                  <span class="text-emerald-400">↑ {{ formatBytes(activeLog.upBytes) }}</span>
                  <span class="text-cyan-400">↓ {{ formatBytes(activeLog.downBytes) }}</span>
                  <span class="font-bold">总计: {{ formatBytes(activeLog.upBytes + activeLog.downBytes) }}</span>
                </div>
                <div v-else class="text-muted-foreground text-[11px]">
                  悬停或点击柱形查看单日用量详情
                </div>
              </div>

              <!-- Bar visualizer -->
              <div v-if="sortedHistoryLogs.length" class="bg-card rounded-md border border-border p-3 overflow-x-auto pb-2">
                <div class="h-36 flex items-end gap-2 pt-2 min-w-full w-max">
                  <div
                    v-for="log in sortedHistoryLogs"
                    :key="log.date"
                    @mouseenter="hoveredLog = log"
                    @mouseleave="hoveredLog = null"
                    @click="toggleSelectLog(log)"
                    class="flex-1 min-w-[32px] max-w-[48px] flex flex-col items-center gap-1.5 group relative h-full justify-end cursor-pointer select-none"
                  >
                    <div class="w-full flex-1 flex flex-col justify-end items-center relative">
                      <div
                        class="w-full max-w-[20px] rounded-t-sm overflow-hidden flex flex-col justify-end bg-neutral-900 transition-all duration-200"
                        :class="{ 'ring-2 ring-foreground scale-105': activeLog?.date === log.date }"
                        :style="{ height: `${getTotalBarHeight(log)}%` }"
                      >
                        <!-- Up Bytes (Emerald) -->
                        <div
                          v-if="log.upBytes > 0"
                          class="w-full bg-emerald-500 hover:bg-emerald-400 transition-all"
                          :style="{ height: `${getSegmentPercent(log.upBytes, log)}%` }"
                        />
                        <!-- Down Bytes (Cyan) -->
                        <div
                          v-if="log.downBytes > 0"
                          class="w-full bg-cyan-500 hover:bg-cyan-400 transition-all"
                          :style="{ height: `${getSegmentPercent(log.downBytes, log)}%` }"
                        />
                      </div>
                    </div>

                    <!-- Date Label -->
                    <span
                      class="text-[10px] font-mono transition-colors truncate w-full text-center shrink-0"
                      :class="activeLog?.date === log.date ? 'text-foreground font-bold' : 'text-muted-foreground group-hover:text-foreground'"
                    >
                      {{ log.date.substring(5) }}
                    </span>
                  </div>
                </div>
              </div>

              <div v-else class="text-center py-8 text-xs text-muted-foreground font-mono">
                暂无历史流量记录
              </div>
            </div>

            <!-- Table breakdown -->
            <div class="space-y-1.5">
              <div class="max-h-44 overflow-y-auto rounded-md border border-border bg-neutral-950">
                <table class="w-full text-left text-xs">
                  <thead class="border-b border-border bg-neutral-900 sticky top-0">
                    <tr class="font-mono text-[11px] text-muted-foreground">
                      <th class="py-2 px-3">日期</th>
                      <th class="py-2 px-3">上行 (Up)</th>
                      <th class="py-2 px-3">下行 (Down)</th>
                      <th class="py-2 px-3">单日总计</th>
                    </tr>
                  </thead>
                  <tbody class="divide-y divide-border/40 font-mono text-[11px]">
                    <tr v-for="log in historyLogs" :key="log.id" class="hover:bg-muted/30">
                      <td class="py-2 px-3 text-foreground font-medium">{{ log.date }}</td>
                      <td class="py-2 px-3 text-emerald-400">{{ formatBytes(log.upBytes) }}</td>
                      <td class="py-2 px-3 text-cyan-400">{{ formatBytes(log.downBytes) }}</td>
                      <td class="py-2 px-3 text-foreground font-bold">{{ formatBytes(log.upBytes + log.downBytes) }}</td>
                    </tr>
                    <tr v-if="!historyLogs.length">
                      <td colspan="4" class="text-center py-4 text-muted-foreground">暂无记录</td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>
          </div>

          <!-- Footer -->
          <div class="p-3.5 border-t border-border bg-card shrink-0 flex items-center justify-end">
            <Button variant="outline" size="sm" @click="showHistoryModal = false">
              关闭
            </Button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import {
  Plus,
  Search,
  Edit,
  Trash2,
  RotateCcw,
  Copy,
  BarChart2,
  Zap,
  CalendarPlus,
  QrCode,
  ShieldCheck,
  Sparkles,
  AlertCircle,
  Check,
  Users,
  Activity,
  SlidersHorizontal,
  X,
} from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import { toast } from '../utils/toast'
import { copyText } from '../utils/clipboard'
import { formatBytes, formatDate } from '../utils/format'
import api from '../api'

// Import UI Primitives
import Table from '../components/ui/Table.vue'
import TableHeader from '../components/ui/TableHeader.vue'
import TableBody from '../components/ui/TableBody.vue'
import TableHead from '../components/ui/TableHead.vue'
import TableRow from '../components/ui/TableRow.vue'
import TableCell from '../components/ui/TableCell.vue'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import Drawer from '../components/ui/Drawer.vue'

// Data States
const users = ref<any[]>([])
const availableInbounds = ref<any[]>([])
const selectedUserIds = ref<number[]>([])

// Drawers & Modals
const showModal = ref(false)
const showInspectorDrawer = ref(false)
const showShareModal = ref(false)
const showHistoryModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const activeShareTab = ref<'link' | 'qrcode' | 'ticket'>('link')

// Search & Filtering
const searchQuery = ref('')
const statusFilter = ref<'all' | 'online' | 'enabled' | 'disabled' | 'expired' | 'overquota'>('all')

// Selected Users for Inspector & Modals
const selectedInspectorUser = ref<any>(null)
const currentInspectorUser = computed(() => {
  if (!selectedInspectorUser.value) return null
  return users.value.find((u) => u.id === selectedInspectorUser.value.id) || selectedInspectorUser.value
})

// Share & Ticket States
const ticketTTL = ref(15)
const ticketMaxUses = ref(2)
const generatingTicket = ref(false)
const generatedTicket = ref<any>(null)
const currentShareData = ref<any>(null)

// History States
const currentHistoryUser = ref<any>(null)
const historyLogs = ref<any[]>([])
const historyDays = ref(14)
const hoveredLog = ref<any>(null)
const selectedLog = ref<any>(null)
const activeLog = computed(() => hoveredLog.value || selectedLog.value)

// Add / Edit Form
const form = ref<any>({
  id: 0,
  email: '',
  selectedTags: [] as string[],
  flow: '',
  totalGB: 0,
  expireDays: 0,
  extendDays: 0,
  resetDay: 0,
  ipLimit: 0,
  enabled: true,
})

// KPI Metrics Computeds
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

// Filtered Users
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
    const inboundMatch = getNodeTags(user).some((tag) => tag.toLowerCase().includes(query))
    return emailMatch || uuidMatch || subTokenMatch || inboundMatch
  })
})

// Selection Helpers
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

const isAllInboundsSelected = computed(() => {
  if (!availableInbounds.value.length) return false
  return form.value.selectedTags.length === availableInbounds.value.length
})

const toggleSelectAllInbounds = () => {
  if (isAllInboundsSelected.value) {
    form.value.selectedTags = []
  } else {
    form.value.selectedTags = availableInbounds.value.map((i) => i.tag)
  }
}

// User Inspection
const inspectUser = (user: any) => {
  selectedInspectorUser.value = user
  showInspectorDrawer.value = true
}

// User Status Badge Helpers
const isUserExpired = (user: any) => {
  return user.expireTime > 0 && user.expireTime < Date.now()
}

const isUserOverQuota = (user: any) => {
  return user.totalBytes > 0 && (user.upBytes + user.downBytes) >= user.totalBytes
}

const getUserBadgeVariant = (user: any): 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive' => {
  if (!user.enabled) return 'destructive'
  if (isUserExpired(user)) return 'warning'
  if (isUserOverQuota(user)) return 'destructive'
  if (user.isOnline) return 'success'
  return 'secondary'
}

const getUserStatusText = (user: any): string => {
  if (!user.enabled) return '已停用'
  if (isUserExpired(user)) return '已到期'
  if (isUserOverQuota(user)) return '已超额'
  if (user.isOnline) return '在线传输'
  return '正常运行'
}

const getTrafficProgressClass = (user: any): string => {
  const percent = getTrafficPercent(user)
  if (percent >= 90) return 'bg-rose-500'
  if (percent >= 75) return 'bg-amber-400'
  return 'bg-foreground'
}

// API Interactions
const fetchAll = async () => {
  try {
    const [uRes, inbRes]: any = await Promise.all([api.get('/users'), api.get('/inbounds')])
    users.value = uRes || []
    availableInbounds.value = inbRes || []
  } catch (err) {
    console.error(err)
  }
}

let speedTimer: any = null
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
  } catch (err) {
    // 保证稳定性
  }
}

const handleVisibilityChange = () => {
  if (!document.hidden) {
    fetchSpeeds()
  }
}

const getNodeTags = (user: any): string[] => {
  if (!user) return []
  let tags: string[] = []
  if (user.inboundTags) {
    tags = user.inboundTags.split(',').map((s: string) => s.trim()).filter((s: string) => s)
  } else if (user.inboundTag) {
    tags = [user.inboundTag]
  }

  if (availableInbounds.value.length > 0) {
    const validTags = new Set(availableInbounds.value.map((i: any) => i.tag))
    const filtered = tags.filter((t) => validTags.has(t))
    if (filtered.length > 0) {
      return filtered
    }
    return availableInbounds.value.map((i: any) => i.tag)
  }
  return tags
}

const getInboundMeta = (tag: string) => {
  return availableInbounds.value.find((i: any) => i.tag === tag)
}

const getSubRoutesCount = (tag: string): number => {
  const inb = availableInbounds.value.find((i: any) => i.tag === tag)
  if (!inb) return 0
  try {
    const srs = JSON.parse(inb.subRoutesJson || '[]')
    return srs.filter((s: any) => s.enabled !== false).length
  } catch (e) {
    return 0
  }
}

const getDirectTokenSubUrl = (user: any): string => {
  if (!user || !user.subToken) return ''
  return `${window.location.origin}/sub/${user.subToken}`
}

const resetUserSubToken = async (userId: number) => {
  if (!confirm('确定重置该用户的订阅与连接密钥吗？所有旧设备将立即断开连接，旧订阅链接也将失效！')) return
  try {
    const res: any = await api.post(`/users/${userId}/reset-token`)
    toast.success('订阅与密钥重置成功，旧设备已断开！')
    if (currentShareData.value && currentShareData.value.user) {
      currentShareData.value.user.subToken = res.subToken
      if (res.uuid) {
        currentShareData.value.user.uuid = res.uuid
      }
    }
    await fetchAll()
  } catch (err: any) {
    toast.error('重置失败: ' + err)
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

const toggleUserEnabled = async (user: any) => {
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

const openAddModal = () => {
  isEditing.value = false
  form.value = {
    id: 0,
    email: '',
    selectedTags: availableInbounds.value.map((i) => i.tag),
    flow: '',
    totalGB: 0,
    expireDays: 30,
    extendDays: 0,
    resetDay: 0,
    ipLimit: 0,
    enabled: true,
  }
  showModal.value = true
}

const openEditModal = (user: any) => {
  isEditing.value = true
  form.value = {
    id: user.id,
    email: user.email,
    selectedTags: getNodeTags(user),
    flow: user.flow || '',
    totalGB: user.totalBytes > 0 ? Math.max(0.01, parseFloat((user.totalBytes / 1073741824).toFixed(2))) : 0,
    expireDays: 0,
    extendDays: 0,
    resetDay: user.resetDay || 0,
    ipLimit: user.ipLimit || 0,
    enabled: user.enabled,
  }
  showModal.value = true
}

const saveUser = async () => {
  if (!form.value.selectedTags.length) {
    toast.warning('请至少选择一个归属的入站节点！')
    return
  }

  saving.value = true
  try {
    const payload: any = {
      email: form.value.email,
      inboundTags: form.value.selectedTags,
      inboundTag: form.value.selectedTags[0],
      flow: form.value.flow,
      totalBytes: form.value.totalGB > 0 ? Math.round(form.value.totalGB * 1073741824) : 0,
      expireDays: form.value.expireDays,
      resetDay: form.value.resetDay,
      ipLimit: form.value.ipLimit,
      enabled: form.value.enabled,
    }

    if (isEditing.value) {
      const existingUser = users.value.find((u) => u.id === form.value.id)
      let expireTime = existingUser.expireTime
      if (form.value.extendDays > 0) {
        const now = Date.now()
        if (!expireTime || expireTime < now) {
          expireTime = now + form.value.extendDays * 86400000
        } else {
          expireTime += form.value.extendDays * 86400000
        }
      }

      await api.put(`/users/${form.value.id}`, {
        ...existingUser,
        inboundTags: form.value.selectedTags.join(','),
        inboundTag: form.value.selectedTags[0],
        flow: form.value.flow,
        totalBytes: payload.totalBytes,
        expireTime,
        resetDay: form.value.resetDay,
        ipLimit: form.value.ipLimit,
        enabled: form.value.enabled,
      })
      toast.success('用户信息与授权节点已保存更新！')
    } else {
      await api.post('/users', payload)
      toast.success('用户已成功创建并同步至核心节点！')
    }

    showModal.value = false
    await fetchAll()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteUser = async (id: number) => {
  if (!confirm('确定删除该用户并将其从所有节点下线吗？')) return
  try {
    await api.delete(`/users/${id}`)
    toast.success('用户已成功删除！')
    if (showInspectorDrawer.value && selectedInspectorUser.value?.id === id) {
      showInspectorDrawer.value = false
    }
    await fetchAll()
  } catch (err: any) {
    toast.error('删除失败: ' + err)
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

const openShareModal = async (user: any) => {
  try {
    const res: any = await api.get(`/users/${user.id}/share`)
    currentShareData.value = {
      ...res,
      user,
    }
    generatedTicket.value = null
    activeShareTab.value = 'link'
    showShareModal.value = true
  } catch (err: any) {
    toast.error('获取订阅链接失败: ' + err)
  }
}

const generateUserTicket = async (userId: number) => {
  if (!userId) return
  generatingTicket.value = true
  try {
    const res: any = await api.post(`/users/${userId}/tickets`, {
      ttl_minutes: ticketTTL.value,
      max_uses: ticketMaxUses.value,
    })
    generatedTicket.value = res
    toast.success('安全提取码生成成功！')
  } catch (err: any) {
    toast.error('生成提取码失败: ' + (err.message || err))
  } finally {
    generatingTicket.value = false
  }
}

const formatTicketExpires = (timestamp: number) => {
  if (!timestamp) return ''
  const d = new Date(timestamp * 1000)
  return `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')} 前有效`
}

const selectTarget = (e: MouseEvent) => {
  const target = e.target as HTMLInputElement | HTMLTextAreaElement | null
  target?.select()
}

// History Analysis
const toggleSelectLog = (log: any) => {
  if (selectedLog.value?.date === log.date) {
    selectedLog.value = null
  } else {
    selectedLog.value = log
  }
}

const openHistoryModal = async (user: any) => {
  currentHistoryUser.value = user
  hoveredLog.value = null
  selectedLog.value = null
  historyDays.value = 14
  showHistoryModal.value = true
  await fetchUserHistory(user.id, 14)
}

const setHistoryDays = async (days: number) => {
  historyDays.value = days
  hoveredLog.value = null
  selectedLog.value = null
  if (currentHistoryUser.value) {
    await fetchUserHistory(currentHistoryUser.value.id, days)
  }
}

const fetchUserHistory = async (userId: number, days: number) => {
  try {
    const res: any = await api.get(`/users/${userId}/traffic-history?days=${days}`)
    historyLogs.value = res || []
  } catch (err) {
    console.error(err)
    historyLogs.value = []
  }
}

const sortedHistoryLogs = computed(() => {
  return [...historyLogs.value].reverse()
})

const maxDayBytes = computed(() => {
  let max = 1
  for (const log of historyLogs.value) {
    const total = log.upBytes + log.downBytes
    if (total > max) max = total
  }
  return max
})

const getTotalBarHeight = (log: any) => {
  const total = (log.upBytes || 0) + (log.downBytes || 0)
  if (!total || maxDayBytes.value <= 0) return 4
  return Math.min(90, Math.max(6, (total / maxDayBytes.value) * 90))
}

const getSegmentPercent = (segmentBytes: number, log: any) => {
  const total = (log.upBytes || 0) + (log.downBytes || 0)
  if (!total || !segmentBytes) return 0
  return Math.round((segmentBytes / total) * 100)
}

const historySummary = computed(() => {
  let up = 0
  let down = 0
  for (const log of historyLogs.value) {
    up += log.upBytes || 0
    down += log.downBytes || 0
  }
  const all = up + down
  const count = historyLogs.value.length || 1
  return {
    totalUp: up,
    totalDown: down,
    totalAll: all,
    avgDay: Math.round(all / count),
  }
})

// Utility Helpers
const getTrafficPercent = (user: any) => {
  if (!user || !user.totalBytes) return 0
  const used = user.upBytes + user.downBytes
  return (used / user.totalBytes) * 100
}

// Lifecycle
onMounted(async () => {
  await fetchAll()
  document.addEventListener('visibilitychange', handleVisibilityChange)
  speedTimer = setInterval(fetchSpeeds, 3000)
})

onUnmounted(() => {
  if (speedTimer) {
    clearInterval(speedTimer)
    speedTimer = null
  }
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
