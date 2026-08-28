<template>
  <div class="space-y-6">
    <!-- Header -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div>
        <div class="flex items-center gap-2.5">
          <h1 class="text-2xl font-extrabold text-white tracking-tight">入站节点管理 (Inbounds)</h1>
          <span class="px-2.5 py-0.5 rounded-full text-[11px] font-semibold bg-amber-500/15 text-amber-400 border border-amber-500/30 flex items-center gap-1">
            <span>🟠 修改配置自动重启核心 (重新绑定系统端口)</span>
          </span>
        </div>
        <p class="text-xs text-gray-400 mt-0.5">全可视化分层配置 Xray 入站代理节点，严格遵循官方传输组合兼容矩阵，保存后自动落盘并重启核心生效</p>
      </div>
      <button
        @click="openCreateModal"
        class="px-4 py-2 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white text-xs font-semibold rounded-xl transition-all shadow-lg shadow-brand-500/25 flex items-center gap-1.5"
      >
        <Plus class="w-4 h-4" />
        <span>添加新节点</span>
      </button>
    </div>

    <!-- Inbounds List Grid -->
    <div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
      <div
        v-for="inb in inbounds"
        :key="inb.id"
        class="glass-panel p-5 rounded-2xl border border-gray-800/80 hover:border-brand-500/40 transition-all flex flex-col justify-between"
      >
        <div>
          <div class="flex items-center justify-between mb-3">
            <span class="text-base font-bold text-white tracking-tight font-mono">{{ inb.tag }}</span>
            <div class="flex items-center gap-1.5">
              <span class="px-2 py-0.5 rounded-md text-[11px] font-mono font-bold uppercase" :class="protocolBadgeColor(inb.protocol)">
                {{ inb.protocol }}
              </span>
              <span class="px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-gray-800 text-cyan-400 border border-gray-700">
                {{ getStreamNetwork(inb) }}
              </span>
            </div>
          </div>

          <div class="space-y-2 text-xs text-gray-400">
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>监听端口</span>
              <div class="flex items-center gap-1.5">
                <span class="text-brand-400 font-mono font-semibold">{{ inb.port }}</span>
                <span v-if="inb.port !== 443 && isReality(inb)" class="text-amber-400 font-semibold text-[10px]" title="非443端口Reality存在阻断风险">⚠️ 非443</span>
              </div>
            </div>
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>安全协议</span>
              <span class="text-gray-200 font-mono font-semibold uppercase">{{ getSecurityType(inb) }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>监听地址</span>
              <span class="text-gray-200 font-mono">{{ inb.listen || '0.0.0.0' }}</span>
            </div>
            <div class="flex justify-between py-1 border-b border-gray-800/60">
              <span>累计上下行</span>
              <span class="text-gray-200 font-mono">{{ formatBytes(inb.upBytes + inb.downBytes) }}</span>
            </div>
            <div class="flex justify-between py-1">
              <span>运行状态</span>
              <span class="text-emerald-400 font-semibold flex items-center gap-1">
                <span class="w-1.5 h-1.5 rounded-full bg-emerald-400"></span> 已启用且与文件同步
              </span>
            </div>
          </div>
        </div>

        <div class="mt-4 pt-3 border-t border-gray-800/60 flex items-center justify-between">
          <button
            @click="editInbound(inb)"
            class="text-xs text-brand-400 hover:text-brand-300 font-medium transition-colors"
          >
            编辑参数
          </button>
          <button
            @click="deleteInbound(inb.id)"
            class="text-xs text-red-400 hover:text-red-300 transition-colors"
          >
            删除
          </button>
        </div>
      </div>
    </div>

    <!-- Empty state -->
    <div v-if="!inbounds.length" class="glass-panel p-12 text-center rounded-2xl">
      <Radio class="w-12 h-12 mx-auto text-gray-600 mb-3" />
      <h3 class="text-sm font-semibold text-gray-300">暂无入站节点</h3>
      <p class="text-xs text-gray-500 mt-1">点击右上角“添加新节点”开始配置</p>
    </div>

    <!-- Comprehensive Cascading Inbound Modal -->
    <div v-if="showModal" class="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/75 backdrop-blur-sm overflow-y-auto">
      <div class="glass-panel w-full max-w-2xl p-6 sm:p-8 rounded-3xl border border-gray-800 shadow-2xl space-y-5 my-8 max-h-[90vh] overflow-y-auto">
        <div class="flex items-center justify-between pb-2 border-b border-gray-800">
          <div>
            <h2 class="text-lg font-bold text-white">{{ isEditing ? '编辑入站节点' : '分层添加新节点' }}</h2>
            <p class="text-xs text-gray-400 mt-0.5">参数将自动校验官方兼容性矩阵，热注入 Xray 并回写 config.json</p>
          </div>
          <button @click="showModal = false" class="text-gray-400 hover:text-white text-lg">✕</button>
        </div>

        <form @submit.prevent="saveInbound" class="space-y-5 text-xs">
          <!-- 1. 基础设置 -->
          <div class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800/80">
            <h3 class="font-bold text-brand-400 uppercase tracking-wider text-[11px] flex items-center gap-1.5">
              <span>① 基础网络与端口设置</span>
            </h3>

            <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label class="block text-gray-300 mb-1 font-medium">节点标识 (Tag)</label>
                <input
                  v-model="form.tag"
                  type="text"
                  required
                  placeholder="vless-reality"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>

              <div>
                <label class="block text-gray-300 mb-1 font-medium">监听 IP</label>
                <input
                  v-model="form.listen"
                  type="text"
                  placeholder="0.0.0.0"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>

              <div>
                <label class="block text-gray-300 mb-1 font-medium">监听端口</label>
                <input
                  v-model.number="form.port"
                  type="number"
                  required
                  placeholder="443"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>

            <!-- 非 443 端口安全告警卡片 -->
            <div
              v-if="form.port !== 443 && form.security === 'reality'"
              class="p-3 rounded-xl bg-amber-500/10 border border-amber-500/30 text-amber-300 text-xs flex items-start gap-2.5"
            >
              <AlertTriangle class="w-4 h-4 text-amber-400 shrink-0 mt-0.5" />
              <div>
                <p class="font-bold">⚠️ 安全风险警报 (Non-443 Port Alert)</p>
                <p class="mt-0.5 text-[11px] opacity-90">
                  当前节点配置了 Reality 伪装，但监听端口为 <b>{{ form.port }}</b>（非标准 443 端口）。监听非 443 端口极易被 GFW 的 SNI 白名单与主动探测特征识别阻断！强烈建议将监听端口设为 <b>443</b>。
                </p>
              </div>
            </div>
          </div>

          <!-- 2. 协议与传输层设置（严格级联互斥） -->
          <div class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800/80">
            <h3 class="font-bold text-brand-400 uppercase tracking-wider text-[11px] flex items-center gap-1.5">
              <span>② 入站协议与传输层 (智能兼容联动)</span>
            </h3>

            <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
              <div>
                <label class="block text-gray-300 mb-1 font-medium">入站协议 (Protocol)</label>
                <select
                  v-model="form.protocol"
                  @change="onProtocolChange"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500 font-medium"
                >
                  <option value="vless">VLESS (推荐)</option>
                  <option value="vmess">VMess</option>
                  <option value="trojan">Trojan</option>
                  <option value="shadowsocks">Shadowsocks</option>
                </select>
              </div>

              <div>
                <label class="block text-gray-300 mb-1 font-medium">传输协议 (Network)</label>
                <select
                  v-model="form.network"
                  @change="onNetworkChange"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500 font-mono"
                >
                  <option value="tcp">TCP (RAW 推荐)</option>
                  <option value="xhttp">XHTTP (SplitHTTP 推荐)</option>
                  <option value="grpc">gRPC</option>
                  <option value="ws">WebSocket</option>
                  <option value="httpupgrade">HTTPUpgrade</option>
                  <option value="mkcp">mKCP (UDP)</option>
                </select>
              </div>

              <div>
                <label class="block text-gray-300 mb-1 font-medium">安全协议 (Security)</label>
                <select
                  v-model="form.security"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500 font-mono"
                >
                  <option v-if="isRealitySupported" value="reality">REALITY (推荐)</option>
                  <option value="tls">TLS</option>
                  <option value="none">None (无加密)</option>
                </select>
              </div>
            </div>

            <!-- 不兼容 Reality 自动限制提示 -->
            <div v-if="!isRealitySupported" class="p-2 rounded-xl bg-blue-500/10 border border-blue-500/20 text-blue-300 text-[11px]">
              ℹ️ 官方规范说明：当前传输协议 (<code>{{ form.network }}</code>) 不支持 REALITY，安全协议仅支持 TLS 或 None。
            </div>

            <!-- VLESS/Trojan None 安全提示 -->
            <div v-if="form.security === 'none' && (form.protocol === 'vless' || form.protocol === 'trojan')" class="p-2.5 rounded-xl bg-amber-500/10 border border-amber-500/20 text-amber-300 text-[11px]">
              ⚠️ 官方安全提示: VLESS/Trojan 在无安全层 (None) 时仅允许连接私网，公网直接使用存在阻断风险，请确保处于 CDN 或前置反代后。
            </div>

            <!-- VLESS 流控选项（仅在 TCP/RAW 下支持 Vision） -->
            <div v-if="form.protocol === 'vless'" class="pt-2 border-t border-gray-800">
              <label class="block text-gray-400 mb-1 font-medium">VLESS 流控模式 (Flow)</label>
              <select
                v-model="form.vlessFlow"
                :disabled="form.network !== 'tcp'"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500 disabled:opacity-40 font-mono"
              >
                <option v-if="form.network === 'tcp'" value="xtls-rprx-vision">xtls-rprx-vision (XTLS Vision 极速流控 - 推荐)</option>
                <option v-if="form.network === 'tcp'" value="xtls-rprx-vision-udp443">xtls-rprx-vision-udp443</option>
                <option value="">none (无流控 - 非 TCP 传输强制留空)</option>
              </select>
              <p v-if="form.network !== 'tcp'" class="text-[11px] text-gray-500 mt-1">
                * 官方速查表规定：Vision 流控仅适用于 TCP/RAW 载荷，当前 {{ form.network }} 传输层已自动禁用流控。
              </p>
            </div>

            <!-- XHTTP 专属配置项 -->
            <div v-if="form.network === 'xhttp'" class="pt-2 border-t border-gray-800 space-y-3">
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label class="block text-gray-400 mb-1">XHTTP 路径 (Path)</label>
                  <input
                    v-model="form.xhttpPath"
                    type="text"
                    placeholder="/mbqyfa4grswh5ntz"
                    class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                  />
                </div>
                <div>
                  <label class="block text-gray-400 mb-1">XHTTP 模式 (Mode)</label>
                  <select
                    v-model="form.xhttpMode"
                    class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500 font-mono"
                  >
                    <option value="auto">auto (自动)</option>
                    <option value="stream-up">stream-up</option>
                    <option value="packet-up">packet-up</option>
                  </select>
                </div>
              </div>
            </div>

            <!-- WS 专属配置项 -->
            <div v-if="form.network === 'ws'" class="pt-2 border-t border-gray-800">
              <label class="block text-gray-400 mb-1">WebSocket 路径 (Path)</label>
              <input
                v-model="form.wsPath"
                type="text"
                placeholder="/ws"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>

            <!-- gRPC 专属配置项 -->
            <div v-if="form.network === 'grpc'" class="pt-2 border-t border-gray-800">
              <label class="block text-gray-400 mb-1">gRPC 服务名 (ServiceName)</label>
              <input
                v-model="form.grpcService"
                type="text"
                placeholder="xray-grpc"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <!-- 3. 安全协议专属设置 (Reality / TLS) -->
          <div v-if="form.security === 'reality'" class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800/80">
            <div class="flex items-center justify-between">
              <h3 class="font-bold text-cyan-400 uppercase tracking-wider text-[11px] flex items-center gap-1.5">
                <span>③ REALITY 伪装与安全设置</span>
              </h3>
              <button
                type="button"
                @click="generateRealityKey"
                class="px-2.5 py-1 bg-cyan-600/20 hover:bg-cyan-600/30 text-cyan-400 rounded-lg text-[11px] font-semibold border border-cyan-500/30 flex items-center gap-1"
              >
                <Key class="w-3 h-3" />
                <span>⚡ 一键生成 x25519 密钥对与 ShortID</span>
              </button>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-gray-300 mb-1 font-medium">目标伪装网站 (Target)</label>
                <input
                  v-model="form.realityTarget"
                  type="text"
                  placeholder="www.titech.ac.jp:443"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
              <div>
                <label class="block text-gray-300 mb-1 font-medium">SNI 域名列表 (多个用逗号隔开)</label>
                <input
                  v-model="form.realityServerNames"
                  type="text"
                  placeholder="www.titech.ac.jp"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
            </div>

            <div>
              <label class="block text-gray-300 mb-1 font-medium">Private Key (私钥)</label>
              <input
                v-model="form.realityPrivateKey"
                type="text"
                required
                placeholder="自动生成或手动填入 base64 私钥"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
              />
            </div>

            <div v-if="form.realityPublicKey" class="p-2.5 rounded-xl bg-gray-900 border border-gray-800 text-[11px] font-mono text-gray-400">
              <span class="text-cyan-400 font-bold">Public Key (对应公钥，分发用):</span> {{ form.realityPublicKey }}
            </div>

            <div>
              <label class="block text-gray-300 mb-1 font-medium">Short IDs (短ID，多个用逗号隔开)</label>
              <input
                v-model="form.realityShortIds"
                type="text"
                placeholder="0123456789abcdef"
                class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
              />
            </div>
          </div>

          <div v-else-if="form.security === 'tls'" class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800/80">
            <h3 class="font-bold text-purple-400 uppercase tracking-wider text-[11px]">
              ③ TLS 证书配置
            </h3>
            <div class="space-y-3">
              <div>
                <label class="block text-gray-300 mb-1">SNI 域名 (ServerName)</label>
                <input
                  v-model="form.tlsServerName"
                  type="text"
                  placeholder="yourdomain.com"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
              <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div>
                  <label class="block text-gray-300 mb-1">证书路径 (Cert File)</label>
                  <input
                    v-model="form.tlsCertFile"
                    type="text"
                    placeholder="/etc/ssl/cert.pem"
                    class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
                  />
                </div>
                <div>
                  <label class="block text-gray-300 mb-1">密钥路径 (Key File)</label>
                  <input
                    v-model="form.tlsKeyFile"
                    type="text"
                    placeholder="/etc/ssl/key.pem"
                    class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono text-[11px] focus:outline-none focus:border-brand-500"
                  />
                </div>
              </div>
            </div>
          </div>

          <!-- 4. 回落分流设置 (Fallbacks) -->
          <div class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800/80">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="font-bold text-gray-200 text-xs">④ 网站回落伪装 (Fallbacks 分流)</h3>
                <p class="text-[11px] text-gray-500">非代理流量自动无缝回落到本地 Web 服务或指定端口</p>
              </div>
              <label class="relative inline-flex items-center cursor-pointer">
                <input type="checkbox" v-model="form.fallbacksEnabled" class="sr-only peer">
                <div class="w-9 h-5 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-brand-600"></div>
              </label>
            </div>

            <div v-if="form.fallbacksEnabled" class="pt-2 border-t border-gray-800 grid grid-cols-1 sm:grid-cols-2 gap-3">
              <div>
                <label class="block text-gray-400 mb-1">回落目标地址/端口 (dest)</label>
                <input
                  v-model="form.fallbackDest"
                  type="text"
                  placeholder="80 或 127.0.0.1:80"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white font-mono focus:outline-none focus:border-brand-500"
                />
              </div>
              <div>
                <label class="block text-gray-400 mb-1">PROXY Protocol (xver)</label>
                <select
                  v-model.number="form.fallbackXver"
                  class="w-full bg-gray-900 border border-gray-700 rounded-xl px-3 py-2 text-white focus:outline-none focus:border-brand-500"
                >
                  <option :value="0">0 (关闭 PROXY 协议)</option>
                  <option :value="1">1 (PROXY protocol v1)</option>
                  <option :value="2">2 (PROXY protocol v2)</option>
                </select>
              </div>
            </div>
          </div>

          <!-- 5. 流量嗅探 (Sniffing) -->
          <div class="space-y-3 bg-gray-900/50 p-4 rounded-2xl border border-gray-800/80">
            <div class="flex items-center justify-between">
              <div>
                <h3 class="font-bold text-gray-200 text-xs">⑤ 流量探测与域名嗅探 (Sniffing)</h3>
                <p class="text-[11px] text-gray-500">自动探测连接真实域名并执行分流规则</p>
              </div>
              <label class="relative inline-flex items-center cursor-pointer">
                <input type="checkbox" v-model="form.sniffingEnabled" class="sr-only peer">
                <div class="w-9 h-5 bg-gray-700 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-brand-600"></div>
              </label>
            </div>

            <div v-if="form.sniffingEnabled" class="pt-2 border-t border-gray-800 flex items-center gap-4 text-gray-400 text-[11px]">
              <label class="flex items-center gap-1.5 cursor-pointer">
                <input type="checkbox" v-model="form.sniffingRouteOnly" class="rounded bg-gray-800 border-gray-700 text-brand-600" />
                <span>routeOnly (仅用于路由匹配，不篡改客户端真实目标地址 - 推荐)</span>
              </label>
            </div>
          </div>

          <!-- Modal Action Buttons -->
          <div class="flex justify-end gap-3 pt-2 border-t border-gray-800">
            <button
              type="button"
              @click="showModal = false"
              class="px-5 py-2.5 rounded-xl bg-gray-800 hover:bg-gray-700 text-gray-300 font-medium text-xs transition-colors"
            >
              取消
            </button>
            <button
              type="submit"
              :disabled="saving"
              class="px-5 py-2.5 bg-gradient-to-r from-brand-600 to-indigo-600 hover:from-brand-500 hover:to-indigo-500 text-white font-semibold text-xs rounded-xl transition-all shadow-lg shadow-brand-500/25 disabled:opacity-50"
            >
              <span>{{ saving ? '保存中...' : '保存并应用' }}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus, Radio, Key, AlertTriangle } from 'lucide-vue-next'
