<template>
  <Drawer
    v-model="isOpen"
    :title="isEditing ? `编辑出站节点: ${form.tag}` : '添加新出站节点'"
    description="配置将同步回写磁盘 config.json 并平滑重启核心应用"
    width="w-full sm:max-w-xl md:max-w-2xl"
  >
    <form id="outbound-form" @submit.prevent="handleSubmit" class="space-y-4 text-xs">
      <SectionCard title="基本标识与协议">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="出站标识 (Tag)" required hint="节点唯一英文标识，如 warp-out 或 proxy-jp">
            <input
              v-model="form.tag"
              type="text"
              required
              :disabled="isEditing"
              placeholder="例如: warp-out 或 proxy-jp"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-50"
            />
          </FormField>

          <FormField label="出站协议 (Protocol)" required>
            <select
              v-model="form.protocol"
              @change="onOutboundProtocolChange"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="freedom">freedom (直连访问)</option>
              <option value="blackhole">blackhole (黑洞丢弃/拦截)</option>
              <option value="wireguard">wireguard (Cloudflare WARP 出站)</option>
              <option value="dns">dns (内置 DNS 路由分流)</option>
              <option value="vless">vless (上游链式代理)</option>
              <option value="vmess">vmess (上游链式代理)</option>
              <option value="trojan">trojan (上游链式代理)</option>
              <option value="shadowsocks">shadowsocks (上游链式代理)</option>
              <option value="socks">socks 代理出站</option>
              <option value="http">http 代理出站</option>
            </select>
          </FormField>
        </div>
      </SectionCard>

      <!-- Freedom 专属配置 -->
      <SectionCard v-if="form.protocol === 'freedom'" title="Freedom 直连策略">
        <FormField label="域名解析策略 (domainStrategy)" hint="控制客户端域名在本地的解析行为">
          <select
            v-model="form.freedomDomainStrategy"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
          >
            <option value="UseIP">UseIP</option>
            <option value="UseIPv4">UseIPv4 (强制IPv4 - 推荐)</option>
            <option value="UseIPv6">UseIPv6 (强制IPv6)</option>
            <option value="AsIs">AsIs (保持原样)</option>
          </select>
        </FormField>
      </SectionCard>

      <!-- Blackhole 专属配置 -->
      <SectionCard v-else-if="form.protocol === 'blackhole'" title="Blackhole 拦截响应">
        <FormField label="响应类型 (Response Type)" hint="none 为静默丢弃连接，http 为响应 HTTP 403 阻断">
          <select
            v-model="form.blackholeResponse"
            class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
          >
            <option value="none">none (直接静默丢弃)</option>
            <option value="http">http (返回 HTTP 403 阻断页面)</option>
          </select>
        </FormField>
      </SectionCard>

      <!-- WireGuard / WARP 专属配置 -->
      <SectionCard v-else-if="form.protocol === 'wireguard'" title="WireGuard (Cloudflare WARP) 详情">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="Secret Key (私钥)" required>
            <input
              v-model="form.wgSecretKey"
              type="text"
              placeholder="填写 WireGuard Private Key (Base64)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="Peer Public Key (对端公钥)" required>
            <input
              v-model="form.wgPeerPublicKey"
              type="text"
              placeholder="填写 Cloudflare WARP 对端公钥"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="Endpoint (对端服务器)">
            <input
              v-model="form.wgEndpoint"
              type="text"
              placeholder="162.159.192.1:2408"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="本地地址 (Address)">
            <input
              v-model="form.wgAddress"
              type="text"
              placeholder="172.16.0.2/32"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/40">
          <FormField label="MTU (最大传输单元)">
            <input
              v-model.number="form.wgMtu"
              type="number"
              placeholder="1280"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <div class="flex items-center pt-6">
            <label class="flex items-center gap-2 cursor-pointer text-muted-foreground hover:text-foreground">
              <input type="checkbox" v-model="form.wgNoKernelTun" class="rounded bg-neutral-950 border-border text-neutral-200" />
              <span>禁用系统内核 TUN 模块 (NoKernelTun)</span>
            </label>
          </div>
        </div>
      </SectionCard>

      <!-- DNS 专属配置 -->
      <SectionCard v-else-if="form.protocol === 'dns'" title="DNS 路由出站">
        <div class="p-3 rounded bg-neutral-950 border border-border text-muted-foreground text-xs leading-relaxed">
          由 Xray 内置 DNS 模块发起查询出站，无需额外参数。通常与路由规则 (Routing) 配合，将 DNS 流量定向路由至此 Tag (例如 <code class="text-cyan-400 font-mono">dns-out</code>)。
        </div>
      </SectionCard>

      <!-- Proxy outbounds (VLESS / VMess / Trojan / Shadowsocks / Socks / HTTP) -->
      <SectionCard v-else title="上游代理服务器连接信息">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="服务器地址 (Host/Address)" required>
            <input
              v-model="form.proxyHost"
              type="text"
              required
              placeholder="127.0.0.1 或 proxy.example.com"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="服务器端口 (Port)" required>
            <input
              v-model.number="form.proxyPort"
              type="number"
              required
              placeholder="443"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <div v-if="['vless', 'vmess', 'trojan'].includes(form.protocol)">
          <FormField label="UUID / 密码 (Password)" required>
            <input
              v-model="form.proxyPassword"
              type="text"
              required
              placeholder="UUID 或密码"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- VMess 专属安全加密 -->
        <div v-if="form.protocol === 'vmess'" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="VMess 加密 (Security)">
            <select
              v-model="form.vmessSecurity"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="auto">auto (自动匹配)</option>
              <option value="aes-128-gcm">aes-128-gcm</option>
              <option value="chacha20-poly1305">chacha20-poly1305</option>
              <option value="none">none</option>
            </select>
          </FormField>
          <FormField label="AlterID (额外ID)">
            <input
              v-model.number="form.vmessAlterId"
              type="number"
              placeholder="0"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- Shadowsocks 专属密码与加密方式 -->
        <div v-if="form.protocol === 'shadowsocks'" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="SS 密码 (Password)" required>
            <input
              v-model="form.proxyPassword"
              type="text"
              required
              placeholder="Shadowsocks 密码"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="加密算法 (Method)">
            <select
              v-model="form.ssMethod"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="2022-blake3-aes-128-gcm">2022-blake3-aes-128-gcm</option>
              <option value="2022-blake3-aes-256-gcm">2022-blake3-aes-256-gcm</option>
              <option value="aes-256-gcm">aes-256-gcm</option>
              <option value="aes-128-gcm">aes-128-gcm</option>
              <option value="chacha20-ietf-poly1305">chacha20-ietf-poly1305</option>
              <option value="none">none</option>
            </select>
          </FormField>
        </div>

        <!-- Socks / HTTP 专属可选用户名密码 -->
        <div v-if="['socks', 'http'].includes(form.protocol)" class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="认证用户名 (选填)">
            <input
              v-model="form.proxyUsername"
              type="text"
              placeholder="用户名 (如无需认证可留空)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="认证密码 (选填)">
            <input
              v-model="form.proxyPassword"
              type="password"
              placeholder="密码 (如无需认证可留空)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- 传输层与安全层联动 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol)" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border">
          <FormField label="传输协议 (Network)">
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
          </FormField>
          <FormField
            label="安全协议 (Security)"
            :hint="['vmess', 'shadowsocks'].includes(form.protocol) ? '面板策略当前仅开放 VLESS / Trojan 协议搭配 REALITY 伪装出站' : undefined"
          >
            <select
              v-model="form.streamSecurity"
              @change="onOutboundSecurityChange"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option v-if="['tcp', 'xhttp', 'grpc'].includes(form.streamNetwork) && !['vmess', 'shadowsocks'].includes(form.protocol)" value="reality">REALITY</option>
              <option value="tls">TLS</option>
              <option value="none">None</option>
            </select>
          </FormField>
        </div>

        <!-- VLESS 出站流控选项 (XTLS Vision) -->
        <div v-if="form.protocol === 'vless'" class="pt-2 border-t border-border">
          <FormField
            label="流控策略 (Flow / XTLS Vision)"
            :hint="form.streamNetwork !== 'tcp' || !['reality', 'tls'].includes(form.streamSecurity) ? '* Vision 流控仅适用于 TCP + TLS / REALITY 架构。' : undefined"
          >
            <template #extra>
              <span class="text-[11px] text-brand-400 font-mono">XTLS / REALITY 极速流控</span>
            </template>
            <select
              v-model="form.vlessFlow"
              :disabled="form.streamNetwork !== 'tcp' || !['reality', 'tls'].includes(form.streamSecurity)"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring disabled:opacity-40 font-mono"
            >
              <option value="xtls-rprx-vision">xtls-rprx-vision (XTLS Vision 极速流控 - 推荐)</option>
              <option value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option>
              <option value="none">none (无流控 - 适用于 XHTTP / gRPC / WS 等)</option>
            </select>
          </FormField>
        </div>

        <!-- XHTTP 专用参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamNetwork === 'xhttp'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
          <FormField label="XHTTP 路径 (Path)">
            <input
              v-model="form.xhttpPath"
              type="text"
              placeholder="/mbqyfa4grswh5ntz"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="XHTTP 模式 (Mode)">
            <select
              v-model="form.xhttpMode"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="auto">auto</option>
              <option value="stream-up">stream-up</option>
              <option value="stream-one">stream-one</option>
            </select>
          </FormField>
        </div>

        <!-- WS 专用参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamNetwork === 'ws'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
          <FormField label="WebSocket 路径 (Path)">
            <input
              v-model="form.wsPath"
              type="text"
              placeholder="/ws"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="WebSocket Host 头部 (选填)">
            <input
              v-model="form.wsHost"
              type="text"
              placeholder="例如: example.com"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- HTTPUpgrade 专用参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamNetwork === 'httpupgrade'" class="grid grid-cols-1 sm:grid-cols-2 gap-3 pt-2 border-t border-border/60">
          <FormField label="HTTPUpgrade 路径 (Path)">
            <input
              v-model="form.httpupgradePath"
              type="text"
              placeholder="/httpupgrade"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="HTTPUpgrade Host 头部 (选填)">
            <input
              v-model="form.httpupgradeHost"
              type="text"
              placeholder="例如: example.com"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- REALITY 专属连接参数 -->
        <div v-if="['vless', 'trojan'].includes(form.protocol) && form.streamSecurity === 'reality'" class="space-y-3 pt-2 border-t border-border/60 bg-neutral-950/60 p-3 rounded-md">
          <h4 class="text-xs font-bold text-cyan-400 font-mono">REALITY 握手伪装参数</h4>
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="SNI 伪装域名 (ServerName)">
              <input
                v-model="form.realityServerName"
                type="text"
                placeholder="例如: apple.com"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>
            <FormField label="指纹伪装 (Fingerprint)">
              <select
                v-model="form.realityFingerprint"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="chrome">chrome (推荐)</option>
                <option value="firefox">firefox</option>
                <option value="safari">safari</option>
                <option value="ios">ios</option>
                <option value="edge">edge</option>
                <option value="random">random</option>
                <option value="randomized">randomized</option>
              </select>
            </FormField>
          </div>
          <FormField label="SpiderX 爬虫路径匹配 (选填)">
            <input
              v-model="form.realitySpiderX"
              type="text"
              placeholder="例如: /"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="Public Key (对端公钥 / pbk)">
            <input
              v-model="form.realityPublicKey"
              type="text"
              placeholder="例如: FMdWD0uS9lrXUAoMmTP5e2LLD-mk8vO8JTZmAE9vdww"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono text-[11px] focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
          <FormField label="Short ID (短ID / sid)">
            <input
              v-model="form.realityShortId"
              type="text"
              placeholder="0123456789abcdef"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <!-- TLS 专属连接参数 -->
        <div v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol) && form.streamSecurity === 'tls'" class="pt-2 border-t border-border/60 bg-neutral-950/60 p-3 rounded-md space-y-3">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="TLS SNI (ServerName)">
              <input
                v-model="form.tlsServerName"
                type="text"
                placeholder="example.com"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>
            <FormField label="TLS 指纹伪装 (Fingerprint)">
              <select
                v-model="form.tlsFingerprint"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
              >
                <option value="chrome">chrome</option>
                <option value="firefox">firefox</option>
                <option value="safari">safari</option>
                <option value="ios">ios</option>
                <option value="edge">edge</option>
                <option value="random">random</option>
                <option value="randomized">randomized</option>
              </select>
            </FormField>
          </div>
        </div>
      </SectionCard>

      <!-- 连接多路复用 (Mux & XUDP) -->
      <SectionCard v-if="isProxyProtocol" title="连接多路复用 (Mux & XUDP)">
        <!-- 互斥提示：XTLS Vision 与 Mux 互斥 -->
        <div v-if="isVisionFlow" class="p-3 rounded-md bg-amber-500/10 border border-amber-500/30 text-amber-300 text-xs flex flex-col sm:flex-row items-start sm:items-center justify-between gap-3 mb-3">
          <div class="flex items-center gap-2">
            <AlertTriangle class="w-4 h-4 text-amber-400 shrink-0" />
            <span>当前启用了 <strong>XTLS Vision 流控</strong>，与 Mux 连接复用互斥。开启 Mux 需关闭 Vision 流控。</span>
          </div>
          <button
            type="button"
            @click="clearVisionForMux"
            class="px-2.5 py-1 text-[11px] rounded bg-amber-500/20 hover:bg-amber-500/30 text-amber-200 border border-amber-500/40 shrink-0 font-medium transition-colors"
          >
            关闭 Vision 并启用 Mux
          </button>
        </div>

        <div class="flex items-center justify-between">
          <div class="space-y-0.5">
            <span class="font-medium text-foreground text-xs block">启用 Mux 多路复用 (Mux & XUDP)</span>
            <span class="text-[11px] text-muted-foreground block">聚合 TCP 与 UDP 连接至单条物理通道，大幅降低高延迟网络握手开销</span>
          </div>
          <label class="relative inline-flex items-center cursor-pointer">
            <input
              type="checkbox"
              :checked="form.muxEnabled"
              @change="handleMuxToggle(($event.target as HTMLInputElement).checked)"
              class="sr-only peer"
            />
            <div class="w-9 h-5 bg-neutral-800 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-purple-600"></div>
          </label>
        </div>

        <div v-if="form.muxEnabled && !isVisionFlow" class="space-y-3 pt-3 border-t border-border/50 mt-3">
          <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <FormField label="TCP 最大并发连接 (concurrency)" hint="默认 8 (范围 1~128)。填写 -1 仅关闭 TCP 复用">
              <input
                v-model.number="form.muxConcurrency"
                type="number"
                min="-1"
                max="128"
                placeholder="8"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>

            <FormField label="XUDP 最大并发连接 (xudpConcurrency)" hint="默认 16 (范围 1~1024)。填写 -1 关闭 UDP 复用">
              <input
                v-model.number="form.muxXudpConcurrency"
                type="number"
                min="-1"
                max="1024"
                placeholder="16"
                class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
              />
            </FormField>
          </div>

          <FormField label="QUIC / UDP 443 转发策略 (xudpProxyUDP443)" hint="处理对端 443 端口 UDP 流量（主要为 QUIC）的行为">
            <select
              v-model="form.muxXudpProxyUDP443"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="reject">reject (默认拦截 QUIC 回退至 HTTP/2 - 推荐)</option>
              <option value="allow">allow (允许复用 UDP 443 / QUIC)</option>
              <option value="skip">skip (绕过 Mux 直传 UDP 443)</option>
            </select>
          </FormField>
        </div>
      </SectionCard>

      <!-- 高级路由与链式代理 (Relay & Sockopt) -->
      <SectionCard v-if="form.protocol !== 'blackhole'" title="高级路由与链式代理 (Relay & Sockopt)">
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="前置代理出站 Tag (dialerProxy)" hint="经由此出站节点建立前置连接（用于 WARP 落地、前置中转或双重代理链路）">
            <input
              v-model="form.dialerProxy"
              type="text"
              list="dialer-proxy-options"
              placeholder="例如: warp-out 或 proxy-pre"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
            <datalist id="dialer-proxy-options">
              <option v-for="tag in availableDialerTags" :key="tag" :value="tag" />
            </datalist>
          </FormField>

          <FormField label="出口绑定源 IP (sendThrough)" hint="多 IP 主机出口绑定特定网卡源 IP 或 origin">
            <input
              v-model="form.sendThrough"
              type="text"
              placeholder="例如: 1.2.3.4 或 origin"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            />
          </FormField>
        </div>

        <div class="pt-2">
          <FormField label="目标域名解析策略 (targetStrategy)" hint="控制此出站节点连接目标域名时的 DNS 解析行为">
            <select
              v-model="form.targetStrategy"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 py-2 text-foreground focus:outline-none focus:ring-1 focus:ring-ring font-mono"
            >
              <option value="">默认 (AsIs 保持原样)</option>
              <option value="UseIP">UseIP</option>
              <option value="UseIPv4">UseIPv4 (强制优先 IPv4)</option>
              <option value="UseIPv6">UseIPv6 (强制优先 IPv6)</option>
              <option value="UseIPv4v6">UseIPv4v6</option>
              <option value="UseIPv6v4">UseIPv6v4</option>
              <option value="ForceIP">ForceIP</option>
              <option value="ForceIPv4">ForceIPv4</option>
              <option value="ForceIPv6">ForceIPv6</option>
            </select>
          </FormField>
        </div>
      </SectionCard>
    </form>

    <template #footer>
      <Button variant="secondary" size="sm" @click="isOpen = false">
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
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { AlertTriangle } from 'lucide-vue-next'
import { toast } from '../../../utils/toast'
import Drawer from '../../../components/ui/Drawer.vue'
import Button from '../../../components/ui/Button.vue'
import FormField from '../../../components/ui/FormField.vue'
import SectionCard from '../../../components/ui/SectionCard.vue'
import type { OutboundItem, OutboundFormState, OutboundPayload } from '../types'
import {
  createDefaultOutboundForm,
  populateOutboundForm,
  sanitizeOutboundPayload,
} from '../types'

