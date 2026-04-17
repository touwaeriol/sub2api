<template>
  <div class="docs-page min-h-screen bg-white dark:bg-gray-950">
    <!-- Header -->
    <header class="sticky top-0 z-30 border-b border-gray-200 dark:border-gray-800 bg-white/80 dark:bg-gray-950/80 backdrop-blur-sm">
      <div class="max-w-7xl mx-auto flex items-center justify-between h-14 px-4 lg:px-8">
        <div class="flex items-center gap-3">
          <router-link to="/" class="flex items-center gap-2 text-gray-900 dark:text-gray-100 hover:text-primary-600 transition-colors">
            <img v-if="siteLogo" :src="siteLogo" alt="logo" class="h-7 w-7 rounded" />
            <span class="font-semibold text-sm">{{ siteName || 'Sub2API' }}</span>
          </router-link>
          <span class="text-gray-300 dark:text-gray-700">/</span>
          <span class="text-sm text-gray-500 dark:text-gray-400">{{ t('docs.title') }}</span>
        </div>
        <div class="flex items-center gap-2">
          <!-- Mobile sidebar toggle -->
          <button
            @click="sidebarOpen = !sidebarOpen"
            class="lg:hidden p-2 rounded-md text-gray-500 hover:text-gray-700 dark:text-gray-400 dark:hover:text-gray-200 hover:bg-gray-100 dark:hover:bg-gray-800"
          >
            <svg class="w-5 h-5" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M4 6h16M4 12h16M4 18h16" />
            </svg>
          </button>
          <router-link to="/" class="text-sm text-gray-500 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400 transition-colors">
            {{ t('docs.backToHome') }}
          </router-link>
        </div>
      </div>
    </header>

    <div class="max-w-7xl mx-auto flex">
      <!-- Sidebar -->
      <aside
        :class="[
          'fixed inset-y-0 left-0 z-20 w-72 bg-white dark:bg-gray-950 border-r border-gray-200 dark:border-gray-800 pt-14 overflow-y-auto transition-transform lg:sticky lg:top-14 lg:h-[calc(100vh-3.5rem)] lg:translate-x-0',
          sidebarOpen ? 'translate-x-0' : '-translate-x-full'
        ]"
      >
        <!-- Mobile overlay -->
        <div
          v-if="sidebarOpen"
          class="fixed inset-0 bg-black/20 lg:hidden z-[-1]"
          @click="sidebarOpen = false"
        />

        <nav class="p-4 space-y-6">
          <div v-if="loading" class="space-y-3">
            <div v-for="i in 4" :key="i" class="h-4 bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
          </div>
          <div v-else-if="config" v-for="group in config.groups" :key="group.title" class="space-y-1">
            <h3 class="text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 px-3 mb-2">
              {{ group.title }}
            </h3>
            <router-link
              v-for="item in group.items"
              :key="item.slug"
              :to="`/docs/${item.slug}`"
              class="block px-3 py-1.5 text-sm rounded-md transition-colors"
              :class="currentSlug === item.slug
                ? 'bg-primary-50 dark:bg-primary-900/20 text-primary-700 dark:text-primary-400 font-medium'
                : 'text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-200 hover:bg-gray-50 dark:hover:bg-gray-900'"
              @click="sidebarOpen = false"
            >
              {{ item.title }}
            </router-link>
          </div>
          <div v-else class="text-sm text-gray-400 dark:text-gray-500 px-3">
            {{ t('docs.noContent') }}
          </div>
        </nav>
      </aside>

      <!-- Main content -->
      <main class="flex-1 min-w-0 px-6 py-8 lg:px-12 lg:py-10">
        <div v-if="contentLoading" class="space-y-4 max-w-3xl">
          <div class="h-8 w-2/3 bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
          <div class="h-4 w-full bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
          <div class="h-4 w-5/6 bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
          <div class="h-4 w-4/6 bg-gray-100 dark:bg-gray-800 rounded animate-pulse" />
        </div>

        <!-- Welcome page when no slug -->
        <div v-else-if="!currentSlug && config?.groups?.length" class="max-w-3xl">
          <h1 class="text-3xl font-bold text-gray-900 dark:text-gray-100 mb-4">{{ t('docs.title') }}</h1>
          <p class="text-gray-500 dark:text-gray-400 mb-8">{{ t('docs.welcomeDescription') }}</p>
          <div v-for="group in config.groups" :key="group.title" class="mb-8">
            <h2 class="text-lg font-semibold text-gray-800 dark:text-gray-200 mb-3">{{ group.title }}</h2>
            <div class="grid gap-2">
              <router-link
                v-for="item in group.items"
                :key="item.slug"
                :to="`/docs/${item.slug}`"
                class="flex items-center gap-3 px-4 py-3 rounded-lg border border-gray-200 dark:border-gray-800 hover:border-primary-300 dark:hover:border-primary-700 hover:bg-primary-50/50 dark:hover:bg-primary-900/10 transition-colors group"
              >
                <svg class="w-4 h-4 text-gray-400 group-hover:text-primary-500 transition-colors flex-shrink-0" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                  <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M9 12h6m-6 4h6m2 5H7a2 2 0 01-2-2V5a2 2 0 012-2h5.586a1 1 0 01.707.293l5.414 5.414a1 1 0 01.293.707V19a2 2 0 01-2 2z" />
                </svg>
                <span class="text-sm text-gray-700 dark:text-gray-300 group-hover:text-primary-700 dark:group-hover:text-primary-400 transition-colors">{{ item.title }}</span>
              </router-link>
            </div>
          </div>
        </div>

        <!-- No docs configured -->
        <div v-else-if="!currentSlug" class="max-w-3xl text-center py-20">
          <svg class="w-16 h-16 text-gray-300 dark:text-gray-700 mx-auto mb-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M12 6.253v13m0-13C10.832 5.477 9.246 5 7.5 5S4.168 5.477 3 6.253v13C4.168 18.477 5.754 18 7.5 18s3.332.477 4.5 1.253m0-13C13.168 5.477 14.754 5 16.5 5c1.747 0 3.332.477 4.5 1.253v13C19.832 18.477 18.247 18 16.5 18c-1.746 0-3.332.477-4.5 1.253" />
          </svg>
          <p class="text-gray-400 dark:text-gray-500">{{ t('docs.noContent') }}</p>
        </div>

        <!-- Rendered markdown content -->
        <article v-else-if="renderedContent" class="docs-content max-w-3xl" v-html="renderedContent" />

        <!-- Document not found -->
        <div v-else-if="contentError" class="max-w-3xl text-center py-20">
          <p class="text-gray-400 dark:text-gray-500">{{ t('docs.notFound') }}</p>
          <router-link to="/docs" class="text-primary-600 hover:text-primary-700 text-sm mt-2 inline-block">
            {{ t('docs.backToIndex') }}
          </router-link>
        </div>

        <!-- Prev / Next navigation -->
        <div v-if="currentSlug && (prevDoc || nextDoc)" class="max-w-3xl mt-12 pt-6 border-t border-gray-200 dark:border-gray-800 flex justify-between">
          <router-link
            v-if="prevDoc"
            :to="`/docs/${prevDoc.slug}`"
            class="text-sm text-gray-500 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400 transition-colors"
          >
            &larr; {{ prevDoc.title }}
          </router-link>
          <span v-else />
          <router-link
            v-if="nextDoc"
            :to="`/docs/${nextDoc.slug}`"
            class="text-sm text-gray-500 hover:text-primary-600 dark:text-gray-400 dark:hover:text-primary-400 transition-colors"
          >
            {{ nextDoc.title }} &rarr;
          </router-link>
        </div>
      </main>

      <!-- Right-side TOC -->
      <aside v-if="toc.length" class="hidden xl:block w-56 shrink-0 sticky top-14 h-[calc(100vh-3.5rem)] overflow-y-auto py-10 pr-4">
        <h4 class="text-xs font-semibold uppercase tracking-wider text-gray-400 dark:text-gray-500 mb-3">{{ t('docs.onThisPage') }}</h4>
        <nav class="space-y-1">
          <a
            v-for="item in toc"
            :key="item.id"
            :href="`#${item.id}`"
            class="block text-sm text-gray-500 dark:text-gray-400 hover:text-primary-600 dark:hover:text-primary-400 transition-colors truncate"
            :class="{ 'pl-3': item.level === 3 }"
          >
            {{ item.text }}
          </a>
        </nav>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { marked } from 'marked'
