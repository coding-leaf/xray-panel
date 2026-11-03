<template>
  <div class="space-y-4">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">路由与分流策略 (Routing)</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ routingConfig.rules?.length || 0 }} 条规则
          </Badge>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20">
            修改自动重启核心生效
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          自上而下优先匹配分流规则，出站与入站标签动态绑定，保存落盘后自动全量重启核心生效
        </p>
      </div>

      <div class="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          @click="openAddRuleDrawer"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加规则</span>
        </Button>

        <Button
          variant="default"
          size="sm"
          :loading="saving"
          @click="saveAllRouting"
        >
          <Check class="w-3.5 h-3.5 mr-1" />
          <span>{{ saving ? '保存中...' : '保存并应用' }}</span>
        </Button>
      </div>
    </div>

    <!-- GeoData 规则库在线升级状态卡片 -->
    <div class="p-3.5 rounded-lg bg-card border border-border flex flex-col gap-3">
      <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3">
        <div class="flex items-center gap-3">
          <div class="p-2 rounded bg-neutral-900 border border-border text-foreground">
            <Database class="w-4 h-4 text-cyan-400" />
          </div>
          <div class="space-y-0.5 text-xs">
            <div class="flex items-center gap-2">
              <span class="font-semibold text-foreground">GeoData 分流规则库 (geoip.dat & geosite.dat)</span>
              <span class="px-1.5 py-0.2 rounded text-[10px] font-mono bg-neutral-900 border border-border text-cyan-400">
                {{ geodataStatus?.platform || 'Xray Core' }}
              </span>
            </div>
            <p class="text-[11px] text-muted-foreground font-mono">
              GeoIP: {{ geodataStatus?.geoipExists ? `${(geodataStatus.geoipSize / 1048576).toFixed(2)} MB` : '未找到' }} | 
              GeoSite: {{ geodataStatus?.geositeExists ? `${(geodataStatus.geositeSize / 1048576).toFixed(2)} MB` : '未找到' }} | 
              路径: {{ geodataStatus?.targetDirectory || './' }}
            </p>
          </div>
        </div>

        <Button
          variant="secondary"
          size="sm"
          :loading="updatingGeo"
          @click="updateGeoData"
          class="shrink-0"
        >
          <RotateCw class="w-3.5 h-3.5 mr-1.5" :class="{ 'animate-spin': updatingGeo }" />
          <span>{{ updatingGeo ? '规则库升级中...' : '一键升级规则库' }}</span>
        </Button>
      </div>

      <!-- 实时下载进度条 -->
      <div v-if="updatingGeo || (geoProgress.percentage > 0 && geoProgress.percentage < 100)" class="p-2.5 bg-neutral-950 rounded-md border border-border space-y-1.5 transition-all">
        <div class="flex items-center justify-between text-xs font-mono">
          <div class="flex items-center gap-2">
            <span class="w-1.5 h-1.5 rounded-full" :class="updatingGeo ? 'bg-cyan-400 animate-ping' : 'bg-emerald-400'"></span>
            <span class="text-muted-foreground text-[11px]">
              {{ geoProgress.message || '正在准备下载...' }}
            </span>
          </div>
          <span class="font-bold text-cyan-400 text-xs">
            {{ geoProgress.percentage }}%
          </span>
        </div>

        <div class="w-full h-1.5 bg-neutral-900 rounded-full overflow-hidden border border-border/60">
          <div
            class="h-full rounded-full bg-cyan-500 transition-all duration-300"
            :style="{ width: `${geoProgress.percentage}%` }"
          ></div>
        </div>
      </div>
    </div>

    <!-- Domain Strategy & Presets Bar -->
    <div class="grid grid-cols-1 lg:grid-cols-3 gap-3">
      <!-- Strategy Selector -->
      <div class="p-3.5 rounded-lg bg-card border border-border space-y-2">
        <label class="block text-xs font-semibold text-foreground uppercase tracking-wider font-mono">
          域名解析策略 (domainStrategy)
        </label>
        <select
          v-model="routingConfig.domainStrategy"
          class="w-full bg-neutral-950 border border-border rounded-md px-2.5 py-1.5 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
        >
          <option value="IPIfNonMatch">IPIfNonMatch (推荐: 未命中则解析为IP再次匹配)</option>
          <option value="AsIs">AsIs (保持原样: 仅匹配客户端直发域名)</option>
          <option value="IPOnDemand">IPOnDemand (强制实时解析IP匹配)</option>
        </select>
        <p class="text-[11px] text-muted-foreground leading-relaxed">
          配合 Inbound 域名嗅探 (Sniffing)，精准识别 TLS/HTTP 连接真实域名。
        </p>
      </div>

      <!-- Presets quick buttons (Interactive Wizard) -->
      <div class="p-3.5 rounded-lg bg-card border border-border lg:col-span-2 space-y-2">
        <div class="flex items-center justify-between">
          <span class="text-xs font-semibold text-foreground uppercase tracking-wider font-mono">
            智能预设向导 (快速注入常用分流规则)
          </span>
          <span class="text-[11px] text-muted-foreground font-mono">点击唤起配置向导</span>
        </div>

        <div class="flex flex-wrap gap-2 pt-1">
          <button
            @click="openPresetWizard('ads')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>🛡️ 拦截广告 (category-ads)</span>
          </button>

          <button
            @click="openPresetWizard('private_ip')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>🔒 屏蔽私有局域网 (private)</span>
          </button>

          <button
            @click="openPresetWizard('bt')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>🚫 拦截 BT 下载 (bittorrent)</span>
          </button>

          <button
            @click="openPresetWizard('smtp')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>📧 封禁邮件 25 端口 (port: 25)</span>
          </button>

          <button
            @click="openPresetWizard('cn')"
            class="px-2.5 py-1.5 rounded-md bg-neutral-950 hover:bg-neutral-900 border border-border text-xs text-muted-foreground hover:text-foreground transition-all flex items-center gap-1.5 font-mono"
          >
            <span>⚡ 国内流量分流 (geosite:cn)</span>
          </button>
        </div>
      </div>
    </div>

    <!-- Rules Table-First List (Desktop >= 640px) -->
    <div class="hidden sm:block relative w-full overflow-hidden rounded-lg border border-border bg-neutral-950">
      <table class="w-full caption-bottom text-xs border-collapse">
        <thead class="border-b border-border bg-neutral-900 font-mono text-[11px]">
          <tr>
            <th class="h-9 px-3 text-center align-middle font-medium text-muted-foreground uppercase tracking-wider w-24">次序</th>
            <th class="h-9 px-3 text-left align-middle font-medium text-muted-foreground uppercase tracking-wider">路由目标 (Target)</th>
            <th class="h-9 px-3 text-left align-middle font-medium text-muted-foreground uppercase tracking-wider">来源入站 (Inbounds)</th>
            <th class="h-9 px-3 text-left align-middle font-medium text-muted-foreground uppercase tracking-wider">匹配条件矩阵</th>
            <th class="h-9 px-3 text-right align-middle font-medium text-muted-foreground uppercase tracking-wider w-44 whitespace-nowrap">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr
            v-for="(rule, idx) in routingConfig.rules"
            :key="idx"
            :draggable="true"
            @dragstart="onDragStart($event, Number(idx))"
            @dragover.prevent="onDragOver($event, Number(idx))"
            @drop.prevent="onDrop(Number(idx))"
            @dragend="onDragEnd"
            class="transition-colors group"
            :class="[
              draggedIndex === idx
                ? 'opacity-30 bg-muted/30 border-dashed border-indigo-500/50'
                : (dragOverIndex === idx && draggedIndex !== idx
                    ? 'bg-indigo-500/10 border-t-2 border-indigo-500'
                    : 'hover:bg-muted/20')
            ]"
          >
            <!-- 1. Drag Handle & Index / Priority -->
            <td class="p-2.5 text-center align-middle font-mono">
              <div class="flex items-center justify-center gap-1.5">
                <span
                  class="cursor-grab active:cursor-grabbing p-1 text-muted-foreground hover:text-foreground transition-colors shrink-0"
                  title="按住拖拽调整顺序"
                >
                  <GripVertical class="w-4 h-4" />
                </span>
                <button
                  type="button"
                  @click="openQuickReorder(Number(idx))"
                  class="px-1.5 py-0.5 rounded font-mono text-[11px] font-semibold bg-neutral-900 border border-border/80 text-foreground hover:border-indigo-500/60 hover:text-indigo-400 hover:bg-neutral-800 transition-all shadow-sm shrink-0"
                  title="点击快速修改位次"
                >
                  #{{ Number(idx) + 1 }}
                </button>
              </div>
            </td>

            <!-- 2. Target Outbound / Balancer -->
            <td class="p-3 align-middle font-mono">
              <div class="flex items-center gap-1.5 flex-wrap">
                <Badge v-if="rule.balancerTag" variant="secondary" class="bg-cyan-950/50 text-cyan-400 border border-cyan-800/60">
                  LB: {{ rule.balancerTag }}
                </Badge>
                <Badge v-else :variant="getOutboundBadgeVariant(rule.outboundTag || '')">
                  {{ rule.outboundTag }}
                </Badge>
                <span v-if="rule.tag" class="px-1.5 py-0.5 rounded bg-neutral-800 text-neutral-300 border border-neutral-700 text-[10px]">
                  {{ rule.tag }}
                </span>
                <span v-if="!rule.balancerTag && rule.outboundTag && !isKnownOutbound(rule.outboundTag)" class="text-amber-400 text-[10px] font-semibold" title="系统中未配置该出站标签">
                  ⚠️ 未知出站
                </span>
              </div>
            </td>

            <!-- 3. Source Inbounds -->
            <td class="p-3 align-middle font-mono text-[11px]">
              <span v-if="rule.inboundTag?.length" class="text-foreground">
                {{ rule.inboundTag.join(', ') }}
              </span>
              <span v-else class="text-muted-foreground">全部入站</span>
            </td>

            <!-- 4. Matching Conditions -->
            <td class="p-3 align-middle">
              <div class="flex flex-wrap items-center gap-1.5 font-mono text-[11px]">
                <span v-if="rule.domainMatcher" class="px-1.5 py-0.5 rounded bg-neutral-800 text-cyan-300 border border-neutral-700">
                  matcher: {{ rule.domainMatcher }}
                </span>
                <span v-if="rule.vlessRoute" class="px-1.5 py-0.5 rounded bg-indigo-950/60 text-indigo-300 border border-indigo-800/50">
                  vlessRoute: {{ rule.vlessRoute }}
                </span>
                <span v-if="rule.user?.length" class="px-1.5 py-0.5 rounded bg-emerald-950/60 text-emerald-300 border border-emerald-800/50">
                  用户: {{ rule.user.join(', ') }}
                </span>
                <span v-if="rule.domain?.length" class="px-1.5 py-0.5 rounded bg-blue-950/60 text-blue-300 border border-blue-800/50">
                  域名: {{ rule.domain.join(', ') }}
                </span>
                <span v-if="rule.ip?.length" class="px-1.5 py-0.5 rounded bg-purple-950/60 text-purple-300 border border-purple-800/50">
                  IP: {{ rule.ip.join(', ') }}
                </span>
                <span v-if="rule.source?.length" class="px-1.5 py-0.5 rounded bg-teal-950/60 text-teal-300 border border-teal-800/50">
                  源IP: {{ rule.source.join(', ') }}
                </span>
                <span v-if="rule.sourcePort" class="px-1.5 py-0.5 rounded bg-teal-950/60 text-teal-300 border border-teal-800/50">
                  源端口: {{ rule.sourcePort }}
                </span>
                <span v-if="rule.protocol?.length" class="px-1.5 py-0.5 rounded bg-amber-950/60 text-amber-300 border border-amber-800/50">
                  协议: {{ rule.protocol.join(', ') }}
                </span>
                <span v-if="rule.port" class="px-1.5 py-0.5 rounded bg-rose-950/60 text-rose-300 border border-rose-800/50">
                  端口: {{ rule.port }}
                </span>
                <span v-if="rule.network" class="px-1.5 py-0.5 rounded bg-neutral-900 text-neutral-300 border border-border">
                  网络: {{ rule.network }}
                </span>
                <span v-if="!rule.domainMatcher && !rule.vlessRoute && !rule.user?.length && !rule.domain?.length && !rule.ip?.length && !rule.source?.length && !rule.sourcePort && !rule.protocol?.length && !rule.port && !rule.network" class="text-muted-foreground text-xs font-sans">
                  无额外限制 (匹配任意流量)
                </span>
              </div>
            </td>

            <!-- 5. Actions -->
            <td class="p-3 align-middle text-right whitespace-nowrap">
              <div class="flex items-center justify-end gap-1.5 shrink-0">
                <button
                  type="button"
                  class="h-8 w-8 inline-flex items-center justify-center rounded-md bg-neutral-800 border border-neutral-600/80 text-neutral-100 hover:bg-neutral-700 hover:text-white disabled:opacity-25 disabled:cursor-not-allowed transition-colors shrink-0 shadow-sm"
                  :disabled="Number(idx) === 0"
                  @click="moveRule(Number(idx), -1)"
                  title="上移规则"
                >
                  <ArrowUp class="w-4 h-4" />
                </button>
                <button
                  type="button"
                  class="h-8 w-8 inline-flex items-center justify-center rounded-md bg-neutral-800 border border-neutral-600/80 text-neutral-100 hover:bg-neutral-700 hover:text-white disabled:opacity-25 disabled:cursor-not-allowed transition-colors shrink-0 shadow-sm"
                  :disabled="Number(idx) === (routingConfig.rules?.length || 1) - 1"
                  @click="moveRule(Number(idx), 1)"
                  title="下移规则"
                >
                  <ArrowDown class="w-4 h-4" />
                </button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-8 w-8 p-0 text-brand-400 hover:text-brand-300"
                  @click="editRule(Number(idx))"
                  title="编辑规则"
                >
                  <Edit3 class="w-3.5 h-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-8 w-8 p-0 text-rose-400 hover:text-rose-300 hover:bg-rose-950/40"
                  @click="deleteRule(Number(idx))"
                  title="删除规则"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </Button>
              </div>
            </td>
          </tr>

          <!-- Empty State -->
          <tr v-if="!routingConfig.rules?.length">
            <td colspan="5" class="p-8 text-center text-muted-foreground text-xs">
              暂无路由分流规则，点击上方「添加规则」或使用智能预设
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Mobile Card Stack List (< 640px) -->
    <div class="sm:hidden space-y-2.5">
      <div
        v-for="(rule, idx) in routingConfig.rules"
        :key="idx"
        :draggable="true"
        @dragstart="onDragStart($event, Number(idx))"
        @dragover.prevent="onDragOver($event, Number(idx))"
        @drop.prevent="onDrop(Number(idx))"
        @dragend="onDragEnd"
        class="p-3 rounded-lg border bg-neutral-950 transition-all space-y-2.5"
        :class="[
          draggedIndex === idx
            ? 'opacity-30 border-dashed border-indigo-500/60'
            : (dragOverIndex === idx && draggedIndex !== idx
                ? 'ring-2 ring-indigo-500/70 bg-indigo-500/10 border-indigo-500'
                : 'border-border hover:border-border/80')
        ]"
      >
        <!-- Card Top Bar: Handle + Index Badge + Outbound Tag + Actions -->
        <div class="flex items-center justify-between gap-2 pb-2 border-b border-border/50">
          <div class="flex items-center gap-1.5 min-w-0">
            <span
              class="cursor-grab active:cursor-grabbing p-1 text-muted-foreground hover:text-foreground shrink-0 touch-none"
              title="按住拖拽排序"
            >
              <GripVertical class="w-4 h-4" />
            </span>
            <button
              type="button"
              @click="openQuickReorder(Number(idx))"
              class="px-1.5 py-0.5 rounded font-mono text-[11px] font-semibold bg-neutral-900 border border-border text-foreground hover:border-indigo-500/60 hover:text-indigo-400 shrink-0"
              title="点击快速修改位次"
            >
              #{{ Number(idx) + 1 }}
            </button>
            <div class="flex items-center gap-1.5 truncate">
              <Badge v-if="rule.balancerTag" variant="secondary" class="bg-cyan-950/50 text-cyan-400 border border-cyan-800/60">
                LB: {{ rule.balancerTag }}
              </Badge>
              <Badge v-else :variant="getOutboundBadgeVariant(rule.outboundTag || '')">
                {{ rule.outboundTag }}
              </Badge>
              <span
                v-if="rule.tag"
                class="px-1.5 py-0.5 rounded bg-neutral-800 text-neutral-300 border border-neutral-700 text-[10px] shrink-0"
              >
                {{ rule.tag }}
              </span>
              <span
                v-if="!rule.balancerTag && rule.outboundTag && !isKnownOutbound(rule.outboundTag)"
                class="text-amber-400 text-[10px] font-semibold shrink-0"
                title="系统中未配置该出站标签"
              >
                ⚠️ 未知
              </span>
            </div>
          </div>

          <!-- Actions -->
          <div class="flex items-center gap-1.5 shrink-0">
            <button
              type="button"
              class="h-8 w-8 inline-flex items-center justify-center rounded-md bg-neutral-800 border border-neutral-600/80 text-neutral-100 hover:bg-neutral-700 hover:text-white disabled:opacity-25 disabled:cursor-not-allowed transition-colors shrink-0 shadow-sm"
              :disabled="Number(idx) === 0"
              @click="moveRule(Number(idx), -1)"
              title="上移规则"
            >
              <ArrowUp class="w-4 h-4" />
            </button>
            <button
              type="button"
              class="h-8 w-8 inline-flex items-center justify-center rounded-md bg-neutral-800 border border-neutral-600/80 text-neutral-100 hover:bg-neutral-700 hover:text-white disabled:opacity-25 disabled:cursor-not-allowed transition-colors shrink-0 shadow-sm"
              :disabled="Number(idx) === (routingConfig.rules?.length || 1) - 1"
              @click="moveRule(Number(idx), 1)"
              title="下移规则"
            >
              <ArrowDown class="w-4 h-4" />
            </button>
            <Button
              variant="ghost"
              size="sm"
              class="h-8 w-8 p-0 text-brand-400 hover:text-brand-300"
              @click="editRule(Number(idx))"
              title="编辑规则"
            >
              <Edit3 class="w-3.5 h-3.5" />
            </Button>
            <Button
              variant="ghost"
              size="sm"
              class="h-8 w-8 p-0 text-rose-400 hover:text-rose-300 hover:bg-rose-950/40"
              @click="deleteRule(Number(idx))"
              title="删除规则"
            >
              <Trash2 class="w-3.5 h-3.5" />
            </Button>
          </div>
        </div>

        <!-- Card Body / Matching Criteria Matrix -->
        <div class="space-y-1.5 text-xs font-mono">
          <!-- Inbounds info if specified -->
          <div v-if="rule.inboundTag?.length" class="flex items-start gap-1.5 text-[11px]">
            <span class="text-muted-foreground shrink-0 font-sans">来源入站:</span>
            <span class="text-foreground break-all">{{ rule.inboundTag.join(', ') }}</span>
          </div>

          <!-- Criteria Badges with auto wrap & zero horizontal overflow -->
          <div class="flex flex-wrap items-center gap-1.5 text-[11px]">
            <span
              v-if="rule.domainMatcher"
              class="px-1.5 py-0.5 rounded bg-neutral-800 text-cyan-300 border border-neutral-700 break-all"
            >
              matcher: {{ rule.domainMatcher }}
            </span>
            <span
              v-if="rule.vlessRoute"
              class="px-1.5 py-0.5 rounded bg-indigo-950/60 text-indigo-300 border border-indigo-800/50 break-all"
            >
              vlessRoute: {{ rule.vlessRoute }}
            </span>
            <span
              v-if="rule.user?.length"
              class="px-1.5 py-0.5 rounded bg-emerald-950/60 text-emerald-300 border border-emerald-800/50 break-all"
            >
              用户: {{ rule.user.join(', ') }}
            </span>
            <span
              v-if="rule.domain?.length"
              class="px-1.5 py-0.5 rounded bg-blue-950/60 text-blue-300 border border-blue-800/50 break-all"
            >
              域名: {{ rule.domain.join(', ') }}
            </span>
            <span
              v-if="rule.ip?.length"
              class="px-1.5 py-0.5 rounded bg-purple-950/60 text-purple-300 border border-purple-800/50 break-all"
            >
              IP: {{ rule.ip.join(', ') }}
            </span>
            <span
              v-if="rule.source?.length"
              class="px-1.5 py-0.5 rounded bg-teal-950/60 text-teal-300 border border-teal-800/50 break-all"
            >
              源IP: {{ rule.source.join(', ') }}
            </span>
            <span
              v-if="rule.sourcePort"
              class="px-1.5 py-0.5 rounded bg-teal-950/60 text-teal-300 border border-teal-800/50"
            >
              源端口: {{ rule.sourcePort }}
            </span>
            <span
              v-if="rule.protocol?.length"
              class="px-1.5 py-0.5 rounded bg-amber-950/60 text-amber-300 border border-amber-800/50 break-all"
            >
              协议: {{ rule.protocol.join(', ') }}
            </span>
            <span
              v-if="rule.port"
              class="px-1.5 py-0.5 rounded bg-rose-950/60 text-rose-300 border border-rose-800/50"
            >
              端口: {{ rule.port }}
            </span>
            <span
              v-if="rule.network"
              class="px-1.5 py-0.5 rounded bg-neutral-900 text-neutral-300 border border-border"
            >
              网络: {{ rule.network }}
            </span>
            <span
              v-if="!rule.domainMatcher && !rule.vlessRoute && !rule.user?.length && !rule.domain?.length && !rule.ip?.length && !rule.source?.length && !rule.sourcePort && !rule.protocol?.length && !rule.port && !rule.network"
              class="text-muted-foreground text-[11px] font-sans"
            >
              无额外限制 (匹配任意流量)
            </span>
          </div>
        </div>
      </div>

      <!-- Mobile Empty State -->
      <div
        v-if="!routingConfig.rules?.length"
        class="p-8 text-center text-muted-foreground text-xs rounded-lg border border-border bg-neutral-950"
      >
        暂无路由分流规则，点击上方「添加规则」或使用智能预设
      </div>
    </div>

    <!-- Rule Edit Drawer (Replacing old centered modal) -->
    <Drawer
      v-model="showRuleDrawer"
      :title="isEditingRule ? `编辑分流规则 #${editingIndex + 1}` : '添加分流规则'"
      description="从已有的入站与出站标签中动态绑定，配置即时暂存"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <form id="rule-form" @submit.prevent="saveRuleInDrawer" class="space-y-4 text-xs">
        <div>
          <label class="block text-foreground mb-1 font-medium">规则标识 / 备注 (Tag，可选)</label>
          <input
            v-model="ruleForm.tag"
            type="text"
            placeholder="例如: block-ads, proxy-google, route-vip"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-0.5">为该规则指定便于识别的标识或备注信息</p>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <!-- 路由目标：出站标签或负载均衡组 -->
          <div>
            <div class="flex items-center justify-between mb-1">
              <label class="text-foreground font-medium">路由目标 (Target)</label>
              <div class="flex items-center gap-2 text-[11px] font-mono">
                <label class="inline-flex items-center gap-1 cursor-pointer">
                  <input type="radio" v-model="ruleForm.targetType" value="outbound" class="text-cyan-500" />
                  <span>出站</span>
                </label>
                <label class="inline-flex items-center gap-1 cursor-pointer">
                  <input type="radio" v-model="ruleForm.targetType" value="balancer" class="text-cyan-500" />
                  <span>负载均衡组</span>
                </label>
              </div>
            </div>

            <div v-if="ruleForm.targetType === 'outbound'" class="space-y-1.5">
              <select
                v-model="ruleForm.outboundTag"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              >
                <option v-for="tag in availableOutboundTags" :key="tag" :value="tag">
                  {{ tag }}
                </option>
                <option value="__custom__">+ 手动输入自定义 Tag</option>
              </select>

              <input
                v-if="ruleForm.outboundTag === '__custom__' || !availableOutboundTags.includes(ruleForm.outboundTag)"
                v-model="ruleForm.customOutboundTag"
                type="text"
                placeholder="输入自定义 OutboundTag"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-1.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>

            <div v-else class="space-y-1">
              <input
                v-model="ruleForm.balancerTag"
                type="text"
                placeholder="例如: balancer-all, lb-us"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
              <p class="text-[10px] text-muted-foreground">将流量导向配置中定义的负载均衡组 (balancerTag)</p>
            </div>
          </div>

          <!-- 动态 InboundTag 匹配选择 -->
          <div>
            <label class="block text-foreground mb-1 font-medium">来源入站 (InboundTag 可选)</label>
            <select
              v-model="ruleForm.selectedInboundTag"
              @change="onSelectInboundTag"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">全部入站 (默认不限制)</option>
              <option v-for="tag in availableInboundTags" :key="tag" :value="tag">
                {{ tag }}
              </option>
            </select>
            <input
              v-model="ruleForm.inboundTagsStr"
              type="text"
              placeholder="或用逗号隔开多个入站Tag (如 api, vless-reality)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-1.5 mt-1.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>

        <!-- 域名匹配算法 (domainMatcher) -->
        <div>
          <label class="block text-foreground mb-1 font-medium">
            域名匹配算法 (domainMatcher，可选)
          </label>
          <select
            v-model="ruleForm.domainMatcher"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="">默认 (跟随核心全局设置)</option>
            <option value="hybrid">hybrid (混合双模匹配 - 推荐)</option>
            <option value="linear">linear (线性规则匹配)</option>
            <option v-if="ruleForm.domainMatcher && !['hybrid', 'linear'].includes(ruleForm.domainMatcher)" :value="ruleForm.domainMatcher">
              {{ ruleForm.domainMatcher }} (保留已有)
            </option>
          </select>
          <p class="text-[10px] text-muted-foreground mt-0.5">Xray 域名匹配加速算法，留空使用全局设置</p>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">
            VLESS 协议路由匹配 (vlessRoute，可选)
          </label>
          <input
            v-model="ruleForm.vlessRoute"
            type="text"
            placeholder="例如: 1 或 2 或 1,2 或 100-200 (匹配客户端 VLESS UUID 中携带的 16 位路由编号)"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-0.5">Xray 官方 VLESS 协议级多节点路由分流条件，支持单值、列表或范围匹配</p>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">
            用户匹配列表 (User，可选，多个用逗号或换行隔开)
          </label>
          <input
            v-model="ruleForm.userStr"
            type="text"
            placeholder="user1@example.com, user2@example.com (匹配指定用户的 Email 分流)"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
          />
          <p class="text-[10px] text-muted-foreground mt-0.5">按用户 Email 进行分流（支持多用户分流至指定出口）</p>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">域名匹配列表 (Domain，多个用逗号或换行隔开)</label>
          <textarea
            v-model="ruleForm.domainStr"
            rows="3"
            placeholder="geosite:category-ads-all, geosite:cn, domain:google.com"
            class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring leading-relaxed"
          ></textarea>
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">IP 匹配列表 (IP，多个用逗号或换行隔开)</label>
          <textarea
            v-model="ruleForm.ipStr"
            rows="3"
            placeholder="geoip:cn, geoip:private, 192.168.0.0/16"
            class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring leading-relaxed"
          ></textarea>
        </div>

        <!-- 客户端源 IP (source) 与 源端口 (sourcePort) -->
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <div>
            <label class="block text-foreground mb-1 font-medium">客户端源 IP / CIDR (Source，可选)</label>
            <textarea
              v-model="ruleForm.sourceStr"
              rows="2"
              placeholder="例如: 192.168.1.0/24, 10.0.0.1, geoip:private"
              class="w-full bg-neutral-950 border border-border rounded-md p-2.5 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring leading-relaxed"
            ></textarea>
            <p class="text-[10px] text-muted-foreground mt-0.5">根据客户端来源 IP/CIDR 分流，多个用逗号或换行隔开</p>
          </div>

          <div>
            <label class="block text-foreground mb-1 font-medium">客户端源端口 (SourcePort，可选)</label>
            <input
              v-model="ruleForm.sourcePort"
              type="text"
              placeholder="例如: 1024-65535 或 50000"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
            <p class="text-[10px] text-muted-foreground mt-0.5">客户端源端口范围或单端口</p>
          </div>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <div>
            <label class="block text-foreground mb-1 font-medium">端口 (Port)</label>
            <input
              v-model="ruleForm.port"
              type="text"
              placeholder="25 或 80,443 或 1-1024"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          <div>
            <label class="block text-foreground mb-1 font-medium">传输层协议 (Network)</label>
            <select
              v-model="ruleForm.network"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option value="">全部 (TCP + UDP)</option>
              <option value="tcp">仅 TCP</option>
              <option value="udp">仅 UDP</option>
            </select>
          </div>

          <div>
            <label class="block text-foreground mb-1 font-medium">应用协议 (Protocol)</label>
            <input
              v-model="ruleForm.protocolStr"
              type="text"
              placeholder="bittorrent, http, tls"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>
        </div>
      </form>

      <template #footer>
        <Button variant="secondary" size="sm" @click="showRuleDrawer = false">
          取消
        </Button>
        <Button
          type="submit"
          form="rule-form"
          variant="default"
          size="sm"
        >
          <span>确认并暂存</span>
        </Button>
      </template>
    </Drawer>

    <!-- Smart Preset Wizard Modal -->
    <div v-if="showPresetModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75">
      <div class="w-full max-w-md p-5 rounded-lg border border-border bg-card shadow-2xl space-y-4">
        <div class="flex items-center justify-between pb-2 border-b border-border">
          <div>
            <h2 class="text-sm font-bold text-foreground">{{ presetData.title }}</h2>
            <p class="text-xs text-muted-foreground mt-0.5">{{ presetData.description }}</p>
          </div>
          <button @click="showPresetModal = false" class="text-muted-foreground hover:text-foreground text-base">✕</button>
        </div>

        <div class="space-y-3 text-xs">
          <div>
            <label class="block text-foreground mb-1 font-medium">选择目标出站 (OutboundTag)</label>
            <select
              v-model="presetData.selectedOutbound"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option v-for="tag in availableOutboundTags" :key="tag" :value="tag">
                {{ tag }}
              </option>
            </select>
          </div>

          <div v-if="!availableOutboundTags.includes(presetData.selectedOutbound)" class="p-2.5 rounded-md bg-amber-950/40 border border-amber-800/50 text-amber-300 text-[11px]">
            ⚠️ 当前系统中未检测到 <code>{{ presetData.selectedOutbound }}</code> 出站，请选择现有出站或先在「出站代理」页面创建。
          </div>

          <div class="p-3 rounded-md bg-neutral-950 border border-border space-y-1 font-mono text-[11px] text-muted-foreground">
            <span class="block text-foreground font-bold">即将注入的匹配条件:</span>
            <div v-for="(rule, i) in presetData.rules" :key="i">
              - {{ formatRuleSummary(rule) }}
            </div>
          </div>
        </div>

        <div class="flex justify-end gap-2.5 pt-2 border-t border-border">
          <Button
            variant="secondary"
            size="sm"
            @click="showPresetModal = false"
          >
            取消
          </Button>
          <Button
            variant="default"
            size="sm"
            @click="confirmInjectPreset"
          >
            确认注入规则
          </Button>
        </div>
      </div>
    </div>

    <!-- Quick Reorder Modal (Direct Jump) -->
    <Modal
      v-model="showReorderModal"
      title="调整规则优先级位次"
      description="长列表快速直达，无需重复长距离拖拽"
      size="sm"
    >
      <form @submit.prevent="confirmQuickReorder" class="space-y-4 text-xs">
        <div class="space-y-3">
          <div class="p-3 rounded-md bg-neutral-900 border border-border flex items-center justify-between font-mono">
            <span class="text-muted-foreground">当前规则与次序:</span>
            <div class="flex items-center gap-1.5">
              <span class="text-foreground font-bold">#{{ reorderSourceIndex + 1 }}</span>
              <Badge v-if="currentReorderRule" :variant="getOutboundBadgeVariant(currentReorderRule.outboundTag)">
                {{ currentReorderRule.outboundTag }}
              </Badge>
            </div>
          </div>

          <div class="space-y-1.5">
            <label class="block text-foreground font-medium">
              目标新位次 (范围: 1 ~ {{ routingConfig.rules?.length || 1 }})
            </label>
            <input
              ref="reorderInputRef"
              v-model.number="reorderTargetPosition"
              type="number"
              :min="1"
              :max="routingConfig.rules?.length || 1"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-sm focus:outline-none focus:ring-1 focus:ring-ring"
              placeholder="输入目标序号，如 1"
              required
            />
            <p class="text-[11px] text-muted-foreground leading-relaxed">
              确认后该规则将直接移动至目标位次，其它规则将依次平移顺延。
            </p>
          </div>
        </div>

        <div class="flex justify-end gap-2.5 pt-2 border-t border-border">
          <Button
            variant="secondary"
            size="sm"
            type="button"
            @click="showReorderModal = false"
          >
            取消
          </Button>
          <Button
            variant="default"
            size="sm"
            type="submit"
          >
            确认调整
          </Button>
        </div>
      </form>
    </Modal>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, computed } from 'vue'