import api from '../api'

const inbounds = ref<any[]>([])
const showModal = ref(false)
const isEditing = ref(false)
const saving = ref(false)

const form = ref<any>({
  id: 0,
  tag: 'vless-reality',
  listen: '0.0.0.0',
  port: 443,
  protocol: 'vless',
  vlessFlow: 'xtls-rprx-vision',
  network: 'tcp',
  security: 'reality',

  // XHTTP
  xhttpPath: '/mbqyfa4grswh5ntz',
  xhttpMode: 'auto',

  // WS
  wsPath: '/ws',

  // gRPC
  grpcService: 'xray-grpc',

  // Reality
  realityTarget: 'www.titech.ac.jp:443',
  realityServerNames: 'www.titech.ac.jp',
  realityPrivateKey: 'OCiaG7JluOeRDE9IIuqPleHWArqqmnKJ_rKTxtjo7mc',
  realityPublicKey: '',
  realityShortIds: '0123456789abcdef',

  // TLS
  tlsServerName: '',
  tlsCertFile: '',
  tlsKeyFile: '',

  // Fallbacks
  fallbacksEnabled: false,
  fallbackDest: '80',
  fallbackXver: 0,

  // Sniffing
  sniffingEnabled: true,
  sniffingRouteOnly: true,
})

