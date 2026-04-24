/**
 * Slot component registry (Layer-2 optional).
 *
 * Plugins may register named Vue components that host views can pull into
 * predefined slots (e.g. a payment checkout widget slot). This is a lightweight
 * alternative to `provide`/`inject` for cross-plugin composition.
 */

import type { Component } from 'vue'
import type { ComponentRegistry } from './types'

const slots = new Map<string, Component>()

export function useComponentRegistry(): ComponentRegistry {
  return {
    register(slotName: string, component: Component): void {
      slots.set(slotName, component)
    },
    get(slotName: string): Component | undefined {
      return slots.get(slotName)
    },
    list(): string[] {
      return Array.from(slots.keys())
    }
  }
}

export function __resetComponentRegistryForTests(): void {
  slots.clear()
}
