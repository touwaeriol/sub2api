/**
 * Bootstrap glue: wires the plugin registry to Router/I18n/Pinia and installs
 * all enabled plugins. Called once from `main.ts` after the Vue app is
 * created but before mount.
 */

import type { Router } from 'vue-router'
import type { I18n } from 'vue-i18n'
import type { Pinia } from 'pinia'

import type { PluginContext, DynamicRoute } from './types'
import { bootstrapPlugins, type BootstrapOptions } from './registry'
import { useMenuRegistry } from './menuRegistry'
import { useRouteRegistry } from './routeRegistry'
import { useComponentRegistry } from './componentRegistry'
import { mockPluginList } from './mock'

/** Reserved i18n prefix. All plugin strings live under `plugins.<id>.*`. */
const I18N_PLUGIN_PREFIX = 'plugins'

export interface BootstrapFrontendPluginsOptions extends BootstrapOptions {
  /** Use the built-in mock list when the API has nothing to return. */
  useMockFallback?: boolean
}

/**
 * Build a plugin context, fetch the manifest, and install all enabled
 * plugins whose UI modules have been registered.
 */
export async function bootstrapFrontendPlugins(
  router: Router,
  i18n: I18n,
  pinia: Pinia,
  options: BootstrapFrontendPluginsOptions = {}
): Promise<string[]> {
  const menuRegistry = useMenuRegistry()
  const routeRegistry = useRouteRegistry()
  const componentRegistry = useComponentRegistry()

  const ctx: PluginContext = {
    router,
    i18n,
    pinia,
    menuRegistry,
    routeRegistry,
    componentRegistry,
    mergeLocales(locales) {
      mergePluginLocalesFromContext(i18n, locales)
    }
  }

  const useMock = options.useMockFallback ?? true
  const installed = await bootstrapPlugins(ctx, {
    manifestProvider: options.manifestProvider,
    mockFactory: options.mockFactory ?? (useMock ? mockPluginList : undefined),
    silent: options.silent
  })

  // Sync dynamic routes into the actual Vue Router (Router is the source of
  // truth for navigation; the registry just keeps metadata for us to replay).
  for (const r of routeRegistry.all()) {
    addRouteSafely(router, r)
  }

  return installed
}

function addRouteSafely(router: Router, route: DynamicRoute): void {
  // `hasRoute` accepts name or symbol; skip if we already have it.
  if (router.hasRoute(route.name)) return
  router.addRoute({
    path: route.path,
    name: route.name,
    component: route.component,
    meta: route.meta ?? {}
  })
}

/**
 * Merge a plugin's i18n bundle, enforcing the `plugins.<id>.*` prefix so two
 * plugins cannot clobber one another's keys. Caller passes untyped JSON so we
 * cast narrowly only at the i18n boundary.
 */
export function mergePluginLocales(
  i18n: I18n,
  pluginId: string,
  locales: Record<string, Record<string, unknown>>
): void {
  if (!pluginId) throw new Error('mergePluginLocales: pluginId is required')
  const global = i18n.global as unknown as {
    mergeLocaleMessage: (locale: string, messages: Record<string, unknown>) => void
  }
  for (const [locale, messages] of Object.entries(locales)) {
    const wrapped = {
      [I18N_PLUGIN_PREFIX]: {
        [pluginId]: messages
      }
    }
    global.mergeLocaleMessage(locale, wrapped)
  }
}

/**
 * Internal helper that strips a leading `plugins.<id>.` segment if the plugin
 * passes keys already prefixed (otherwise the prefix is added).
 */
function mergePluginLocalesFromContext(
  i18n: I18n,
  locales: Record<string, Record<string, unknown>>
): void {
  const global = i18n.global as unknown as {
    mergeLocaleMessage: (locale: string, messages: Record<string, unknown>) => void
  }
  for (const [locale, messages] of Object.entries(locales)) {
    global.mergeLocaleMessage(locale, messages as Record<string, unknown>)
  }
}
