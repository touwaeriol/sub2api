/**
 * Frontend plugin system types.
 *
 * Field names here intentionally mirror the backend SDK
 * (`backend/pkg/plugin/meta.go` and `resources.go`) so the
 * on-the-wire manifest can be consumed without translation.
 */

import type { Router } from 'vue-router'
import type { I18n } from 'vue-i18n'
import type { Pinia } from 'pinia'
import type { Component } from 'vue'

// ==================== Menu Visibility ====================

export const MENU_VISIBILITY_ADMIN = 'admin'
export const MENU_VISIBILITY_USER = 'user'
export const MENU_VISIBILITY_BOTH = 'both'

export type MenuVisibility =
  | typeof MENU_VISIBILITY_ADMIN
  | typeof MENU_VISIBILITY_USER
  | typeof MENU_VISIBILITY_BOTH

// ==================== Field Schema ====================

export const FIELD_TYPE_STRING = 'string'
export const FIELD_TYPE_INT = 'int'
export const FIELD_TYPE_BOOL = 'bool'
export const FIELD_TYPE_ENUM = 'enum'
export const FIELD_TYPE_SECRET = 'secret'
export const FIELD_TYPE_JSON = 'json'

export type FieldType =
  | typeof FIELD_TYPE_STRING
  | typeof FIELD_TYPE_INT
  | typeof FIELD_TYPE_BOOL
  | typeof FIELD_TYPE_ENUM
  | typeof FIELD_TYPE_SECRET
  | typeof FIELD_TYPE_JSON

export type FieldValue = string | number | boolean | null | Record<string, unknown> | unknown[]

export interface FieldOption {
  value: string | number | boolean
  i18nLabelKey: string
}

export interface FieldSchema {
  key: string
  type: FieldType
  i18nLabelKey: string
  i18nHintKey?: string
  default?: FieldValue
  secret?: boolean
  options?: FieldOption[]
  placeholder?: string
  validator?: string
  required?: boolean
}

// ==================== Plugin Manifest (from API) ====================

export interface PluginMenu {
  id?: string
  path: string
  labelI18nKey: string
  icon?: string
  iconSvg?: string
  visibility: MenuVisibility
  order?: number
  parent?: string
}

export interface PluginSetting {
  key: string
  i18nLabelKey: string
  i18nHintKey?: string
  default?: FieldValue
  secret?: boolean
  schema: FieldSchema
}

export interface PluginManifest {
  id: string
  name: string
  version?: string
  enabled: boolean
  menus?: PluginMenu[]
  settings?: PluginSetting[]
  /** Layer-2 bundle name, e.g. "@myorg/plugin-alipay-ui". */
  bundleName?: string
}

// ==================== Runtime Registry Types ====================

export interface DynamicMenuItem {
  pluginId: string
  path: string
  labelI18nKey: string
  icon?: string
  iconSvg?: string
  visibility: MenuVisibility
  order: number
}

export interface DynamicRoute {
  pluginId: string
  path: string
  name: string
  component: () => Promise<Component> | Component
  meta?: Record<string, unknown>
}

// ==================== Plugin Install Context (Layer 2) ====================

export interface ComponentRegistry {
  register(slotName: string, component: Component): void
  get(slotName: string): Component | undefined
  list(): string[]
}

export interface MenuRegistryApi {
  register(item: DynamicMenuItem): void
  remove(pluginId: string): void
  getVisible(visibility: MenuVisibility | 'admin' | 'user'): DynamicMenuItem[]
  all(): DynamicMenuItem[]
}

export interface RouteRegistryApi {
  register(route: DynamicRoute): void
  remove(pluginId: string): void
  all(): DynamicRoute[]
}

export interface PluginContext {
  router: Router
  i18n: I18n
  pinia: Pinia
  menuRegistry: MenuRegistryApi
  routeRegistry: RouteRegistryApi
  componentRegistry: ComponentRegistry
  /** Merge locale strings (auto-prefixed with `plugins.<id>.`). */
  mergeLocales(locales: Record<string, Record<string, unknown>>): void
}

export interface PluginUIModule {
  /** Must match the backend plugin ID. */
  id: string
  install(ctx: PluginContext): void
}

export function definePlugin(mod: PluginUIModule): PluginUIModule {
  return mod
}
