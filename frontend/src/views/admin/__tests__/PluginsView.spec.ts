import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import PluginsView from '../PluginsView.vue'

const { listPlugins, enablePlugin, disablePlugin, uninstallPlugin } = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  enablePlugin: vi.fn(),
  disablePlugin: vi.fn(),
  uninstallPlugin: vi.fn()
}))

vi.mock('@/api/admin/plugins', () => ({
  listPlugins,
  enablePlugin,
  disablePlugin,
  uninstallPlugin
}))

const showError = vi.fn()
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string>) => {
        if (!params) return key
        return `${key}:${JSON.stringify(params)}`
      }
    })
  }
})

// Layout + common components are shallow-stubbed so the test focuses on the
// view's own logic (state branching, handler wiring) rather than chrome.
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { name: 'AppLayout', template: '<div><slot /></div>' }
}))
vi.mock('@/components/layout/TablePageLayout.vue', () => ({
  default: { name: 'TablePageLayout', template: '<div><slot name="filters" /><slot /></div>' }
}))
vi.mock('@/components/common/StatusBadge.vue', () => ({
  default: { name: 'StatusBadge', template: '<span />', props: ['status', 'label'] }
}))
vi.mock('@/components/common/ConfirmDialog.vue', () => ({
  default: {
    name: 'ConfirmDialog',
    template: '<div v-if="show" class="confirm-dialog"><button class="confirm" @click="$emit(\'confirm\')" /></div>',
    props: ['show', 'title', 'message', 'confirmText', 'danger'],
    emits: ['confirm', 'cancel']
  }
}))

describe('PluginsView', () => {
  beforeEach(() => {
    listPlugins.mockReset()
    enablePlugin.mockReset()
    disablePlugin.mockReset()
    uninstallPlugin.mockReset()
    showError.mockReset()
  })

  it('loads plugins on mount and renders a row per plugin', async () => {
    listPlugins.mockResolvedValueOnce([
      { id: 'a', name: 'Alpha', version: '1.0.0', state: 'enabled', installedAt: 0, declaredTables: [], dependencies: [], permissions: [] },
      { id: 'b', name: 'Beta', version: '1.0.0', state: 'disabled', installedAt: 0, declaredTables: [], dependencies: [], permissions: [] }
    ])
    const wrapper = mount(PluginsView)
    await flushPromises()
    expect(listPlugins).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Alpha')
    expect(wrapper.text()).toContain('Beta')
  })

  it('shows empty state when the API returns no plugins', async () => {
    listPlugins.mockResolvedValueOnce([])
    const wrapper = mount(PluginsView)
    await flushPromises()
    expect(wrapper.text()).toContain('admin.plugins.empty')
  })

  it('calls enablePlugin when the Enable button is clicked', async () => {
    listPlugins.mockResolvedValueOnce([
      { id: 'a', name: 'Alpha', version: '1', state: 'disabled', installedAt: 0, declaredTables: [], dependencies: [], permissions: [] }
    ])
    enablePlugin.mockResolvedValueOnce(undefined)
    listPlugins.mockResolvedValueOnce([
      { id: 'a', name: 'Alpha', version: '1', state: 'enabled', installedAt: 0, declaredTables: [], dependencies: [], permissions: [] }
    ])
    const wrapper = mount(PluginsView)
    await flushPromises()
    await wrapper.find('tbody .btn-primary').trigger('click')
    await flushPromises()
    expect(enablePlugin).toHaveBeenCalledWith('a')
  })

  it('calls disablePlugin when the Disable button is clicked on enabled plugin', async () => {
    listPlugins.mockResolvedValueOnce([
      { id: 'a', name: 'Alpha', version: '1', state: 'enabled', installedAt: 0, declaredTables: [], dependencies: [], permissions: [] }
    ])
    disablePlugin.mockResolvedValueOnce(undefined)
    listPlugins.mockResolvedValueOnce([])
    const wrapper = mount(PluginsView)
    await flushPromises()
    // Only the row's Disable button is inside `tbody`. The filter-row
    // Refresh button (which also carries btn-secondary) lives above the
    // table, so scoping to tbody guarantees the right click target.
    await wrapper.find('tbody .btn-secondary').trigger('click')
    await flushPromises()
    expect(disablePlugin).toHaveBeenCalledWith('a')
  })

  it('requires confirmation before purging', async () => {
    listPlugins.mockResolvedValueOnce([
      { id: 'a', name: 'Alpha', version: '1', state: 'disabled', installedAt: 0, declaredTables: [], dependencies: [], permissions: [] }
    ])
    uninstallPlugin.mockResolvedValueOnce(undefined)
    listPlugins.mockResolvedValueOnce([])
    const wrapper = mount(PluginsView)
    await flushPromises()

    // Clicking purge alone doesn't trigger the API — it opens the dialog.
    await wrapper.find('.btn-danger').trigger('click')
    expect(uninstallPlugin).not.toHaveBeenCalled()

    // Confirming the dialog performs the purge.
    await wrapper.find('.confirm-dialog .confirm').trigger('click')
    await flushPromises()
    expect(uninstallPlugin).toHaveBeenCalledWith('a', true)
  })

  it('surfaces API failures via appStore.showError', async () => {
    listPlugins.mockRejectedValueOnce({ code: 50001, message: 'boom' })
    mount(PluginsView)
    await flushPromises()
    expect(showError).toHaveBeenCalled()
  })
})
