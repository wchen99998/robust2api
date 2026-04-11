import { ref } from 'vue'
import { embedAPI } from '@/api'

const EXPIRY_SKEW_MS = 30_000

export function useEmbedToken() {
  const token = ref<string | null>(null)
  const expiresAt = ref<string | null>(null)
  const loading = ref(false)

  function hasUsableToken(): boolean {
    if (!token.value || !expiresAt.value) {
      return false
    }
    const expiresAtMs = Date.parse(expiresAt.value)
    if (Number.isNaN(expiresAtMs)) {
      return false
    }
    return expiresAtMs - Date.now() > EXPIRY_SKEW_MS
  }

  async function ensureEmbedToken(force = false): Promise<string | null> {
    if (!force && hasUsableToken()) {
      return token.value
    }

    loading.value = true
    try {
      const response = await embedAPI.createEmbedToken()
      token.value = response.token
      expiresAt.value = response.expires_at
      return token.value
    } catch {
      token.value = null
      expiresAt.value = null
      return null
    } finally {
      loading.value = false
    }
  }

  return {
    token,
    expiresAt,
    loading,
    ensureEmbedToken
  }
}

export default useEmbedToken
