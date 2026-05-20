<template>
  <label
    class="flex cursor-pointer items-center gap-2 rounded px-2 py-1.5 transition-colors hover:bg-white dark:hover:bg-dark-700"
    :title="t('admin.groups.rateAndAccounts', { rate: group.rate_multiplier, count: group.account_count || 0 })"
  >
    <input
      type="checkbox"
      :value="group.id"
      :checked="checked"
      @change="emit('toggle', ($event.target as HTMLInputElement).checked)"
      class="h-3.5 w-3.5 shrink-0 rounded border-gray-300 text-primary-500 focus:ring-primary-500 dark:border-dark-500"
    />
    <GroupBadge
      :name="group.name"
      :platform="group.platform"
      :subscription-type="group.subscription_type"
      :rate-multiplier="group.rate_multiplier"
      class="min-w-0 flex-1"
    />
    <span class="shrink-0 text-xs text-gray-400">{{ group.account_count || 0 }}</span>
  </label>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import GroupBadge from './GroupBadge.vue'
import type { AdminGroup } from '@/types'

const { t } = useI18n()

defineProps<{
  group: AdminGroup
  checked: boolean
}>()

const emit = defineEmits<{
  toggle: [checked: boolean]
}>()
</script>
