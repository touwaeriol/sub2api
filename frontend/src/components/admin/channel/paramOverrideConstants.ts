/**
 * Frontend constants for channel parameter override UI.
 *
 * Action / target string values are re-exported from `@/api/admin/channels`
 * so the UI never hard-codes `'body'` / `'set'` etc. The presets below are
 * purely UX hints (auto-complete lists); they are never sent to the backend.
 */

import {
  PARAM_OVERRIDE_ACTIONS,
  PARAM_OVERRIDE_TARGETS,
  type ParamOverrideAction,
  type ParamOverrideTarget,
} from '@/api/admin/channels'

export { PARAM_OVERRIDE_ACTIONS, PARAM_OVERRIDE_TARGETS }
export type { ParamOverrideAction, ParamOverrideTarget }

/** 平台 body 路径预设，仅作为 datalist 提示（用户可自行输入） */
export const BODY_PATH_PRESETS: Record<string, readonly string[]> = {
  anthropic: ['thinking.type', 'thinking.budget_tokens'],
  openai: ['reasoning.effort', 'reasoning.summary'],
  gemini: [],
  antigravity: ['thinking.type', 'thinking.budget_tokens'],
}

/** 平台 header key 预设，仅作为 datalist 提示 */
export const HEADER_KEY_PRESETS: Record<string, readonly string[]> = {
  anthropic: ['anthropic-beta'],
  openai: ['openai-beta'],
  gemini: [],
  antigravity: ['x-goog-user-project'],
}

/**
 * Body 路径保留字段：禁止覆盖，否则会导致计费错乱。
 * 与后端 channel_handler.go 的 path_model_reserved 校验保持一致。
 */
export const RESERVED_BODY_PATHS = ['model'] as const
export type ReservedBodyPath = (typeof RESERVED_BODY_PATHS)[number]
