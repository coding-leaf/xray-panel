<template>
  <Drawer
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    :title="isEditing ? `编辑入站: ${form.tag}` : '添加新入站节点'"
    :description="isEditing ? '修改入站端口、传输层安全与关联授权用户' : '分层配置 Xray 协议、网络传输层与 Reality 密钥'"
    width="w-full sm:max-w-xl md:max-w-2xl lg:max-w-3xl"
  >
    <form id="inbound-form" @submit.prevent="saveInbound" class="space-y-4 text-xs">
      <!-- 1. 基础网络与端口设置 -->
      <SectionCard title="① 基础网络与端口设置">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <FormField label="节点标识 (Tag)" required>
            <Input
              v-model="form.tag"
              type="text"
              required
              placeholder="vless-reality"
            />
          </FormField>

          <FormField label="监听 IP">
            <Input
              v-model="form.listen"
              type="text"
              placeholder="0.0.0.0"
            />
          </FormField>

          <FormField label="内部监听端口 (Port)" required>
            <Input
              v-model.number="form.port"
              type="number"
              required
              placeholder="443"
            />
          </FormField>

          <div>
            <FormField label="外部公网端口 (External Port)" hint="前置 Nginx 反代或端口映射端口">
              <Input
                v-model.number="form.externalPort"
                type="number"
                placeholder="443 (默认443)"
              />
            </FormField>
          </div>

          <div class="sm:col-span-2">
            <FormField label="外部域名/IP (External Host)" hint="客户端订阅下发的主机地址">
              <Input
                v-model="form.externalHost"
                type="text"
                placeholder="留空则继承全局节点公网域名"
              />
            </FormField>
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
      </SectionCard>

      <!-- 2. 入站协议与传输层 -->
      <SectionCard title="② 协议与传输层 (定义节点级流控)">
        <div class="grid grid-cols-1 sm:grid-cols-3 gap-3">
          <FormField label="入站协议 (Protocol)">
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
          </FormField>

          <FormField label="传输协议 (Network)">
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
          </FormField>

          <FormField label="安全协议 (Security)">
            <select
              v-model="form.security"
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring"
            >
              <option v-if="isRealitySupported" value="reality">REALITY (推荐)</option>
              <option value="tls">TLS</option>
              <option value="none">None (无加密)</option>
            </select>
          </FormField>
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
          <FormField label="XHTTP 路径 (Path)">
            <Input v-model="form.xhttpPath" placeholder="/split" />
          </FormField>
          <FormField label="XHTTP 模式">
            <select v-model="form.xhttpMode" class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring">
              <option value="auto">auto (自动)</option>
              <option value="stream-up">stream-up</option>
              <option value="packet-up">packet-up</option>
            </select>
          </FormField>
        </div>

        <!-- WS options -->
        <div v-if="form.network === 'ws'" class="pt-2 border-t border-border/40">
          <FormField label="WebSocket 路径">
            <Input v-model="form.wsPath" placeholder="/ws" />
          </FormField>
        </div>

        <!-- gRPC options -->
        <div v-if="form.network === 'grpc'" class="pt-2 border-t border-border/40">
          <FormField label="gRPC 服务名">
            <Input v-model="form.grpcService" placeholder="xray-grpc" />
          </FormField>
        </div>
      </SectionCard>

      <!-- 3. REALITY 伪装与安全配置 -->
      <SectionCard
        v-if="!['socks', 'http', 'dokodemo-door'].includes(form.protocol) && form.security === 'reality'"
        title="③ REALITY 伪装与密钥"
      >
        <template #actions>
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
        </template>

        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="目标伪装网站 (Target)">
            <Input v-model="form.realityTarget" placeholder="www.example.com:443" />
          </FormField>
          <FormField label="SNI 域名列表 (逗号隔开)">
            <Input v-model="form.realityServerNames" placeholder="www.example.com" />
          </FormField>
        </div>

        <FormField label="Private Key (私钥)">
          <Input v-model="form.realityPrivateKey" placeholder="base64 私钥" />
        </FormField>

        <div v-if="form.realityPublicKey" class="p-2.5 rounded bg-neutral-950 border border-border text-[11px] font-mono text-muted-foreground flex items-center justify-between">
          <span class="truncate pr-2"><b>Public Key:</b> <span class="text-foreground">{{ form.realityPublicKey }}</span></span>
          <Button type="button" variant="ghost" size="sm" class="h-6 px-2 text-[10px]" @click="copyText(form.realityPublicKey)">
            复制公钥
          </Button>
        </div>

        <FormField label="Short IDs (多个逗号隔开)">
          <Input v-model="form.realityShortIds" placeholder="0123456789abcdef" />
        </FormField>
      </SectionCard>

      <!-- TLS 配置 -->
      <SectionCard
        v-else-if="!['socks', 'http', 'dokodemo-door'].includes(form.protocol) && form.security === 'tls'"
        title="③ TLS 证书配置"
      >
        <FormField label="SNI 域名 (ServerName)">
          <Input v-model="form.tlsServerName" placeholder="yourdomain.com" />
        </FormField>
        <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <FormField label="证书文件路径 (Cert File)">
            <Input v-model="form.tlsCertFile" placeholder="/etc/ssl/cert.pem" />
          </FormField>
          <FormField label="私钥文件路径 (Key File)">
            <Input v-model="form.tlsKeyFile" placeholder="/etc/ssl/key.pem" />
          </FormField>
        </div>
      </SectionCard>

      <!-- 4. 回落与嗅探设置 -->
      <SectionCard title="④ 回落伪装与流量探测 (Fallbacks & Sniffing)">
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
          <FormField label="回落目标地址/端口">
            <Input v-model="form.fallbackDest" placeholder="80 或 127.0.0.1:80" />
          </FormField>
          <FormField label="PROXY Protocol (xver)">
            <select v-model.number="form.fallbackXver" class="w-full bg-neutral-950 border border-border rounded-md px-3 h-9 text-foreground font-mono focus:outline-none focus:ring-1 focus:ring-ring">
              <option :value="0">0 (关闭)</option>
              <option :value="1">1 (v1)</option>
              <option :value="2">2 (v2)</option>
            </select>
          </FormField>
        </div>
      </SectionCard>

      <!-- 5. 分流订阅线路 (Sub-Routes) -->
      <SectionCard v-if="form.protocol === 'vless'">
        <template #header>
          <div>
            <span>⑤ 分流订阅线路 (Sub-Routes)</span>
            <p class="text-[10px] text-muted-foreground mt-0.5 normal-case tracking-normal font-normal">
              所有线路共用该入站的单端口与 Reality 密钥，订阅导出多个节点
            </p>
          </div>
        </template>
        <template #actions>
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
        </template>

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
                    {{ isSubRouteAllOpen(sr) ? '全员开放' : `已选 ${sr.allowedUsers?.length || 0} 人` }}
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

                    <div
                      v-if="usersList.length > 0 && filteredUsersForSubRoute.length === 0"
                      class="py-3 text-center text-muted-foreground font-mono text-[11px]"
                    >
                      无匹配用户
                    </div>

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
      </SectionCard>

      <!-- 6. 关联授权用户 -->
      <SectionCard
        v-if="['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.protocol)"
        title="⑥ 关联授权用户 (双向绑定)"
      >
        <template #actions>
          <button
            type="button"
            @click="toggleSelectAllUsers"
            class="text-neutral-300 hover:text-white text-[11px] font-mono underline"
          >
            {{ isAllUsersSelected ? '取消全选' : '全选所有用户' }}
          </button>
        </template>

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
      </SectionCard>
    </form>

    <template #footer>
      <Button
        type="button"
        variant="outline"
        size="sm"
        @click="emit('update:modelValue', false)"
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
</template>

