<template>
  <Modal
    :model-value="modelValue"
    @update:model-value="emit('update:modelValue', $event)"
    title="全节点订阅与安全凭据"
    :description="shareData?.user?.email || shareData?.email || ''"
    size="lg"
  >
    <template #icon>
      <Zap class="w-4 h-4 text-amber-400" />
    </template>

    <template #sub-header>
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
    </template>

    <div v-if="shareData" class="space-y-4">
      <!-- 1. 二维码模式 -->
      <div v-if="activeShareTab === 'qrcode'" class="text-center py-4 space-y-3">
        <div class="inline-block p-4 bg-white rounded-lg shadow-sm border border-border">
          <QrcodeVue
            :value="subUrl"
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
            @click="generateUserTicket"
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

          <div
            v-if="generatedTicket.share_text?.includes('127.0.0.1') || generatedTicket.share_text?.includes('localhost')"
            class="p-2.5 bg-amber-500/10 border border-amber-500/20 rounded-md text-[11px] text-amber-300 flex items-start gap-2"
          >
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
              v-if="userId"
              variant="ghost"
              size="sm"
              class="h-6 text-[10px] text-rose-400 font-mono px-2"
              @click="emit('reset-token', userId)"
            >
              <RotateCcw class="w-3 h-3 mr-1" /> 重置
            </Button>
          </div>

          <div class="flex items-center gap-2 font-mono">
            <input
              :value="subUrl"
              readonly
              class="w-full bg-neutral-950 border border-border rounded-md px-3 h-8 text-foreground text-[11px] select-all focus:outline-none"
            />
            <Button
              variant="secondary"
              size="sm"
              class="shrink-0 h-8 font-mono"
              @click="copyText(subUrl)"
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
              v-for="link in normalizedLinks"
              :key="link.tag + link.url"
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
            <div v-if="!normalizedLinks.length" class="text-center py-4 text-muted-foreground text-xs font-mono">
              该用户暂无直连节点链接
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <Button variant="outline" size="sm" @click="emit('update:modelValue', false)">
        关闭
      </Button>
    </template>
  </Modal>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import {
  Zap,
  Copy,
  QrCode,
  ShieldCheck,
  Sparkles,
  AlertCircle,
  RotateCcw,
} from 'lucide-vue-next'
import QrcodeVue from 'qrcode.vue'
import api from '../../../api'
import { toast } from '../../../utils/toast'
import { copyText } from '../../../utils/clipboard'
import { UserSubscriptionService } from '../services/subscription'
import type { UserShareData } from '../types'

import Modal from '../../../components/ui/Modal.vue'
import Button from '../../../components/ui/Button.vue'
import Badge from '../../../components/ui/Badge.vue'

interface Props {
  modelValue: boolean
  shareData?: UserShareData | null
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
  (e: 'reset-token', userId: number): void
}>()

const activeShareTab = ref<'link' | 'qrcode' | 'ticket'>('link')
const ticketTTL = ref(15)
const ticketMaxUses = ref(2)
const generatingTicket = ref(false)
const generatedTicket = ref<any>(null)

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      activeShareTab.value = 'link'
      generatedTicket.value = null
    }
  }
)

const userId = computed(() => {
  return props.shareData?.user?.id || props.shareData?.userId || 0
})

const subUrl = computed(() => {
  if (props.shareData?.allSubUrl) return props.shareData.allSubUrl
  if (props.shareData?.user) return UserSubscriptionService.getDirectTokenSubUrl(props.shareData.user)
  if (props.shareData?.subToken) return `${window.location.origin}/sub/${props.shareData.subToken}`
  return ''
})

const normalizedLinks = computed(() => {
  if (!props.shareData) return []
  if (Array.isArray(props.shareData.nodes) && props.shareData.nodes.length > 0) {
    return props.shareData.nodes.map((node) => ({
      tag: node.remark || node.tag,
      url: node.shareLink || node.url || '',
    }))
  }
  if (Array.isArray(props.shareData.links)) {
    return props.shareData.links.map((link) => {
      if (typeof link === 'string') {
        return { tag: '节点直连链接', url: link }
      }
      return {
        tag: link.tag || '节点直连链接',
        url: link.url || link.shareLink || '',
      }
    })
  }
  return []
})

const generateUserTicket = async () => {
  if (!userId.value) return
  generatingTicket.value = true
  try {
    const res: any = await api.post(`/users/${userId.value}/tickets`, {
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
</script>
