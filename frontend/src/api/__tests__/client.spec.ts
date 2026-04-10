import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import axios from 'axios'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh-CN'
}))

describe('API Client', () => {
  let apiClient: AxiosInstance

  beforeEach(async () => {
    localStorage.clear()
    vi.resetModules()
    Object.defineProperty(document, 'cookie', {
      configurable: true,
      get: () => 'control_csrf_token=test-csrf-token'
    })
    const mod = await import('@/api/client')
    apiClient = mod.apiClient
  })

  afterEach(() => {
    vi.restoreAllMocks()
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
  })
})
