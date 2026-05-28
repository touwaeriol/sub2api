<template>
  <div class="min-h-screen bg-gray-50 text-gray-900 dark:bg-dark-950 dark:text-white">
    <header class="border-b border-gray-200 bg-white/95 dark:border-dark-800 dark:bg-dark-900/95">
      <div class="mx-auto flex max-w-6xl items-center justify-between gap-4 px-4 py-4 sm:px-6">
        <RouterLink to="/home" class="flex min-w-0 items-center gap-3">
          <span class="flex h-10 w-10 flex-shrink-0 items-center justify-center overflow-hidden rounded-xl bg-white shadow-sm ring-1 ring-gray-200 dark:bg-dark-800 dark:ring-dark-700">
            <img :src="siteLogo || '/logo.png'" alt="Logo" class="h-full w-full object-contain" />
          </span>
          <span class="truncate text-base font-semibold text-gray-950 dark:text-white">
            {{ siteName }}
          </span>
        </RouterLink>
        <span class="text-sm text-gray-400 dark:text-dark-500">OpusMax Monitor</span>
      </div>
    </header>

    <main class="mx-auto max-w-6xl px-4 py-8 sm:px-6">
      <div v-if="loading && !data" class="flex min-h-[320px] items-center justify-center">
        <div class="h-8 w-8 animate-spin rounded-full border-b-2 border-primary-600"></div>
      </div>

      <section
        v-else-if="error"
        class="rounded-lg border border-red-200 bg-red-50 p-6 text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-200"
      >
        <h1 class="text-lg font-semibold">{{ t('loadFailed') }}</h1>
        <p class="mt-2 text-sm">{{ error }}</p>
      </section>

      <template v-else-if="data">
        <div class="mb-6 flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white">
              {{ t('title') }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              {{ t('accountCount') }}: {{ data.accounts.length }}
            </p>
            <div class="mt-3 flex flex-wrap gap-x-6 gap-y-2 text-sm">
              <div>
                <span class="text-gray-500 dark:text-dark-400">{{ t('sumTodayStandardCost') }}:</span>
                <span class="ml-2 font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCost(sumTodayStandardCost) }}</span>
              </div>
              <div>
                <span class="text-gray-500 dark:text-dark-400">{{ t('sumTotalStandardCost') }}:</span>
                <span class="ml-2 font-semibold tabular-nums text-gray-900 dark:text-white">{{ formatCost(sumTotalStandardCost) }}</span>
              </div>
            </div>
          </div>
          <div class="flex items-center gap-3 text-xs text-gray-400 dark:text-dark-500">
            <span v-if="data.updated_at">{{ t('updatedAt') }} {{ formatTime(data.updated_at) }}</span>
            <button
              @click="fetchData"
              :disabled="loading"
              class="rounded-md bg-gray-100 px-3 py-1.5 text-gray-600 transition hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700"
            >
              {{ loading ? t('refreshing') : t('refresh') }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50">
                <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-dark-300">{{ t('name') }}</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">{{ t('status') }}</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">{{ t('plan') }}</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">{{ t('rpm') }}</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">{{ t('windowUsage') }}</th>
                <th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-dark-300">{{ t('todayStandardCost') }}</th>
                <th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-dark-300">{{ t('totalStandardCost') }}</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">{{ t('requests24h') }}</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">{{ t('expiresAt') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-gray-100 dark:divide-dark-800">
              <tr
                v-for="(account, idx) in data.accounts"
                :key="idx"
                class="transition hover:bg-gray-50 dark:hover:bg-dark-800/30"
              >
                <td class="px-4 py-3 font-medium text-gray-900 dark:text-white">
                  {{ account.name }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span :class="statusClass(account.status)" class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium">
                    {{ statusLabel(account.status) }}
                  </span>
                </td>
                <td class="px-4 py-3 text-center text-gray-600 dark:text-dark-300">
                  {{ account.plan_name || '-' }}
                </td>
                <td class="px-4 py-3 text-center tabular-nums text-gray-600 dark:text-dark-300">
                  {{ account.rpm > 0 ? account.rpm : '-' }}
                </td>
                <td class="px-4 py-3 text-center">
                  <div class="flex items-center justify-center gap-2">
                    <div class="h-2 w-16 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700">
                      <div
                        class="h-full rounded-full transition-all"
                        :class="usageColorClass(account.usage_percent)"
                        :style="{ width: Math.min(100, account.usage_percent) + '%' }"
                      ></div>
                    </div>
                    <span class="text-xs tabular-nums text-gray-500 dark:text-dark-500">
                      {{ account.usage_percent.toFixed(1) }}%
                    </span>
                  </div>
                </td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-dark-300">
                  {{ formatCost(account.today_standard_cost) }}
                </td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-600 dark:text-dark-300">
                  {{ formatCost(account.total_standard_cost) }}
                </td>
                <td class="px-4 py-3 text-center tabular-nums text-gray-600 dark:text-dark-300">
                  {{ account.last_24h_requests.toLocaleString() }}
                </td>
                <td class="px-4 py-3 text-center text-gray-600 dark:text-dark-300">
                  {{ formatExpires(account.expires_at) }}
                </td>
              </tr>
              <tr v-if="data.accounts.length === 0">
                <td colspan="9" class="px-4 py-8 text-center text-gray-400 dark:text-dark-500">
                  {{ t('noAccounts') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useAppStore } from '@/stores/app'
import { storeToRefs } from 'pinia'
import { getOpusMaxAccounts, type OpusMaxMonitorResponse } from '@/api/monitor'
import { extractApiErrorMessage } from '@/utils/apiError'

const appStore = useAppStore()
const { siteName, siteLogo } = storeToRefs(appStore)

const isZh = /^zh/i.test(navigator.language)

const messages: Record<string, Record<string, string>> = {
  zh: {
    loadFailed: '加载失败',
    title: 'OpusMax 账号监控',
    accountCount: '账号数',
    updatedAt: '更新于',
    refreshing: '刷新中...',
    refresh: '刷新',
    name: '名称',
    status: '状态',
    plan: '套餐',
    rpm: 'RPM',
    windowUsage: '窗口用量',
    requests24h: '24h 请求',
    expiresAt: '到期时间',
    noAccounts: '暂无 OpusMax 账号',
    active: '活跃',
    disabled: '停用',
    error: '异常',
    inactive: '未激活',
    loadError: '加载 OpusMax 监控数据失败',
    todayStandardCost: '今日标准计费',
    totalStandardCost: '总标准计费',
    sumTodayStandardCost: '总今日标准计费',
    sumTotalStandardCost: '总标准计费',
  },
  en: {
    loadFailed: 'Failed to load',
    title: 'OpusMax Account Monitor',
    accountCount: 'Accounts',
    updatedAt: 'Updated at',
    refreshing: 'Refreshing...',
    refresh: 'Refresh',
    name: 'Name',
    status: 'Status',
    plan: 'Plan',
    rpm: 'RPM',
    windowUsage: 'Window Usage',
    requests24h: '24h Requests',
    expiresAt: 'Expires At',
    noAccounts: 'No OpusMax accounts',
    active: 'Active',
    disabled: 'Disabled',
    error: 'Error',
    inactive: 'Inactive',
    loadError: 'Failed to load OpusMax monitor data',
    todayStandardCost: 'Today Standard Cost',
    totalStandardCost: 'Total Standard Cost',
    sumTodayStandardCost: 'Sum Today Cost',
    sumTotalStandardCost: 'Sum Total Cost',
  },
}

function t(key: string): string {
  const lang = isZh ? 'zh' : 'en'
  return messages[lang][key] ?? key
}

const data = ref<OpusMaxMonitorResponse | null>(null)
const loading = ref(false)
const error = ref('')

let refreshTimer: ReturnType<typeof setInterval> | null = null

const sumTodayStandardCost = computed(() =>
  data.value?.accounts.reduce((acc, a) => acc + (a.today_standard_cost || 0), 0) ?? 0
)
const sumTotalStandardCost = computed(() =>
  data.value?.accounts.reduce((acc, a) => acc + (a.total_standard_cost || 0), 0) ?? 0
)

function formatCost(v: number): string {
  if (v == null || isNaN(v)) return '$0.00'
  if (v >= 0.01 || v === 0) return '$' + v.toFixed(2)
  return '$' + v.toFixed(4)
}

function statusLabel(status: string): string {
  return t(status)
}

function statusClass(status: string): string {
  const classes: Record<string, string> = {
    active: 'bg-green-100 text-green-700 dark:bg-green-500/15 dark:text-green-400',
    disabled: 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-400',
    error: 'bg-red-100 text-red-700 dark:bg-red-500/15 dark:text-red-400',
    inactive: 'bg-yellow-100 text-yellow-700 dark:bg-yellow-500/15 dark:text-yellow-400',
  }
  return classes[status] ?? 'bg-gray-100 text-gray-600 dark:bg-dark-700 dark:text-dark-400'
}

function usageColorClass(percent: number): string {
  if (percent >= 90) return 'bg-red-500'
  if (percent >= 70) return 'bg-yellow-500'
  return 'bg-green-500'
}

function formatExpires(expiresAt: string): string {
  if (!expiresAt) return '-'
  try {
    const date = new Date(expiresAt)
    if (isNaN(date.getTime())) return expiresAt
    return date.toLocaleDateString(isZh ? 'zh-CN' : 'en-US', {
      year: 'numeric',
      month: 'short',
      day: 'numeric',
    })
  } catch {
    return expiresAt
  }
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString(isZh ? 'zh-CN' : 'en-US', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return iso
  }
}

async function fetchData() {
  loading.value = true
  try {
    data.value = await getOpusMaxAccounts()
    error.value = ''
  } catch (err: unknown) {
    error.value = extractApiErrorMessage(err, t('loadError'))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await appStore.fetchPublicSettings()
  await fetchData()
  refreshTimer = setInterval(fetchData, 60000) // Refresh every minute
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>