const props = defineProps<{
  modelValue: boolean
  editingOutbound: OutboundItem | null
  outbounds?: OutboundItem[]
  saving: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'save', payload: OutboundPayload): void
}>()

const form = ref<OutboundFormState>(createDefaultOutboundForm())

const isOpen = computed({
  get: () => props.modelValue,
  set: (val: boolean) => emit('update:modelValue', val),
})

const isEditing = computed(() => !!props.editingOutbound)

const isProxyProtocol = computed(() =>
  ['vless', 'vmess', 'trojan', 'shadowsocks', 'socks', 'http'].includes(form.value.protocol)
)

const isVisionFlow = computed(() =>
  form.value.protocol === 'vless' &&
  (form.value.vlessFlow === 'xtls-rprx-vision' || form.value.vlessFlow === 'xtls-rprx-vision-udp443')
)

const availableDialerTags = computed(() => {
  if (!props.outbounds) return []
  return props.outbounds
    .filter((o) => o.tag !== form.value.tag && o.protocol !== 'blackhole')
    .map((o) => o.tag)
})

const handleMuxToggle = (val: boolean) => {
  if (val && isVisionFlow.value) {
    form.value.vlessFlow = 'none'
    form.value.muxEnabled = true
    toast.info('已将 VLESS 流控切换为 none 并启用 Mux')
    return
  }
  form.value.muxEnabled = val
}

