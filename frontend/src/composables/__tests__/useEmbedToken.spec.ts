import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useEmbedToken } from '@/composables/useEmbedToken'

const mockCreateEmbedToken = vi.fn()

vi.mock('@/api', () => ({
  embedAPI: {
    createEmbedToken: (...args: any[]) => mockCreateEmbedToken(...args)
  }
}))

describe('useEmbedToken', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.useRealTimers()
  })

  it('caches embed token until near expiry', async () => {
    mockCreateEmbedToken.mockResolvedValue({
      token: 'embed-token-1',
      expires_at: new Date(Date.now() + 5 * 60_000).toISOString()
    })
    const hook = useEmbedToken()

    const first = await hook.ensureEmbedToken()
    const second = await hook.ensureEmbedToken()

    expect(first).toBe('embed-token-1')
    expect(second).toBe('embed-token-1')
    expect(mockCreateEmbedToken).toHaveBeenCalledTimes(1)
  })

  it('refreshes token when force=true', async () => {
    mockCreateEmbedToken
      .mockResolvedValueOnce({
        token: 'embed-token-1',
        expires_at: new Date(Date.now() + 5 * 60_000).toISOString()
      })
      .mockResolvedValueOnce({
        token: 'embed-token-2',
        expires_at: new Date(Date.now() + 5 * 60_000).toISOString()
      })
    const hook = useEmbedToken()

    await hook.ensureEmbedToken()
    const second = await hook.ensureEmbedToken(true)

    expect(second).toBe('embed-token-2')
    expect(mockCreateEmbedToken).toHaveBeenCalledTimes(2)
  })
})
