<template>
  <!-- Custom Home Content: Full Page Mode -->
  <div v-if="homeContent" class="min-h-screen">
    <iframe
      v-if="isHomeContentUrl"
      :src="homeContent.trim()"
      class="h-screen w-full border-0"
      title="Custom content"
      allowfullscreen
    ></iframe>
    <!-- SECURITY: homeContent is admin-only setting, XSS risk is acceptable -->
    <div v-else v-html="homeContent"></div>
  </div>

  <!-- Default Home Page -->
  <div v-else class="home-glitch">
    <div class="home-glitch__bg">
      <LetterGlitch
        :glitch-colors="['#1d1d1f', '#6e6e73', '#007aff', '#aeaeb2']"
        :glitch-speed="38"
        :smooth="true"
        :outer-vignette="false"
        :center-vignette="false"
        characters="ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%&*<>/\|{}[]+=~:;()_-"
      />
    </div>
    <div class="home-glitch__shade"></div>
    <div class="home-glitch__scanlines"></div>
    <div class="home-glitch__bloom home-glitch__bloom--left"></div>
    <div class="home-glitch__bloom home-glitch__bloom--right"></div>

    <div class="home-glitch__content">
      <nav class="glass-nav">
        <div class="glass-nav__surface">
          <div class="glass-nav__inner">
            <router-link to="/home" class="glass-nav__logo">
              {{ siteName }}
            </router-link>

            <div class="glass-nav__links">
              <router-link
                v-if="isAuthenticated"
                :to="dashboardPath"
                class="glass-nav__link"
              >
                {{ t('home.dashboard') }}
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="glass-nav__link"
              >
                {{ t('home.docs') }}
              </a>
              <div class="glass-nav__locale">
                <LocaleSwitcher />
              </div>
              <button @click="toggleTheme" class="glass-nav__link glass-nav__link--icon" :title="isDark ? t('home.switchToLight') : t('home.switchToDark')" :aria-label="isDark ? t('home.switchToLight') : t('home.switchToDark')">
                <Icon v-if="isDark" name="sun" size="sm" />
                <Icon v-else name="moon" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </nav>

      <main class="glass-stage">
        <div class="glass-stage__halo glass-stage__halo--left"></div>
        <div class="glass-stage__halo glass-stage__halo--right"></div>
        <div class="glass-hero__surface">
          <section class="glass-hero">
            <div class="glass-hero__badge">
              <span class="glass-hero__badge-icon">
                <Icon name="terminal" size="xs" />
              </span>
              <span>{{ siteName }}</span>
              <span class="glass-hero__badge-divider"></span>
              <span class="glass-hero__badge-copy">/v1 · /responses · /chat</span>
            </div>

            <h1 class="glass-hero__title">
              <span class="glass-hero__title-row">
                <span>{{ t('home.heroTitle.line1') }}</span>
                <span>{{ t('home.heroTitle.line2') }}</span>
              </span>
              <span class="glass-hero__title-row glass-hero__title-row--accent">
                {{ t('home.heroTitle.line3') }}
              </span>
            </h1>

            <p class="glass-hero__desc">
              {{ t('home.heroDesc') }}
            </p>

            <div class="glass-hero__actions">
              <router-link
                :to="isAuthenticated ? dashboardPath : '/login'"
                class="glass-hero__btn glass-hero__btn--primary"
              >
                {{ isAuthenticated ? t('home.goToDashboard') : t('home.getStarted') }}
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="glass-hero__btn glass-hero__btn--secondary"
              >
                {{ t('home.documentation') }}
              </a>
            </div>

            <div class="glass-hero__proof">
              <div class="glass-hero__code">
                <span class="glass-hero__code-prompt">$</span>
                <span class="glass-hero__code-cmd">curl</span>
                <span class="glass-hero__code-flag">-X POST</span>
                <span class="glass-hero__code-url">/v1/messages</span>
                <span class="glass-hero__code-spacer"></span>
                <span class="glass-hero__code-arrow">→</span>
                <span class="glass-hero__code-ok">200 OK</span>
              </div>

              <div class="glass-hero__metrics">
                <span>{{ t('home.metrics.autoscaling') }}</span>
                <span class="glass-hero__metrics-dot">·</span>
                <span>{{ t('home.metrics.sla') }}</span>
                <span class="glass-hero__metrics-dot">·</span>
                <span>{{ t('home.metrics.deploys') }}</span>
              </div>
            </div>
          </section>
        </div>
      </main>

      <footer class="glass-footer">
        <span>&copy; {{ currentYear }} {{ siteName }}</span>
        <span class="glass-footer__dot"></span>
        <span>{{ t('home.footer.allRightsReserved') }}</span>
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
import LetterGlitch from '@/components/effects/LetterGlitch.vue'


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

