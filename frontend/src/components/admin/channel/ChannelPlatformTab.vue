<template>
  <div class="space-y-4">
    <!-- Groups -->
    <div>
      <label class="input-label text-xs">
        {{ t('admin.channels.form.groups', 'Associated Groups') }} <span class="text-red-500">*</span>
        <span v-if="section.group_ids.length > 0" class="ml-1 font-normal text-gray-400">
          ({{ t('admin.channels.form.selectedCount', { count: section.group_ids.length }, `已选 ${section.group_ids.length} 个`) }})
        </span>
      </label>
      <div class="max-h-40 overflow-auto rounded-lg border border-gray-200 bg-gray-50 p-2 dark:border-dark-600 dark:bg-dark-900">
        <div v-if="groupsLoading" class="py-2 text-center text-xs text-gray-500">
          {{ t('common.loading', 'Loading...') }}
        </div>
        <div v-else-if="platformGroups.length === 0" class="py-2 text-center text-xs text-gray-500">
          {{ t('admin.channels.form.noGroupsAvailable', 'No groups available') }}
        </div>
        <div v-else class="flex flex-wrap gap-1">
          <label
            v-for="group in platformGroups"
            :key="group.id"
            class="inline-flex cursor-pointer items-center gap-1.5 rounded-md border border-gray-200 px-2 py-1 text-xs transition-colors hover:bg-gray-50 dark:border-dark-600 dark:hover:bg-dark-700"
            :class="[
              section.group_ids.includes(group.id) ? 'bg-primary-50 border-primary-300 dark:bg-primary-900/20 dark:border-primary-700' : '',
              isGroupInOtherChannel(group.id) ? 'opacity-40' : ''
            ]"
          >
            <input
              type="checkbox"
              :checked="section.group_ids.includes(group.id)"
              :disabled="isGroupInOtherChannel(group.id)"
              class="h-3 w-3 rounded border-gray-300 text-primary-600 focus:ring-primary-500"
              @change="toggleGroup(group.id)"
            />
            <span :class="['font-medium', getPlatformTextColor(group.platform)]">{{ group.name }}</span>
            <span
              :class="['rounded-full px-1 py-0 text-[10px]', getRateBadgeClass(group.platform)]"
            >{{ group.rate_multiplier }}x</span>
            <span class="text-[10px] text-gray-400">{{ group.account_count || 0 }}</span>
            <span
              v-if="isGroupInOtherChannel(group.id)"
              class="text-[10px] text-gray-400"
            >{{ getGroupInOtherChannelLabel(group.id) }}</span>
          </label>
        </div>
      </div>
    </div>

    <!-- Model Mapping -->
    <div>
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelMapping', 'Model Mapping') }}</label>
        <button type="button" @click="addMappingEntry" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('common.add', 'Add') }}
        </button>
      </div>
      <div
        v-if="Object.keys(section.model_mapping).length === 0"
        class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
      >
        {{ t('admin.channels.form.noMappingRules', 'No mapping rules. Click "Add" to create one.') }}
      </div>
      <div v-else class="space-y-1">
        <div
          v-for="(_, srcModel) in section.model_mapping"
          :key="srcModel"
          class="flex items-center gap-2"
        >
          <input
            :value="srcModel"
            type="text"
            class="input flex-1 text-xs"
            :class="getPlatformTextColor(section.platform)"
            :placeholder="t('admin.channels.form.mappingSource', 'Source model')"
            @change="renameMappingKey(srcModel, ($event.target as HTMLInputElement).value)"
          />
          <span class="text-gray-400 text-xs">→</span>
          <input
            :value="section.model_mapping[srcModel]"
            type="text"
            class="input flex-1 text-xs"
            :class="getPlatformTextColor(section.platform)"
            :placeholder="t('admin.channels.form.mappingTarget', 'Target model')"
            @input="updateMappingValue(srcModel, ($event.target as HTMLInputElement).value)"
          />
          <button
            type="button"
            @click="removeMappingEntry(srcModel)"
            class="rounded p-0.5 text-gray-400 hover:text-red-500"
          >
            <Icon name="trash" size="sm" />
          </button>
        </div>
      </div>
    </div>

    <!-- Model Pricing -->
    <div>
      <div class="mb-1 flex items-center justify-between">
        <label class="input-label text-xs mb-0">{{ t('admin.channels.form.modelPricing', 'Model Pricing') }}</label>
        <button type="button" @click="addPricingEntry" class="text-xs text-primary-600 hover:text-primary-700">
          + {{ t('common.add', 'Add') }}
        </button>
      </div>
      <div
        v-if="section.model_pricing.length === 0"
        class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
      >
        {{ t('admin.channels.form.noPricingRules', 'No pricing rules yet. Click "Add" to create one.') }}
      </div>
      <div v-else class="space-y-2">
        <PricingEntryCard
          v-for="(entry, idx) in section.model_pricing"
          :key="idx"
          :entry="entry"
          :platform="section.platform"
          @update="updatePricingEntry(idx, $event)"
          @remove="removePricingEntry(idx)"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { AdminGroup } from '@/types'
import type { Channel } from '@/api/admin/channels'
import type { PlatformSection, PricingFormEntry } from './types'
import { getPlatformTextColor, getRateBadgeClass } from './types'
import Icon from '@/components/icons/Icon.vue'
import PricingEntryCard from './PricingEntryCard.vue'

const { t } = useI18n()

const props = defineProps<{
  platformGroups: AdminGroup[]
  groupsLoading: boolean
  groupToChannelMap: Map<number, Channel>
}>()

const section = defineModel<PlatformSection>('section', { required: true })

// ── Group helpers ──
function isGroupInOtherChannel(groupId: number): boolean {
  return props.groupToChannelMap.has(groupId)
}

function getGroupChannelName(groupId: number): string {
  return props.groupToChannelMap.get(groupId)?.name || ''
}

function getGroupInOtherChannelLabel(groupId: number): string {
  const name = getGroupChannelName(groupId)
  return t('admin.channels.form.inOtherChannel', { name }, `In "${name}"`)
}

function toggleGroup(groupId: number) {
  const idx = section.value.group_ids.indexOf(groupId)
  if (idx >= 0) {
    section.value.group_ids.splice(idx, 1)
  } else {
    section.value.group_ids.push(groupId)
  }
}

// ── Mapping helpers ──
function addMappingEntry() {
  const mapping = section.value.model_mapping
  let key = ''
  let i = 1
  while (key === '' || key in mapping) {
    key = `model-${i}`
    i++
  }
  mapping[key] = ''
}

function removeMappingEntry(key: string) {
  delete section.value.model_mapping[key]
}

function renameMappingKey(oldKey: string, newKey: string) {
  newKey = newKey.trim()
  if (!newKey || newKey === oldKey) return
  const mapping = section.value.model_mapping
  if (newKey in mapping) return
  const value = mapping[oldKey]
  delete mapping[oldKey]
  mapping[newKey] = value
}

function updateMappingValue(key: string, value: string) {
  section.value.model_mapping[key] = value
}

// ── Pricing helpers ──
function addPricingEntry() {
  section.value.model_pricing.push({
    models: [],
    billing_mode: 'token',
    input_price: null,
    output_price: null,
    cache_write_price: null,
    cache_read_price: null,
    image_output_price: null,
    per_request_price: null,
    intervals: []
  })
}

function updatePricingEntry(idx: number, updated: PricingFormEntry) {
  section.value.model_pricing.splice(idx, 1, updated)
}

function removePricingEntry(idx: number) {
  section.value.model_pricing.splice(idx, 1)
}
</script>
