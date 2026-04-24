/**
 * Core plugin registry.
 *
 * Layer 2 flow:
 *   1. Plugin authors ship an npm package (e.g. `@myorg/plugin-alipay-ui`)
 *      that default-exports a PluginUIModule.
 *   2. The main app imports these modules statically and calls
 *      `registerPluginModule(mod)` during bootstrap.
 *   3. `bootstrapPlugins(ctx)` fetches the backend manifest, determines
 *      which IDs are enabled, and invokes `install(ctx)` for each match.
 *
 * `bootstrapPlugins` is idempotent — calling it twice will not double-install
 * a plugin.
 */

import { ref, readonly, type Ref } from 'vue'
import type { PluginManifest, PluginUIModule, PluginContext } from './types'

// Static registration (populated by `registerPluginModule`).
const registered = new Map<string, PluginUIModule>()

// Runtime state: which IDs were actually installed.
const installed = ref<Set<string>>(new Set())
// Last manifest fetched from the backend.
const manifests = ref<PluginManifest[]>([])

/**
 * Register a plugin UI module. Call this at module top-level or before
 * `bootstrapPlugins`. Registering the same ID twice overwrites the previous
 * entry (useful for HMR).
 */
export function registerPluginModule(mod: PluginUIModule): void {
  if (!mod || typeof mod.id !== 'string' || mod.id.length === 0) {
    throw new Error('registerPluginModule: module must have a non-empty id')
  }
  registered.set(mod.id, mod)
}

/** Unregister a plugin UI module (primarily for tests / HMR). */
export function unregisterPluginModule(id: string): void {
  registered.delete(id)
}

/** Returns registered (not necessarily installed) plugin IDs. */
export function listRegisteredPluginIds(): string[] {
  return Array.from(registered.keys())
}

/** Returns a shallow copy of the last-fetched manifest list. */
export function getPluginManifests(): PluginManifest[] {
  return [...manifests.value]
}

/** Reactive view of the installed plugin set. */
export function useInstalledPlugins(): Readonly<Ref<ReadonlySet<string>>> {
  return readonly(installed)
}

export function isPluginInstalled(id: string): boolean {
  return installed.value.has(id)
}

// ==================== Manifest fetch ====================

const MANIFEST_ENDPOINT = '/api/v1/plugins'

/**
 * Fetch manifest from the real backend endpoint. Returns:
 *   - `null` when the endpoint is unreachable or returns 404/5xx (→ caller
 *     may fall back to mock in dev mode).
 *   - `[]` or a populated array when the endpoint succeeded (→ caller must
 *     NOT fall back to mock, an empty list is a valid answer).
 */
async function fetchManifestFromApi(): Promise<PluginManifest[] | null> {
  try {
    const resp = await fetch(MANIFEST_ENDPOINT, {
      method: 'GET',
      headers: { Accept: 'application/json' },
      credentials: 'same-origin'
    })
    if (resp.status === 404) return null
    if (!resp.ok) return null
    const body: unknown = await resp.json()
    const parsed = extractManifests(body)
    return parsed ?? []
  } catch {
    return null
  }
}

function extractManifests(body: unknown): PluginManifest[] | null {
  if (!body || typeof body !== 'object') return null
  const raw = body as { data?: unknown; items?: unknown }
  const source = Array.isArray(raw.data)
    ? raw.data
    : Array.isArray((raw.data as { items?: unknown } | undefined)?.items)
      ? (raw.data as { items: unknown[] }).items
      : Array.isArray(raw.items)
        ? raw.items
        : null
  if (!source) return null
  return source
    .map(normalizeManifest)
    .filter((m): m is PluginManifest => m !== null)
}

/**
 * Normalize a manifest entry. The backend Admin plugin API returns the new
 * PluginDTO shape ({ id, state, ... }); earlier mock data used the legacy
 * PluginManifest ({ id, enabled, menus, settings }). We accept both so the
 * frontend keeps working during the rollout.
 */
function normalizeManifest(item: unknown): PluginManifest | null {
  if (!item || typeof item !== 'object') return null
  const record = item as Record<string, unknown>
  if (typeof record.id !== 'string') return null

  // DTO shape — derive `enabled` from `state`.
  if (typeof record.state === 'string') {
    return {
      id: record.id,
      name: typeof record.name === 'string' ? record.name : record.id,
      version: typeof record.version === 'string' ? record.version : undefined,
      enabled: record.state === 'enabled',
      menus: undefined,
      settings: undefined
    }
  }

  // Legacy manifest shape.
  if (typeof record.enabled === 'boolean') {
    return record as unknown as PluginManifest
  }
  return null
}

/**
 * Resolve the plugin manifest: prefer the live API. Fall back to mock only
 * when the API failed (404/network/5xx, i.e. `fetchManifestFromApi` returns
 * null) — a successful-but-empty response is respected as an authoritative
 * "no plugins".
 */
export async function resolveManifests(
  mockFactory?: () => PluginManifest[]
): Promise<PluginManifest[]> {
  const live = await fetchManifestFromApi()
  if (live !== null) {
    manifests.value = live
    return live
  }
  if (mockFactory) {
    const mocked = mockFactory()
    manifests.value = mocked
    return mocked
  }
  manifests.value = []
  return []
}

// ==================== Bootstrap ====================

export interface BootstrapOptions {
  /** Override manifest source (useful for tests). */
  manifestProvider?: () => Promise<PluginManifest[]>
  /** Mock fallback when the API is unavailable. */
  mockFactory?: () => PluginManifest[]
  /** Suppress console warnings on install errors (tests). */
  silent?: boolean
}

/**
 * Install all enabled plugins whose UI module has been registered.
 *
 * Safe to call multiple times. Modules already present in `installed`
 * are skipped.
 */
export async function bootstrapPlugins(
  ctx: PluginContext,
  options: BootstrapOptions = {}
): Promise<string[]> {
  const list = options.manifestProvider
    ? await options.manifestProvider()
    : await resolveManifests(options.mockFactory)

  manifests.value = list
  const justInstalled: string[] = []

  for (const manifest of list) {
    if (!manifest.enabled) continue
    if (installed.value.has(manifest.id)) continue
    const mod = registered.get(manifest.id)
    if (!mod) continue
    try {
      mod.install(ctx)
      installed.value.add(manifest.id)
      justInstalled.push(manifest.id)
    } catch (err) {
      if (!options.silent) {
        // eslint-disable-next-line no-console
        console.error(`[plugins] install failed for ${manifest.id}:`, err)
      }
    }
  }

  return justInstalled
}

// Test-only helper: clear installed + registered + manifest state.
export function __resetRegistryForTests(): void {
  registered.clear()
  installed.value = new Set()
  manifests.value = []
}
