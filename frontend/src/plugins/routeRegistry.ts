/**
 * Dynamic route registry.
 *
 * Plugins declare routes via `ctx.routeRegistry.register()`. The bootstrapper
 * mirrors them into the Vue Router via `router.addRoute()`. Keeping a
 * separate registry lets us re-install or remove a plugin's routes without
 * touching other parts of the router.
 */

import { ref } from 'vue'
import type { DynamicRoute, RouteRegistryApi } from './types'

const routes = ref<DynamicRoute[]>([])

export function useRouteRegistry(): RouteRegistryApi {
  const register = (route: DynamicRoute): void => {
    const idx = routes.value.findIndex(
      (r) => r.pluginId === route.pluginId && r.path === route.path
    )
    if (idx >= 0) {
      routes.value.splice(idx, 1, route)
    } else {
      routes.value.push(route)
    }
  }

  const remove = (pluginId: string): void => {
    routes.value = routes.value.filter((r) => r.pluginId !== pluginId)
  }

  const all = (): DynamicRoute[] => [...routes.value]

  return { register, remove, all }
}

export function __resetRouteRegistryForTests(): void {
  routes.value = []
}
