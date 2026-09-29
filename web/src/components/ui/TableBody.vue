<template>
  <tbody :class="['divide-y divide-border/40', customClass]">
    <slot />
  </tbody>
</template>

<script setup lang="ts">
const props = defineProps<{
  class?: string
}>()
const customClass = props.class
</script>