import { Plus, Check, Edit3, Trash2, Database, RotateCw, GripVertical, ArrowUp, ArrowDown } from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import Drawer from '../components/ui/Drawer.vue'
import Modal from '../components/ui/Modal.vue'
import { toast } from '../utils/toast'
import api from '../api'

export interface RoutingRule {
  tag?: string
  type?: string
  inboundTag?: string[]
  outboundTag?: string
  balancerTag?: string
  domainMatcher?: string
  vlessRoute?: string
  domain?: string[]
  ip?: string[]
  port?: string
  network?: string
  protocol?: string[]
  user?: string[]
  attrs?: string
  source?: string[]
  sourcePort?: string
}

export interface RoutingConfig {
  domainStrategy: string
  rules: RoutingRule[]
}

export interface RuleFormData {
  tag: string
  targetType: 'outbound' | 'balancer'
  outboundTag: string
  customOutboundTag: string
  balancerTag: string
  domainMatcher: string
  selectedInboundTag: string
  inboundTagsStr: string
  vlessRoute: string
  userStr: string
  domainStr: string
  ipStr: string
  sourceStr: string
  sourcePort: string
  port: string
  network: string
  protocolStr: string
}

const routingConfig = ref<RoutingConfig>({
  domainStrategy: 'IPIfNonMatch',
  rules: [],
})