<style scoped>
.home-glitch {
  position: relative;
  width: 100%;
  min-height: 100vh;
  min-height: 100svh;
  overflow: clip;
  font-family: inherit;
  color: theme('colors.mica-text.primary');
}

:global(.dark) .home-glitch {
  color: theme('colors.mica-text.primary-dark');
}

@media (max-width: 768px) {
  .home-glitch {
    overflow: hidden;
  }
}

.home-glitch__bg {
  position: absolute;
  inset: 0;
  z-index: 0;
}

.home-glitch__shade,
.home-glitch__scanlines,
.home-glitch__bloom {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.home-glitch__shade {
  z-index: 1;
  background:
    radial-gradient(circle at 50% 42%, transparent 0%, transparent 35%, rgba(0, 0, 0, 0.04) 60%, rgba(0, 0, 0, 0.15) 100%);
}

.home-glitch__scanlines {
  z-index: 2;
  opacity: 0.06;
  background-image: repeating-linear-gradient(
    180deg,
    rgba(255, 255, 255, 0.045) 0,
    rgba(255, 255, 255, 0.045) 1px,
    transparent 1px,
    transparent 4px
  );
  mix-blend-mode: screen;
}

.home-glitch__bloom {
  z-index: 2;
  filter: blur(90px);
  opacity: 0.45;
}

.home-glitch__bloom--left {
  background: radial-gradient(circle at 25% 22%, rgba(0, 122, 255, 0.35) 0%, transparent 32%);
}

.home-glitch__bloom--right {
  background: radial-gradient(circle at 76% 58%, rgba(174, 174, 178, 0.34) 0%, transparent 28%);
}

.home-glitch__content {
  position: relative;
  z-index: 3;
  width: 100%;
  min-height: 100vh;
  min-height: 100svh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: space-between;
  padding: 22px 24px 28px;
}

@media (max-width: 768px) {
  .home-glitch__content {
    min-height: 100svh;
    padding: 16px 14px 20px;
  }
}

.glass-nav {
  position: relative;
  z-index: 10;
  width: 100%;
  max-width: 670px;
  flex-shrink: 0;
  animation: nav-enter 680ms cubic-bezier(0.16, 1, 0.3, 1) both;
}

.glass-nav__surface {
  border-radius: 999px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(20px) saturate(1.8);
  -webkit-backdrop-filter: blur(20px) saturate(1.8);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04), inset 0 1px 0 rgba(255, 255, 255, 0.5);
}

:global(.dark) .glass-nav__surface {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(44, 44, 46, 0.85);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.2), inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.glass-nav__inner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  padding: 9px 14px 9px 18px;
}

.glass-nav__logo {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 15px;
  font-weight: 700;
  color: theme('colors.mica-text.primary');
  text-decoration: none;
  letter-spacing: -0.03em;
}

:global(.dark) .glass-nav__logo {
  color: theme('colors.mica-text.primary-dark');
}

.glass-nav__logo-mark {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  border-radius: 999px;
  background: linear-gradient(180deg, rgba(0, 122, 255, 0.12), rgba(0, 122, 255, 0.04));
  color: theme('colors.status-blue.DEFAULT');
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.5);
}

.glass-nav__links {
  display: flex;
  align-items: center;
  gap: 4px;
}

