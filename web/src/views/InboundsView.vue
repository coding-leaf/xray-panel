<template>
  <div class="space-y-4">
    <!-- Top Action & Filter Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">入站网关 (Inbounds)</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ filteredInbounds.length }} 个节点
          </Badge>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          管理 Xray 入站代理节点，支持 VLESS Reality 伪装巡检与单端口多出口分流
        </p>
      </div>

      <!-- Action Buttons -->
      <div class="flex items-center gap-2">
        <Button
          variant="secondary"
          size="sm"
          :loading="checkingReality"
          @click="triggerRealityCheck"
          title="巡检所有 Reality 伪装域名证书与可达性"
        >
          <ShieldCheck class="w-3.5 h-3.5 mr-1.5 text-emerald-400" />
          <span>{{ checkingReality ? '检测中...' : '检测 Reality' }}</span>
        </Button>
        <Button
          variant="default"
          size="sm"
          @click="openCreateDrawer"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加新入站</span>
        </Button>
      </div>
    </div>

    <!-- Filter Bar -->
    <div class="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-2.5">
      <div class="flex flex-1 items-center gap-2 max-w-md">
        <!-- Search Input -->
        <div class="relative w-full">
          <Search class="absolute left-2.5 top-2.5 w-3.5 h-3.5 text-muted-foreground" />
          <input
            v-model="searchQuery"
            type="text"
            placeholder="搜索节点 Tag / 端口 / 协议 / 域名..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <!-- Protocol Filter -->
        <select
          v-model="protocolFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部协议</option>
          <option value="vless">VLESS</option>
          <option value="vmess">VMess</option>
          <option value="trojan">Trojan</option>
          <option value="shadowsocks">Shadowsocks</option>
          <option value="socks">Socks</option>
          <option value="http">HTTP</option>
          <option value="dokodemo-door">dokodemo-door</option>
        </select>
      </div>

      <!-- Reality Alert Summary Banner if any warning/error -->
      <div v-if="realityAlertCount > 0" class="flex items-center gap-1.5 px-2.5 py-1 rounded-md bg-amber-500/10 border border-amber-500/20 text-amber-300 text-xs font-mono">
        <AlertTriangle class="w-3.5 h-3.5 text-amber-400 shrink-0" />
        <span>发现 {{ realityAlertCount }} 个 Reality 目标需关注</span>
      </div>
    </div>

    <!-- Main Table View -->
    <div class="relative w-full overflow-hidden rounded-lg border border-border bg-neutral-950">
      <table class="w-full caption-bottom text-xs border-collapse">
        <thead class="border-b border-border bg-neutral-900">
          <tr>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">节点标识 (Tag)</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">协议与流控</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">端口映射与网络</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">传输安全 / 伪装</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">用户与线路</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">端口状态</th>
            <th class="h-9 px-3.5 text-right align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr
            v-for="inb in filteredInbounds"
            :key="inb.id"
            @click="inspectInbound(inb)"
            class="border-b border-border/30 transition-colors hover:bg-muted/30 cursor-pointer"
            :class="{ 'bg-muted/20': selectedInbound?.id === inb.id }"
          >
            <!-- 1. Tag & Remarks -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-2">
                <span class="font-mono font-semibold text-foreground text-xs">{{ inb.tag }}</span>
                <span v-if="inb.routeId > 0 && (!inb.subRoutes || inb.subRoutes.length === 0)" class="text-[10px] font-mono px-1 py-0.2 rounded bg-neutral-800 text-neutral-300 border border-neutral-700">
                  #{{ inb.routeId }}
                </span>
              </div>
              <div class="text-[11px] text-muted-foreground mt-0.5 font-mono truncate max-w-[200px]">
                {{ inb.listen || '0.0.0.0' }}:{{ inb.port }}
              </div>
            </td>

            <!-- 2. Protocol & Flow -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-1.5">
                <Badge :variant="getProtocolBadgeVariant(inb.protocol)">
                  {{ inb.protocol?.toUpperCase() }}
                </Badge>
                <span v-if="getNodeFlow(inb) !== 'none'" class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-neutral-900 text-neutral-300 border border-neutral-800">
                  {{ getNodeFlow(inb) }}
                </span>
              </div>
            </td>

            <!-- 3. Port & Network -->
            <td class="p-3.5 align-middle font-mono">
              <div class="flex items-center gap-1.5 text-xs">
                <span class="text-foreground">:{{ inb.externalPort || inb.port }}</span>
                <span class="text-neutral-600">/</span>
                <span class="text-neutral-400 text-[11px] uppercase">{{ getStreamNetwork(inb) }}</span>
                <span v-if="(inb.externalPort || inb.port) !== 443 && isReality(inb)" class="text-amber-400 text-[10px]" title="非443端口Reality存在阻断风险">
                  ⚠️
                </span>
              </div>
              <div v-if="inb.externalHost" class="text-[10px] text-muted-foreground truncate max-w-[150px]">
                {{ inb.externalHost }}
              </div>
            </td>

            <!-- 4. Security & Reality -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-1.5">
                <Badge :variant="getSecurityBadgeVariant(getSecurityType(inb))">
                  {{ getSecurityType(inb)?.toUpperCase() }}
                </Badge>
                <!-- Reality Status Indicator -->
                <div v-if="isReality(inb) && getInboundRealityOverallStatus(inb.tag)">
                  <span
                    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-mono border"
                    :class="getRealityBadgeClass(getInboundRealityOverallStatus(inb.tag)!.status)"
                    :title="getInboundRealityOverallStatus(inb.tag)!.badgeText"
                  >
                    <span class="w-1 h-1 rounded-full" :class="getRealityDotClass(getInboundRealityOverallStatus(inb.tag)!.status)"></span>
                    <span>{{ getInboundRealityOverallStatus(inb.tag)!.status.toUpperCase() }}</span>
                  </span>
                </div>
              </div>
            </td>

            <!-- 5. Users & SubRoutes -->
            <td class="p-3.5 align-middle font-mono text-xs">
              <div class="flex items-center gap-2">
                <span class="text-foreground">{{ getClientCount(inb) }} 用户</span>
                <span v-if="inb.subRoutes?.length" class="text-[10px] px-1.5 py-0.5 rounded bg-muted/60 text-muted-foreground border border-border/40">
                  {{ inb.subRoutes.length }} 线路
                </span>
              </div>
            </td>

            <!-- 6. Alive & Latency -->
            <td class="p-3.5 align-middle font-mono text-xs">
              <div class="flex items-center gap-1.5">
                <span
                  class="w-1.5 h-1.5 rounded-full"
                  :class="inb.isAlive ? 'bg-emerald-400' : 'bg-rose-500'"
                />
                <span :class="inb.isAlive ? 'text-neutral-300' : 'text-rose-400'">
                  {{ inb.isAlive ? `${inb.latencyMs || 1}ms` : '未响应' }}
                </span>
              </div>
            </td>

            <!-- 7. Actions -->
            <td class="p-3.5 align-middle text-right" @click.stop>
              <div class="flex items-center justify-end gap-1.5">
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px]"
                  @click="inspectInbound(inb)"
                >
                  详情
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  class="h-7 px-2 text-[11px]"
                  @click="editInbound(inb)"
                >
                  编辑
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-[11px] text-rose-400 hover:text-rose-300 hover:bg-rose-500/10"
                  @click="deleteInbound(inb.id)"
                >
                  删除
                </Button>
              </div>
            </td>
          </tr>

          <!-- Empty State -->
          <tr v-if="!filteredInbounds.length">
            <td colspan="7" class="p-10 text-center text-muted-foreground">
              <div class="flex flex-col items-center justify-center gap-2">
                <Radio class="w-8 h-8 text-neutral-600 mb-1" />
                <p class="text-xs font-medium text-neutral-300">没有匹配的入站节点</p>
                <p class="text-[11px] text-muted-foreground">可尝试调整搜索条件，或点击右上角添加新节点</p>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Inspector Drawer (Right Sheet) -->
    <Drawer
      v-model="showInspectorDrawer"
      :title="`入站节点: ${selectedInbound?.tag || ''}`"
      :description="`协议 ${selectedInbound?.protocol?.toUpperCase()} / 端口 :${selectedInbound?.port}`"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <div v-if="selectedInbound" class="space-y-4 text-xs">
        <!-- 1. Node Overview Card -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">节点概览 (Overview)</span>
            <div class="flex items-center gap-1.5">
              <Badge :variant="selectedInbound.isAlive ? 'success' : 'destructive'" :dot="true">
                {{ selectedInbound.isAlive ? `运行正常 (${selectedInbound.latencyMs || 1}ms)` : '端口未响应' }}
              </Badge>
              <Badge :variant="getProtocolBadgeVariant(selectedInbound.protocol)">
                {{ selectedInbound.protocol?.toUpperCase() }}
              </Badge>
            </div>
          </div>

          <div class="grid grid-cols-2 sm:grid-cols-3 gap-3 font-mono">
            <div>
              <span class="block text-[10px] text-muted-foreground">Tag 标识</span>
              <span class="text-foreground font-semibold">{{ selectedInbound.tag }}</span>
            </div>
            <div>
              <span class="block text-[10px] text-muted-foreground">内部监听</span>
              <span class="text-neutral-300">{{ selectedInbound.listen || '0.0.0.0' }}:{{ selectedInbound.port }}</span>
            </div>
            <div>
              <span class="block text-[10px] text-muted-foreground">公网外部端口</span>
              <span class="text-foreground font-semibold">:{{ selectedInbound.externalPort || selectedInbound.port }}</span>
            </div>
            <div>
              <span class="block text-[10px] text-muted-foreground">传输协议 (Network)</span>
              <span class="text-neutral-300 uppercase">{{ getStreamNetwork(selectedInbound) }}</span>
            </div>
            <div>
              <span class="block text-[10px] text-muted-foreground">安全协议 (Security)</span>
              <span class="text-neutral-300 uppercase">{{ getSecurityType(selectedInbound) }}</span>
            </div>
            <div>
              <span class="block text-[10px] text-muted-foreground">节点流控 (Flow)</span>
              <span class="text-neutral-300">{{ getNodeFlow(selectedInbound) }}</span>
            </div>
          </div>

          <div v-if="selectedInbound.externalHost" class="pt-2 border-t border-border/40 font-mono text-[11px]">
            <span class="text-muted-foreground">自定义外部域名: </span>
            <span class="text-neutral-300">{{ selectedInbound.externalHost }}</span>
          </div>
        </div>

        <!-- 2. Reality / TLS Inspection Card -->
        <div v-if="isReality(selectedInbound)" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <div class="flex items-center gap-2">
              <Shield class="w-3.5 h-3.5 text-neutral-300" />
              <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">REALITY 伪装配置与合规状态</span>
            </div>
            <Button
              variant="outline"
              size="sm"
              class="h-6 px-2 text-[10px]"
              :loading="checkingReality"
              @click="triggerRealityCheck"
            >
              刷新检测
            </Button>
          </div>

          <!-- Reality Target & SNI -->
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-2 font-mono text-[11px]">
            <div class="p-2 rounded bg-neutral-900/80 border border-border/40">
              <span class="block text-[10px] text-muted-foreground">回落伪装目标 (Dest)</span>
              <span class="text-foreground">{{ getInboundRealityField(selectedInbound, 'dest') || 'www.example.com:443' }}</span>
            </div>
            <div class="p-2 rounded bg-neutral-900/80 border border-border/40">
              <span class="block text-[10px] text-muted-foreground">SNI 域名列表</span>
              <span class="text-foreground">{{ getInboundRealityField(selectedInbound, 'serverNames') || 'www.example.com' }}</span>
            </div>
          </div>

          <!-- Public Key & ShortIDs -->
          <div class="space-y-1.5 font-mono text-[11px]">
            <div class="p-2 rounded bg-neutral-900/80 border border-border/40 flex items-center justify-between">
              <div class="min-w-0 pr-2">
                <span class="block text-[10px] text-muted-foreground">Short ID</span>
                <span class="text-foreground truncate">{{ getInboundRealityField(selectedInbound, 'shortIds') || '0123456789abcdef' }}</span>
              </div>
              <Button
                variant="ghost"
                size="sm"
                class="h-6 px-2 text-[10px] shrink-0"
                @click="copyToClipboard(getInboundRealityField(selectedInbound, 'shortIds'))"
              >
                <Copy class="w-3 h-3 mr-1" />
                复制
              </Button>
            </div>
          </div>

          <!-- Reality Detailed Check Items -->
          <div v-if="getInboundRealityOverallStatus(selectedInbound.tag)" class="pt-2 border-t border-border/40 space-y-2">
            <span class="text-[10px] font-mono uppercase text-muted-foreground tracking-wider">巡检明细 (Inspection Results)</span>
            <div
              v-for="(item, idx) in getInboundRealityOverallStatus(selectedInbound.tag)!.items"
              :key="idx"
              class="p-2.5 rounded-md border text-[11px] font-mono space-y-1"
              :class="item.status === 'ok' ? 'bg-neutral-900/60 border-neutral-800' : item.status === 'warning' ? 'bg-amber-500/5 border-amber-500/20' : 'bg-rose-500/5 border-rose-500/20'"
            >
              <div class="flex items-center justify-between">
                <span class="text-foreground font-semibold">{{ item.serverName }}</span>
                <span
                  class="px-1.5 py-0.2 rounded text-[10px] font-semibold"
                  :class="item.status === 'ok' ? 'text-emerald-400 bg-emerald-500/10' : item.status === 'warning' ? 'text-amber-400 bg-amber-500/10' : 'text-rose-400 bg-rose-500/10'"
                >
                  {{ item.status.toUpperCase() }}
                </span>
              </div>
              <div class="flex items-center justify-between text-[10px] text-muted-foreground">
                <span>目标: {{ item.dest }}</span>
                <span v-if="item.daysLeft >= 0">证书剩 {{ item.daysLeft }} 天</span>
                <span>{{ item.tlsVersion || 'TLS' }} / {{ item.latencyMs || 0 }}ms</span>
              </div>
              <div v-if="item.details" class="text-[10px] text-neutral-400">
                {{ item.details }}
              </div>
            </div>
          </div>
          <div v-else class="text-[11px] text-muted-foreground font-mono">
            暂无巡检缓存，请点击“刷新检测”执行实时探测。
          </div>
        </div>

        <!-- 3. SubRoutes (单端口多出口) -->
        <div v-if="selectedInbound.subRoutes?.length" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">分流订阅线路 (Sub-Routes)</span>
            <span class="text-[10px] font-mono text-muted-foreground">共 {{ selectedInbound.subRoutes.length }} 条</span>
          </div>

          <div class="space-y-2">
            <div
              v-for="sr in selectedInbound.subRoutes"
              :key="sr.id || sr.routeId"
              class="p-2.5 rounded-md border border-border/60 bg-neutral-900/60 font-mono text-[11px] flex items-center justify-between"
            >
              <div class="flex items-center gap-2">
                <span class="px-1.5 py-0.5 rounded bg-neutral-800 text-neutral-300 font-bold">#{{ sr.routeId }}</span>
                <span class="text-foreground font-sans font-medium">{{ sr.name || ('线路 #' + sr.routeId) }}</span>
                <span class="text-neutral-500 text-[10px]">➔ {{ sr.outboundTag || 'direct' }}</span>
              </div>
              <div class="flex items-center gap-1.5">
                <Badge :variant="sr.enabled ? 'success' : 'secondary'">
                  {{ sr.enabled ? '已启用' : '已停用' }}
                </Badge>
                <Badge variant="outline" class="text-[10px]">
                  {{ sr.allowedUsers?.length ? `${sr.allowedUsers.length} 人授权` : '全员' }}
                </Badge>
              </div>
            </div>
          </div>
        </div>

        <!-- 4. Authorized Users -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(selectedInbound.protocol)" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between border-b border-border/60 pb-2">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">已关联授权用户</span>
            <span class="text-[10px] font-mono text-muted-foreground">{{ getClientCount(selectedInbound) }} 位用户</span>
          </div>

          <div class="flex flex-wrap gap-1.5 max-h-36 overflow-y-auto">
            <span
              v-for="email in getAssignedUserEmails(selectedInbound)"
              :key="email"
              class="px-2 py-1 rounded bg-neutral-900 border border-border/60 text-neutral-300 font-mono text-[11px] flex items-center gap-1.5"
            >
              <span class="w-1.5 h-1.5 rounded-full bg-neutral-500"></span>
              <span>{{ email }}</span>
            </span>
            <span v-if="!getAssignedUserEmails(selectedInbound).length" class="text-[11px] text-muted-foreground font-mono">
              暂无绑定用户
            </span>
          </div>
        </div>
      </div>

      <template #footer>
        <Button
          variant="outline"
          size="sm"
          @click="showInspectorDrawer = false"
        >
          关闭
        </Button>
        <Button
          variant="destructive"
          size="sm"
          @click="deleteInbound(selectedInbound.id)"
        >
          删除节点
        </Button>
        <Button
          variant="default"
          size="sm"
          @click="editInbound(selectedInbound)"
        >
          编辑配置
        </Button>
      </template>
    </Drawer>

    <!-- Create / Edit Inbound Drawer (Right Sheet) -->
    <Drawer
      v-model="showFormDrawer"
      :title="isEditing ? `编辑入站: ${form.tag}` : '添加新入站节点'"
      :description="isEditing ? '修改入站端口、传输层安全与关联授权用户' : '分层配置 Xray 协议、网络传输层与 Reality 密钥'"
      width="w-full sm:max-w-xl md:max-w-2xl lg:max-w-3xl"
    >
      <form id="inbound-form" @submit.prevent="saveInbound" class="space-y-4 text-xs">
        <!-- 1. 基础网络与端口设置 -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider flex items-center gap-1.5">
            <span>① 基础网络与端口设置</span>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">节点标识 (Tag)</label>
              <Input
                v-model="form.tag"
                type="text"
                required
                placeholder="vless-reality"
              />
            </div>

            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">监听 IP</label>
              <Input
                v-model="form.listen"
                type="text"
                placeholder="0.0.0.0"
              />
            </div>

            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">内部监听端口 (Port)</label>
              <Input
                v-model.number="form.port"
                type="number"
                required
                placeholder="443"
              />
            </div>

            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">外部公网端口 (External Port)</label>
              <Input
                v-model.number="form.externalPort"
                type="number"
                placeholder="443 (默认443)"
              />
              <p class="text-[10px] text-muted-foreground mt-0.5">前置 Nginx 反代或端口映射端口</p>
            </div>

            <div class="sm:col-span-2">
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">外部域名/IP (External Host)</label>
              <Input
                v-model="form.externalHost"
                type="text"
                placeholder="留空则继承全局节点公网域名"
              />
              <p class="text-[10px] text-muted-foreground mt-0.5">客户端订阅下发的主机地址</p>
            </div>
          </div>

          <!-- Non-443 Port Alert -->
          <div
            v-if="(form.externalPort || form.port) !== 443 && form.security === 'reality'"
            class="p-2.5 rounded-md bg-amber-500/10 border border-amber-500/20 text-amber-300 text-[11px] flex items-start gap-2"
          >
            <AlertTriangle class="w-3.5 h-3.5 text-amber-400 shrink-0 mt-0.5" />
            <span>当前配置了 Reality 伪装，但公网端口为 <b>{{ form.externalPort || form.port }}</b>（非 443）。监听非 443 端口可能存在 GFW 嗅探阻断风险，建议映射为 443 端口。</span>
          </div>
        </div>

        <!-- 2. 入站协议与传输层 -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider flex items-center gap-1.5">
            <span>② 协议与传输层 (定义节点级流控)</span>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">入站协议 (Protocol)</label>
              <select
                v-model="form.protocol"
                @change="onProtocolChange"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              >
                <option value="vless">VLESS (推荐)</option>
                <option value="vmess">VMess</option>
                <option value="trojan">Trojan</option>
                <option value="shadowsocks">Shadowsocks</option>
                <option value="socks">Socks</option>
                <option value="http">HTTP</option>
                <option value="dokodemo-door">dokodemo-door</option>
              </select>
            </div>

            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">传输协议 (Network)</label>
              <select
                v-model="form.network"
                @change="onNetworkChange"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              >
                <option value="tcp">TCP (RAW 推荐)</option>
                <option value="xhttp">XHTTP (SplitHTTP)</option>
                <option value="grpc">gRPC</option>
                <option value="ws">WebSocket</option>
                <option value="httpupgrade">HTTPUpgrade</option>
                <option value="mkcp">mKCP (UDP)</option>
              </select>
            </div>

            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">安全协议 (Security)</label>
              <select
                v-model="form.security"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              >
                <option v-if="isRealitySupported" value="reality">REALITY (推荐)</option>
                <option value="tls">TLS</option>
                <option value="none">None (无加密)</option>
              </select>
            </div>
          </div>

          <!-- Flow Policy -->
          <div v-if="form.protocol === 'vless'" class="pt-2 border-t border-border/40">
            <div class="flex items-center justify-between mb-1">
              <label class="text-muted-foreground font-mono text-[11px]">默认流控模式 (Flow Policy)</label>
              <span class="text-[10px] text-muted-foreground">分配给该节点的用户将继承此策略</span>
            </div>
            <select
              v-model="form.vlessFlow"
              :disabled="form.network !== 'tcp'"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-40"
            >
              <option v-if="form.network === 'tcp'" value="xtls-rprx-vision">xtls-rprx-vision (XTLS Vision 极速流控 - 推荐)</option>
              <option v-if="form.network === 'tcp'" value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option>
              <option value="">none (无流控 - 适用 XHTTP / gRPC / WS 等)</option>
            </select>
          </div>

          <!-- XHTTP options -->
          <div v-if="form.network === 'xhttp'" class="pt-2 border-t border-border/40 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">XHTTP 路径 (Path)</label>
              <Input v-model="form.xhttpPath" placeholder="/split" />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">XHTTP 模式</label>
              <select v-model="form.xhttpMode" class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring">
                <option value="auto">auto (自动)</option>
                <option value="stream-up">stream-up</option>
                <option value="packet-up">packet-up</option>
              </select>
            </div>
          </div>

          <!-- WS options -->
          <div v-if="form.network === 'ws'" class="pt-2 border-t border-border/40">
            <label class="block text-muted-foreground mb-1 font-mono text-[11px]">WebSocket 路径</label>
            <Input v-model="form.wsPath" placeholder="/ws" />
          </div>

          <!-- gRPC options -->
          <div v-if="form.network === 'grpc'" class="pt-2 border-t border-border/40">
            <label class="block text-muted-foreground mb-1 font-mono text-[11px]">gRPC 服务名</label>
            <Input v-model="form.grpcService" placeholder="xray-grpc" />
          </div>
        </div>

        <!-- 3. REALITY 伪装与安全配置 -->
        <div v-if="!['socks', 'http', 'dokodemo-door'].includes(form.protocol) && form.security === 'reality'" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">③ REALITY 伪装与密钥</span>
            <Button
              type="button"
              variant="outline"
              size="sm"
              class="h-7 text-[11px]"
              @click="generateRealityKey"
            >
              <Key class="w-3 h-3 mr-1 text-neutral-300" />
              <span>生成 x25519 密钥对</span>
            </Button>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">目标伪装网站 (Target)</label>
              <Input v-model="form.realityTarget" placeholder="www.example.com:443" />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">SNI 域名列表 (逗号隔开)</label>
              <Input v-model="form.realityServerNames" placeholder="www.example.com" />
            </div>
          </div>

          <div>
            <label class="block text-muted-foreground mb-1 font-mono text-[11px]">Private Key (私钥)</label>
            <Input v-model="form.realityPrivateKey" placeholder="base64 私钥" />
          </div>

          <div v-if="form.realityPublicKey" class="p-2.5 rounded bg-neutral-950 border border-border text-[11px] font-mono text-muted-foreground flex items-center justify-between">
            <span class="truncate pr-2"><b>Public Key:</b> <span class="text-foreground">{{ form.realityPublicKey }}</span></span>
            <Button type="button" variant="ghost" size="sm" class="h-6 px-2 text-[10px]" @click="copyToClipboard(form.realityPublicKey)">
              复制公钥
            </Button>
          </div>

          <div>
            <label class="block text-muted-foreground mb-1 font-mono text-[11px]">Short IDs (多个逗号隔开)</label>
            <Input v-model="form.realityShortIds" placeholder="0123456789abcdef" />
          </div>
        </div>

        <!-- TLS 配置 -->
        <div v-else-if="!['socks', 'http', 'dokodemo-door'].includes(form.protocol) && form.security === 'tls'" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider block">③ TLS 证书配置</span>
          <div>
            <label class="block text-muted-foreground mb-1 font-mono text-[11px]">SNI 域名 (ServerName)</label>
            <Input v-model="form.tlsServerName" placeholder="yourdomain.com" />
          </div>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">证书文件路径 (Cert File)</label>
              <Input v-model="form.tlsCertFile" placeholder="/etc/ssl/cert.pem" />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">私钥文件路径 (Key File)</label>
              <Input v-model="form.tlsKeyFile" placeholder="/etc/ssl/key.pem" />
            </div>
          </div>
        </div>

        <!-- 4. 回落与嗅探设置 -->
        <div class="rounded-lg border border-border bg-card p-4 space-y-3">
          <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider block">④ 回落伪装与流量探测 (Fallbacks & Sniffing)</span>
          <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pt-1">
            <label class="flex items-center gap-2 cursor-pointer text-muted-foreground hover:text-foreground">
              <input type="checkbox" v-model="form.fallbacksEnabled" class="rounded bg-neutral-950 border-border text-neutral-200" />
              <span>启用网站回落 (Fallbacks)</span>
            </label>
            <label class="flex items-center gap-2 cursor-pointer text-muted-foreground hover:text-foreground">
              <input type="checkbox" v-model="form.sniffingEnabled" class="rounded bg-neutral-950 border-border text-neutral-200" />
              <span>启用域名嗅探 (Sniffing)</span>
            </label>
          </div>

          <div v-if="form.fallbacksEnabled" class="pt-2 border-t border-border/40 grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">回落目标地址/端口</label>
              <Input v-model="form.fallbackDest" placeholder="80 或 127.0.0.1:80" />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1 font-mono text-[11px]">PROXY Protocol (xver)</label>
              <select v-model.number="form.fallbackXver" class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring">
                <option :value="0">0 (关闭)</option>
                <option :value="1">1 (v1)</option>
                <option :value="2">2 (v2)</option>
              </select>
            </div>
          </div>
        </div>

        <!-- 5. 分流订阅线路 (Sub-Routes) -->
        <div v-if="form.protocol === 'vless'" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between">
            <div>
              <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider block">⑤ 分流订阅线路 (Sub-Routes)</span>
              <p class="text-[10px] text-muted-foreground mt-0.5">所有线路共用该入站的单端口与 Reality 密钥，订阅导出多个节点</p>
            </div>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              class="h-7 text-[11px]"
              @click="addSubRoute"
            >
              <Plus class="w-3 h-3 mr-1" />
              <span>添加线路</span>
            </Button>
          </div>

          <div v-if="form.subRoutes?.length" class="space-y-2">
            <div
              v-for="(sr, index) in form.subRoutes"
              :key="sr.id || index"
              class="p-2.5 rounded-md border border-border/60 bg-neutral-950"
              :class="{ 'relative z-20': activeSubRoutePopoverIndex === index }"
            >
              <div class="flex flex-col sm:flex-row items-stretch sm:items-end gap-2">
                <!-- Route ID -->
                <div class="w-full sm:w-14 shrink-0">
                  <label class="block text-[10px] text-muted-foreground font-mono mb-1 truncate">ID</label>
                  <input
                    v-model.number="sr.routeId"
                    type="number"
                    min="1"
                    class="w-full bg-neutral-900 border border-border rounded px-2 h-8 text-foreground font-mono text-xs focus:outline-none focus:ring-1 focus:ring-ring text-center"
                  />
                </div>

                <!-- 线路名称 -->
                <div class="flex-1 min-w-[120px]">
                  <label class="block text-[10px] text-muted-foreground mb-1 truncate">线路名称</label>
                  <input
                    v-model="sr.name"
                    type="text"
                    placeholder="如 🇯🇵 日本原生直连"
                    class="w-full bg-neutral-900 border border-border rounded px-2.5 h-8 text-foreground text-xs focus:outline-none focus:ring-1 focus:ring-ring"
                  />
                </div>

                <!-- 目标出站 -->
                <div class="w-full sm:w-36 shrink-0">
                  <label class="block text-[10px] text-muted-foreground mb-1 truncate">目标出站</label>
                  <select
                    v-model="sr.outboundTag"
                    class="w-full bg-neutral-900 border border-border rounded px-2 h-8 text-foreground font-mono text-xs focus:outline-none focus:ring-1 focus:ring-ring"
                  >
                    <option v-for="tag in availableOutbounds" :key="tag" :value="tag">
                      {{ tag }}
                    </option>
                  </select>
                </div>

                <!-- 授权用户 Popover Trigger -->
                <div class="relative shrink-0">
                  <label class="block text-[10px] text-muted-foreground mb-1 truncate">授权用户</label>
                  <button
                    type="button"
                    @click.stop="toggleSubRoutePopover(index)"
                    class="h-8 px-2.5 rounded border text-xs font-mono flex items-center gap-1.5 transition-colors cursor-pointer select-none"
                    :class="[
                      isSubRouteAllOpen(sr)
                        ? 'border-border bg-neutral-900 text-muted-foreground hover:text-foreground hover:border-neutral-700'
                        : 'border-border bg-neutral-900 text-foreground font-medium hover:border-neutral-700',
                      activeSubRoutePopoverIndex === index ? 'ring-1 ring-ring border-neutral-600' : ''
                    ]"
                  >
                    <span
                      class="w-1.5 h-1.5 rounded-full shrink-0"
                      :class="isSubRouteAllOpen(sr) ? 'bg-neutral-500' : 'bg-emerald-400'"
                    />
                    <span class="truncate max-w-[110px]">
                      {{ isSubRouteAllOpen(sr) ? '全员开放' : `已选 ${sr.allowedUsers.length} 人` }}
                    </span>
                    <ChevronDown
                      class="w-3 h-3 text-muted-foreground transition-transform duration-150 shrink-0"
                      :class="{ 'rotate-180': activeSubRoutePopoverIndex === index }"
                    />
                  </button>

                  <!-- Floating Popover Menu -->
                  <div
                    v-if="activeSubRoutePopoverIndex === index"
                    @click.stop
                    class="absolute right-0 top-full mt-1.5 w-72 rounded-lg border border-border bg-neutral-950 shadow-2xl shadow-black/80 z-30 py-1 text-xs select-none"
                  >
                    <!-- Search input -->
                    <div class="p-2 border-b border-border/60">
                      <div class="relative">
                        <Search class="w-3.5 h-3.5 text-muted-foreground absolute left-2.5 top-1/2 -translate-y-1/2 pointer-events-none" />
                        <input
                          ref="userSearchInputRef"
                          v-model="subRouteUserSearch"
                          type="text"
                          placeholder="搜索用户..."
                          class="w-full bg-neutral-900 border border-border rounded px-2 pl-8 h-7 text-foreground text-xs placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring"
                          @keydown.enter.prevent.stop
                          @keydown.stop
                        />
                      </div>
                    </div>

                    <!-- Mode Radios: All vs Specific -->
                    <div class="p-1.5 space-y-0.5 border-b border-border/60">
                      <button
                        type="button"
                        @click="setSubRouteMode(sr, 'all')"
                        class="w-full flex items-center gap-2 px-2 py-1.5 rounded hover:bg-neutral-900 text-left transition-colors cursor-pointer group"
                      >
                        <span
                          class="w-3.5 h-3.5 rounded-full border flex items-center justify-center shrink-0 transition-colors"
                          :class="isSubRouteAllOpen(sr) ? 'border-foreground' : 'border-neutral-600 group-hover:border-neutral-500'"
                        >
                          <span v-if="isSubRouteAllOpen(sr)" class="w-1.5 h-1.5 rounded-full bg-foreground" />
                        </span>
                        <span
                          class="text-xs"
                          :class="isSubRouteAllOpen(sr) ? 'text-foreground font-medium' : 'text-muted-foreground group-hover:text-foreground'"
                        >
                          全员开放 (不限制具体用户)
                        </span>
                      </button>
                      <button
                        type="button"
                        @click="setSubRouteMode(sr, 'specific')"
                        class="w-full flex items-center gap-2 px-2 py-1.5 rounded hover:bg-neutral-900 text-left transition-colors cursor-pointer group"
                      >
                        <span
                          class="w-3.5 h-3.5 rounded-full border flex items-center justify-center shrink-0 transition-colors"
                          :class="!isSubRouteAllOpen(sr) ? 'border-foreground' : 'border-neutral-600 group-hover:border-neutral-500'"
                        >
                          <span v-if="!isSubRouteAllOpen(sr)" class="w-1.5 h-1.5 rounded-full bg-foreground" />
                        </span>
                        <span
                          class="text-xs"
                          :class="!isSubRouteAllOpen(sr) ? 'text-foreground font-medium' : 'text-muted-foreground group-hover:text-foreground'"
                        >
                          指定具体用户
                        </span>
                      </button>
                    </div>

                    <!-- User Checkbox List -->
                    <div class="max-h-48 overflow-y-auto p-1.5 space-y-0.5">
                      <div
                        v-for="u in filteredUsersForSubRoute"
                        :key="u.email"
                        @click="toggleSubRouteUser(sr, u.email)"
                        class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-neutral-900 cursor-pointer group transition-colors"
                      >
                        <div
                          class="w-3.5 h-3.5 rounded border flex items-center justify-center shrink-0 transition-colors"
                          :class="isSubRouteUserSelected(sr, u.email)
                            ? 'bg-foreground border-foreground text-background'
                            : 'border-neutral-600 bg-neutral-900 group-hover:border-neutral-500'"
                        >
                          <Check v-if="isSubRouteUserSelected(sr, u.email)" class="w-2.5 h-2.5 stroke-[3]" />
                        </div>
                        <span
                          class="truncate font-mono text-[11px]"
                          :class="isSubRouteUserSelected(sr, u.email) ? 'text-foreground font-medium' : 'text-muted-foreground group-hover:text-foreground'"
                        >
                          {{ u.email }}
                        </span>
                      </div>

                      <!-- Empty state for search -->
                      <div
                        v-if="usersList.length > 0 && filteredUsersForSubRoute.length === 0"
                        class="py-3 text-center text-muted-foreground font-mono text-[11px]"
                      >
                        无匹配用户
                      </div>

                      <!-- Empty state for no users in system -->
                      <div
                        v-if="usersList.length === 0"
                        class="py-3 text-center text-muted-foreground text-[11px]"
                      >
                        暂无用户，可在「用户与订阅」模块添加
                      </div>
                    </div>

                    <!-- Quick Actions Footer -->
                    <div
                      v-if="usersList.length > 0"
                      class="px-2.5 py-1.5 border-t border-border/60 flex items-center justify-between text-[10px] font-mono text-muted-foreground"
                    >
                      <button
                        type="button"
                        @click="selectAllSubRouteUsers(sr)"
                        class="hover:text-foreground transition-colors cursor-pointer"
                      >
                        全选
                      </button>
                      <button
                        type="button"
                        @click="clearSubRouteUsers(sr)"
                        class="hover:text-foreground transition-colors cursor-pointer"
                      >
                        清空 (恢复全员)
                      </button>
                    </div>
                  </div>
                </div>

                <!-- 启用 & 删除 操作 -->
                <div class="flex items-center gap-2 pt-1 sm:pt-0 shrink-0 h-8">
                  <label class="flex items-center gap-1 cursor-pointer text-muted-foreground hover:text-foreground text-[11px] select-none h-8 px-1">
                    <input
                      type="checkbox"
                      v-model="sr.enabled"
                      class="rounded bg-neutral-950 border-border text-neutral-200"
                    />
                    <span>启用</span>
                  </label>
                  <Button
                    type="button"
                    variant="ghost"
                    size="sm"
                    class="h-8 w-8 p-0 text-muted-foreground hover:text-rose-400 hover:bg-rose-500/10"
                    @click="removeSubRoute(Number(index))"
                    title="删除线路"
                  >
                    <Trash2 class="w-3.5 h-3.5" />
                  </Button>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="text-center py-3 text-muted-foreground font-mono text-[11px] border border-dashed border-border rounded">
            暂未配置分流线路（将仅作为单一常规节点导出）
          </div>
        </div>

        <!-- 6. 关联授权用户 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol)" class="rounded-lg border border-border bg-card p-4 space-y-3">
          <div class="flex items-center justify-between">
            <span class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider">⑥ 关联授权用户 (双向绑定)</span>
            <button
              type="button"
              @click="toggleSelectAllUsers"
              class="text-neutral-300 hover:text-white text-[11px] font-mono underline"
            >
              {{ isAllUsersSelected ? '取消全选' : '全选所有用户' }}
            </button>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-3 gap-2 max-h-48 overflow-y-auto p-2 bg-neutral-950 rounded-md border border-border">
            <label
              v-for="u in usersList"
              :key="u.email"
              class="flex items-center justify-between p-2 rounded hover:bg-neutral-900 cursor-pointer font-mono text-[11px] border border-border/40"
            >
              <div class="flex items-center gap-2 min-w-0 pr-1">
                <input
                  type="checkbox"
                  :value="u.email"
                  v-model="form.selectedUserEmails"
                  class="rounded bg-neutral-900 border-border text-neutral-200"
                />
                <span class="text-foreground truncate">{{ u.email }}</span>
              </div>
              <span class="text-muted-foreground text-[10px] shrink-0">{{ u.uuid?.substring(0, 6) }}...</span>
            </label>
            <div v-if="!usersList.length" class="col-span-full text-center py-3 text-muted-foreground text-[11px]">
              暂无用户，可在「用户与订阅」模块中创建
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <Button
          type="button"
          variant="outline"
          size="sm"
          @click="showFormDrawer = false"
        >
          取消
        </Button>
        <Button
          type="submit"
          form="inbound-form"
          variant="default"
          size="sm"
          :loading="saving"
        >
          <span>{{ saving ? '保存中...' : '保存节点并重载核心' }}</span>
        </Button>
      </template>
    </Drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, watch, nextTick } from 'vue'
