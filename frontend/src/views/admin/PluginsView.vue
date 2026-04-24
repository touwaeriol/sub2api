<template>
  <AppLayout>
    <TablePageLayout>
      <template #filters>
        <div class="flex items-center justify-between gap-4">
          <div>
            <h2 class="text-lg font-semibold text-gray-900 dark:text-gray-100">
              {{ t('admin.plugins.title') }}
            </h2>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.plugins.description') }}
            </p>
          </div>
          <button
            class="btn btn-secondary"
            :disabled="loading"
            @click="loadPlugins"
          >
            {{ t('common.refresh') }}
          </button>
        </div>
      </template>

      <div class="overflow-x-auto">
        <table class="min-w-full divide-y divide-gray-200 dark:divide-dark-600">
          <thead class="bg-gray-50 dark:bg-dark-700">
            <tr>
              <th class="table-th">{{ t('admin.plugins.columns.id') }}</th>
              <th class="table-th">{{ t('admin.plugins.columns.name') }}</th>
              <th class="table-th">{{ t('admin.plugins.columns.version') }}</th>
              <th class="table-th">{{ t('admin.plugins.columns.state') }}</th>
              <th class="table-th">{{ t('admin.plugins.columns.installedAt') }}</th>
              <th class="table-th text-right">
                {{ t('admin.plugins.columns.actions') }}
              </th>
            </tr>
          </thead>
          <tbody class="divide-y divide-gray-200 bg-white dark:divide-dark-600 dark:bg-dark-800">
            <tr v-if="loading">
              <td colspan="6" class="table-td text-center text-gray-500">
                {{ t('common.loading') }}
              </td>
            </tr>
            <tr v-else-if="!plugins.length">
              <td colspan="6" class="table-td text-center text-gray-500">
                {{ t('admin.plugins.empty') }}
              </td>
            </tr>
            <tr v-for="p in plugins" :key="p.id">
              <td class="table-td font-mono text-xs text-gray-600 dark:text-gray-300">
                {{ p.id }}
              </td>
              <td class="table-td">{{ p.name }}</td>
              <td class="table-td font-mono text-xs">{{ p.version }}</td>
              <td class="table-td">
                <StatusBadge :status="badgeStatus(p.state)" :label="stateLabel(p.state)" />
              </td>
              <td class="table-td text-xs text-gray-500">
                {{ formatDate(p.installedAt) }}
              </td>
              <td class="table-td">
                <div class="flex flex-wrap justify-end gap-2">
                  <button
                    v-if="p.state === 'installed' || p.state === 'disabled'"
                    class="btn btn-primary btn-sm"
                    :disabled="busyId === p.id"
                    @click="handleEnable(p)"
                  >
                    {{ t('admin.plugins.enable') }}
                  </button>
                  <button
                    v-if="p.state === 'enabled'"
                    class="btn btn-secondary btn-sm"
                    :disabled="busyId === p.id"
                    @click="handleDisable(p)"
                  >
                    {{ t('admin.plugins.disable') }}
                  </button>
                  <button
                    v-if="p.state === 'disabled'"
                    class="btn btn-secondary btn-sm"
                    :disabled="busyId === p.id"
                    @click="handleUninstall(p, false)"
                  >
                    {{ t('admin.plugins.uninstall') }}
                  </button>
                  <button
                    v-if="p.state === 'disabled'"
                    class="btn btn-danger btn-sm"
                    :disabled="busyId === p.id"
                    @click="confirmPurge(p)"
                  >
                    {{ t('admin.plugins.uninstallPurge') }}
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </TablePageLayout>

    <ConfirmDialog
      :show="purgeTarget !== null"
      :title="t('admin.plugins.confirmPurgeTitle')"
      :message="t('admin.plugins.confirmPurge', { name: purgeTarget?.name ?? '' })"
      :confirm-text="t('admin.plugins.uninstallPurge')"
      danger
      @confirm="performPurge"
      @cancel="purgeTarget = null"
    />
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import AppLayout from '@/components/layout/AppLayout.vue'
import TablePageLayout from '@/components/layout/TablePageLayout.vue'
import StatusBadge from '@/components/common/StatusBadge.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import { useAppStore } from '@/stores/app'
import { extractApiErrorMessage } from '@/utils/apiError'
import {
  listPlugins,
  enablePlugin,
  disablePlugin,
  uninstallPlugin,
  type PluginDTO,
  type PluginState
} from '@/api/admin/plugins'

const { t } = useI18n()
const appStore = useAppStore()

const plugins = ref<PluginDTO[]>([])
const loading = ref(false)
const busyId = ref<string | null>(null)
const purgeTarget = ref<PluginDTO | null>(null)

async function loadPlugins(): Promise<void> {
  loading.value = true
  try {
    plugins.value = await listPlugins()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t('admin.plugins.loadFailed')))
  } finally {
    loading.value = false
  }
}

async function withBusy(id: string, fn: () => Promise<void>, fallbackKey: string): Promise<void> {
  busyId.value = id
  try {
    await fn()
    await loadPlugins()
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t(fallbackKey)))
  } finally {
    busyId.value = null
  }
}

async function handleEnable(p: PluginDTO): Promise<void> {
  await withBusy(p.id, () => enablePlugin(p.id), 'admin.plugins.enableFailed')
}

async function handleDisable(p: PluginDTO): Promise<void> {
  await withBusy(p.id, () => disablePlugin(p.id), 'admin.plugins.disableFailed')
}

async function handleUninstall(p: PluginDTO, purge: boolean): Promise<void> {
  await withBusy(p.id, () => uninstallPlugin(p.id, purge), 'admin.plugins.uninstallFailed')
}

function confirmPurge(p: PluginDTO): void {
  purgeTarget.value = p
}

async function performPurge(): Promise<void> {
  const target = purgeTarget.value
  purgeTarget.value = null
  if (!target) return
  await handleUninstall(target, true)
}

// ==================== Presentation helpers ====================

/** Maps PluginState → StatusBadge variant (active/disabled/error/...). */
function badgeStatus(state: PluginState): string {
  switch (state) {
    case 'enabled':
      return 'active'
    case 'disabled':
      return 'disabled'
    case 'uninstalled':
      return 'error'
    case 'installed':
    default:
      return 'warning'
  }
}

function stateLabel(state: PluginState): string {
  return t(`admin.plugins.state.${state}`)
}

function formatDate(ts?: number): string {
  if (!ts) return '-'
  const d = new Date(ts * 1000)
  if (Number.isNaN(d.getTime())) return '-'
  return d.toLocaleString()
}

onMounted(loadPlugins)
</script>

<style scoped>
.table-th {
  @apply px-4 py-2 text-left text-xs font-medium uppercase tracking-wider text-gray-500 dark:text-gray-400;
}
.table-td {
  @apply px-4 py-3 text-sm text-gray-800 dark:text-gray-200;
}
.btn-sm {
  @apply px-3 py-1 text-xs;
}
</style>
