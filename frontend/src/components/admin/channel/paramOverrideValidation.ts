/**
 * Pre-submit validation for the channel-form's parameter-override rules.
 * Mirrors the backend validateParamOverrideRule so the admin UI surfaces
 * the same rejections without a server round-trip. Uses the shared
 * `classifyRuleIssue` single source of truth from paramOverrideHelpers.
 */

import type { ChannelParamOverrideRule } from '@/api/admin/channels'
import type { GroupPlatform } from '@/types'
import type { PlatformSection } from './channelFormTypes'
import { classifyRuleIssue, type ParamOverrideIssue } from './paramOverrideHelpers'

// Re-exported so callers outside this module keep the historical import
// path stable.
export { classifyRuleIssue as paramOverrideRuleIssue } from './paramOverrideHelpers'
export type { ParamOverrideIssue } from './paramOverrideHelpers'

export interface ParamOverrideValidationError {
  platform: GroupPlatform
  message: string
}

/**
 * Simplified translator shape matching the subset of vue-i18n we use here:
 *   t(key)                    — plain lookup
 *   t(key, { foo: 'bar' })    — named interpolation
 * Fallbacks are handled by i18n config, not this signature.
 */
export type ParamOverrideTranslator = (key: string, named?: Record<string, unknown>) => string

/**
 * 在提交前校验所有启用平台的参数覆盖规则。复用与后端
 * validateParamOverrideRule 相同的判定条件（保留字段、merge+header、
 * append+body、value 必填），规则无效时返回错误及对应平台；否则返回 null。
 */
export function validateParamOverrideSections(
  sections: PlatformSection[],
  t: ParamOverrideTranslator,
): ParamOverrideValidationError | null {
  for (const section of sections) {
    if (!section.enabled) continue
    for (let idx = 0; idx < section.param_overrides.length; idx++) {
      const rule = section.param_overrides[idx]
      const reason = classifyRuleIssue(rule)
      if (!reason) continue
      const platformLabel = t('admin.groups.platforms.' + section.platform)
      const message = formatParamOverrideIssue(platformLabel, idx, rule, reason, t)
      return { platform: section.platform, message }
    }
  }
  return null
}

function formatParamOverrideIssue(
  platformLabel: string,
  idx: number,
  rule: ChannelParamOverrideRule,
  reason: ParamOverrideIssue,
  t: ParamOverrideTranslator,
): string {
  const prefix = `${platformLabel} - #${idx + 1}`
  switch (reason) {
    case 'path_required':
      return `${prefix}: ${t('admin.channels.form.paramOverride.path')}`
    case 'reserved_path':
      return `${prefix}: ${t('admin.channels.form.paramOverride.reservedPath', { path: rule.path })}`
    case 'merge_not_supported_for_header':
      return `${prefix}: ${t('admin.channels.form.paramOverride.mergeHeaderNotSupported')}`
    case 'append_requires_header_target':
      return `${prefix}: ${t('admin.channels.form.paramOverride.appendBodyNotSupported')}`
    case 'value_required':
      return `${prefix}: ${t('admin.channels.form.paramOverride.value')}`
    case 'value_null_use_remove':
      return `${prefix}: ${t('admin.channels.form.paramOverride.valueNullUseRemove')}`
  }
}