import { useRoute } from 'vue-router'
import {
  Plus,
  Radio,
  Key,
  AlertTriangle,
  Trash2,
  ShieldCheck,
  Search,
  Copy,
  Shield,
  ChevronDown,
  Check,
} from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Input from '../components/ui/Input.vue'
import Badge from '../components/ui/Badge.vue'
import Drawer from '../components/ui/Drawer.vue'
import { toast } from '../utils/toast'
import api from '../api'
import { getRealityStatus, checkRealityStatus, type RealitySummaryStatus, type RealityCheckItem } from '../api/reality'

interface SubRouteItem {
  id: string
  name: string
  routeId: number
  outboundTag: string
  enabled: boolean
  allowedUsers?: string[]
}

const inbounds = ref<any[]>([])
const usersList = ref<any[]>([])
const availableOutbounds = ref<string[]>(['direct', 'block'])
const showInspectorDrawer = ref(false)
const showFormDrawer = ref(false)
const selectedInbound = ref<any>(null)
const isEditing = ref(false)
const saving = ref(false)
const checkingReality = ref(false)
const realitySummary = ref<RealitySummaryStatus | null>(null)
const searchQuery = ref('')
const protocolFilter = ref('all')
const route = useRoute()

const activeSubRoutePopoverIndex = ref<number | null>(null)
const subRouteUserSearch = ref('')
const userSearchInputRef = ref<HTMLInputElement | null>(null)

