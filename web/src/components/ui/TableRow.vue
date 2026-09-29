<template>
  <tr
    :class="[
      'border-b border-border/30 transition-colors hover:bg-muted/40 cursor-pointer',
      customClass
    ]"
  >
    <slot />
  </tr>
</template>

<script setup lang="ts">
const props = defineProps<{
  class?: any
}>()
const customClass = props.class
</script>
