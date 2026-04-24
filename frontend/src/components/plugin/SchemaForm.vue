<template>
  <div class="space-y-4">
    <div v-for="field in schema" :key="field.key" class="schema-form-field">
      <label :for="fieldId(field)" class="input-label mb-1.5 block">
        {{ t(field.i18nLabelKey) }}
        <span v-if="field.required" class="text-red-500">*</span>
      </label>
      <!-- string / secret -->
      <div v-if="field.type === FIELD_TYPE_STRING || field.type === FIELD_TYPE_SECRET" class="relative">
        <Input :id="fieldId(field)" :model-value="stringValue(field)" :type="fieldInputType(field)" :placeholder="resolvePlaceholder(field)" :error="fieldErrors[field.key]" @update:model-value="onStringInput(field, $event)">
          <template v-if="isSecretLike(field)" #suffix>
            <button type="button" class="text-xs text-gray-500 hover:text-gray-700 dark:text-dark-300 dark:hover:text-dark-100" @click="toggleSecret(field)">
              {{ secretVisible[field.key] ? t('common.hide') : t('common.show') }}
            </button>
          </template>
        </Input>
      </div>
      <!-- int -->
      <div v-else-if="field.type === FIELD_TYPE_INT">
        <Input :id="fieldId(field)" :model-value="numberValue(field)" type="number" :placeholder="resolvePlaceholder(field)" :error="fieldErrors[field.key]" @update:model-value="onIntInput(field, $event)" />
      </div>
      <!-- bool -->
      <div v-else-if="field.type === FIELD_TYPE_BOOL" class="flex items-center">
        <Toggle :id="fieldId(field)" :model-value="boolValue(field)" @update:model-value="onBoolInput(field, $event)" />
      </div>
      <!-- enum -->
      <div v-else-if="field.type === FIELD_TYPE_ENUM">
        <Select :id="fieldId(field)" :model-value="enumValue(field)" :options="selectOptionsFor(field)" :placeholder="resolvePlaceholder(field)" @update:model-value="onEnumInput(field, $event)" />
      </div>
      <!-- json -->
      <div v-else-if="field.type === FIELD_TYPE_JSON">
        <TextArea :id="fieldId(field)" :model-value="jsonText[field.key] ?? ''" :placeholder="resolvePlaceholder(field)" :rows="6" :error="fieldErrors[field.key]" @update:model-value="onJsonInput(field, $event)" />
      </div>
      <!-- hint -->
      <p v-if="field.i18nHintKey && !fieldErrors[field.key]" class="mt-1.5 text-xs text-gray-500 dark:text-dark-400">
        {{ t(field.i18nHintKey) }}
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from "vue"
import { useI18n } from "vue-i18n"
import Input from "@/components/common/Input.vue"
import TextArea from "@/components/common/TextArea.vue"
import Toggle from "@/components/common/Toggle.vue"
import Select from "@/components/common/Select.vue"
import type { SelectOption } from "@/components/common/Select.vue"
import {
  FIELD_TYPE_BOOL,
  FIELD_TYPE_ENUM,
  FIELD_TYPE_INT,
  FIELD_TYPE_JSON,
  FIELD_TYPE_SECRET,
  FIELD_TYPE_STRING,
  type FieldSchema,
  type FieldValue
} from "@/plugins/types"

interface Props {
  modelValue: Record<string, FieldValue | undefined>
  schema: FieldSchema[]
  context?: "settings" | "credentials"
  idPrefix?: string
}

const props = withDefaults(defineProps<Props>(), {
  context: "settings",
  idPrefix: "schema-form"
})

const emit = defineEmits<{
  (e: "update:modelValue", value: Record<string, FieldValue | undefined>): void
  (e: "field-change", key: string, value: FieldValue | undefined): void
  (e: "validation-change", errors: Record<string, string>): void
}>()

const { t } = useI18n()

const fieldErrors = reactive<Record<string, string>>({})
const secretVisible = reactive<Record<string, boolean>>({})
const jsonText = reactive<Record<string, string>>({})