const filteredUsersForSubRoute = computed(() => {
  const query = subRouteUserSearch.value.trim().toLowerCase()
  if (!query) return usersList.value
  return usersList.value.filter((u: any) =>
    (u.email || '').toLowerCase().includes(query)
  )
})

const form = ref<any>({
  id: 0,
  tag: 'vless-reality',
  listen: '0.0.0.0',
  port: 4434,
  externalPort: 443,
  externalHost: '',
  routeId: 0,
  subRoutes: [] as SubRouteItem[],
  protocol: 'vless',
  vlessFlow: 'xtls-rprx-vision',
  network: 'tcp',
  security: 'reality',
  selectedUserEmails: [] as string[],
  xhttpPath: '/mbqyfa4grswh5ntz',
  xhttpMode: 'auto',
  wsPath: '/ws',
  grpcService: 'xray-grpc',
  realityTarget: 'www.example.com:443',
  realityServerNames: 'www.example.com',
  realityPrivateKey: '',
  realityPublicKey: '',
  realityShortIds: '0123456789abcdef',
  tlsServerName: '',
  tlsCertFile: '',
  tlsKeyFile: '',
  fallbacksEnabled: false,
  fallbackDest: '80',
  fallbackXver: 0,
  socksAuth: 'noauth',
  socksUdp: true,
  socksUsername: '',
  socksPassword: '',
  httpUsername: '',
  httpPassword: '',
  dokoAddress: '127.0.0.1',
  dokoPort: 53,
  dokoNetwork: 'tcp,udp',
  sniffingEnabled: true,
  sniffingRouteOnly: true,
})

