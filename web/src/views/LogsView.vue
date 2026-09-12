<template>
  <div class="space-y-5">
    <!-- Header & Controls Bar -->
    <div class="flex flex-col lg:flex-row items-start lg:items-center justify-between gap-4">
      <div>
        <h1 class="text-2xl font-extrabold text-white tracking-tight flex items-center gap-2.5">
          <span>运行、访问与审计日志</span>
          <span class="text-xs px-2.5 py-0.5 rounded-full font-mono bg-indigo-500/10 text-indigo-400 border border-indigo-500/20">
            Lightweight Inspector
          </span>
        </h1>
        <p class="text-xs text-gray-400 mt-1">后端定块秒级解析 Xray 客户端访问流向、路由决策、错误诊断与管理审计轨迹</p>
      </div>

      <div class="flex flex-wrap items-center gap-2.5">
        <!-- View Mode (Table / Terminal) - 仅在非审计模式下展示 -->
        <div v-if="logType !== 'audit'" class="bg-gray-900/90 p-1 rounded-xl border border-gray-800 flex shadow-sm">
          <button
            @click="viewMode = 'table'"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5"
            :class="viewMode === 'table' ? 'bg-indigo-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            <Table class="w-3.5 h-3.5" />
            <span>结构化表格</span>
          </button>
          <button
            @click="viewMode = 'terminal'"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all flex items-center gap-1.5"
            :class="viewMode === 'terminal' ? 'bg-indigo-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            <Terminal class="w-3.5 h-3.5" />
            <span>原始终端</span>
          </button>
        </div>

        <!-- Log Type (Access / Error / Audit) -->
        <div class="bg-gray-900/90 p-1 rounded-xl border border-gray-800 flex shadow-sm">
          <button
            @click="switchType('access')"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'access' ? 'bg-emerald-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            访问日志 (Access)
          </button>
          <button
            @click="switchType('error')"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'error' ? 'bg-rose-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            错误诊断 (Error)
          </button>
          <button
            @click="switchType('audit')"
            class="px-3 py-1.5 rounded-lg text-xs font-semibold transition-all"
            :class="logType === 'audit' ? 'bg-purple-600 text-white shadow-md' : 'text-gray-400 hover:text-white'"
          >
            操作审计 (Audit)
          </button>
        </div>

        <!-- Auto Refresh Toggle (默认关闭以保持零后台开销) -->
        <button
          @click="toggleAutoRefresh"
          class="px-3 py-2 rounded-xl text-xs border transition-all flex items-center gap-2 shadow-sm font-mono"
          :class="autoRefresh ? 'bg-emerald-500/15 text-emerald-300 border-emerald-500/30 font-semibold' : 'bg-gray-900/80 text-gray-400 border-gray-800'"
        >
          <span class="w-2 h-2 rounded-full" :class="autoRefresh ? 'bg-emerald-400 animate-ping' : 'bg-gray-600'"></span>
          <span>{{ autoRefresh ? '自动刷新 (5s)' : '按需加载' }}</span>
        </button>

        <!-- Refresh Button -->
        <button
          @click="fetchLogs"
          :disabled="loading"
          class="p-2.5 rounded-xl bg-gray-900 hover:bg-gray-800 text-gray-200 border border-gray-800 transition-colors shadow-sm"
          title="手动刷新"
        >
          <RefreshCw class="w-4 h-4" :class="{ 'animate-spin': loading }" />
        </button>

        <!-- Clear Logs Button (Context-aware) -->
        <button
          @click="showClearModal = true"
          class="px-3 py-2 rounded-xl bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/30 transition-all flex items-center gap-1.5 text-xs font-semibold shadow-sm"
          :title="logType === 'audit' ? '清空审计记录' : '清空日志文件'"
        >
          <Trash2 class="w-3.5 h-3.5" />
          <span>{{ logType === 'audit' ? '清空审计' : '清空日志' }}</span>
        </button>
      </div>
    </div>

    <!-- Filters & Selectors Card -->
    <div class="glass-panel p-3.5 sm:p-4 rounded-2xl border border-gray-800/80 flex flex-col md:flex-row items-stretch md:items-center justify-between gap-3 bg-[#0a0d14]/70">
      <!-- Search Input -->
      <div class="flex items-center gap-2.5 flex-1 bg-black/40 px-3.5 py-2 rounded-xl border border-gray-800/80 focus-within:border-indigo-500/60 transition-colors">
        <Search class="w-4 h-4 text-gray-500 shrink-0" />
        <input
          v-model="searchKeyword"
          @keyup.enter="handleKeywordSearch"
          type="text"
          :placeholder="searchPlaceholder"
          class="w-full bg-transparent text-white focus:outline-none placeholder-gray-500 font-mono text-xs"
        />
        <button v-if="searchKeyword" @click="clearKeyword" class="text-gray-500 hover:text-white text-xs">
          ✕
        </button>
      </div>

      <!-- Filters depending on type -->
      <div class="flex flex-wrap items-center gap-2 shrink-0">
        <!-- Inbound Dropdown Filter (Access Mode) -->
        <div v-if="logType === 'access'" class="flex items-center gap-1.5 bg-black/40 px-3 py-1.5 rounded-xl border border-gray-800 text-xs">
          <Radio class="w-3.5 h-3.5 text-cyan-400 shrink-0" />
          <span class="text-gray-400 shrink-0">入站:</span>
          <select
            v-model="selectedInbound"
            @change="fetchLogs"
            class="bg-transparent text-white font-mono font-medium focus:outline-none cursor-pointer pr-2 text-xs"
          >
            <option value="" class="bg-gray-900 text-gray-300">全部入站</option>
            <option v-for="tag in inboundOptions" :key="tag" :value="tag" class="bg-gray-900 text-cyan-300">
              {{ tag }}
            </option>
          </select>
        </div>

        <!-- User Dropdown Filter (Access Mode) -->
        <div v-if="logType === 'access'" class="flex items-center gap-1.5 bg-black/40 px-3 py-1.5 rounded-xl border border-gray-800 text-xs">
          <User class="w-3.5 h-3.5 text-indigo-400 shrink-0" />
          <span class="text-gray-400 shrink-0">用户:</span>
          <select
            v-model="selectedUserEmail"
            class="bg-transparent text-white font-mono font-medium focus:outline-none cursor-pointer pr-2 text-xs"
          >
            <option value="" class="bg-gray-900 text-gray-300">全部用户</option>
            <option v-for="email in userOptions" :key="email" :value="email" class="bg-gray-900 text-indigo-300">
              {{ email }}
            </option>
          </select>
        </div>

        <!-- Level Filter (Error Mode) -->
        <div v-else-if="logType === 'error'" class="flex items-center gap-1.5 bg-black/40 px-3 py-1.5 rounded-xl border border-gray-800 text-xs">
          <ShieldAlert class="w-3.5 h-3.5 text-amber-400 shrink-0" />
          <span class="text-gray-400 shrink-0">级别:</span>
          <select
            v-model="selectedLevel"
            class="bg-transparent text-white font-mono font-medium focus:outline-none cursor-pointer pr-2 text-xs"
          >
            <option value="" class="bg-gray-900 text-gray-300">全部级别</option>
            <option value="ERROR" class="bg-gray-900 text-rose-400">仅 Error (错误)</option>
            <option value="WARN" class="bg-gray-900 text-amber-400">仅 Warn (警告)</option>
            <option value="INFO" class="bg-gray-900 text-cyan-400">仅 Info (信息)</option>
          </select>
        </div>

        <!-- Action Category Filter (Audit Mode) -->
        <div v-else-if="logType === 'audit'" class="flex items-center gap-1.5 bg-black/40 px-3 py-1.5 rounded-xl border border-gray-800 text-xs">
          <ShieldCheck class="w-3.5 h-3.5 text-purple-400 shrink-0" />
          <span class="text-gray-400 shrink-0">动作:</span>
          <select
            v-model="selectedAction"
            @change="fetchLogs"
            class="bg-transparent text-white font-mono font-medium focus:outline-none cursor-pointer pr-2 text-xs max-w-[160px] truncate"
          >
            <option v-for="opt in auditActionOptions" :key="opt.value" :value="opt.value" class="bg-gray-900 text-gray-300">
              {{ opt.label }}
            </option>
          </select>
        </div>

        <!-- Lines / PageSize Selector -->
        <select
          v-if="logType !== 'audit'"
          v-model="maxLines"
          @change="fetchLogs"
          class="bg-black/40 text-gray-300 px-3 py-2 rounded-xl border border-gray-800 text-xs font-mono focus:outline-none cursor-pointer"
        >
          <option :value="100">100 行</option>
          <option :value="200">200 行</option>
          <option :value="500">500 行</option>
          <option :value="1000">1000 行</option>
        </select>

        <select
          v-else
          v-model="auditPageSize"
          @change="fetchLogs"
          class="bg-black/40 text-gray-300 px-3 py-2 rounded-xl border border-gray-800 text-xs font-mono focus:outline-none cursor-pointer"
        >
          <option :value="20">20 条/页</option>
          <option :value="50">50 条/页</option>
          <option :value="100">100 条/页</option>
        </select>

        <!-- Sort Order (Desc / Asc) -->
        <button
          @click="toggleSortOrder"
          class="px-3 py-2 rounded-xl text-xs border transition-all flex items-center gap-1.5 shadow-sm font-mono"
          :class="sortOrder === 'desc' ? 'bg-indigo-500/15 text-indigo-300 border-indigo-500/30 font-semibold' : 'bg-gray-900/80 text-gray-400 border-gray-800'"
          :title="sortOrder === 'desc' ? '当前：最新记录在顶端 (倒序)' : '当前：旧记录在顶端 (正序)'"
        >
          <ArrowUpDown class="w-3.5 h-3.5" />
          <span>{{ sortOrder === 'desc' ? '最新在顶' : '正序排列' }}</span>
        </button>
      </div>
    </div>

    <!-- 1. Access Logs Structured View -->
    <div v-if="logType === 'access' && viewMode === 'table'" class="space-y-3">
      <!-- Desktop Access Table -->
      <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90 hidden md:block">
        <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
            <span class="w-2 h-2 rounded-full bg-emerald-400"></span>
            <span>结构化访问记录 (共 {{ filteredAccessLogs.length }} 条)</span>
          </div>
          <div class="flex items-center gap-2">
            <span v-if="selectedInbound" class="text-cyan-400 font-mono text-[11px] bg-cyan-950/40 px-2 py-0.5 rounded border border-cyan-800/40">
              入站节点: {{ selectedInbound }}
            </span>
            <span v-if="selectedUserEmail" class="text-indigo-400 font-mono text-[11px] bg-indigo-950/40 px-2 py-0.5 rounded border border-indigo-800/40">
              过滤用户: {{ selectedUserEmail }}
            </span>
          </div>
        </div>

        <div class="overflow-x-auto max-h-[620px] overflow-y-auto">
          <table class="w-full text-left text-[13px] border-collapse font-sans">
            <thead class="text-gray-400 bg-gray-950/80 border-b border-gray-800 sticky top-0 z-10 text-xs font-semibold uppercase tracking-wider backdrop-blur-md">
              <tr>
                <th class="py-3 px-4">时间</th>
                <th class="py-3 px-4">用户 Email</th>
                <th class="py-3 px-4">客户端源</th>
                <th class="py-3 px-4">目标地址 / 端口</th>
                <th class="py-3 px-4">路由流向 (入站 → 出站)</th>
                <th class="py-3 px-4 text-center">状态</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-800/50 font-mono text-xs">
              <tr
                v-for="(row, idx) in filteredAccessLogs"
                :key="idx"
                class="hover:bg-white/[0.03] transition-colors"
              >
                <td class="py-3 px-4 text-gray-400 whitespace-nowrap">{{ row.time }}</td>

                <!-- User Email -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <span
                    v-if="row.email"
                    @click="selectedUserEmail = row.email"
                    class="px-2.5 py-1 rounded-lg text-[11px] font-semibold bg-indigo-500/10 text-indigo-300 border border-indigo-500/20 hover:bg-indigo-500/20 cursor-pointer transition-all inline-block"
                    title="点击以此用户过滤"
                  >
                    {{ row.email }}
                  </span>
                  <span v-else class="text-gray-500 text-[11px] italic">匿名 / 系统</span>
                </td>

                <!-- Client IP -->
                <td class="py-3 px-4 text-gray-300 whitespace-nowrap">{{ row.from_ip || '-' }}</td>

                <!-- Target Address -->
                <td class="py-3 px-4 text-white font-medium break-all max-w-[280px]">
                  <span class="text-cyan-400 font-semibold">{{ row.protocol || 'TCP' }}</span>
                  <span class="text-gray-500 mx-1">:</span>
                  <span class="text-gray-200">{{ row.target }}</span>
                </td>

                <!-- Routing Decision (Separated Inbound & Outbound) -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <div v-if="row.inbound_tag || row.outbound_tag" class="flex items-center gap-1.5">
                    <span
                      v-if="row.inbound_tag"
                      @click="selectedInbound = row.inbound_tag; fetchLogs()"
                      class="px-2 py-0.5 rounded text-[11px] font-bold font-mono bg-cyan-500/10 text-cyan-300 border border-cyan-500/30 hover:bg-cyan-500/20 cursor-pointer transition-all"
                      :title="'点击以此入站过滤: ' + row.inbound_tag"
                    >
                      {{ row.inbound_tag }}
                    </span>
                    <span class="text-gray-500 text-xs">→</span>
                    <span
                      class="px-2 py-0.5 rounded text-[11px] font-bold font-mono border"
                      :class="getRouteBadgeClass(row.outbound_tag || row.route)"
                      :title="'出站分流: ' + (row.outbound_tag || row.route || 'direct')"
                    >
                      {{ row.outbound_tag || row.route || 'direct' }}
                    </span>
                  </div>
                  <span
                    v-else
                    class="px-2.5 py-1 rounded-lg text-[11px] font-bold border"
                    :class="getRouteBadgeClass(row.route)"
                  >
                    {{ row.route || 'direct' }}
                  </span>
                </td>

                <!-- Action / Status -->
                <td class="py-3 px-4 text-center whitespace-nowrap">
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-semibold font-mono"
                    :class="row.action === 'accepted' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'"
                  >
                    {{ row.action || 'accepted' }}
                  </span>
                </td>
              </tr>

              <tr v-if="!filteredAccessLogs.length">
                <td colspan="6" class="text-center py-16 text-gray-500 font-sans text-xs">
                  暂无符合条件的访问记录
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Mobile Access Log Stream -->
      <div class="space-y-2.5 md:hidden">
        <div
          v-for="(row, idx) in filteredAccessLogs"
          :key="idx"
          class="glass-panel p-3.5 rounded-2xl border border-gray-800 space-y-2 text-xs"
        >
          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-gray-400">{{ row.time }}</span>
            <span
              class="px-2 py-0.5 rounded text-[10px] font-bold"
              :class="row.action === 'accepted' ? 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30' : 'bg-rose-500/15 text-rose-400 border border-rose-500/30'"
            >
              {{ row.action || 'accepted' }}
            </span>
          </div>

          <div class="font-mono text-xs text-white break-all flex items-start gap-1">
            <span class="text-cyan-400 font-bold shrink-0">{{ row.protocol || 'TCP' }}</span>
            <span class="text-gray-500">:</span>
            <span class="text-gray-200">{{ row.target }}</span>
          </div>

          <div class="flex items-center justify-between text-[11px] font-mono pt-1.5 border-t border-white/[0.04]">
            <span class="text-indigo-300 font-medium truncate max-w-[140px]">{{ row.email || '匿名' }}</span>
            <div v-if="row.inbound_tag || row.outbound_tag" class="flex items-center gap-1 shrink-0">
              <span v-if="row.inbound_tag" class="px-1.5 py-0.5 rounded text-[10px] font-bold bg-cyan-500/15 text-cyan-300 border border-cyan-500/30">
                {{ row.inbound_tag }}
              </span>
              <span class="text-gray-500 text-[10px]">→</span>
              <span class="px-1.5 py-0.5 rounded text-[10px] font-bold border" :class="getRouteBadgeClass(row.outbound_tag || row.route)">
                {{ row.outbound_tag || row.route || 'direct' }}
              </span>
            </div>
            <span
              v-else
              class="px-2 py-0.5 rounded text-[10px] font-bold border shrink-0"
              :class="getRouteBadgeClass(row.route)"
            >
              {{ row.route || 'direct' }}
            </span>
          </div>
        </div>

        <div v-if="!filteredAccessLogs.length" class="text-center py-12 text-gray-500 font-sans text-xs glass-panel rounded-2xl border border-gray-800">
          暂无符合条件的访问记录
        </div>
      </div>
    </div>

    <!-- 2. Error / Diagnostic Structured View -->
    <div v-else-if="logType === 'error' && viewMode === 'table'" class="space-y-3">
      <!-- Desktop Error Table -->
      <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90 hidden md:block">
        <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
            <span class="w-2 h-2 rounded-full bg-rose-400"></span>
            <span>结构化错误与诊断报告 (共 {{ filteredErrorLogs.length }} 条)</span>
          </div>
          <div class="text-gray-400 font-mono text-[11px]">
            后端即时故障诊断
          </div>
        </div>

        <div class="overflow-x-auto max-h-[620px] overflow-y-auto">
          <table class="w-full text-left text-[13px] border-collapse font-sans">
            <thead class="text-gray-400 bg-gray-950/80 border-b border-gray-800 sticky top-0 z-10 text-xs font-semibold uppercase tracking-wider backdrop-blur-md">
              <tr>
                <th class="py-3 px-4">时间</th>
                <th class="py-3 px-4">级别</th>
                <th class="py-3 px-4">来源模块</th>
                <th class="py-3 px-4">错误原因与诊断详情</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-800/50 font-mono text-xs">
              <tr
                v-for="(row, idx) in filteredErrorLogs"
                :key="idx"
                class="hover:bg-white/[0.03] transition-colors"
              >
                <!-- Time -->
                <td class="py-3 px-4 text-gray-400 whitespace-nowrap">{{ row.time }}</td>

                <!-- Level -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <span
                    class="px-2.5 py-0.5 rounded text-[11px] font-bold border font-mono"
                    :class="getErrorLevelBadge(row.level)"
                  >
                    {{ row.level || 'ERROR' }}
                  </span>
                </td>

                <!-- Module -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <span class="px-2 py-0.5 rounded bg-gray-800/80 text-gray-300 border border-gray-700 text-[11px]">
                    {{ row.module || 'core' }}
                  </span>
                </td>

                <!-- Message & Diagnosis -->
                <td class="py-3 px-4 text-gray-200 break-all leading-relaxed font-sans text-xs">
                  <div class="flex items-start gap-2">
                    <span class="font-mono text-gray-300">{{ row.message }}</span>
                  </div>
                  <div v-if="row.smartTip" class="mt-1 text-[11px] text-amber-300/90 flex items-center gap-1 font-mono">
                    <span>💡 诊断建议: {{ row.smartTip }}</span>
                  </div>
                </td>
              </tr>

              <tr v-if="!filteredErrorLogs.length">
                <td colspan="4" class="text-center py-16 text-gray-500 font-sans text-xs">
                  暂无匹配的错误诊断日志 (系统运行平稳)
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Mobile Error Log Stream -->
      <div class="space-y-2.5 md:hidden">
        <div
          v-for="(row, idx) in filteredErrorLogs"
          :key="idx"
          class="glass-panel p-3.5 rounded-2xl border border-gray-800 space-y-2 text-xs"
        >
          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-gray-400">{{ row.time }}</span>
            <span
              class="px-2.5 py-0.5 rounded text-[10px] font-bold border font-mono"
              :class="getErrorLevelBadge(row.level)"
            >
              {{ row.level || 'ERROR' }}
            </span>
          </div>

          <div class="text-[11px] font-mono text-gray-400 flex items-center gap-1.5">
            <span class="px-2 py-0.5 rounded bg-gray-800/80 text-gray-300 border border-gray-700 text-[10px]">{{ row.module || 'core' }}</span>
          </div>

          <div class="text-xs font-mono text-gray-200 break-all leading-relaxed">{{ row.message }}</div>

          <div v-if="row.smartTip" class="text-[11px] text-amber-300 bg-amber-500/10 p-2.5 rounded-xl border border-amber-500/20 font-mono">
            💡 {{ row.smartTip }}
          </div>
        </div>

        <div v-if="!filteredErrorLogs.length" class="text-center py-12 text-gray-500 font-sans text-xs glass-panel rounded-2xl border border-gray-800">
          暂无匹配的错误诊断日志
        </div>
      </div>
    </div>

    <!-- 3. Audit Logs Structured View -->
    <div v-else-if="logType === 'audit'" class="space-y-3">
      <!-- Desktop Audit Table -->
      <div class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#06080F]/90 hidden md:block">
        <div class="px-5 py-3 bg-gray-900/60 border-b border-gray-800/80 flex items-center justify-between text-xs">
          <div class="flex items-center gap-2 font-mono text-gray-300 font-semibold">
            <span class="w-2 h-2 rounded-full bg-purple-400"></span>
            <span>管理审查日志 (共 {{ auditTotal }} 条记录)</span>
          </div>
          <div class="text-purple-300/80 font-mono text-[11px]">
            全量运维变更跟踪 (SQLite WAL 存储)
          </div>
        </div>

        <div class="overflow-x-auto max-h-[620px] overflow-y-auto">
          <table class="w-full text-left text-[13px] border-collapse font-sans">
            <thead class="text-gray-400 bg-gray-950/80 border-b border-gray-800 sticky top-0 z-10 text-xs font-semibold uppercase tracking-wider backdrop-blur-md">
              <tr>
                <th class="py-3 px-4">操作时间</th>
                <th class="py-3 px-4">操作人</th>
                <th class="py-3 px-4">来源 IP</th>
                <th class="py-3 px-4">动作类型</th>
                <th class="py-3 px-4">目标对象</th>
                <th class="py-3 px-4 text-center">状态</th>
                <th class="py-3 px-4">变更摘要</th>
                <th class="py-3 px-4 text-center">操作</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-800/50 font-mono text-xs">
              <tr
                v-for="row in displayedAuditLogs"
                :key="row.id"
                class="hover:bg-white/[0.03] transition-colors"
              >
                <!-- Time -->
                <td class="py-3 px-4 text-gray-400 whitespace-nowrap">{{ formatAuditTime(row.createdAt) }}</td>

                <!-- Operator -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <span class="px-2 py-0.5 rounded bg-gray-800/80 text-purple-300 border border-purple-500/20 font-semibold text-[11px]">
                    {{ row.operator || 'system' }}
                  </span>
                </td>

                <!-- IP -->
                <td class="py-3 px-4 text-gray-300 whitespace-nowrap">{{ row.clientIp || '-' }}</td>

                <!-- Action -->
                <td class="py-3 px-4 whitespace-nowrap">
                  <span
                    class="px-2.5 py-1 rounded-lg text-[11px] font-bold border"
                    :class="getAuditActionBadge(row.action)"
                  >
                    {{ row.action }}
                  </span>
                </td>

                <!-- Target -->
                <td class="py-3 px-4 text-gray-200 font-medium whitespace-nowrap max-w-[150px] truncate" :title="row.target">
                  {{ row.target || '-' }}
                </td>

                <!-- Status -->
                <td class="py-3 px-4 text-center whitespace-nowrap">
                  <span
                    class="px-2 py-0.5 rounded text-[10px] font-semibold"
                    :class="row.status === 'SUCCESS' ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20' : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'"
                  >
                    {{ row.status || 'SUCCESS' }}
                  </span>
                </td>

                <!-- Details Preview -->
                <td class="py-3 px-4 text-gray-300 font-sans text-xs max-w-[280px] truncate" :title="row.details">
                  {{ row.details || '-' }}
                </td>

                <!-- Action / Detail Button -->
                <td class="py-3 px-4 text-center whitespace-nowrap">
                  <button
                    @click="selectedAuditDetail = row"
                    class="px-2.5 py-1 rounded-lg text-[11px] bg-gray-800/80 hover:bg-gray-700 text-indigo-300 border border-gray-700 transition-colors inline-flex items-center gap-1 font-sans font-medium"
                  >
                    <Eye class="w-3 h-3" />
                    <span>详情</span>
                  </button>
                </td>
              </tr>

              <tr v-if="!displayedAuditLogs.length">
                <td colspan="8" class="text-center py-16 text-gray-500 font-sans text-xs">
                  暂无匹配的管理审查日志
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <!-- Audit Pagination Bar -->
        <div class="px-5 py-3 bg-gray-950/80 border-t border-gray-800 flex items-center justify-between text-xs font-mono">
          <div class="text-gray-400">
            共 {{ auditTotal }} 条记录，当前第 {{ auditPage }} / {{ auditTotalPages }} 页
          </div>
          <div class="flex items-center gap-2">
            <button
              @click="changeAuditPage(auditPage - 1)"
              :disabled="auditPage <= 1"
              class="px-3 py-1.5 rounded-lg border border-gray-800 bg-gray-900 text-gray-300 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed transition-all flex items-center gap-1"
            >
              <ChevronLeft class="w-3.5 h-3.5" />
              <span>上一页</span>
            </button>
            <button
              @click="changeAuditPage(auditPage + 1)"
              :disabled="auditPage >= auditTotalPages"
              class="px-3 py-1.5 rounded-lg border border-gray-800 bg-gray-900 text-gray-300 hover:text-white disabled:opacity-40 disabled:cursor-not-allowed transition-all flex items-center gap-1"
            >
              <span>下一页</span>
              <ChevronRight class="w-3.5 h-3.5" />
            </button>
          </div>
        </div>
      </div>

      <!-- Mobile Audit Log Stream -->
      <div class="space-y-2.5 md:hidden">
        <div
          v-for="row in displayedAuditLogs"
          :key="row.id"
          class="glass-panel p-3.5 rounded-2xl border border-gray-800 space-y-2.5 text-xs"
        >
          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-gray-400">{{ formatAuditTime(row.createdAt) }}</span>
            <span
              class="px-2 py-0.5 rounded text-[10px] font-bold"
              :class="row.status === 'SUCCESS' ? 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30' : 'bg-rose-500/15 text-rose-400 border border-rose-500/30'"
            >
              {{ row.status || 'SUCCESS' }}
            </span>
          </div>

          <div class="flex items-center justify-between text-[11px] font-mono">
            <span class="text-purple-300 font-semibold">{{ row.operator || 'system' }} ({{ row.clientIp || '-' }})</span>
            <span
              class="px-2 py-0.5 rounded text-[10px] font-bold border"
              :class="getAuditActionBadge(row.action)"
            >
              {{ row.action }}
            </span>
          </div>

          <div class="text-gray-200 font-sans text-xs break-all">
            <span class="text-gray-400 font-mono">对象: </span>
            <span class="font-semibold text-white">{{ row.target || '-' }}</span>
          </div>

          <div class="text-gray-300 font-sans text-xs bg-black/40 p-2 rounded-xl border border-gray-800/80 break-all line-clamp-2">
            {{ row.details || '-' }}
          </div>

          <div class="pt-1 flex justify-end">
            <button
              @click="selectedAuditDetail = row"
              class="px-3 py-1 rounded-lg text-xs bg-gray-800 hover:bg-gray-700 text-indigo-300 border border-gray-700 transition-colors flex items-center gap-1 font-sans"
            >
              <Eye class="w-3.5 h-3.5" />
              <span>查看详情</span>
            </button>
          </div>
        </div>

        <div v-if="!displayedAuditLogs.length" class="text-center py-12 text-gray-500 font-sans text-xs glass-panel rounded-2xl border border-gray-800">
          暂无匹配的管理审查日志
        </div>

        <!-- Mobile Pagination -->
        <div v-if="displayedAuditLogs.length" class="flex items-center justify-between glass-panel p-3 rounded-2xl border border-gray-800 text-xs font-mono">
          <span class="text-gray-400">{{ auditPage }} / {{ auditTotalPages }} 页</span>
          <div class="flex items-center gap-2">
            <button
              @click="changeAuditPage(auditPage - 1)"
              :disabled="auditPage <= 1"
              class="px-2.5 py-1 rounded-lg border border-gray-800 bg-gray-900 text-gray-300 disabled:opacity-40"
            >
              上页
            </button>
            <button
              @click="changeAuditPage(auditPage + 1)"
              :disabled="auditPage >= auditTotalPages"
              class="px-2.5 py-1 rounded-lg border border-gray-800 bg-gray-900 text-gray-300 disabled:opacity-40"
            >
              下页
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- 4. Terminal High-Contrast Log View (Raw Mode) -->
    <div v-else class="glass-panel rounded-2xl border border-gray-800/80 overflow-hidden shadow-2xl bg-[#04060B]">
      <!-- Terminal Header -->
      <div class="flex items-center justify-between px-4 py-2.5 bg-gray-900/90 border-b border-gray-800 text-xs">
        <div class="flex items-center gap-2">
          <span class="w-3 h-3 rounded-full bg-rose-500/80 inline-block shadow-sm"></span>
          <span class="w-3 h-3 rounded-full bg-amber-500/80 inline-block shadow-sm"></span>
          <span class="w-3 h-3 rounded-full bg-emerald-500/80 inline-block shadow-sm"></span>
          <span class="ml-2 font-mono text-gray-300 font-semibold">
            {{ logType === 'access' ? 'xray-access.log' : 'xray-error.log' }}
          </span>
        </div>
        <div class="text-gray-400 font-mono text-xs">
          展示最近 {{ filteredRawLines.length }} 行
        </div>
      </div>

      <!-- Terminal Output Box -->
      <div
        ref="logBox"
        class="h-[600px] overflow-y-auto p-4 sm:p-5 font-['JetBrains_Mono',monospace] text-[13.5px] leading-[1.7] select-text space-y-0.5"
      >
        <div
          v-for="(line, idx) in filteredRawLines"
          :key="idx"
          class="py-1 px-2 rounded hover:bg-white/[0.04] transition-colors flex items-start gap-3"
          :class="highlightLine(line)"
        >
          <span class="text-gray-600 select-none shrink-0 w-10 text-right font-mono text-xs pt-0.5">
            {{ Number(idx) + 1 }}
          </span>
          <span class="break-all whitespace-pre-wrap flex-1">{{ line }}</span>
        </div>

        <div v-if="!filteredRawLines.length" class="text-center text-gray-600 py-24 font-sans text-xs">
          暂无符合条件的日志记录
        </div>
      </div>
    </div>

    <!-- Clear Confirmation Modal -->
    <div
      v-if="showClearModal"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 animate-fade-in"
    >
      <div class="glass-panel w-full max-w-md p-6 rounded-2xl border border-gray-800 bg-[#0c1017] shadow-2xl space-y-4">
        <div class="flex items-center gap-3 text-rose-400">
          <div class="p-2.5 rounded-xl bg-rose-500/10 border border-rose-500/20">
            <AlertTriangle class="w-6 h-6" />
          </div>
          <div>
            <h3 class="text-base font-bold text-white">
              {{ logType === 'audit' ? '确认清空管理审查日志' : `确认清空 ${logType === 'access' ? '访问' : '错误'} 日志` }}
            </h3>
            <p class="text-xs text-gray-400 mt-0.5">高危维护操作，请谨慎确认</p>
          </div>
        </div>

        <div class="bg-rose-500/5 p-3.5 rounded-xl border border-rose-500/20 text-xs text-rose-300 leading-relaxed font-sans">
          <p v-if="logType === 'audit'">
            确定要清空所有管理员操作审查日志吗？清空后历史操作痕迹将不可恢复，且该清空操作本身将被记入新的审计条目。
          </p>
          <p v-else>
            确定要清空当前的 <strong>xray-{{ logType }}.log</strong> 磁盘文件吗？已记录的数据将被截断置空且无法撤销。
          </p>
        </div>

        <div class="flex items-center justify-end gap-3 pt-2">
          <button
            @click="showClearModal = false"
            :disabled="clearing"
            class="px-4 py-2 rounded-xl text-xs font-semibold text-gray-400 hover:text-white bg-gray-900 border border-gray-800 transition-colors"
          >
            取消
          </button>
          <button
            @click="handleClearConfirm"
            :disabled="clearing"
            class="px-4 py-2 rounded-xl text-xs font-semibold text-white bg-rose-600 hover:bg-rose-500 transition-all flex items-center gap-1.5 shadow-lg shadow-rose-900/30 disabled:opacity-50"
          >
            <RefreshCw v-if="clearing" class="w-3.5 h-3.5 animate-spin" />
            <Trash2 v-else class="w-3.5 h-3.5" />
            <span>{{ clearing ? '正在清空...' : '确认清空' }}</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Audit Log Detail Modal -->
    <div
      v-if="selectedAuditDetail"
      class="fixed inset-0 z-50 bg-black/80 backdrop-blur-sm flex items-center justify-center p-4 animate-fade-in"
    >
      <div class="glass-panel w-full max-w-2xl p-6 rounded-2xl border border-gray-800 bg-[#0c1017] shadow-2xl space-y-4 max-h-[90vh] flex flex-col">
        <div class="flex items-center justify-between pb-3 border-b border-gray-800">
          <div class="flex items-center gap-2.5">
            <ShieldCheck class="w-5 h-5 text-purple-400" />
            <h3 class="text-base font-bold text-white">审查日志详细记录 #{{ selectedAuditDetail.id }}</h3>
          </div>
          <button
            @click="selectedAuditDetail = null"
            class="p-1.5 rounded-lg text-gray-400 hover:text-white hover:bg-gray-800 transition-colors"
          >
            <X class="w-4 h-4" />
          </button>
        </div>

        <div class="space-y-4 overflow-y-auto pr-1">
          <!-- Key Fields Grid -->
          <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 text-xs font-mono">
            <div class="bg-black/40 p-3 rounded-xl border border-gray-800/80">
              <span class="text-gray-500 block text-[11px] mb-1">操作时间</span>
              <span class="text-gray-200">{{ formatAuditTime(selectedAuditDetail.createdAt) }}</span>
            </div>
            <div class="bg-black/40 p-3 rounded-xl border border-gray-800/80">
              <span class="text-gray-500 block text-[11px] mb-1">操作人员</span>
              <span class="text-purple-300 font-semibold">{{ selectedAuditDetail.operator || 'system' }}</span>
            </div>
            <div class="bg-black/40 p-3 rounded-xl border border-gray-800/80">
              <span class="text-gray-500 block text-[11px] mb-1">客户端 IP</span>
              <span class="text-gray-200">{{ selectedAuditDetail.clientIp || '-' }}</span>
            </div>
            <div class="bg-black/40 p-3 rounded-xl border border-gray-800/80">
              <span class="text-gray-500 block text-[11px] mb-1">动作标识</span>
              <span
                class="px-2 py-0.5 rounded text-[10px] font-bold border inline-block"
                :class="getAuditActionBadge(selectedAuditDetail.action)"
              >
                {{ selectedAuditDetail.action }}
              </span>
            </div>
            <div class="bg-black/40 p-3 rounded-xl border border-gray-800/80">
              <span class="text-gray-500 block text-[11px] mb-1">目标对象</span>
              <span class="text-white font-medium">{{ selectedAuditDetail.target || '-' }}</span>
            </div>
            <div class="bg-black/40 p-3 rounded-xl border border-gray-800/80">
              <span class="text-gray-500 block text-[11px] mb-1">执行状态</span>
              <span
                class="px-2 py-0.5 rounded text-[10px] font-bold inline-block"
                :class="selectedAuditDetail.status === 'SUCCESS' ? 'bg-emerald-500/15 text-emerald-400 border border-emerald-500/30' : 'bg-rose-500/15 text-rose-400 border border-rose-500/30'"
              >
                {{ selectedAuditDetail.status || 'SUCCESS' }}
              </span>
            </div>
          </div>

          <!-- Full Details Payload -->
          <div class="space-y-1.5">
            <span class="text-xs font-semibold text-gray-300">变更记录详情与载荷</span>
            <div class="bg-black/60 p-4 rounded-xl border border-gray-800 font-mono text-xs text-gray-200 break-all whitespace-pre-wrap select-text max-h-[280px] overflow-y-auto leading-relaxed">
              {{ selectedAuditDetail.details || '(无附加详情)' }}
            </div>
          </div>
        </div>

        <div class="pt-3 border-t border-gray-800 flex justify-end">
          <button
            @click="selectedAuditDetail = null"
            class="px-4 py-2 rounded-xl text-xs font-semibold text-white bg-indigo-600 hover:bg-indigo-500 transition-colors shadow-md"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, shallowRef, computed, onMounted, onUnmounted } from 'vue'