const inboundsList = ref<any[]>([])
const outboundsList = ref<any[]>([])
const geodataStatus = ref<any>(null)
const updatingGeo = ref(false)

const showRuleDrawer = ref(false)
const isEditingRule = ref(false)
const editingIndex = ref(-1)
const saving = ref(false)

const fetchGeoStatus = async () => {
  try {
    const res: any = await api.get('/geodata/status')
    geodataStatus.value = res
  } catch (err) {
    console.error(err)
  }
}

const geoProgress = ref<any>({
  isUpdating: false,
  percentage: 0,
  message: '',
  speedBps: 0,
})
let progressTimer: any = null

const pollGeoProgress = async () => {
  try {
    const res: any = await api.get('/geodata/progress')
    if (res) {
      geoProgress.value = res
      if (!res.isUpdating) {
        if (progressTimer) {
          clearInterval(progressTimer)
          progressTimer = null
        }
        updatingGeo.value = false
        if (res.step === 'done') {
          toast.success(res.message || 'GeoData 规则库更新成功！')
          await fetchGeoStatus()
          setTimeout(() => {
            geoProgress.value.percentage = 0
          }, 4000)
        } else if (res.step === 'error') {
          toast.error(res.message || '更新规则库失败')
        }
      }
    }
  } catch (e) {
    console.error('poll progress failed', e)
  }
}

