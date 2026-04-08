/**
 * Admin Dashboard API endpoints
 * Usage analytics used by the admin Usage page
 */

import { apiClient } from '../client'
import type {
  DashboardStats,
  TrendDataPoint,
  ModelStat,
  GroupStat,
  UsageRequestType
} from '@/types'

export interface TrendParams {
  start_date?: string
  end_date?: string
  granularity?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  model?: string
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
}

export interface ModelStatsParams {
  start_date?: string
  end_date?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
  model_source?: 'requested' | 'upstream' | 'mapping'
}

export interface ModelStatsResponse {
  models: ModelStat[]
  start_date: string
  end_date: string
}

export async function getModelStats(params?: ModelStatsParams): Promise<ModelStatsResponse> {
  const { data } = await apiClient.get<ModelStatsResponse>('/admin/dashboard/models', { params })
  return data
}

export interface GroupStatsParams {
  start_date?: string
  end_date?: string
  user_id?: number
  api_key_id?: number
  account_id?: number
  group_id?: number
  request_type?: UsageRequestType
  stream?: boolean
  billing_type?: number | null
}

export interface GroupStatsResponse {
  groups: GroupStat[]
}

export async function getGroupStats(params?: GroupStatsParams): Promise<GroupStatsResponse> {
  const { data } = await apiClient.get<GroupStatsResponse>('/admin/dashboard/groups', { params })
  return data
}

export interface DashboardSnapshotV2Params extends TrendParams {
  include_stats?: boolean
  include_trend?: boolean
  include_model_stats?: boolean
  include_group_stats?: boolean
  include_users_trend?: boolean
}

export interface DashboardSnapshotV2Response {
  trend?: TrendDataPoint[]
  models?: ModelStat[]
  groups?: GroupStat[]
  stats?: DashboardStats
}

export async function getSnapshotV2(params?: DashboardSnapshotV2Params): Promise<DashboardSnapshotV2Response> {
  const { data } = await apiClient.get<DashboardSnapshotV2Response>('/admin/dashboard/snapshot-v2', {
    params
  })
  return data
}

export const dashboardAPI = {
  getModelStats,
  getGroupStats,
  getSnapshotV2
}

export default dashboardAPI
