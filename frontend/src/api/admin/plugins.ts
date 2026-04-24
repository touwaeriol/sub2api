/**
 * Admin Plugins API endpoints.
 *
 * Wraps the backend Admin plugin-management surface defined by developer A:
 *
 *   GET    /api/v1/plugins                           — public manifest
 *   GET    /api/v1/admin/plugins                     — admin list
 *   GET    /api/v1/admin/plugins/:id                 — admin detail
 *   POST   /api/v1/admin/plugins/:id/enable          — enable
 *   POST   /api/v1/admin/plugins/:id/disable         — disable
 *   POST   /api/v1/admin/plugins/:id/uninstall       — uninstall (body {purge})
 *   GET    /api/v1/admin/plugins/dead-letters        — dead-letter list
 *   POST   /api/v1/admin/plugins/dead-letters/:id/retry
 *   DELETE /api/v1/admin/plugins/dead-letters/:id
 *
 * The apiClient interceptor unwraps `{ code, message, data }` so functions
 * below return the inner data directly.
 */

import { apiClient } from '../client'

export type PluginState = 'installed' | 'enabled' | 'disabled' | 'uninstalled'

export interface PluginDependency {
  id: string
  versionReq: string
  optional: boolean
}

export interface PluginDTO {
  id: string
  name: string
  version: string
  apiVersion: string
  state: PluginState
  installedAt: number
  lastEnabledAt?: number
  declaredTables: string[]
  dependencies: PluginDependency[]
  permissions: string[]
  description?: string
  author?: string
}

export interface DeadLetterDTO {
  id: number
  pluginId: string
  eventTopic: string
  payload: unknown
  error: string
  attempts: number
  firstSeenAt: number
  lastSeenAt: number
}

export interface DeadLetterListParams {
  pluginId?: string
  topic?: string
  page?: number
  pageSize?: number
}

// ==================== Public (non-admin) manifest ====================

/**
 * Fetch the publicly visible plugin manifest (enabled plugins only).
 * Used by the frontend bootstrap flow to decide which UI modules to install.
 */
export async function listPublicPlugins(): Promise<PluginDTO[]> {
  // apiClient baseURL is /api/v1, so we can use the relative path.
  const { data } = await apiClient.get<PluginDTO[]>('/plugins')
  return data ?? []
}

// ==================== Admin plugin list ====================

export async function listPlugins(): Promise<PluginDTO[]> {
  const { data } = await apiClient.get<PluginDTO[]>('/admin/plugins')
  return data ?? []
}

export async function getPlugin(id: string): Promise<PluginDTO> {
  const { data } = await apiClient.get<PluginDTO>(`/admin/plugins/${encodeURIComponent(id)}`)
  return data
}

export async function enablePlugin(id: string): Promise<void> {
  await apiClient.post(`/admin/plugins/${encodeURIComponent(id)}/enable`)
}

export async function disablePlugin(id: string): Promise<void> {
  await apiClient.post(`/admin/plugins/${encodeURIComponent(id)}/disable`)
}

export async function uninstallPlugin(id: string, purge: boolean): Promise<void> {
  await apiClient.post(`/admin/plugins/${encodeURIComponent(id)}/uninstall`, { purge })
}

// ==================== Dead-letter queue ====================

export async function listDeadLetters(params: DeadLetterListParams = {}): Promise<DeadLetterDTO[]> {
  const { data } = await apiClient.get<DeadLetterDTO[]>('/admin/plugins/dead-letters', {
    params: {
      plugin_id: params.pluginId,
      topic: params.topic,
      page: params.page,
      page_size: params.pageSize
    }
  })
  return data ?? []
}

export async function retryDeadLetter(id: number): Promise<void> {
  await apiClient.post(`/admin/plugins/dead-letters/${id}/retry`)
}

export async function deleteDeadLetter(id: number): Promise<void> {
  await apiClient.delete(`/admin/plugins/dead-letters/${id}`)
}
