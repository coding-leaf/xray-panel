<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-white tracking-tight">用户与多节点归属管理</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-emerald-500/15 text-emerald-400 border border-emerald-500/30 flex items-center gap-1">
            <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
            <span>gRPC 毫秒内存热生效 (零断网 / 免重启)</span>
          </span>
        </div>
        <p class="text-xs text-gray-400 mt-0.5">支持批量延期与流量重置、独立安全订阅 Token、每月周期自动重置与并发设备限制</p>
      </div>

      <div class="flex items-center gap-3">
        <button
          @click="openAddModal"
          class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
        >
          <UserPlus class="w-4 h-4" />
          <span>添加用户</span>
        </button>
      </div>
    </div>

    <!-- Batch Action Floating / Fixed Bar -->
    <div
      v-if="selectedUserIds.length > 0"
      class="glass-panel p-3.5 sm:p-4 rounded-2xl border border-brand-500/40 bg-brand-950/30 flex flex-wrap items-center justify-between gap-3 animate-in fade-in duration-200"
    >
      <div class="flex items-center gap-2 text-xs">
        <span class="px-2 py-0.5 rounded-md bg-brand-500/20 text-brand-300 font-bold font-mono">
          已选择 {{ selectedUserIds.length }} 位用户
        </span>
        <span class="text-gray-400">可执行批量周期操作：</span>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <!-- 批量延期按钮 -->
        <button
          @click="batchRenew(30)"
          class="px-3 py-1.5 rounded-xl bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-300 border border-emerald-500/30 text-xs font-semibold transition-colors flex items-center gap-1"
        >
          <CalendarPlus class="w-3.5 h-3.5" />
          <span>+30天 (1个月)</span>
        </button>
        <button
          @click="batchRenew(90)"
          class="px-3 py-1.5 rounded-xl bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-300 border border-emerald-500/30 text-xs font-semibold transition-colors flex items-center gap-1"
        >
          <CalendarPlus class="w-3.5 h-3.5" />
          <span>+90天 (1季度)</span>
        </button>
        <button
          @click="batchRenew(365)"
          class="px-3 py-1.5 rounded-xl bg-emerald-600/20 hover:bg-emerald-600/30 text-emerald-300 border border-emerald-500/30 text-xs font-semibold transition-colors flex items-center gap-1"
        >
          <CalendarPlus class="w-3.5 h-3.5" />
          <span>+365天 (1年)</span>
        </button>

        <!-- 批量重置已用流量 -->
        <button
          @click="batchResetTraffic"
          class="px-3 py-1.5 rounded-xl bg-cyan-600/20 hover:bg-cyan-600/30 text-cyan-300 border border-cyan-500/30 text-xs font-semibold transition-colors flex items-center gap-1"
        >
          <RotateCcw class="w-3.5 h-3.5" />
          <span>重置已用流量</span>
        </button>

        <!-- 批量启用/禁用 -->
        <button
          @click="batchSetStatus(true)"
          class="px-3 py-1.5 rounded-xl bg-indigo-600/20 hover:bg-indigo-600/30 text-indigo-300 border border-indigo-500/30 text-xs font-semibold transition-colors"
        >
          批量启用
        </button>
        <button
          @click="batchSetStatus(false)"
          class="px-3 py-1.5 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 border border-gray-700 text-xs font-semibold transition-colors"
        >
          批量禁用
        </button>

        <button
          @click="selectedUserIds = []"
          class="px-2.5 py-1.5 rounded-xl text-gray-400 hover:text-white text-xs transition-colors"
        >
          取消选择
        </button>
      </div>
    </div>

    <!-- Users Table -->
    <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead>
            <tr class="border-b border-gray-800/80 bg-gray-900/50 text-gray-400 font-semibold">
              <th class="py-3.5 px-3 text-center w-10">
                <input
                  type="checkbox"
                  :checked="isAllUsersSelected"
                  @change="toggleSelectAllUsers"
                  class="rounded bg-gray-900 border-gray-700 text-brand-500 focus:ring-0 cursor-pointer"
                />
              </th>
              <th class="py-3.5 px-4">用户名 / 邮箱</th>
              <th class="py-3.5 px-4">归属节点 (Inbound Tags)</th>
              <th class="py-3.5 px-4">已用 / 总限额</th>
              <th class="py-3.5 px-4">到期时间 / 重置周期</th>
              <th class="py-3.5 px-4">状态</th>
              <th class="py-3.5 px-4 text-right">操作</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-800/40">
            <tr v-for="user in users" :key="user.id" class="hover:bg-white/[0.02] transition-colors">
              <td class="py-3.5 px-3 text-center">
                <input
                  type="checkbox"
                  :value="user.id"
                  v-model="selectedUserIds"
                  class="rounded bg-gray-900 border-gray-700 text-brand-500 focus:ring-0 cursor-pointer"
                />
              </td>

              <td class="py-3.5 px-4">
                <div class="font-mono font-medium text-white">{{ user.email }}</div>
                <div class="text-[10px] text-gray-500 font-mono flex items-center gap-1.5 mt-0.5">
                  <span v-if="user.ipLimit > 0" class="text-amber-400/90">限 {{ user.ipLimit }} IP</span>
                  <span v-else>无IP限制</span>
                  <span>•</span>
                  <span>Token: {{ user.subToken ? user.subToken.substring(0, 8) + '...' : '未生成' }}</span>
                </div>
              </td>

              <!-- 归属多节点展示 -->
              <td class="py-3.5 px-4">
                <div class="flex flex-wrap gap-1">
                  <span
                    v-for="tag in getNodeTags(user)"
                    :key="tag"
                    class="px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-brand-500/15 text-brand-300 border border-brand-500/20"
                  >
                    {{ tag }}
                  </span>
                </div>
              </td>

              <td class="py-3.5 px-4 font-mono text-gray-300">
                <div class="space-y-1">
                  <div>{{ formatBytes(user.upBytes + user.downBytes) }} / {{ user.totalBytes > 0 ? formatBytes(user.totalBytes) : '无限制' }}</div>
                  <div v-if="user.totalBytes > 0" class="w-24 h-1.5 bg-gray-800 rounded-full overflow-hidden">
                    <div
                      class="h-full rounded-full transition-all"
                      :class="getTrafficPercent(user) > 90 ? 'bg-rose-500' : 'bg-brand-500'"
                      :style="{ width: `${Math.min(100, getTrafficPercent(user))}%` }"
                    ></div>
                  </div>
                </div>
              </td>

              <td class="py-3.5 px-4 text-gray-400 font-mono text-[11px]">
                <div>{{ user.expireTime > 0 ? formatDate(user.expireTime) : '永久有效' }}</div>
                <div v-if="user.resetDay > 0" class="text-[10px] text-cyan-400/90">
                  每月 {{ user.resetDay }} 号自动清零
                </div>
              </td>

              <td class="py-3.5 px-4">
                <div class="space-y-1">
                  <div class="flex items-center gap-1.5">
                    <span
                      class="w-2 h-2 rounded-full"
                      :class="!user.enabled ? 'bg-gray-600' : (user.isOnline ? 'bg-emerald-400 animate-pulse' : 'bg-gray-500')"
                    ></span>
                    <span
                      class="text-[11px] font-semibold"
                      :class="!user.enabled ? 'text-gray-500' : (user.isOnline ? 'text-emerald-400' : 'text-gray-400')"
                    >
                      {{ !user.enabled ? '已禁用' : (user.isOnline ? '在线连接' : '离线') }}
                    </span>
                  </div>
                  <!-- 实时上下行速率 -->
                  <div v-if="user.isOnline && (user.upSpeed > 0 || user.downSpeed > 0)" class="text-[10px] font-mono text-cyan-300 flex items-center gap-1">
                    <span>↑{{ formatBytes(user.upSpeed) }}/s</span>
                    <span>↓{{ formatBytes(user.downSpeed) }}/s</span>
                  </div>
                </div>
              </td>

              <td class="py-3.5 px-4 text-right space-x-1.5">
                <!-- 快捷复制一键全节点订阅链接 -->
                <button
                  @click="copyDirectSubLink(user)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-brand-600/20 text-brand-300 transition-colors"
                  title="一键复制专属聚合订阅链接"
                >
                  <Zap class="w-3.5 h-3.5 text-amber-400" />
                </button>

                <!-- 流量历史趋势按钮 -->
                <button
                  @click="openHistoryModal(user)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-cyan-500/20 text-cyan-400 transition-colors"
                  title="查看每日流量历史"
                >
                  <BarChart2 class="w-3.5 h-3.5" />
                </button>

                <!-- 订阅与分享面板 -->
                <button
                  @click="openShareModal(user)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-brand-600/20 text-brand-400 transition-colors"
                  title="获取多客户端订阅与二维码"
                >
                  <Share2 class="w-3.5 h-3.5" />
                </button>

                <!-- 重置已用流量 -->
                <button
                  @click="resetTraffic(user.id)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 transition-colors"
                  title="单用户重置流量"
                >
                  <RotateCcw class="w-3.5 h-3.5" />
                </button>

                <!-- 编辑 -->
                <button
                  @click="openEditModal(user)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-gray-700 text-indigo-400 transition-colors"
                  title="编辑用户与节点"
                >
                  <Edit class="w-3.5 h-3.5" />
                </button>

                <!-- 删除 -->
                <button
                  @click="deleteUser(user.id)"
                  class="p-1 rounded-lg bg-gray-800 hover:bg-rose-500/20 text-rose-400 transition-colors"
                  title="删除"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Add/Edit User Modal -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-lg p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-5 my-8">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white">{{ isEditing ? '编辑用户权限与周期' : '添加新用户' }}</h2>
            <p class="text-xs text-gray-400 mt-0.5">选择该用户归属的节点与计费周期，将自动同步至 Xray 内存</p>
          </div>
          <button @click="showModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <form @submit.prevent="saveUser" class="space-y-4 text-xs">
          <div>
            <label class="block text-gray-300 font-semibold mb-1">用户名 / 邮箱</label>
            <input
              v-model="form.email"
              type="text"
              required
              :disabled="isEditing"
              placeholder="user@example.com 或 纯用户名"
              class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500 disabled:opacity-60"
            />
          </div>

          <!-- 授权节点多选 (Inbounds Checkboxes) -->
          <div>
            <div class="flex items-center justify-between mb-1.5">
              <label class="block text-gray-300 font-semibold">
                授权入站节点 (Inbounds) <span class="text-rose-400">*</span>
              </label>
              <button
                type="button"
                @click="toggleSelectAllInbounds"
                class="text-[11px] text-brand-400 hover:text-brand-300 transition-colors"
              >
                {{ isAllInboundsSelected ? '取消全选' : '全选所有节点' }}
              </button>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 max-h-36 overflow-y-auto p-2 bg-gray-900/80 rounded-xl border border-gray-800">
              <label
                v-for="inb in availableInbounds"
                :key="inb.id"
                class="flex items-center gap-2 p-2 rounded-lg hover:bg-gray-800/60 cursor-pointer transition-colors border border-transparent"
                :class="{ 'border-brand-500/40 bg-brand-500/10': form.selectedTags.includes(inb.tag) }"
              >
                <input
                  type="checkbox"
                  :value="inb.tag"
                  v-model="form.selectedTags"
                  class="rounded bg-gray-900 border-gray-700 text-brand-500 focus:ring-0"
                />
                <div class="overflow-hidden">
                  <div class="font-mono text-white text-[11px] font-semibold truncate">{{ inb.tag }}</div>
                  <div class="text-[10px] text-gray-400 uppercase">{{ inb.protocol }} :{{ inb.port }}</div>
                </div>
              </label>
            </div>
          </div>

          <!-- 计费与配额设置 -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-gray-300 mb-1">流量限额 (GB, 0为不限制)</label>
              <input
                v-model.number="form.totalGB"
                type="number"
                min="0"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div v-if="!isEditing">
              <label class="block text-gray-300 mb-1">初始有效天数 (0为永久)</label>
              <input
                v-model.number="form.expireDays"
                type="number"
                min="0"
                placeholder="例如 30"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div v-else>
              <label class="block text-gray-300 mb-1">延长有效天数 (+天)</label>
              <input
                v-model.number="form.extendDays"
                type="number"
                min="0"
                placeholder="增加天数如 30"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div>
              <label class="block text-gray-300 mb-1">每月重置流量日 (0为不重置)</label>
              <input
                v-model.number="form.resetDay"
                type="number"
                min="0"
                max="31"
                placeholder="1-31 (如每月1号清零)"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <div>
              <label class="block text-gray-300 mb-1">并发连接设备限制 (0为不限)</label>
              <input
                v-model.number="form.ipLimit"
                type="number"
                min="0"
                max="100"
                placeholder="例如限制 2 IP"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <div class="flex items-center gap-2 pt-1">
            <input
              type="checkbox"
              id="enabled"
              v-model="form.enabled"
              class="rounded bg-gray-900 border-gray-700 text-brand-500 focus:ring-0"
            />
            <label for="enabled" class="text-gray-300 font-medium cursor-pointer">账号处于启用状态</label>
          </div>

          <div class="flex justify-end gap-3 pt-3 border-t border-gray-800">
            <button
              type="button"
              @click="showModal = false"
              class="px-5 py-2 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 font-medium text-xs transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              :disabled="saving"
              class="px-5 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-semibold text-xs rounded-xl transition-all shadow-lg shadow-brand-500/25 disabled:opacity-50"
            >
              {{ saving ? '保存中...' : '确认保存' }}
            </button>
          </div>
        </form>
      </div>
    </div>

    <!-- Share & Subscription Modal -->
    <div v-if="showShareModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-lg p-6 sm:p-7 rounded-3xl border border-gray-800 shadow-2xl space-y-5">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-base font-bold text-white flex items-center gap-2">
              <Zap class="w-4 h-4 text-amber-400" />
              <span>全节点订阅与独立安全 Token</span>
            </h2>
            <p class="text-xs text-gray-400 mt-0.5">用户专属聚合订阅，解耦 UUID 并支持一键重置 Token</p>
          </div>
          <button @click="showShareModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <!-- Share Tabs -->
        <div class="flex rounded-xl bg-white/[0.04] p-1 border border-white/[0.06]">
          <button
            type="button"
            @click="activeShareTab = 'link'"
            class="flex-1 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center justify-center gap-1.5"
            :class="activeShareTab === 'link' ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'"
          >
            <Copy class="w-3.5 h-3.5" />
            <span>订阅链接 (Text)</span>
          </button>
          <button
            type="button"
            @click="activeShareTab = 'qrcode'"
            class="flex-1 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center justify-center gap-1.5"
            :class="activeShareTab === 'qrcode' ? 'bg-indigo-600 text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'"
          >
            <QrCode class="w-3.5 h-3.5" />
            <span>手机扫码导入 (QR Code)</span>
          </button>
        </div>

        <div v-if="currentShareData" class="space-y-4 text-xs">
          <!-- 二维码模式 -->
          <div v-if="activeShareTab === 'qrcode'" class="text-center py-2 space-y-3">
            <div class="inline-block p-4 bg-white rounded-3xl shadow-2xl ring-4 ring-indigo-500/20">
              <qrcode-vue
                :value="getDirectTokenSubUrl(currentShareData.user)"
                :size="190"
                level="M"
                render-as="svg"
              />
            </div>
            <p class="text-[11px] text-gray-400">使用 Shadowrocket / Clash / V2Ray / Sing-box 相机直接扫码添加</p>
          </div>

          <!-- 链接模式 -->
          <div v-else class="space-y-4">
            <!-- 核心专属安全订阅链接 (Token-based) -->
            <div class="p-3.5 bg-indigo-950/40 rounded-2xl border border-indigo-500/30 space-y-2">
              <div class="flex items-center justify-between">
                <span class="font-bold text-white flex items-center gap-1.5">
                  <span class="w-2 h-2 rounded-full bg-indigo-400 animate-pulse"></span>
                  <span>聚合订阅 (All-In-One Sub URL)</span>
                </span>
                <button
                  @click="resetUserSubToken(currentShareData.user?.id)"
                  class="text-[10px] text-rose-400 hover:text-rose-300 font-mono transition-colors flex items-center gap-1"
                >
                  <RotateCcw class="w-3 h-3" />
                  <span>重置订阅 Token</span>
                </button>
              </div>

              <div class="flex items-center gap-2">
                <input
                  :value="getDirectTokenSubUrl(currentShareData.user)"
                  readonly
                  class="w-full bg-gray-900/90 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] select-all focus:outline-none focus:border-indigo-500"
                />
                <button
                  @click="copyText(getDirectTokenSubUrl(currentShareData.user))"
                  class="px-3.5 py-2 bg-indigo-600 hover:bg-indigo-500 text-white rounded-xl font-semibold transition-colors shrink-0 flex items-center gap-1 shadow-sm"
                >
                  <Copy class="w-3.5 h-3.5" />
                  <span>复制</span>
                </button>
              </div>
              <p class="text-[10px] text-gray-400">客户端自动识别全协议并定时静默更新节点</p>
            </div>

            <!-- 单节点分享链接列表 -->
            <div class="space-y-2">
              <label class="block text-gray-300 font-semibold text-[11px]">单个节点独立直连链接</label>
              <div class="max-h-40 overflow-y-auto space-y-2 pr-1">
                <div
                  v-for="link in currentShareData.links"
                  :key="link.tag"
                  class="p-2.5 bg-gray-900/80 rounded-xl border border-gray-800 flex items-center justify-between gap-2 hover:border-gray-700 transition-colors"
                >
                  <div class="overflow-hidden">
                    <div class="font-mono text-white text-[11px] font-semibold truncate">{{ link.tag }}</div>
                    <div class="text-[10px] text-gray-400 truncate font-mono">{{ link.url }}</div>
                  </div>
                  <button
                    @click="copyText(link.url)"
                    class="p-1.5 bg-gray-800 hover:bg-gray-700 text-gray-200 rounded-lg transition-colors shrink-0"
                    title="复制单个节点链接"
                  >
                    <Copy class="w-3.5 h-3.5" />
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Traffic History Modal -->
    <div v-if="showHistoryModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm">
      <div class="glass-panel w-full max-w-2xl p-6 sm:p-7 rounded-3xl border border-gray-800 shadow-2xl space-y-5">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-base font-bold text-white flex items-center gap-2">
              <BarChart2 class="w-4 h-4 text-cyan-400" />
              <span>流量历史趋势 — {{ currentHistoryUser?.email }}</span>
            </h2>
            <p class="text-xs text-gray-400 mt-0.5">每日聚合流量归档与可视化统计</p>
          </div>
          <button @click="showHistoryModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <!-- Time Range Selector & Metrics -->
        <div class="flex items-center justify-between gap-2 text-xs">
          <div class="flex items-center gap-1 bg-gray-900 p-1 rounded-xl border border-gray-800">
            <button
              v-for="d in [7, 14, 30]"
              :key="d"
              @click="setHistoryDays(d)"
              class="px-3 py-1 rounded-lg font-medium transition-colors"
              :class="historyDays === d ? 'bg-brand-600 text-white font-semibold' : 'text-gray-400 hover:text-white'"
            >
              近 {{ d }} 天
            </button>
          </div>

          <div class="text-[11px] font-mono text-gray-400">
            区间总流量: <span class="text-brand-300 font-bold">{{ formatBytes(historySummary.totalAll) }}</span>
          </div>
        </div>

        <!-- Metric Badges -->
        <div class="grid grid-cols-2 sm:grid-cols-4 gap-2.5 text-xs">
          <div class="p-3 bg-gray-900/80 rounded-2xl border border-gray-800 space-y-0.5">
            <span class="text-[10px] text-gray-400">总上行流量</span>
            <div class="text-sm font-bold font-mono text-emerald-400">{{ formatBytes(historySummary.totalUp) }}</div>
          </div>
          <div class="p-3 bg-gray-900/80 rounded-2xl border border-gray-800 space-y-0.5">
            <span class="text-[10px] text-gray-400">总下行流量</span>
            <div class="text-sm font-bold font-mono text-cyan-400">{{ formatBytes(historySummary.totalDown) }}</div>
          </div>
          <div class="p-3 bg-gray-900/80 rounded-2xl border border-gray-800 space-y-0.5">
            <span class="text-[10px] text-gray-400">单日最高</span>
            <div class="text-sm font-bold font-mono text-amber-400">{{ formatBytes(maxDayBytes) }}</div>
          </div>
          <div class="p-3 bg-gray-900/80 rounded-2xl border border-gray-800 space-y-0.5">
            <span class="text-[10px] text-gray-400">日均消耗</span>
            <div class="text-sm font-bold font-mono text-indigo-300">{{ formatBytes(historySummary.avgDay) }}</div>
          </div>
        </div>

        <!-- Bar Chart Visualizer -->
        <div class="space-y-2">
          <div class="flex items-center justify-between text-[11px] text-gray-400">
            <span>每日用量柱状走势图</span>
            <div class="flex items-center gap-3">
              <span class="flex items-center gap-1"><span class="w-2 h-2 rounded bg-emerald-500"></span> 上行</span>
              <span class="flex items-center gap-1"><span class="w-2 h-2 rounded bg-cyan-500"></span> 下行</span>
            </div>
          </div>

          <div v-if="sortedHistoryLogs.length" class="h-44 bg-gray-900/60 rounded-2xl border border-gray-800 p-3.5 pt-6 flex items-end gap-2 overflow-x-auto">
            <div
              v-for="log in sortedHistoryLogs"
              :key="log.date"
              class="flex-1 min-w-[28px] max-w-[48px] flex flex-col items-center gap-1.5 group relative h-full justify-end"
            >
              <!-- 柱子有效高度绘制区 (留出顶部余量) -->
              <div class="w-full flex-1 flex flex-col justify-end items-center relative">
                <div
                  class="w-full max-w-[22px] rounded-t-md overflow-hidden flex flex-col justify-end bg-gray-800/40 transition-all duration-300 group-hover:ring-2 group-hover:ring-indigo-500/50"
                  :style="{ height: `${getTotalBarHeight(log)}%` }"
                >
                  <!-- 下行 (Cyan) -->
                  <div
                    class="w-full bg-cyan-500 hover:bg-cyan-400 transition-all"
                    :style="{ height: `${getSegmentPercent(log.downBytes, log)}%` }"
                  ></div>
                  <!-- 上行 (Emerald) -->
                  <div
                    class="w-full bg-emerald-500 hover:bg-emerald-400 transition-all border-t border-gray-900/30"
                    :style="{ height: `${getSegmentPercent(log.upBytes, log)}%` }"
                  ></div>
                </div>
              </div>

              <!-- 悬浮数据卡片 -->
              <div class="absolute bottom-full mb-3 hidden group-hover:flex flex-col p-2.5 bg-gray-950/95 text-white rounded-xl text-[10px] font-mono shadow-2xl border border-gray-700/80 whitespace-nowrap z-30 pointer-events-none backdrop-blur-md">
                <span class="font-bold text-indigo-300 mb-1 flex items-center gap-1">
                  <span>📅</span>
                  <span>{{ log.date }}</span>
                </span>
                <span class="text-emerald-400">↑ 上行: {{ formatBytes(log.upBytes) }}</span>
                <span class="text-cyan-400">↓ 下行: {{ formatBytes(log.downBytes) }}</span>
                <span class="text-gray-300 border-t border-gray-800 pt-1 mt-1 font-bold">总计: {{ formatBytes(log.upBytes + log.downBytes) }}</span>
              </div>

              <!-- 底部日期标签 -->
              <span class="text-[10px] font-mono text-gray-400 group-hover:text-white transition-colors truncate w-full text-center shrink-0">
                {{ log.date.substring(5) }}
              </span>
            </div>
          </div>

          <div v-else class="text-center py-10 text-xs text-gray-500">
            暂无历史流量记录
          </div>
        </div>

        <!-- History Records Table -->
        <div class="space-y-2 text-xs">
          <div class="max-h-40 overflow-y-auto rounded-2xl border border-gray-800 bg-gray-900/50">
            <table class="w-full text-left">
              <thead>
                <tr class="border-b border-gray-800 bg-gray-900/80 text-gray-400 font-semibold text-[11px]">
                  <th class="py-2 px-3">日期</th>
                  <th class="py-2 px-3">上行 (Up)</th>
                  <th class="py-2 px-3">下行 (Down)</th>
                  <th class="py-2 px-3">单日总计</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-gray-800/40 font-mono text-[11px]">
                <tr v-for="log in historyLogs" :key="log.id" class="hover:bg-white/[0.02]">
                  <td class="py-2 px-3 text-white font-medium">{{ log.date }}</td>
                  <td class="py-2 px-3 text-emerald-400">{{ formatBytes(log.upBytes) }}</td>
                  <td class="py-2 px-3 text-cyan-400">{{ formatBytes(log.downBytes) }}</td>
                  <td class="py-2 px-3 text-brand-300 font-bold">{{ formatBytes(log.upBytes + log.downBytes) }}</td>
                </tr>
                <tr v-if="!historyLogs.length">
                  <td colspan="4" class="text-center py-3 text-gray-500">暂无记录</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import {
  UserPlus,
  Edit,
  Trash2,
  RotateCcw,
  Share2,
  Copy,
  BarChart2,
  Zap,
  CalendarPlus,
  QrCode,
} from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import { toast } from '../utils/toast'
import api from '../api'

