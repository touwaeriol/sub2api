/**
 * Dynamic menu registry for plugin-contributed nav items.
 *
 * The sidebar reads `getVisible()` and merges the result after the hardcoded
 * items. Each plugin owns its entries; `remove(pluginId)` clears them when a
 * plugin is disabled or reloaded.
 */

import { ref, computed, type ComputedRef } from 'vue'
import type {
  DynamicMenuItem,
  MenuRegistryApi,
  MenuVisibility
} from './types'
import {
  MENU_VISIBILITY_ADMIN,
  MENU_VISIBILITY_USER,
  MENU_VISIBILITY_BOTH
} from './types'

const DEFAULT_ORDER = 1000

const items = ref<DynamicMenuItem[]>([])

function normalizeOrder(order?: number): number {
  return typeof order === 'number' && Number.isFinite(order) ? order : DEFAULT_ORDER
}

function matchesVisibility(item: DynamicMenuItem, target: MenuVisibility | 'admin' | 'user'): boolean {
  if (item.visibility === MENU_VISIBILITY_BOTH) return true
  if (target === MENU_VISIBILITY_BOTH) return true
  return item.visibility === target
}

function sortItems(list: DynamicMenuItem[]): DynamicMenuItem[] {
  return [...list].sort((a, b) => {
    const diff = normalizeOrder(a.order) - normalizeOrder(b.order)
    if (diff !== 0) return diff
    return a.path.localeCompare(b.path)
  })
}

export function useMenuRegistry(): MenuRegistryApi & {
  itemsRef: ComputedRef<DynamicMenuItem[]>
} {
  const register = (item: DynamicMenuItem): void => {
    const idx = items.value.findIndex(
      (existing) => existing.pluginId === item.pluginId && existing.path === item.path
    )
    const normalized: DynamicMenuItem = {
      ...item,
      order: normalizeOrder(item.order)
    }
    if (idx >= 0) {
      items.value.splice(idx, 1, normalized)
    } else {
      items.value.push(normalized)
    }
  }

  const remove = (pluginId: string): void => {
    items.value = items.value.filter((i) => i.pluginId !== pluginId)
  }

  const getVisible = (visibility: MenuVisibility | 'admin' | 'user'): DynamicMenuItem[] => {
    return sortItems(items.value.filter((item) => matchesVisibility(item, visibility)))
  }

  const all = (): DynamicMenuItem[] => sortItems(items.value)

  const itemsRef = computed(() => sortItems(items.value))

  return { register, remove, getVisible, all, itemsRef }
}

// Test-only helper to reset the singleton state between tests.
export function __resetMenuRegistryForTests(): void {
  items.value = []
}

export { MENU_VISIBILITY_ADMIN, MENU_VISIBILITY_USER, MENU_VISIBILITY_BOTH }
