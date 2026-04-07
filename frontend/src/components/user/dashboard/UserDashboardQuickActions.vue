<template>
  <div class="rounded-mica-lg bg-white/70 dark:bg-white/[0.05] backdrop-blur-xl border border-black/[0.06] dark:border-white/[0.08] overflow-hidden">
    <div class="px-5 py-3">
      <p class="text-mica-caption uppercase tracking-wide text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
        {{ t('dashboard.quickActions') }}
      </p>
    </div>

    <button
      @click="router.push('/keys')"
      class="flex w-full items-center gap-3.5 border-t border-black/[0.06] dark:border-white/[0.08] px-5 py-3 text-left transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
    >
      <Icon name="key" size="md" class="flex-shrink-0 text-mica-text-secondary dark:text-mica-text-secondary-dark" />
      <span class="flex-1 text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">{{ t('dashboard.createApiKey') }}</span>
      <Icon name="chevronRight" size="sm" class="flex-shrink-0 text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
    </button>

    <button
      @click="router.push('/usage')"
      class="flex w-full items-center gap-3.5 border-t border-black/[0.06] dark:border-white/[0.08] px-5 py-3 text-left transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
    >
      <Icon name="chart" size="md" class="flex-shrink-0 text-mica-text-secondary dark:text-mica-text-secondary-dark" />
      <span class="flex-1 text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">{{ t('dashboard.viewUsage') }}</span>
      <Icon name="chevronRight" size="sm" class="flex-shrink-0 text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
    </button>

    <button
      @click="router.push('/redeem')"
      class="flex w-full items-center gap-3.5 border-t border-black/[0.06] dark:border-white/[0.08] px-5 py-3 text-left transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.02]"
    >
      <Icon name="gift" size="md" class="flex-shrink-0 text-mica-text-secondary dark:text-mica-text-secondary-dark" />
      <span class="flex-1 text-mica-body text-mica-text-primary dark:text-mica-text-primary-dark">{{ t('dashboard.redeemCode') }}</span>
      <Icon name="chevronRight" size="sm" class="flex-shrink-0 text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
const router = useRouter()
const { t } = useI18n()
</script>
