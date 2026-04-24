import { describe, it, expect, beforeEach } from 'vitest'
import {
  useMenuRegistry,
  __resetMenuRegistryForTests
} from '../menuRegistry'
import {
  MENU_VISIBILITY_ADMIN,
  MENU_VISIBILITY_USER,
  MENU_VISIBILITY_BOTH
} from '../types'

describe('menuRegistry', () => {
  beforeEach(() => {
    __resetMenuRegistryForTests()
  })

  it('filters by visibility and respects "both"', () => {
    const m = useMenuRegistry()
    m.register({
      pluginId: 'a',
      path: '/admin/a',
      labelI18nKey: 'plugins.a.name',
      visibility: MENU_VISIBILITY_ADMIN,
      order: 0
    })
    m.register({
      pluginId: 'b',
      path: '/u/b',
      labelI18nKey: 'plugins.b.name',
      visibility: MENU_VISIBILITY_USER,
      order: 0
    })
    m.register({
      pluginId: 'c',
      path: '/c',
      labelI18nKey: 'plugins.c.name',
      visibility: MENU_VISIBILITY_BOTH,
      order: 0
    })

    const admin = m.getVisible('admin').map((i) => i.pluginId)
    const user = m.getVisible('user').map((i) => i.pluginId)
    expect(admin.sort()).toEqual(['a', 'c'])
    expect(user.sort()).toEqual(['b', 'c'])
  })

  it('sorts items by order ascending, then path', () => {
    const m = useMenuRegistry()
    m.register({
      pluginId: 'a',
      path: '/z',
      labelI18nKey: 'x',
      visibility: MENU_VISIBILITY_ADMIN,
      order: 10
    })
    m.register({
      pluginId: 'b',
      path: '/a',
      labelI18nKey: 'x',
      visibility: MENU_VISIBILITY_ADMIN,
      order: 5
    })
    m.register({
      pluginId: 'c',
      path: '/m',
      labelI18nKey: 'x',
      visibility: MENU_VISIBILITY_ADMIN,
      order: 5
    })
    const paths = m.getVisible('admin').map((i) => i.path)
    expect(paths).toEqual(['/a', '/m', '/z'])
  })

  it('removes all items for a plugin id', () => {
    const m = useMenuRegistry()
    m.register({ pluginId: 'a', path: '/a', labelI18nKey: 'x', visibility: MENU_VISIBILITY_ADMIN, order: 0 })
    m.register({ pluginId: 'a', path: '/b', labelI18nKey: 'x', visibility: MENU_VISIBILITY_ADMIN, order: 0 })
    m.register({ pluginId: 'c', path: '/c', labelI18nKey: 'x', visibility: MENU_VISIBILITY_ADMIN, order: 0 })
    m.remove('a')
    expect(m.all().map((i) => i.pluginId)).toEqual(['c'])
  })

  it('deduplicates on (pluginId, path): re-register replaces entry', () => {
    const m = useMenuRegistry()
    m.register({ pluginId: 'a', path: '/x', labelI18nKey: 'old', visibility: MENU_VISIBILITY_ADMIN, order: 5 })
    m.register({ pluginId: 'a', path: '/x', labelI18nKey: 'new', visibility: MENU_VISIBILITY_ADMIN, order: 5 })
    expect(m.all()).toHaveLength(1)
    expect(m.all()[0].labelI18nKey).toBe('new')
  })
})
