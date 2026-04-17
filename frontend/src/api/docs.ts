import { apiClient } from './client'

export interface DocsGroup {
  title: string
  items: DocsItem[]
}

export interface DocsItem {
  slug: string
  title: string
}

export interface DocsConfig {
  groups: DocsGroup[]
}

export async function getDocsConfig(): Promise<DocsConfig> {
  const { data } = await apiClient.get<DocsConfig>('/docs/config')
  return data
}

export async function getDocContent(slug: string): Promise<string> {
  const { data } = await apiClient.get<string>(`/docs/${slug}`, {
    responseType: 'text',
    transformResponse: [(data) => data] // prevent axios from parsing as JSON
  })
  return data
}
