<template>
  <div class="fixed top-5 right-5 z-50 flex flex-col gap-2.5 max-w-sm w-full pointer-events-none">
    <transition-group name="toast-slide">
      <div
        v-for="item in toasts"
        :key="item.id"
        class="pointer-events-auto flex items-start gap-3 p-3.5 rounded-2xl shadow-2xl backdrop-blur-xl border transition-all duration-300 relative overflow-hidden"
        :class="getToastClass(item.type)"
      >
        <!-- Icon -->
        <component :is="getIcon(item.type)" class="w-5 h-5 shrink-0 mt-0.5" />

        <!-- Message -->
        <div class="flex-1 text-xs font-medium leading-relaxed pr-2">
          {{ item.message }}
        </div>

        <!-- Close Button -->
        <button
          @click="removeToast(item.id)"
          class="text-gray-400 hover:text-white transition-colors p-0.5 rounded-lg hover:bg-white/10"
        >
          <X class="w-3.5 h-3.5" />
        </button>

        <!-- Progress bar -->
        <div
          class="absolute bottom-0 left-0 h-0.5 bg-current opacity-30 animate-progress"
          :style="{ animationDuration: `${item.duration}ms` }"
        ></div>
      </div>
    </transition-group>
  </div>
</template>

<script setup lang="ts">
import { toasts, removeToast, type ToastType } from '../utils/toast'
import { CheckCircle2, AlertCircle, AlertTriangle, Info, X } from 'lucide-vue-next'

const getIcon = (type: ToastType) => {
  switch (type) {
    case 'success':
      return CheckCircle2
    case 'error':
      return AlertCircle
    case 'warning':
      return AlertTriangle
    default:
      return Info
  }
}

const getToastClass = (type: ToastType) => {
  switch (type) {
    case 'success':
      return 'bg-[#0B1416]/90 border-emerald-500/40 text-emerald-300 shadow-emerald-500/10'
    case 'error':
      return 'bg-[#180B0F]/90 border-rose-500/40 text-rose-300 shadow-rose-500/10'
    case 'warning':
      return 'bg-[#18130B]/90 border-amber-500/40 text-amber-300 shadow-amber-500/10'
    default:
      return 'bg-[#0C121E]/90 border-indigo-500/40 text-indigo-300 shadow-indigo-500/10'
  }
}
</script>

<style scoped>
.toast-slide-enter-active,
.toast-slide-leave-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}
.toast-slide-enter-from {
  opacity: 0;
  transform: translateX(40px) scale(0.95);
}
.toast-slide-leave-to {
  opacity: 0;
  transform: translateX(40px) scale(0.95);
}

@keyframes progress {
  from {
    width: 100%;
  }
  to {
    width: 0%;
  }
}

.animate-progress {
  animation-name: progress;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
}
</style>