const filteredInbounds = computed(() => {
  return inbounds.value.filter((ib) => {
    // Protocol filter
    if (protocolFilter.value !== 'all' && ib.protocol?.toLowerCase() !== protocolFilter.value.toLowerCase()) {
      return false
    }
    // Search query
    if (!searchQuery.value) return true
    const q = searchQuery.value.toLowerCase().trim()
    const tagMatch = ib.tag?.toLowerCase().includes(q)
    const portMatch = String(ib.port).includes(q) || String(ib.externalPort || '').includes(q)
    const protoMatch = ib.protocol?.toLowerCase().includes(q)
    const hostMatch = ib.externalHost?.toLowerCase().includes(q)
    return tagMatch || portMatch || protoMatch || hostMatch
  })
})

const realityAlertCount = computed(() => {
  return (realitySummary.value?.errorCount || 0) + (realitySummary.value?.warningCount || 0)
})

const isRealitySupported = computed(() => {
  return ['tcp', 'xhttp', 'grpc'].includes(form.value.network)
})

const isAllUsersSelected = computed(() => {
  if (!usersList.value.length) return false
  return form.value.selectedUserEmails.length === usersList.value.length
})

const toggleSelectAllUsers = () => {
  if (isAllUsersSelected.value) {
    form.value.selectedUserEmails = []
  } else {
    form.value.selectedUserEmails = usersList.value.map((u) => u.email)
  }
}

