<template>
  <thead :class="['border-b border-border bg-neutral-900', customClass]">
    <slot />
  </thead>
</template>

<script setup lang="ts">
const props = defineProps<{
  class?: string
}>()
const customClass = props.class
</script>
