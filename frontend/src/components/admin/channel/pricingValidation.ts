/**
 * Pricing-form validation: model-pattern conflict detection and per-interval
 * sanity checks. Validation functions accept a vue-i18n-style translator so
 * every user-facing message is localised (no hardcoded strings here).
 */

import type { IntervalFormEntry } from './channelFormTypes'

/**
 * Vue-i18n-style translator narrowed to the calls made by this module.
 *   t(key)                    — plain lookup
 *   t(key, { idx, value })    — named interpolation (see admin.channels.pricing.interval.*)
 */
export type PricingValidationTranslator = (key: string, named?: Record<string, unknown>) => string

// ── 模型模式冲突检测 ──────────────────────────────────────

interface ModelPattern {
  pattern: string
  prefix: string  // lowercase, 通配符去掉尾部 *
  wildcard: boolean
}

function toModelPattern(model: string): ModelPattern {
  const lower = model.toLowerCase()
  const wildcard = lower.endsWith('*')
  return {
    pattern: model,
    prefix: wildcard ? lower.slice(0, -1) : lower,
    wildcard,
  }
}

function patternsConflict(a: ModelPattern, b: ModelPattern): boolean {
  if (!a.wildcard && !b.wildcard) return a.prefix === b.prefix
  if (a.wildcard && !b.wildcard) return b.prefix.startsWith(a.prefix)
  if (!a.wildcard && b.wildcard) return a.prefix.startsWith(b.prefix)
  // 双通配符：任一前缀是另一前缀的前缀即冲突
  return a.prefix.startsWith(b.prefix) || b.prefix.startsWith(a.prefix)
}

/** 检测模型模式列表中的冲突，返回冲突的两个模式名；无冲突返回 null */
export function findModelConflict(models: string[]): [string, string] | null {
  const patterns = models.map(toModelPattern)
  for (let i = 0; i < patterns.length; i++) {
    for (let j = i + 1; j < patterns.length; j++) {
      if (patternsConflict(patterns[i], patterns[j])) {
        return [patterns[i].pattern, patterns[j].pattern]
      }
    }
  }
  return null
}

// ── 区间校验 ──────────────────────────────────────────────

/** 校验区间列表的合法性，返回错误消息；通过则返回 null */
export function validateIntervals(
  intervals: IntervalFormEntry[],
  t: PricingValidationTranslator,
): string | null {
  if (!intervals || intervals.length === 0) return null

  // 按 min_tokens 排序（不修改原数组）
  const sorted = [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)

  for (let i = 0; i < sorted.length; i++) {
    const err = validateSingleInterval(sorted[i], i, t)
    if (err) return err
  }
  return checkIntervalOverlap(sorted, t)
}

function validateSingleInterval(
  iv: IntervalFormEntry,
  idx: number,
  t: PricingValidationTranslator,
): string | null {
  const n = idx + 1
  if (iv.min_tokens < 0) {
    return t('admin.channels.pricing.interval.minNegative', { idx: n, value: iv.min_tokens })
  }
  if (iv.max_tokens != null) {
    if (iv.max_tokens <= 0) {
      return t('admin.channels.pricing.interval.maxNotPositive', { idx: n, value: iv.max_tokens })
    }
    if (iv.max_tokens <= iv.min_tokens) {
      return t('admin.channels.pricing.interval.maxNotGreaterThanMin', {
        idx: n,
        max: iv.max_tokens,
        min: iv.min_tokens,
      })
    }
  }
  return validateIntervalPrices(iv, idx, t)
}

function validateIntervalPrices(
  iv: IntervalFormEntry,
  idx: number,
  t: PricingValidationTranslator,
): string | null {
  const n = idx + 1
  const prices: [string, number | string | null][] = [
    ['admin.channels.pricing.interval.priceInput', iv.input_price],
    ['admin.channels.pricing.interval.priceOutput', iv.output_price],
    ['admin.channels.pricing.interval.priceCacheWrite', iv.cache_write_price],
    ['admin.channels.pricing.interval.priceCacheRead', iv.cache_read_price],
    ['admin.channels.pricing.interval.pricePerRequest', iv.per_request_price],
  ]
  for (const [labelKey, val] of prices) {
    if (val != null && val !== '' && Number(val) < 0) {
      return t('admin.channels.pricing.interval.priceNegative', { idx: n, label: t(labelKey) })
    }
  }
  return null
}

function checkIntervalOverlap(
  sorted: IntervalFormEntry[],
  t: PricingValidationTranslator,
): string | null {
  for (let i = 0; i < sorted.length; i++) {
    // 无上限区间必须是最后一个
    if (sorted[i].max_tokens == null && i < sorted.length - 1) {
      return t('admin.channels.pricing.interval.unboundedMustBeLast', { idx: i + 1 })
    }
    if (i === 0) continue
    const prev = sorted[i - 1]
    // (min, max] 语义：前一个区间上界 > 当前区间下界则重叠
    if (prev.max_tokens == null || prev.max_tokens > sorted[i].min_tokens) {
      const prevMax = prev.max_tokens == null
        ? t('admin.channels.pricing.interval.unbounded')
        : String(prev.max_tokens)
      return t('admin.channels.pricing.interval.overlap', {
        prevIdx: i,
        currIdx: i + 1,
        prevMax,
        currMin: sorted[i].min_tokens,
      })
    }
  }
  return null
}
