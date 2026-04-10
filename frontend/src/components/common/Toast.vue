<template>
  <Teleport to="body">
    <div
      class="pointer-events-none fixed right-4 top-4 z-[9999] space-y-3"
      aria-live="polite"
      aria-atomic="true"
    >
      <TransitionGroup
        enter-active-class="transition ease-out duration-300"
        enter-from-class="opacity-0 translate-x-full"
        enter-to-class="opacity-100 translate-x-0"
        leave-active-class="transition ease-in duration-200"
        leave-from-class="opacity-100 translate-x-0"
        leave-to-class="opacity-0 translate-x-full"
      >
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :class="[
            'pointer-events-auto min-w-[320px] max-w-md overflow-hidden rounded-lg shadow-lg',
            'bg-white dark:bg-dark-800',
            getBgTint(toast.type)
          ]"
        >
          <div class="p-4">
            <div class="flex items-start gap-3">
              <!-- Icon -->
              <div class="mt-0.5 flex-shrink-0">
                <Icon
                  :name="getToastIconName(toast.type)"
                  size="md"
                  :class="getIconColor(toast.type)"
                  aria-hidden="true"
                />
              </div>

              <!-- Content -->
              <div class="min-w-0 flex-1">
                <p v-if="toast.title" class="text-sm font-semibold text-mica-text-primary dark:text-mica-text-primary-dark">
                  {{ toast.title }}
                </p>
                <p
                  :class="[
                    'text-sm leading-relaxed',
                    toast.title
                      ? 'mt-1 text-mica-text-secondary dark:text-mica-text-secondary-dark'
                      : 'text-mica-text-primary dark:text-mica-text-primary-dark'
                  ]"
                >
                  {{ toast.message }}
                </p>
              </div>

              <!-- Close button -->
              <button
                @click="removeToast(toast.id)"
                class="-m-1 flex-shrink-0 rounded p-1 text-mica-text-tertiary transition-colors hover:bg-black/[0.03] hover:text-mica-text-secondary dark:text-mica-text-tertiary-dark dark:hover:bg-white/[0.06] dark:hover:text-mica-text-secondary-dark"
                aria-label="Close notification"
              >
                <Icon name="x" size="sm" />
              </button>
            </div>
          </div>

          <!-- Progress bar -->
          <div v-if="toast.duration" class="h-1 bg-black/[0.04] dark:bg-white/[0.06]">
            <div
              :class="['h-full toast-progress', getProgressBarColor(toast.type)]"
              :style="{ animationDuration: `${toast.duration}ms` }"
            ></div>
          </div>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()

const toasts = computed(() => appStore.toasts)

const getToastIconName = (type: string): 'checkCircle' | 'xCircle' | 'exclamationTriangle' | 'infoCircle' => {
  switch (type) {
    case 'success':
      return 'checkCircle'
    case 'error':
      return 'xCircle'
    case 'warning':
      return 'exclamationTriangle'
    case 'info':
    default:
      return 'infoCircle'
  }
}

const getIconColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'text-status-green dark:text-status-green-dark',
    error: 'text-status-red dark:text-status-red-dark',
    warning: 'text-status-amber dark:text-status-amber-dark',
    info: 'text-status-blue dark:text-status-blue-dark'
  }
  return colors[type] || colors.info
}

const getBgTint = (type: string): string => {
  const tints: Record<string, string> = {
    success: 'bg-status-green/[0.06] dark:bg-status-green-dark/[0.08]',
    error: 'bg-status-red/[0.06] dark:bg-status-red-dark/[0.08]',
    warning: 'bg-status-amber/[0.06] dark:bg-status-amber-dark/[0.08]',
    info: 'bg-status-blue/[0.06] dark:bg-status-blue-dark/[0.08]'
  }
  return tints[type] || tints.info
}

const getProgressBarColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'bg-status-green',
    error: 'bg-status-red',
    warning: 'bg-status-amber',
    info: 'bg-status-blue'
  }
  return colors[type] || colors.info
}

const removeToast = (id: string) => {
  appStore.hideToast(id)
}
</script>

<style scoped>
.toast-progress {
  width: 100%;
  animation-name: toast-progress-shrink;
  animation-timing-function: linear;
  animation-fill-mode: forwards;
}

@keyframes toast-progress-shrink {
  from {
    width: 100%;
  }
  to {
    width: 0%;
  }
}
</style>
