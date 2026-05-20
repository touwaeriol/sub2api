import { apiClient } from '../client'
import type { Protocol } from '@/types'

export async function list(platform?: string): Promise<Protocol[]> {
  const { data } = await apiClient.get<Protocol[]>('/admin/protocols', {
    params: platform ? { platform } : undefined
  })
  return data
}

export const protocolsAPI = {
  list
}

export default protocolsAPI
