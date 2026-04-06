/**
 * Admin Ops API endpoints
 * - Error logs list/detail + retry (client/upstream)
 * - Concurrency & account availability stats
 * - Runtime logging config
 * - Request details
 */

import { apiClient } from '../client'
import type { PaginatedResponse } from '@/types'

export type OpsRetryMode = 'client' | 'upstream'

export interface OpsRequestOptions {
  signal?: AbortSignal
}

export interface OpsRetryRequest {
  mode: OpsRetryMode
  pinned_account_id?: number
  force?: boolean
}

export interface OpsRetryAttempt {
  id: number
  created_at: string
  requested_by_user_id: number
  source_error_id: number
  mode: string
  pinned_account_id?: number | null
  pinned_account_name?: string

  status: string
  started_at?: string | null
  finished_at?: string | null
  duration_ms?: number | null

  success?: boolean | null
  http_status_code?: number | null
  upstream_request_id?: string | null
  used_account_id?: number | null
  used_account_name?: string
  response_preview?: string | null
  response_truncated?: boolean | null

  result_request_id?: string | null
  result_error_id?: number | null
  error_message?: string | null
}

export type OpsUpstreamErrorEvent = {
  at_unix_ms?: number
  platform?: string
  account_id?: number
  account_name?: string
  upstream_status_code?: number
  upstream_request_id?: string
  upstream_request_body?: string
  kind?: string
  message?: string
  detail?: string
}

export interface OpsRetryResult {
  attempt_id: number
  mode: OpsRetryMode
  status: 'running' | 'succeeded' | 'failed' | string

  pinned_account_id?: number | null
  used_account_id?: number | null

  http_status_code: number
  upstream_request_id: string

  response_preview: string
  response_truncated: boolean

  error_message: string

  started_at: string
  finished_at: string
  duration_ms: number
}

export type OpsRequestKind = 'success' | 'error'
export type OpsRequestDetailsKind = OpsRequestKind | 'all'
export type OpsRequestDetailsSort = 'created_at_desc' | 'duration_desc'

export interface OpsRequestDetail {
  kind: OpsRequestKind
  created_at: string
  request_id: string

  platform?: string
  model?: string
  duration_ms?: number | null
  status_code?: number | null

  error_id?: number | null
  phase?: string
  severity?: string
  message?: string

  user_id?: number | null
  api_key_id?: number | null
  account_id?: number | null
  group_id?: number | null

  stream?: boolean
}

export interface OpsRequestDetailsParams {
  time_range?: '5m' | '30m' | '1h' | '6h' | '24h'
  start_time?: string
  end_time?: string

  kind?: OpsRequestDetailsKind

  platform?: string
  group_id?: number | null

  user_id?: number
  api_key_id?: number
  account_id?: number

  model?: string
  request_id?: string
  q?: string

  min_duration_ms?: number
  max_duration_ms?: number

  sort?: OpsRequestDetailsSort

  page?: number
  page_size?: number
}

export type OpsRequestDetailsResponse = PaginatedResponse<OpsRequestDetail>