<script setup lang="ts">
import { ref, computed, watch, nextTick, onMounted, onUnmounted } from 'vue'
import {
  AlertTriangle,
  Key,
  Plus,
  Search,
  ChevronDown,
  Check,
  Trash2,
} from 'lucide-vue-next'
import Drawer from '../../../components/ui/Drawer.vue'
import Button from '../../../components/ui/Button.vue'
import Input from '../../../components/ui/Input.vue'
import FormField from '../../../components/ui/FormField.vue'
import SectionCard from '../../../components/ui/SectionCard.vue'
import { toast } from '../../../utils/toast'
import { copyText } from '../../../utils/clipboard'
import api from '../../../api'
import {
  type InboundItem,
  type InboundFormData,
  type SubRouteItem,
  getDefaultInboundFormData,
  buildInboundPayload,
} from '../types'

const props = defineProps<{
  modelValue: boolean
  isEditing: boolean
  initialData?: InboundItem | null
  usersList: any[]
  availableOutbounds: string[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'saved'): void
}>()

const saving = ref(false)
const form = ref<InboundFormData>(getDefaultInboundFormData([]))

const activeSubRoutePopoverIndex = ref<number | null>(null)
const subRouteUserSearch = ref('')
const userSearchInputRef = ref<HTMLInputElement | null>(null)

