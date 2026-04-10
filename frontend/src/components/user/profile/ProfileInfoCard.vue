<template>
  <div class="grouped-surface">
    <!-- User Hero -->
    <div class="px-6 py-6">
      <div class="flex items-center gap-5">
        <div
          class="flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-[#8e8e93] to-[#48484a] text-2xl font-bold text-white flex-shrink-0"
        >
          {{ user?.email?.charAt(0).toUpperCase() || 'U' }}
        </div>
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-mica-text-primary dark:text-mica-text-primary-dark" style="font-size: 20px; font-weight: 700; letter-spacing: -0.02em; line-height: 1.2;">
            {{ user?.email }}
          </h2>
          <div class="mt-1.5 flex items-center gap-2">
            <span :class="['badge', user?.role === 'admin' ? 'badge-primary' : 'badge-gray']">
              {{ user?.role === 'admin' ? t('profile.administrator') : t('profile.user') }}
            </span>
            <span
              :class="['badge', user?.status === 'active' ? 'badge-success' : 'badge-danger']"
            >
              {{ user?.status }}
            </span>
          </div>
        </div>
      </div>
    </div>

    <!-- Account stats -->
    <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>
    <div class="flex items-center gap-5 px-6 py-5">
      <div class="flex-1 min-w-0">
        <p class="text-[11px] uppercase tracking-widest text-mica-text-tertiary dark:text-mica-text-tertiary-dark">{{ t('profile.accountBalance') }}</p>
        <p class="mt-1 text-[22px] font-bold tabular-nums tracking-tight text-mica-text-primary dark:text-mica-text-primary-dark leading-none">${{ balance.toFixed(2) }}</p>
      </div>
      <div class="w-px h-10 bg-black/[0.06] dark:bg-white/[0.08]"></div>
      <div class="flex-1 min-w-0">
        <p class="text-[11px] uppercase tracking-widest text-mica-text-tertiary dark:text-mica-text-tertiary-dark">{{ t('profile.concurrencyLimit') }}</p>
        <p class="mt-1 text-[22px] font-bold tabular-nums tracking-tight text-mica-text-primary dark:text-mica-text-primary-dark leading-none">{{ concurrency }}</p>
      </div>
    </div>

    <!-- Detail rows -->
    <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>
    <div class="flex items-center gap-3 px-6 py-3.5">
      <Icon name="mail" size="sm" class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
      <span class="text-mica-body text-mica-text-secondary dark:text-mica-text-secondary-dark truncate">{{ user?.email }}</span>
    </div>
    <template v-if="user?.username">
      <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>
      <div class="flex items-center gap-3 px-6 py-3.5">
        <Icon name="user" size="sm" class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
        <span class="text-mica-body text-mica-text-secondary dark:text-mica-text-secondary-dark truncate">{{ user.username }}</span>
      </div>
    </template>
    <template v-if="memberSince">
      <div class="mx-5 border-t border-black/[0.06] dark:border-white/[0.08]"></div>
      <div class="flex items-center gap-3 px-6 py-3.5">
        <Icon name="calendar" size="sm" class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
        <span class="text-mica-body text-mica-text-secondary dark:text-mica-text-secondary-dark">{{ formatMemberSince(memberSince) }}</span>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { User } from '@/types'

defineProps<{
  user: User | null
  balance: number
  concurrency: number
  memberSince: string
}>()

const { t } = useI18n()

const formatMemberSince = (dateStr: string) => {
  if (!dateStr) return ''
  const date = new Date(dateStr)
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'long' })
}
</script>
