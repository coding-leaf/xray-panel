<template>
  <div :class="['relative w-full overflow-auto rounded-lg border border-border bg-neutral-950', customClass]">
    <table class="w-full caption-bottom text-xs border-collapse">
      <slot />
    </table>
  </div>
</template>

<script setup lang="ts">
const props = defineProps<{
  class?: string
}>()
const customClass = props.class
</script>