const clearVisionForMux = () => {
  form.value.vlessFlow = 'none'
  form.value.muxEnabled = true
  toast.info('已切换为无流控并启用 Mux')
}

// 当用户选择 XTLS Vision 流控时，自动互斥关闭 Mux
watch(
  () => form.value.vlessFlow,
  (newFlow) => {
    if (
      form.value.protocol === 'vless' &&
      (newFlow === 'xtls-rprx-vision' || newFlow === 'xtls-rprx-vision-udp443') &&
      form.value.muxEnabled
    ) {
      form.value.muxEnabled = false
      toast.info('已自动关闭 Mux（与 XTLS Vision 流控互斥）')
    }
  }
)

const onOutboundProtocolChange = () => {
  if (!['vless', 'vmess', 'trojan', 'shadowsocks', 'socks', 'http'].includes(form.value.protocol)) {
    form.value.muxEnabled = false
  }
  if (form.value.protocol === 'blackhole') {
    form.value.dialerProxy = ''
    form.value.sendThrough = ''
    form.value.targetStrategy = ''
  }

  if (['freedom', 'blackhole', 'wireguard', 'socks', 'http', 'dns'].includes(form.value.protocol)) {
    form.value.streamNetwork = 'tcp'
    form.value.streamSecurity = 'none'
    form.value.vlessFlow = ''
  } else if (['vmess', 'shadowsocks'].includes(form.value.protocol)) {
    if (form.value.streamSecurity === 'reality') {
      form.value.streamSecurity = 'none'
    }
    form.value.vlessFlow = ''
  } else if (form.value.protocol === 'trojan') {
    form.value.vlessFlow = ''
  } else if (form.value.protocol === 'vless') {
    if (form.value.streamNetwork === 'tcp' && ['reality', 'tls'].includes(form.value.streamSecurity)) {
      if (!form.value.vlessFlow) form.value.vlessFlow = 'xtls-rprx-vision'
    } else {
      form.value.vlessFlow = 'none'
    }
  }
}

