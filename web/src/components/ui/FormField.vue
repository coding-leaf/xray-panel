<script setup lang="ts">
defineProps<{
  label?: string
  required?: boolean
  hint?: string
  error?: string
}>()
</script>

<template>
  <div class="space-y-1">
    <div v-if="label || $slots.label" class="flex items-center justify-between text-[11px] font-mono text-muted-foreground">
      <label class="flex items-center gap-1">
        <slot name="label">{{ label }}</slot>
        <span v-if="required" class="text-rose-400">*</span>
      </label>
      <slot name="extra" />
    </div>
    <div>
      <slot />
    </div>
    <p v-if="hint && !error" class="text-[10px] text-muted-foreground font-mono leading-tight">
      {{ hint }}
    </p>
    <p v-if="error" class="text-[10px] text-rose-400 font-mono leading-tight">
      {{ error }}
    </p>
  </div>
</template>
