<template>
  <Teleport to="body">
    <div v-if="modelValue" class="fixed inset-0 z-50 flex justify-end">
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
          class="fixed inset-0 bg-black/65"
          @click="close"
        />
      </transition>

      <!-- Drawer Content Panel -->
      <transition
        enter-active-class="transition-transform duration-200 ease-out"
        enter-from-class="translate-x-full"
        enter-to-class="translate-x-0"
        leave-active-class="transition-transform duration-150 ease-in"
        leave-from-class="translate-x-0"
        leave-to-class="translate-x-full"
      >
        <div
          v-if="modelValue"
          :class="[
            'relative z-10 h-full bg-neutral-950 border-l border-border flex flex-col shadow-2xl overflow-hidden',
            widthClass
          ]"
        >
          <!-- Header -->
          <div class="h-14 px-5 border-b border-border flex items-center justify-between shrink-0 bg-card">
            <div class="min-w-0 pr-3">
              <h3 class="text-sm font-semibold text-foreground truncate">
                {{ title }}
              </h3>
              <p v-if="description" class="text-[11px] text-muted-foreground truncate mt-0.5">
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

          <!-- Body -->
          <div class="flex-1 overflow-y-auto p-5 space-y-5">
            <slot />
          </div>

          <!-- Footer -->
          <div v-if="$slots.footer" class="p-4 border-t border-border bg-card shrink-0 flex items-center justify-end gap-2.5">
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

interface Props {
  modelValue: boolean
  title?: string
  description?: string
  width?: string
}

const props = withDefaults(defineProps<Props>(), {
  title: '',
  description: '',
  width: 'w-full sm:max-w-xl md:max-w-2xl',
})

const widthClass = computed(() => props.width)

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
