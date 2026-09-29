<template>
  <div class="space-y-4">
    <!-- Top Action & Filter Header -->
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-3 pb-2 border-b border-border/60">
      <div>
        <div class="flex items-center gap-2">
          <h1 class="text-lg font-semibold text-foreground tracking-tight">出站代理管理 (Outbounds)</h1>
          <Badge variant="outline" class="text-[10px]">
            {{ filteredOutbounds.length }} 个节点
          </Badge>
          <span class="px-2 py-0.5 rounded text-[10px] font-mono bg-amber-500/10 text-amber-400 border border-amber-500/20">
            修改自动重启核心生效
          </span>
        </div>
        <p class="text-xs text-muted-foreground mt-0.5">
          配置直连、黑洞拦截、Cloudflare WARP (WireGuard) 与链式上游代理，保存后自动落盘并平滑应用
        </p>
      </div>

      <!-- Action Buttons -->
      <div class="flex items-center gap-2">
        <Button
          variant="default"
          size="sm"
          @click="openCreateDrawer"
        >
          <Plus class="w-3.5 h-3.5 mr-1" />
          <span>添加出站节点</span>
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
            placeholder="搜索节点 Tag / 协议 / 目标地址..."
            class="w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md pl-8 pr-3 h-8 placeholder:text-muted-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
          />
        </div>

        <!-- Protocol Filter -->
        <select
          v-model="protocolFilter"
          class="h-8 bg-neutral-950 border border-border text-foreground text-xs rounded-md px-2.5 font-mono focus:outline-none focus:ring-1 focus:ring-ring shrink-0"
        >
          <option value="all">全部协议</option>
          <option value="freedom">freedom (直连)</option>
          <option value="wireguard">wireguard (WARP)</option>
          <option value="vless">vless</option>
          <option value="vmess">vmess</option>
          <option value="trojan">trojan</option>
          <option value="shadowsocks">shadowsocks</option>
          <option value="socks">socks</option>
          <option value="http">http</option>
          <option value="blackhole">blackhole (黑洞)</option>
        </select>
      </div>
    </div>

    <!-- Main Table View -->
    <div class="relative w-full overflow-hidden rounded-lg border border-border bg-neutral-950">
      <table class="w-full caption-bottom text-xs border-collapse">
        <thead class="border-b border-border bg-neutral-900">
          <tr>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">节点标识 (Tag)</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">出站协议</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">传输与安全</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">目标端点 / 地址</th>
            <th class="h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">配置摘要</th>
            <th class="h-9 px-3.5 text-right align-middle font-mono text-[11px] font-medium text-muted-foreground uppercase tracking-wider">操作</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-border/40">
          <tr
            v-for="ob in filteredOutbounds"
            :key="ob.tag"
            @click="inspectOutbound(ob)"
            class="border-b border-border/30 transition-colors hover:bg-muted/30 cursor-pointer"
            :class="{ 'bg-muted/20': selectedOutbound?.tag === ob.tag }"
          >
            <!-- 1. Tag & Description -->
            <td class="p-3.5 align-middle">
              <div class="flex items-center gap-2">
                <span class="font-mono font-semibold text-foreground text-xs">{{ ob.tag }}</span>
                <span v-if="ob.tag === 'direct' || ob.tag === 'block'" class="text-[10px] font-mono px-1 py-0.2 rounded bg-neutral-800 text-neutral-400 border border-neutral-700">
                  系统默认
                </span>
              </div>
              <div class="text-[11px] text-muted-foreground mt-0.5 truncate max-w-[200px]">
                {{ getUsageDesc(ob) }}
              </div>
            </td>

            <!-- 2. Protocol -->
            <td class="p-3.5 align-middle">
              <Badge :variant="getProtocolBadgeVariant(ob.protocol)">
                {{ ob.protocol?.toUpperCase() }}
              </Badge>
            </td>

            <!-- 3. Network & Security -->
            <td class="p-3.5 align-middle font-mono">
              <div v-if="getOutboundStreamInfo(ob).network || getOutboundStreamInfo(ob).security" class="flex items-center gap-1.5">
                <span v-if="getOutboundStreamInfo(ob).network" class="px-1.5 py-0.5 rounded text-[10px] bg-neutral-900 border border-border text-neutral-300">
                  {{ getOutboundStreamInfo(ob).network.toUpperCase() }}
                </span>
                <span v-if="getOutboundStreamInfo(ob).security && getOutboundStreamInfo(ob).security !== 'none'" class="px-1.5 py-0.5 rounded text-[10px] bg-cyan-950/60 border border-cyan-800/50 text-cyan-300">
                  {{ getOutboundStreamInfo(ob).security.toUpperCase() }}
                </span>
              </div>
              <span v-else class="text-neutral-500 text-[11px]">-</span>
            </td>

            <!-- 4. Target Endpoint / Host -->
            <td class="p-3.5 align-middle font-mono text-[11px] text-foreground">
              {{ getTargetEndpoint(ob) }}
            </td>

            <!-- 5. Settings Summary -->
            <td class="p-3.5 align-middle">
              <div class="text-[11px] font-mono text-muted-foreground max-w-[240px] truncate" :title="formatSettingsSummary(ob)">
                {{ formatSettingsSummary(ob) }}
              </div>
            </td>

            <!-- 6. Actions -->
            <td class="p-3.5 align-middle text-right" @click.stop>
              <div class="flex items-center justify-end gap-1.5">
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-xs"
                  @click="inspectOutbound(ob)"
                  title="查看详情与参数"
                >
                  <Eye class="w-3.5 h-3.5" />
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-xs text-brand-400 hover:text-brand-300"
                  @click="editOutbound(ob)"
                  title="编辑配置"
                >
                  <Edit3 class="w-3.5 h-3.5" />
                </Button>
                <Button
                  v-if="ob.tag !== 'direct' && ob.tag !== 'block'"
                  variant="ghost"
                  size="sm"
                  class="h-7 px-2 text-xs text-rose-400 hover:text-rose-300 hover:bg-rose-950/40"
                  @click="deleteOutbound(ob.tag)"
                  title="删除节点"
                >
                  <Trash2 class="w-3.5 h-3.5" />
                </Button>
              </div>
            </td>
          </tr>

          <!-- Empty State -->
          <tr v-if="filteredOutbounds.length === 0">
            <td colspan="6" class="p-8 text-center text-muted-foreground">
              <p class="text-xs">未找到符合条件的出站节点</p>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Inspector Drawer (巡检与参数详情) -->
    <Drawer
      v-model="showInspectorDrawer"
      :title="`出站节点详情: ${selectedOutbound?.tag || ''}`"
      description="查看节点结构化参数、流控策略与底层 JSON 配置"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <div v-if="selectedOutbound" class="space-y-5">
        <!-- Overview Card -->
        <div class="p-4 rounded-lg bg-card border border-border space-y-3">
          <div class="flex items-center justify-between">
            <div class="flex items-center gap-2">
              <span class="text-base font-bold font-mono text-foreground">{{ selectedOutbound.tag }}</span>
              <Badge :variant="getProtocolBadgeVariant(selectedOutbound.protocol)">
                {{ selectedOutbound.protocol?.toUpperCase() }}
              </Badge>
            </div>
            <span class="text-xs text-muted-foreground font-mono">
              {{ getUsageDesc(selectedOutbound) }}
            </span>
          </div>

          <div class="grid grid-cols-2 gap-2 text-xs font-mono pt-2 border-t border-border/60">
            <div>
              <span class="text-muted-foreground block text-[11px]">出站协议</span>
              <span class="text-foreground font-semibold">{{ selectedOutbound.protocol }}</span>
            </div>
            <div>
              <span class="text-muted-foreground block text-[11px]">目标端点</span>
              <span class="text-foreground font-semibold">{{ getTargetEndpoint(selectedOutbound) }}</span>
            </div>
          </div>
        </div>

        <!-- WireGuard / WARP details if wireguard -->
        <div v-if="selectedOutbound.protocol === 'wireguard'" class="p-4 rounded-lg bg-card border border-border space-y-3">
          <h4 class="text-xs font-semibold text-cyan-400 font-mono flex items-center gap-1.5">
            <ShieldCheck class="w-3.5 h-3.5" />
            <span>WireGuard / Cloudflare WARP 参数</span>
          </h4>
          <div class="space-y-2 text-xs font-mono">
            <div class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">Endpoint (对端服务器)</span>
              <span class="text-foreground">{{ getParsedSettings(selectedOutbound).peers?.[0]?.endpoint || '-' }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">Address (本地隧道分配IP)</span>
              <span class="text-foreground">{{ (getParsedSettings(selectedOutbound).address || []).join(', ') || '-' }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">Peer Public Key</span>
              <span class="text-foreground truncate max-w-[280px]" :title="getParsedSettings(selectedOutbound).peers?.[0]?.publicKey">
                {{ getParsedSettings(selectedOutbound).peers?.[0]?.publicKey || '-' }}
              </span>
            </div>
          </div>
        </div>

        <!-- Stream Security / TLS / Reality details -->
        <div v-if="getOutboundStreamInfo(selectedOutbound).network" class="p-4 rounded-lg bg-card border border-border space-y-3">
          <h4 class="text-xs font-semibold text-foreground font-mono flex items-center gap-1.5">
            <Layers class="w-3.5 h-3.5 text-brand-400" />
            <span>传输与安全层 (StreamSettings)</span>
          </h4>
          <div class="space-y-2 text-xs font-mono">
            <div class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">传输协议 (Network)</span>
              <span class="text-foreground">{{ getOutboundStreamInfo(selectedOutbound).network?.toUpperCase() }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">安全协议 (Security)</span>
              <span class="text-foreground">{{ getOutboundStreamInfo(selectedOutbound).security?.toUpperCase() || 'NONE' }}</span>
            </div>
            <div v-if="getOutboundStreamInfo(selectedOutbound).realitySettings?.serverName" class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">REALITY SNI 伪装</span>
              <span class="text-cyan-400 font-semibold">{{ getOutboundStreamInfo(selectedOutbound).realitySettings?.serverName }}</span>
            </div>
            <div v-if="getOutboundStreamInfo(selectedOutbound).realitySettings?.publicKey" class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">REALITY Public Key</span>
              <span class="text-foreground truncate max-w-[280px]">{{ getOutboundStreamInfo(selectedOutbound).realitySettings?.publicKey }}</span>
            </div>
            <div v-if="getOutboundStreamInfo(selectedOutbound).tlsSettings?.serverName" class="flex justify-between py-1 border-b border-border/40">
              <span class="text-muted-foreground">TLS ServerName</span>
              <span class="text-foreground">{{ getOutboundStreamInfo(selectedOutbound).tlsSettings?.serverName }}</span>
            </div>
          </div>
        </div>

        <!-- Raw Settings JSON -->
        <div class="p-4 rounded-lg bg-card border border-border space-y-2">
          <div class="flex items-center justify-between">
            <h4 class="text-xs font-semibold text-muted-foreground font-mono">底盘配置 JSON (settingsJson)</h4>
            <Button variant="ghost" size="sm" class="h-6 px-2 text-[11px]" @click="copyText(selectedOutbound.settingsJson)">
              <Copy class="w-3 h-3 mr-1" />
              <span>复制</span>
            </Button>
          </div>
          <pre class="bg-neutral-950 p-3 rounded border border-border text-[11px] font-mono text-muted-foreground overflow-x-auto max-h-48 leading-relaxed">{{ formatJsonPretty(selectedOutbound.settingsJson) }}</pre>
        </div>

        <!-- Raw StreamSettings JSON if exists -->
        <div v-if="selectedOutbound.streamSettings" class="p-4 rounded-lg bg-card border border-border space-y-2">
          <div class="flex items-center justify-between">
            <h4 class="text-xs font-semibold text-muted-foreground font-mono">传输配置 JSON (streamSettings)</h4>
            <Button variant="ghost" size="sm" class="h-6 px-2 text-[11px]" @click="copyText(selectedOutbound.streamSettings)">
              <Copy class="w-3 h-3 mr-1" />
              <span>复制</span>
            </Button>
          </div>
          <pre class="bg-neutral-950 p-3 rounded border border-border text-[11px] font-mono text-muted-foreground overflow-x-auto max-h-48 leading-relaxed">{{ formatJsonPretty(selectedOutbound.streamSettings) }}</pre>
        </div>
      </div>

      <template #footer>
        <Button variant="secondary" size="sm" @click="showInspectorDrawer = false">
          关闭
        </Button>
        <Button
          v-if="selectedOutbound && selectedOutbound.tag !== 'direct' && selectedOutbound.tag !== 'block'"
          variant="destructive"
          size="sm"
          @click="deleteOutbound(selectedOutbound.tag)"
        >
          <Trash2 class="w-3.5 h-3.5 mr-1" />
          <span>删除节点</span>
        </Button>
        <Button
          v-if="selectedOutbound"
          variant="default"
          size="sm"
          @click="editOutbound(selectedOutbound)"
        >
          <Edit3 class="w-3.5 h-3.5 mr-1" />
          <span>编辑配置</span>
        </Button>
      </template>
    </Drawer>

    <!-- Outbound Form Drawer (创建与编辑抽屉) -->
    <Drawer
      v-model="showFormDrawer"
      :title="isEditing ? `编辑出站节点: ${form.tag}` : '添加新出站节点'"
      description="配置将同步回写磁盘 config.json 并平滑重启核心应用"
      width="w-full sm:max-w-xl md:max-w-2xl"
    >
      <form id="outbound-form" @submit.prevent="saveOutbound" class="space-y-4 text-xs">
        <div>
          <label class="block text-foreground mb-1 font-medium">出站标识 (Tag)</label>
          <input
            v-model="form.tag"
            type="text"
            required
            :disabled="isEditing"
            placeholder="例如: warp-out 或 proxy-jp"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
          />
        </div>

        <div>
          <label class="block text-foreground mb-1 font-medium">出站协议 (Protocol)</label>
          <select
            v-model="form.protocol"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring"
          >
            <option value="freedom">freedom (直连访问)</option>
            <option value="blackhole">blackhole (黑洞丢弃/拦截)</option>
            <option value="wireguard">wireguard (Cloudflare WARP 出站)</option>
            <option value="vless">vless (上游链式代理)</option>
            <option value="vmess">vmess (上游链式代理)</option>
            <option value="trojan">trojan (上游链式代理)</option>
            <option value="shadowsocks">shadowsocks (上游链式代理)</option>
            <option value="socks">socks 代理出站</option>
            <option value="http">http 代理出站</option>
          </select>
        </div>

        <!-- Freedom 专属配置 -->
        <div v-if="form.protocol === 'freedom'" class="space-y-3 bg-neutral-900/60 p-4 rounded-lg border border-border">
          <h3 class="font-bold text-emerald-400 text-xs">Freedom 策略</h3>
          <div>
            <label class="block text-muted-foreground mb-1">域名解析策略 (domainStrategy)</label>
            <select
              v-model="form.freedomDomainStrategy"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="UseIP">UseIP</option>
              <option value="UseIPv4">UseIPv4 (强制IPv4 - 推荐)</option>
              <option value="UseIPv6">UseIPv6 (强制IPv6)</option>
              <option value="AsIs">AsIs (保持原样)</option>
            </select>
          </div>
        </div>

        <!-- Blackhole 专属配置 -->
        <div v-else-if="form.protocol === 'blackhole'" class="space-y-3 bg-neutral-900/60 p-4 rounded-lg border border-border">
          <h3 class="font-bold text-rose-400 text-xs">Blackhole 拦截响应</h3>
          <div>
            <label class="block text-muted-foreground mb-1">响应类型 (Response Type)</label>
            <select
              v-model="form.blackholeResponse"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="none">none (直接静默丢弃)</option>
              <option value="http">http (返回 HTTP 403 阻断页面)</option>
            </select>
          </div>
        </div>

        <!-- WireGuard / WARP 专属配置 -->
        <div v-else-if="form.protocol === 'wireguard'" class="space-y-3 bg-neutral-900/60 p-4 rounded-lg border border-border">
          <h3 class="font-bold text-cyan-400 text-xs">WireGuard (Cloudflare WARP) 详情</h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1">Secret Key (私钥)</label>
              <input
                v-model="form.wgSecretKey"
                type="text"
                placeholder="填写 WireGuard Private Key (Base64)"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">Peer Public Key (对端公钥)</label>
              <input
                v-model="form.wgPeerPublicKey"
                type="text"
                placeholder="填写 Cloudflare WARP 对端公钥"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1">Endpoint (对端服务器)</label>
              <input
                v-model="form.wgEndpoint"
                type="text"
                placeholder="162.159.192.1:2408"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">本地地址 (Address)</label>
              <input
                v-model="form.wgAddress"
                type="text"
                placeholder="172.16.0.2/32"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>
        </div>

        <!-- Proxy outbounds (VLESS / VMess / Trojan / Shadowsocks / Socks / HTTP) -->
        <div v-else class="space-y-3 bg-neutral-900/60 p-4 rounded-lg border border-border">
          <h3 class="font-bold text-indigo-400 text-xs">上游代理服务器连接信息</h3>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1">服务器地址 (Host/Address)</label>
              <input
                v-model="form.proxyHost"
                type="text"
                placeholder="127.0.0.1 或 proxy.example.com"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">服务器端口 (Port)</label>
              <input
                v-model.number="form.proxyPort"
                type="number"
                placeholder="443"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <div v-if="['vless', 'vmess', 'trojan'].includes(form.protocol)">
            <label class="block text-muted-foreground mb-1">UUID / 密码 (Password)</label>
            <input
              v-model="form.proxyPassword"
              type="text"
              placeholder="UUID 或密码"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </div>

          <!-- VMess 专属安全加密 -->
          <div v-if="form.protocol === 'vmess'" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1">VMess 加密 (Security)</label>
              <select
                v-model="form.vmessSecurity"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="auto">auto (自动匹配)</option>
                <option value="aes-128-gcm">aes-128-gcm</option>
                <option value="chacha20-poly1305">chacha20-poly1305</option>
                <option value="none">none</option>
              </select>
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">AlterID (额外ID)</label>
              <input
                v-model.number="form.vmessAlterId"
                type="number"
                placeholder="0"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <!-- Shadowsocks 专属密码与加密方式 -->
          <div v-if="form.protocol === 'shadowsocks'" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1">SS 密码 (Password)</label>
              <input
                v-model="form.proxyPassword"
                type="text"
                placeholder="Shadowsocks 密码"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">加密算法 (Method)</label>
              <select
                v-model="form.ssMethod"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
                <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm</option>
                <option value="2022-blake3-chacha20-poly1305">2022-blake3-chacha20-poly1305</option>
                <option value="aes-256-gcm">aes-256-gcm</option>
                <option value="aes-128-gcm">aes-128-gcm</option>
                <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
                <option value="none">none</option>
              </select>
            </div>
          </div>

          <!-- Socks / HTTP 专属可选用户名密码 -->
          <div v-if="['socks', 'http'].includes(form.protocol)" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <div>
              <label class="block text-muted-foreground mb-1">认证用户名 (选填)</label>
              <input
                v-model="form.proxyUsername"
                type="text"
                placeholder="用户名 (如无需认证可留空)"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">认证密码 (选填)</label>
              <input
                v-model="form.proxyPassword"
                type="password"
                placeholder="密码 (如无需认证可留空)"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <!-- 传输层与安全层联动 -->
          <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol)" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border">
            <div>
              <label class="block text-muted-foreground mb-1">传输协议 (Network)</label>
              <select
                v-model="form.streamNetwork"
                @change="onOutboundNetworkChange"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="tcp">TCP</option>
                <option value="xhttp">XHTTP</option>
                <option value="grpc">gRPC</option>
                <option value="ws">WebSocket</option>
                <option value="httpupgrade">HTTPUpgrade</option>
              </select>
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">安全协议 (Security)</label>
              <select
                v-model="form.streamSecurity"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option v-if="['tcp', 'xhttp', 'grpc'].includes(form.streamNetwork)" value="reality">REALITY</option>
                <option value="tls">TLS</option>
                <option value="none">None</option>
              </select>
            </div>
          </div>

          <!-- VLESS 出站流控选项 (XTLS Vision) -->
          <div v-if="form.protocol === 'vless'" class="pt-2 border-t border-border">
            <div class="flex items-center justify-between mb-1">
              <label class="text-foreground font-medium">流控策略 (Flow / XTLS Vision)</label>
              <span class="text-[11px] text-brand-400 font-mono">XTLS / REALITY 极速流控</span>
            </div>
            <select
              v-model="form.vlessFlow"
              :disabled="form.streamNetwork !== 'tcp' || !['reality', 'tls'].includes(form.streamSecurity)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-40 font-mono"
            >
              <option value="xtls-rprx-vision">xtls-rprx-vision (XTLS Vision 极速流控 - 推荐)</option>
              <option value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option>
              <option value="">none (无流控 - 适用于 XHTTP / gRPC / WS 等)</option>
            </select>
            <p v-if="form.streamNetwork !== 'tcp' || !['reality', 'tls'].includes(form.streamSecurity)" class="text-[11px] text-muted-foreground mt-1">
              * Vision 流控仅适用于 TCP + TLS / REALITY 架构。
            </p>
          </div>

          <!-- XHTTP 专用参数 -->
          <div v-if="form.streamNetwork === 'xhttp'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
            <div>
              <label class="block text-muted-foreground mb-1">XHTTP 路径 (Path)</label>
              <input
                v-model="form.xhttpPath"
                type="text"
                placeholder="/mbqyfa4grswh5ntz"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">XHTTP 模式 (Mode)</label>
              <select
                v-model="form.xhttpMode"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="auto">auto</option>
                <option value="stream-up">stream-up</option>
                <option value="stream-one">stream-one</option>
              </select>
            </div>
          </div>

          <!-- WS 专用参数 -->
          <div v-if="form.streamNetwork === 'ws'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
            <div>
              <label class="block text-muted-foreground mb-1">WebSocket 路径 (Path)</label>
              <input
                v-model="form.wsPath"
                type="text"
                placeholder="/ws"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">WebSocket Host 头部 (选填)</label>
              <input
                v-model="form.wsHost"
                type="text"
                placeholder="例如: example.com"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <!-- REALITY 专属连接参数 -->
          <div v-if="form.streamSecurity === 'reality'" class="space-y-3 pt-2 border-t border-border/60 bg-neutral-950/60 p-3 rounded-md">
            <h4 class="text-xs font-bold text-cyan-400">REALITY 握手伪装参数</h4>
            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-muted-foreground mb-1">SNI 伪装域名 (ServerName)</label>
                <input
                  v-model="form.realityServerName"
                  type="text"
                  placeholder="www.example.com"
                  class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
                />
              </div>
              <div>
                <label class="block text-muted-foreground mb-1">指纹伪装 (Fingerprint)</label>
                <select
                  v-model="form.realityFingerprint"
                  class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
                >
                  <option value="chrome">chrome (推荐)</option>
                  <option value="firefox">firefox</option>
                  <option value="safari">safari</option>
                  <option value="ios">ios</option>
                  <option value="edge">edge</option>
                </select>
              </div>
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">Public Key (对端公钥 / pbk)</label>
              <input
                v-model="form.realityPublicKey"
                type="text"
                placeholder="例如: FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div>
              <label class="block text-muted-foreground mb-1">Short ID (短ID / sid)</label>
              <input
                v-model="form.realityShortId"
                type="text"
                placeholder="0123456789abcdef"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
          </div>

          <!-- TLS 专属连接参数 -->
          <div v-if="form.streamSecurity === 'tls'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60 bg-neutral-950/60 p-3 rounded-md">
            <div>
              <label class="block text-muted-foreground mb-1">TLS SNI (ServerName)</label>
              <input
                v-model="form.tlsServerName"
                type="text"
                placeholder="example.com"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </div>
            <div class="flex items-center pt-6">
              <label class="flex items-center gap-2 text-xs text-foreground cursor-pointer">
                <input v-model="form.tlsAllowInsecure" type="checkbox" class="w-4 h-4 rounded text-brand-500 bg-neutral-950 border-border focus:ring-0" />
                <span>允许不安全证书 (allowInsecure)</span>
              </label>
            </div>
          </div>
        </div>
      </form>

      <template #footer>
        <Button variant="secondary" size="sm" @click="showFormDrawer = false">
          取消
        </Button>
        <Button
          type="submit"
          form="outbound-form"
          variant="default"
          size="sm"
          :loading="saving"
        >
          <span>{{ saving ? '保存中...' : '保存出站配置' }}</span>
        </Button>
      </template>
    </Drawer>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Plus, Search, Edit3, Trash2, Eye, ShieldCheck, Layers, Copy } from 'lucide-vue-next'
import Button from '../components/ui/Button.vue'
import Badge from '../components/ui/Badge.vue'
import Drawer from '../components/ui/Drawer.vue'
import { toast } from '../utils/toast'
import { copyText } from '../utils/clipboard'
import api from '../api'

const outbounds = ref<any[]>([])
const searchQuery = ref('')
const protocolFilter = ref('all')

const showFormDrawer = ref(false)
const showInspectorDrawer = ref(false)
const selectedOutbound = ref<any>(null)
const isEditing = ref(false)
const saving = ref(false)
const route = useRoute()

const form = ref<any>({
  tag: '',
  protocol: 'wireguard',
  freedomDomainStrategy: 'UseIPv4',
  blackholeResponse: 'none',
  wgSecretKey: '',
  wgPeerPublicKey: '',
  wgEndpoint: '162.159.192.1:2408',
  wgAddress: '172.16.0.2/32',
  proxyHost: '',
  proxyPort: 443,
  proxyPassword: '',
  vlessFlow: 'xtls-rprx-vision',
  streamNetwork: 'xhttp',
  streamSecurity: 'reality',
  xhttpPath: '/mbqyfa4grswh5ntz',
  xhttpMode: 'auto',
  wsPath: '/ws',
  wsHost: '',
  grpcServiceName: 'xray-grpc',
  realityServerName: 'www.example.com',
  realityFingerprint: 'chrome',
  realityPublicKey: '',
  realityShortId: '0123456789abcdef',
  tlsServerName: '',
  tlsAllowInsecure: false,
  tlsFingerprint: 'chrome',
})

const filteredOutbounds = computed(() => {
  return outbounds.value.filter((ob) => {
    if (protocolFilter.value !== 'all' && ob.protocol?.toLowerCase() !== protocolFilter.value.toLowerCase()) {
      return false
    }
    if (!searchQuery.value) return true
    const q = searchQuery.value.toLowerCase()
    return (
      ob.tag?.toLowerCase().includes(q) ||
      ob.protocol?.toLowerCase().includes(q) ||
      getTargetEndpoint(ob).toLowerCase().includes(q) ||
      formatSettingsSummary(ob).toLowerCase().includes(q)
    )
  })
})

const onOutboundNetworkChange = () => {
  if (!['tcp', 'xhttp', 'grpc'].includes(form.value.streamNetwork) && form.value.streamSecurity === 'reality') {
    form.value.streamSecurity = 'tls'
  }
}

const fetchOutbounds = async () => {
  try {
    outbounds.value = await api.get('/outbounds')
    if (selectedOutbound.value) {
      const updated = outbounds.value.find((o) => o.tag === selectedOutbound.value.tag)
      if (updated) selectedOutbound.value = updated
    }
  } catch (err) {
    console.error(err)
  }
}

const inspectOutbound = (ob: any) => {
  selectedOutbound.value = ob
  showInspectorDrawer.value = true
}

const openCreateDrawer = () => {
  isEditing.value = false
  form.value = {
    tag: '',
    protocol: 'vless',
    freedomDomainStrategy: 'UseIPv4',
    blackholeResponse: 'none',
    wgSecretKey: '',
    wgPeerPublicKey: '',
    wgEndpoint: '162.159.192.1:2408',
    wgAddress: '172.16.0.2/32',
    proxyHost: '',
    proxyPort: 443,
    proxyPassword: '',
    vlessFlow: 'xtls-rprx-vision',
    streamNetwork: 'xhttp',
    streamSecurity: 'reality',
    xhttpPath: '/mbqyfa4grswh5ntz',
    xhttpMode: 'auto',
    wsPath: '/ws',
    wsHost: '',
    grpcServiceName: 'xray-grpc',
    realityServerName: 'www.example.com',
    realityFingerprint: 'chrome',
    realityPublicKey: '',
    realityShortId: '0123456789abcdef',
    tlsServerName: '',
    tlsAllowInsecure: false,
    tlsFingerprint: 'chrome',
  }
  showFormDrawer.value = true
}

const editOutbound = (ob: any) => {
  isEditing.value = true
  form.value.tag = ob.tag
  form.value.protocol = ob.protocol

  let s: any = {}
  try {
    s = JSON.parse(ob.settingsJson || '{}')
  } catch (e) {}

  let str: any = {}
  try {
    str = JSON.parse(ob.streamSettings || '{}')
  } catch (e) {}

  form.value.streamNetwork = str.network || 'tcp'
  form.value.streamSecurity = str.security || 'none'
  form.value.freedomDomainStrategy = s.domainStrategy || 'UseIPv4'
  form.value.blackholeResponse = s.response?.type || 'none'

  if (str.xhttpSettings) {
    form.value.xhttpPath = str.xhttpSettings.path || '/mbqyfa4grswh5ntz'
    form.value.xhttpMode = str.xhttpSettings.mode || 'auto'
  }
  if (str.wsSettings) {
    form.value.wsPath = str.wsSettings.path || '/ws'
    form.value.wsHost = str.wsSettings.headers?.Host || ''
  }
  if (str.grpcSettings) {
    form.value.grpcServiceName = str.grpcSettings.serviceName || 'xray-grpc'
  }
  if (str.realitySettings) {
    form.value.realityServerName = str.realitySettings.serverName || 'www.example.com'
    form.value.realityPublicKey = str.realitySettings.publicKey || ''
    form.value.realityShortId = str.realitySettings.shortId || ''
    form.value.realityFingerprint = str.realitySettings.fingerprint || 'chrome'
  }
  if (str.tlsSettings) {
    form.value.tlsServerName = str.tlsSettings.serverName || ''
    form.value.tlsAllowInsecure = str.tlsSettings.allowInsecure === true
    form.value.tlsFingerprint = str.tlsSettings.fingerprint || 'chrome'
  }

  if (ob.protocol === 'wireguard') {
    form.value.wgSecretKey = s.secretKey || ''
    form.value.wgAddress = (s.address || ['172.16.0.2/32'])[0]
    if (s.peers?.length > 0) {
      form.value.wgEndpoint = s.peers[0].endpoint || ''
      form.value.wgPeerPublicKey = s.peers[0].publicKey || ''
    }
  } else if (['vless', 'vmess'].includes(ob.protocol)) {
    if (s.vnext?.length > 0) {
      form.value.proxyHost = s.vnext[0].address || ''
      form.value.proxyPort = s.vnext[0].port || 443
      if (s.vnext[0].users?.length > 0) {
        const u = s.vnext[0].users[0]
        form.value.proxyPassword = u.id || u.password || ''
        if (ob.protocol === 'vless') {
          form.value.vlessFlow = u.flow !== undefined ? u.flow : 'xtls-rprx-vision'
        }
      }
    }
  } else if (ob.protocol === 'trojan') {
    if (s.servers?.length > 0) {
      form.value.proxyHost = s.servers[0].address || ''
      form.value.proxyPort = s.servers[0].port || 443
      form.value.proxyPassword = s.servers[0].password || ''
    }
  } else if (ob.protocol === 'shadowsocks') {
    if (s.servers?.length > 0) {
      form.value.proxyHost = s.servers[0].address || ''
      form.value.proxyPort = s.servers[0].port || 443
      form.value.proxyPassword = s.servers[0].password || ''
      form.value.ssMethod = s.servers[0].method || '2022-blake3-aes-128-gcm'
    }
  } else if (['socks', 'http'].includes(ob.protocol)) {
    if (s.servers?.length > 0) {
      form.value.proxyHost = s.servers[0].address || ''
      form.value.proxyPort = s.servers[0].port || 1080
      if (s.servers[0].users?.length > 0) {
        form.value.proxyUsername = s.servers[0].users[0].user || ''
        form.value.proxyPassword = s.servers[0].users[0].pass || ''
      }
    }
  }

  showInspectorDrawer.value = false
  showFormDrawer.value = true
}

const buildSettingsJSON = () => {
  if (form.value.protocol === 'freedom') {
    return JSON.stringify({
      domainStrategy: form.value.freedomDomainStrategy || 'UseIPv4',
    })
  } else if (form.value.protocol === 'blackhole') {
    return JSON.stringify({
      response: {
        type: form.value.blackholeResponse || 'none',
      },
    })
  } else if (form.value.protocol === 'wireguard') {
    return JSON.stringify({
      secretKey: form.value.wgSecretKey,
      address: [form.value.wgAddress || '172.16.0.2/32'],
      noKernelTun: true,
      mtu: 1280,
      peers: [
        {
          endpoint: form.value.wgEndpoint,
          publicKey: form.value.wgPeerPublicKey,
        },
      ],
    })
  } else if (form.value.protocol === 'vless') {
    const isTcp = form.value.streamNetwork === 'tcp'
    const isTlsOrReality = ['reality', 'tls'].includes(form.value.streamSecurity)
    const userObj: any = {
      id: form.value.proxyPassword,
      encryption: 'none',
    }
    if (isTcp && isTlsOrReality && form.value.vlessFlow) {
      userObj.flow = form.value.vlessFlow
    }
    return JSON.stringify({
      vnext: [
        {
          address: form.value.proxyHost,
          port: form.value.proxyPort,
          users: [userObj],
        },
      ],
    })
  } else if (form.value.protocol === 'vmess') {
    return JSON.stringify({
      vnext: [
        {
          address: form.value.proxyHost,
          port: form.value.proxyPort,
          users: [
            {
              id: form.value.proxyPassword,
              security: form.value.vmessSecurity || 'auto',
              alterId: form.value.vmessAlterId || 0,
            },
          ],
        },
      ],
    })
  } else if (form.value.protocol === 'shadowsocks') {
    return JSON.stringify({
      servers: [
        {
          address: form.value.proxyHost,
          port: form.value.proxyPort,
          method: form.value.ssMethod || '2022-blake3-aes-128-gcm',
          password: form.value.proxyPassword,
        },
      ],
    })
  } else if (form.value.protocol === 'trojan') {
    return JSON.stringify({
      servers: [
        {
          address: form.value.proxyHost,
          port: form.value.proxyPort,
          password: form.value.proxyPassword,
        },
      ],
    })
  } else if (['socks', 'http'].includes(form.value.protocol)) {
    const srv: any = {
      address: form.value.proxyHost,
      port: form.value.proxyPort,
    }
    if (form.value.proxyUsername || form.value.proxyPassword) {
      srv.users = [
        {
          user: form.value.proxyUsername || '',
          pass: form.value.proxyPassword || '',
        },
      ]
    }
    return JSON.stringify({
      servers: [srv],
    })
  }
  return '{}'
}

const buildStreamSettingsJSON = () => {
  if (['freedom', 'blackhole', 'wireguard'].includes(form.value.protocol)) {
    return ''
  }
  const stream: any = {
    network: form.value.streamNetwork,
    security: form.value.streamSecurity,
  }

  if (form.value.streamNetwork === 'xhttp') {
    stream.xhttpSettings = {
      path: form.value.xhttpPath || '/',
      mode: form.value.xhttpMode || 'auto',
    }
  } else if (form.value.streamNetwork === 'ws') {
    stream.wsSettings = {
      path: form.value.wsPath || '/',
      headers: form.value.wsHost ? { Host: form.value.wsHost } : undefined,
    }
  } else if (form.value.streamNetwork === 'grpc') {
    stream.grpcSettings = {
      serviceName: form.value.grpcServiceName || 'xray-grpc',
    }
  }

  if (form.value.streamSecurity === 'reality') {
    stream.realitySettings = {
      serverName: form.value.realityServerName || 'www.example.com',
      publicKey: form.value.realityPublicKey || '',
      shortId: form.value.realityShortId || '',
      fingerprint: form.value.realityFingerprint || 'chrome',
    }
  } else if (form.value.streamSecurity === 'tls') {
    stream.tlsSettings = {
      serverName: form.value.tlsServerName || '',
      allowInsecure: form.value.tlsAllowInsecure === true,
      fingerprint: form.value.tlsFingerprint || 'chrome',
    }
  }

  return JSON.stringify(stream, null, 2)
}

const saveOutbound = async () => {
  saving.value = true
  try {
    const payload = {
      tag: form.value.tag,
      protocol: form.value.protocol,
      settingsJson: buildSettingsJSON(),
      streamSettings: buildStreamSettingsJSON(),
    }
    await api.post('/outbounds', payload)
    showFormDrawer.value = false
    toast.success('出站配置已保存成功！')
    await fetchOutbounds()
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteOutbound = async (tag: string) => {
  if (!confirm(`确定删除出站节点 ${tag} 吗？`)) return
  try {
    await api.delete(`/outbounds/${tag}`)
    toast.success('出站节点已成功删除！')
    showInspectorDrawer.value = false
    await fetchOutbounds()
  } catch (err: any) {
    toast.error('删除失败: ' + err)
  }
}

const getProtocolBadgeVariant = (proto: string): 'default' | 'secondary' | 'outline' | 'destructive' => {
  switch (proto?.toLowerCase()) {
    case 'freedom':
      return 'default'
    case 'blackhole':
      return 'destructive'
    case 'wireguard':
      return 'secondary'
    default:
      return 'outline'
  }
}

const getUsageDesc = (ob: any) => {
  switch (ob.protocol?.toLowerCase()) {
    case 'freedom':
      return '直接向目标发起网络连接'
    case 'blackhole':
      return '静默丢弃或拦截阻断连接'
    case 'wireguard':
      return 'Cloudflare WARP 清洁 IP 出站'
    default:
      return '转发至上游代理'
  }
}

const getTargetEndpoint = (ob: any) => {
  if (ob.protocol === 'wireguard') {
    try {
      const s = JSON.parse(ob.settingsJson || '{}')
      return s.peers?.[0]?.endpoint || 'engage.cloudflareclient.com:2408'
    } catch {
      return '-'
    }
  }
  if (['freedom', 'blackhole'].includes(ob.protocol)) {
    return '内置策略'
  }
  try {
    const s = JSON.parse(ob.settingsJson || '{}')
    if (s.vnext?.[0]) return `${s.vnext[0].address}:${s.vnext[0].port}`
    if (s.servers?.[0]) return `${s.servers[0].address}:${s.servers[0].port}`
  } catch {}
  return '-'
}

const getOutboundStreamInfo = (ob: any) => {
  if (!ob.streamSettings) return {}
  try {
    return JSON.parse(ob.streamSettings)
  } catch {
    return {}
  }
}

const getParsedSettings = (ob: any) => {
  if (!ob.settingsJson) return {}
  try {
    return JSON.parse(ob.settingsJson)
  } catch {
    return {}
  }
}

const formatSettingsSummary = (ob: any) => {
  if (!ob.settingsJson || ob.settingsJson === '{}') return '默认系统策略'
  try {
    const s = JSON.parse(ob.settingsJson)
    if (s.domainStrategy) return `解析策略: ${s.domainStrategy}`
    if (s.peers?.length > 0) return `WARP Endpoint: ${s.peers[0].endpoint}`
    if (s.response?.type) return `拦截类型: ${s.response.type}`
    if (s.vnext?.length > 0) return `上游代理: ${s.vnext[0].address}:${s.vnext[0].port}`
    if (s.servers?.length > 0) return `代理服务器: ${s.servers[0].address}:${s.servers[0].port}`
    return JSON.stringify(s)
  } catch (e) {
    return ob.settingsJson
  }
}

const formatJsonPretty = (str: string) => {
  if (!str) return '{}'
  try {
    return JSON.stringify(JSON.parse(str), null, 2)
  } catch {
    return str
  }
}

const checkRouteQuery = () => {
  const editQuery = route.query.edit as string
  if (editQuery && outbounds.value.length > 0) {
    const target = outbounds.value.find((ob) => ob.tag === editQuery)
    if (target) {
      editOutbound(target)
    } else {
      toast.warning('未找到指定的出站节点: ' + editQuery)
    }
  } else if (route.query.action === 'create' || route.query.create) {
    openCreateDrawer()
  }
}

onMounted(async () => {
  await fetchOutbounds()
  checkRouteQuery()
})

watch(
  () => [route.query.edit, route.query.action, route.query.create],
  () => {
    checkRouteQuery()
  }
)
</script>
