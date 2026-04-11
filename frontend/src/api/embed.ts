import { apiClient } from './client'
import type { EmbedTokenResponse } from '@/types'

export async function createEmbedToken(): Promise<EmbedTokenResponse> {
  const { data } = await apiClient.post<EmbedTokenResponse>('/embed-token', {})
  return data
}

export const embedAPI = {
  createEmbedToken
}

export default embedAPI
