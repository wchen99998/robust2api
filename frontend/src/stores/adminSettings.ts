import { defineStore } from 'pinia'
import { ref } from 'vue'
import { adminAPI } from '@/api'
import type { CustomMenuItem } from '@/types'

export const useAdminSettingsStore = defineStore('adminSettings', () => {
  const loaded = ref(false)
  const loading = ref(false)
  const customMenuItems = ref<CustomMenuItem[]>([])

  async function fetch(force = false): Promise<void> {
    if (loaded.value && !force) return
    if (loading.value) return

    loading.value = true
    try {
      const settings = await adminAPI.settings.getSettings()
      customMenuItems.value = Array.isArray(settings.custom_menu_items) ? settings.custom_menu_items : []
      loaded.value = true
    } catch (err) {
      loaded.value = true
      console.error('[adminSettings] Failed to fetch settings:', err)
    } finally {
      loading.value = false
    }
  }

  return {
    loaded,
    loading,
    customMenuItems,
    fetch
  }
})