const onNetworkChange = () => {
  if (!isRealitySupported.value && form.value.security === 'reality') {
    form.value.security = 'tls'
  }
  if (form.value.network !== 'tcp') {
    form.value.vlessFlow = ''
  } else if (form.value.protocol === 'vless' && !form.value.vlessFlow) {
    form.value.vlessFlow = 'xtls-rprx-vision'
  }
}

const onProtocolChange = () => {
  if (form.value.protocol === 'vless' && form.value.network === 'tcp') {
    form.value.vlessFlow = 'xtls-rprx-vision'
  } else {
    form.value.vlessFlow = ''
  }
}

const copyToClipboard = async (text: string) => {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    toast.success('已复制到剪贴板')
  } catch {
    toast.error('复制失败，请手动选择复制')
  }
}

const inspectInbound = (inb: any) => {
  selectedInbound.value = inb
  showInspectorDrawer.value = true
}

const fetchRealityStatus = async () => {
  try {
    const rawRes: any = await getRealityStatus()
    realitySummary.value = (rawRes?.data && typeof rawRes.data === 'object' && rawRes.data.totalChecked !== undefined)
      ? rawRes.data
      : rawRes
  } catch (err) {
    console.error('Failed to fetch reality status:', err)
  }
}

const triggerRealityCheck = async () => {
  checkingReality.value = true
  try {
    const rawRes: any = await checkRealityStatus()
    const res: RealitySummaryStatus = (rawRes?.data && typeof rawRes.data === 'object' && rawRes.data.totalChecked !== undefined)
      ? rawRes.data
      : rawRes
    realitySummary.value = res
    const count = res?.totalChecked ?? res?.totalCount ?? (res?.items?.length || 0)
    const errCount = res?.errorCount ?? 0
    const warnCount = res?.warningCount ?? 0
    if (errCount > 0) {
      toast.warning(`检测完成：发现 ${errCount} 个 Reality 异常域名`)
    } else if (warnCount > 0) {
      toast.warning(`检测完成：发现 ${warnCount} 个 Reality 预警域名`)
    } else {
      toast.success(`Reality 检测完成：共巡检 ${count} 个目标，全部合规`)
    }
  } catch (err: any) {
    toast.error('检测失败: ' + (err?.message || err))
  } finally {
    checkingReality.value = false
  }
}

