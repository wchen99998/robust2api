<template>
  <AppLayout>
    <div class="account-page">
      <div class="account-page-header enter-stage">
        <div class="account-page-heading">
          <div>
            <h1 class="account-page-title">{{ t('profile.title') }}</h1>
            <p class="account-page-subtitle">{{ t('profile.description') }}</p>
          </div>
        </div>
      </div>

      <div class="grid gap-5 lg:grid-cols-[320px,minmax(0,1fr)]">
        <div class="space-y-5 enter-stage enter-stage-delay-1">
          <ProfileInfoCard
            :user="user"
            :balance="user?.balance || 0"
            :concurrency="user?.concurrency || 0"
            :member-since="user?.created_at || ''"
          />

          <div v-if="contactInfo" class="grouped-list">
            <div class="grouped-list-row">
              <span class="grouped-list-label">{{ t('common.contactSupport') }}</span>
              <span class="grouped-list-value">{{ contactInfo }}</span>
            </div>
          </div>
        </div>

        <div class="space-y-5 enter-stage enter-stage-delay-2">
          <ProfileEditForm :initial-username="user?.username || ''" />
          <ProfilePasswordForm v-if="passwordChangeEnabled" />
          <ProfileTotpCard v-if="mfaSelfServiceEnabled" />
        </div>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore } from '@/stores/auth'
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
const passwordChangeEnabled = computed(
  () => authStore.authCapabilities?.password_change_enabled ?? true
)
const mfaSelfServiceEnabled = computed(
  () => authStore.authCapabilities?.mfa_self_service_enabled ?? true
)

onMounted(async () => {
  try { const s = await authAPI.getPublicSettings(); contactInfo.value = s.contact_info || '' } catch (error) { console.error('Failed to load contact info:', error) }
})
</script>