import {
  RefreshCw,
  Search,
  Table,
  Terminal,
  User,
  ShieldAlert,
  ArrowUpDown,
  Radio,
  ShieldCheck,
  Trash2,
  Eye,
  ChevronLeft,
  ChevronRight,
  X,
  AlertTriangle,
} from 'lucide-vue-next'
import api from '../api'
import { toast } from '../utils/toast'

const viewMode = ref<'table' | 'terminal'>('table')
const logType = ref<'access' | 'error' | 'audit'>('access')
const autoRefresh = ref(false)
const loading = ref(false)
const clearing = ref(false)
const sortOrder = ref<'desc' | 'asc'>('desc')
const searchKeyword = ref('')
const selectedUserEmail = ref('')
const selectedInbound = ref('')
const selectedLevel = ref('')
const selectedAction = ref('')
const maxLines = ref(200)

// 审计分页状态
const auditPage = ref(1)
const auditPageSize = ref(50)
const auditTotal = ref(0)
const auditTotalPages = computed(() => Math.max(1, Math.ceil(auditTotal.value / auditPageSize.value)))

// 弹窗状态
const showClearModal = ref(false)
const selectedAuditDetail = ref<any | null>(null)

// 使用 shallowRef 避免深层响应式开销
const rawAccessEntries = shallowRef<any[]>([])
const rawErrorEntries = shallowRef<any[]>([])
const rawLines = shallowRef<string[]>([])
const auditEntries = shallowRef<any[]>([])
const knownUsers = ref<any[]>([])
const knownInbounds = ref<any[]>([])
let timer: any = null

