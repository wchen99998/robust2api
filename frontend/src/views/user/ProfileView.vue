<template>
  <AppLayout>
    <div class="account-page">
      <div class="account-page-header">
        <div class="account-page-eyebrow">My Account</div>
        <div class="account-page-heading">
          <div>
            <h1 class="account-page-title">{{ t('profile.title') }}</h1>
            <p class="account-page-subtitle">{{ t('profile.description') }}</p>
          </div>
        </div>
      </div>

      <div class="grid gap-5 lg:grid-cols-[320px,minmax(0,1fr)]">
        <div class="space-y-5">
          <div class="metric-strip xl:grid-cols-1">
            <div class="metric-panel">
              <p class="metric-panel-label">{{ t('profile.accountBalance') }}</p>
              <p class="metric-panel-value">{{ formatCurrency(user?.balance || 0) }}</p>
            </div>
            <div class="metric-panel">
              <p class="metric-panel-label">{{ t('profile.concurrencyLimit') }}</p>
              <p class="metric-panel-value">{{ user?.concurrency || 0 }}</p>
            </div>
            <div class="metric-panel">
              <p class="metric-panel-label">{{ t('profile.memberSince') }}</p>
              <p class="metric-panel-value text-[22px]">{{ formatDate(user?.created_at || '', { year: 'numeric', month: 'long' }) }}</p>
            </div>
          </div>

          <ProfileInfoCard :user="user" />

          <div v-if="contactInfo" class="grouped-list">
            <div class="grouped-list-row">
              <span class="grouped-list-label">{{ t('common.contactSupport') }}</span>
              <span class="grouped-list-value">{{ contactInfo }}</span>
            </div>
          </div>
        </div>

        <div class="space-y-5">
          <ProfileEditForm :initial-username="user?.username || ''" />
          <ProfilePasswordForm />
          <ProfileTotpCard />
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
import { formatDate } from '@/utils/format'
import { authAPI } from '@/api'
import AppLayout from '@/components/layout/AppLayout.vue'
import ProfileInfoCard from '@/components/user/profile/ProfileInfoCard.vue'
import ProfileEditForm from '@/components/user/profile/ProfileEditForm.vue'
import ProfilePasswordForm from '@/components/user/profile/ProfilePasswordForm.vue'
import ProfileTotpCard from '@/components/user/profile/ProfileTotpCard.vue'

const { t } = useI18n()
const authStore = useAuthStore()
const user = computed(() => authStore.user)
const contactInfo = ref('')

onMounted(async () => {
  try { const s = await authAPI.getPublicSettings(); contactInfo.value = s.contact_info || '' } catch (error) { console.error('Failed to load contact info:', error) }
})
const formatCurrency = (v: number) => `$${v.toFixed(2)}`
</script>
