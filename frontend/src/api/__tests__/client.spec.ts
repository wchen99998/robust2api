import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import axios from 'axios'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN'
}))

describe('API Client', () => {
  let apiClient: AxiosInstance
  let cookieValue = 'control_csrf_token=test-csrf-token'
  const originalLocation = window.location

  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    vi.resetModules()
    cookieValue = 'control_csrf_token=test-csrf-token'
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      get: () => cookieValue
    })
    Object.defineProperty(window, 'location', {
      configurable: true,
      writable: true,
      value: {
        pathname: '/dashboard',
        href: '/dashboard'
      }
    })
    const mod = await import('@/api/client')
    apiClient = mod.apiClient
  })

  afterEach(() => {
    vi.restoreAllMocks()
    Object.defineProperty(window, 'location', {
      configurable: true,
      value: originalLocation
    })
  })

  describe('request interceptor', () => {
    it('attaches locale header', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {} },
        headers: {},
        config: {},
        statusText: 'OK'
      })
      apiClient.defaults.adapter = adapter

      await apiClient.get('/test')

      const config = adapter.mock.calls[0][0]
      expect(config.headers.get('Accept-Language')).toBe('zh-CN')
    })

    it('attaches timezone for GET', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {} },
        headers: {},
        config: {},
        statusText: 'OK'
      })
      apiClient.defaults.adapter = adapter

      await apiClient.get('/test')

      const config = adapter.mock.calls[0][0]
      expect(config.params).toHaveProperty('timezone')
    })

    it('attaches csrf header for mutating requests', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {} },
        headers: {},
        config: {},
        statusText: 'OK'
      })
      apiClient.defaults.adapter = adapter

      await apiClient.post('/test', { ok: true })

      const config = adapter.mock.calls[0][0]
      expect(config.headers.get('X-CSRF-Token')).toBe('test-csrf-token')
    })

    it('attaches csrf header for explicit session refresh requests', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: {} },
        headers: {},
        config: {},
        statusText: 'OK'
      })
      apiClient.defaults.adapter = adapter

      await apiClient.post('/session/refresh', {})

      const config = adapter.mock.calls[0][0]
      expect(config.headers.get('X-CSRF-Token')).toBe('test-csrf-token')
    })
  })

  describe('response interceptor', () => {
    it('unwraps code=0 payload', async () => {
      const adapter = vi.fn().mockResolvedValue({
        status: 200,
        data: { code: 0, data: { name: 'test' }, message: 'ok' },
        headers: {},
        config: {},
        statusText: 'OK'
      })
      apiClient.defaults.adapter = adapter

      const response = await apiClient.get('/test')
      expect(response.data).toEqual({ name: 'test' })
    })

    it('returns structured network error', async () => {
      const adapter = vi.fn().mockRejectedValue({
        code: 'ERR_NETWORK',
        message: 'Network Error',
        config: { url: '/test' }
      })
      apiClient.defaults.adapter = adapter

      await expect(apiClient.get('/test')).rejects.toEqual(
        expect.objectContaining({
          status: 0,
          message: 'Network error. Please check your connection.'
        })
      )
    })

    it('keeps cancellation error untouched', async () => {
      const source = axios.CancelToken.source()
      const adapter = vi.fn().mockRejectedValue(new axios.Cancel('Operation canceled'))
      apiClient.defaults.adapter = adapter

      await expect(apiClient.get('/test', { cancelToken: source.token })).rejects.toBeDefined()
    })

    it('replays mutating requests with the refreshed csrf token', async () => {
      cookieValue = 'control_csrf_token=stale-csrf-token'
      let attempt = 0
      const adapter = vi.fn().mockImplementation(async (config) => {
        attempt += 1

        if (attempt === 1) {
          expect(config.headers.get('X-CSRF-Token')).toBe('stale-csrf-token')
          return Promise.reject({
            response: {
              status: 401,
              data: {
                code: 'TOKEN_EXPIRED',
                message: 'expired'
              }
            },
            config
          })
        }

        expect(config.headers.get('X-CSRF-Token')).toBe('fresh-csrf-token')
        return {
          status: 200,
          data: { code: 0, data: { ok: true } },
          headers: {},
          config,
          statusText: 'OK'
        }
      })
      apiClient.defaults.adapter = adapter

      const refreshSpy = vi.spyOn(axios, 'post').mockImplementation(async () => {
        cookieValue = 'control_csrf_token=fresh-csrf-token'
        return {
          data: {
            code: 0,
            data: {}
          }
        } as any
      })

      const response = await apiClient.post('/me/password/change', { ok: true })

      expect(refreshSpy).toHaveBeenCalledTimes(1)
      expect(adapter).toHaveBeenCalledTimes(2)
      expect(response.data).toEqual({ ok: true })
    })

    it('does not mark auth as expired when login returns 401', async () => {
      const adapter = vi.fn().mockRejectedValue({
        response: {
          status: 401,
          data: {
            code: 'INVALID_CREDENTIALS',
            message: 'invalid credentials'
          }
        },
        config: {
          url: '/session/login',
          method: 'post',
          headers: {}
        }
      })
      apiClient.defaults.adapter = adapter
      const setItemSpy = vi.spyOn(window.sessionStorage, 'setItem')

      await expect(
        apiClient.post('/session/login', {
          email: 'test@example.com',
          password: 'bad-password'
        })
      ).rejects.toMatchObject({
        status: 401,
        code: 'INVALID_CREDENTIALS'
      })

      expect(setItemSpy).not.toHaveBeenCalledWith('auth_expired', '1')
      expect(window.location.href).toBe('/login')
    })

    it('marks auth as expired when refresh fails for a protected endpoint', async () => {
      const adapter = vi.fn().mockRejectedValue({
        response: {
          status: 401,
          data: {
            code: 'TOKEN_EXPIRED',
            message: 'expired'
          }
        },
        config: {
          url: '/me/password/change',
          method: 'post',
          headers: {}
        }
      })
      apiClient.defaults.adapter = adapter
      const setItemSpy = vi.spyOn(window.sessionStorage, 'setItem')
      vi.spyOn(axios, 'post').mockRejectedValue(new Error('refresh failed'))

      await expect(apiClient.post('/me/password/change', { password: 'new-password' })).rejects.toMatchObject({
        status: 401,
        code: 'TOKEN_REFRESH_FAILED'
      })

      expect(setItemSpy).toHaveBeenCalledWith('auth_expired', '1')
      expect(window.location.href).toBe('/login')
    })
  })
})
