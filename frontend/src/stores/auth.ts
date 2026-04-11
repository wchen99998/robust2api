/**
 * Authentication Store
 * Cookie/session based auth state with bootstrap as the canonical source.
 */

import { defineStore } from 'pinia'
import { ref, computed, readonly } from 'vue'
import { authAPI, isTotp2FARequired, type LoginResponse } from '@/api'
import type {
  User,
  LoginRequest,
  RegisterRequest,
  BootstrapResponse,
  BootstrapPendingRegistration,
  BootstrapAuthCapabilities,
  BootstrapAuthProvider
} from '@/types'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const token = ref<string | null>(null)
  const runMode = ref<'standard' | 'simple'>('standard')
  const initialized = ref(false)
  const initializing = ref<Promise<void> | null>(null)
  const csrfToken = ref<string>('')
  const pendingRegistration = ref<BootstrapPendingRegistration | null>(null)
  const authCapabilities = ref<BootstrapAuthCapabilities | null>(null)
  const authProviders = ref<BootstrapAuthProvider[]>([])

  const isAuthenticated = computed(() => Boolean(user.value))
  const isAdmin = computed(() => user.value?.role === 'admin')
  const isSimpleMode = computed(() => runMode.value === 'simple')

  function clearLegacyAuthStorage(): void {
    localStorage.removeItem('auth_token')
    localStorage.removeItem('auth_user')
    localStorage.removeItem('refresh_token')
    localStorage.removeItem('token_expires_at')
  }

  function clearAuth(): void {
    token.value = null
    user.value = null
    runMode.value = 'standard'
    csrfToken.value = ''
    pendingRegistration.value = null
    authCapabilities.value = null
    authProviders.value = []
    clearLegacyAuthStorage()
  }

  function applyBootstrap(response: BootstrapResponse): void {
    const currentUser = response.me?.user ?? null
    const effectiveRunMode =
      response.run_mode ||
      (currentUser?.run_mode as 'standard' | 'simple' | undefined) ||
      'standard'
    user.value = currentUser
      ? ({
          ...currentUser,
          run_mode: (currentUser.run_mode as 'standard' | 'simple' | undefined) || effectiveRunMode
        } as User)
      : null
    runMode.value = effectiveRunMode
    csrfToken.value = response.csrf_token || ''
    pendingRegistration.value = response.pending_registration ?? null
    authCapabilities.value = response.auth_capabilities ?? null
    authProviders.value = response.auth_providers ?? []
    if (!currentUser) {
      token.value = null
      clearLegacyAuthStorage()
      return
    }

    token.value = response.access_token || null
    localStorage.setItem('auth_user', JSON.stringify(currentUser))
  }

  function hydrate(response: BootstrapResponse): void {
    applyBootstrap(response)
    initialized.value = true
  }

  async function bootstrapSession(): Promise<void> {
    const response = await authAPI.bootstrap()
    if (!response.auth_state.authenticated && response.auth_state.refresh_available) {
      try {
        const refreshed = await authAPI.refreshSession()
        hydrate(refreshed)
        return
      } catch {
        applyBootstrap(response)
        return
      }
    }
    applyBootstrap(response)
  }

  async function initialize(force = false): Promise<void> {
    if (initialized.value && !force) {
      return
    }
    if (initializing.value) {
      await initializing.value
      return
    }

    const task = (async () => {
      try {
        await bootstrapSession()
      } catch {
        clearAuth()
      } finally {
        initialized.value = true
        initializing.value = null
      }
    })()

    initializing.value = task
    await task
  }

  function checkAuth(): Promise<void> {
    return initialize()
  }

  async function login(credentials: LoginRequest): Promise<LoginResponse> {
    const response = await authAPI.login(credentials)
    if (isTotp2FARequired(response)) {
      return response
    }

    hydrate(response)
    return response
  }

  async function login2FA(tempToken: string, totpCode: string): Promise<User> {
    const response = await authAPI.login2FA({
      login_challenge_id: tempToken,
      totp_code: totpCode
    })
    hydrate(response)
    if (!user.value) {
      throw new Error('Authentication failed')
    }
    return user.value
  }

  async function register(userData: RegisterRequest): Promise<User> {
    const response = await authAPI.register(userData)
    hydrate(response)
    if (!user.value) {
      throw new Error('Registration failed')
    }
    return user.value
  }

  async function setToken(_newToken: string): Promise<User> {
    await initialize(true)
    if (!user.value) {
      throw new Error('Not authenticated')
    }
    return user.value
  }

  async function logout(): Promise<void> {
    try {
      await authAPI.logout()
    } finally {
      clearAuth()
      initialized.value = true
    }
  }

  async function refreshUser(): Promise<User> {
    await initialize(true)
    if (!user.value) {
      throw new Error('Not authenticated')
    }
    return user.value
  }

  return {
    user,
    token,
    csrfToken: readonly(csrfToken),
    pendingRegistration: readonly(pendingRegistration),
    authCapabilities: readonly(authCapabilities),
    authProviders: readonly(authProviders),
    runMode: readonly(runMode),
    isAuthenticated,
    isAdmin,
    isSimpleMode,
    initialized: readonly(initialized),
    hydrate,
    login,
    login2FA,
    register,
    setToken,
    logout,
    checkAuth,
    initialize,
    refreshUser
  }
})