import DOMPurify from 'dompurify'
import { getDocsConfig, getDocContent, type DocsConfig, type DocsItem } from '@/api/docs'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const route = useRoute()
const appStore = useAppStore()

const config = ref<DocsConfig | null>(null)
const loading = ref(true)
const contentLoading = ref(false)
const contentError = ref(false)
const renderedContent = ref('')
const sidebarOpen = ref(false)

const siteLogo = computed(() => appStore.cachedPublicSettings?.site_logo || '')
const siteName = computed(() => appStore.cachedPublicSettings?.site_name || '')

const currentSlug = computed(() => (route.params.slug as string) || '')

// Flatten all items for prev/next navigation
const allItems = computed(() => {
  if (!config.value) return [] as DocsItem[]
  return config.value.groups.flatMap(g => g.items)
})

const currentIndex = computed(() => allItems.value.findIndex(i => i.slug === currentSlug.value))
const prevDoc = computed(() => currentIndex.value > 0 ? allItems.value[currentIndex.value - 1] : null)
const nextDoc = computed(() => currentIndex.value >= 0 && currentIndex.value < allItems.value.length - 1 ? allItems.value[currentIndex.value + 1] : null)

// Extract TOC from rendered HTML
interface TocItem {
  id: string
  text: string
  level: number
}
const toc = ref<TocItem[]>([])

