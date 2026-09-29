<script setup lang="ts">
defineProps<{
  title?: string
  stepNumber?: number | string
}>()
</script>

<template>
  <div class="rounded-lg border border-border bg-card p-4 space-y-3">
    <div v-if="title || $slots.header" class="flex items-center justify-between border-b border-border/40 pb-2.5">
      <div class="font-mono font-semibold text-foreground text-xs uppercase tracking-wider flex items-center gap-1.5">
        <span v-if="stepNumber" class="inline-flex items-center justify-center w-4 h-4 rounded-full bg-primary/20 text-primary text-[10px]">
          {{ stepNumber }}
        </span>
        <slot name="header">
          <span>{{ title }}</span>
        </slot>
      </div>
      <slot name="actions" />
    </div>
    <slot />
  </div>
</template>
