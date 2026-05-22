/**
 * Lightweight SVG sanitizer for plugin-sdk components.
 *
 * Stripped because plugin-sdk does not depend on DOMPurify (the host does — see
 * frontend/src/utils/sanitize.ts). The host's bundle ships DOMPurify, but plugin
 * entries are loaded via importmap and adding DOMPurify to the SDK's runtime
 * dependency tree would force every plugin to also resolve it.
 *
 * Removes attack vectors that DOMPurify's SVG profile filters out:
 *   - <script> elements (any case, with or without attributes)
 *   - on*= event handler attributes (onload, onerror, onclick, ...)
 *   - href / xlink:href values starting with `javascript:`
 *   - <foreignObject> (allows embedded HTML / scripts)
 *
 * This is a defense-in-depth string filter; for fully audited sanitization the
 * host pipeline should validate SVG before it reaches the plugin.
 */

const SCRIPT_TAG_RE = /<script\b[\s\S]*?<\/script\s*>/gi
const SCRIPT_SELF_CLOSE_RE = /<script\b[^>]*\/?>/gi
const FOREIGN_OBJECT_RE = /<\/?foreignObject\b[^>]*>/gi
const EVENT_HANDLER_RE = /\s+on[a-z]+\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)/gi
const JS_URL_RE = /\b(href|xlink:href)\s*=\s*("javascript:[^"]*"|'javascript:[^']*'|javascript:[^\s>]+)/gi

export function sanitizeSvg(svg: string): string {
  if (!svg) return ''
  return svg
    .replace(SCRIPT_TAG_RE, '')
    .replace(SCRIPT_SELF_CLOSE_RE, '')
    .replace(FOREIGN_OBJECT_RE, '')
    .replace(EVENT_HANDLER_RE, '')
    .replace(JS_URL_RE, '')
}
