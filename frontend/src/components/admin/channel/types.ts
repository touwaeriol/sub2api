import type {
  BillingMode,
  Channel,
  ChannelModelPricing,
  ChannelParamOverrideRule,
  ChannelParamOverrides,
  PricingInterval,
} from '@/api/admin/channels'
import type { AdminGroup, GroupPlatform } from '@/types'

/** 平台排序顺序（界面显示 + 转换时遍历） */
export const PLATFORM_ORDER: GroupPlatform[] = ['anthropic', 'openai', 'gemini', 'antigravity']

export interface PlatformSection {
  platform: GroupPlatform
  enabled: boolean
  collapsed: boolean
  group_ids: number[]
  model_mapping: Record<string, string>
  model_pricing: PricingFormEntry[]
  param_overrides: ChannelParamOverrideRule[]
}

export interface IntervalFormEntry {
  min_tokens: number
  max_tokens: number | null
  tier_label: string
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  per_request_price: number | string | null
  sort_order: number
}

export interface PricingFormEntry {
  models: string[]
  billing_mode: BillingMode
  input_price: number | string | null
  output_price: number | string | null
  cache_write_price: number | string | null
  cache_read_price: number | string | null
  image_output_price: number | string | null
  per_request_price: number | string | null
  intervals: IntervalFormEntry[]
}

// 价格转换：后端存 per-token，前端显示 per-MTok ($/1M tokens)
const MTOK = 1_000_000

export function toNullableNumber(val: number | string | null | undefined): number | null {
  if (val === null || val === undefined || val === '') return null
  const num = Number(val)
  return isNaN(num) ? null : num
}

/** 前端显示值($/MTok) → 后端存储值(per-token) */
export function mTokToPerToken(val: number | string | null | undefined): number | null {
  const num = toNullableNumber(val)
  return num === null ? null : parseFloat((num / MTOK).toPrecision(10))
}

/** 后端存储值(per-token) → 前端显示值($/MTok) */
export function perTokenToMTok(val: number | null | undefined): number | null {
  if (val === null || val === undefined) return null
  // toPrecision(10) 消除 IEEE 754 浮点乘法精度误差，如 5e-8 * 1e6 = 0.04999...96 → 0.05
  return parseFloat((val * MTOK).toPrecision(10))
}

export function apiIntervalsToForm(intervals: PricingInterval[]): IntervalFormEntry[] {
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label || '',
    input_price: perTokenToMTok(iv.input_price),
    output_price: perTokenToMTok(iv.output_price),
    cache_write_price: perTokenToMTok(iv.cache_write_price),
    cache_read_price: perTokenToMTok(iv.cache_read_price),
    per_request_price: iv.per_request_price,
    sort_order: iv.sort_order
  }))
}

export function formIntervalsToAPI(intervals: IntervalFormEntry[]): PricingInterval[] {
  return (intervals || []).map(iv => ({
    min_tokens: iv.min_tokens,
    max_tokens: iv.max_tokens,
    tier_label: iv.tier_label,
    input_price: mTokToPerToken(iv.input_price),
    output_price: mTokToPerToken(iv.output_price),
    cache_write_price: mTokToPerToken(iv.cache_write_price),
    cache_read_price: mTokToPerToken(iv.cache_read_price),
    per_request_price: toNullableNumber(iv.per_request_price),
    sort_order: iv.sort_order
  }))
}

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
export function validateIntervals(intervals: IntervalFormEntry[]): string | null {
  if (!intervals || intervals.length === 0) return null

  // 按 min_tokens 排序（不修改原数组）
  const sorted = [...intervals].sort((a, b) => a.min_tokens - b.min_tokens)

  for (let i = 0; i < sorted.length; i++) {
    const err = validateSingleInterval(sorted[i], i)
    if (err) return err
  }
  return checkIntervalOverlap(sorted)
}

function validateSingleInterval(iv: IntervalFormEntry, idx: number): string | null {
  if (iv.min_tokens < 0) {
    return `区间 #${idx + 1}: 最小 token 数 (${iv.min_tokens}) 不能为负数`
  }
  if (iv.max_tokens != null) {
    if (iv.max_tokens <= 0) {
      return `区间 #${idx + 1}: 最大 token 数 (${iv.max_tokens}) 必须大于 0`
    }
    if (iv.max_tokens <= iv.min_tokens) {
      return `区间 #${idx + 1}: 最大 token 数 (${iv.max_tokens}) 必须大于最小 token 数 (${iv.min_tokens})`
    }
  }
  return validateIntervalPrices(iv, idx)
}

function validateIntervalPrices(iv: IntervalFormEntry, idx: number): string | null {
  const prices: [string, number | string | null][] = [
    ['输入价格', iv.input_price],
    ['输出价格', iv.output_price],
    ['缓存写入价格', iv.cache_write_price],
    ['缓存读取价格', iv.cache_read_price],
    ['单次价格', iv.per_request_price],
  ]
  for (const [name, val] of prices) {
    if (val != null && val !== '' && Number(val) < 0) {
      return `区间 #${idx + 1}: ${name}不能为负数`
    }
  }
  return null
}

