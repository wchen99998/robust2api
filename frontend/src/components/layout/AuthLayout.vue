<template>
  <div class="auth-split">
    <!-- Left pane: PixelBlast -->
    <div class="auth-split__left">
      <PixelBlast
        variant="circle"
        color="#1d1d1f"
        :pixel-size="5"
        :pattern-scale="2"
        :pattern-density="1"
        :speed="0.5"
        :edge-fade="0.15"
        :enable-ripples="true"
        :ripple-speed="0.3"
        :ripple-thickness="0.1"
        :ripple-intensity-scale="1"
      />
    </div>

    <!-- Right pane: Form -->
    <div class="auth-split__right">
      <div class="auth-split__form">
        <template v-if="settingsLoaded">
          <div class="auth-split__header">
            <div class="auth-split__logo">
              <img :src="siteLogo || '/logo.png'" alt="Logo" />
            </div>
            <h1 class="auth-split__site-name">{{ siteName }}</h1>
            <p class="auth-split__subtitle">{{ siteSubtitle }}</p>
          </div>
        </template>

        <div class="auth-split__card">
          <slot />
        </div>

        <div class="auth-split__footer-slot">
          <slot name="footer" />
        </div>

        <div class="auth-split__copyright">
          &copy; {{ currentYear }} {{ siteName }}
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, defineAsyncComponent } from 'vue'
import { useAppStore } from '@/stores'
import { sanitizeUrl } from '@/utils/url'

const PixelBlast = defineAsyncComponent(() => import('@/components/effects/PixelBlast.vue'))

const appStore = useAppStore()

