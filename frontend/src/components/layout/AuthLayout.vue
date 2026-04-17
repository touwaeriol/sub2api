<template>
  <div class="auth-editorial">
    <div class="auth-container">
      <router-link to="/" class="auth-brand" aria-label="Claude">
        <ClaudeLogo class="auth-brand-logo" label="Claude" />
      </router-link>

      <div class="auth-card">
        <slot />
      </div>

      <div class="auth-footer-slot">
        <slot name="footer" />
      </div>

      <div class="auth-copy">
        &copy; {{ currentYear }} {{ siteName }}. All rights reserved.
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores'
import ClaudeLogo from '@/components/icons/ClaudeLogo.vue'

const { t } = useI18n()
const appStore = useAppStore()

// Brand string: locale-specific override wins, admin setting is fallback.
const siteName = computed(() => {
  const translated = t('brand.name')
  if (translated && translated !== 'brand.name' && translated.trim() !== '') {
    return translated
  }
  return appStore.siteName || 'Claude'
})
const currentYear = computed(() => new Date().getFullYear())

onMounted(() => {
  appStore.fetchPublicSettings()
})
</script>

<style scoped>
.auth-editorial {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 56px 24px;
  background: #F5F4EE;
  color: #1A1A1A;
  font-feature-settings: 'kern';
  -webkit-font-smoothing: antialiased;
}
.dark .auth-editorial {
  background: #0F0E0B;
  color: #F5F4EE;
}

.auth-container {
  width: 100%;
  max-width: 420px;
}

.auth-brand {
  display: flex;
  justify-content: center;
  margin-bottom: 32px;
}
.auth-brand-logo {
  height: 32px;
  width: auto;
  color: #1A1A1A;
}
.dark .auth-brand-logo {
  color: #F5F4EE;
}

.auth-card {
  padding: 40px 36px 36px;
  border-radius: 18px;
  border: 1px solid rgba(26, 26, 26, 0.09);
  background: #FBFAF4;
  box-shadow: 0 28px 60px -40px rgba(26, 26, 26, 0.22);
}
.dark .auth-card {
  background: #15130E;
  border-color: rgba(245, 244, 238, 0.09);
  box-shadow: 0 28px 60px -40px rgba(0, 0, 0, 0.6);
}

.auth-footer-slot {
  margin-top: 24px;
  text-align: center;
  font-size: 13px;
}

.auth-copy {
  margin-top: 40px;
  text-align: center;
  font-size: 11px;
  letter-spacing: 0.04em;
  color: rgba(26, 26, 26, 0.4);
}
.dark .auth-copy {
  color: rgba(245, 244, 238, 0.4);
}

/* ---------- :deep() overrides for slotted auth form content ---------- */

/* Title heading — LoginView / RegisterView's <h2> */
.auth-card :deep(h2) {
  font-family:
    'Iowan Old Style', 'Charter', 'Source Serif Pro', Georgia, 'Times New Roman',
    'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', 'Noto Sans SC', sans-serif;
  font-weight: 400;
  font-size: 32px;
  line-height: 1.08;
  letter-spacing: -0.015em;
  color: #1A1A1A;
  background: none !important;
  background-clip: initial !important;
  -webkit-background-clip: initial !important;
  -webkit-text-fill-color: initial !important;
}
.dark .auth-card :deep(h2) {
  color: #F5F4EE;
}
:global(html[lang="zh"]) .auth-card :deep(h2) {
  font-weight: 300;
  letter-spacing: 0;
}

/* Subtitle p directly under title */
.auth-card :deep(.text-center > p) {
  margin-top: 10px;
  font-size: 13px;
  color: rgba(26, 26, 26, 0.55);
}
.dark .auth-card :deep(.text-center > p) {
  color: rgba(245, 244, 238, 0.55);
}

/* Input label */
.auth-card :deep(.input-label) {
  display: block;
  margin-bottom: 8px;
  font-size: 12px;
  font-weight: 500;
  color: rgba(26, 26, 26, 0.7);
  letter-spacing: 0.02em;
}
.dark .auth-card :deep(.input-label) {
  color: rgba(245, 244, 238, 0.7);
}

/* Hide prefix icon containers inside input wrappers — cleaner Claude look */
.auth-card :deep(.relative > .pointer-events-none.absolute.left-0) {
  display: none;
}