const users = ref<any[]>([])
const availableInbounds = ref<any[]>([])
const selectedUserIds = ref<number[]>([])

const showModal = ref(false)
const showShareModal = ref(false)
const showHistoryModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const activeShareTab = ref<'link' | 'qrcode'>('link')

const currentShareData = ref<any>(null)
const currentHistoryUser = ref<any>(null)
const historyLogs = ref<any[]>([])
const historyDays = ref(14)

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

const isAllUsersSelected = computed(() => {
  if (!users.value.length) return false
  return selectedUserIds.value.length === users.value.length
})

const toggleSelectAllUsers = () => {
  if (isAllUsersSelected.value) {
    selectedUserIds.value = []
  } else {
    selectedUserIds.value = users.value.map((u) => u.id)
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

const fetchAll = async () => {
  try {
    const [uRes, inbRes]: any = await Promise.all([api.get('/users'), api.get('/inbounds')])
    users.value = uRes || []
    availableInbounds.value = inbRes || []
  } catch (err) {
    console.error(err)
  }
}

const getNodeTags = (user: any): string[] => {
  if (user.inboundTags) {
    return user.inboundTags.split(',').map((s: string) => s.trim()).filter((s: string) => s)
  }
  if (user.inboundTag) return [user.inboundTag]
  return []
}

const getDirectTokenSubUrl = (user: any): string => {
  if (!user || !user.subToken) return ''
  return `${window.location.origin}/sub/${user.subToken}`
}

const copyDirectSubLink = (user: any) => {
  const url = getDirectTokenSubUrl(user)
  if (!url) {
    toast.error('该用户暂未生成专属订阅 Token')
    return
  }
  copyText(url)
}

const resetUserSubToken = async (userId: number) => {
  if (!confirm('确定重新生成该用户的安全订阅 Token 吗？旧的订阅链接将立即失效！')) return
  try {
    const res: any = await api.post(`/users/${userId}/reset-token`)
    toast.success('订阅 Token 重置成功，旧链接已失效！')
    if (currentShareData.value && currentShareData.value.user) {
      currentShareData.value.user.subToken = res.subToken
    }
    await fetchAll()
  } catch (err: any) {
    toast.error('重置失败: ' + err)
  }
}

// 批量操作
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
    totalGB: user.totalBytes > 0 ? Math.round(user.totalBytes / 1073741824) : 0,
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
      totalBytes: form.value.totalGB > 0 ? form.value.totalGB * 1073741824 : 0,
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
    showShareModal.value = true
  } catch (err: any) {
    toast.error('获取订阅链接失败: ' + err)
  }
}

// 流量历史统计分析 (Traffic History Analysis)
const openHistoryModal = async (user: any) => {
  currentHistoryUser.value = user
  historyDays.value = 14
  showHistoryModal.value = true
  await fetchUserHistory(user.id, 14)
}

const setHistoryDays = async (days: number) => {
  historyDays.value = days
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
  // 保持在 6% 到 90% 之间，留足顶部呼吸空间，避免顶到边框
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

const copyText = (text: string) => {
  if (!text) return
  navigator.clipboard.writeText(text)
  alert('已成功复制到剪贴板！')
}

const getTrafficPercent = (user: any) => {
  if (!user.totalBytes) return 0
  const used = user.upBytes + user.downBytes
  return (used / user.totalBytes) * 100
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

const formatDate = (ms: number) => {
  return new Date(ms).toLocaleDateString()
}

onMounted(() => {
  fetchAll()
})
</script>
