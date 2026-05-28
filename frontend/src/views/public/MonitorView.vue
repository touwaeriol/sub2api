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
        <span class="text-sm text-gray-400 dark:text-dark-500">Monitor</span>
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
        <h1 class="text-lg font-semibold">加载失败</h1>
        <p class="mt-2 text-sm">{{ error }}</p>
      </section>

      <template v-else-if="data">
        <div class="mb-6 flex flex-wrap items-center justify-between gap-4">
          <div>
            <h1 class="text-xl font-bold text-gray-900 dark:text-white">
              {{ data.group_name }}
            </h1>
            <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">
              平台: {{ data.platform }} · 账号数: {{ data.accounts.length }}
            </p>
          </div>
          <div class="flex items-center gap-3 text-xs text-gray-400 dark:text-dark-500">
            <span v-if="data.updated_at">更新于 {{ formatTime(data.updated_at) }}</span>
            <button
              @click="fetchData"
              :disabled="loading"
              class="rounded-md bg-gray-100 px-3 py-1.5 text-gray-600 transition hover:bg-gray-200 dark:bg-dark-800 dark:text-dark-300 dark:hover:bg-dark-700"
            >
              {{ loading ? '刷新中...' : '刷新' }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto rounded-lg border border-gray-200 bg-white shadow-sm dark:border-dark-700 dark:bg-dark-900">
          <table class="w-full text-sm">
            <thead>
              <tr class="border-b border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50">
                <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-dark-300">名称</th>
                <th class="px-4 py-3 text-left font-medium text-gray-600 dark:text-dark-300">平台/类型</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">容量</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">状态</th>
                <th class="px-4 py-3 text-center font-medium text-gray-600 dark:text-dark-300">调度</th>
                <th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-dark-300">总标记计费</th>
                <th class="px-4 py-3 text-right font-medium text-gray-600 dark:text-dark-300">今日标准计费</th>
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
                <td class="px-4 py-3 text-gray-600 dark:text-dark-300">
                  {{ account.platform }}/{{ account.type }}
                </td>
                <td class="px-4 py-3 text-center text-gray-600 dark:text-dark-300">
                  {{ account.concurrency }}
                </td>
                <td class="px-4 py-3 text-center">
                  <span :class="statusClass(account.status)" class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium">
                    {{ statusLabel(account.status) }}
                  </span>
                </td>
                <td class="px-4 py-3 text-center">
                  <span
                    :class="account.schedulable
                      ? 'text-green-600 dark:text-green-400'
                      : 'text-gray-400 dark:text-dark-500'"
                  >
                    {{ account.schedulable ? '是' : '否' }}
                  </span>
                </td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-dark-200">
                  ${{ formatCost(account.total_marked_cost) }}
                </td>
                <td class="px-4 py-3 text-right tabular-nums text-gray-700 dark:text-dark-200">
                  ${{ formatCost(account.today_standard_cost) }}
                </td>
              </tr>
              <tr v-if="data.accounts.length === 0">
                <td colspan="7" class="px-4 py-8 text-center text-gray-400 dark:text-dark-500">
                  该分组下暂无账号
                </td>
              </tr>
            </tbody>
            <tfoot v-if="data.accounts.length > 0" class="border-t border-gray-200 bg-gray-50 dark:border-dark-700 dark:bg-dark-800/50">
              <tr>
                <td colspan="5" class="px-4 py-3 text-right font-medium text-gray-600 dark:text-dark-300">合计</td>
                <td class="px-4 py-3 text-right tabular-nums font-semibold text-gray-900 dark:text-white">
                  ${{ formatCost(totalMarkedCost) }}
                </td>
                <td class="px-4 py-3 text-right tabular-nums font-semibold text-gray-900 dark:text-white">
                  ${{ formatCost(totalTodayCost) }}
                </td>
              </tr>
            </tfoot>
          </table>
        </div>
      </template>
    </main>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { useAppStore } from '@/stores/app'
import { storeToRefs } from 'pinia'
import { getGroupAccountMonitor, type MonitorResponse } from '@/api/monitor'
import { extractApiErrorMessage } from '@/utils/apiError'

const route = useRoute()
const appStore = useAppStore()
const { siteName, siteLogo } = storeToRefs(appStore)

const data = ref<MonitorResponse | null>(null)
const loading = ref(false)
const error = ref('')

let refreshTimer: ReturnType<typeof setInterval> | null = null

const totalMarkedCost = computed(() =>
  data.value?.accounts.reduce((sum, a) => sum + a.total_marked_cost, 0) ?? 0
)
const totalTodayCost = computed(() =>
  data.value?.accounts.reduce((sum, a) => sum + a.today_standard_cost, 0) ?? 0
)

function statusLabel(status: string): string {
  const labels: Record<string, string> = {
    active: '活跃',
    disabled: '停用',
    error: '异常',
    inactive: '未激活',
  }
  return labels[status] ?? status
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

function formatCost(cost: number): string {
  return cost.toPrecision(10).replace(/\.?0+$/, '')
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString('zh-CN', {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    })
  } catch {
    return iso
  }
}

async function fetchData() {
  const platform = String(route.params.platform || '')
  const groupName = String(route.params.groupName || '')
  if (!platform || !groupName) {
    error.value = '缺少 platform 或 group_name 参数'
    return
  }

  loading.value = true
  try {
    data.value = await getGroupAccountMonitor(platform, groupName)
    error.value = ''
  } catch (err: unknown) {
    error.value = extractApiErrorMessage(err, '加载监控数据失败')
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await appStore.fetchPublicSettings()
  await fetchData()
  refreshTimer = setInterval(fetchData, 30000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
    refreshTimer = null
  }
})
</script>