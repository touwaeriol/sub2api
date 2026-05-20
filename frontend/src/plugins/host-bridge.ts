/**
 * Host SDK bridge — 构建并暴露 HostSdk 给插件前端 bundle。
 *
 * 职责：
 *   1. 创建 HostSdk 对象（theme / i18n / notify / router / auth / http / vue / runtime / keys / images）
 *   2. 将 HostSdk 挂到 window.__SUB2API_HOST_SDK__（插件 entry.js 通过 SDK helper 读取）
 *   3. 将 Vue / VueRouter / VueI18n / Pinia / Axios 暴露为全局变量供 importmap 使用
 *
 * 安全措施：
 *   - registerNamespace 拒绝覆盖核心 i18n 顶层 key（P0-5）
 *   - window.__SUB2API_HOST_SDK__ 进行深度冻结，防止外部篡改拦截器（P1-4）
 */
import {
  ref, computed, watch, h, defineComponent,
  onMounted, onUnmounted, createApp,
  type Ref, type Component,
} from 'vue'
import type { Router } from 'vue-router'
import { storeToRefs } from 'pinia'
import type { Pinia } from 'pinia'

import i18n from '@/i18n'
import { getLocale } from '@/i18n'
import { apiClient } from '@/api/client'
import { useAppStore, useAuthStore } from '@/stores'
import * as keysApi from '@/api/keys'
import { getUserGroupRates } from '@/api/groups'

import type {
  ThemeMode, FontSize,
  HostTheme, HostFont, HostI18n, HostNotify, HostRouter, HostAuth,
  HostHttp, HostVue, HostRuntime, HostRuntimeAppOptions, PluginAppInstance,
  HostKeys, HostImages, HostSdk, SdkApiKey, UploadedImage,
} from './host-bridge-types'

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const HOST_SDK_VERSION = '1.0.0'
const HOST_SDK_GLOBAL_KEY = '__SUB2API_HOST_SDK__'
const THEME_STORAGE_KEY = 'theme'
const DEFAULT_PRIMARY_COLOR = '#3b82f6'
const DEFAULT_FONT_FAMILY =
  'Inter, ui-sans-serif, system-ui, -apple-system, ' +
  '"Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'

/** provide/inject key — shared via Symbol.for for cross-bundle identity */
const PLUGIN_TELEPORT_TARGET = Symbol.for('sub2api.plugin-teleport-target')

/**
 * Core i18n top-level keys that plugins must NOT overwrite.
 * Derived from frontend/src/i18n/locales/{en,zh}.ts top-level keys.
 */
const CORE_I18N_KEYS: ReadonlySet<string> = new Set([
  'home', 'keyUsage', 'setup', 'common', 'nav', 'auth', 'dashboard',
  'groups', 'keys', 'usage', 'redeem', 'profile', 'empty', 'table',
  'pagination', 'errors', 'dates', 'admin', 'subscriptionProgress',
  'version', 'purchase', 'customPage', 'announcements',
  'userSubscriptions', 'onboarding', 'payment',
])

// ---------------------------------------------------------------------------
// SDK section builders
// ---------------------------------------------------------------------------

function createHostTheme(): HostTheme {
  const initial: ThemeMode =
    document.documentElement.classList.contains('dark') ? 'dark' : 'light'
  const mode = ref<ThemeMode>(initial)
  const primaryColor = ref(DEFAULT_PRIMARY_COLOR)

  if (typeof MutationObserver !== 'undefined') {
    new MutationObserver(() => {
      const cur: ThemeMode =
        document.documentElement.classList.contains('dark') ? 'dark' : 'light'
      if (cur !== mode.value) mode.value = cur
    }).observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['class'],
    })
  }

  function set(m: ThemeMode): void {
    mode.value = m
    document.documentElement.classList.toggle('dark', m === 'dark')
    try { localStorage.setItem(THEME_STORAGE_KEY, m) } catch { /* */ }
  }

  function toggle(): void {
    set(mode.value === 'dark' ? 'light' : 'dark')
  }

  return { mode, primaryColor, toggle, set }
}

function createHostFont(): HostFont {
  return {
    size: ref<FontSize>('base'),
    family: ref(DEFAULT_FONT_FAMILY),
  }
}

