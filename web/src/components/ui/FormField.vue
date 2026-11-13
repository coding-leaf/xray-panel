<template>
  <div class="space-y-1">
    <div
      v-if="label || $slots.label"
      class="flex items-center justify-between text-[11px] font-mono text-muted-foreground"
    >
      <label :for="controlId" class="flex items-center gap-1">
        <slot name="label">{{ label }}</slot>
        <span v-if="required" class="text-rose-400">*</span>
      </label>
      <slot name="extra" />
    </div>
    <div>
      <slot :id="controlId" :described-by="describedBy" :invalid="Boolean(error)" />
    </div>
    <p
      v-if="hint && !error"
      :id="hintId"
      class="text-[10px] text-muted-foreground font-mono leading-tight"
    >
      {{ hint }}
    </p>
    <p v-if="error" :id="errorId" class="text-[10px] text-rose-400 font-mono leading-tight">
      {{ error }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { nextUid } from '../../utils/uid'

const props = defineProps<{
  label?: string
  required?: boolean
  hint?: string
  error?: string
  forId?: string
}>()

const fieldId = nextUid('field')
const hintId = nextUid('field-hint')
const errorId = nextUid('field-error')

const controlId = computed(() => props.forId || fieldId)
const describedBy = computed(() => {
  if (props.error) return errorId
  if (props.hint) return hintId
  return undefined
})
</script>