const fieldId = (field: FieldSchema): string => `${props.idPrefix}-${field.key}`

const model = computed(() => props.modelValue)

function cloneValue(): Record<string, FieldValue | undefined> {
  return { ...props.modelValue }
}

function emitFieldChange(key: string, value: FieldValue | undefined): void {
  const next = cloneValue()
  next[key] = value
  emit("update:modelValue", next)
  emit("field-change", key, value)
}

function setError(key: string, message: string | null): void {
  if (message) {
    fieldErrors[key] = message
  } else {
    delete fieldErrors[key]
  }
  emit("validation-change", { ...fieldErrors })
}

function isSecretLike(field: FieldSchema): boolean {
  return field.type === FIELD_TYPE_SECRET || field.secret === true
}

function fieldInputType(field: FieldSchema): string {
  if (isSecretLike(field) && !secretVisible[field.key]) return "password"
  return "text"
}

function resolvePlaceholder(field: FieldSchema): string {
  return field.placeholder ?? ""
}

function stringValue(field: FieldSchema): string {
  const v = model.value[field.key]
  if (v == null) return ""
  return String(v)
}

function numberValue(field: FieldSchema): string {
  const v = model.value[field.key]
  if (v == null) return ""
  return String(v)
}

function boolValue(field: FieldSchema): boolean {
  return model.value[field.key] === true
}

function enumValue(field: FieldSchema): string | number | boolean | null {
  const v = model.value[field.key]
  if (v == null) return null
  if (typeof v === "string" || typeof v === "number" || typeof v === "boolean") return v
  return null
}

function toggleSecret(field: FieldSchema): void {
  secretVisible[field.key] = !secretVisible[field.key]
}

function onStringInput(field: FieldSchema, value: string): void {
  emitFieldChange(field.key, value)
  validatePattern(field, value)
}

function onIntInput(field: FieldSchema, raw: string): void {
  if (raw === "") {
    emitFieldChange(field.key, undefined)
    setError(field.key, null)
    return
  }
  const parsed = Number(raw)
  if (Number.isNaN(parsed) || !Number.isFinite(parsed)) {
    setError(field.key, t("common.invalidNumber"))
    return
  }
  setError(field.key, null)
  emitFieldChange(field.key, Math.trunc(parsed))
}

function onBoolInput(field: FieldSchema, value: boolean): void {
  emitFieldChange(field.key, value)
}

function onEnumInput(field: FieldSchema, value: string | number | boolean | null): void {
  emitFieldChange(field.key, value)
}

function onJsonInput(field: FieldSchema, raw: string): void {
  jsonText[field.key] = raw
  if (raw.trim() === "") {
    emitFieldChange(field.key, undefined)
    setError(field.key, null)
    return
  }
  try {
    const parsed = JSON.parse(raw)
    emitFieldChange(field.key, parsed as FieldValue)
    setError(field.key, null)
  } catch {
    setError(field.key, t("common.invalidJson"))
  }
}

function validatePattern(field: FieldSchema, value: string): void {
  if (!field.validator) return
  try {
    const re = new RegExp(field.validator)
    if (!re.test(value)) {
      setError(field.key, t("common.invalidFormat"))
    } else {
      setError(field.key, null)
    }
  } catch {
    // Invalid user-supplied regex: ignore silently.
  }
}

function selectOptionsFor(field: FieldSchema): SelectOption[] {
  const opts = field.options ?? []
  return opts.map((o) => ({
    value: o.value,
    label: t(o.i18nLabelKey)
  }))
}

// Seed jsonText on mount / schema change so existing JSON shows up in the textarea.
watch(
  () => props.schema,
  (list) => {
    for (const f of list) {
      if (f.type === FIELD_TYPE_JSON) {
        const v = props.modelValue[f.key]
        if (v !== undefined && jsonText[f.key] === undefined) {
          jsonText[f.key] = typeof v === "string" ? v : JSON.stringify(v, null, 2)
        }
      }
    }
  },
  { immediate: true }
)
</script>