function createHostI18n(): HostI18n {
  const localeRef = i18n.global.locale
  const currentLocale = ref<string>(localeRef.value || getLocale())

  watch(
    () => localeRef.value,
    (v) => { currentLocale.value = v },
    { immediate: false },
  )

  function t(key: string, params?: Record<string, unknown>): string {
    return params && Object.keys(params).length > 0
      ? i18n.global.t(key, params)
      : i18n.global.t(key)
  }

  const registered = new Set<string>()

  /**
   * Register plugin i18n messages, merging into the host vue-i18n instance.
   *
   * P0-5 security: rejects namespaces whose messages contain keys that
   * collide with core i18n top-level keys (e.g. 'common', 'admin', 'auth').
   */
  function registerNamespace(
    namespace: string,
    messages: Record<string, Record<string, unknown>>,
  ): void {
    if (!namespace || registered.has(namespace)) return

    // P0-5: check every locale for core key conflicts
    for (const [locale, localeMessages] of Object.entries(messages)) {
      if (!localeMessages || typeof localeMessages !== 'object') continue
      const conflicts = Object.keys(localeMessages)
        .filter((k) => CORE_I18N_KEYS.has(k))
      if (conflicts.length > 0) {
        // eslint-disable-next-line no-console
        console.warn(
          `[plugin i18n] namespace "${namespace}" rejected: ` +
          `locale "${locale}" contains keys that conflict with core i18n: ` +
          `[${conflicts.join(', ')}]. ` +
          `Plugins must use unique top-level keys to avoid overwriting ` +
          `host translations.`,
        )
        return
      }
    }

    registered.add(namespace)
    for (const [locale, localeMessages] of Object.entries(messages)) {
      if (!localeMessages || typeof localeMessages !== 'object') continue
      i18n.global.mergeLocaleMessage(locale, localeMessages)
    }
  }

  function onChange(cb: (locale: string) => void): () => void {
    return watch(currentLocale, (v) => cb(v))
  }

  return { t, currentLocale, registerNamespace, onChange }
}

function createHostNotify(): HostNotify {
  const store = useAppStore()
  return {
    success: (m, d) => store.showSuccess(m, d ?? 3000),
    error: (m, d) => store.showError(m, d ?? 5000),
    warning: (m, d) => store.showWarning(m, d ?? 4000),
    info: (m, d) => store.showInfo(m, d ?? 3000),
  }
}

function createHostRouter(router: Router): HostRouter {
  return {
    push: (path) => router.push(path),
    back: () => router.back(),
    currentRoute: router.currentRoute,
  }
}

function createHostAuth(): HostAuth {
  const auth = useAuthStore()
  const refs = storeToRefs(auth)
  return {
    user: refs.user as Ref<{ id: number; username: string; [key: string]: unknown } | null>,
    isAdmin: refs.isAdmin as Ref<boolean>,
    isAuthenticated: refs.isAuthenticated as Ref<boolean>,
  }
}

function createHostHttp(): HostHttp {
  return { apiClient }
}

function createHostVue(): HostVue {
  return { h, defineComponent, ref, computed, watch, onMounted, onUnmounted }
}

function createHostRuntime(pinia: Pinia, router: Router): HostRuntime {
  function create(
    rootComponent: Component,
    target: Element,
    options?: HostRuntimeAppOptions,
  ): PluginAppInstance {
    const app = createApp(rootComponent)
    app.use(pinia)
    app.use(i18n)
    app.use(router)

    if (options?.components) {
      for (const [name, comp] of Object.entries(options.components)) {
        app.component(name, comp)
      }
    }

    // Provide teleport target inside ShadowRoot
    const rootNode = target.getRootNode()
    if (rootNode instanceof ShadowRoot) {
      const portal = rootNode.querySelector('.plugin-shadow-portal')
      if (portal) app.provide(PLUGIN_TELEPORT_TARGET, portal)
    }

    app.config.errorHandler = (err, inst, info) => {
      if (options?.onError) {
        options.onError(err, inst, info)
        return
      }
      const msg = err instanceof Error ? err.message : String(err)
      // eslint-disable-next-line no-console
      console.error('[plugin runtime] vue error', msg, info, err)
      try { useAppStore().showError(`Plugin error: ${msg}`) } catch { /* */ }
    }

    app.mount(target)
    return { app, unmount: () => app.unmount() }
  }

  return { createApp: create }
}