const getRealityItemsForInbound = (tag: string): RealityCheckItem[] => {
  return realitySummary.value?.items?.filter((item) => item.inboundTag === tag) || []
}

const getInboundRealityOverallStatus = (tag: string) => {
  const items = getRealityItemsForInbound(tag)
  if (!items.length) return null
  if (items.some((i) => i.status === 'error')) {
    const firstErr = items.find((i) => i.status === 'error')!
    return {
      status: 'error' as const,
      badgeText: `Reality 异常: ${firstErr.details || firstErr.errorType || '合规异常'}`,
      item: firstErr,
      items,
    }
  }
  if (items.some((i) => i.status === 'warning')) {
    const firstWarn = items.find((i) => i.status === 'warning')!
    return {
      status: 'warning' as const,
      badgeText: `Reality 预警: ${firstWarn.details || firstWarn.errorType || '临期或预警'}`,
      item: firstWarn,
      items,
    }
  }
  const firstOk = items[0]
  const tls = firstOk.tlsVersion || 'TLS 1.3'
  const alpn = firstOk.alpn ? ` / ${firstOk.alpn}` : ''
  return {
    status: 'ok' as const,
    badgeText: `Reality 正常 (${tls}${alpn})`,
    item: firstOk,
    items,
  }
}

const getInboundRealityField = (inb: any, field: string) => {
  try {
    const stream = JSON.parse(inb.streamSettingsJson || inb.streamSettings || '{}')
    if (stream.realitySettings) {
      if (field === 'serverNames' && Array.isArray(stream.realitySettings.serverNames)) {
        return stream.realitySettings.serverNames.join(', ')
      }
      if (field === 'shortIds' && Array.isArray(stream.realitySettings.shortIds)) {
        return stream.realitySettings.shortIds.join(', ')
      }
      return stream.realitySettings[field] || ''
    }
    return ''
  } catch {
    return ''
  }
}

const getAssignedUserEmails = (inb: any): string[] => {
  const result: string[] = []
  try {
    const s = JSON.parse(inb.settingsJson || '{}')
    if (Array.isArray(s.clients)) {
      for (const c of s.clients) {
        if (c.email) result.push(c.email)
      }
    }
  } catch {}
  if (Array.isArray(usersList.value)) {
    for (const u of usersList.value) {
      const tags = (u.inboundTags || u.inboundTag || '').split(',').map((s: string) => s.trim())
      if (tags.includes(inb.tag) && !result.includes(u.email)) {
        result.push(u.email)
      }
    }
  }
  return result
}

const getRealityBadgeClass = (status: 'ok' | 'warning' | 'error') => {
  switch (status) {
    case 'ok':
      return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
    case 'warning':
      return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'error':
      return 'bg-rose-500/10 text-rose-400 border-rose-500/20'
  }
}

const getRealityDotClass = (status: 'ok' | 'warning' | 'error') => {
  switch (status) {
    case 'ok':
      return 'bg-emerald-400'
    case 'warning':
      return 'bg-amber-400'
    case 'error':
      return 'bg-rose-400'
  }
}

const getProtocolBadgeVariant = (proto: string) => {
  switch (proto?.toLowerCase()) {
    case 'vless':
    case 'trojan':
    case 'vmess':
      return 'default'
    case 'shadowsocks':
    case 'socks':
      return 'secondary'
    case 'http':
    case 'dokodemo-door':
      return 'outline'
    default:
      return 'secondary'
  }
}

const getSecurityBadgeVariant = (sec: string) => {
  switch (sec?.toLowerCase()) {
    case 'reality':
      return 'default'
    case 'tls':
      return 'secondary'
    case 'none':
      return 'outline'
    default:
      return 'outline'
  }
}

const fetchAll = async () => {
  try {
    const [inbRes, uRes, obRes]: any = await Promise.all([
      api.get('/inbounds'),
      api.get('/users'),
      api.get('/outbounds').catch(() => []),
    ])
    inbounds.value = (inbRes || []).map((ib: any) => {
      let srs: any[] = []
      try {
        srs = JSON.parse(ib.subRoutesJson || '[]')
      } catch (e) {}
      return {
        ...ib,
        subRoutes: srs.map((r: any) => ({
          ...r,
          name: r.name || r.remark || '',
          allowedUsers: Array.isArray(r.allowedUsers) ? r.allowedUsers : [],
        })),
      }
    })
    usersList.value = uRes || []
    if (Array.isArray(obRes) && obRes.length > 0) {
      availableOutbounds.value = obRes.map((o: any) => o.tag).filter((t: string) => t)
    }
    if (!availableOutbounds.value.includes('direct')) {
      availableOutbounds.value.unshift('direct')
    }
    fetchRealityStatus()
  } catch (err) {
    console.error(err)
  }
}

const isSubRouteAllOpen = (sr: any): boolean => {
  return !sr.allowedUsers || sr.allowedUsers.length === 0
}

const toggleSubRoutePopover = (index: number) => {
  if (activeSubRoutePopoverIndex.value === index) {
    closeSubRoutePopover()
  } else {
    activeSubRoutePopoverIndex.value = index
    subRouteUserSearch.value = ''
    nextTick(() => {
      userSearchInputRef.value?.focus()
    })
  }
}

const closeSubRoutePopover = () => {
  activeSubRoutePopoverIndex.value = null
  subRouteUserSearch.value = ''
}

const setSubRouteMode = (sr: any, mode: 'all' | 'specific') => {
  if (mode === 'all') {
    sr.allowedUsers = []
  } else {
    if (isSubRouteAllOpen(sr)) {
      sr.allowedUsers = usersList.value.map((u: any) => u.email)
    }
  }
}

const handleDocumentClick = () => {
  if (activeSubRoutePopoverIndex.value !== null) {
    closeSubRoutePopover()
  }
}

const handleSubRouteKeyDown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && activeSubRoutePopoverIndex.value !== null) {
    e.stopPropagation()
    closeSubRoutePopover()
  }
}

const clearSubRouteUsers = (sr: any) => {
  sr.allowedUsers = []
}

const selectAllSubRouteUsers = (sr: any) => {
  sr.allowedUsers = usersList.value.map((u: any) => u.email)
}

const isSubRouteUserSelected = (sr: any, email: string): boolean => {
  return Array.isArray(sr.allowedUsers) && sr.allowedUsers.includes(email)
}

