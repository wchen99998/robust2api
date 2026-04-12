/**
 * Axios HTTP Client Configuration
 * Base client with interceptors for authentication, token refresh, and error handling
 */

import axios, {
  AxiosHeaders,
  AxiosInstance,
  AxiosError,
  InternalAxiosRequestConfig,
  AxiosResponse
} from 'axios'
import type { ApiResponse } from '@/types'
import { getLocale } from '@/i18n'

// ==================== Axios Instance Configuration ====================

const API_BASE_URL = import.meta.env.VITE_API_BASE_URL || '/api/v1'

export const apiClient: AxiosInstance = axios.create({
  baseURL: API_BASE_URL,
  timeout: 30000,
  withCredentials: true,
  headers: {
    'Content-Type': 'application/json'
  }
})

// ==================== Token Refresh State ====================

let isRefreshing = false
let refreshSubscribers: Array<(ok: boolean) => void> = []

function subscribeTokenRefresh(callback: (ok: boolean) => void): void {
  refreshSubscribers.push(callback)
}

function onTokenRefreshed(ok: boolean): void {
  refreshSubscribers.forEach((callback) => callback(ok))
  refreshSubscribers = []
}

const CSRF_COOKIE_NAME = 'control_csrf_token'
const CSRF_HEADER_NAME = 'X-CSRF-Token'
const MUTATING_METHODS = new Set(['post', 'put', 'patch', 'delete'])
const API_PATH_PREFIX = API_BASE_URL.startsWith('/') ? API_BASE_URL : ''

const AUTH_EXCLUDED_PATH_PREFIXES = [
  '/session/login',
  '/session/login/totp',
  '/registration',
  '/registration/preflight',
  '/registration/email-code',
  '/password/forgot',
  '/password/reset'
]

const REFRESH_PATH = '/session/refresh'

function getCookie(name: string): string | null {
  if (typeof document === 'undefined') {
    return null
  }
  const needle = `${encodeURIComponent(name)}=`
  const found = document.cookie.split('; ').find((cookie) => cookie.startsWith(needle))
  if (!found) {
    return null
  }
  return decodeURIComponent(found.slice(needle.length))
}

export function getCSRFToken(): string | null {
  return getCookie(CSRF_COOKIE_NAME)
}

function normalizePath(url: string | undefined): string {
  if (!url) return ''
  const stripPrefix = (path: string) => {
    if (API_PATH_PREFIX && path.startsWith(`${API_PATH_PREFIX}/`)) {
      return path.slice(API_PATH_PREFIX.length)
    }
    if (API_PATH_PREFIX && path === API_PATH_PREFIX) {
      return '/'
    }
    return path
  }
  if (url.startsWith('http://') || url.startsWith('https://')) {
    try {
      const parsed = new URL(url)
      return stripPrefix(parsed.pathname)
    } catch {
      return stripPrefix(url)
    }
  }
  const normalized = url.startsWith('/') ? url : `/${url}`
  return stripPrefix(normalized)
}

function shouldAttachCSRF(config: InternalAxiosRequestConfig): boolean {
  const method = String(config.method || 'get').toLowerCase()
  if (!MUTATING_METHODS.has(method)) {
    return false
  }
  return true
}

function isAuthExcludedPath(path: string): boolean {
  if (!path) return false
  if (path === REFRESH_PATH) return true
  return AUTH_EXCLUDED_PATH_PREFIXES.some(
    (prefix) => path === prefix || path.startsWith(`${prefix}/`)
  )
}

function syncCSRFHeader(config: InternalAxiosRequestConfig): void {
  if (!shouldAttachCSRF(config)) {
    return
  }

  const headers = AxiosHeaders.from(config.headers)
  headers.delete(CSRF_HEADER_NAME)
  headers.delete(CSRF_HEADER_NAME.toLowerCase())

  const csrf = getCSRFToken()
  if (csrf) {
    headers.set(CSRF_HEADER_NAME, csrf)
  }

  config.headers = headers
}

// ==================== Request Interceptor ====================

// Get user's timezone
const getUserTimezone = (): string => {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  } catch {
    return 'UTC'
  }
}