function createHostKeys(): HostKeys {
  return {
    async listActive(): Promise<SdkApiKey[]> {
      const result = await keysApi.list(1, 100, { status: 'active' })
      const now = Date.now()
      return (result.items || [])
        .filter((k) => {
          if (k.status !== 'active') return false
          if (k.expires_at && new Date(k.expires_at).getTime() <= now) {
            return false
          }
          return true
        })
        .map((k) => ({
          id: k.id,
          name: k.name,
          key: k.key,
          status: k.status,
          expires_at: k.expires_at ?? null,
          group: k.group
            ? {
                id: k.group.id,
                name: k.group.name,
                platform: k.group.platform,
                subscription_type: k.group.subscription_type ?? '',
                rate_multiplier: k.group.rate_multiplier ?? 1,
              }
            : undefined,
        }))
    },
    getUserGroupRates,
  }
}

function createHostImages(): HostImages {
  return {
    async upload(file: File): Promise<UploadedImage> {
      const fd = new FormData()
      fd.append('file', file)
      const resp = await apiClient.post('/admin/uploads/image', fd)
      return resp.data as UploadedImage
    },
  }
}

// ---------------------------------------------------------------------------
// SDK assembly + freeze (P1-4)
// ---------------------------------------------------------------------------

function buildHostSdk(router: Router, pinia: Pinia): HostSdk {
  return {
    version: HOST_SDK_VERSION,
    theme: createHostTheme(),
    font: createHostFont(),
    i18n: createHostI18n(),
    notify: createHostNotify(),
    router: createHostRouter(router),
    auth: createHostAuth(),
    http: createHostHttp(),
    vue: createHostVue(),
    runtime: createHostRuntime(pinia, router),
    keys: createHostKeys(),
    images: createHostImages(),
  }
}

/**
 * Deep-freeze an object: freeze the object itself plus all enumerable
 * own properties that are plain objects. Skips Vue Refs (which must
 * remain mutable internally) and functions.
 */
function deepFreeze<T extends Record<string, unknown>>(obj: T): T {
  Object.freeze(obj)
  for (const value of Object.values(obj)) {
    if (
      value !== null &&
      typeof value === 'object' &&
      !Object.isFrozen(value) &&
      !isVueRef(value)
    ) {
      deepFreeze(value as Record<string, unknown>)
    }
  }
  return obj
}

/** Best-effort check for Vue Ref without importing internal symbol. */
function isVueRef(value: unknown): boolean {
  return (
    typeof value === 'object' &&
    value !== null &&
    '__v_isRef' in (value as Record<string, unknown>)
  )
}

// ---------------------------------------------------------------------------
// Singleton + global exposure
// ---------------------------------------------------------------------------

let cachedSdk: HostSdk | null = null

/**
 * Create (or return cached) HostSdk and attach to window global.
 * The exposed object is deep-frozen (P1-4) to prevent external code from
 * injecting malicious interceptors on http.apiClient or auth references.
 */
export function getOrCreateHostSdk(router: Router, pinia: Pinia): HostSdk {
  if (cachedSdk) return cachedSdk

  const sdk = buildHostSdk(router, pinia)
  cachedSdk = sdk

  if (typeof window !== 'undefined') {
    deepFreeze(sdk as unknown as Record<string, unknown>)
    ;(window as unknown as Record<string, unknown>)[HOST_SDK_GLOBAL_KEY] = sdk
  }
  return sdk
}

/**
 * Get previously created SDK (for loader-runtime to call).
 */
export function getHostSdk(): HostSdk | null {
  if (cachedSdk) return cachedSdk
  if (typeof window === 'undefined') return null
  const w = window as unknown as Record<string, unknown>
  return (w[HOST_SDK_GLOBAL_KEY] as HostSdk | undefined) ?? null
}

/**
 * Expose shared libraries as window globals for plugin importmap resolution.
 */
export function exposeSharedLibs(): void {
  if (typeof window === 'undefined') return

  // These are assigned synchronously from the main bundle — no dynamic import
  // needed since vite bundles them into the vendor chunks anyway.
  const w = window as unknown as Record<string, unknown>

  // Use dynamic import to avoid pulling the entire module graph into main chunk
  // eslint-disable-next-line @typescript-eslint/no-floating-promises
  Promise.all([
    import('vue').then((m) => { w.__SUB2API_HOST_VUE__ = m }),
    import('vue-router').then((m) => { w.__SUB2API_HOST_VUE_ROUTER__ = m }),
    import('vue-i18n').then((m) => { w.__SUB2API_HOST_VUE_I18N__ = m }),
    import('pinia').then((m) => { w.__SUB2API_HOST_PINIA__ = m }),
    import('axios').then((m) => {
      w.__SUB2API_HOST_AXIOS__ = (m as Record<string, unknown>).default ?? m
    }),
  ])
}
