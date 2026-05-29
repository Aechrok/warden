<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold" style="color: var(--text-primary);">Audit Log</h1>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="exportEvents"
      >
        Export CSV
      </button>
    </div>

    <!-- Filters -->
    <div
      class="rounded-xl p-4 mb-6"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <div class="grid grid-cols-1 sm:grid-cols-2 md:grid-cols-4 gap-3">
        <div>
          <label class="block text-xs font-medium mb-1" style="color: var(--text-muted);">Type</label>
          <select
            v-model="filters.aggregate_type"
            class="w-full rounded-lg px-3 py-2 text-sm"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          >
            <option value="">All types</option>
            <option v-for="t in aggregateTypes" :key="t" :value="t">{{ t }}</option>
          </select>
        </div>
        <div>
          <label class="block text-xs font-medium mb-1" style="color: var(--text-muted);">Since</label>
          <input
            v-model="filters.since"
            type="datetime-local"
            class="w-full rounded-lg px-3 py-2 text-sm"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          />
        </div>
        <div>
          <label class="block text-xs font-medium mb-1" style="color: var(--text-muted);">Limit: {{ filters.limit }}</label>
          <input
            v-model.number="filters.limit"
            type="range"
            min="10"
            max="500"
            step="10"
            class="w-full"
          />
        </div>
        <div class="flex items-end">
          <button
            class="w-full px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
            :disabled="loading"
            @click="loadEvents"
          >
            <span v-if="loading">Loading...</span>
            <span v-else>Apply</span>
          </button>
        </div>
      </div>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <!-- Events table -->
    <div
      class="rounded-xl overflow-hidden"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <div v-if="loading" class="py-12 flex items-center justify-center">
        <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
      </div>

      <div v-else-if="events.length === 0" class="py-12 text-center text-sm" style="color: var(--text-muted);">
        No events found
      </div>

      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr style="border-bottom: 1px solid var(--border);">
              <th
                v-for="col in columns"
                :key="col.key"
                class="text-left px-4 py-3 text-xs font-semibold"
                style="color: var(--text-muted);"
              >
                {{ col.label }}
              </th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="event in events"
              :key="event.id"
              class="transition-colors hover:bg-[var(--surface)]"
              style="border-bottom: 1px solid var(--border);"
            >
              <td class="px-4 py-3 text-xs font-mono" style="color: var(--text-muted);">
                {{ formatDate(event.created_at) }}
              </td>
              <td class="px-4 py-3">
                <span
                  class="inline-block px-2 py-0.5 rounded text-xs font-medium"
                  style="background: var(--nav-active-bg); color: var(--nav-active-text);"
                >
                  {{ event.aggregate_type }}
                </span>
              </td>
              <td class="px-4 py-3 text-xs font-mono" style="color: var(--text-muted);">
                {{ event.aggregate_id.slice(0, 8) }}...
              </td>
              <td class="px-4 py-3 text-sm" style="color: var(--text-primary);">
                {{ event.type }}
              </td>
              <td class="px-4 py-3 text-xs" style="color: var(--text-muted);">
                {{ event.actor_display ?? '—' }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { queryAuditEvents, exportAudit } from '../api/audit'
import type { AuditEvent } from '../api/types'
import { ApiError } from '../api/client'
import ErrorBanner from '../components/ErrorBanner.vue'

const events = ref<AuditEvent[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

const filters = reactive({
  aggregate_type: '',
  since: '',
  limit: 100,
})

const aggregateTypes = ['hold', 'custodian', 'action', 'approval', 'breakglass', 'token', 'user']

const columns = [
  { key: 'created_at', label: 'Timestamp' },
  { key: 'aggregate_type', label: 'Type' },
  { key: 'aggregate_id', label: 'ID' },
  { key: 'type', label: 'Event' },
  { key: 'actor_display', label: 'Actor' },
]

async function loadEvents() {
  loading.value = true
  error.value = null
  try {
    events.value = await queryAuditEvents({
      aggregate_type: filters.aggregate_type || undefined,
      since: filters.since ? new Date(filters.since).toISOString() : undefined,
      limit: filters.limit,
    })
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      error.value = 'Insufficient permissions to view audit log'
    } else {
      error.value = 'Failed to load audit events'
    }
  } finally {
    loading.value = false
  }
}

function exportEvents() {
  exportAudit('csv')
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleString()
}

onMounted(loadEvents)
</script>
