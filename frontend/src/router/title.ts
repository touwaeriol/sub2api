import { i18n } from '@/i18n'

/**
 * 统一生成页面标题，避免多处写入 document.title 产生覆盖冲突。
 * 优先使用 titleKey 通过 i18n 翻译，fallback 到静态 routeTitle。
 */
export function resolveDocumentTitle(
  routeTitle: unknown,
  siteName?: string,
  titleKey?: string,
  omitSiteName = false
): string {
  const normalizedSiteName = typeof siteName === 'string' && siteName.trim() ? siteName.trim() : 'Sub2API'

  if (typeof titleKey === 'string' && titleKey.trim()) {
    const translated = i18n.global.t(titleKey)
    if (translated && translated !== titleKey) {
      return omitSiteName ? translated : `${translated} - ${normalizedSiteName}`
    }
  }

  if (typeof routeTitle === 'string' && routeTitle.trim()) {
    return omitSiteName ? routeTitle.trim() : `${routeTitle.trim()} - ${normalizedSiteName}`
  }

  return normalizedSiteName
}

/**
 * Write <meta name="description"> / <meta name="keywords"> for the current route.
 * Also syncs the og:description / twitter:description mirrors.
 * Empty/undefined i18n keys leave the baseline tags from index.html in place.
 */
export function resolveDocumentMeta(descriptionKey?: string, keywordsKey?: string): void {
  if (typeof document === 'undefined') return

  const setMetaByName = (name: string, content: string) => {
    let el = document.querySelector<HTMLMetaElement>(`meta[name="${name}"]`)
    if (!el) {
      el = document.createElement('meta')
      el.setAttribute('name', name)
      document.head.appendChild(el)
    }
    el.setAttribute('content', content)
  }

  const setMetaByProperty = (prop: string, content: string) => {
    let el = document.querySelector<HTMLMetaElement>(`meta[property="${prop}"]`)
    if (!el) {
      el = document.createElement('meta')
      el.setAttribute('property', prop)
      document.head.appendChild(el)
    }
    el.setAttribute('content', content)
  }

  if (typeof descriptionKey === 'string' && descriptionKey.trim()) {
    const translated = i18n.global.t(descriptionKey)
    if (translated && translated !== descriptionKey) {
      setMetaByName('description', translated)
      setMetaByProperty('og:description', translated)
      setMetaByName('twitter:description', translated)
    }
  }

  if (typeof keywordsKey === 'string' && keywordsKey.trim()) {
    const translated = i18n.global.t(keywordsKey)
    if (translated && translated !== keywordsKey) {
      setMetaByName('keywords', translated)
    }
  }
}
