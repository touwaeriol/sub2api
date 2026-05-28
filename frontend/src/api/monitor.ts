import axios from 'axios'

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
  return axios
    .get<{ code: number; data: MonitorResponse }>('/api/v1/monitor/group', {
      params: { platform, group_name: groupName },
    })
    .then((res) => res.data.data)
}
