<template>
  <AuthLayout>
    <div class="space-y-6">
      <div class="text-center">
        <h2 class="text-mica-title1 text-mica-text-primary dark:text-mica-text-primary-dark">
          {{ t('auth.oidc.callbackTitle', { providerName }) }}
        </h2>
        <p class="mt-2 text-mica-subhead text-mica-text-secondary dark:text-mica-text-secondary-dark">
          {{
            isProcessing
              ? t('auth.oidc.callbackProcessing', { providerName })
              : t('auth.oidc.callbackHint')
          }}
        </p>
      </div>

      <transition name="fade">
        <div v-if="needsInvitation" class="space-y-4">
          <p class="text-mica-body text-mica-text-secondary dark:text-mica-text-secondary-dark">
            {{ t('auth.oidc.invitationRequired', { providerName }) }}
          </p>
          <input
            v-model="invitationCode"
            type="text"
            class="input w-full"
            :placeholder="t('auth.invitationCodePlaceholder')"
            :disabled="isSubmitting"
            @keyup.enter="handleSubmitInvitation"
          />
          <p v-if="invitationError" class="input-error-text">
            {{ invitationError }}
          </p>
          <button
            class="btn btn-primary w-full"
            :disabled="isSubmitting || !invitationCode.trim()"
            @click="handleSubmitInvitation"
          >
            {{ isSubmitting ? t('auth.oidc.completing') : t('auth.oidc.completeRegistration') }}
          </button>
        </div>
      </transition>

      <transition name="fade">
        <div
          v-if="errorMessage"
          class="rounded-mica-lg border border-status-red/20 bg-status-red/[0.06] p-4 dark:border-status-red-dark/20 dark:bg-status-red-dark/[0.06]"
        >
          <div class="flex items-start gap-3">
            <div class="flex-shrink-0">
              <Icon name="exclamationCircle" size="md" class="text-status-red dark:text-status-red-dark" />
            </div>
            <div class="space-y-2">
              <p class="text-sm text-status-red dark:text-status-red-dark">
                {{ errorMessage }}
              </p>
              <router-link to="/login" class="btn btn-primary">
                {{ t('auth.oidc.backToLogin') }}
              </router-link>
            </div>
          </div>
        </div>
      </transition>
    </div>
  </AuthLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { AuthLayout } from '@/components/layout'
import Icon from '@/components/icons/Icon.vue'
import { useAuthStore, useAppStore } from '@/stores'
import { bootstrap, completeOAuthRegistration } from '@/api/auth'

const route = useRoute()
const router = useRouter()
const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const isProcessing = ref(true)
const errorMessage = ref('')
const needsInvitation = ref(false)
const invitationCode = ref('')
const isSubmitting = ref(false)
const invitationError = ref('')
const redirectTo = ref('/dashboard')
const providerName = ref('OIDC')

function sanitizeRedirectPath(path: string | null | undefined): string {
  if (!path || !path.startsWith('/')) return '/dashboard'
  if (path.startsWith('//') || path.includes('://') || path.includes('\n') || path.includes('\r')) {
    return '/dashboard'
  }
  return path
}

async function loadProviderName() {
  try {
    const boot = await bootstrap()
    const oidcProvider = (boot.auth_providers || []).find((provider) => provider.id === 'oidc')
    if (oidcProvider?.display_name) {
      providerName.value = oidcProvider.display_name
    }
  } catch {
    // Keep the fallback provider name.
  }
}

async function handleSubmitInvitation() {
  invitationError.value = ''
  if (!invitationCode.value.trim()) return

  isSubmitting.value = true
  try {
    const response = await completeOAuthRegistration(invitationCode.value.trim())
    authStore.hydrate(response)
    appStore.showSuccess(t('auth.loginSuccess'))
    await router.replace(redirectTo.value)
  } catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { message?: string } } }
    invitationError.value =
      err.response?.data?.message || err.message || t('auth.oidc.completeRegistrationFailed')
  } finally {
    isSubmitting.value = false
  }
}

onMounted(async () => {
  void loadProviderName()

  const redirect = sanitizeRedirectPath(
    (route.query.redirect as string | undefined) || '/dashboard'
  )
  const error = (route.query.error as string | undefined) || ''
  const errorDesc =
    (route.query.error_description as string | undefined) ||
    (route.query.error_message as string | undefined) ||
    ''

  if (error) {
    errorMessage.value = errorDesc || error
    appStore.showError(errorMessage.value)
    isProcessing.value = false
    return
  }

  try {
    await authStore.initialize(true)
    if (authStore.isAuthenticated) {
      appStore.showSuccess(t('auth.loginSuccess'))
      await router.replace(redirect)
      return
    }

    if (authStore.pendingRegistration) {
      redirectTo.value = sanitizeRedirectPath(authStore.pendingRegistration.redirect_to || redirect)
      needsInvitation.value = true
      isProcessing.value = false
      return
    }

    errorMessage.value = t('auth.loginFailed')
    appStore.showError(errorMessage.value)
    isProcessing.value = false
  } catch (e: unknown) {
    const err = e as { message?: string; response?: { data?: { detail?: string } } }
    errorMessage.value = err.response?.data?.detail || err.message || t('auth.loginFailed')
    appStore.showError(errorMessage.value)
    isProcessing.value = false
  }
})
</script>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: all 0.3s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}
</style>