const updateGeoData = async () => {
  if (!confirm('确定在线更新 GeoIP / GeoSite 规则库吗？更新完成后将自动重载 Xray 核心。')) return
  updatingGeo.value = true
  geoProgress.value = { isUpdating: true, percentage: 5, message: '正在启动后台下载引擎...' }
  try {
    await api.post('/geodata/update')
    if (progressTimer) clearInterval(progressTimer)
    progressTimer = setInterval(pollGeoProgress, 500)
  } catch (err: any) {
    updatingGeo.value = false
    toast.error('触发更新失败: ' + err)
  }
}

// Preset Wizard State
const showPresetModal = ref(false)
const presetData = ref<any>({
  type: '',
  title: '',
  description: '',
  selectedOutbound: 'block',
  rules: [],
})

const ruleForm = ref<RuleFormData>({
  tag: '',
  targetType: 'outbound',
  outboundTag: 'direct',
  customOutboundTag: '',
  balancerTag: '',
  domainMatcher: '',
  selectedInboundTag: '',
  inboundTagsStr: '',
  vlessRoute: '',
  userStr: '',
  domainStr: '',
  ipStr: '',
  sourceStr: '',
  sourcePort: '',
  port: '',
  network: '',
  protocolStr: '',
})

const availableOutboundTags = computed(() => {
  const tags = outboundsList.value.map((o) => o.tag).filter((t) => t)
  if (!tags.includes('direct')) tags.unshift('direct')
  if (!tags.includes('block')) tags.push('block')
  return tags
})

