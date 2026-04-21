/**
 * Helpers for the param-override editor UI.
 *
 * Value is stored as `unknown` in ChannelParamOverrideRule because the
 * backend accepts arbitrary JSON. In the UI the user edits a textual
 * representation, so we always round-trip through JSON.parse/stringify.
 */

import type { ChannelParamOverrideRule } from '@/api/admin/channels'
import {
  ACTION_APPEND,
  ACTION_MERGE,
  ACTION_REMOVE,
  ACTION_SET,
  RESERVED_BODY_PATHS,
  TARGET_BODY,
  TARGET_HEADER,
} from './paramOverrideConstants'

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

/**
 * Signature of vue-i18n's `t` narrowed to the lookups this helper performs.
 * Exported so ParamOverrideEntryCard can pass its own `t` in without the
 * full vue-i18n overloaded type noise.
 */
export type ParamOverrideWarningTranslator = (key: string, named?: Record<string, unknown>) => string

/**
 * Set of inline warnings shown on a single rule card. Each field is either
 * the translated message string or null when the corresponding condition
 * does not apply. Kept flat (rather than a single union) so the template can
 * v-if each warning independently without computing a tag type.
 */
export interface RuleWarnings {
  reservedPathError: string | null
  mergeHeaderWarning: string | null
  appendBodyWarning: string | null
  nullValueWarning: string | null
}

/**
 * Compute all four static-shape warnings a ParamOverrideEntryCard displays
 * inline. Pure function so the component body stays under the 220-line
 * soft cap and the validation logic can be unit-tested without mounting Vue.
 */
export function computeRuleWarnings(
  rule: ChannelParamOverrideRule,
  t: ParamOverrideWarningTranslator,
): RuleWarnings {
  const reservedPathError =
    rule.target === TARGET_BODY && (RESERVED_BODY_PATHS as readonly string[]).includes(rule.path)
      ? t('admin.channels.form.paramOverride.reservedPath', { path: rule.path })
      : null

  const mergeHeaderWarning =
    rule.action === ACTION_MERGE && rule.target === TARGET_HEADER
      ? t('admin.channels.form.paramOverride.mergeHeaderNotSupported')
      : null

  const appendBodyWarning =
    rule.action === ACTION_APPEND && rule.target === TARGET_BODY
      ? t('admin.channels.form.paramOverride.appendBodyNotSupported')
      : null

  const nullValueWarning =
    rule.action !== ACTION_REMOVE && rule.value === null
      ? t('admin.channels.form.paramOverride.valueNullUseRemove')
      : null

  return { reservedPathError, mergeHeaderWarning, appendBodyWarning, nullValueWarning }
}