function extractToc(html: string): TocItem[] {
  const items: TocItem[] = []
  const regex = /<h([23])\s+id="([^"]+)"[^>]*>(.*?)<\/h[23]>/gi
  let match
  while ((match = regex.exec(html)) !== null) {
    items.push({
      level: parseInt(match[1]),
      id: match[2],
      text: match[3].replace(/<[^>]*>/g, '') // strip inner HTML tags
    })
  }
  return items
}

// Configure marked to add id to headings
const renderer = new marked.Renderer()
renderer.heading = function ({ text, depth }: { text: string; depth: number }) {
  const id = text
    .toLowerCase()
    .replace(/<[^>]*>/g, '')
    .replace(/[^\w\u4e00-\u9fff]+/g, '-')
    .replace(/^-+|-+$/g, '')
  return `<h${depth} id="${id}">${text}</h${depth}>`
}
marked.use({ renderer })

async function loadConfig() {
  try {
    config.value = await getDocsConfig()
  } catch {
    config.value = { groups: [] }
  } finally {
    loading.value = false
  }
}

async function loadContent(slug: string) {
  if (!slug) {
    renderedContent.value = ''
    contentError.value = false
    toc.value = []
    return
  }
  contentLoading.value = true
  contentError.value = false
  try {
    const md = await getDocContent(slug)
    const html = await marked.parse(md)
    renderedContent.value = DOMPurify.sanitize(html)
    toc.value = extractToc(renderedContent.value)
  } catch {
    renderedContent.value = ''
    contentError.value = true
    toc.value = []
  } finally {
    contentLoading.value = false
  }
}

watch(currentSlug, (slug) => loadContent(slug), { immediate: true })

onMounted(() => {
  loadConfig()
  if (!appStore.cachedPublicSettings) {
    appStore.fetchPublicSettings()
  }
})
</script>

<style>
/* Docs prose styles */
.docs-content {
  @apply text-gray-800 dark:text-gray-200 leading-relaxed;
}
.docs-content h1 {
  @apply text-3xl font-bold mt-0 mb-4 text-gray-900 dark:text-gray-100;
}
.docs-content h2 {
  @apply text-2xl font-semibold mt-10 mb-4 pb-2 border-b border-gray-200 dark:border-gray-800 text-gray-900 dark:text-gray-100;
}
.docs-content h3 {
  @apply text-lg font-semibold mt-8 mb-3 text-gray-900 dark:text-gray-100;
}
.docs-content p {
  @apply my-4;
}
.docs-content ul, .docs-content ol {
  @apply my-4 pl-6;
}
.docs-content ul {
  @apply list-disc;
}
.docs-content ol {
  @apply list-decimal;
}
.docs-content li {
  @apply my-1;
}
.docs-content a {
  @apply text-primary-600 dark:text-primary-400 hover:underline;
}
.docs-content code {
  @apply text-sm bg-gray-100 dark:bg-gray-800 px-1.5 py-0.5 rounded text-primary-700 dark:text-primary-300;
}
.docs-content pre {
  @apply my-4 p-4 rounded-lg bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-800 overflow-x-auto;
}
.docs-content pre code {
  @apply bg-transparent p-0 text-gray-800 dark:text-gray-200;
}
.docs-content blockquote {
  @apply my-4 pl-4 border-l-4 border-primary-300 dark:border-primary-700 text-gray-600 dark:text-gray-400 italic;
}
.docs-content table {
  @apply w-full my-4 border-collapse;
}
.docs-content th {
  @apply text-left px-4 py-2 bg-gray-50 dark:bg-gray-900 border border-gray-200 dark:border-gray-800 font-semibold text-sm;
}
.docs-content td {
  @apply px-4 py-2 border border-gray-200 dark:border-gray-800 text-sm;
}
.docs-content img {
  @apply rounded-lg max-w-full my-4;
}
.docs-content hr {
  @apply my-8 border-gray-200 dark:border-gray-800;
}
</style>