const availableInboundTags = computed(() => {
  return inboundsList.value.map((i) => i.tag).filter((t) => t)
})

const fetchAllDependencies = async () => {
  try {
    const [routeRes, inbRes, outRes]: any = await Promise.all([
      api.get('/routing'),
      api.get('/inbounds'),
      api.get('/outbounds'),
    ])
    routingConfig.value = routeRes || { domainStrategy: 'IPIfNonMatch', rules: [] }
    inboundsList.value = inbRes || []
    outboundsList.value = outRes || []
  } catch (err) {
    console.error(err)
  }
}

const isKnownOutbound = (tag: string) => {
  return availableOutboundTags.value.includes(tag) || tag === 'api'
}

const openAddRuleDrawer = () => {
  isEditingRule.value = false
  editingIndex.value = -1
  const defaultOutbound = availableOutboundTags.value.includes('block') ? 'block' : (availableOutboundTags.value[0] || 'direct')
  ruleForm.value = {
    tag: '',
    targetType: 'outbound',
    outboundTag: defaultOutbound,
    customOutboundTag: '',
    balancerTag: '',
    domainMatcher: '',
    selectedInboundTag: '',
    inboundTagsStr: '',
    vlessRoute: '',
    userStr: '',
    domainStr: '',
    ipStr: '',
    sourceStr: '',
    sourcePort: '',
    port: '',
    network: '',
    protocolStr: '',
  }
  showRuleDrawer.value = true
}