.glass-nav__link {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 44px;
  padding: 0 15px;
  font-size: 13px;
  font-weight: 600;
  color: theme('colors.mica-text.secondary');
  text-decoration: none;
  border-radius: 999px;
  transition:
    color 0.18s ease,
    background-color 0.18s ease,
    transform 0.18s ease,
    border-color 0.18s ease;
  background: transparent;
  border: none;
  cursor: pointer;
}

:global(.dark) .glass-nav__link {
  color: theme('colors.mica-text.secondary-dark');
}

.glass-nav__link:hover {
  color: theme('colors.mica-text.primary');
  background: rgba(0, 0, 0, 0.04);
  transform: translateY(-1px);
}

:global(.dark) .glass-nav__link:hover {
  color: theme('colors.mica-text.primary-dark');
  background: rgba(255, 255, 255, 0.06);
}

.glass-nav__link--icon {
  min-width: 44px;
  padding: 0;
}

.glass-stage {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  flex: 1;
  padding: 28px 0;
}

.glass-stage__halo {
  position: absolute;
  width: 320px;
  height: 320px;
  border-radius: 999px;
  filter: blur(100px);
  pointer-events: none;
  opacity: 0.18;
}

.glass-stage__halo--left {
  left: calc(50% - 430px);
  top: 28%;
  background: rgba(0, 122, 255, 0.6);
}

.glass-stage__halo--right {
  right: calc(50% - 420px);
  bottom: 20%;
  background: rgba(174, 174, 178, 0.5);
}

.glass-hero__surface {
  width: min(100%, 820px);
  margin: 0 auto;
  border-radius: 34px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(255, 255, 255, 0.92);
  backdrop-filter: blur(20px) saturate(1.8);
  -webkit-backdrop-filter: blur(20px) saturate(1.8);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.04), inset 0 1px 0 rgba(255, 255, 255, 0.5);
  animation: hero-enter 820ms cubic-bezier(0.16, 1, 0.3, 1) 120ms both;
}

:global(.dark) .glass-hero__surface {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(44, 44, 46, 0.85);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.2), inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.glass-hero {
  position: relative;
  width: 100%;
  padding: 42px 46px 32px;
  text-align: center;
}

@media (max-width: 768px) {
  .glass-hero__surface {
    width: min(100%, 640px) !important;
    border-radius: 28px !important;
  }

  .glass-hero {
    padding: 34px 24px 26px;
  }
}

@media (max-width: 480px) {
  .glass-hero__surface {
    border-radius: 24px !important;
  }

  .glass-hero {
    padding: 28px 18px 22px;
  }
}

.glass-hero__badge {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  max-width: 100%;
  padding: 10px 16px;
  margin-bottom: 24px;
  border-radius: 999px;
  border: 1px solid rgba(0, 0, 0, 0.06);
  background: rgba(255, 255, 255, 0.55);
  backdrop-filter: blur(20px);
  -webkit-backdrop-filter: blur(20px);
  color: theme('colors.mica-text.primary');
  font-size: 12px;
  font-weight: 600;
  letter-spacing: 0.02em;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.5);
}

:global(.dark) .glass-hero__badge {
  border-color: rgba(255, 255, 255, 0.08);
  background: rgba(58, 58, 60, 0.5);
  color: theme('colors.mica-text.primary-dark');
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.06);
}

.glass-hero__badge-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  border-radius: 999px;
  background: rgba(0, 122, 255, 0.1);
  color: theme('colors.status-blue.DEFAULT');
}

.glass-hero__badge-divider {
  width: 1px;
  height: 12px;
  background: rgba(0, 0, 0, 0.1);
}

:global(.dark) .glass-hero__badge-divider {
  background: rgba(255, 255, 255, 0.12);
}

.glass-hero__badge-copy {
  color: theme('colors.mica-text.tertiary');
  white-space: nowrap;
}

:global(.dark) .glass-hero__badge-copy {
  color: theme('colors.mica-text.tertiary-dark');
}

@media (max-width: 640px) {
  .glass-hero__badge {
    gap: 8px;
    padding: 8px 12px;
  }

  .glass-hero__badge-copy {
    display: none;
  }
}

