import axios from 'axios'

export interface OpusMaxKeyStatus {
  window_token_limit: string
  window_tokens_used: string
  plan_name: string
  expires_at: string
  total_requests: number
  last_24h_requests: number
}

export interface OpusMaxAccountItem {
  id: number
  name: string
  status: string
  schedulable: boolean
  concurrency: number
  rpm: number
  window_tokens_limit: string
  window_tokens_used: string
  usage_percent: number
  plan_name: string
  expires_at: string
  total_requests: number
  last_24h_requests: number
}

export interface OpusMaxMonitorResponse {
  accounts: OpusMaxAccountItem[]
  updated_at: string
  total_count: number
}

export function getOpusMaxAccounts() {
  return axios
    .get<{ code: number; data: OpusMaxMonitorResponse }>('/api/v1/opusmax/accounts')
    .then((res) => res.data.data)
}