export interface PlatformConcurrencyInfo {
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface GroupConcurrencyInfo {
  group_id: number
  group_name: string
  platform: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface AccountConcurrencyInfo {
  account_id: number
  account_name?: string
  platform: string
  group_id: number
  group_name: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface OpsConcurrencyStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformConcurrencyInfo>
  group: Record<string, GroupConcurrencyInfo>
  account: Record<string, AccountConcurrencyInfo>
  timestamp?: string
}

export interface UserConcurrencyInfo {
  user_id: number
  user_email: string
  username: string
  current_in_use: number
  max_capacity: number
  load_percentage: number
  waiting_in_queue: number
}

export interface OpsUserConcurrencyStatsResponse {
  enabled: boolean
  user: Record<string, UserConcurrencyInfo>
  timestamp?: string
}

export async function getConcurrencyStats(platform?: string, groupId?: number | null): Promise<OpsConcurrencyStatsResponse> {
  const params: Record<string, any> = {}
  if (platform) {
    params.platform = platform
  }
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  }

  const { data } = await apiClient.get<OpsConcurrencyStatsResponse>('/admin/ops/concurrency', { params })
  return data
}

export async function getUserConcurrencyStats(): Promise<OpsUserConcurrencyStatsResponse> {
  const { data } = await apiClient.get<OpsUserConcurrencyStatsResponse>('/admin/ops/user-concurrency')
  return data
}

export interface PlatformAvailability {
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
}

export interface GroupAvailability {
  group_id: number
  group_name: string
  platform: string
  total_accounts: number
  available_count: number
  rate_limit_count: number
  error_count: number
}

export interface AccountAvailability {
  account_id: number
  account_name: string
  platform: string
  group_id: number
  group_name: string
  status: string
  is_available: boolean
  is_rate_limited: boolean
  rate_limit_reset_at?: string
  rate_limit_remaining_sec?: number
  is_overloaded: boolean
  overload_until?: string
  overload_remaining_sec?: number
  has_error: boolean
  error_message?: string
}

export interface OpsAccountAvailabilityStatsResponse {
  enabled: boolean
  platform: Record<string, PlatformAvailability>
  group: Record<string, GroupAvailability>
  account: Record<string, AccountAvailability>
  timestamp?: string
}

export async function getAccountAvailabilityStats(platform?: string, groupId?: number | null): Promise<OpsAccountAvailabilityStatsResponse> {
  const params: Record<string, any> = {}
  if (platform) {
    params.platform = platform
  }
  if (typeof groupId === 'number' && groupId > 0) {
    params.group_id = groupId
  }
  const { data } = await apiClient.get<OpsAccountAvailabilityStatsResponse>('/admin/ops/account-availability', { params })
  return data
}

export type OpsSeverity = string
export type OpsPhase = string

export interface OpsRuntimeLogConfig {
  level: 'debug' | 'info' | 'warn' | 'error'
  enable_sampling: boolean
  sampling_initial: number
  sampling_thereafter: number
  caller: boolean
  stacktrace_level: 'none' | 'error' | 'fatal'
  retention_days: number
  source?: string
  updated_at?: string
  updated_by_user_id?: number
}

export interface OpsErrorLog {
  id: number
  created_at: string

  // Standardized classification
  phase: OpsPhase
  type: string
  error_owner: 'client' | 'provider' | 'platform' | string
  error_source: 'client_request' | 'upstream_http' | 'gateway' | string

  severity: OpsSeverity
  status_code: number
  platform: string
  model: string

  is_retryable: boolean
  retry_count: number

  resolved: boolean
  resolved_at?: string | null
  resolved_by_user_id?: number | null
  resolved_retry_id?: number | null

  client_request_id: string
  request_id: string
  message: string

  user_id?: number | null
  user_email: string
  api_key_id?: number | null
  account_id?: number | null
  account_name: string
  group_id?: number | null
  group_name: string

  client_ip?: string | null
  request_path?: string
  stream?: boolean

  // Error observability context (endpoint + model mapping)
  inbound_endpoint?: string
  upstream_endpoint?: string
  requested_model?: string
  upstream_model?: string
  request_type?: number | null
}

export interface OpsErrorDetail extends OpsErrorLog {
  error_body: string
  user_agent: string

  // Upstream context (optional; enriched by gateway services)
  upstream_status_code?: number | null
  upstream_error_message?: string
  upstream_error_detail?: string
  upstream_errors?: string

  auth_latency_ms?: number | null
  routing_latency_ms?: number | null
  upstream_latency_ms?: number | null
  response_latency_ms?: number | null
  time_to_first_token_ms?: number | null

  request_body: string
  request_body_truncated: boolean
  request_body_bytes?: number | null

  is_business_limited: boolean
}

export type OpsErrorLogsResponse = PaginatedResponse<OpsErrorLog>

export type OpsErrorListView = 'errors' | 'excluded' | 'all'

export type OpsErrorListQueryParams = {
  page?: number
  page_size?: number
  time_range?: string
  start_time?: string
  end_time?: string
  platform?: string
  group_id?: number | null
  account_id?: number | null

  phase?: string
  error_owner?: string
  error_source?: string
  resolved?: string
  view?: OpsErrorListView

  q?: string
  status_codes?: string
  status_codes_other?: string
}

// Legacy unified endpoints
export async function listErrorLogs(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/errors', { params })
  return data
}

export async function getErrorLogDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/errors/${id}`)
  return data
}

export async function retryErrorRequest(id: number, req: OpsRetryRequest): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/errors/${id}/retry`, req)
  return data
}

