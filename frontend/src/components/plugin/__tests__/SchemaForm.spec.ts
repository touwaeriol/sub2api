import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, ref, nextTick } from 'vue'
import SchemaForm from '../SchemaForm.vue'
import {
  FIELD_TYPE_BOOL,
  FIELD_TYPE_ENUM,
  FIELD_TYPE_INT,
  FIELD_TYPE_JSON,
  FIELD_TYPE_SECRET,
  FIELD_TYPE_STRING,
  type FieldSchema,
  type FieldValue
} from '@/plugins/types'

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key,
    locale: ref('en')
  })
}))

const InputStub = defineComponent({
  name: 'InputStub',
  props: {
    modelValue: { type: [String, Number], default: '' },
    type: { type: String, default: 'text' },
    placeholder: { type: String, default: '' },
    error: { type: String, default: '' }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('input', {
        class: 'stub-input',
        type: props.type,
        value: props.modelValue ?? '',
        onInput: (e: Event) => emit('update:modelValue', (e.target as HTMLInputElement).value)
      })
  }
})

const TextAreaStub = defineComponent({
  name: 'TextAreaStub',
  props: {
    modelValue: { type: String, default: '' },
    placeholder: { type: String, default: '' },
    rows: { type: [Number, String], default: 3 },
    error: { type: String, default: '' }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h('textarea', {
        class: 'stub-textarea',
        value: props.modelValue,
        onInput: (e: Event) => emit('update:modelValue', (e.target as HTMLTextAreaElement).value)
      })
  }
})

const ToggleStub = defineComponent({
  name: 'ToggleStub',
  props: { modelValue: { type: Boolean, default: false } },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h(
        'button',
        {
          class: 'stub-toggle',
          'data-value': String(props.modelValue),
          onClick: () => emit('update:modelValue', !props.modelValue)
        },
        'tgl'
      )
  }
})

interface StubSelectOption { value: string | number | boolean; label: string }

const SelectStub = defineComponent({
  name: 'SelectStub',
  props: {
    modelValue: { type: [String, Number, Boolean, Object], default: null },
    options: { type: Array as () => StubSelectOption[], default: () => [] }
  },
  emits: ['update:modelValue'],
  setup(props, { emit }) {
    return () =>
      h(
        'select',
        {
          class: 'stub-select',
          value: props.modelValue,
          onChange: (e: Event) => emit('update:modelValue', (e.target as HTMLSelectElement).value)
        },
        props.options.map((o) =>
          h('option', { key: String(o.value), value: o.value }, o.label)
        )
      )
  }
})

function mountForm(schema: FieldSchema[], initial: Record<string, FieldValue | undefined> = {}) {
  return mount(SchemaForm, {
    props: { modelValue: initial, schema },
    global: {
      stubs: {
        Input: InputStub,
        TextArea: TextAreaStub,
        Toggle: ToggleStub,
        Select: SelectStub
      }
    }
  })
}

describe('SchemaForm', () => {
  it('renders string input and emits update on change', async () => {
    const wrapper = mountForm([
      { key: 'name', type: FIELD_TYPE_STRING, i18nLabelKey: 'label.name' }
    ])
    expect(wrapper.find('label').text()).toContain('label.name')
    const input = wrapper.find('input.stub-input')
    await input.setValue('hello')
    const events = wrapper.emitted('update:modelValue')
    expect(events).toBeTruthy()
    expect(events![events!.length - 1][0]).toEqual({ name: 'hello' })
  })

  it('masks secret fields by default as password input', () => {
    const wrapper = mountForm([
      { key: 'token', type: FIELD_TYPE_SECRET, i18nLabelKey: 'label.token' }
    ])
    const input = wrapper.find('input.stub-input')
    expect(input.attributes('type')).toBe('password')
  })

  it('parses int input and rejects invalid numbers', async () => {
    const wrapper = mountForm([
      { key: 'qty', type: FIELD_TYPE_INT, i18nLabelKey: 'label.qty' }
    ])
    const input = wrapper.find('input.stub-input')
    // Dispatch update:modelValue directly so we can simulate invalid raw text
    // (jsdom would otherwise strip "not-a-number" from a <input type=number>).
    await input.setValue('42')
    const events = wrapper.emitted('update:modelValue')
    expect(events![events!.length - 1][0]).toEqual({ qty: 42 })

    const stub = wrapper.findComponent({ name: 'InputStub' })
    stub.vm.$emit('update:modelValue', 'not-a-number')
    await wrapper.vm.$nextTick()
    const validation = wrapper.emitted('validation-change')
    expect(validation).toBeTruthy()
    const latest = validation![validation!.length - 1][0] as Record<string, string>
    expect(latest.qty).toBe('common.invalidNumber')
  })

  it('toggles bool values', async () => {
    const wrapper = mountForm(
      [{ key: 'flag', type: FIELD_TYPE_BOOL, i18nLabelKey: 'label.flag' }],
      { flag: false }
    )
    await wrapper.find('button.stub-toggle').trigger('click')
    const events = wrapper.emitted('update:modelValue')
    expect(events![events!.length - 1][0]).toEqual({ flag: true })
  })

  it('renders enum options and emits selection', async () => {
    const wrapper = mountForm([
      {
        key: 'color',
        type: FIELD_TYPE_ENUM,
        i18nLabelKey: 'label.color',
        options: [
          { value: 'red', i18nLabelKey: 'color.red' },
          { value: 'blue', i18nLabelKey: 'color.blue' }
        ]
      }
    ])
    const select = wrapper.find('select.stub-select')
    expect(select.findAll('option')).toHaveLength(2)
    await select.setValue('red')
    const events = wrapper.emitted('update:modelValue')
    expect(events![events!.length - 1][0]).toEqual({ color: 'red' })
  })

  it('parses JSON input and reports errors on bad JSON', async () => {
    const wrapper = mountForm([
      { key: 'cfg', type: FIELD_TYPE_JSON, i18nLabelKey: 'label.cfg' }
    ])
    const textarea = wrapper.find('textarea.stub-textarea')
    await textarea.setValue('{"a":1}')
    const events = wrapper.emitted('update:modelValue')
    expect(events![events!.length - 1][0]).toEqual({ cfg: { a: 1 } })

    await textarea.setValue('{ not json')
    const validation = wrapper.emitted('validation-change')
    expect(validation).toBeTruthy()
    const latest = validation![validation!.length - 1][0] as Record<string, string>
    expect(latest.cfg).toBe('common.invalidJson')
  })

  it('renders the hint key when provided', async () => {
    const wrapper = mountForm([
      {
        key: 'greeting',
        type: FIELD_TYPE_STRING,
        i18nLabelKey: 'label.greeting',
        i18nHintKey: 'hint.greeting'
      }
    ])
    await nextTick()
    expect(wrapper.text()).toContain('hint.greeting')
  })
})