/* Inputs */
.auth-card :deep(.input) {
  width: 100%;
  padding: 13px 16px !important;
  border-radius: 10px;
  border: 1px solid rgba(26, 26, 26, 0.14);
  background: #FFFFFF;
  font-size: 14px;
  line-height: 1.4;
  color: #1A1A1A;
  box-shadow: none;
  transition: border-color 150ms ease, box-shadow 150ms ease;
}
.auth-card :deep(.input)::placeholder {
  color: rgba(26, 26, 26, 0.35);
}
.auth-card :deep(.input):focus {
  outline: none;
  border-color: #1A1A1A;
  box-shadow: 0 0 0 3px rgba(26, 26, 26, 0.08);
}
.auth-card :deep(.input.input-error),
.auth-card :deep(.input.border-red-500) {
  border-color: #E05C4A;
}
.auth-card :deep(.input.input-error):focus,
.auth-card :deep(.input.border-red-500):focus {
  box-shadow: 0 0 0 3px rgba(224, 92, 74, 0.12);
}
.dark .auth-card :deep(.input) {
  background: #1C1914;
  border-color: rgba(245, 244, 238, 0.14);
  color: #F5F4EE;
}
.dark .auth-card :deep(.input)::placeholder {
  color: rgba(245, 244, 238, 0.35);
}
.dark .auth-card :deep(.input):focus {
  border-color: #F5F4EE;
  box-shadow: 0 0 0 3px rgba(245, 244, 238, 0.08);
}

/* Password eye-toggle button — recolor only */
.auth-card :deep(.relative > button.absolute.right-0) {
  color: rgba(26, 26, 26, 0.4);
}
.auth-card :deep(.relative > button.absolute.right-0:hover) {
  color: rgba(26, 26, 26, 0.75);
}
.dark .auth-card :deep(.relative > button.absolute.right-0) {
  color: rgba(245, 244, 238, 0.4);
}
.dark .auth-card :deep(.relative > button.absolute.right-0:hover) {
  color: rgba(245, 244, 238, 0.75);
}

/* Primary submit button */
.auth-card :deep(.btn.btn-primary) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  width: 100%;
  padding: 14px 24px;
  border-radius: 9999px;
  background: #1A1A1A;
  background-image: none;
  color: #F5F4EE;
  font-size: 14px;
  font-weight: 500;
  box-shadow: none;
  transition: background-color 150ms ease, transform 150ms ease;
}
.auth-card :deep(.btn.btn-primary):hover {
  background: #2A2A2A;
  background-image: none;
  box-shadow: none;
}
.auth-card :deep(.btn.btn-primary):disabled {
  opacity: 0.55;
  cursor: not-allowed;
}
.dark .auth-card :deep(.btn.btn-primary) {
  background: #F5F4EE;
  background-image: none;
  color: #1A1A1A;
}
.dark .auth-card :deep(.btn.btn-primary):hover {
  background: #ffffff;
  background-image: none;
}

/* Hide decorative icons inside the primary button (login icon, etc.),
   but keep the loading spinner visible. */
.auth-card :deep(.btn.btn-primary svg:not(.animate-spin)) {
  display: none;
}

/* Forgot password / inline text links (primary-colored in original) */
.auth-card :deep(a.text-primary-600),
.auth-card :deep(a.text-primary-500),
.auth-card :deep(a.dark\:text-primary-400),
.auth-card :deep(.text-primary-600),
.auth-card :deep(.text-primary-500) {
  color: #CC785C !important;
}
.auth-card :deep(a.hover\:text-primary-500:hover),
.auth-card :deep(a.hover\:text-primary-600:hover) {
  color: #B86647 !important;
}

/* OAuth section dividers — use muted ink */
.auth-card :deep(.h-px.bg-gray-200),
.auth-card :deep(.h-px.dark\:bg-dark-700) {
  background: rgba(26, 26, 26, 0.08);
}
.dark .auth-card :deep(.h-px) {
  background: rgba(245, 244, 238, 0.1);
}

/* Footer slot link color */
.auth-footer-slot :deep(a),
.auth-footer-slot :deep(.text-primary-600),
.auth-footer-slot :deep(.text-primary-500) {
  color: #1A1A1A;
  font-weight: 500;
  text-decoration: underline;
  text-underline-offset: 4px;
  text-decoration-color: rgba(26, 26, 26, 0.22);
  transition: text-decoration-color 150ms ease;
}
.auth-footer-slot :deep(a:hover) {
  text-decoration-color: #1A1A1A;
}
.dark .auth-footer-slot :deep(a) {
  color: #F5F4EE;
  text-decoration-color: rgba(245, 244, 238, 0.22);
}
.dark .auth-footer-slot :deep(a:hover) {
  text-decoration-color: #F5F4EE;
}
</style>
