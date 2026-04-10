import { beforeEach, describe, expect, it, vi } from 'vitest'

const mockGet = vi.fn()
const mockPost = vi.fn()

vi.mock('@/api/client', () => ({
  apiClient: {
    get: (...args: any[]) => mockGet(...args),
    post: (...args: any[]) => mockPost(...args)
  }
}))

describe('auth API normalization', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('normalizes bootstrap payload returned by control auth BFF', async () => {
    const { bootstrap } = await import('@/api/auth')
    mockGet.mockResolvedValue({
      data: {
        access_token: 'access-token-1',
        csrf_token: 'csrf-1',
        run_mode: 'simple',
        authenticated: true,
        refresh_available: true,
        settings: {
          registration_enabled: true
        },
        subject: {
          subject_id: 'subject-1',
          session_id: 'session-1'
        },
        profile: {
          id: 1,
          username: 'tester',
          email: 'tester@example.com',
          balance: 0,
          concurrency: 1,
          status: 'active',
          allowed_groups: null,
          created_at: '2026-01-01T00:00:00Z',
          updated_at: '2026-01-01T00:00:00Z'
        },
        roles: ['admin'],
        primary_role: 'admin',
        mfa: {
          totp_enabled: true,
          feature_enabled: true
        },
        session: {
          session_id: 'session-1',
          expires_at: '2026-01-01T01:00:00Z',
          absolute_expires_at: '2026-01-10T01:00:00Z',
          last_seen_at: '2026-01-01T00:30:00Z'
        },
        pending_registration: {
          challenge_id: 'challenge-1',
          provider: 'oidc',
          email: 'pending@example.com',
          redirect_to: '/dashboard',
          expires_at: '2026-01-01T01:00:00Z'
        }
      }
    })

    const data = await bootstrap()

    expect(data.access_token).toBe('access-token-1')
    expect(data.csrf_token).toBe('csrf-1')
    expect(data.run_mode).toBe('simple')
    expect(data.auth_state.authenticated).toBe(true)
    expect(data.auth_state.refresh_available).toBe(true)
    expect(data.me?.subject_id).toBe('subject-1')
    expect(data.me?.sid).toBe('session-1')
    expect(data.me?.roles).toEqual(['admin'])
    expect(data.me?.user?.role).toBe('admin')
    expect(data.me?.user?.run_mode).toBe('simple')
    expect(data.pending_registration?.challenge_id).toBe('challenge-1')
  })

  it('maps login TOTP masked email from masked_email field', async () => {
    const { login, isTotp2FARequired } = await import('@/api/auth')
    mockPost.mockResolvedValue({
      data: {
        mfa_required: true,
        login_challenge_id: 'challenge-1',
        masked_email: 'te***@example.com'
      }
    })

    const response = await login({
      email: 'tester@example.com',
      password: 'password123'
    })

    expect(isTotp2FARequired(response)).toBe(true)
    expect((response as any).user_email_masked).toBe('te***@example.com')
  })
})
