/**
 * Host SDK type definitions for the host-bridge module.
 *
 * These mirror the plugin-sdk's host-sdk.ts interfaces but are declared
 * independently to avoid importing from the plugin-sdk package (which
 * lives outside the host's dependency tree).
 */
import type { Ref, Component, App } from 'vue'
import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type { AxiosInstance } from 'axios'

export type ThemeMode = 'light' | 'dark'
export type FontSize = 'sm' | 'base' | 'lg'

export interface HostTheme {
  mode: Ref<ThemeMode>
  primaryColor: Ref<string>
  toggle(): void
  set(mode: ThemeMode): void
}

export interface HostFont {
  size: Ref<FontSize>
  family: Ref<string>
}

export interface HostI18n {
  t(key: string, params?: Record<string, unknown>): string
  currentLocale: Ref<string>
  registerNamespace(
    namespace: string,
    messages: Record<string, Record<string, unknown>>,
  ): void
  onChange(callback: (locale: string) => void): () => void
}

export interface HostNotify {
  success(message: string, duration?: number): void
  error(message: string, duration?: number): void
  warning(message: string, duration?: number): void
  info(message: string, duration?: number): void
}

export interface HostRouter {
  push(path: string): Promise<unknown>
  back(): void
  currentRoute: Ref<RouteLocationNormalizedLoaded>
}

export interface HostAuth {
  user: Ref<{ id: number; username: string; [key: string]: unknown } | null>
  isAdmin: Ref<boolean>
  isAuthenticated: Ref<boolean>
}

export interface HostHttp {
  apiClient: AxiosInstance
}

export interface HostVue {
  h: typeof import('vue').h
  defineComponent: typeof import('vue').defineComponent
  ref: typeof import('vue').ref
  computed: typeof import('vue').computed
  watch: typeof import('vue').watch
  onMounted: typeof import('vue').onMounted
  onUnmounted: typeof import('vue').onUnmounted
}

export interface HostRuntimeAppOptions {
  components?: Record<string, Component>
  onError?: (err: unknown, instance: unknown, info: string) => void
}

export interface PluginAppInstance {
  app: App
  unmount(): void
}

export interface HostRuntime {
  createApp(
    rootComponent: Component,
    target: Element,
    options?: HostRuntimeAppOptions,
  ): PluginAppInstance
}

export interface SdkApiKey {
  id: number
  name: string
  key: string
  status: string
  expires_at: string | null
  group?: {
    id: number
    name: string
    platform: string
    subscription_type: string
    rate_multiplier: number
  }
}

export interface HostKeys {
  listActive(): Promise<SdkApiKey[]>
  getUserGroupRates(): Promise<Record<number, number>>
}

export interface UploadedImage {
  url: string
}

export interface HostImages {
  upload(file: File): Promise<UploadedImage>
}

export interface HostSdk {
  version: string
  theme: HostTheme
  font: HostFont
  i18n: HostI18n
  notify: HostNotify
  router: HostRouter
  auth: HostAuth
  http: HostHttp
  vue: HostVue
  runtime: HostRuntime
  keys: HostKeys
  images: HostImages
}
