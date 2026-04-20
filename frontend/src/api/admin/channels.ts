/**
 * Admin Channels API endpoints
 * Handles channel management for administrators
 */

import { apiClient } from '../client'

export type BillingMode = 'token' | 'per_request' | 'image'

// ── 参数覆盖规则（param_overrides） ──────────────────────────
//
// 与 backend (service.ChannelParamOverrideRule) 字段对齐。规则按数组顺序应用，
// 没有独立的 priority 字段 —— 位置靠后的规则可覆盖前面规则写入的同路径值。

export const PARAM_OVERRIDE_TARGETS = ['body', 'header'] as const
export type ParamOverrideTarget = (typeof PARAM_OVERRIDE_TARGETS)[number]

export const PARAM_OVERRIDE_ACTIONS = ['set', 'merge', 'remove', 'append'] as const
export type ParamOverrideAction = (typeof PARAM_OVERRIDE_ACTIONS)[number]

export interface ChannelParamOverrideRule {
  enabled: boolean
  model_glob: string
  target: ParamOverrideTarget
  action: ParamOverrideAction
  path: string
  value: unknown
  description: string
}

export type ChannelParamOverrides = Record<string, ChannelParamOverrideRule[]>

export interface PricingInterval {
  id?: number
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  per_request_price: number | null
  sort_order: number
}

export interface ChannelModelPricing {
  id?: number
  platform: string
  models: string[]
  billing_mode: BillingMode
  input_price: number | null
  output_price: number | null
  cache_write_price: number | null
  cache_read_price: number | null
  image_output_price: number | null
  per_request_price: number | null
  intervals: PricingInterval[]
}

export interface Channel {
  id: number
  name: string
  description: string
  status: string
  billing_model_source: string // "requested" | "upstream" | "channel_mapped"
  restrict_models: boolean
  features?: string
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>> // platform → {src→dst}
  param_overrides?: ChannelParamOverrides // platform → ordered rule list
  created_at: string
  updated_at: string
}

export interface CreateChannelRequest {
  name: string
  description?: string
  group_ids?: number[]
  model_pricing?: ChannelModelPricing[]
  model_mapping?: Record<string, Record<string, string>>
  param_overrides?: ChannelParamOverrides
  billing_model_source?: string
  restrict_models?: boolean
  features?: string
}

export interface UpdateChannelRequest {
  name?: string
  description?: string
  status?: string
  group_ids?: number[]
  model_pricing?: ChannelModelPricing[]
  model_mapping?: Record<string, Record<string, string>>
  param_overrides?: ChannelParamOverrides
  billing_model_source?: string
  restrict_models?: boolean
  features?: string
}

interface PaginatedResponse<T> {
  items: T[]
  total: number
}

/**
 * List channels with pagination
 */
export async function list(
  page: number = 1,
  pageSize: number = 20,
  filters?: {
    status?: string
    search?: string
    sort_by?: string
    sort_order?: 'asc' | 'desc'
  },
  options?: { signal?: AbortSignal }
): Promise<PaginatedResponse<Channel>> {
  const { data } = await apiClient.get<PaginatedResponse<Channel>>('/admin/channels', {
    params: {
      page,
      page_size: pageSize,
      ...filters
    },
    signal: options?.signal
  })
  return data
}

/**
 * Get channel by ID
 */
export async function getById(id: number): Promise<Channel> {
  const { data } = await apiClient.get<Channel>(`/admin/channels/${id}`)
  return data
}

/**
 * Create a new channel
 */
export async function create(req: CreateChannelRequest): Promise<Channel> {
  const { data } = await apiClient.post<Channel>('/admin/channels', req)
  return data
}

/**
 * Update a channel
 */
export async function update(id: number, req: UpdateChannelRequest): Promise<Channel> {
  const { data } = await apiClient.put<Channel>(`/admin/channels/${id}`, req)
  return data
}

/**
 * Delete a channel
 */
export async function remove(id: number): Promise<void> {
  await apiClient.delete(`/admin/channels/${id}`)
}

export interface ModelDefaultPricing {
  found: boolean
  input_price?: number    // per-token price
  output_price?: number
  cache_write_price?: number
  cache_read_price?: number
  image_output_price?: number
}

export async function getModelDefaultPricing(model: string): Promise<ModelDefaultPricing> {
  const { data } = await apiClient.get<ModelDefaultPricing>('/admin/channels/model-pricing', {
    params: { model }
  })
  return data
}

const channelsAPI = { list, getById, create, update, remove, getModelDefaultPricing }
export default channelsAPI
