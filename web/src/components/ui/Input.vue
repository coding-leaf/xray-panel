<template>
  <div class="relative flex items-center w-full">
    <slot name="prefix" />
    <input
      :type="type"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :required="required"
      @input="onInput"
      @change="$emit('change', ($event.target as HTMLInputElement).value)"
      :class="[
        'w-full bg-neutral-950 border border-border text-foreground text-xs rounded-md transition-colors placeholder:text-muted-foreground font-mono focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring focus-visible:border-neutral-500 disabled:cursor-not-allowed disabled:opacity-50',
        size === 'sm' ? 'h-8 px-2.5' : 'h-9 px-3',
        $slots.prefix ? 'pl-8' : '',
        $slots.suffix ? 'pr-8' : '',
        customClass
      ]"
    />
    <slot name="suffix" />
  </div>
</template>

<script setup lang="ts">
interface Props {
  modelValue?: string | number
  type?: string
  placeholder?: string
  disabled?: boolean
  readonly?: boolean
  required?: boolean
  size?: 'sm' | 'default'
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  modelValue: '',
  type: 'text',
  placeholder: '',
  disabled: false,
  readonly: false,
  required: false,
  size: 'default',
  class: '',
})

const customClass = props.class

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
  (e: 'change', value: string): void
}>()

const onInput = (event: Event) => {
  const target = event.target as HTMLInputElement
  emit('update:modelValue', props.type === 'number' ? Number(target.value) : target.value)
}
</script>
