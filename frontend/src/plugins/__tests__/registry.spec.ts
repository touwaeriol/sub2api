import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia } from 'pinia'
import { createI18n } from 'vue-i18n'
import { createRouter, createMemoryHistory } from 'vue-router'

import {
  registerPluginModule,
  unregisterPluginModule,
  listRegisteredPluginIds,
  bootstrapPlugins,
  isPluginInstalled,
  __resetRegistryForTests
} from '../registry'
import { __resetMenuRegistryForTests, useMenuRegistry } from '../menuRegistry'
import { __resetRouteRegistryForTests, useRouteRegistry } from '../routeRegistry'
import { __resetComponentRegistryForTests, useComponentRegistry } from '../componentRegistry'
import {
  definePlugin,
  MENU_VISIBILITY_ADMIN,
  type PluginContext,
  type PluginManifest
} from '../types'

function makeContext(): PluginContext {
  const router = createRouter({ history: createMemoryHistory(), routes: [] })
  const i18n = createI18n({ legacy: false, locale: 'en', messages: { en: {}, zh: {} } })
  const pinia = createPinia()
  return {
    router,
    i18n,
    pinia,
    menuRegistry: useMenuRegistry(),
    routeRegistry: useRouteRegistry(),
    componentRegistry: useComponentRegistry(),
    mergeLocales: () => {}
  }
}

const demoManifest: PluginManifest = {
  id: 'demo',
  name: 'Demo',
  enabled: true,
  menus: [
    {
      path: '/admin/plugins/demo',
      labelI18nKey: 'plugins.demo.name',
      visibility: MENU_VISIBILITY_ADMIN
    }
  ],
  settings: []
}

describe('plugin registry', () => {
  beforeEach(() => {
    __resetRegistryForTests()
    __resetMenuRegistryForTests()
    __resetRouteRegistryForTests()
    __resetComponentRegistryForTests()
    vi.restoreAllMocks()
  })

  it('registers and deduplicates plugin modules', () => {
    const mod = definePlugin({ id: 'demo', install: () => {} })
    registerPluginModule(mod)
    registerPluginModule(mod)
    expect(listRegisteredPluginIds()).toEqual(['demo'])
  })

  it('rejects modules with an empty id', () => {
    expect(() => registerPluginModule({ id: '', install: () => {} })).toThrow()
  })

  it('bootstraps only enabled plugins with a registered module', async () => {
    const installSpy = vi.fn()
    registerPluginModule({ id: 'demo', install: installSpy })
    registerPluginModule({ id: 'missing-enabled', install: vi.fn() })

    const ctx = makeContext()
    const installed = await bootstrapPlugins(ctx, {
      manifestProvider: async () => [
        demoManifest,
        { id: 'disabled', name: 'Disabled', enabled: false }
      ]
    })

    expect(installed).toEqual(['demo'])
    expect(installSpy).toHaveBeenCalledTimes(1)
    expect(isPluginInstalled('demo')).toBe(true)
    expect(isPluginInstalled('disabled')).toBe(false)
  })

  it('is idempotent — double bootstrap does not reinstall', async () => {
    const installSpy = vi.fn()
    registerPluginModule({ id: 'demo', install: installSpy })
    const ctx = makeContext()
    await bootstrapPlugins(ctx, { manifestProvider: async () => [demoManifest] })
    await bootstrapPlugins(ctx, { manifestProvider: async () => [demoManifest] })
    expect(installSpy).toHaveBeenCalledTimes(1)
  })

  it('swallows install errors but still records other plugins', async () => {
    registerPluginModule({ id: 'bad', install: () => { throw new Error('boom') } })
    registerPluginModule({ id: 'good', install: () => {} })
    const ctx = makeContext()
    const installed = await bootstrapPlugins(ctx, {
      silent: true,
      manifestProvider: async () => [
        { id: 'bad', name: 'Bad', enabled: true },
        { id: 'good', name: 'Good', enabled: true }
      ]
    })
    expect(installed).toEqual(['good'])
    expect(isPluginInstalled('bad')).toBe(false)
    expect(isPluginInstalled('good')).toBe(true)
  })

  it('unregisters modules on demand', () => {
    registerPluginModule({ id: 'demo', install: () => {} })
    unregisterPluginModule('demo')
    expect(listRegisteredPluginIds()).toEqual([])
  })
})
