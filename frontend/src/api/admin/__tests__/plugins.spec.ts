import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AxiosInstance } from 'axios'

vi.mock('@/i18n', () => ({
  getLocale: () => 'en'
}))

describe('admin plugins API client', () => {
  let apiClient: AxiosInstance
  let adapter: ReturnType<typeof vi.fn>
  let pluginsApi: typeof import('../plugins')

  beforeEach(async () => {
    vi.resetModules()
    const clientMod = await import('@/api/client')
    apiClient = clientMod.apiClient
    adapter = vi.fn().mockResolvedValue({
      status: 200,
      data: { code: 0, data: [] },
      headers: {},
      config: {},
      statusText: 'OK'
    })
    apiClient.defaults.adapter = adapter
    pluginsApi = await import('../plugins')
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('listPublicPlugins GETs /plugins', async () => {
    await pluginsApi.listPublicPlugins()
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('get')
    expect(cfg.url).toBe('/plugins')
  })

  it('listPlugins GETs /admin/plugins', async () => {
    await pluginsApi.listPlugins()
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('get')
    expect(cfg.url).toBe('/admin/plugins')
  })

  it('getPlugin encodes the ID in the URL', async () => {
    adapter.mockResolvedValueOnce({
      status: 200,
      data: { code: 0, data: { id: 'foo/bar', state: 'enabled' } },
      headers: {},
      config: {},
      statusText: 'OK'
    })
    await pluginsApi.getPlugin('foo/bar')
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.url).toBe('/admin/plugins/foo%2Fbar')
  })

  it('enablePlugin POSTs to the enable endpoint', async () => {
    await pluginsApi.enablePlugin('demo')
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('post')
    expect(cfg.url).toBe('/admin/plugins/demo/enable')
  })

  it('disablePlugin POSTs to the disable endpoint', async () => {
    await pluginsApi.disablePlugin('demo')
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('post')
    expect(cfg.url).toBe('/admin/plugins/demo/disable')
  })

  it('uninstallPlugin POSTs body {purge}', async () => {
    await pluginsApi.uninstallPlugin('demo', true)
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('post')
    expect(cfg.url).toBe('/admin/plugins/demo/uninstall')
    expect(cfg.data).toEqual(JSON.stringify({ purge: true }))
  })

  it('listDeadLetters maps camelCase params to snake_case', async () => {
    await pluginsApi.listDeadLetters({ pluginId: 'demo', pageSize: 50 })
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('get')
    expect(cfg.url).toBe('/admin/plugins/dead-letters')
    expect(cfg.params).toMatchObject({
      plugin_id: 'demo',
      page_size: 50
    })
  })

  it('retryDeadLetter POSTs to the retry endpoint', async () => {
    await pluginsApi.retryDeadLetter(42)
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('post')
    expect(cfg.url).toBe('/admin/plugins/dead-letters/42/retry')
  })

  it('deleteDeadLetter DELETEs the entry', async () => {
    await pluginsApi.deleteDeadLetter(42)
    const cfg = adapter.mock.calls[0][0]
    expect(cfg.method).toBe('delete')
    expect(cfg.url).toBe('/admin/plugins/dead-letters/42')
  })

  it('returns [] when the server returns null data', async () => {
    adapter.mockResolvedValueOnce({
      status: 200,
      data: { code: 0, data: null },
      headers: {},
      config: {},
      statusText: 'OK'
    })
    const result = await pluginsApi.listPlugins()
    expect(result).toEqual([])
  })
})
