<template>
  <div class="fixed top-5 left-4 right-4 sm:left-auto sm:right-5 sm:w-80 z-50 flex flex-col gap-2.5 pointer-events-none">
    <transition-group name="toast-slide">
      <div
        v-for="item in toasts"
        :key="item.id"
        class="pointer-events-auto flex items-start gap-3 p-3.5 rounded-lg shadow-lg border bg-neutral-900 border-border text-foreground transition-all duration-300 relative overflow-hidden"
        :class="getToastClass(item.type)"
      >
        <!-- Icon -->
        <component :is="getIcon(item.type)" class="w-4 h-4 shrink-0 mt-0.5" :class="getIconClass(item.type)" />

        <!-- Message -->
        <div class="flex-1 text-xs font-medium leading-relaxed pr-2">
          {{ item.message }}
        </div>

        <!-- Close Button -->
        <button
          @click="removeToast(item.id)"
          class="text-muted-foreground hover:text-foreground transition-colors p-0.5 rounded-md hover:bg-muted"
        >
          <X class="w-3.5 h-3.5" />
        </button>

        <!-- Progress bar -->
        <div
          class="absolute bottom-0 left-0 h-0.5 bg-neutral-600 opacity-40 animate-progress"
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

const getIconClass = (type: ToastType) => {
  switch (type) {
    case 'success':
      return 'text-emerald-400'
    case 'error':
      return 'text-rose-400'
    case 'warning':
      return 'text-amber-400'
    default:
      return 'text-muted-foreground'
  }
}

const getToastClass = (type: ToastType) => {
  switch (type) {
    case 'success':
      return 'border-emerald-500/30'
    case 'error':
      return 'border-rose-500/30'
    case 'warning':
      return 'border-amber-500/30'
    default:
      return 'border-border'
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
