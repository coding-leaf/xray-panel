<template>
  <th
    :class="[
      'h-9 px-3.5 text-left align-middle font-mono text-[11px] font-medium text-muted-foreground tracking-wider uppercase select-none',
      customClass
    ]"
  >
    <slot />
  </th>
</template>

<script setup lang="ts">
const props = defineProps<{
  class?: string
}>()
const customClass = props.class
</script>