const onOutboundSecurityChange = () => {
  if (form.value.protocol === 'vless') {
    if (form.value.streamNetwork === 'tcp' && ['reality', 'tls'].includes(form.value.streamSecurity)) {
      if (!form.value.vlessFlow) form.value.vlessFlow = 'xtls-rprx-vision'
    } else {
      form.value.vlessFlow = 'none'
    }
  }
}

const onOutboundNetworkChange = () => {
  if (['vmess', 'shadowsocks'].includes(form.value.protocol)) {
    if (form.value.streamSecurity === 'reality') {
      form.value.streamSecurity = 'none'
    }
    return
  }
  if (!['tcp', 'xhttp', 'grpc'].includes(form.value.streamNetwork) && form.value.streamSecurity === 'reality') {
    form.value.streamSecurity = 'tls'
  }
  if (form.value.protocol === 'vless') {
    if (form.value.streamNetwork === 'tcp' && ['reality', 'tls'].includes(form.value.streamSecurity)) {
      if (!form.value.vlessFlow) form.value.vlessFlow = 'xtls-rprx-vision'
    } else {
      form.value.vlessFlow = 'none'
    }
  }
}

watch(
  () => [props.modelValue, props.editingOutbound] as const,
  ([open, ob]) => {
    if (open) {
      if (ob) {
        form.value = populateOutboundForm(ob)
      } else {
        form.value = createDefaultOutboundForm()
      }
    }
  },
  { immediate: true }
)

const handleSubmit = () => {
  if (['vless', 'trojan'].includes(form.value.protocol) && form.value.streamSecurity === 'reality') {
    if (!form.value.realityServerName?.trim()) {
      toast.error('请填写 REALITY SNI 伪装域名 (ServerName)')
      return
    }
  }
  const payload = sanitizeOutboundPayload(form.value)
  emit('save', payload)
}
</script>