function checkIntervalOverlap(sorted: IntervalFormEntry[]): string | null {
  for (let i = 0; i < sorted.length; i++) {
    // 无上限区间必须是最后一个
    if (sorted[i].max_tokens == null && i < sorted.length - 1) {
      return `区间 #${i + 1}: 无上限区间（最大 token 数为空）只能是最后一个`
    }
    if (i === 0) continue
    const prev = sorted[i - 1]
    // (min, max] 语义：前一个区间上界 > 当前区间下界则重叠
    if (prev.max_tokens == null || prev.max_tokens > sorted[i].min_tokens) {
      const prevMax = prev.max_tokens == null ? '∞' : String(prev.max_tokens)
      return `区间 #${i} 和 #${i + 1} 重叠：前一个区间上界 (${prevMax}) 大于当前区间下界 (${sorted[i].min_tokens})`
    }
  }
  return null
}

/** 平台对应的模型 tag 样式（背景+文字） */
export function getPlatformTagClass(platform: string): string {
  switch (platform) {
    case 'anthropic': return 'bg-orange-100 text-orange-700 dark:bg-orange-900/30 dark:text-orange-400'
    case 'openai': return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-900/30 dark:text-emerald-400'
    case 'gemini': return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400'
    case 'antigravity': return 'bg-purple-100 text-purple-700 dark:bg-purple-900/30 dark:text-purple-400'
    default: return 'bg-gray-100 text-gray-700 dark:bg-gray-900/30 dark:text-gray-400'
  }
}

// ── 表单 ↔ API 转换 ──────────────────────────────────────

export interface PlatformSectionsAPIPayload {
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>>
  param_overrides: ChannelParamOverrides
}

/** 将启用的 PlatformSection 聚合为后端请求体 */
export function platformSectionsToAPI(sections: PlatformSection[]): PlatformSectionsAPIPayload {
  const group_ids: number[] = []
  const model_pricing: ChannelModelPricing[] = []
  const model_mapping: Record<string, Record<string, string>> = {}
  const param_overrides: ChannelParamOverrides = {}

  for (const section of sections) {
    if (!section.enabled) continue
    group_ids.push(...section.group_ids)

    if (Object.keys(section.model_mapping).length > 0) {
      model_mapping[section.platform] = { ...section.model_mapping }
    }

    if (section.param_overrides.length > 0) {
      param_overrides[section.platform] = section.param_overrides.map(r => ({ ...r }))
    }

    for (const entry of section.model_pricing) {
      if (entry.models.length === 0) continue
      model_pricing.push({
        platform: section.platform,
        models: entry.models,
        billing_mode: entry.billing_mode,
        input_price: mTokToPerToken(entry.input_price),
        output_price: mTokToPerToken(entry.output_price),
        cache_write_price: mTokToPerToken(entry.cache_write_price),
        cache_read_price: mTokToPerToken(entry.cache_read_price),
        image_output_price: mTokToPerToken(entry.image_output_price),
        per_request_price: entry.per_request_price != null && entry.per_request_price !== '' ? Number(entry.per_request_price) : null,
        intervals: formIntervalsToAPI(entry.intervals || [])
      })
    }
  }

  return { group_ids, model_pricing, model_mapping, param_overrides }
}

/** 将后端返回的 Channel 转换为 PlatformSection 列表（用于编辑表单） */
export function channelToPlatformSections(channel: Channel, allGroups: AdminGroup[]): PlatformSection[] {
  const groupPlatformMap = new Map<number, GroupPlatform>()
  for (const g of allGroups) {
    groupPlatformMap.set(g.id, g.platform)
  }

  const activePlatforms = new Set<GroupPlatform>()
  for (const gid of channel.group_ids || []) {
    const p = groupPlatformMap.get(gid)
    if (p) activePlatforms.add(p)
  }
  for (const p of channel.model_pricing || []) {
    if (p.platform) activePlatforms.add(p.platform as GroupPlatform)
  }
  for (const p of Object.keys(channel.model_mapping || {})) {
    if (PLATFORM_ORDER.includes(p as GroupPlatform)) activePlatforms.add(p as GroupPlatform)
  }
  for (const p of Object.keys(channel.param_overrides || {})) {
    if (PLATFORM_ORDER.includes(p as GroupPlatform)) activePlatforms.add(p as GroupPlatform)
  }

  const sections: PlatformSection[] = []
  for (const platform of PLATFORM_ORDER) {
    if (!activePlatforms.has(platform)) continue

    const groupIds = (channel.group_ids || []).filter(gid => groupPlatformMap.get(gid) === platform)
    const mapping = (channel.model_mapping || {})[platform] || {}
    const overrides = (channel.param_overrides || {})[platform] || []
    const pricing = (channel.model_pricing || [])
      .filter(p => (p.platform || 'anthropic') === platform)
      .map(p => ({
        models: p.models || [],
        billing_mode: p.billing_mode,
        input_price: perTokenToMTok(p.input_price),
        output_price: perTokenToMTok(p.output_price),
        cache_write_price: perTokenToMTok(p.cache_write_price),
        cache_read_price: perTokenToMTok(p.cache_read_price),
        image_output_price: perTokenToMTok(p.image_output_price),
        per_request_price: p.per_request_price,
        intervals: apiIntervalsToForm(p.intervals || [])
      } as PricingFormEntry))

    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids: groupIds,
      model_mapping: { ...mapping },
      model_pricing: pricing,
      param_overrides: overrides.map(r => ({ ...r }))
    })
  }

  return sections
}
