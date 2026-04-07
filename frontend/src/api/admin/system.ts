/**
 * System API endpoints for admin operations
 */

import { apiClient } from '../client'

/**
 * Get current version
 */
export async function getVersion(): Promise<{ version: string }> {
  const { data } = await apiClient.get<{ version: string }>('/admin/system/version')
  return data
}

export const systemAPI = {
  getVersion
}

export default systemAPI