// 官方传输组合矩阵判定：只有 TCP, XHTTP, gRPC 支持 REALITY
const isRealitySupported = computed(() => {
  return ['tcp', 'xhttp', 'grpc'].includes(form.value.network)
})

const onNetworkChange = () => {
  // 如果切换到不支持 Reality 的网络协议，自动纠错为 TLS
  if (!isRealitySupported.value && form.value.security === 'reality') {
    form.value.security = 'tls'
  }
  // Vision 流控仅支持 TCP/RAW，切换到非 TCP 时自动清空
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

const fetchInbounds = async () => {
  try {
    inbounds.value = await api.get('/inbounds')
  } catch (err) {
    console.error(err)
  }
}

const openCreateModal = () => {
  isEditing.value = false
  form.value = {
    id: 0,
    tag: `vless-tcp`,
    listen: '0.0.0.0',
    port: 443,
    protocol: 'vless',
    vlessFlow: 'xtls-rprx-vision',
    network: 'tcp',
    security: 'reality',
    xhttpPath: '/' + Math.random().toString(36).substring(2, 12),
    xhttpMode: 'auto',
    wsPath: '/ws',
    grpcService: 'xray-grpc',
    realityTarget: 'www.titech.ac.jp:443',
    realityServerNames: 'www.titech.ac.jp',
    realityPrivateKey: '',
    realityPublicKey: '',
    realityShortIds: '0123456789abcdef',
    tlsServerName: '',
    tlsCertFile: '',
    tlsKeyFile: '',
    fallbacksEnabled: false,
    fallbackDest: '80',
    fallbackXver: 0,
    sniffingEnabled: true,
    sniffingRouteOnly: true,
  }
  generateRealityKey()
  showModal.value = true
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
  form.value.protocol = inb.protocol

  // 解析 settings
  let settings: any = {}
  try {
    settings = JSON.parse(inb.settingsJson || '{}')
  } catch (e) {}

  if (settings.fallbacks?.length > 0) {
    form.value.fallbacksEnabled = true
    form.value.fallbackDest = settings.fallbacks[0].dest || '80'
    form.value.fallbackXver = settings.fallbacks[0].xver || 0
  } else {
    form.value.fallbacksEnabled = false
  }

  // 解析 streamSettings
  let stream: any = {}
  try {
    stream = JSON.parse(inb.streamSettings || '{}')
  } catch (e) {}

  form.value.network = stream.network || 'tcp'
  form.value.security = stream.security || 'none'

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
    form.value.realityTarget = stream.realitySettings.target || ''
    form.value.realityServerNames = (stream.realitySettings.serverNames || []).join(', ')
    form.value.realityPrivateKey = stream.realitySettings.privateKey || ''
    form.value.realityShortIds = (stream.realitySettings.shortIds || []).join(', ')
  }
  if (stream.tlsSettings) {
    form.value.tlsServerName = stream.tlsSettings.serverName || ''
    if (stream.tlsSettings.certificates?.length > 0) {
      form.value.tlsCertFile = stream.tlsSettings.certificates[0].certificateFile || ''
      form.value.tlsKeyFile = stream.tlsSettings.certificates[0].keyFile || ''
    }
  }

  // 解析 sniffing
  let sniff: any = {}
  try {
    sniff = JSON.parse(inb.sniffingJson || '{}')
  } catch (e) {}
  form.value.sniffingEnabled = sniff.enabled !== false
  form.value.sniffingRouteOnly = sniff.routeOnly === true

  showModal.value = true
}

