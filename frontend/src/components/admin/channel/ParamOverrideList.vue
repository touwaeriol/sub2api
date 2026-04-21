<template>
  <div>
    <div class="mb-1 flex items-center justify-between">
      <div class="flex flex-col">
        <label class="input-label text-xs mb-0">
          {{ t('admin.channels.form.paramOverrides') }}
          <span
            v-if="rules.length > 0"
            class="ml-1 font-normal text-gray-400"
          >
            ({{ t('admin.channels.form.paramOverridesCount', { count: rules.length }) }})
          </span>
        </label>
        <p class="mt-0.5 text-[11px] text-gray-400">
          {{ t('admin.channels.form.paramOverridesHint') }}
        </p>
      </div>
      <button
        type="button"
        class="text-xs text-primary-600 hover:text-primary-700"
        @click="addRule"
      >
        + {{ t('admin.channels.form.paramOverride.addRule') }}
      </button>
    </div>

    <div
      v-if="rules.length === 0"
      class="rounded border border-dashed border-gray-300 p-2 text-center text-xs text-gray-400 dark:border-dark-500"
    >
      {{ t('admin.channels.form.noParamOverrides') }}
    </div>

    <div v-else class="space-y-2">
      <div
        v-for="(rule, idx) in rules"
        :key="idx"
        class="relative"
      >
        <ParamOverrideEntryCard
          :rule="rule"
          :platform="platform"
          :rule-index="idx"
          @update="updateRule(idx, $event)"
          @remove="removeRule(idx)"
        />
        <!-- Reorder controls (move up / down), positioned on the card's left gutter -->
        <div class="absolute left-[-22px] top-2 flex flex-col gap-0.5">
          <button
            type="button"
            class="rounded p-0.5 text-gray-300 hover:text-primary-500 disabled:opacity-20"
            :disabled="idx === 0"
            :title="t('admin.channels.form.paramOverride.moveUp')"
            :aria-label="t('admin.channels.form.paramOverride.moveUp')"
            @click="moveRule(idx, idx - 1)"
          >
            <Icon name="chevronUp" size="sm" />
          </button>
          <button
            type="button"
            class="rounded p-0.5 text-gray-300 hover:text-primary-500 disabled:opacity-20"
            :disabled="idx === rules.length - 1"
            :title="t('admin.channels.form.paramOverride.moveDown')"
            :aria-label="t('admin.channels.form.paramOverride.moveDown')"
            @click="moveRule(idx, idx + 1)"
          >
            <Icon name="chevronDown" size="sm" />
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import ParamOverrideEntryCard from './ParamOverrideEntryCard.vue'
import { createEmptyRule } from './paramOverrideHelpers'
import type { ChannelParamOverrideRule } from '@/api/admin/channels'

const { t } = useI18n()

const props = defineProps<{
  rules: ChannelParamOverrideRule[]
  platform: string
}>()

const emit = defineEmits<{
  update: [rules: ChannelParamOverrideRule[]]
}>()

function addRule() {
  emit('update', [...props.rules, createEmptyRule()])
}

function updateRule(idx: number, rule: ChannelParamOverrideRule) {
  const next = [...props.rules]
  next[idx] = rule
  emit('update', next)
}

function removeRule(idx: number) {
  const next = [...props.rules]
  next.splice(idx, 1)
  emit('update', next)
}

function moveRule(from: number, to: number) {
  if (from === to || to < 0 || to >= props.rules.length) return
  const next = [...props.rules]
  const [moved] = next.splice(from, 1)
  next.splice(to, 0, moved)
  emit('update', next)
}
</script>
