<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 overflow-y-auto">
      <!-- Backdrop -->
      <transition
        enter-active-class="transition-opacity duration-200 ease-out"
        enter-from-class="opacity-0"
        enter-to-class="opacity-100"
        leave-active-class="transition-opacity duration-150 ease-in"
        leave-from-class="opacity-100"
        leave-to-class="opacity-0"
      >
        <div
          v-if="modelValue"
          class="fixed inset-0 bg-black/75"
          @click="close"
        />
      </transition>

      <!-- Modal Content Panel -->
      <transition
        enter-active-class="transition-all duration-200 ease-out"
        enter-from-class="opacity-0 scale-95"
        enter-to-class="opacity-100 scale-100"
        leave-active-class="transition-all duration-150 ease-in"
        leave-from-class="opacity-100 scale-100"
        leave-to-class="opacity-0 scale-95"
      >
        <div
          v-if="modelValue"
          :class="[
            'relative z-10 w-full bg-neutral-950 border border-border rounded-lg shadow-2xl flex flex-col overflow-hidden max-h-[90vh]',
            sizeClass
          ]"
        >
          <!-- Header -->
          <slot name="header">
            <div v-if="title || $slots.header" class="h-14 px-5 border-b border-border flex items-center justify-between shrink-0 bg-card">
              <div class="min-w-0 pr-3">
                <h3 class="text-sm font-semibold text-foreground truncate flex items-center gap-2">
                  <slot name="icon" />
                  <span>{{ title }}</span>
                </h3>
                <p v-if="description" class="text-[11px] text-muted-foreground truncate mt-0.5 font-mono">
                  {{ description }}
                </p>
              </div>
              <button
                type="button"
                @click="close"
                class="h-7 w-7 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted/60 flex items-center justify-center transition-colors"
                title="关闭 (Esc)"
              >
                <X class="w-4 h-4" />
              </button>
            </div>
          </slot>

          <!-- Sub-header / Tabs Slot -->
          <slot name="sub-header" />

          <!-- Body -->
          <div class="flex-1 overflow-y-auto p-5 space-y-4 text-xs">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="p-3.5 border-t border-border bg-card shrink-0 flex items-center justify-end gap-2.5">
            <slot name="footer" />
          </div>
        </div>
      </transition>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed, watch, onMounted, onUnmounted } from 'vue'
import { X } from 'lucide-vue-next'

interface ModalProps {
  modelValue: boolean
  title?: string
  description?: string
  size?: 'sm' | 'md' | 'lg' | 'xl' | '2xl'
}

const props = withDefaults(defineProps<ModalProps>(), {
  title: '',
  description: '',
  size: 'md',
})

const sizeMap: Record<NonNullable<ModalProps['size']>, string> = {
  sm: 'max-w-md',
  md: 'max-w-lg',
  lg: 'max-w-2xl',
  xl: 'max-w-4xl',
  '2xl': 'max-w-6xl',
}

const sizeClass = computed(() => sizeMap[props.size || 'md'])

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'close'): void
}>()

const close = () => {
  emit('update:modelValue', false)
  emit('close')
}

const handleKeyDown = (e: KeyboardEvent) => {
  if (e.key === 'Escape' && props.modelValue) {
    close()
  }
}

watch(
  () => props.modelValue,
  (val) => {
    if (val) {
      document.body.style.overflow = 'hidden'
    } else {
      document.body.style.overflow = ''
    }
  }
)

onMounted(() => {
  window.addEventListener('keydown', handleKeyDown)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeyDown)
  document.body.style.overflow = ''
})
</script>
