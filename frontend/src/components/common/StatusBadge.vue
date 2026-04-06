<template>
  <span :class="['inline-flex items-center gap-1.5 rounded-mica-sm px-2.5 py-0.5 text-xs font-medium', badgeClass]">
    <span :class="['inline-block h-1.5 w-1.5 rounded-full', dotClass]"></span>
    {{ label }}
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  status: string
  label: string
}>()

const badgeClass = computed(() => {
  switch (props.status) {
    case 'active': case 'success':
      return 'bg-status-green/[0.12] text-[#248a3d] dark:text-status-green-dark'
    case 'error': case 'danger':
      return 'bg-status-red/10 text-[#d70015] dark:text-status-red-dark'
    case 'disabled': case 'inactive': case 'warning':
      return 'bg-status-amber/10 text-[#c93400] dark:text-status-amber-dark'
    default:
      return 'bg-black/[0.04] dark:bg-white/[0.06] text-mica-text-secondary dark:text-mica-text-secondary-dark'
  }
})

const dotClass = computed(() => {
  switch (props.status) {
    case 'active': case 'success': return 'bg-status-green dark:bg-status-green-dark'
    case 'error': case 'danger': return 'bg-status-red dark:bg-status-red-dark'
    case 'disabled': case 'inactive': case 'warning': return 'bg-status-amber dark:bg-status-amber-dark'
    default: return 'bg-mica-text-tertiary dark:bg-mica-text-tertiary-dark'
  }
})
</script>
