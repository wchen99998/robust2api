<template>
  <div class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] p-4 transition-colors hover:bg-white/90 dark:hover:bg-white/[0.08]">
    <p class="text-mica-caption uppercase text-mica-text-secondary dark:text-mica-text-secondary-dark truncate">{{ title }}</p>
    <p class="mt-1.5 text-[28px] font-semibold tracking-tight text-mica-text-primary dark:text-mica-text-primary-dark truncate" :title="String(formattedValue)">{{ formattedValue }}</p>
    <p v-if="change !== undefined" :class="['mt-1 text-mica-caption font-medium', trendClass]">
      <span v-if="changeType === 'up'">+</span><span v-else-if="changeType === 'down'">-</span>{{ formattedChange }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Component } from 'vue'

type ChangeType = 'up' | 'down' | 'neutral'
type IconVariant = 'primary' | 'success' | 'warning' | 'danger'

interface Props {
  title: string
  value: number | string
  icon?: Component
  iconVariant?: IconVariant
  change?: number
  changeType?: ChangeType
  formatValue?: (value: number | string) => string
}

const props = withDefaults(defineProps<Props>(), {
  changeType: 'neutral',
  iconVariant: 'primary'
})

const formattedValue = computed(() => {
  if (props.formatValue) {
    return props.formatValue(props.value)
  }
  if (typeof props.value === 'number') {
    return props.value.toLocaleString()
  }
  return props.value
})

const formattedChange = computed(() => {
  if (props.change === undefined) return ''
  const absChange = Math.abs(props.change)
  return `${absChange}%`
})

const trendClass = computed(() => {
  const classes: Record<ChangeType, string> = {
    up: 'text-status-green dark:text-status-green-dark',
    down: 'text-status-red dark:text-status-red-dark',
    neutral: 'text-mica-text-secondary dark:text-mica-text-secondary-dark'
  }
  return classes[props.changeType]
})
</script>