const isRealitySupported = computed(() => {
  return ['tcp', 'xhttp', 'grpc'].includes(form.value.network)
})

const isAllUsersSelected = computed(() => {
  if (!props.usersList.length) return false
  return form.value.selectedUserEmails.length === props.usersList.length
})

const filteredUsersForSubRoute = computed(() => {
  const query = subRouteUserSearch.value.trim().toLowerCase()
  if (!query) return props.usersList
  return props.usersList.filter((u: any) =>
    (u.email || '').toLowerCase().includes(query)
  )
})

const toggleSelectAllUsers = () => {
  if (isAllUsersSelected.value) {
    form.value.selectedUserEmails = []
  } else {
    form.value.selectedUserEmails = props.usersList.map((u) => u.email)
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
      sr.allowedUsers = props.usersList.map((u: any) => u.email)
    }
  }
}

const clearSubRouteUsers = (sr: any) => {
  sr.allowedUsers = []
}

const selectAllSubRouteUsers = (sr: any) => {
  sr.allowedUsers = props.usersList.map((u: any) => u.email)
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
    outboundTag: props.availableOutbounds[0] || 'direct',
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

const initFormData = () => {
  if (props.isEditing && props.initialData) {
    const inb = props.initialData
    form.value.id = inb.id
    form.value.tag = inb.tag
    form.value.listen = inb.listen || '0.0.0.0'
    form.value.port = inb.port
    form.value.externalPort = inb.externalPort || 0
    form.value.externalHost = inb.externalHost || ''
    form.value.routeId = inb.routeId || 0
    form.value.protocol = inb.protocol

    let srs: SubRouteItem[] = []
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

    const isTcp = (stream.network || inb.protocol === 'vless' ? (stream.network || 'tcp') : 'tcp') === 'tcp'
    form.value.vlessFlow = isTcp ? (settings.flow !== undefined ? settings.flow : (inb.protocol === 'vless' ? 'xtls-rprx-vision' : '')) : ''

    const assignedEmails: string[] = []
    if (settings.clients?.length > 0) {
      for (const c of settings.clients) {
        if (c.email) assignedEmails.push(c.email)
      }
    }
    for (const u of props.usersList) {
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

    form.value.socksAuth = settings.auth || 'noauth'
    form.value.socksUdp = settings.udp !== false
    if (settings.accounts?.length > 0) {
      form.value.socksUsername = settings.accounts[0].user || ''
      form.value.socksPassword = settings.accounts[0].pass || ''
    } else {
      form.value.socksUsername = ''
      form.value.socksPassword = ''
    }

    if (settings.accounts?.length > 0) {
      form.value.httpUsername = settings.accounts[0].user || ''
      form.value.httpPassword = settings.accounts[0].pass || ''
    } else {
      form.value.httpUsername = ''
      form.value.httpPassword = ''
    }

    form.value.dokoAddress = settings.address || '127.0.0.1'
    form.value.dokoPort = settings.port || 53
    form.value.dokoNetwork = settings.network || 'tcp,udp'

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
  } else {
    form.value = getDefaultInboundFormData(props.usersList)
    generateRealityKey()
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      initFormData()
    } else {
      closeSubRoutePopover()
    }
  },
  { immediate: true }
)

const saveInbound = async () => {
  saving.value = true
  try {
    const payload = buildInboundPayload(form.value, props.usersList)

    if (props.isEditing) {
      await api.put(`/inbounds/${form.value.id}`, payload)
    } else {
      await api.post('/inbounds', payload)
    }

    // 同步更新用户的 InboundTags 关系
    if (['vless', 'vmess', 'trojan', 'shadowsocks'].includes(form.value.protocol)) {
      for (const u of props.usersList) {
        const currentTags = (u.inboundTags || u.inboundTag || '')
          .split(',')
          .map((s: string) => s.trim())
          .filter((s: string) => s)
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

    emit('update:modelValue', false)
    emit('saved')
  } catch (err: any) {
    toast.error('保存失败: ' + err)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  window.addEventListener('click', handleDocumentClick)
  window.addEventListener('keydown', handleSubRouteKeyDown, { capture: true })
})

onUnmounted(() => {
  window.removeEventListener('click', handleDocumentClick)
  window.removeEventListener('keydown', handleSubRouteKeyDown, { capture: true })
})
</script>
