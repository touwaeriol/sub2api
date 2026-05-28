import { apiClient } from './client'

export interface MonitorAccountItem {
  name: string
  platform: string
  type: string
  concurrency: number
  status: string
  schedulable: boolean
  total_marked_cost: number
  today_standard_cost: number
}

export interface MonitorResponse {
  group_name: string
  platform: string
  accounts: MonitorAccountItem[]
  updated_at: string
}

export function getGroupAccountMonitor(platform: string, groupName: string) {
  return apiClient
    .get<MonitorResponse>(
      `/monitor/${encodeURIComponent(platform)}/${encodeURIComponent(groupName)}`
    )
    .then((res) => res.data)
}
