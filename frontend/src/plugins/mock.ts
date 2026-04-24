/**
 * Mock plugin manifest used when the backend Admin API is not yet
 * available (Phase 0) or returns 404. The list intentionally stays tiny —
 * downstream tests and dev-mode smoke checks rely on a predictable shape.
 */

import {
  FIELD_TYPE_STRING,
  FIELD_TYPE_BOOL,
  MENU_VISIBILITY_ADMIN,
  type PluginManifest
} from './types'

export function mockPluginList(): PluginManifest[] {
  return [
    {
      id: 'demo',
      name: 'Demo Plugin',
      version: '0.0.1',
      enabled: true,
      menus: [
        {
          id: 'demo-home',
          path: '/admin/plugins/demo',
          labelI18nKey: 'plugins.demo.name',
          visibility: MENU_VISIBILITY_ADMIN,
          order: 900
        }
      ],
      settings: [
        {
          key: 'greeting',
          i18nLabelKey: 'plugins.demo.settings.greeting.label',
          i18nHintKey: 'plugins.demo.settings.greeting.hint',
          default: 'Hello',
          schema: {
            key: 'greeting',
            type: FIELD_TYPE_STRING,
            i18nLabelKey: 'plugins.demo.settings.greeting.label',
            i18nHintKey: 'plugins.demo.settings.greeting.hint',
            default: 'Hello'
          }
        },
        {
          key: 'enabled',
          i18nLabelKey: 'plugins.demo.settings.enabled.label',
          default: true,
          schema: {
            key: 'enabled',
            type: FIELD_TYPE_BOOL,
            i18nLabelKey: 'plugins.demo.settings.enabled.label',
            default: true
          }
        }
      ]
    }
  ]
}