const toggleSubRouteUser = (sr: any, email: string) => {
  if (!Array.isArray(sr.allowedUsers)) {
    sr.allowedUsers = []
  }
  const idx = sr.allowedUsers.indexOf(email)
  if (idx > -1) {
    sr.allowedUsers.splice(idx, 1)
  } else {
    sr.allowedUsers.push(email)
  }
}

const addSubRoute = () => {
  if (!form.value.subRoutes) form.value.subRoutes = []
  const currentMax = form.value.subRoutes.reduce((max: number, sr: any) => Math.max(max, sr.routeId || 0), 0)
  const nextId = currentMax + 1
  form.value.subRoutes.push({
    id: Math.random().toString(36).substring(2, 9),
    name: `分流线路 #${nextId}`,
    routeId: nextId,
    outboundTag: availableOutbounds.value[0] || 'direct',
    enabled: true,
    allowedUsers: [],
  })
}

const removeSubRoute = (idx: number) => {
  if (activeSubRoutePopoverIndex.value === idx) {
    closeSubRoutePopover()
  } else if (activeSubRoutePopoverIndex.value !== null && activeSubRoutePopoverIndex.value > idx) {
    activeSubRoutePopoverIndex.value -= 1
  }
  form.value.subRoutes.splice(idx, 1)
}

const openCreateDrawer = () => {
  isEditing.value = false
  form.value = {
    id: 0,
    tag: `vless-tcp`,
    listen: '0.0.0.0',
    port: 443,
    externalPort: 443,
    externalHost: '',
    routeId: 0,
    subRoutes: [
      { id: '1', name: '🇯🇵 日本原生直连', routeId: 1, outboundTag: 'direct', enabled: true, allowedUsers: [] },
    ],
    protocol: 'vless',
    vlessFlow: 'xtls-rprx-vision',
    network: 'tcp',
    security: 'reality',
    selectedUserEmails: usersList.value.map((u) => u.email),
    xhttpPath: '/' + Math.random().toString(36).substring(2, 12),
    xhttpMode: 'auto',
    wsPath: '/ws',
    grpcService: 'xray-grpc',
    realityTarget: 'www.example.com:443',
    realityServerNames: 'www.example.com',
    realityPrivateKey: '',
    realityPublicKey: '',
    realityShortIds: '0123456789abcdef',
    tlsServerName: '',
    tlsCertFile: '',
    tlsKeyFile: '',
    fallbacksEnabled: false,
    fallbackDest: '80',
    fallbackXver: 0,
    socksAuth: 'noauth',
    socksUdp: true,
    socksUsername: '',
    socksPassword: '',
    httpUsername: '',
    httpPassword: '',
    dokoAddress: '127.0.0.1',
    dokoPort: 53,
    dokoNetwork: 'tcp,udp',
    sniffingEnabled: true,
    sniffingRouteOnly: true,
  }
  generateRealityKey()
  showInspectorDrawer.value = false
  showFormDrawer.value = true
}

const generateRealityKey = async () => {
  try {
    const pair: any = await api.get('/inbounds/reality-keypair')
    form.value.realityPrivateKey = pair.privateKey
    form.value.realityPublicKey = pair.publicKey
    if (!form.value.realityShortIds) {
      form.value.realityShortIds = pair.shortId
    }
  } catch (err) {
    console.error(err)
  }
}

const editInbound = (inb: any) => {
  isEditing.value = true
  form.value.id = inb.id
  form.value.tag = inb.tag
  form.value.listen = inb.listen || '0.0.0.0'
  form.value.port = inb.port
  form.value.externalPort = inb.externalPort || 0
  form.value.externalHost = inb.externalHost || ''
  form.value.routeId = inb.routeId || 0
  form.value.protocol = inb.protocol

  let srs: any[] = []
  try {
    srs = JSON.parse(inb.subRoutesJson || '[]')
  } catch (e) {}
  form.value.subRoutes = srs.length > 0 ? srs.map((r: any) => ({
    ...r,
    name: r.name || r.remark || '',
    allowedUsers: Array.isArray(r.allowedUsers) ? r.allowedUsers : [],
  })) : []

  let settings: any = {}
  try {
    settings = JSON.parse(inb.settingsJson || '{}')
  } catch (e) {}

  let stream: any = {}
  try {
    stream = JSON.parse(inb.streamSettingsJson || inb.streamSettings || '{}')
  } catch (e) {}

  const isTcp = (stream.network || inb.network || 'tcp') === 'tcp'
  form.value.vlessFlow = isTcp ? (settings.flow !== undefined ? settings.flow : (inb.protocol === 'vless' ? 'xtls-rprx-vision' : '')) : ''

  // 提取已绑定此节点的用户
  const assignedEmails: string[] = []
  if (settings.clients?.length > 0) {
    for (const c of settings.clients) {
      if (c.email) assignedEmails.push(c.email)
    }
  }
  for (const u of usersList.value) {
    const tags = (u.inboundTags || u.inboundTag || '').split(',').map((s: string) => s.trim())
    if (tags.includes(inb.tag) && !assignedEmails.includes(u.email)) {
      assignedEmails.push(u.email)
    }
  }
  form.value.selectedUserEmails = assignedEmails

  if (settings.fallbacks?.length > 0) {
    form.value.fallbacksEnabled = true
    form.value.fallbackDest = settings.fallbacks[0].dest || '80'
    form.value.fallbackXver = settings.fallbacks[0].xver || 0
  } else {
    form.value.fallbacksEnabled = false
  }

  // Socks
  form.value.socksAuth = settings.auth || 'noauth'
  form.value.socksUdp = settings.udp !== false
  if (settings.accounts?.length > 0) {
    form.value.socksUsername = settings.accounts[0].user || ''
    form.value.socksPassword = settings.accounts[0].pass || ''
  } else {
    form.value.socksUsername = ''
    form.value.socksPassword = ''
  }

  // HTTP Inbound
  if (settings.accounts?.length > 0) {
    form.value.httpUsername = settings.accounts[0].user || ''
    form.value.httpPassword = settings.accounts[0].pass || ''
  } else {
    form.value.httpUsername = ''
    form.value.httpPassword = ''
  }

  // dokodemo-door
  form.value.dokoAddress = settings.address || '127.0.0.1'
  form.value.dokoPort = settings.port || 53
  form.value.dokoNetwork = settings.network || 'tcp,udp'

  // stream settings
  form.value.network = stream.network || inb.network || 'tcp'
  form.value.security = stream.security || inb.security || 'none'
  if (stream.xhttpSettings) {
    form.value.xhttpPath = stream.xhttpSettings.path || ''
    form.value.xhttpMode = stream.xhttpSettings.mode || 'auto'
  }
  if (stream.wsSettings) {
    form.value.wsPath = stream.wsSettings.path || ''
  }
  if (stream.grpcSettings) {
    form.value.grpcService = stream.grpcSettings.serviceName || ''
  }
  if (stream.realitySettings) {
    form.value.realityTarget = stream.realitySettings.dest || stream.realitySettings.target || ''
    form.value.realityServerNames = (stream.realitySettings.serverNames || []).join(', ')
    form.value.realityPrivateKey = stream.realitySettings.privateKey || ''
    form.value.realityShortIds = (stream.realitySettings.shortIds || []).join(', ')
  }
  if (stream.tlsSettings) {
    form.value.tlsServerName = stream.tlsSettings.serverName || ''
    if (stream.tlsSettings.certificates?.length > 0) {
      form.value.tlsCertFile = stream.tlsSettings.certificates[0].certificateFile || ''
      form.value.tlsKeyFile = stream.tlsSettings.certificates[0].keyFile || ''
    } else {
      form.value.tlsCertFile = ''
      form.value.tlsKeyFile = ''
    }
  } else {
    form.value.tlsServerName = ''
    form.value.tlsCertFile = ''
    form.value.tlsKeyFile = ''
  }

  let sniff: any = {}
  try {
    sniff = JSON.parse(inb.sniffingJson || '{}')
  } catch (e) {}
  form.value.sniffingEnabled = sniff.enabled !== false
  form.value.sniffingRouteOnly = sniff.routeOnly === true

  showInspectorDrawer.value = false
  showFormDrawer.value = true
}

