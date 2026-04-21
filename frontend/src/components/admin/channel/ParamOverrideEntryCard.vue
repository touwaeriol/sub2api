<template>
  <div class="rounded-lg border border-gray-200 bg-gray-50 p-3 dark:border-dark-600 dark:bg-dark-800">
    <!-- Header row: enabled toggle + glob + target + action + remove -->
    <div class="flex flex-wrap items-center gap-2">
      <span class="text-xs text-gray-400 dark:text-gray-500">#{{ ruleIndex + 1 }}</span>

      <Toggle :model-value="rule.enabled" @update:model-value="emitField('enabled', $event)" />

      <input
        :value="rule.model_glob"
        type="text"
        class="input w-32 text-xs"
        :placeholder="t('admin.channels.form.paramOverride.modelGlobPlaceholder', '* 或 claude-*')"
        :aria-label="t('admin.channels.form.paramOverride.modelGlob', 'Model glob')"
        @input="emitField('model_glob', ($event.target as HTMLInputElement).value)"
      />

      <Select
        :model-value="rule.target"
        :options="targetOptions"
        class="w-28"
        @update:model-value="onTargetChange(($event ?? PARAM_OVERRIDE_TARGETS[0]) as ParamOverrideTarget)"
      />

      <Select
        :model-value="rule.action"
        :options="actionOptions"
        class="w-28"
        @update:model-value="onActionChange(($event ?? PARAM_OVERRIDE_ACTIONS[0]) as ParamOverrideAction)"
      />

      <div class="ml-auto flex items-center gap-1">
        <button
          type="button"
          class="rounded p-1 text-gray-400 hover:text-red-500"
          :title="t('admin.channels.form.paramOverride.remove', '删除')"
          @click="emit('remove')"
        >
          <Icon name="trash" size="sm" />
        </button>
      </div>
    </div>

    <!-- Path + value row -->
    <div class="mt-2 grid grid-cols-1 gap-2 md:grid-cols-[minmax(0,1fr)_minmax(0,2fr)]">
      <div>
        <label class="input-label text-[11px] mb-1">
          {{ t('admin.channels.form.paramOverride.path', '字段路径') }}
        </label>
        <input
          :value="rule.path"
          type="text"
          class="input text-xs"
          :list="pathListId"
          :placeholder="pathPlaceholder"
          @input="emitField('path', ($event.target as HTMLInputElement).value)"
        />
        <datalist :id="pathListId">
          <option v-for="preset in pathPresets" :key="preset" :value="preset" />
        </datalist>
        <p v-if="reservedPathError" class="mt-1 text-[11px] text-red-500">
          {{ reservedPathError }}
        </p>
      </div>

      <div>
        <label class="input-label text-[11px] mb-1">
          {{ t('admin.channels.form.paramOverride.value', '值 (JSON)') }}
        </label>
        <textarea
          :value="valueText"
          rows="2"
          class="input text-xs font-mono"
          :disabled="isValueDisabled"
          :placeholder="t('admin.channels.form.paramOverride.valuePlaceholder', 'JSON 格式的值')"
          @input="onValueInput(($event.target as HTMLTextAreaElement).value)"
        ></textarea>
        <p v-if="isValueDisabled" class="mt-1 text-[11px] text-gray-400">
          {{ t('admin.channels.form.paramOverride.valueDisabledHint', '移除动作无需填写值') }}
        </p>
        <p v-else-if="jsonError" class="mt-1 text-[11px] text-red-500">
          {{ t('admin.channels.form.paramOverride.invalidJson', '值不是合法 JSON') }}
        </p>
        <p v-else-if="nullValueWarning" class="mt-1 text-[11px] text-red-500">
          {{ nullValueWarning }}
        </p>
      </div>
    </div>

    <!-- Incompatible combo warnings (action/target) -->
    <p v-if="mergeHeaderWarning" class="mt-2 text-[11px] text-red-500">
      {{ mergeHeaderWarning }}
    </p>
    <p v-else-if="appendBodyWarning" class="mt-2 text-[11px] text-red-500">
      {{ appendBodyWarning }}
    </p>

    <!-- Description -->
    <div class="mt-2">
      <input
        :value="rule.description"
        type="text"
        class="input text-xs"
        :placeholder="t('admin.channels.form.paramOverride.descriptionPlaceholder', '可选备注')"
        :aria-label="t('admin.channels.form.paramOverride.description', '备注')"
        @input="emitField('description', ($event.target as HTMLInputElement).value)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import {
  PARAM_OVERRIDE_ACTIONS,
  PARAM_OVERRIDE_TARGETS,
  TARGET_BODY,
  TARGET_HEADER,
  ACTION_MERGE,
  ACTION_REMOVE,
  ACTION_APPEND,
  type ParamOverrideAction,
  type ParamOverrideTarget,
  BODY_PATH_PRESETS,
  HEADER_KEY_PRESETS,
  RESERVED_BODY_PATHS,
} from './paramOverrideConstants'
import { parseJsonValue, stringifyValue } from './paramOverrideHelpers'
import type { ChannelParamOverrideRule } from '@/api/admin/channels'

