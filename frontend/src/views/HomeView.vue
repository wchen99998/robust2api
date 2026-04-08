<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      allowfullscreen
    ></iframe>
    <!-- SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div
    v-else
    class="flex h-screen flex-col overflow-hidden bg-canvas max-md:h-auto max-md:min-h-screen max-md:overflow-auto dark:bg-canvas-dark"
  >
    <div class="mx-auto flex h-full w-full max-w-[1120px] flex-col px-10 max-md:px-6">
      <!-- Nav -->
      <nav class="flex shrink-0 items-center justify-between py-4">
        <router-link
          to="/home"
          class="font-serif text-lg font-bold tracking-tight text-mica-text-primary dark:text-mica-text-primary-dark"
          style="font-family: 'New York', 'Iowan Old Style', Georgia, 'Times New Roman', serif"
        >
          {{ siteName }}
        </router-link>

        <div class="flex items-center gap-1">
          <!-- Locale Switcher -->
          <LocaleSwitcher />

          <!-- Doc Link -->
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="rounded-lg px-3 py-1.5 text-[13px] font-medium text-mica-text-secondary transition-all duration-150 hover:bg-black/[0.03] hover:text-mica-text-primary dark:text-mica-text-secondary-dark dark:hover:bg-white/[0.03] dark:hover:text-mica-text-primary-dark"
          >
            {{ t('home.docs') }}
          </a>

          <!-- Theme Toggle -->
          <button
            @click="toggleTheme"
            class="rounded-lg p-1.5 text-mica-text-secondary transition-all duration-150 hover:bg-black/[0.03] hover:text-mica-text-primary dark:text-mica-text-secondary-dark dark:hover:bg-white/[0.03] dark:hover:text-mica-text-primary-dark"
            :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
          >
            <Icon v-if="isDark" name="sun" size="sm" />
            <Icon v-else name="moon" size="sm" />
          </button>

          <!-- Login / Dashboard -->
          <router-link
            v-if="isAuthenticated"
            :to="dashboardPath"
            class="ml-1 inline-flex items-center gap-1.5 rounded-lg bg-mica-text-primary px-3.5 py-1.5 text-[13px] font-medium text-canvas transition-all duration-150 hover:bg-black dark:bg-mica-text-primary-dark dark:text-canvas-dark dark:hover:bg-white"
          >
            {{ t('home.dashboard') }}
            <Icon name="arrowRight" size="xs" :stroke-width="2.5" />
          </router-link>
          <router-link
            v-else
            to="/login"
            class="ml-1 inline-flex items-center rounded-lg bg-mica-text-primary px-3.5 py-1.5 text-[13px] font-medium text-canvas transition-all duration-150 hover:bg-black dark:bg-mica-text-primary-dark dark:text-canvas-dark dark:hover:bg-white"
          >
            {{ t('home.login') }}
          </router-link>
        </div>
      </nav>

      <!-- Hero — vertically centered with optical correction -->
      <div class="flex flex-1 items-center pb-12">
        <section class="w-full">
          <h1
            class="mb-6 text-[76px] font-extrabold leading-[0.9] tracking-[-4px] text-mica-text-primary max-md:text-[52px] max-md:tracking-[-2.5px] max-[480px]:text-[42px] max-[480px]:tracking-[-2px] dark:text-mica-text-primary-dark"
          >
            {{ t('home.heroTitle.line1') }}<br />
            {{ t('home.heroTitle.line2') }}<br />
            <span class="text-[#c7c7c9] dark:text-[#48484a]">{{ t('home.heroTitle.line3') }}</span>
          </h1>

          <p
            class="mb-8 max-w-[420px] text-[17px] font-normal leading-[1.6] tracking-[-0.1px] text-mica-text-secondary max-md:text-base dark:text-mica-text-secondary-dark"
          >
            {{ t('home.heroDesc') }}
          </p>

          <div class="mb-10 flex items-center gap-2 max-[480px]:flex-col max-[480px]:items-start">
            <router-link
              :to="isAuthenticated ? dashboardPath : '/login'"
              class="inline-flex items-center gap-[7px] rounded-lg bg-mica-text-primary px-5 py-2.5 text-sm font-medium tracking-[-0.1px] text-canvas transition-all duration-150 hover:-translate-y-px hover:bg-black dark:bg-mica-text-primary-dark dark:text-canvas-dark dark:hover:bg-white"
            >
              {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              <Icon name="arrowRight" size="xs" :stroke-width="2.5" />
            </router-link>
            <a
              v-if="docUrl"
              :href="docUrl"
              target="_blank"
              rel="noopener noreferrer"
              class="inline-flex items-center rounded-lg border border-black/[0.06] bg-white/50 px-5 py-2.5 text-sm font-medium tracking-[-0.1px] text-mica-text-secondary transition-all duration-150 hover:border-black/[0.1] hover:bg-white/80 hover:text-mica-text-primary dark:border-white/[0.08] dark:bg-white/[0.05] dark:text-mica-text-secondary-dark dark:hover:border-white/[0.12] dark:hover:bg-white/[0.08] dark:hover:text-mica-text-primary-dark"
            >
              {{ t('home.documentation') }}
            </a>
          </div>

          <!-- Code Strip -->
          <div
            class="mb-4 flex items-center gap-2.5 rounded-lg border border-black/[0.04] bg-black/[0.03] px-5 py-3.5 font-mono text-[13px] text-mica-text-secondary max-md:overflow-x-auto max-md:text-xs max-[480px]:gap-1.5 max-[480px]:px-3.5 max-[480px]:py-3 max-[480px]:text-[11px] dark:border-white/[0.06] dark:bg-white/[0.04] dark:text-mica-text-secondary-dark"
          >
            <span class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark">$</span>
            <span class="font-medium text-mica-text-primary dark:text-mica-text-primary-dark">curl</span>
            <span class="text-[#bf5af2]">-X POST</span>
            <span class="text-[#007aff] dark:text-[#0a84ff]">/v1/messages</span>
            <span class="flex-1"></span>
            <span class="text-[11px] text-mica-text-tertiary dark:text-mica-text-tertiary-dark">→</span>
            <span class="font-semibold text-status-green dark:text-status-green">200 OK</span>
          </div>

          <!-- Metrics -->
          <div class="flex items-center gap-1.5 pl-0.5 text-xs tracking-[-0.1px] text-mica-text-tertiary max-[480px]:flex-wrap dark:text-mica-text-tertiary-dark">
            <span>{{ t('home.metrics.autoscaling') }}</span>
            <span class="text-[10px] text-[#d2d2d7] dark:text-[#48484a]">·</span>
            <span>{{ t('home.metrics.sla') }}</span>
            <span class="text-[10px] text-[#d2d2d7] dark:text-[#48484a]">·</span>
            <span>{{ t('home.metrics.deploys') }}</span>
          </div>
        </section>
      </div>

      <!-- Footer -->
      <footer class="flex shrink-0 items-center justify-between py-4 max-md:flex-col max-md:gap-2">
        <p class="text-xs text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
          &copy; {{ currentYear }} {{ siteName }}. {{ t('home.footer.allRightsReserved') }}
        </p>
        <div class="flex gap-4">
          <a
            v-if="docUrl"
            :href="docUrl"
            target="_blank"
            rel="noopener noreferrer"
            class="text-xs text-mica-text-tertiary transition-colors duration-150 hover:text-mica-text-primary dark:text-mica-text-tertiary-dark dark:hover:text-mica-text-primary-dark"
          >
            {{ t('home.docs') }}
          </a>
        </div>
      </footer>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAuthStore, useAppStore } from '@/stores'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()