const buildSettingsJSON = () => {
  const settings: any = {}

  if (form.value.protocol === 'socks') {
    settings.auth = form.value.socksAuth || 'noauth'
    settings.udp = form.value.socksUdp !== false
    if (form.value.socksAuth === 'password' && (form.value.socksUsername || form.value.socksPassword)) {
      settings.accounts = [
        {
          user: form.value.socksUsername || '',
          pass: form.value.socksPassword || '',
        },
      ]
    }
    return JSON.stringify(settings, null, 2)
  }

  if (form.value.protocol === 'http') {
    if (form.value.httpUsername || form.value.httpPassword) {
      settings.accounts = [
        {
          user: form.value.httpUsername || '',
          pass: form.value.httpPassword || '',
        },
      ]
    }
    return JSON.stringify(settings, null, 2)
  }

  if (form.value.protocol === 'dokodemo-door') {
    settings.address = form.value.dokoAddress || '127.0.0.1'
    settings.port = form.value.dokoPort || 53
    settings.network = form.value.dokoNetwork || 'tcp,udp'
    return JSON.stringify(settings, null, 2)
  }

  const isTcp = form.value.network === 'tcp'
  const isTlsOrReality = form.value.security === 'reality' || form.value.security === 'tls'
  const flowVal = (isTcp && isTlsOrReality) ? (form.value.vlessFlow || '') : ''
  settings.flow = flowVal
  if (form.value.protocol === 'vless') {
    settings.decryption = 'none'
  }

  const clients: any[] = []
  for (const email of form.value.selectedUserEmails) {
    const userObj = usersList.value.find((u) => u.email === email)
    if (userObj) {
      clients.push({
        id: userObj.uuid,
        email: userObj.email,
        flow: flowVal,
        level: 0,
      })
    }
  }
  settings.clients = clients

  if (form.value.fallbacksEnabled && form.value.fallbackDest) {
    settings.fallbacks = [
      {
        dest: form.value.fallbackDest,
        xver: form.value.fallbackXver || 0,
      },
    ]
  }

  return JSON.stringify(settings, null, 2)
}

const buildStreamSettingsJSON = () => {
  const stream: any = {
    network: form.value.network,
    security: form.value.security,
  }

  if (form.value.network === 'xhttp') {
    stream.xhttpSettings = {
      path: form.value.xhttpPath || '/',
      mode: form.value.xhttpMode || 'auto',
      extra: {
        xPaddingBytes: '100-1000',
        scMaxEachPostBytes: '500000-1000000',
        scStreamUpServerSecs: '20-80',
      },
    }
  } else if (form.value.network === 'ws') {
    stream.wsSettings = {
      path: form.value.wsPath || '/',
    }
  } else if (form.value.network === 'grpc') {
    stream.grpcSettings = {
      serviceName: form.value.grpcService || 'xray-grpc',
    }
  }

  if (form.value.security === 'reality') {
    const sNames = form.value.realityServerNames
      .split(',')
      .map((s: string) => s.trim())
      .filter((s: string) => s)
    const sIds = form.value.realityShortIds
      .split(',')
      .map((s: string) => s.trim())

    stream.realitySettings = {
      dest: form.value.realityTarget || 'www.example.com:443',
      serverNames: sNames.length > 0 ? sNames : ['www.example.com'],
      privateKey: form.value.realityPrivateKey,
      shortIds: sIds,
    }
  } else if (form.value.security === 'tls') {
    const certs: any[] = []
    if (form.value.tlsCertFile?.trim() || form.value.tlsKeyFile?.trim()) {
      certs.push({
        certificateFile: form.value.tlsCertFile?.trim() || '',
        keyFile: form.value.tlsKeyFile?.trim() || '',
      })
    }
    stream.tlsSettings = {
      serverName: form.value.tlsServerName || '',
      ...(certs.length > 0 ? { certificates: certs } : {}),
    }
  }

  return JSON.stringify(stream, null, 2)
}

const buildSniffingJSON = () => {
  return JSON.stringify(
    {
      enabled: form.value.sniffingEnabled,
      destOverride: ['http', 'tls', 'quic'],
      routeOnly: form.value.sniffingRouteOnly,
    },
    null,
    2
  )
}

const saveInbound = async () => {
  saving.value = true
  try {
    const payload = {
      id: form.value.id,
      tag: form.value.tag,
      port: form.value.port,
      externalPort: form.value.externalPort || 0,
      externalHost: form.value.externalHost || '',
      routeId: form.value.routeId || 0,
      subRoutesJson: JSON.stringify(
        (form.value.subRoutes || []).map((sr: any) => ({
          id: String(sr.id || Math.random().toString(36).substring(2, 9)),
          name: sr.name || '',
          routeId: Number(sr.routeId || 0),
          outboundTag: sr.outboundTag || 'direct',
          enabled: Boolean(sr.enabled),
          allowedUsers: Array.isArray(sr.allowedUsers) && sr.allowedUsers.length > 0 ? sr.allowedUsers : undefined,
        }))
      ),
      listen: form.value.listen || '0.0.0.0',
      protocol: form.value.protocol,
      settingsJson: buildSettingsJSON(),
      streamSettings: buildStreamSettingsJSON(),
      sniffingJson: buildSniffingJSON(),
      enabled: true,
    }

    if (isEditing.value) {
      await api.put(`/inbounds/${form.value.id}`, payload)
    } else {
      await api.post('/inbounds', payload)
    }

    // 同步更新用户的 InboundTags 关系
    if (['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.value.protocol)) {
      for (const u of usersList.value) {
        const currentTags = (u.inboundTags || u.inboundTag || '').split(',').map((s: string) => s.trim()).filter((s: string) => s)
        const shouldHave = form.value.selectedUserEmails.includes(u.email)
        const has = currentTags.includes(form.value.tag)
        if (shouldHave && !has) {
          currentTags.push(form.value.tag)
          await api.put(`/users/${u.id}`, { ...u, inboundTags: currentTags.join(','), inboundTag: currentTags[0] })
        } else if (!shouldHave && has) {
          const nextTags = currentTags.filter((t: string) => t !== form.value.tag)
          await api.put(`/users/${u.id}`, { ...u, inboundTags: nextTags.join(','), inboundTag: nextTags[0] || '' })
        }
      }
    }

    showFormDrawer.value = false
    toast.success('节点配置已成功保存并重载核心')
    await fetchAll()
    if (selectedInbound.value && selectedInbound.value.id === form.value.id) {
      selectedInbound.value = inbounds.value.find((i) => i.id === form.value.id) || null
    }
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteInbound = async (id: number) => {
  if (!confirm('确定删除该入站节点吗？')) return
  try {
    await api.delete(`/inbounds/${id}`)
    toast.success('入站节点已成功删除')
    if (selectedInbound.value?.id === id) {
      showInspectorDrawer.value = false
      selectedInbound.value = null
    }
    await fetchAll()
  } catch (err: any) {
    toast.error('删除失败: ' + err)
  }
}

const getNodeFlow = (inb: any) => {
  try {
    const s = JSON.parse(inb.settingsJson || '{}')
    if (s.flow !== undefined) return s.flow || 'none'
    return 'none'
  } catch {
    return 'none'
  }
}

const getClientCount = (inb: any) => {
  if (['socks', 'http', 'dokodemo-door'].includes(inb.protocol)) {
    return 0
  }
  if (Array.isArray(usersList.value) && usersList.value.length > 0) {
    return usersList.value.filter((u: any) => {
      const tags = (u.inboundTags || u.inboundTag || '').split(',').map((s: string) => s.trim())
      return tags.includes(inb.tag)
    }).length
  }
  try {
    const s = JSON.parse(inb.settingsJson || '{}')
    return s.clients?.length || 0
  } catch {
    return 0
  }
}

const getStreamNetwork = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.network || 'tcp'
  } catch {
    return 'tcp'
  }
}

const getSecurityType = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.security || 'none'
  } catch {
    return 'none'
  }
}

const isReality = (inb: any) => {
  return getSecurityType(inb) === 'reality'
}

const checkRouteQuery = () => {
  const editQuery = route.query.edit as string
  if (editQuery && inbounds.value.length > 0) {
    const target = inbounds.value.find(
      (ib) => ib.tag === editQuery || String(ib.id) === String(editQuery)
    )
    if (target) {
      editInbound(target)
    } else {
      toast.warning('未找到指定的入站节点: ' + editQuery)
    }
  } else if (route.query.action === 'create' || route.query.create) {
    openCreateDrawer()
  }
}

onMounted(async () => {
  window.addEventListener('click', handleDocumentClick)
  window.addEventListener('keydown', handleSubRouteKeyDown, { capture: true })
  await fetchAll()
  checkRouteQuery()
})

onUnmounted(() => {
  window.removeEventListener('click', handleDocumentClick)
  window.removeEventListener('keydown', handleSubRouteKeyDown, { capture: true })
})

watch(showFormDrawer, (val) => {
  if (!val) {
    closeSubRoutePopover()
  }
})

watch(
  () => [route.query.edit, route.query.action, route.query.create],
  () => {
    checkRouteQuery()
  }
)
</script>
