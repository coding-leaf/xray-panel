<template>
  <button
    :type="type"
    :disabled="disabled || loading"
    :class="[
      'inline-flex items-center justify-center font-medium transition-colors select-none focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring disabled:pointer-events-none disabled:opacity-50',
      variantClasses[variant],
      sizeClasses[size],
      customClass
    ]"
  >
    <svg
      v-if="loading"
      class="animate-spin -ml-0.5 mr-2 h-3.5 w-3.5"
      xmlns="http://www.w3.org/2000/svg"
      fill="none"
      viewBox="0 0 24 24"
    >
      <circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle>
      <path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
    </svg>
    <slot />
  </button>
</template>

<script setup lang="ts">
interface Props {
  variant?: 'default' | 'secondary' | 'outline' | 'ghost' | 'destructive'
  size?: 'default' | 'sm' | 'lg' | 'icon'
  disabled?: boolean
  loading?: boolean
  type?: 'button' | 'submit' | 'reset'
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  size: 'default',
  disabled: false,
  loading: false,
  type: 'button',
  class: '',
})

const customClass = props.class

const variantClasses: Record<string, string> = {
  default: 'bg-foreground text-background hover:bg-foreground/90 font-medium shadow-sm',
  secondary: 'bg-muted text-foreground hover:bg-muted/80 border border-border/60',
  outline: 'border border-border bg-transparent text-foreground hover:bg-muted/50',
  ghost: 'text-muted-foreground hover:text-foreground hover:bg-muted/40',
  destructive: 'bg-rose-600 text-white hover:bg-rose-700 shadow-sm',
}

const sizeClasses: Record<string, string> = {
  default: 'h-9 px-3.5 py-1.5 text-xs rounded-md',
  sm: 'h-8 px-2.5 text-xs rounded-md',
  lg: 'h-10 px-4 text-sm rounded-md',
  icon: 'h-8 w-8 p-0 rounded-md',
}
</script>
