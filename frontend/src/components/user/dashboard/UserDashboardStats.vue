<template>
  <!-- Hero Stats — 3 primary KPIs with large-title numbers -->
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-3">
    <!-- Balance -->
    <div
      v-if="!isSimple"
      class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] px-5 py-5"
    >
      <p class="text-mica-subhead text-mica-text-secondary dark:text-mica-text-secondary-dark">
        {{ t('dashboard.balance') }}
      </p>
      <p class="mt-3 text-mica-large-title text-mica-text-primary dark:text-mica-text-primary-dark">
        ${{ formatBalance(balance) }}
      </p>
      <p class="mt-1 text-mica-subhead text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
        {{ t('common.available') }}
      </p>
    </div>

    <!-- Today Requests -->
    <div class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] px-5 py-5">
      <p class="text-mica-subhead text-mica-text-secondary dark:text-mica-text-secondary-dark">
        {{ t('dashboard.todayRequests') }}
      </p>
      <p class="mt-3 text-mica-large-title text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ stats?.today_requests || 0 }}
      </p>
      <p class="mt-1 text-mica-subhead text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
        {{ formatNumber(stats?.total_requests || 0) }} {{ t('common.total').toLowerCase() }}
      </p>
    </div>

    <!-- Today Cost -->
    <div class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] px-5 py-5">
      <p class="text-mica-subhead text-mica-text-secondary dark:text-mica-text-secondary-dark">
        {{ t('dashboard.todayCost') }}
      </p>
      <p class="mt-3 text-mica-large-title text-mica-text-primary dark:text-mica-text-primary-dark">
        ${{ formatCost(stats?.today_actual_cost || 0) }}
      </p>
      <p class="mt-1 text-mica-subhead text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
        ${{ formatCost(stats?.total_actual_cost || 0) }} {{ t('common.total').toLowerCase() }}
      </p>
    </div>
  </div>

  <!-- Detail Metrics — HIG Grouped List -->
  <div class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] overflow-hidden">
    <!-- API Keys -->
    <div class="flex items-center justify-between px-5 py-3.5">
      <span class="text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ t('dashboard.apiKeys') }}
      </span>
      <span class="text-mica-body text-mica-text-secondary dark:text-mica-text-secondary-dark">
        <span class="font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ stats?.active_api_keys || 0 }}</span>
        {{ t('common.active') }}
        <span class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark"> · {{ stats?.total_api_keys || 0 }} {{ t('common.total').toLowerCase() }}</span>
      </span>
    </div>

    <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>

    <!-- Today Tokens -->
    <div class="flex items-center justify-between px-5 py-3.5">
      <span class="text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ t('dashboard.todayTokens') }}
      </span>
      <div class="text-right">
        <span class="text-mica-body font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ formatTokens(stats?.today_tokens || 0) }}</span>
        <p class="text-[11px] text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
          {{ formatTokens(stats?.today_input_tokens || 0) }} in · {{ formatTokens(stats?.today_output_tokens || 0) }} out
        </p>
      </div>
    </div>

    <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>

    <!-- Total Tokens -->
    <div class="flex items-center justify-between px-5 py-3.5">
      <span class="text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ t('dashboard.totalTokens') }}
      </span>
      <div class="text-right">
        <span class="text-mica-body font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ formatTokens(stats?.total_tokens || 0) }}</span>
        <p class="text-[11px] text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
          {{ formatTokens(stats?.total_input_tokens || 0) }} in · {{ formatTokens(stats?.total_output_tokens || 0) }} out
        </p>
      </div>
    </div>

    <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>

    <!-- Performance -->
    <div class="flex items-center justify-between px-5 py-3.5">
      <span class="text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ t('dashboard.performance') }}
      </span>
      <span class="text-mica-body text-mica-text-secondary dark:text-mica-text-secondary-dark">
        <span class="font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ formatTokens(stats?.rpm || 0) }}</span>
        <span class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark"> RPM</span>
        <span class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark"> · </span>
        <span class="font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ formatTokens(stats?.tpm || 0) }}</span>
        <span class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark"> TPM</span>
      </span>
    </div>

    <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>

    <!-- Avg Response -->
    <div class="flex items-center justify-between px-5 py-3.5">
      <span class="text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ t('dashboard.avgResponse') }}
      </span>
      <span class="text-mica-body font-medium text-mica-text-primary dark:text-mica-text-primary-dark">
        {{ formatDuration(stats?.average_duration_ms || 0) }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { UserDashboardStats as UserStatsType } from '@/api/usage'

defineProps<{
  stats: UserStatsType
  balance: number
  isSimple: boolean
}>()
const { t } = useI18n()

const formatBalance = (b: number) =>
  new Intl.NumberFormat('en-US', {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2
  }).format(b)

const formatNumber = (n: number) => n.toLocaleString()
const formatCost = (c: number) => c.toFixed(4)
const formatTokens = (t: number) => {
  if (t >= 1_000_000) return `${(t / 1_000_000).toFixed(1)}M`
  if (t >= 1000) return `${(t / 1000).toFixed(1)}K`
  return t.toString()
}
const formatDuration = (ms: number) => ms >= 1000 ? `${(ms / 1000).toFixed(2)}s` : `${ms.toFixed(0)}ms`
</script>
