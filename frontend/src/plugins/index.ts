/**
 * Public entry point of the frontend plugin system.
 *
 * Main app:
 *   import { bootstrapFrontendPlugins, registerPluginModule } from '@/plugins'
 *
 * Plugin authors:
 *   import { definePlugin, type PluginUIModule } from '@/plugins'
 */

export * from './types'
export {
  registerPluginModule,
  unregisterPluginModule,
  listRegisteredPluginIds,
  getPluginManifests,
  useInstalledPlugins,
  isPluginInstalled,
  bootstrapPlugins,
  resolveManifests
} from './registry'
export { useMenuRegistry } from './menuRegistry'
export { useRouteRegistry } from './routeRegistry'
export { useComponentRegistry } from './componentRegistry'
export {
  bootstrapFrontendPlugins,
  mergePluginLocales,
  type BootstrapFrontendPluginsOptions
} from './bootstrap'
export { mockPluginList } from './mock'