.glass-hero__title {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  font-size: 58px;
  font-weight: 800;
  line-height: 0.94;
  letter-spacing: -0.075em;
  color: theme('colors.mica-text.primary');
  margin-bottom: 18px;
}

:global(.dark) .glass-hero__title {
  color: theme('colors.mica-text.primary-dark');
}

.glass-hero__title-row {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 0.28em;
}

@media (max-width: 768px) {
  .glass-hero__title {
    font-size: 46px;
  }
}

@media (max-width: 480px) {
  .glass-hero__title {
    font-size: 34px;
    line-height: 1;
  }
}

.glass-hero__title-row--accent {
  color: theme('colors.status-blue.DEFAULT');
}

:global(.dark) .glass-hero__title-row--accent {
  color: theme('colors.status-blue.dark');
}

.glass-hero__desc {
  max-width: 560px;
  margin: 0 auto 32px;
  font-size: 16px;
  font-weight: 400;
  line-height: 1.7;
  color: theme('colors.mica-text.secondary');
}

:global(.dark) .glass-hero__desc {
  color: theme('colors.mica-text.secondary-dark');
}

@media (max-width: 480px) {
  .glass-hero__desc {
    font-size: 14px;
    margin-bottom: 22px;
  }
}

.glass-hero__actions {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  margin-bottom: 26px;
}

@media (max-width: 480px) {
  .glass-hero__actions {
    flex-direction: column;
    gap: 10px;
  }
}

.glass-hero__btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 168px;
  min-height: 54px;
  padding: 0 28px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: -0.02em;
  border-radius: 999px;
  text-decoration: none;
  transition:
    transform 0.18s ease,
    box-shadow 0.18s ease,
    background-color 0.18s ease,
    color 0.18s ease,
    border-color 0.18s ease;
  cursor: pointer;
  border: 1px solid transparent;
}

.glass-hero__btn--primary {
  color: #ffffff;
  background: theme('colors.accent.DEFAULT');
  box-shadow:
    0 10px 28px rgba(0, 0, 0, 0.15),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

:global(.dark) .glass-hero__btn--primary {
  color: #1c1c1e;
  background: theme('colors.accent.dark');
  box-shadow:
    0 10px 28px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
}

.glass-hero__btn--primary:hover {
  background: #3a3a3c;
  transform: translateY(-1px);
  box-shadow:
    0 14px 34px rgba(0, 0, 0, 0.2),
    inset 0 1px 0 rgba(255, 255, 255, 0.08);
}

:global(.dark) .glass-hero__btn--primary:hover {
  background: #ffffff;
}

.glass-hero__btn--secondary {
  color: theme('colors.mica-text.secondary');
  background: rgba(255, 255, 255, 0.35);
  border-color: rgba(0, 0, 0, 0.08);
  backdrop-filter: blur(16px);
}

:global(.dark) .glass-hero__btn--secondary {
  color: theme('colors.mica-text.secondary-dark');
  background: rgba(255, 255, 255, 0.06);
  border-color: rgba(255, 255, 255, 0.1);
}

.glass-hero__btn--secondary:hover {
  color: theme('colors.mica-text.primary');
  background: rgba(255, 255, 255, 0.5);
  border-color: rgba(0, 0, 0, 0.12);
  transform: translateY(-1px);
}

:global(.dark) .glass-hero__btn--secondary:hover {
  color: theme('colors.mica-text.primary-dark');
  background: rgba(255, 255, 255, 0.1);
  border-color: rgba(255, 255, 255, 0.15);
}

.glass-hero__proof {
  display: grid;
  gap: 14px;
}

.glass-hero__code {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 12px 20px;
  margin: 0 auto;
  max-width: 500px;
  border-radius: 18px;
  font-family: 'SF Mono', ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  background: rgba(29, 29, 31, 0.06);
  border: 1px solid rgba(0, 0, 0, 0.05);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.4);
}

