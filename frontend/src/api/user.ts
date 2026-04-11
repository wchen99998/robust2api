/**
 * User API endpoints
 * Handles user profile management and password changes
 */

import { apiClient } from './client'
import type { User, ChangePasswordRequest, BootstrapResponse } from '@/types'
import { normalizeBootstrapResponse } from './auth'

interface PatchMeResponse {
  subject?: Record<string, unknown>
  profile?: User
  roles?: string[]
  primary_role?: string
}

/**
 * Get current user profile
 * @returns User profile data
 */
export async function getProfile(): Promise<User> {
  const { data } = await apiClient.get<unknown>('/bootstrap')
  const normalized = normalizeBootstrapResponse(data)
  if (normalized.me?.user) {
    return normalized.me.user as User
  }
  throw new Error('Not authenticated')
}

/**
 * Update current user profile
 * @param profile - Profile data to update
 * @returns Updated user profile data
 */
export async function updateProfile(profile: {
  username?: string
}): Promise<User> {
  const { data } = await apiClient.patch<PatchMeResponse>('/me', profile)
  if (data.profile) {
    return data.profile
  }
  throw new Error('Invalid profile response')
}

/**
 * Change current user password
 * @param passwords - Old and new password
 * @returns Success message
 */
export async function changePassword(
  oldPassword: string,
  newPassword: string
): Promise<BootstrapResponse> {
  const payload: ChangePasswordRequest = {
    old_password: oldPassword,
    new_password: newPassword
  }

  const { data } = await apiClient.post<unknown>('/me/password/change', payload)
  return normalizeBootstrapResponse(data)
}

export const userAPI = {
  getProfile,
  updateProfile,
  changePassword
}

export default userAPI
