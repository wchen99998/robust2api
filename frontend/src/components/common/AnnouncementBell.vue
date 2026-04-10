<template>
  <div>
    <!-- 铃铛按钮 -->
    <button
      @click="openModal"
      class="relative flex h-11 w-11 items-center justify-center rounded-lg text-mica-text-secondary transition-all hover:bg-black/[0.03] dark:text-mica-text-secondary-dark dark:hover:bg-white/[0.08]"
      :class="{ 'text-status-blue dark:text-status-blue-dark': unreadCount > 0 }"
      :aria-label="t('announcements.title')"
    >
      <Icon name="bell" size="md" />
      <!-- 未读红点 -->
      <span
        v-if="unreadCount > 0"
        class="absolute right-1 top-1 flex h-2 w-2"
      >
        <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-status-red opacity-75"></span>
        <span class="relative inline-flex h-2 w-2 rounded-full bg-status-red"></span>
      </span>
    </button>

    <!-- 公告列表 Modal -->
    <Teleport to="body">
      <Transition name="modal-fade">
        <div
          v-if="isModalOpen"
          class="fixed inset-0 z-[100] flex items-start justify-center overflow-y-auto bg-gradient-to-br from-black/70 via-black/60 to-black/70 p-4 pt-[8vh] backdrop-blur-md"
          @click="closeModal"
        >
          <div
            class="w-full max-w-[620px] overflow-hidden rounded-3xl bg-white shadow-2xl ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
            @click.stop
          >
            <!-- Header with Gradient -->
            <div class="relative overflow-hidden border-b border-black/[0.06] bg-status-blue/[0.04] px-6 py-5 dark:border-white/[0.08] dark:bg-status-blue-dark/[0.06]">
              <div class="relative z-10 flex items-start justify-between">
                <div>
                  <div class="flex items-center gap-2">
                    <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-status-blue text-white shadow-sm dark:bg-status-blue-dark">
                      <Icon name="bell" size="sm" />
                    </div>
                    <h2 class="text-lg font-semibold text-mica-text-primary dark:text-mica-text-primary-dark">
                      {{ t('announcements.title') }}
                    </h2>
                  </div>
                  <p v-if="unreadCount > 0" class="mt-2 text-sm text-mica-text-secondary dark:text-mica-text-secondary-dark">
                    <span class="font-medium text-status-blue dark:text-status-blue-dark">{{ unreadCount }}</span>
                    {{ t('announcements.unread') }}
                  </p>
                </div>
                <div class="flex items-center gap-2">
                  <button
                    v-if="unreadCount > 0"
                    @click="markAllAsRead"
                    :disabled="loading"
                    class="rounded-lg bg-status-blue px-4 py-2 text-xs font-medium text-white shadow-sm transition-all hover:opacity-85 disabled:opacity-50 dark:bg-status-blue-dark"
                  >
                    {{ t('announcements.markAllRead') }}
                  </button>
                  <button
                    @click="closeModal"
                    class="flex h-9 w-9 items-center justify-center rounded-lg bg-white/50 text-mica-text-tertiary backdrop-blur-sm transition-all hover:bg-white hover:text-mica-text-secondary dark:bg-white/[0.05] dark:text-mica-text-tertiary-dark dark:hover:bg-white/[0.08] dark:hover:text-mica-text-secondary-dark"
                    :aria-label="t('common.close')"
                  >
                    <Icon name="x" size="sm" />
                  </button>
                </div>
              </div>
            </div>

            <!-- Body -->
            <div class="max-h-[65vh] overflow-y-auto">
              <!-- Loading -->
              <div v-if="loading" class="flex items-center justify-center py-16">
                <div class="relative">
                  <div class="h-12 w-12 animate-spin rounded-full border-4 border-black/[0.06] border-t-status-blue dark:border-white/[0.08] dark:border-t-status-blue-dark"></div>
                </div>
              </div>

              <!-- Announcements List -->
              <div v-else-if="announcements.length > 0">
                <div
                  v-for="item in announcements"
                  :key="item.id"
                  class="group relative flex items-center gap-4 border-b border-black/[0.06] px-6 py-4 transition-all hover:bg-black/[0.02] dark:border-white/[0.08] dark:hover:bg-white/[0.03]"
                  :class="{ 'bg-status-blue/[0.04] dark:bg-status-blue-dark/[0.04]': !item.read_at }"
                  style="min-height: 72px"
                  @click="openDetail(item)"
                >
                  <!-- Status Indicator -->
                  <div class="flex h-10 w-10 flex-shrink-0 items-center justify-center">
                    <div
                      v-if="!item.read_at"
                      class="relative flex h-10 w-10 items-center justify-center rounded-xl bg-status-blue text-white shadow-sm dark:bg-status-blue-dark"
                    >
                      <!-- Pulse ring -->
                      <span class="absolute inline-flex h-full w-full animate-ping rounded-xl bg-status-blue opacity-75 dark:bg-status-blue-dark"></span>
                      <!-- Icon -->
                      <svg class="relative z-10 h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2.5">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                    <div
                      v-else
                      class="flex h-10 w-10 items-center justify-center rounded-xl bg-black/[0.04] text-mica-text-tertiary dark:bg-white/[0.06] dark:text-mica-text-tertiary-dark"
                    >
                      <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M9 12l2 2 4-4m6 2a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                  </div>

                  <!-- Content -->
                  <div class="flex min-w-0 flex-1 items-center justify-between gap-4">
                    <div class="min-w-0 flex-1">
                      <h3 class="truncate text-sm font-medium text-mica-text-primary dark:text-mica-text-primary-dark">
                        {{ item.title }}
                      </h3>
                      <div class="mt-1 flex items-center gap-2">
                        <time class="text-xs text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
                          {{ formatRelativeTime(item.created_at) }}
                        </time>
                        <span
                          v-if="!item.read_at"
                          class="inline-flex items-center gap-1 rounded-md bg-status-blue/[0.1] px-1.5 py-0.5 text-xs font-medium text-status-blue dark:bg-status-blue-dark/[0.12] dark:text-status-blue-dark"
                        >
                          <span class="relative flex h-1.5 w-1.5">
                            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-status-blue opacity-75"></span>
                            <span class="relative inline-flex h-1.5 w-1.5 rounded-full bg-status-blue"></span>
                          </span>
                          {{ t('announcements.unread') }}
                        </span>
                      </div>
                    </div>

                    <!-- Arrow -->
                    <div class="flex-shrink-0">
                      <svg
                        class="h-5 w-5 text-mica-text-tertiary transition-transform group-hover:translate-x-1 dark:text-mica-text-tertiary-dark"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        stroke-width="2"
                      >
                        <path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
                      </svg>
                    </div>
                  </div>

                </div>
              </div>

              <!-- Empty State -->
              <div v-else class="flex flex-col items-center justify-center py-16">
                <div class="relative mb-4">
                  <div class="flex h-20 w-20 items-center justify-center rounded-full bg-black/[0.04] dark:bg-white/[0.06]">
                    <Icon name="inbox" size="xl" class="text-mica-text-tertiary dark:text-mica-text-tertiary-dark" />
                  </div>
                  <div class="absolute -right-1 -top-1 flex h-6 w-6 items-center justify-center rounded-full bg-status-green text-white">
                    <svg class="h-3.5 w-3.5" fill="currentColor" viewBox="0 0 20 20">
                      <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                    </svg>
                  </div>
                </div>
                <p class="text-sm font-medium text-mica-text-primary dark:text-mica-text-primary-dark">{{ t('announcements.empty') }}</p>
                <p class="mt-1 text-xs text-mica-text-tertiary dark:text-mica-text-tertiary-dark">{{ t('announcements.emptyDescription') }}</p>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <!-- 公告详情 Modal -->
    <Teleport to="body">
      <Transition name="modal-fade">
        <div
          v-if="detailModalOpen && selectedAnnouncement"
          class="fixed inset-0 z-[110] flex items-start justify-center overflow-y-auto bg-gradient-to-br from-black/70 via-black/60 to-black/70 p-4 pt-[6vh] backdrop-blur-md"
          @click="closeDetail"
        >
          <div
            class="w-full max-w-[780px] overflow-hidden rounded-3xl bg-white shadow-2xl ring-1 ring-black/5 dark:bg-dark-800 dark:ring-white/10"
            @click.stop
          >
            <!-- Header with Decorative Elements -->
            <div class="relative overflow-hidden border-b border-black/[0.06] bg-status-blue/[0.06] px-8 py-6 dark:border-white/[0.08] dark:bg-status-blue-dark/[0.08]">

              <div class="relative z-10 flex items-start justify-between gap-4">
                <div class="flex-1 min-w-0">
                  <!-- Icon and Category -->
                  <div class="mb-3 flex items-center gap-2">
                    <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-status-blue text-white shadow-sm dark:bg-status-blue-dark">
                      <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                    </div>
                    <div class="flex items-center gap-2">
                      <span class="rounded-lg bg-status-blue/[0.1] px-2.5 py-1 text-xs font-medium text-status-blue dark:bg-status-blue-dark/[0.12] dark:text-status-blue-dark">
                        {{ t('announcements.title') }}
                      </span>
                      <span
                        v-if="!selectedAnnouncement.read_at"
                        class="inline-flex items-center gap-1.5 rounded-lg bg-status-blue px-2.5 py-1 text-xs font-medium text-white shadow-sm dark:bg-status-blue-dark"
                      >
                        <span class="relative flex h-2 w-2">
                          <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-white opacity-75"></span>
                          <span class="relative inline-flex h-2 w-2 rounded-full bg-white"></span>
                        </span>
                        {{ t('announcements.unread') }}
                      </span>
                    </div>
                  </div>

                  <!-- Title -->
                  <h2 class="mb-3 text-2xl font-bold leading-tight text-mica-text-primary dark:text-mica-text-primary-dark">
                    {{ selectedAnnouncement.title }}
                  </h2>

                  <!-- Meta Info -->
                  <div class="flex items-center gap-4 text-sm text-mica-text-secondary dark:text-mica-text-secondary-dark">
                    <div class="flex items-center gap-1.5">
                      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l3 3m6-3a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      <time>{{ formatRelativeWithDateTime(selectedAnnouncement.created_at) }}</time>
                    </div>
                    <div class="flex items-center gap-1.5">
                      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                        <path stroke-linecap="round" stroke-linejoin="round" d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z" />
                      </svg>
                      <span>{{ selectedAnnouncement.read_at ? t('announcements.read') : t('announcements.unread') }}</span>
                    </div>
                  </div>
                </div>

                <!-- Close button -->
                <button
                  @click="closeDetail"
                  class="flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl bg-white/50 text-mica-text-tertiary backdrop-blur-sm transition-all hover:bg-white hover:text-mica-text-secondary dark:bg-white/[0.05] dark:text-mica-text-tertiary-dark dark:hover:bg-white/[0.08] dark:hover:text-mica-text-secondary-dark"
                  :aria-label="t('common.close')"
                >
                  <Icon name="x" size="md" />
                </button>
              </div>
            </div>

            <!-- Body with Enhanced Markdown -->
            <div class="max-h-[60vh] overflow-y-auto bg-white px-8 py-8 dark:bg-dark-800">
              <!-- Content with decorative border -->
              <div>
                <div>
                  <div
                    class="markdown-body prose prose-sm max-w-none dark:prose-invert"
                    v-html="renderMarkdown(selectedAnnouncement.content)"
                  ></div>
                </div>
              </div>
            </div>

            <!-- Footer with Actions -->
            <div class="border-t border-black/[0.06] bg-black/[0.02] px-8 py-5 dark:border-white/[0.08] dark:bg-white/[0.03]">
              <div class="flex items-center justify-between">
                <div class="flex items-center gap-2 text-xs text-mica-text-tertiary dark:text-mica-text-tertiary-dark">
                  <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                    <path stroke-linecap="round" stroke-linejoin="round" d="M13 16h-1v-4h-1m1-4h.01M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <span>{{ selectedAnnouncement.read_at ? t('announcements.readStatus') : t('announcements.markReadHint') }}</span>
                </div>
                <div class="flex items-center gap-3">
                  <button
                    @click="closeDetail"
                    class="rounded-xl border border-black/[0.1] bg-white px-5 py-2.5 text-sm font-medium text-mica-text-secondary shadow-sm transition-all hover:bg-black/[0.02] hover:shadow dark:border-white/[0.1] dark:bg-dark-700 dark:text-mica-text-secondary-dark dark:hover:bg-dark-600"
                  >
                    {{ t('common.close') }}
                  </button>
                  <button
                    v-if="!selectedAnnouncement.read_at"
                    @click="markAsReadAndClose(selectedAnnouncement.id)"
                    class="rounded-xl bg-status-blue px-5 py-2.5 text-sm font-medium text-white shadow-sm transition-all hover:opacity-85 dark:bg-status-blue-dark"
                  >
                    <span class="flex items-center gap-2">
                      <svg class="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
                        <path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
                      </svg>
                      {{ t('announcements.markRead') }}
                    </span>
                  </button>
                </div>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { storeToRefs } from 'pinia'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { useAppStore } from '@/stores/app'