apiClient.interceptors.request.use(
  (config: InternalAxiosRequestConfig) => {
    if (config.headers) {
      config.headers['Accept-Language'] = getLocale()
    }

    if (shouldAttachCSRF(config) && config.headers && !config.headers[CSRF_HEADER_NAME]) {
      const csrf = getCSRFToken()
      if (csrf) {
        config.headers[CSRF_HEADER_NAME] = csrf
      }
    }

    if (config.method === 'get') {
      if (!config.params) {
        config.params = {}
      }
      config.params.timezone = getUserTimezone()
    }

    return config
  },
  (error) => {
    return Promise.reject(error)
  }
)

// ==================== Response Interceptor ====================

apiClient.interceptors.response.use(
  (response: AxiosResponse) => {
    // Unwrap standard API response format { code, message, data }
    const apiResponse = response.data as ApiResponse<unknown>
    if (apiResponse && typeof apiResponse === 'object' && 'code' in apiResponse) {
      if (apiResponse.code === 0) {
        // Success - return the data portion
        response.data = apiResponse.data
      } else {
        // API error
        return Promise.reject({
          status: response.status,
          code: apiResponse.code,
          message: apiResponse.message || 'Unknown error'
        })
      }
    }
    return response
  },
  async (error: AxiosError<ApiResponse<unknown>>) => {
    // Request cancellation: keep the original axios cancellation error so callers can ignore it.
    // Otherwise we'd misclassify it as a generic "network error".
    if (error.code === 'ERR_CANCELED' || axios.isCancel(error)) {
      return Promise.reject(error)
    }

    const originalRequest = error.config as InternalAxiosRequestConfig & { _retry?: boolean }

    // Handle common errors
    if (error.response) {
      const { status, data } = error.response
      const url = String(error.config?.url || '')

      // Validate `data` shape to avoid HTML error pages breaking our error handling.
      const apiData = (typeof data === 'object' && data !== null ? data : {}) as Record<string, any>

      if (status === 401 && !originalRequest._retry) {
        const path = normalizePath(url)
        const isAuthEndpoint = isAuthExcludedPath(path)

        if (!isAuthEndpoint) {
          if (isRefreshing) {
            return new Promise((resolve, reject) => {
              subscribeTokenRefresh((ok: boolean) => {
                if (ok) {
                  originalRequest._retry = true
                  syncCSRFHeader(originalRequest)
                  resolve(apiClient(originalRequest))
                } else {
                  reject({
                    status,
                    code: apiData.code,
                    message: apiData.message || apiData.detail || error.message
                  })
                }
              })
            })
          }

          originalRequest._retry = true
          isRefreshing = true

          try {
            await axios.post(
              `${API_BASE_URL}${REFRESH_PATH}`,
              {},
              {
                withCredentials: true,
                headers: {
                  'Content-Type': 'application/json',
                  ...(getCSRFToken() ? { [CSRF_HEADER_NAME]: getCSRFToken() as string } : {})
                }
              }
            )

            onTokenRefreshed(true)

            isRefreshing = false
            syncCSRFHeader(originalRequest)
            return apiClient(originalRequest)
          } catch (refreshError) {
            onTokenRefreshed(false)
            isRefreshing = false

            sessionStorage.setItem('auth_expired', '1')

            if (!window.location.pathname.includes('/login')) {
              window.location.href = '/login'
            }

            return Promise.reject({
              status: 401,
              code: 'TOKEN_REFRESH_FAILED',
              message: 'Session expired. Please log in again.'
            })
          }
        }
        if (!window.location.pathname.includes('/login')) {
          if (!isAuthEndpoint) {
            sessionStorage.setItem('auth_expired', '1')
          }
          window.location.href = '/login'
        }
      }

      // Return structured error
      return Promise.reject({
        status,
        code: apiData.code,
        error: apiData.error,
        message: apiData.message || apiData.detail || error.message
      })
    }

    // Network error
    return Promise.reject({
      status: 0,
      message: 'Network error. Please check your connection.'
    })
  }
)

export default apiClient
