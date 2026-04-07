<template>
  <div class="grouped-surface">
    <!-- User Hero — warm neutral, no gradient -->
    <div class="px-6 py-5">
      <div class="flex items-center gap-4">
        <div
          class="flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-[#8e8e93] to-[#636366] text-xl font-semibold text-white"
        >
          {{ user?.email?.charAt(0).toUpperCase() || 'U' }}
        </div>
        <div class="min-w-0 flex-1">
          <h2 class="truncate text-mica-title2 text-mica-text-primary dark:text-mica-text-primary-dark">
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

    <!-- Detail rows — HIG grouped list -->
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
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { User } from '@/types'

defineProps<{
  user: User | null
}>()

const { t } = useI18n()
</script>