:global(.dark) .glass-hero__code {
  background: rgba(255, 255, 255, 0.04);
  border-color: rgba(255, 255, 255, 0.06);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}

@media (max-width: 480px) {
  .glass-hero__code {
    font-size: 11px;
    gap: 5px;
    padding: 8px 14px;
  }
}

.glass-hero__code-prompt { color: theme('colors.mica-text.tertiary'); }
.glass-hero__code-cmd { font-weight: 600; color: theme('colors.mica-text.primary'); }
.glass-hero__code-flag { color: theme('colors.status-blue.DEFAULT'); }
.glass-hero__code-url { color: theme('colors.status-blue.DEFAULT'); }
.glass-hero__code-spacer { flex: 1; }
.glass-hero__code-arrow { font-size: 10px; color: theme('colors.mica-text.tertiary'); }
.glass-hero__code-ok { font-weight: 600; color: theme('colors.status-green.DEFAULT'); }

:global(.dark) .glass-hero__code-prompt { color: theme('colors.mica-text.tertiary-dark'); }
:global(.dark) .glass-hero__code-cmd { color: theme('colors.mica-text.primary-dark'); }
:global(.dark) .glass-hero__code-flag { color: theme('colors.status-blue.dark'); }
:global(.dark) .glass-hero__code-url { color: theme('colors.status-blue.dark'); }
:global(.dark) .glass-hero__code-arrow { color: theme('colors.mica-text.tertiary-dark'); }
:global(.dark) .glass-hero__code-ok { color: theme('colors.status-green.dark'); }

.glass-hero__metrics {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  font-size: 11px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: theme('colors.mica-text.tertiary');
}

:global(.dark) .glass-hero__metrics {
  color: theme('colors.mica-text.tertiary-dark');
}

@media (max-width: 480px) {
  .glass-hero__metrics {
    flex-wrap: wrap;
  }
}

.glass-hero__metrics-dot {
  color: theme('colors.mica-text.tertiary');
}

.glass-footer {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 12px;
  font-size: 12px;
  color: theme('colors.mica-text.tertiary');
  text-align: center;
  animation: footer-enter 760ms cubic-bezier(0.16, 1, 0.3, 1) 200ms both;
}

:global(.dark) .glass-footer {
  color: theme('colors.mica-text.tertiary-dark');
}

.glass-footer__dot {
  width: 3px;
  height: 3px;
  border-radius: 999px;
  background: theme('colors.mica-text.tertiary');
}

:global(.dark) .glass-footer__dot {
  background: theme('colors.mica-text.tertiary-dark');
}

/* --- Nav button base overrides --- */
.glass-nav :deep(button) {
  min-height: 44px;
  border-radius: 999px;
  color: theme('colors.mica-text.secondary') !important;
  background: transparent !important;
  border: 1px solid transparent !important;
  transition:
    color 0.18s ease,
    background-color 0.18s ease,
    transform 0.18s ease;
}

:global(.dark) .glass-nav :deep(button) {
  color: theme('colors.mica-text.secondary-dark') !important;
}

.glass-nav :deep(button:hover) {
  color: theme('colors.mica-text.primary') !important;
  background: rgba(0, 0, 0, 0.04) !important;
  transform: translateY(-1px);
}

:global(.dark) .glass-nav :deep(button:hover) {
  color: theme('colors.mica-text.primary-dark') !important;
  background: rgba(255, 255, 255, 0.06) !important;
}

/* --- LocaleSwitcher overrides via wrapper class --- */
.glass-nav__locale :deep(button) {
  color: theme('colors.mica-text.secondary') !important;
  font-weight: 600;
}

.glass-nav__locale :deep(button span) {
  color: theme('colors.mica-text.secondary') !important;
}

:global(.dark) .glass-nav__locale :deep(button span) {
  color: theme('colors.mica-text.secondary-dark') !important;
}

.glass-nav__locale :deep(button svg) {
  color: theme('colors.mica-text.tertiary') !important;
}

:global(.dark) .glass-nav__locale :deep(button svg) {
  color: theme('colors.mica-text.tertiary-dark') !important;
}

