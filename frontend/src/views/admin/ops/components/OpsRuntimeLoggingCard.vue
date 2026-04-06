<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import Select from '@/components/common/Select.vue'
import { opsAPI, type OpsRuntimeLogConfig } from '@/api/admin/ops'
import { useAppStore } from '@/stores'

const appStore = useAppStore()

const loading = ref(false)
const saving = ref(false)
const config = reactive<OpsRuntimeLogConfig>({
  level: 'info',
  enable_sampling: false,
  sampling_initial: 100,
  sampling_thereafter: 100,
  caller: true,
  stacktrace_level: 'error',
  retention_days: 30
})

const levelOptions = [
  { value: 'debug', label: 'debug' },
  { value: 'info', label: 'info' },
  { value: 'warn', label: 'warn' },
  { value: 'error', label: 'error' }
]

const stacktraceOptions = [
  { value: 'none', label: 'none' },
  { value: 'error', label: 'error' },
  { value: 'fatal', label: 'fatal' }
]

function assign(next: OpsRuntimeLogConfig) {
  config.level = next.level
  config.enable_sampling = next.enable_sampling
  config.sampling_initial = next.sampling_initial
  config.sampling_thereafter = next.sampling_thereafter
  config.caller = next.caller
  config.stacktrace_level = next.stacktrace_level
  config.retention_days = next.retention_days
}

async function load() {
  loading.value = true
  try {
    assign(await opsAPI.getRuntimeLogConfig())
  } catch (err: any) {
    console.error('[OpsRuntimeLoggingCard] Failed to load runtime log config', err)
    appStore.showError(err?.response?.data?.detail || 'Failed to load runtime log configuration')
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    assign(await opsAPI.updateRuntimeLogConfig({ ...config }))
    appStore.showSuccess('Runtime log configuration updated')
  } catch (err: any) {
    console.error('[OpsRuntimeLoggingCard] Failed to save runtime log config', err)
    appStore.showError(err?.response?.data?.detail || 'Failed to save runtime log configuration')
  } finally {
    saving.value = false
  }
}

async function reset() {
  saving.value = true
  try {
    assign(await opsAPI.resetRuntimeLogConfig())
    appStore.showSuccess('Runtime log configuration reset')
  } catch (err: any) {
    console.error('[OpsRuntimeLoggingCard] Failed to reset runtime log config', err)
    appStore.showError(err?.response?.data?.detail || 'Failed to reset runtime log configuration')
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  load()
})
</script>

<template>
  <section class="rounded-2xl border border-gray-200 bg-white p-5 shadow-sm dark:border-dark-700 dark:bg-dark-900/60">
    <div class="mb-4 flex items-start justify-between gap-3">
      <div>
        <h3 class="text-sm font-semibold text-gray-900 dark:text-white">Runtime Logging</h3>
        <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">
          Change log verbosity without restarting the service.
        </p>
      </div>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="loading" @click="load">
        {{ loading ? 'Loading...' : 'Refresh' }}
      </button>
    </div>

    <div class="grid grid-cols-1 gap-4 md:grid-cols-2">
      <label class="text-xs text-gray-600 dark:text-gray-300">
        Level
        <Select v-model="config.level" class="mt-1" :options="levelOptions" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        Stacktrace Threshold
        <Select v-model="config.stacktrace_level" class="mt-1" :options="stacktraceOptions" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        Sampling Initial
        <input v-model.number="config.sampling_initial" class="input mt-1" min="1" type="number" />
      </label>
      <label class="text-xs text-gray-600 dark:text-gray-300">
        Sampling Thereafter
        <input v-model.number="config.sampling_thereafter" class="input mt-1" min="1" type="number" />
      </label>
    </div>

    <div class="mt-4 flex flex-wrap items-center gap-4">
      <label class="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
        <input v-model="config.caller" type="checkbox" />
        Include caller
      </label>
      <label class="inline-flex items-center gap-2 text-xs text-gray-600 dark:text-gray-300">
        <input v-model="config.enable_sampling" type="checkbox" />
        Enable sampling
      </label>
    </div>

    <div class="mt-5 flex flex-wrap gap-2">
      <button type="button" class="btn btn-primary btn-sm" :disabled="saving" @click="save">
        {{ saving ? 'Saving...' : 'Save' }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" :disabled="saving" @click="reset">
        Reset
      </button>
    </div>
  </section>
</template>