const onSelectInboundTag = () => {
  if (ruleForm.value.selectedInboundTag) {
    const current = parseArray(ruleForm.value.inboundTagsStr) || []
    if (!current.includes(ruleForm.value.selectedInboundTag)) {
      current.push(ruleForm.value.selectedInboundTag)
      ruleForm.value.inboundTagsStr = current.join(', ')
    }
  }
}

const editRule = (idx: number) => {
  isEditingRule.value = true
  editingIndex.value = idx
  const r = routingConfig.value.rules[idx]
  const isCustom = Boolean(r.outboundTag && !availableOutboundTags.value.includes(r.outboundTag))

  ruleForm.value = {
    tag: r.tag || '',
    targetType: r.balancerTag ? 'balancer' : 'outbound',
    outboundTag: isCustom ? '__custom__' : (r.outboundTag || availableOutboundTags.value[0] || 'direct'),
    customOutboundTag: isCustom ? (r.outboundTag || '') : '',
    balancerTag: r.balancerTag || '',
    domainMatcher: r.domainMatcher || '',
    selectedInboundTag: '',
    inboundTagsStr: (r.inboundTag || []).join(', '),
    vlessRoute: r.vlessRoute || '',
    userStr: (r.user || []).join(', '),
    domainStr: (r.domain || []).join(',\n'),
    ipStr: (r.ip || []).join(',\n'),
    sourceStr: (r.source || []).join(',\n'),
    sourcePort: r.sourcePort || '',
    port: r.port || '',
    network: r.network || '',
    protocolStr: (r.protocol || []).join(', '),
  }
  showRuleDrawer.value = true
}

