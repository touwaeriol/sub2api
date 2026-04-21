/**
 * Centralized API error message extraction
 *
 * The API client interceptor rejects with a plain object: { status, code, message, error }
 * This utility extracts the user-facing message from any error shape.
 */

interface ApiErrorLike {
  status?: number
  code?: number | string
  message?: string
  error?: string
  reason?: string
  metadata?: Record<string, unknown>
  response?: {
    data?: {
      detail?: string
      message?: string
      code?: number | string
    }
  }
}

/**
 * Extract the error code from an API error object.
 */
export function extractApiErrorCode(err: unknown): string | undefined {
  if (!err || typeof err !== 'object') return undefined
  const e = err as ApiErrorLike
  const code = e.code ?? e.reason ?? e.response?.data?.code
  return code != null ? String(code) : undefined
}

/**
 * Extract the structured `reason` tag from an API error's metadata. The
 * backend attaches a short, stable identifier (e.g. `value_null_use_remove`,
 * `compile_failed`) so the frontend can render a localized message instead
 * of the raw English debug string. Returns undefined when the error carries
 * no reason tag.
 */
export function extractApiErrorReason(err: unknown): string | undefined {
  if (!err || typeof err !== 'object') return undefined
  const e = err as ApiErrorLike
  const direct = e.reason
  if (typeof direct === 'string' && direct !== '') return direct
  const meta = e.metadata
  if (meta && typeof meta === 'object') {
    const r = (meta as Record<string, unknown>).reason
    if (typeof r === 'string' && r !== '') return r
  }
  return undefined
}

/**
 * Translator signature narrowed to the calls `extractApiErrorMessage` makes
 * when resolving structured reason codes. Matches vue-i18n's `t`.
 */
export type ApiErrorTranslator = (key: string, named?: Record<string, unknown>) => string

/**
 * Options for resolving a structured error reason into a localized message.
 * When both are set, `extractApiErrorMessage` prefers the reason-based lookup
 * over the raw backend `message`, falling back only if the i18n key is missing.
 */
export interface ApiErrorReasonI18n {
  /** e.g. 'admin.channels.form.paramOverride.reasons.' */
  prefix: string
  /** The vue-i18n `t` function. */
  t: ApiErrorTranslator
  /** Optional fallback key appended to prefix when the reason is unknown. */
  unknownKey?: string
}

/** Type guard distinguishing the reason-i18n shape from the code-map shape. */
function isReasonI18n(
  value: Record<string, string> | ApiErrorReasonI18n | undefined,
): value is ApiErrorReasonI18n {
  return (
    value !== undefined &&
    typeof (value as ApiErrorReasonI18n).prefix === 'string' &&
    typeof (value as ApiErrorReasonI18n).t === 'function'
  )
}

/**
 * Extract a displayable error message from an API error.
 *
 * @param err - The caught error (unknown type)
 * @param fallback - Fallback message if none can be extracted (use t('common.error') or similar)
 * @param i18nMap - Optional map of error codes to i18n translated strings
 */
export function extractApiErrorMessage(
  err: unknown,
  fallback = 'Unknown error',
  i18nMap?: Record<string, string> | ApiErrorReasonI18n,
): string {
  if (!err) return fallback

  // Structured reason lookup: when the caller passes an {prefix, t} pair,
  // prefer the localized message derived from metadata.reason over the raw
  // backend debug string. This is the contract for backend handlers that
  // emit reason codes (e.g. paramOverrideReasonXxx).
  if (isReasonI18n(i18nMap)) {
    const reason = extractApiErrorReason(err)
    if (reason) {
      const key = i18nMap.prefix + reason
      const translated = i18nMap.t(key, { reason })
      // vue-i18n returns the key itself on miss; fall through to the raw
      // message only when the translation is absent.
      if (translated && translated !== key) return translated
    }
    if (i18nMap.unknownKey) {
      const unknownKey = i18nMap.prefix + i18nMap.unknownKey
      const unknown = i18nMap.t(unknownKey)
      if (unknown && unknown !== unknownKey) return unknown
    }
  } else if (i18nMap) {
    // Backwards-compatible code-based map lookup.
    const code = extractApiErrorCode(err)
    if (code && i18nMap[code]) return i18nMap[code]
  }

  // Plain object from API client interceptor (most common case)
  if (typeof err === 'object' && err !== null) {
    const e = err as ApiErrorLike
    // Interceptor shape: { message, error }
    if (e.message) return e.message
    if (e.error) return e.error
    // Legacy axios shape: { response.data.detail }
    if (e.response?.data?.detail) return e.response.data.detail
    if (e.response?.data?.message) return e.response.data.message
  }

  // Standard Error
  if (err instanceof Error) return err.message

  // Last resort
  const str = String(err)
  return str === '[object Object]' ? fallback : str
}
