<template>
  <div>
    <label class="input-label">
      {{ t('admin.users.groups') }}
      <span class="font-normal text-gray-400">{{ t('common.selectedCount', { count: modelValue.length }) }}</span>
    </label>
    <div
      v-if="isSearchable"
      class="flex items-center gap-2 rounded-t-lg border border-b-0 border-gray-200 bg-gray-50 px-3 py-2 dark:border-dark-600 dark:bg-dark-800"
    >
      <Icon name="search" size="sm" class="shrink-0 text-gray-400" />
      <input
        v-model="searchText"
        type="text"
        :placeholder="t('common.searchPlaceholder')"
        class="flex-1 bg-transparent text-sm text-gray-900 placeholder:text-gray-400 focus:outline-none dark:text-gray-100 dark:placeholder:text-dark-400"
      />
    </div>
    <div
      :class="[
        'max-h-48 overflow-y-auto p-2',
        isSearchable
          ? 'rounded-b-lg border border-t-0 border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
          : 'rounded-lg border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'
      ]"
    >
      <template v-if="hasProtocolSections">
        <div v-for="section in protocolSections" :key="section.protocolId ?? 'ungrouped'" class="mb-2 last:mb-0">
          <div class="mb-1 flex items-center gap-1.5 px-1 text-xs font-medium text-gray-500 dark:text-gray-400">
            <span
              v-if="section.themeColor"
              class="inline-block h-2 w-2 rounded-full"
              :style="{ backgroundColor: section.themeColor }"
            />
            {{ section.label }}
          </div>
          <div class="grid grid-cols-2 gap-1">
            <GroupCheckboxItem
              v-for="group in section.groups"
              :key="group.id"
              :group="group"
              :checked="modelValue.includes(group.id)"
              @toggle="handleChange(group.id, $event)"
            />
          </div>
        </div>
      </template>
      <div v-else class="grid grid-cols-2 gap-1">
        <GroupCheckboxItem
          v-for="group in filteredGroups"
          :key="group.id"
          :group="group"
          :checked="modelValue.includes(group.id)"
          @toggle="handleChange(group.id, $event)"
        />
      </div>
      <div
        v-if="filteredGroups.length === 0"
        class="col-span-2 py-2 text-center text-sm text-gray-500 dark:text-gray-400"
      >
        {{ t('common.noGroupsAvailable') }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import GroupCheckboxItem from './GroupCheckboxItem.vue'
import Icon from '@/components/icons/Icon.vue'
import type { AdminGroup, GroupPlatform, Protocol } from '@/types'

const { t } = useI18n()

interface ProtocolSection {
  protocolId: number | null
  label: string
  themeColor: string | null
  groups: AdminGroup[]
}

interface Props {
  modelValue: number[]
  groups: AdminGroup[]
  platform?: GroupPlatform
  protocols?: Protocol[]
  mixedScheduling?: boolean
  searchable?: boolean | 'auto'
}

const props = withDefaults(defineProps<Props>(), {
  searchable: 'auto'
})
const emit = defineEmits<{
  'update:modelValue': [value: number[]]
}>()

const searchText = ref('')

const isSearchable = computed(() => {
  if (props.searchable === 'auto') return props.groups.length > 5
  return props.searchable
})

const filteredGroups = computed(() => {
  let result: AdminGroup[] = props.groups
  if (props.platform) {
    if (props.platform === 'antigravity' && props.mixedScheduling) {
      result = result.filter(
        (g) => g.platform === 'antigravity' || g.platform === 'anthropic' || g.platform === 'gemini'
      )
    } else {
      result = result.filter((g) => g.platform === props.platform)
    }
  }
  if (isSearchable.value && searchText.value) {
    const q = searchText.value.toLowerCase()
    result = result.filter(
      (g) => g.name.toLowerCase().includes(q) || g.description?.toLowerCase().includes(q)
    )
  }
  return result
})

const hasProtocolSections = computed(() => {
  return props.protocols && props.protocols.length > 0
})

const protocolSections = computed<ProtocolSection[]>(() => {
  if (!props.protocols || props.protocols.length === 0) return []
  const groups = filteredGroups.value
  const sorted = [...props.protocols].sort((a, b) => a.sort_order - b.sort_order)
  const sections: ProtocolSection[] = []
  for (const proto of sorted) {
    const matched = groups.filter((g) => g.protocol_id === proto.id)
    if (matched.length > 0) {
      sections.push({
        protocolId: proto.id,
        label: proto.display_name,
        themeColor: proto.theme_color || null,
        groups: matched
      })
    }
  }
  const unmatched = groups.filter(
    (g) => !g.protocol_id || !sorted.some((p) => p.id === g.protocol_id)
  )
  if (unmatched.length > 0) {
    sections.push({
      protocolId: null,
      label: t('common.other'),
      themeColor: null,
      groups: unmatched
    })
  }
  return sections
})

const handleChange = (groupId: number, checked: boolean) => {
  const newValue = checked
    ? [...props.modelValue, groupId]
    : props.modelValue.filter((id) => id !== groupId)
  emit('update:modelValue', newValue)
}
</script>