const parseArray = (str: string) => {
  if (!str) return undefined
  const items = str
    .split(/[\n,]/)
    .map((s) => s.trim())
    .filter((s) => s)
  return items.length > 0 ? items : undefined
}

const saveRuleInDrawer = () => {
  const newRule: RoutingRule = {
    type: 'field',
  }

  // 1. 目标验证
  if (ruleForm.value.targetType === 'balancer') {
    const balancer = ruleForm.value.balancerTag.trim()
    if (!balancer) {
      toast.error('请填写负载均衡组标识 (BalancerTag)')
      return
    }
    newRule.balancerTag = balancer
  } else {
    let targetOutbound = ruleForm.value.outboundTag
    if (targetOutbound === '__custom__') {
      targetOutbound = ruleForm.value.customOutboundTag.trim()
    }
    if (!targetOutbound) {
      toast.error('请选择或输入目标出站 (OutboundTag)')
      return
    }
    newRule.outboundTag = targetOutbound
  }

  if (ruleForm.value.tag && ruleForm.value.tag.trim()) {
    newRule.tag = ruleForm.value.tag.trim()
  }

  if (ruleForm.value.domainMatcher && ruleForm.value.domainMatcher.trim()) {
    newRule.domainMatcher = ruleForm.value.domainMatcher.trim()
  }

  if (ruleForm.value.vlessRoute && ruleForm.value.vlessRoute.trim()) {
    newRule.vlessRoute = ruleForm.value.vlessRoute.trim()
  }

  const users = parseArray(ruleForm.value.userStr)
  if (users) newRule.user = users

  const inbounds = parseArray(ruleForm.value.inboundTagsStr)
  if (inbounds) newRule.inboundTag = inbounds

  const domains = parseArray(ruleForm.value.domainStr)
  if (domains) newRule.domain = domains

  const ips = parseArray(ruleForm.value.ipStr)
  if (ips) newRule.ip = ips

  const sources = parseArray(ruleForm.value.sourceStr)
  if (sources) newRule.source = sources

  if (ruleForm.value.sourcePort && ruleForm.value.sourcePort.trim()) {
    newRule.sourcePort = ruleForm.value.sourcePort.trim()
  }

  if (ruleForm.value.port && ruleForm.value.port.trim()) {
    newRule.port = ruleForm.value.port.trim()
  }

  if (ruleForm.value.network && ruleForm.value.network.trim()) {
    newRule.network = ruleForm.value.network.trim()
  }

  const protos = parseArray(ruleForm.value.protocolStr)
  if (protos) newRule.protocol = protos

  // 2. 匹配条件验证
  const hasMatchCondition =
    Boolean(newRule.domain && newRule.domain.length > 0) ||
    Boolean(newRule.ip && newRule.ip.length > 0) ||
    Boolean(newRule.port) ||
    Boolean(newRule.source && newRule.source.length > 0) ||
    Boolean(newRule.sourcePort) ||
    Boolean(newRule.network) ||
    Boolean(newRule.protocol && newRule.protocol.length > 0) ||
    Boolean(newRule.inboundTag && newRule.inboundTag.length > 0) ||
    Boolean(newRule.user && newRule.user.length > 0) ||
    Boolean(newRule.vlessRoute)

  if (!hasMatchCondition) {
    toast.error('请至少配置一项匹配条件 (域名/IP/源IP/源端口/目的端口/网络/协议/入站/用户/VLESS路由)')
    return
  }

  if (isEditingRule.value && editingIndex.value >= 0) {
    routingConfig.value.rules[editingIndex.value] = newRule
  } else {
    routingConfig.value.rules.push(newRule)
  }
  showRuleDrawer.value = false
}

const deleteRule = (idx: number) => {
  routingConfig.value.rules.splice(idx, 1)
}

const moveRule = (idx: number, delta: number) => {
  const rules = routingConfig.value.rules
  if (!rules) return
  const newIdx = idx + delta
  if (newIdx < 0 || newIdx >= rules.length) return
  const [moved] = rules.splice(idx, 1)
  rules.splice(newIdx, 0, moved)
}

// Drag-and-Drop Reorder Handlers
const draggedIndex = ref<number | null>(null)
const dragOverIndex = ref<number | null>(null)