const siteName = computed(() => appStore.siteName || 'Robust2API')
const siteLogo = computed(() => sanitizeUrl(appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const siteSubtitle = computed(() => appStore.cachedPublicSettings?.site_subtitle || 'Subscription to API Conversion Platform')
const settingsLoaded = computed(() => appStore.publicSettingsLoaded)

const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-split {
  display: flex;
  min-height: 100vh;
  min-height: 100svh;
  background: theme('colors.canvas.DEFAULT');
}

:global(.dark) .auth-split {
  background: theme('colors.canvas.dark');
}

/* Left pane - PixelBlast */
.auth-split__left {
  flex: 1;
  position: relative;
  background: theme('colors.canvas.DEFAULT');
  overflow: hidden;
}

:global(.dark) .auth-split__left {
  background: theme('colors.canvas.dark');
}

/* Right pane - Form */
.auth-split__right {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background: theme('colors.canvas.DEFAULT');
}

:global(.dark) .auth-split__right {
  background: theme('colors.canvas.dark');
}

.auth-split__form {
  width: 100%;
  max-width: 400px;
  display: flex;
  flex-direction: column;
  align-items: center;
}

.auth-split__header {
  text-align: center;
  margin-bottom: 36px;
  animation: fade-down 500ms cubic-bezier(0.16, 1, 0.3, 1) both;
}

.auth-split__logo {
  display: inline-flex;
  width: 48px;
  height: 48px;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  border-radius: 12px;
  margin-bottom: 12px;
}

.auth-split__logo img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.auth-split__site-name {
  font-size: 20px;
  font-weight: 600;
  letter-spacing: -0.3px;
  color: theme('colors.mica-text.primary');
  margin-bottom: 2px;
}

:global(.dark) .auth-split__site-name {
  color: theme('colors.mica-text.primary-dark');
}

.auth-split__subtitle {
  font-size: 13px;
  font-weight: 500;
  color: theme('colors.mica-text.secondary');
}

:global(.dark) .auth-split__subtitle {
  color: theme('colors.mica-text.secondary-dark');
}

.auth-split__card {
  width: 100%;
  animation: fade-up 500ms cubic-bezier(0.16, 1, 0.3, 1) 60ms both;
}

/* Mica-style form overrides */
.auth-split__card :deep(h2) {
  color: theme('colors.mica-text.primary');
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.3px;
}

:global(.dark) .auth-split__card :deep(h2) {
  color: theme('colors.mica-text.primary-dark');
}

.auth-split__card :deep(.text-mica-text-secondary) {
  color: theme('colors.mica-text.secondary');
}

:global(.dark) .auth-split__card :deep(.text-mica-text-secondary) {
  color: theme('colors.mica-text.secondary-dark');
}

.auth-split__card :deep(.text-mica-text-tertiary) {
  color: theme('colors.mica-text.tertiary');
}

:global(.dark) .auth-split__card :deep(.text-mica-text-tertiary) {
  color: theme('colors.mica-text.tertiary-dark');
}

.auth-split__card :deep(.input-label) {
  color: theme('colors.mica-text.primary');
  font-size: 13px;
  font-weight: 500;
}

:global(.dark) .auth-split__card :deep(.input-label) {
  color: theme('colors.mica-text.primary-dark');
}

.auth-split__card :deep(.input) {
  background-color: theme('colors.surface.DEFAULT');
  border: 1px solid theme('colors.dark.200');
  border-radius: 8px;
  color: theme('colors.mica-text.primary');
  font-size: 15px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

:global(.dark) .auth-split__card :deep(.input) {
  background-color: theme('colors.surface.solid-dark');
  border-color: theme('colors.dark.700');
  color: theme('colors.mica-text.primary-dark');
}

.auth-split__card :deep(.input:focus) {
  border-color: theme('colors.status-blue.DEFAULT');
  box-shadow: 0 0 0 3px rgba(0, 122, 255, 0.12);
  outline: none;
}

:global(.dark) .auth-split__card :deep(.input:focus) {
  border-color: theme('colors.status-blue.dark');
  box-shadow: 0 0 0 3px rgba(10, 132, 255, 0.2);
}

.auth-split__card :deep(.input::placeholder) {
  color: theme('colors.mica-text.tertiary');
}

:global(.dark) .auth-split__card :deep(.input::placeholder) {
  color: theme('colors.mica-text.tertiary-dark');
}

.auth-split__card :deep(.btn-primary) {
  background-color: theme('colors.mica-text.primary');
  color: theme('colors.surface.DEFAULT');
  border-radius: 8px;
  font-weight: 600;
  font-size: 15px;
  padding: 10px 20px;
  transition: background-color 0.18s ease, transform 0.18s ease;
}

:global(.dark) .auth-split__card :deep(.btn-primary) {
  background-color: theme('colors.mica-text.primary-dark');
  color: theme('colors.canvas.dark');
}

.auth-split__card :deep(.btn-primary:hover) {
  background-color: theme('colors.dark.700');
}

:global(.dark) .auth-split__card :deep(.btn-primary:hover) {
  background-color: theme('colors.surface.DEFAULT');
}

.auth-split__card :deep(.btn-primary:active) {
  transform: scale(0.98);
}

/* Password toggle */
.auth-split__card :deep(button:not(.btn)) {
  color: theme('colors.mica-text.tertiary');
}

:global(.dark) .auth-split__card :deep(button:not(.btn)) {
  color: theme('colors.mica-text.tertiary-dark');
}

.auth-split__card :deep(button:not(.btn):hover) {
  color: theme('colors.mica-text.primary');
}

:global(.dark) .auth-split__card :deep(button:not(.btn):hover) {
  color: theme('colors.mica-text.primary-dark');
}

/* Links */
.auth-split__card :deep(a) {
  color: theme('colors.status-blue.DEFAULT');
  font-weight: 500;
}

:global(.dark) .auth-split__card :deep(a) {
  color: theme('colors.status-blue.dark');
}

.auth-split__card :deep(a:hover) {
  color: theme('colors.primary.700');
}

:global(.dark) .auth-split__card :deep(a:hover) {
  color: #409cff;
}

/* Error states */
.auth-split__card :deep(.text-status-red) {
  color: theme('colors.status-red.DEFAULT');
}

:global(.dark) .auth-split__card :deep(.text-status-red) {
  color: theme('colors.status-red.dark');
}

.auth-split__card :deep(.input-error-text) {
  color: theme('colors.status-red.DEFAULT');
}

:global(.dark) .auth-split__card :deep(.input-error-text) {
  color: theme('colors.status-red.dark');
}

.auth-split__card :deep(.input-error) {
  border-color: theme('colors.status-red.DEFAULT');
}

:global(.dark) .auth-split__card :deep(.input-error) {
  border-color: theme('colors.status-red.dark');
}

.auth-split__card :deep(.input-error:focus) {
  box-shadow: 0 0 0 3px rgba(255, 59, 48, 0.12);
}

:global(.dark) .auth-split__card :deep(.input-error:focus) {
  box-shadow: 0 0 0 3px rgba(255, 69, 58, 0.2);
}

/* Footer slot */
.auth-split__footer-slot {
  margin-top: 28px;
  text-align: center;
  font-size: 13px;
  font-weight: 500;
  animation: fade-up 500ms cubic-bezier(0.16, 1, 0.3, 1) 120ms both;
}

.auth-split__footer-slot :deep(p) {
  color: theme('colors.mica-text.secondary');
}

:global(.dark) .auth-split__footer-slot :deep(p) {
  color: theme('colors.mica-text.secondary-dark');
}

.auth-split__footer-slot :deep(a) {
  color: theme('colors.status-blue.DEFAULT');
  font-weight: 600;
}

:global(.dark) .auth-split__footer-slot :deep(a) {
  color: theme('colors.status-blue.dark');
}

.auth-split__footer-slot :deep(a:hover) {
  color: theme('colors.primary.700');
}

:global(.dark) .auth-split__footer-slot :deep(a:hover) {
  color: #409cff;
}

.auth-split__copyright {
  margin-top: 36px;
  text-align: center;
  font-size: 11px;
  letter-spacing: 0.3px;
  font-weight: 500;
  color: theme('colors.mica-text.tertiary');
  animation: fade-up 500ms cubic-bezier(0.16, 1, 0.3, 1) 180ms both;
}

:global(.dark) .auth-split__copyright {
  color: theme('colors.mica-text.tertiary-dark');
}

/* Mobile: stack vertically, hide pixel blast */
@media (max-width: 768px) {
  .auth-split {
    flex-direction: column;
  }

  .auth-split__left {
    flex: none;
    height: 180px;
  }

  .auth-split__right {
    flex: 1;
    padding: 24px 20px;
  }
}

@keyframes fade-down {
  from {
    opacity: 0;
    transform: translateY(-10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@keyframes fade-up {
  from {
    opacity: 0;
    transform: translateY(12px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (prefers-reduced-motion: reduce) {
  .auth-split__header,
  .auth-split__card,
  .auth-split__footer-slot,
  .auth-split__copyright {
    animation: none;
  }
}
</style>
