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

/**
 * Named aliases for the individual action / target values. UI components
 * compare rule fields against these instead of bare string literals, so a
 * rename on the wire format only needs to touch the API constants file.
 *
 * Literals are written explicitly (rather than PARAM_OVERRIDE_TARGETS[0]
 * positional indexing) and constrained via `satisfies` so:
 *  (a) a reader doesn't need to count array positions to know which value
 *      each name refers to, and
 *  (b) if the enum ever drifts, TS fails at compile time because the
 *      literal no longer satisfies the corresponding union.
 */
export const TARGET_BODY = 'body' as const satisfies ParamOverrideTarget
export const TARGET_HEADER = 'header' as const satisfies ParamOverrideTarget
export const ACTION_SET = 'set' as const satisfies ParamOverrideAction
export const ACTION_MERGE = 'merge' as const satisfies ParamOverrideAction
export const ACTION_REMOVE = 'remove' as const satisfies ParamOverrideAction
export const ACTION_APPEND = 'append' as const satisfies ParamOverrideAction

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
