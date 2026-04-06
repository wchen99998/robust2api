<template>
  <AppLayout>
    <div class="space-y-6 pb-12">
      <section class="rounded-2xl border border-gray-200 bg-white p-6 shadow-sm dark:border-dark-700 dark:bg-dark-900/60">
        <div class="flex flex-col gap-4 lg:flex-row lg:items-end lg:justify-between">
          <div>
            <h1 class="text-2xl font-semibold text-gray-900 dark:text-white">Operations</h1>
            <p class="mt-2 max-w-2xl text-sm text-gray-500 dark:text-gray-400">
              Error drill-down, live concurrency and account availability, and runtime logging controls.
            </p>
          </div>

          <div class="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <label class="text-xs text-gray-600 dark:text-gray-300">
              Time Range
              <select v-model="timeRange" class="input mt-1">
                <option value="5m">5m</option>
                <option value="30m">30m</option>
                <option value="1h">1h</option>
                <option value="6h">6h</option>
                <option value="24h">24h</option>
              </select>
            </label>
            <label class="text-xs text-gray-600 dark:text-gray-300">
              Platform
              <input v-model.trim="platform" class="input mt-1" placeholder="openai / anthropic / gemini" type="text" />
            </label>
            <label class="text-xs text-gray-600 dark:text-gray-300">
              Group ID
              <input v-model.trim="groupIdInput" class="input mt-1" min="1" placeholder="Optional" type="number" />
            </label>
            <div class="flex items-end">
              <button type="button" class="btn btn-primary btn-sm w-full" :disabled="loading" @click="refresh">
                {{ loading ? 'Refreshing...' : 'Refresh' }}
              </button>
            </div>
          </div>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary btn-sm" @click="openErrorDetails('request')">
            All Request Errors
          </button>
          <button type="button" class="btn btn-secondary btn-sm" @click="openErrorDetails('upstream')">
            All Upstream Errors
          </button>
          <button type="button" class="btn btn-secondary btn-sm" @click="showRequestDetails = true">
            Request Drill-down
          </button>
        </div>
      </section>

      <div class="grid grid-cols-1 gap-6 xl:grid-cols-2">
        <OpsConcurrencyCard
          :group-id-filter="groupId"
          :platform-filter="platform"
          :refresh-token="refreshToken"
        />
        <OpsRuntimeLoggingCard />
      </div>

      <section class="rounded-2xl border border-gray-200 bg-white p-4 shadow-sm dark:border-dark-700 dark:bg-dark-900/60">
        <div class="mb-4 flex items-start justify-between gap-3">
          <div>
            <h2 class="text-sm font-semibold text-gray-900 dark:text-white">Recent Request Errors</h2>
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
              Client-visible failures retained for replay and resolution workflows.
            </p>
          </div>
          <span v-if="errorMessage" class="text-xs text-red-600 dark:text-red-400">{{ errorMessage }}</span>
        </div>

        <OpsErrorLogTable
          :loading="loading"
          :page="page"
          :page-size="pageSize"
          :rows="rows"
          :total="total"
          @open-error-detail="openRequestError"
          @update:page="page = $event"
          @update:page-size="pageSize = $event"
        />
      </section>

      <OpsErrorDetailsModal
        :show="showErrorDetails"
        :error-type="errorDetailsType"
        :group-id="groupId"
        :platform="platform"
        :time-range="timeRange"
        @open-error-detail="openModalError"
        @update:show="showErrorDetails = $event"
      />

      <OpsErrorDetailModal
        v-model:show="showErrorModal"
        :error-id="selectedErrorId"
        :error-type="selectedErrorType"
      />

      <OpsRequestDetailsModal
        v-model="showRequestDetails"
        :group-id="groupId"
        :platform="platform"
        :preset="requestDetailsPreset"
        :time-range="timeRange"
        @open-error-detail="openModalError"
      />
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { opsAPI, type OpsErrorLog } from '@/api/admin/ops'
import OpsConcurrencyCard from './components/OpsConcurrencyCard.vue'
import OpsErrorDetailModal from './components/OpsErrorDetailModal.vue'
import OpsErrorDetailsModal from './components/OpsErrorDetailsModal.vue'
import OpsErrorLogTable from './components/OpsErrorLogTable.vue'
import OpsRequestDetailsModal, { type OpsRequestDetailsPreset } from './components/OpsRequestDetailsModal.vue'
import OpsRuntimeLoggingCard from './components/OpsRuntimeLoggingCard.vue'

const timeRange = ref<'5m' | '30m' | '1h' | '6h' | '24h'>('1h')
const platform = ref('')
const groupIdInput = ref('')

const loading = ref(false)
const errorMessage = ref('')
const rows = ref<OpsErrorLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const refreshToken = ref(0)

const showErrorDetails = ref(false)
const errorDetailsType = ref<'request' | 'upstream'>('request')
const showErrorModal = ref(false)
const selectedErrorId = ref<number | null>(null)
const selectedErrorType = ref<'request' | 'upstream'>('request')
const showRequestDetails = ref(false)

const requestDetailsPreset = ref<OpsRequestDetailsPreset>({
  title: 'Request Drill-down',
  kind: 'all',
  sort: 'created_at_desc'
})

const groupId = computed<number | null>(() => {
  const value = Number.parseInt(groupIdInput.value, 10)
  return Number.isFinite(value) && value > 0 ? value : null
})

async function loadRequestErrors() {
  loading.value = true
  errorMessage.value = ''
  try {
    const response = await opsAPI.listRequestErrors({
      page: page.value,
      page_size: pageSize.value,
      time_range: timeRange.value,
      platform: platform.value || undefined,
      group_id: groupId.value,
      view: 'errors'
    })
    rows.value = response.items || []
    total.value = response.total || 0
  } catch (err: any) {
    console.error('[OpsDashboard] Failed to load request errors', err)
    rows.value = []
    total.value = 0
    errorMessage.value = err?.response?.data?.detail || 'Failed to load request errors'
  } finally {
    loading.value = false
  }
}

function refresh() {
  refreshToken.value++
  void loadRequestErrors()
}

function openErrorDetails(type: 'request' | 'upstream') {
  errorDetailsType.value = type
  showErrorDetails.value = true
}

function openModalError(errorId: number, type: 'request' | 'upstream' = errorDetailsType.value) {
  selectedErrorId.value = errorId
  selectedErrorType.value = type
  showErrorModal.value = true
}

function openRequestError(errorId: number) {
  openModalError(errorId, 'request')
}

watch([timeRange, platform, groupId], () => {
  page.value = 1
  void loadRequestErrors()
})

watch([page, pageSize], () => {
  void loadRequestErrors()
})

onMounted(() => {
  void loadRequestErrors()
})
</script>