export async function listRetryAttempts(errorId: number, limit = 50): Promise<OpsRetryAttempt[]> {
  const { data } = await apiClient.get<OpsRetryAttempt[]>(`/admin/ops/errors/${errorId}/retries`, { params: { limit } })
  return data
}

export async function updateErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/errors/${errorId}/resolve`, { resolved })
}

// New split endpoints
export async function listRequestErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/request-errors', { params })
  return data
}

export async function listUpstreamErrors(params: OpsErrorListQueryParams): Promise<OpsErrorLogsResponse> {
  const { data } = await apiClient.get<OpsErrorLogsResponse>('/admin/ops/upstream-errors', { params })
  return data
}

export async function getRequestErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/request-errors/${id}`)
  return data
}

export async function getUpstreamErrorDetail(id: number): Promise<OpsErrorDetail> {
  const { data } = await apiClient.get<OpsErrorDetail>(`/admin/ops/upstream-errors/${id}`)
  return data
}

export async function retryRequestErrorClient(id: number): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/request-errors/${id}/retry-client`, {})
  return data
}

export async function retryRequestErrorUpstreamEvent(id: number, idx: number): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/request-errors/${id}/upstream-errors/${idx}/retry`, {})
  return data
}

export async function retryUpstreamError(id: number): Promise<OpsRetryResult> {
  const { data } = await apiClient.post<OpsRetryResult>(`/admin/ops/upstream-errors/${id}/retry`, {})
  return data
}

export async function updateRequestErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/request-errors/${errorId}/resolve`, { resolved })
}

export async function updateUpstreamErrorResolved(errorId: number, resolved: boolean): Promise<void> {
  await apiClient.put(`/admin/ops/upstream-errors/${errorId}/resolve`, { resolved })
}

export async function listRequestErrorUpstreamErrors(
  id: number,
  params: OpsErrorListQueryParams = {},
  options: { include_detail?: boolean } = {}
): Promise<PaginatedResponse<OpsErrorDetail>> {
  const query: Record<string, any> = { ...params }
  if (options.include_detail) query.include_detail = '1'
  const { data } = await apiClient.get<PaginatedResponse<OpsErrorDetail>>(`/admin/ops/request-errors/${id}/upstream-errors`, { params: query })
  return data
}

export async function listRequestDetails(params: OpsRequestDetailsParams): Promise<OpsRequestDetailsResponse> {
  const { data } = await apiClient.get<OpsRequestDetailsResponse>('/admin/ops/requests', { params })
  return data
}

// Runtime logging config
export async function getRuntimeLogConfig(): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.get<OpsRuntimeLogConfig>('/admin/ops/runtime/logging')
  return data
}

export async function updateRuntimeLogConfig(config: OpsRuntimeLogConfig): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.put<OpsRuntimeLogConfig>('/admin/ops/runtime/logging', config)
  return data
}

export async function resetRuntimeLogConfig(): Promise<OpsRuntimeLogConfig> {
  const { data } = await apiClient.post<OpsRuntimeLogConfig>('/admin/ops/runtime/logging/reset')
  return data
}

export const opsAPI = {
  getConcurrencyStats,
  getUserConcurrencyStats,
  getAccountAvailabilityStats,

  // Legacy unified endpoints
  listErrorLogs,
  getErrorLogDetail,
  retryErrorRequest,
  listRetryAttempts,
  updateErrorResolved,

  // New split endpoints
  listRequestErrors,
  listUpstreamErrors,
  getRequestErrorDetail,
  getUpstreamErrorDetail,
  retryRequestErrorClient,
  retryRequestErrorUpstreamEvent,
  retryUpstreamError,
  updateRequestErrorResolved,
  updateUpstreamErrorResolved,
  listRequestErrorUpstreamErrors,

  listRequestDetails,
  getRuntimeLogConfig,
  updateRuntimeLogConfig,
  resetRuntimeLogConfig
}

export default opsAPI