const authStore = useAuthStore()
const appStore = useAppStore()

const siteName = computed(() => appStore.cachedPublicSettings?.site_name || appStore.siteName || 'Sub2API')
const docUrl = computed(() => appStore.cachedPublicSettings?.doc_url || appStore.docUrl || '')
const homeContent = computed(() => appStore.cachedPublicSettings?.home_content || '')

const isHomeContentUrl = computed(() => {
  const content = homeContent.value.trim()
  return content.startsWith('http://') || content.startsWith('https://')
})

const isDark = ref(document.documentElement.classList.contains('dark'))
const isAuthenticated = computed(() => authStore.isAuthenticated)
const isAdmin = computed(() => authStore.isAdmin)
const dashboardPath = computed(() => isAdmin.value ? '/admin/dashboard' : '/dashboard')
const currentYear = computed(() => new Date().getFullYear())

function toggleTheme() {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
}

function initTheme() {
  const savedTheme = localStorage.getItem('theme')
  if (
    savedTheme === 'dark' ||
    (!savedTheme && window.matchMedia('(prefers-color-scheme: dark)').matches)
  ) {
    isDark.value = true
    document.documentElement.classList.add('dark')
  }
}

onMounted(() => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    appStore.fetchPublicSettings()
  }
})
</script>