const onDragStart = (e: DragEvent, idx: number) => {
  draggedIndex.value = idx
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(idx))
  }
}

const onDragOver = (e: DragEvent, idx: number) => {
  if (draggedIndex.value === null) return
  e.preventDefault()
  if (e.dataTransfer) {
    e.dataTransfer.dropEffect = 'move'
  }
  if (dragOverIndex.value !== idx) {
    dragOverIndex.value = idx
  }
}

const onDrop = (idx: number) => {
  if (draggedIndex.value === null) return
  const fromIdx = draggedIndex.value
  const toIdx = idx
  const rules = routingConfig.value.rules
  if (rules && fromIdx !== toIdx && fromIdx >= 0 && fromIdx < rules.length && toIdx >= 0 && toIdx < rules.length) {
    const [moved] = rules.splice(fromIdx, 1)
    rules.splice(toIdx, 0, moved)
  }
  draggedIndex.value = null
  dragOverIndex.value = null
}

const onDragEnd = () => {
  draggedIndex.value = null
  dragOverIndex.value = null
}

// Quick Reorder Modal (Direct Jump) Handlers
const showReorderModal = ref(false)
const reorderSourceIndex = ref<number>(-1)
const reorderTargetPosition = ref<number>(1)
const reorderInputRef = ref<HTMLInputElement | null>(null)

const currentReorderRule = computed(() => {
  if (reorderSourceIndex.value >= 0 && routingConfig.value.rules) {
    return routingConfig.value.rules[reorderSourceIndex.value]
  }
  return null
})

const openQuickReorder = (idx: number) => {
  reorderSourceIndex.value = idx
  reorderTargetPosition.value = idx + 1
  showReorderModal.value = true
  setTimeout(() => {
    reorderInputRef.value?.focus()
    reorderInputRef.value?.select()
  }, 100)
}

const confirmQuickReorder = () => {
  const rules = routingConfig.value.rules
  if (!rules || reorderSourceIndex.value < 0 || reorderSourceIndex.value >= rules.length) {
    showReorderModal.value = false
    return
  }
  const total = rules.length
  if (total <= 1) {
    showReorderModal.value = false
    return
  }
  const target = Math.max(1, Math.min(total, Math.floor(Number(reorderTargetPosition.value) || 1)))
  const targetIdx = target - 1
  if (targetIdx !== reorderSourceIndex.value) {
    const fromIdx = reorderSourceIndex.value
    const [item] = rules.splice(fromIdx, 1)
    rules.splice(targetIdx, 0, item)
    toast.success(`规则已从 #${fromIdx + 1} 调整至 #${target}`)
  }
  showReorderModal.value = false
}

// 智能预设向导 (Smart Preset Wizard)
const openPresetWizard = (type: string) => {
  let defOutbound = 'block'
  if (type === 'cn') {
    defOutbound = availableOutboundTags.value.includes('warp-out')
      ? 'warp-out'
      : (availableOutboundTags.value.includes('direct') ? 'direct' : availableOutboundTags.value[0] || 'direct')
  } else {
    defOutbound = availableOutboundTags.value.includes('block')
      ? 'block'
      : (availableOutboundTags.value[0] || 'block')
  }

  if (type === 'ads') {
    presetData.value = {
      type: 'ads',
      title: '🛡️ 广告拦截预设向导',
      description: '拦截常见广告联盟与追踪域名',
      selectedOutbound: defOutbound,
      rules: [{ domain: ['geosite:category-ads-all'] }],
    }
  } else if (type === 'private_ip') {
    presetData.value = {
      type: 'private_ip',
      title: '🔒 局域网私有 IP 屏蔽向导',
      description: '防止穿透访问服务器内网私有地址',
      selectedOutbound: defOutbound,
      rules: [{ ip: ['geoip:private'] }],
    }
  } else if (type === 'bt') {
    presetData.value = {
      type: 'bt',
      title: '🚫 BT/BitTorrent 拦截向导',
      description: '防止服务器遭遇版权版权投诉 (DMCA)',
      selectedOutbound: defOutbound,
      rules: [{ protocol: ['bittorrent'] }],
    }
  } else if (type === 'smtp') {
    presetData.value = {
      type: 'smtp',
      title: '📧 邮件 25 端口封锁向导',
      description: '防止滥用服务器发送垃圾邮件',
      selectedOutbound: defOutbound,
      rules: [{ port: '25', network: 'tcp' }],
    }
  } else if (type === 'cn') {
    presetData.value = {
      type: 'cn',
      title: '⚡ 国内流量分流向导',
      description: '将国内域名与 IP 路由至指定出站（如 WARP 或直连）',
      selectedOutbound: defOutbound,
      rules: [{ domain: ['geosite:cn'] }, { ip: ['geoip:cn'] }],
    }
  }

  showPresetModal.value = true
}

const confirmInjectPreset = () => {
  for (const r of presetData.value.rules) {
    routingConfig.value.rules.push({
      ...r,
      outboundTag: presetData.value.selectedOutbound,
    })
  }
  showPresetModal.value = false
}

const formatRuleSummary = (rule: any) => {
  if (rule.tag) return `[${rule.tag}]`
  if (rule.vlessRoute) return `vlessRoute: ${rule.vlessRoute}`
  if (rule.user?.length) return `用户: ${rule.user.join(', ')}`
  if (rule.domain?.length) return `域名: ${rule.domain.join(', ')}`
  if (rule.ip?.length) return `IP: ${rule.ip.join(', ')}`
  if (rule.source?.length) return `源IP: ${rule.source.join(', ')}`
  if (rule.sourcePort) return `源端口: ${rule.sourcePort}`
  if (rule.protocol?.length) return `协议: ${rule.protocol.join(', ')}`
  if (rule.port) return `端口: ${rule.port} (${rule.network || '全部'})`
  if (rule.balancerTag) return `Balancer: ${rule.balancerTag}`
  return JSON.stringify(rule)
}

const saveAllRouting = async () => {
  saving.value = true
  try {
    await api.post('/routing', routingConfig.value)
    toast.success('路由分流配置已保存并平滑生效！')
    await fetchAllDependencies()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const getOutboundBadgeVariant = (tag: string): 'default' | 'secondary' | 'outline' | 'destructive' => {
  switch (tag?.toLowerCase()) {
    case 'direct':
      return 'default'
    case 'block':
      return 'destructive'
    case 'warp-out':
      return 'secondary'
    default:
      return 'outline'
  }
}

onMounted(() => {
  fetchAllDependencies()
  fetchGeoStatus()
})
</script>
