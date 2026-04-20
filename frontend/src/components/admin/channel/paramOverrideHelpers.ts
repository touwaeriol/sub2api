/**
 * Helpers for the param-override editor UI.
 *
 * Value is stored as `unknown` in ChannelParamOverrideRule because the
 * backend accepts arbitrary JSON. In the UI the user edits a textual
 * representation, so we always round-trip through JSON.parse/stringify.
 */

import type { ChannelParamOverrideRule } from '@/api/admin/channels'
import { ACTION_SET, TARGET_BODY } from './paramOverrideConstants'

export interface ParsedJsonValue {
  value: unknown
  error: string | null
}

/**
 * Parse a textual JSON value the user has typed. Empty input is treated as
 * `null` (consistent with "unset"). Returns a structured result so callers
 * can distinguish parse failures from valid `null`/`false`/`0` values.
 */
export function parseJsonValue(text: string): ParsedJsonValue {
  const trimmed = text.trim()
  if (trimmed === '') {
    return { value: null, error: null }
  }
  try {
    return { value: JSON.parse(trimmed), error: null }
  } catch (err: unknown) {
    return { value: null, error: err instanceof Error ? err.message : 'invalid JSON' }
  }
}

/**
 * Render a value for the editor's textarea. `null`/`undefined` become empty
 * strings so the user sees an empty field (not the literal "null"). All
 * other shapes round-trip through JSON.stringify.
 */
export function stringifyValue(v: unknown): string {
  if (v === null || v === undefined) return ''
  if (typeof v === 'string') return JSON.stringify(v)
  try {
    return JSON.stringify(v)
  } catch {
    return ''
  }
}

/**
 * Factory for a blank rule. Defaults: enabled, `*` glob (match all), body
 * Set action, no path or value — the user fills those in.
 */
export function createEmptyRule(): ChannelParamOverrideRule {
  return {
    enabled: true,
    model_glob: '*',
    target: TARGET_BODY,
    action: ACTION_SET,
    path: '',
    value: null,
    description: '',
  }
}