/* --- LocaleSwitcher dropdown panel --- */
.glass-nav__locale :deep(div[class*="absolute"]) {
  border: 1px solid rgba(0, 0, 0, 0.08) !important;
  background: rgba(255, 255, 255, 0.92) !important;
  backdrop-filter: blur(28px) saturate(150%) !important;
  -webkit-backdrop-filter: blur(28px) saturate(150%) !important;
  box-shadow:
    0 20px 50px rgba(0, 0, 0, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.5) !important;
  border-radius: 14px !important;
  overflow: hidden !important;
}

:global(.dark) .glass-nav__locale :deep(div[class*="absolute"]) {
  border-color: rgba(255, 255, 255, 0.1) !important;
  background: rgba(44, 44, 46, 0.92) !important;
  box-shadow:
    0 20px 50px rgba(0, 0, 0, 0.3),
    inset 0 1px 0 rgba(255, 255, 255, 0.06) !important;
}

/* --- LocaleSwitcher dropdown items --- */
.glass-nav__locale :deep(div[class*="absolute"] button) {
  color: theme('colors.mica-text.primary') !important;
  background: transparent !important;
  border-radius: 0 !important;
  min-height: auto !important;
  padding: 9px 14px !important;
  border: none !important;
}

:global(.dark) .glass-nav__locale :deep(div[class*="absolute"] button) {
  color: theme('colors.mica-text.primary-dark') !important;
}

.glass-nav__locale :deep(div[class*="absolute"] button:hover) {
  color: theme('colors.mica-text.primary') !important;
  background: rgba(0, 0, 0, 0.04) !important;
  transform: none !important;
}

:global(.dark) .glass-nav__locale :deep(div[class*="absolute"] button:hover) {
  color: theme('colors.mica-text.primary-dark') !important;
  background: rgba(255, 255, 255, 0.06) !important;
}

/* Active locale highlight */
.glass-nav__locale :deep(div[class*="absolute"] button.bg-primary-50),
.glass-nav__locale :deep(div[class*="absolute"] button[class*="bg-primary"]) {
  background: rgba(0, 122, 255, 0.08) !important;
  color: theme('colors.status-blue.DEFAULT') !important;
}

:global(.dark) .glass-nav__locale :deep(div[class*="absolute"] button.bg-primary-50),
:global(.dark) .glass-nav__locale :deep(div[class*="absolute"] button[class*="bg-primary"]) {
  background: rgba(10, 132, 255, 0.12) !important;
  color: theme('colors.status-blue.dark') !important;
}

/* Check icon in active item */
.glass-nav__locale :deep(div[class*="absolute"] button svg) {
  color: theme('colors.status-blue.DEFAULT') !important;
}

:global(.dark) .glass-nav__locale :deep(div[class*="absolute"] button svg) {
  color: theme('colors.status-blue.dark') !important;
}

@media (max-width: 768px) {
  .glass-nav__inner {
    width: 100%;
    padding-inline: 12px;
  }

  .glass-nav__logo {
    font-size: 14px;
  }

  .glass-nav__links {
    gap: 2px;
  }

  .glass-stage {
    padding: 18px 0;
  }

  .glass-stage__halo {
    display: none;
  }
}

@media (max-width: 480px) {
  .glass-nav__logo-mark {
    width: 28px;
    height: 28px;
  }

  .glass-nav__link {
    min-height: 44px;
    padding: 0 12px;
    font-size: 12px;
  }

  .glass-hero__btn {
    width: 100%;
    min-width: 0;
  }

  .glass-footer {
    flex-wrap: wrap;
    justify-content: center;
    gap: 6px;
  }
}

@keyframes nav-enter {
  from {
    opacity: 0;
    transform: translateY(-16px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes hero-enter {
  from {
    opacity: 0;
    transform: translateY(28px) scale(0.985);
  }

  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

@keyframes footer-enter {
  from {
    opacity: 0;
    transform: translateY(14px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .glass-nav,
  .glass-hero__surface,
  .glass-footer {
    animation: none;
  }
}
</style>