import { useAnnouncementStore } from '@/stores/announcements'
import { formatRelativeTime, formatRelativeWithDateTime } from '@/utils/format'
import type { UserAnnouncement } from '@/types'
import Icon from '@/components/icons/Icon.vue'

const { t } = useI18n()
const appStore = useAppStore()
const announcementStore = useAnnouncementStore()

// Configure marked
marked.setOptions({
  breaks: true,
  gfm: true,
})

// Use store state (storeToRefs for reactivity)
const { announcements, loading } = storeToRefs(announcementStore)
const unreadCount = computed(() => announcementStore.unreadCount)

// Local modal state
const isModalOpen = ref(false)
const detailModalOpen = ref(false)
const selectedAnnouncement = ref<UserAnnouncement | null>(null)

// Methods
function renderMarkdown(content: string): string {
  if (!content) return ''
  const html = marked.parse(content) as string
  return DOMPurify.sanitize(html)
}

function openModal() {
  isModalOpen.value = true
}

function closeModal() {
  isModalOpen.value = false
}

function openDetail(announcement: UserAnnouncement) {
  selectedAnnouncement.value = announcement
  detailModalOpen.value = true
  if (!announcement.read_at) {
    markAsRead(announcement.id)
  }
}

function closeDetail() {
  detailModalOpen.value = false
  selectedAnnouncement.value = null
}