const auditActionOptions = [
  { label: '全部操作', value: '' },
  { label: '用户登录 (AUTH_LOGIN)', value: 'AUTH_LOGIN' },
  { label: '用户登出 (AUTH_LOGOUT)', value: 'AUTH_LOGOUT' },
  { label: '创建用户 (USER_CREATE)', value: 'USER_CREATE' },
  { label: '更新用户 (USER_UPDATE)', value: 'USER_UPDATE' },
  { label: '删除用户 (USER_DELETE)', value: 'USER_DELETE' },
  { label: '重置流量 (USER_RESET_TRAFFIC)', value: 'USER_RESET_TRAFFIC' },
  { label: '创建入站 (INBOUND_CREATE)', value: 'INBOUND_CREATE' },
  { label: '更新入站 (INBOUND_UPDATE)', value: 'INBOUND_UPDATE' },
  { label: '删除入站 (INBOUND_DELETE)', value: 'INBOUND_DELETE' },
  { label: '创建出站 (OUTBOUND_CREATE)', value: 'OUTBOUND_CREATE' },
  { label: '更新出站 (OUTBOUND_UPDATE)', value: 'OUTBOUND_UPDATE' },
  { label: '删除出站 (OUTBOUND_DELETE)', value: 'OUTBOUND_DELETE' },
  { label: '创建路由 (ROUTING_RULE_CREATE)', value: 'ROUTING_RULE_CREATE' },
  { label: '更新路由 (ROUTING_RULE_UPDATE)', value: 'ROUTING_RULE_UPDATE' },
  { label: '删除路由 (ROUTING_RULE_DELETE)', value: 'ROUTING_RULE_DELETE' },
  { label: '配置保存 (CONFIG_UPDATE)', value: 'CONFIG_UPDATE' },
  { label: '配置重载 (CONFIG_RELOAD)', value: 'CONFIG_RELOAD' },
  { label: '更新设置 (SETTING_UPDATE)', value: 'SETTING_UPDATE' },
  { label: '清空日志 (LOG_CLEAR)', value: 'LOG_CLEAR' },
  { label: '清空审计 (AUDIT_CLEAR)', value: 'AUDIT_CLEAR' },
]

