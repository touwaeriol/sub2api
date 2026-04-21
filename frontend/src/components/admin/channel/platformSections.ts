/**
 * Conversion between the server-side Channel payload and the in-memory
 * PlatformSection[] the form edits. Every platform block (groups / pricing /
 * model-mapping / param-overrides) is aggregated/split here so the form
 * components only deal with one platform at a time.
 */

import type {
  Channel,
  ChannelModelPricing,
  ChannelParamOverrideRule,
  ChannelParamOverrides,
} from '@/api/admin/channels'
import type { AdminGroup, GroupPlatform } from '@/types'
import type { PlatformSection, PricingFormEntry } from './channelFormTypes'
import { PLATFORM_ORDER } from './channelFormTypes'
import { newClientId } from './paramOverrideHelpers'
import {
  apiIntervalsToForm,
  formIntervalsToAPI,
  mTokToPerToken,
  perTokenToMTok,
} from './pricingConversion'

export interface PlatformSectionsAPIPayload {
  group_ids: number[]
  model_pricing: ChannelModelPricing[]
  model_mapping: Record<string, Record<string, string>>
  param_overrides: ChannelParamOverrides
}

/**
 * Return a shallow copy of the rule with the frontend-only `_clientId`
 * field removed, so the payload is safe to send to the backend (which
 * doesn't know about the field and treats unknown JSON fields as a
 * validation problem).
 */
function stripClientId(rule: ChannelParamOverrideRule): ChannelParamOverrideRule {
  const copy = { ...rule }
  delete copy._clientId
  return copy
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
      // Strip the frontend-only `_clientId` from every rule before it hits
      // the API — the backend rejects unknown fields and, even if it
      // didn't, we don't want the identifier to land in the DB.
      param_overrides[section.platform] = section.param_overrides.map(stripClientId)
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

  const activePlatforms = collectActivePlatforms(channel, groupPlatformMap)

  const sections: PlatformSection[] = []
  for (const platform of PLATFORM_ORDER) {
    if (!activePlatforms.has(platform)) continue

    const groupIds = (channel.group_ids || []).filter(gid => groupPlatformMap.get(gid) === platform)
    const mapping = (channel.model_mapping || {})[platform] || {}
    const overrides = (channel.param_overrides || {})[platform] || []
    const pricing = channelPricingToForm(channel, platform)

    sections.push({
      platform,
      enabled: true,
      collapsed: false,
      group_ids: groupIds,
      model_mapping: { ...mapping },
      model_pricing: pricing,
      // Synthesise a fresh _clientId for every rule read from the server,
      // so Vue's :key remains stable across subsequent reorders / edits
      // in this form session. The id is stripped again in
      // platformSectionsToAPI before the next save.
      param_overrides: overrides.map(r => ({ ...r, _clientId: newClientId() }))
    })
  }

  return sections
}

/** 汇总 channel 中出现过的平台（group / pricing / mapping / overrides 任意来源）。 */
function collectActivePlatforms(
  channel: Channel,
  groupPlatformMap: Map<number, GroupPlatform>,
): Set<GroupPlatform> {
  const active = new Set<GroupPlatform>()
  for (const gid of channel.group_ids || []) {
    const p = groupPlatformMap.get(gid)
    if (p) active.add(p)
  }
  for (const p of channel.model_pricing || []) {
    if (p.platform) active.add(p.platform as GroupPlatform)
  }
  for (const p of Object.keys(channel.model_mapping || {})) {
    if (PLATFORM_ORDER.includes(p as GroupPlatform)) active.add(p as GroupPlatform)
  }
  for (const p of Object.keys(channel.param_overrides || {})) {
    if (PLATFORM_ORDER.includes(p as GroupPlatform)) active.add(p as GroupPlatform)
  }
  return active
}

/** 将 channel.model_pricing 中属于给定平台的条目转换为表单结构。 */
function channelPricingToForm(channel: Channel, platform: GroupPlatform): PricingFormEntry[] {
  return (channel.model_pricing || [])
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
}
