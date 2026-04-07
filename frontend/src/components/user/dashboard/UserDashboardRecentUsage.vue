<template>
  <div class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] overflow-hidden">
    <div class="flex items-center justify-between px-5 py-3">
      <p class="text-mica-caption uppercase tracking-wide text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
        {{ t('dashboard.recentUsage') }}
      </p>
      <span class="text-mica-caption text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
        {{ t('dashboard.last7Days') }}
      </span>
    </div>

    <div v-if="loading" class="flex items-center justify-center border-t border-black/[0.06] dark:border-white/[0.08] py-12">
      <LoadingSpinner size="lg" />
    </div>

    <div v-else-if="data.length === 0" class="border-t border-black/[0.06] dark:border-white/[0.08] py-8">
      <EmptyState :title="t('dashboard.noUsageRecords')" :description="t('dashboard.startUsingApi')" />
    </div>

    <template v-else>
      <div
        v-for="log in data"
        :key="log.id"
        class="flex items-center justify-between border-t border-black/[0.06] dark:border-white/[0.08] px-5 py-3 transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
      >
        <div class="min-w-0">
          <p class="text-mica-body font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ log.model }}</p>
          <p class="mt-0.5 text-[11px] text-mica-text-tertiary dark:text-mica-text-tertiary-dark">{{ formatDateTime(log.created_at) }}</p>
        </div>
        <div class="text-right flex-shrink-0 pl-4">
          <p class="text-mica-body font-medium tabular-nums text-mica-text-primary dark:text-mica-text-primary-dark">
            ${{ formatCost(log.actual_cost) }}
          </p>
          <p class="mt-0.5 text-[11px] tabular-nums text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
            {{ (log.input_tokens + log.output_tokens).toLocaleString() }} tokens
          </p>
        </div>
      </div>

      <div class="border-t border-black/[0.06] dark:border-white/[0.08]">
        <router-link
          to="/usage"
          class="flex items-center justify-center gap-1.5 px-5 py-3 text-mica-subhead font-medium text-status-blue dark:text-status-blue-dark transition-colors hover:bg-status-blue/[0.04] dark:hover:bg-status-blue-dark/[0.06]"
        >
          {{ t('dashboard.viewAllUsage') }}
          <Icon name="arrowRight" size="sm" />
        </router-link>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime } from '@/utils/format'
import type { UsageLog } from '@/types'

defineProps<{
  data: UsageLog[]
  loading: boolean
}>()
const { t } = useI18n()
const formatCost = (c: number) => c.toFixed(4)
</script>