const searchPlaceholder = computed(() => {
  if (logType.value === 'access') return '快速搜索关键词 (如 IP、目标域名、分流出站)...'
  if (logType.value === 'error') return '快速搜索错误内容 (如 DNS, account, timeout, rejected)...'
  return '搜索审计记录 (操作人、目标对象、动作或详情)...'
})

const toggleSortOrder = () => {
  sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
}

const clearKeyword = () => {
  searchKeyword.value = ''
  if (logType.value === 'audit') {
    auditPage.value = 1
    fetchLogs()
  }
}

const handleKeywordSearch = () => {
  if (logType.value === 'audit') {
    auditPage.value = 1
    fetchLogs()
  }
}

const fetchLogs = async () => {
  loading.value = true
  try {
    if (logType.value === 'audit') {
      const params = new URLSearchParams()
      params.append('page', auditPage.value.toString())
      params.append('pageSize', auditPageSize.value.toString())
      if (selectedAction.value) {
        params.append('action', selectedAction.value)
      }
      if (searchKeyword.value) {
        params.append('keyword', searchKeyword.value)
      }

      const res: any = await api.get(`/audit-logs?${params.toString()}`)
      if (res) {
        auditEntries.value = res.items || []
        auditTotal.value = res.total || 0
      }
    } else {
      const params = new URLSearchParams()
      params.append('type', logType.value)
      params.append('lines', maxLines.value.toString())
      if (logType.value === 'access' && selectedInbound.value) {
        params.append('inbound', selectedInbound.value)
      }
      if (searchKeyword.value) {
        params.append('keyword', searchKeyword.value)
      }

      const res: any = await api.get(`/logs?${params.toString()}`)
      if (res) {
        if (res.access) {
          rawAccessEntries.value = res.access
        }
        if (res.errors) {
          rawErrorEntries.value = res.errors
        }
        if (res.lines) {
          rawLines.value = res.lines
        }
      }
    }
  } catch (err) {
    console.error('Failed to fetch logs', err)
  } finally {
    loading.value = false
  }
}

