<template>
  <AppLayout>
    <div class="space-y-3">
      <div v-if="grafanaDashboardUrl" class="flex justify-end">
        <a
          :href="grafanaDashboardUrl"
          target="_blank"
          rel="noopener noreferrer"
          class="btn btn-secondary btn-sm inline-flex items-center gap-1.5"
        >
          <Icon name="externalLink" size="sm" />
          <span>{{ t('admin.dashboard.openInNewTab') }}</span>
        </a>
      </div>

      <div
        v-if="grafanaDashboardUrl"
        class="overflow-hidden rounded-xl border border-gray-200 bg-white dark:border-dark-700 dark:bg-dark-800"
      >
        <iframe
          :src="grafanaDashboardUrl"
          class="h-[calc(100vh-168px)] min-h-[560px] w-full border-0"
          loading="lazy"
          referrerpolicy="strict-origin-when-cross-origin"
        />
      </div>

      <div v-else class="card px-6 py-12">
        <EmptyState
          :title="t('admin.dashboard.grafanaNotReadyTitle')"
          :description="
            grafanaConfigState === 'missing'
              ? t('admin.dashboard.grafanaMissingDescription')
              : t('admin.dashboard.grafanaInvalidDescription')
          "
        >
          <template #icon>
            <div class="flex h-10 w-10 items-center justify-center rounded-lg bg-gray-100 dark:bg-dark-700">
              <Icon name="chart" size="md" class="text-gray-500 dark:text-gray-300" />
            </div>
          </template>
        </EmptyState>
      </div>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import AppLayout from '@/components/layout/AppLayout.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()

const GRAFANA_DASHBOARD_UID = 'sub2api-admin-overview'
const GRAFANA_DASHBOARD_SLUG = 'sub2api-admin-overview'

const rawGrafanaUrl = computed(() => {
  return appStore.cachedPublicSettings?.grafana_url ?? appStore.grafanaUrl ?? ''
})

const grafanaDashboardUrl = computed(() => buildGrafanaDashboardUrl(rawGrafanaUrl.value))

const grafanaConfigState = computed<'missing' | 'invalid'>(() => {
  if (!rawGrafanaUrl.value.trim()) {
    return 'missing'
  }
  return 'invalid'
})

function buildGrafanaDashboardUrl(rawUrl: string): string | null {
  const trimmed = rawUrl.trim()
  if (!trimmed) {
    return null
  }

  try {
    const parsed = new URL(trimmed)
    if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
      return null
    }

    // Accept a plain Grafana base URL or an already deep Grafana route and normalize both.
    const dashboardPathIndex = parsed.pathname.indexOf('/d/')
    const basePath = (dashboardPathIndex >= 0 ? parsed.pathname.slice(0, dashboardPathIndex) : parsed.pathname)
      .replace(/\/+$/, '')
    parsed.pathname = `${basePath}/d/${GRAFANA_DASHBOARD_UID}/${GRAFANA_DASHBOARD_SLUG}`.replace(/\/{2,}/g, '/')

    return parsed.toString()
  } catch {
    return null
  }
}
</script>