const { t } = useI18n()

const props = defineProps<{
  rule: ChannelParamOverrideRule
  platform: string
  ruleIndex: number
}>()

const emit = defineEmits<{
  update: [rule: ChannelParamOverrideRule]
  remove: []
}>()

// ── Dropdown option labels (all via i18n) ──
const targetOptions = computed(() =>
  PARAM_OVERRIDE_TARGETS.map(v => ({
    value: v,
    label: t(`admin.channels.form.paramOverride.targets.${v}`, v),
  })),
)
const actionOptions = computed(() =>
  PARAM_OVERRIDE_ACTIONS.map(v => ({
    value: v,
    label: t(`admin.channels.form.paramOverride.actions.${v}`, v),
  })),
)

// ── Path presets (datalist auto-complete) ──
const pathListId = computed(() => `po-path-${props.platform}-${props.ruleIndex}`)
const pathPresets = computed<readonly string[]>(() => {
  const table = props.rule.target === TARGET_HEADER ? HEADER_KEY_PRESETS : BODY_PATH_PRESETS
  return table[props.platform] ?? []
})
const pathPlaceholder = computed(() =>
  props.rule.target === TARGET_HEADER
    ? t('admin.channels.form.paramOverride.pathPlaceholderHeader', 'e.g. anthropic-beta')
    : t('admin.channels.form.paramOverride.pathPlaceholderBody', 'e.g. thinking.budget_tokens'),
)

// ── Value editor (local text state + parse validation) ──
const valueText = ref<string>(stringifyValue(props.rule.value))
const jsonError = ref<string | null>(null)

// Re-sync local text when the parent replaces the rule reference (e.g. on
// reorder or reset). Reference equality is enough because edits flow through
// emit('update', ...) and the parent sends back a new object.
watch(
  () => props.rule,
  (next) => {
    const rendered = stringifyValue(next.value)
    if (rendered !== valueText.value) {
      valueText.value = rendered
      jsonError.value = null
    }
  },
)

const isValueDisabled = computed(() => props.rule.action === ACTION_REMOVE)

function onValueInput(text: string) {
  valueText.value = text
  const parsed = parseJsonValue(text)
  jsonError.value = parsed.error
  if (parsed.error) return
  emit('update', { ...props.rule, value: parsed.value })
}

// ── Validation warnings (displayed inline; backend still authoritative) ──
const reservedPathError = computed<string | null>(() => {
  if (props.rule.target !== TARGET_BODY) return null
  if (!(RESERVED_BODY_PATHS as readonly string[]).includes(props.rule.path)) return null
  return t('admin.channels.form.paramOverride.reservedPath', { path: props.rule.path })
})

const mergeHeaderWarning = computed<string | null>(() =>
  props.rule.action === ACTION_MERGE && props.rule.target === TARGET_HEADER
    ? t('admin.channels.form.paramOverride.mergeHeaderNotSupported', 'Merge not supported for headers')
    : null,
)

const appendBodyWarning = computed<string | null>(() =>
  props.rule.action === ACTION_APPEND && props.rule.target === TARGET_BODY
    ? t('admin.channels.form.paramOverride.appendBodyNotSupported', 'Append not supported for body')
    : null,
)

// Non-remove actions + value == null: prompt the user to switch to Remove.
// Mirrors the backend paramOverrideReasonValueNullUseRemove check.
const nullValueWarning = computed<string | null>(() => {
  if (props.rule.action === ACTION_REMOVE) return null
  if (props.rule.value !== null) return null
  return t('admin.channels.form.paramOverride.valueNullUseRemove')
})

// ── Field emitters ──
function emitField<K extends keyof ChannelParamOverrideRule>(field: K, value: ChannelParamOverrideRule[K]) {
  emit('update', { ...props.rule, [field]: value })
}

function onTargetChange(target: ParamOverrideTarget) {
  emit('update', { ...props.rule, target })
}

function onActionChange(action: ParamOverrideAction) {
  emit('update', { ...props.rule, action })
}
</script>