const fetchUsers = async () => {
  try {
    const uList: any = await api.get('/users')
    knownUsers.value = uList || []
  } catch (e) {
    console.error(e)
  }
}

const fetchInbounds = async () => {
  try {
    const inbounds: any = await api.get('/inbounds')
    knownInbounds.value = inbounds || []
  } catch (e) {
    console.error(e)
  }
}

const switchType = (type: 'access' | 'error' | 'audit') => {
  if (logType.value === type) return
  logType.value = type
  if (type === 'audit') {
    viewMode.value = 'table'
    auditPage.value = 1
  }
  fetchLogs()
}

const changeAuditPage = (page: number) => {
  if (page < 1 || page > auditTotalPages.value) return
  auditPage.value = page
  fetchLogs()
}

const toggleAutoRefresh = () => {
  autoRefresh.value = !autoRefresh.value
  if (autoRefresh.value) {
    timer = setInterval(fetchLogs, 5000)
  } else if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const handleClearConfirm = async () => {
  clearing.value = true
  try {
    if (logType.value === 'audit') {
      await api.delete('/audit-logs')
      toast.success('操作审查记录已清空')
    } else {
      await api.post(`/logs/clear?type=${logType.value}`)
      toast.success(`Xray ${logType.value === 'access' ? '访问' : '错误'}日志已清空`)
    }
    showClearModal.value = false
    await fetchLogs()
  } catch (err: any) {
    toast.error('清空失败: ' + (err?.message || err))
  } finally {
    clearing.value = false
  }
}

// 收集所有已知与日志中解析出的 User Email
const userOptions = computed(() => {
  const emailSet = new Set<string>()
  for (const u of knownUsers.value) {
    if (u.email) emailSet.add(u.email)
  }
  for (const item of rawAccessEntries.value) {
    if (item.email) emailSet.add(item.email)
  }
  return Array.from(emailSet)
})

// 收集所有入站 Tag (已知入站 + 访问日志解析)
const inboundOptions = computed(() => {
  const tagSet = new Set<string>()
  for (const inb of knownInbounds.value) {
    if (inb.tag) tagSet.add(inb.tag)
  }
  for (const item of rawAccessEntries.value) {
    if (item.inbound_tag) tagSet.add(item.inbound_tag)
  }
  return Array.from(tagSet)
})

// 1. 结构化 Access 访问日志
const filteredAccessLogs = computed(() => {
  if (logType.value !== 'access') return []

  const kw = searchKeyword.value.toLowerCase()
  const filterEmail = selectedUserEmail.value.toLowerCase()
  const filterInbound = selectedInbound.value.toLowerCase()

  const list = rawAccessEntries.value.filter((item) => {
    if (filterEmail && (!item.email || !item.email.toLowerCase().includes(filterEmail))) {
      return false
    }
    if (filterInbound && item.inbound_tag && !item.inbound_tag.toLowerCase().includes(filterInbound)) {
      return false
    }
    if (kw) {
      const match =
        (item.target && item.target.toLowerCase().includes(kw)) ||
        (item.from_ip && item.from_ip.includes(kw)) ||
        (item.route && item.route.toLowerCase().includes(kw)) ||
        (item.inbound_tag && item.inbound_tag.toLowerCase().includes(kw)) ||
        (item.outbound_tag && item.outbound_tag.toLowerCase().includes(kw)) ||
        (item.email && item.email.toLowerCase().includes(kw))
      if (!match) return false
    }
    return true
  })

  return sortOrder.value === 'desc' ? [...list].reverse() : list
})

// 2. 结构化 Error 诊断日志
const filteredErrorLogs = computed(() => {
  if (logType.value !== 'error') return []

  const kw = searchKeyword.value.toLowerCase()
  const filterLvl = selectedLevel.value.toUpperCase()

  const list = rawErrorEntries.value.filter((item) => {
    if (filterLvl && item.level !== filterLvl) {
      return false
    }
    if (kw) {
      const match =
        (item.message && item.message.toLowerCase().includes(kw)) ||
        (item.module && item.module.toLowerCase().includes(kw)) ||
        (item.smartTip && item.smartTip.toLowerCase().includes(kw))
      if (!match) return false
    }
    return true
  })

  return sortOrder.value === 'desc' ? [...list].reverse() : list
})

// 3. 结构化 Audit 审计日志展示
const displayedAuditLogs = computed(() => {
  if (logType.value !== 'audit') return []
  const list = auditEntries.value
  return sortOrder.value === 'asc' ? [...list].reverse() : list
})

// 4. 原始终端高亮筛选
const filteredRawLines = computed(() => {
  if (!rawLines.value) return []
  const kw = searchKeyword.value.toLowerCase()
  const filterEmail = selectedUserEmail.value.toLowerCase()
  const filterInbound = selectedInbound.value.toLowerCase()
  const filterLvl = selectedLevel.value.toLowerCase()

  const list = rawLines.value.filter((line: string) => {
    const l = line.toLowerCase()
    if (kw && !l.includes(kw)) return false
    if (logType.value === 'access' && filterEmail && !l.includes(filterEmail)) return false
    if (logType.value === 'access' && filterInbound && !l.includes(filterInbound)) return false
    if (logType.value === 'error' && filterLvl && !l.includes(filterLvl)) return false
    return true
  })

  return sortOrder.value === 'desc' ? [...list].reverse() : list
})

const getRouteBadgeClass = (route: string) => {
  if (!route) return 'bg-gray-800 text-gray-400 border-gray-700'
  if (route.includes('warp')) return 'bg-cyan-500/10 text-cyan-300 border-cyan-500/30'
  if (route.includes('direct')) return 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
  if (route.includes('block')) return 'bg-rose-500/10 text-rose-300 border-rose-500/30'
  return 'bg-indigo-500/10 text-indigo-300 border-indigo-500/30'
}

const getErrorLevelBadge = (level: string) => {
  const l = (level || '').toUpperCase()
  if (l.includes('ERROR')) return 'bg-rose-500/15 text-rose-400 border-rose-500/30'
  if (l.includes('WARN')) return 'bg-amber-500/15 text-amber-400 border-amber-500/30'
  if (l.includes('INFO')) return 'bg-cyan-500/15 text-cyan-400 border-cyan-500/30'
  return 'bg-gray-800 text-gray-300 border-gray-700'
}

const getAuditActionBadge = (action: string) => {
  const a = (action || '').toUpperCase()
  if (a.startsWith('AUTH_')) return 'bg-blue-500/10 text-blue-300 border-blue-500/30'
  if (a.startsWith('USER_')) return 'bg-amber-500/10 text-amber-300 border-amber-500/30'
  if (a.startsWith('INBOUND_')) return 'bg-cyan-500/10 text-cyan-300 border-cyan-500/30'
  if (a.startsWith('OUTBOUND_')) return 'bg-indigo-500/10 text-indigo-300 border-indigo-500/30'
  if (a.startsWith('ROUTING_')) return 'bg-emerald-500/10 text-emerald-300 border-emerald-500/30'
  if (a.startsWith('CONFIG_') || a.startsWith('SETTING_')) return 'bg-purple-500/10 text-purple-300 border-purple-500/30'
  if (a.startsWith('LOG_') || a.startsWith('AUDIT_')) return 'bg-rose-500/10 text-rose-300 border-rose-500/30'
  return 'bg-gray-800 text-gray-300 border-gray-700'
}

const formatAuditTime = (timeStr: string) => {
  if (!timeStr) return '-'
  try {
    const d = new Date(timeStr)
    return d.toLocaleString('zh-CN', {
      hour12: false,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return timeStr
  }
}

const highlightLine = (line: string) => {
  const lower = line.toLowerCase()
  if (lower.includes('warn') || lower.includes('[warning]')) return 'text-amber-300 bg-amber-500/5'
  if (lower.includes('error') || lower.includes('[error]') || lower.includes('failed') || lower.includes('rejected')) return 'text-rose-300 bg-rose-500/10 font-bold'
  if (lower.includes('accepted')) return 'text-emerald-300'
  if (lower.includes('warp')) return 'text-cyan-300'
  if (lower.includes('doh')) return 'text-indigo-300'
  return 'text-gray-300'
}

const handleVisibilityChange = () => {
  if (document.hidden) {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  } else {
    if (autoRefresh.value) {
      fetchLogs()
      if (!timer) {
        timer = setInterval(fetchLogs, 5000)
      }
    }
  }
}

onMounted(() => {
  fetchLogs()
  fetchUsers()
  fetchInbounds()
  document.addEventListener('visibilitychange', handleVisibilityChange)
})

onUnmounted(() => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>