const buildSettingsJSON = (existingJSON?: string) => {
  let settings: any = {}
  try {
    settings = JSON.parse(existingJSON || '{}')
  } catch (e) {}

  if (!settings.clients) {
    settings.clients = []
  }
  if (form.value.protocol === 'vless') {
    settings.decryption = 'none'
  }

  if (form.value.fallbacksEnabled && form.value.fallbackDest) {
    settings.fallbacks = [
      {
        dest: form.value.fallbackDest,
        xver: form.value.fallbackXver || 0,
      },
    ]
  } else {
    delete settings.fallbacks
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
      target: form.value.realityTarget || 'www.titech.ac.jp:443',
      serverNames: sNames.length > 0 ? sNames : ['www.titech.ac.jp'],
      privateKey: form.value.realityPrivateKey,
      shortIds: sIds,
    }
  } else if (form.value.security === 'tls') {
    stream.tlsSettings = {
      serverName: form.value.tlsServerName,
      certificates: [
        {
          certificateFile: form.value.tlsCertFile,
          keyFile: form.value.tlsKeyFile,
        },
      ],
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

    showModal.value = false
    await fetchInbounds()
  } catch (err: any) {
    alert('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

const deleteInbound = async (id: number) => {
  if (!confirm('确定删除该入站节点吗？')) return
  try {
    await api.delete(`/inbounds/${id}`)
    await fetchInbounds()
  } catch (err: any) {
    alert('删除失败: ' + err)
  }
}

const getStreamNetwork = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.network || 'tcp'
  } catch (e) {
    return 'tcp'
  }
}

const getSecurityType = (inb: any) => {
  try {
    const s = JSON.parse(inb.streamSettings || '{}')
    return s.security || 'none'
  } catch (e) {
    return 'none'
  }
}

const isReality = (inb: any) => {
  return getSecurityType(inb) === 'reality'
}

const protocolBadgeColor = (proto: string) => {
  switch (proto?.toLowerCase()) {
    case 'vless':
      return 'bg-brand-500/15 text-brand-300 border border-brand-500/20'
    case 'vmess':
      return 'bg-purple-500/15 text-purple-300 border border-purple-500/20'
    case 'trojan':
      return 'bg-cyan-500/15 text-cyan-300 border border-cyan-500/20'
    default:
      return 'bg-gray-700 text-gray-300'
  }
}

const formatBytes = (bytes: number) => {
  if (!bytes || bytes <= 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(2) + ' ' + sizes[i]
}

onMounted(() => {
  fetchInbounds()
})
</script>
