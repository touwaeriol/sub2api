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
 * Set action, no path or value — the user fills those in. A fresh
 * `_clientId` is assigned so the rule is usable as a stable Vue :key
 * throughout its edit lifetime.
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
    _clientId: newClientId(),
  }
}

/**
 * Generate a stable identifier for a rule's _clientId field. Uses
 * crypto.randomUUID() when available (all modern browsers + Node 18+) and
 * falls back to a Math.random-derived string for the rare legacy target.
 * The value is opaque to the backend — see the comment on
 * ChannelParamOverrideRule._clientId.
 */
export function newClientId(): string {
  const cryptoObj = typeof globalThis !== 'undefined' ? globalThis.crypto : undefined
  if (cryptoObj && typeof cryptoObj.randomUUID === 'function') {
    return cryptoObj.randomUUID()
  }
  return `po-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}`
}

/**
 * Issue union emitted by `classifyRuleIssue`. Variants mirror the subset of
 * backend `paramOverrideReasonXxx` codes the admin UI can detect without a
 * server round-trip. Kept here alongside the predicates below so adding a
 * new defect only requires updating this file.
 */
export type ParamOverrideIssue = 'path_required' | 'reserved_path' | 'merge_not_supported_for_header' | 'append_requires_header_target' | 'value_required' | 'value_null_use_remove'

// Predicates for the 4 rule-shape defects that both the form-level validator
// and the inline card warnings share. Extracting them is the DRY backbone:
// `classifyRuleIssue` and `computeRuleWarnings` each call the same predicate.

export function isReservedBodyPathIssue(rule: ChannelParamOverrideRule): boolean {
  return (
    rule.target === TARGET_BODY &&
    (RESERVED_BODY_PATHS as readonly string[]).includes(rule.path)
  )
}

export function isMergeOnHeaderIssue(rule: ChannelParamOverrideRule): boolean {
  return rule.action === ACTION_MERGE && rule.target === TARGET_HEADER
}

export function isAppendOnBodyIssue(rule: ChannelParamOverrideRule): boolean {
  return rule.action === ACTION_APPEND && rule.target === TARGET_BODY
}

export function isNullValueNonRemoveIssue(rule: ChannelParamOverrideRule): boolean {
  return rule.action !== ACTION_REMOVE && rule.value === null
}

/**
 * classifyRuleIssue is the single source of truth for "what's the most
 * important defect on this rule, if any?". Used by paramOverrideRuleIssue in
 * types.ts to drive pre-submit validation; the inline card warnings in
 * `computeRuleWarnings` below consume the shared predicates directly (they
 * need to surface multiple issues at once rather than a single top pick).
 *
 * Order reflects user-action priority: errors that prevent saving come
 * first, semantic mismatches next, then the null-value nudge.
 */
export function classifyRuleIssue(rule: ChannelParamOverrideRule): ParamOverrideIssue | null {
  if (!rule.enabled) return null
  if (rule.path.trim() === '') return 'path_required'
  if (isReservedBodyPathIssue(rule)) return 'reserved_path'
  if (isMergeOnHeaderIssue(rule)) return 'merge_not_supported_for_header'
  if (isAppendOnBodyIssue(rule)) return 'append_requires_header_target'
  if (rule.action !== ACTION_REMOVE) {
    if (rule.value === undefined) return 'value_required'
    if (rule.value === null) return 'value_null_use_remove'
  }
  return null
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
  return {
    reservedPathError: isReservedBodyPathIssue(rule)
      ? t('admin.channels.form.paramOverride.reservedPath', { path: rule.path })
      : null,
    mergeHeaderWarning: isMergeOnHeaderIssue(rule)
      ? t('admin.channels.form.paramOverride.mergeHeaderNotSupported')
      : null,
    appendBodyWarning: isAppendOnBodyIssue(rule)
      ? t('admin.channels.form.paramOverride.appendBodyNotSupported')
      : null,
    nullValueWarning: isNullValueNonRemoveIssue(rule)
      ? t('admin.channels.form.paramOverride.valueNullUseRemove')
      : null,
  }
}