async function markAsRead(id: number) {
  try {
    await announcementStore.markAsRead(id)
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

async function markAsReadAndClose(id: number) {
  await markAsRead(id)
  appStore.showSuccess(t('announcements.markedAsRead'))
  closeDetail()
}

async function markAllAsRead() {
  try {
    await announcementStore.markAllAsRead()
    appStore.showSuccess(t('announcements.allMarkedAsRead'))
  } catch (err: any) {
    appStore.showError(err?.message || t('common.unknownError'))
  }
}

function handleEscape(e: KeyboardEvent) {
  if (e.key === 'Escape') {
    if (detailModalOpen.value) {
      closeDetail()
    } else if (isModalOpen.value) {
      closeModal()
    }
  }
}

onMounted(() => {
  document.addEventListener('keydown', handleEscape)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', handleEscape)
  document.body.style.overflow = ''
})

watch(
  [isModalOpen, detailModalOpen, () => announcementStore.currentPopup],
  ([modal, detail, popup]) => {
    document.body.style.overflow = (modal || detail || popup) ? 'hidden' : ''
  }
)
</script>

<style scoped>
/* Modal Animations */
.modal-fade-enter-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.modal-fade-leave-active {
  transition: all 0.2s cubic-bezier(0.4, 0, 1, 1);
}

.modal-fade-enter-from,
.modal-fade-leave-to {
  opacity: 0;
}

.modal-fade-enter-from > div {
  transform: scale(0.94) translateY(-12px);
  opacity: 0;
}

.modal-fade-leave-to > div {
  transform: scale(0.96) translateY(-8px);
  opacity: 0;
}

/* Scrollbar Styling */
.overflow-y-auto::-webkit-scrollbar {
  width: 8px;
}

.overflow-y-auto::-webkit-scrollbar-track {
  background: transparent;
}

.overflow-y-auto::-webkit-scrollbar-thumb {
  background: linear-gradient(to bottom, #cbd5e1, #94a3b8);
  border-radius: 4px;
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb {
  background: linear-gradient(to bottom, #4b5563, #374151);
}

.overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: linear-gradient(to bottom, #94a3b8, #64748b);
}

.dark .overflow-y-auto::-webkit-scrollbar-thumb:hover {
  background: linear-gradient(to bottom, #6b7280, #4b5563);
}
</style>

<style>
/* Enhanced Markdown Styles */
.markdown-body {
  @apply text-[15px] leading-[1.75];
  @apply text-mica-text-secondary dark:text-mica-text-secondary-dark;
}

.markdown-body h1 {
  @apply mb-6 mt-8 border-b border-black/[0.06] pb-3 text-3xl font-bold text-mica-text-primary dark:border-white/[0.08] dark:text-mica-text-primary-dark;
}

.markdown-body h2 {
  @apply mb-4 mt-7 border-b border-black/[0.06] pb-2 text-2xl font-bold text-mica-text-primary dark:border-white/[0.08] dark:text-mica-text-primary-dark;
}

.markdown-body h3 {
  @apply mb-3 mt-6 text-xl font-semibold text-mica-text-primary dark:text-mica-text-primary-dark;
}

.markdown-body h4 {
  @apply mb-2 mt-5 text-lg font-semibold text-mica-text-primary dark:text-mica-text-primary-dark;
}

.markdown-body p {
  @apply mb-4 leading-relaxed;
}

.markdown-body a {
  @apply font-medium text-status-blue underline decoration-status-blue/30 decoration-2 underline-offset-2 transition-all hover:decoration-status-blue dark:text-status-blue-dark dark:decoration-status-blue-dark/30 dark:hover:decoration-status-blue-dark;
}

.markdown-body ul,
.markdown-body ol {
  @apply mb-4 ml-6 space-y-2;
}

.markdown-body ul {
  @apply list-disc;
}

.markdown-body ol {
  @apply list-decimal;
}

.markdown-body li {
  @apply leading-relaxed;
  @apply pl-2;
}

.markdown-body li::marker {
  @apply text-status-blue dark:text-status-blue-dark;
}

.markdown-body blockquote {
  @apply relative my-5 border border-status-blue/30 bg-status-blue/[0.04] py-3 pl-5 pr-4 italic text-mica-text-secondary dark:border-status-blue-dark/30 dark:bg-status-blue-dark/[0.06] dark:text-mica-text-secondary-dark;
}

.markdown-body blockquote::before {
  content: '"';
  @apply absolute -left-1 top-0 text-5xl font-serif text-status-blue/20 dark:text-status-blue-dark/20;
}

.markdown-body code {
  @apply rounded-lg bg-black/[0.04] px-2 py-1 text-[13px] font-mono text-status-red dark:bg-white/[0.06] dark:text-status-red-dark;
}

.markdown-body pre {
  @apply my-5 overflow-x-auto rounded-xl border border-black/[0.06] bg-black/[0.02] p-5 dark:border-white/[0.08] dark:bg-dark-900/50;
}

.markdown-body pre code {
  @apply bg-transparent p-0 text-[13px] text-mica-text-primary dark:text-mica-text-primary-dark;
}

.markdown-body hr {
  @apply my-8 border-0 border-t-2 border-black/[0.06] dark:border-white/[0.08];
}

.markdown-body table {
  @apply mb-5 w-full overflow-hidden rounded-lg border border-black/[0.06] dark:border-white/[0.08];
}

.markdown-body th,
.markdown-body td {
  @apply border-r border-b border-black/[0.06] px-4 py-3 text-left dark:border-white/[0.08];
}

.markdown-body th:last-child,
.markdown-body td:last-child {
  @apply border-r-0;
}

.markdown-body tr:last-child td {
  @apply border-b-0;
}

.markdown-body th {
  @apply bg-status-blue/[0.04] font-semibold text-mica-text-primary dark:bg-status-blue-dark/[0.06] dark:text-mica-text-primary-dark;
}

.markdown-body tbody tr {
  @apply transition-colors hover:bg-black/[0.02] dark:hover:bg-white/[0.03];
}

.markdown-body img {
  @apply my-5 max-w-full rounded-xl border border-black/[0.06] shadow-md dark:border-white/[0.08];
}

.markdown-body strong {
  @apply font-semibold text-mica-text-primary dark:text-mica-text-primary-dark;
}

.markdown-body em {
  @apply italic text-mica-text-secondary dark:text-mica-text-secondary-dark;
}
</style>
