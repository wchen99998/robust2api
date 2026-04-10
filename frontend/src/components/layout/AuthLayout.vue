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

const siteName = computed(() => appStore.siteName || 'Sub2API')
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
  background: #f2f0ed;
}

:global(.dark) .auth-split {
  background: #1c1c1e;
}

/* Left pane - PixelBlast */
.auth-split__left {
  flex: 1;
  position: relative;
  background: #f2f0ed;
  overflow: hidden;
}

:global(.dark) .auth-split__left {
  background: #1c1c1e;
}

/* Right pane - Form */
.auth-split__right {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px;
  background: #f2f0ed;
}

:global(.dark) .auth-split__right {
  background: #1c1c1e;
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
  color: #1d1d1f;
  margin-bottom: 2px;
}

:global(.dark) .auth-split__site-name {
  color: #f5f5f7;
}

.auth-split__subtitle {
  font-size: 13px;
  font-weight: 500;
  color: #6e6e73;
}

:global(.dark) .auth-split__subtitle {
  color: #a1a1a6;
}

.auth-split__card {
  width: 100%;
  animation: fade-up 500ms cubic-bezier(0.16, 1, 0.3, 1) 60ms both;
}

/* Mica-style form overrides */
.auth-split__card :deep(h2) {
  color: #1d1d1f;
  font-size: 24px;
  font-weight: 600;
  letter-spacing: -0.3px;
}

:global(.dark) .auth-split__card :deep(h2) {
  color: #f5f5f7;
}

.auth-split__card :deep(.text-mica-text-secondary) {
  color: #6e6e73;
}

:global(.dark) .auth-split__card :deep(.text-mica-text-secondary) {
  color: #a1a1a6;
}

.auth-split__card :deep(.text-mica-text-tertiary) {
  color: #aeaeb2;
}

:global(.dark) .auth-split__card :deep(.text-mica-text-tertiary) {
  color: #636366;
}

.auth-split__card :deep(.input-label) {
  color: #1d1d1f;
  font-size: 13px;
  font-weight: 500;
}

:global(.dark) .auth-split__card :deep(.input-label) {
  color: #f5f5f7;
}

.auth-split__card :deep(.input) {
  background-color: #ffffff;
  border: 1px solid #e5e3e0;
  border-radius: 8px;
  color: #1d1d1f;
  font-size: 15px;
  transition: border-color 0.2s ease, box-shadow 0.2s ease;
}

:global(.dark) .auth-split__card :deep(.input) {
  background-color: #2c2c2e;
  border-color: #3a3a3c;
  color: #f5f5f7;
}

.auth-split__card :deep(.input:focus) {
  border-color: #007aff;
  box-shadow: 0 0 0 3px rgba(0, 122, 255, 0.12);
  outline: none;
}

:global(.dark) .auth-split__card :deep(.input:focus) {
  border-color: #0a84ff;
  box-shadow: 0 0 0 3px rgba(10, 132, 255, 0.2);
}

.auth-split__card :deep(.input::placeholder) {
  color: #aeaeb2;
}

:global(.dark) .auth-split__card :deep(.input::placeholder) {
  color: #636366;
}

.auth-split__card :deep(.btn-primary) {
  background-color: #1d1d1f;
  color: #ffffff;
  border-radius: 8px;
  font-weight: 600;
  font-size: 15px;
  padding: 10px 20px;
  transition: background-color 0.18s ease, transform 0.18s ease;
}

:global(.dark) .auth-split__card :deep(.btn-primary) {
  background-color: #f5f5f7;
  color: #1c1c1e;
}

.auth-split__card :deep(.btn-primary:hover) {
  background-color: #3a3a3c;
}

:global(.dark) .auth-split__card :deep(.btn-primary:hover) {
  background-color: #ffffff;
}

.auth-split__card :deep(.btn-primary:active) {
  transform: scale(0.98);
}

/* Password toggle */
.auth-split__card :deep(button:not(.btn)) {
  color: #aeaeb2;
}

:global(.dark) .auth-split__card :deep(button:not(.btn)) {
  color: #636366;
}

.auth-split__card :deep(button:not(.btn):hover) {
  color: #1d1d1f;
}

:global(.dark) .auth-split__card :deep(button:not(.btn):hover) {
  color: #f5f5f7;
}

/* Links */
.auth-split__card :deep(a) {
  color: #007aff;
  font-weight: 500;
}

:global(.dark) .auth-split__card :deep(a) {
  color: #0a84ff;
}

.auth-split__card :deep(a:hover) {
  color: #0066d6;
}

:global(.dark) .auth-split__card :deep(a:hover) {
  color: #409cff;
}

/* Error states */
.auth-split__card :deep(.text-status-red) {
  color: #ff3b30;
}

:global(.dark) .auth-split__card :deep(.text-status-red) {
  color: #ff453a;
}

.auth-split__card :deep(.input-error-text) {
  color: #ff3b30;
}

:global(.dark) .auth-split__card :deep(.input-error-text) {
  color: #ff453a;
}

.auth-split__card :deep(.input-error) {
  border-color: #ff3b30;
}

:global(.dark) .auth-split__card :deep(.input-error) {
  border-color: #ff453a;
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
  color: #6e6e73;
}

:global(.dark) .auth-split__footer-slot :deep(p) {
  color: #a1a1a6;
}

.auth-split__footer-slot :deep(a) {
  color: #007aff;
  font-weight: 600;
}

:global(.dark) .auth-split__footer-slot :deep(a) {
  color: #0a84ff;
}

.auth-split__footer-slot :deep(a:hover) {
  color: #0066d6;
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
  color: #aeaeb2;
  animation: fade-up 500ms cubic-bezier(0.16, 1, 0.3, 1) 180ms both;
}

:global(.dark) .auth-split__copyright {
  color: #636366;
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
</style>
