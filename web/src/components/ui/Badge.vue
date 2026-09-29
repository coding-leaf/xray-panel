<template>
  <span
    :class="[
      'inline-flex items-center gap-1.5 px-2 py-0.5 rounded-md text-[11px] font-mono border font-medium transition-colors select-none',
      variantClasses[variant],
      customClass
    ]"
  >
    <span
      v-if="dot"
      :class="[
        'w-1.5 h-1.5 rounded-full shrink-0',
        dotColor || dotColorByVariant[variant]
      ]"
    />
    <slot />
  </span>
</template>

<script setup lang="ts">
interface Props {
  variant?: 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive'
  dot?: boolean
  dotColor?: string
  class?: string
}

const props = withDefaults(defineProps<Props>(), {
  variant: 'default',
  dot: false,
  dotColor: '',
  class: '',
})

const customClass = props.class

const variantClasses: Record<string, string> = {
  default: 'bg-muted/70 text-foreground border-border',
  secondary: 'bg-muted/30 text-muted-foreground border-border/40',
  outline: 'border-border text-foreground bg-transparent',
  success: 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20',
  warning: 'bg-amber-500/10 text-amber-400 border-amber-500/20',
  destructive: 'bg-rose-500/10 text-rose-400 border-rose-500/20',
}

const dotColorByVariant: Record<string, string> = {
  default: 'bg-foreground',
  secondary: 'bg-muted-foreground',
  outline: 'bg-foreground',
  success: 'bg-emerald-400',
  warning: 'bg-amber-400',
  destructive: 'bg-rose-400',
}
</script>
