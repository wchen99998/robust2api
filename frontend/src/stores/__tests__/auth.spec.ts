import { describe, it, expect, vi, beforeEach } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useAuthStore } from '@/stores/auth'

const mockBootstrap = vi.fn()
const mockLogin = vi.fn()
const mockLogin2FA = vi.fn()
const mockRegister = vi.fn()
const mockLogout = vi.fn()
const mockRefreshSession = vi.fn()

vi.mock('@/api', () => ({
  authAPI: {
    bootstrap: (...args: any[]) => mockBootstrap(...args),
    login: (...args: any[]) => mockLogin(...args),
    login2FA: (...args: any[]) => mockLogin2FA(...args),
    register: (...args: any[]) => mockRegister(...args),
    logout: (...args: any[]) => mockLogout(...args),
    refreshSession: (...args: any[]) => mockRefreshSession(...args)
  },
  isTotp2FARequired: (response: any) => response?.mfa_required === true || response?.requires_2fa === true
}))

const fakeUser = {
  id: 1,
  username: 'testuser',
  email: 'test@example.com',
  role: 'user' as const,
  balance: 100,
  concurrency: 5,
  status: 'active' as const,
  allowed_groups: null,
  created_at: '2024-01-01',
  updated_at: '2024-01-01'
}

const fakeBootstrap = {
  access_token: 'access-token-1',
  csrf_token: 'csrf-token',
  run_mode: 'simple' as const,
  public_settings: {} as any,
  auth_capabilities: {
    provider: 'local',
    password_login_enabled: true,
    registration_enabled: true,
    email_verification_enabled: true,
    password_reset_enabled: true,
    password_change_enabled: true,
    mfa_self_service_enabled: true,
    profile_self_service_enabled: true
  },
  auth_providers: [
    {
      id: 'oidc',
      type: 'oidc',
      display_name: 'Auth0',
      start_path: '/api/v1/oauth/oidc/start'
    }
  ],
  auth_state: { authenticated: true },
  me: { user: fakeUser, roles: ['user'] }
}

describe('useAuthStore', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    localStorage.clear()
    vi.clearAllMocks()
  })

  it('initialize uses bootstrap and sets authenticated user', async () => {
    mockBootstrap.mockResolvedValue(fakeBootstrap)
    const store = useAuthStore()

    await store.initialize()

    expect(store.isAuthenticated).toBe(true)
    expect(store.user?.email).toBe('test@example.com')
    expect(store.csrfToken).toBe('csrf-token')
    expect(store.runMode).toBe('simple')
    expect(store.token).toBe('access-token-1')
    expect(store.authCapabilities?.password_change_enabled).toBe(true)
    expect(store.authProviders[0]?.id).toBe('oidc')
  })

  it('login handles normal bootstrap payload', async () => {
    mockLogin.mockResolvedValue(fakeBootstrap)
    const store = useAuthStore()

    const result = await store.login({ email: 'test@example.com', password: '123456' })

    expect(result).toEqual(fakeBootstrap)
    expect(store.isAuthenticated).toBe(true)
    expect(store.user?.username).toBe('testuser')
    expect(store.token).toBe('access-token-1')
  })

  it('login returns MFA challenge without authenticating', async () => {
    mockLogin.mockResolvedValue({
      mfa_required: true,
      login_challenge_id: 'challenge-1',
      user_email_masked: 'te***@example.com'
    })
    const store = useAuthStore()

    const result = await store.login({ email: 'test@example.com', password: '123456' })

    expect((result as any).mfa_required).toBe(true)
    expect(store.isAuthenticated).toBe(false)
  })

  it('stores pending registration from bootstrap when unauthenticated', async () => {
    mockBootstrap.mockResolvedValue({
      csrf_token: 'csrf-token',
      run_mode: 'standard' as const,
      public_settings: {} as any,
      auth_state: { authenticated: false },
      pending_registration: {
        challenge_id: 'challenge-1',
        provider: 'oidc',
        email: 'pending@example.com',
        redirect_to: '/dashboard',
        expires_at: '2026-01-01T00:00:00Z'
      },
      me: { user: null }
    })
    const store = useAuthStore()

    await store.initialize()

    expect(store.isAuthenticated).toBe(false)
    expect(store.pendingRegistration?.challenge_id).toBe('challenge-1')
  })

  it('refreshes the session during initialize when bootstrap says refresh is available', async () => {
    mockBootstrap.mockResolvedValue({
      csrf_token: 'csrf-token',
      run_mode: 'standard' as const,
      public_settings: {} as any,
      auth_state: {
        authenticated: false,
        refresh_available: true
      },
      me: { user: null }
    })
    mockRefreshSession.mockResolvedValue(fakeBootstrap)
    const store = useAuthStore()

    await store.initialize()

    expect(mockRefreshSession).toHaveBeenCalledTimes(1)
    expect(store.isAuthenticated).toBe(true)
    expect(store.user?.email).toBe('test@example.com')
    expect(store.token).toBe('access-token-1')
  })

  it('login2FA authenticates via challenge', async () => {
    mockLogin2FA.mockResolvedValue(fakeBootstrap)
    const store = useAuthStore()

    const user = await store.login2FA('challenge-1', '123456')

    expect(user.email).toBe('test@example.com')
    expect(store.isAuthenticated).toBe(true)
    expect(mockLogin2FA).toHaveBeenCalledWith({
      login_challenge_id: 'challenge-1',
      totp_code: '123456'
    })
  })

  it('register authenticates user', async () => {
    mockRegister.mockResolvedValue(fakeBootstrap)
    const store = useAuthStore()

    const user = await store.register({
      email: 'test@example.com',
      password: '123456'
    })

    expect(user.username).toBe('testuser')
    expect(store.isAuthenticated).toBe(true)
  })

  it('logout clears auth state', async () => {
    mockBootstrap.mockResolvedValue(fakeBootstrap)
    mockLogout.mockResolvedValue(undefined)
    const store = useAuthStore()
    await store.initialize()

    await store.logout()

    expect(store.isAuthenticated).toBe(false)
    expect(store.user).toBeNull()
  })
})
