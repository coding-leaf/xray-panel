<template>
  <td
    :class="[
      'p-3.5 align-middle text-xs text-foreground',
      customClass
    ]"
  >
    <slot />
  </td>
</template>

<script setup lang="ts">
const props = defineProps<{
  class?: string
}>()
const customClass = props.class
</script>
