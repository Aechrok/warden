<template>
  <!-- Backdrop -->
  <div
    class="fixed inset-0 z-50"
    style="background: rgba(0,0,0,0.4);"
    @click="$emit('close')"
  />

  <!-- Slide-in panel -->
  <div
    class="fixed top-0 right-0 bottom-0 z-50 flex flex-col w-[520px] max-w-full shadow-2xl"
    style="background: var(--card); border-left: 1px solid var(--border);"
  >
    <!-- Header -->
    <div class="flex items-center justify-between px-5 py-4 flex-shrink-0" style="border-bottom: 1px solid var(--border);">
      <span class="font-semibold text-sm" style="color: var(--text-primary);">Debug Panel</span>
      <button
        class="text-base leading-none opacity-60 hover:opacity-100 transition-opacity"
        style="color: var(--text-muted);"
        @click="$emit('close')"
      >✕</button>
    </div>

    <!-- Stats summary -->
    <div class="grid grid-cols-2 gap-3 p-4 flex-shrink-0" style="border-bottom: 1px solid var(--border);">
      <div class="rounded-lg p-3" style="background: var(--surface); border: 1px solid var(--border);">
        <div class="text-xs mb-1" style="color: var(--text-muted);">API Calls</div>
        <div class="text-xl font-bold font-mono" style="color: var(--text-primary);">{{ debugStore.totalCalls }}</div>
      </div>
      <div class="rounded-lg p-3" style="background: var(--surface); border: 1px solid var(--border);">
        <div class="text-xs mb-1" style="color: var(--text-muted);">Avg API Time</div>
        <div class="text-xl font-bold font-mono" style="color: var(--text-primary);">{{ debugStore.avgMs }}<span class="text-sm font-normal ml-1" style="color: var(--text-muted);">ms</span></div>
      </div>
      <div class="rounded-lg p-3" style="background: var(--surface); border: 1px solid var(--border);">
        <div class="text-xs mb-1" style="color: var(--text-muted);">DB Queries</div>
        <div class="text-xl font-bold font-mono" style="color: var(--text-primary);">{{ dbStats ? dbStats.query_count : '—' }}</div>
      </div>
      <div class="rounded-lg p-3" style="background: var(--surface); border: 1px solid var(--border);">
        <div class="text-xs mb-1" style="color: var(--text-muted);">Avg DB Time</div>
        <div class="text-xl font-bold font-mono" style="color: var(--text-primary);">
          <template v-if="dbStats">{{ Math.round(dbStats.avg_ms * 10) / 10 }}<span class="text-sm font-normal ml-1" style="color: var(--text-muted);">ms</span></template>
          <template v-else>—</template>
        </div>
      </div>
    </div>

    <!-- Tabs -->
    <div class="flex gap-0 px-4 pt-3 flex-shrink-0" style="border-bottom: 1px solid var(--border);">
      <button
        v-for="t in tabs"
        :key="t.key"
        class="px-4 py-1.5 text-xs font-medium transition-colors"
        :style="activeTab === t.key
          ? 'color: var(--text-primary); border-bottom: 2px solid var(--nav-active-text); padding-bottom: 10px;'
          : 'color: var(--text-muted); border-bottom: 2px solid transparent; padding-bottom: 10px;'"
        @click="activeTab = t.key"
      >{{ t.label }}</button>
    </div>

    <!-- Tab content -->
    <div class="flex-1 overflow-y-auto">

      <!-- API Calls tab -->
      <template v-if="activeTab === 'api'">
        <div class="flex items-center justify-between px-4 py-2" style="border-bottom: 1px solid var(--border);">
          <span class="text-xs" style="color: var(--text-muted);">{{ debugStore.calls.length }} calls in buffer</span>
          <button class="text-xs hover:underline" style="color: var(--text-muted);" @click="debugStore.clear()">Clear</button>
        </div>
        <table class="w-full text-xs border-collapse">
          <thead>
            <tr style="background: var(--surface);">
              <th class="text-left px-4 py-2 font-medium w-14" style="color: var(--text-muted);">Method</th>
              <th class="text-left px-4 py-2 font-medium" style="color: var(--text-muted);">Path</th>
              <th class="text-right px-4 py-2 font-medium w-14" style="color: var(--text-muted);">Status</th>
              <th class="text-right px-4 py-2 font-medium w-16" style="color: var(--text-muted);">Time</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(call, i) in debugStore.calls" :key="i" style="border-top: 1px solid var(--border);">
              <td class="px-4 py-1.5 font-mono font-semibold" :style="methodColor(call.method)">{{ call.method }}</td>
              <td class="px-4 py-1.5 font-mono truncate max-w-0 w-full" style="color: var(--text-primary);" :title="call.path">{{ call.path }}</td>
              <td class="px-4 py-1.5 text-right font-mono" :style="statusColor(call.status)">{{ call.status }}</td>
              <td class="px-4 py-1.5 text-right font-mono whitespace-nowrap" style="color: var(--text-muted);">{{ call.durationMs }}ms</td>
            </tr>
            <tr v-if="debugStore.calls.length === 0">
              <td colspan="4" class="px-4 py-8 text-center" style="color: var(--text-muted);">No calls recorded yet</td>
            </tr>
          </tbody>
        </table>
      </template>

      <!-- DB Queries tab -->
      <template v-if="activeTab === 'db'">
        <div class="flex items-center justify-between px-4 py-2" style="border-bottom: 1px solid var(--border);">
          <span class="text-xs" style="color: var(--text-muted);">
            {{ dbStats ? dbStats.queries.length + ' queries in buffer' : 'Loading…' }}
          </span>
          <button class="text-xs hover:underline" style="color: var(--text-muted);" @click="fetchDbStats">Refresh</button>
        </div>
        <div v-if="dbError" class="mx-4 mt-3 px-4 py-3 text-xs rounded-lg" style="background: var(--surface); border: 1px solid var(--border); color: var(--text-muted);">
          {{ dbError }}
        </div>
        <table v-else-if="dbStats" class="w-full text-xs border-collapse">
          <thead>
            <tr style="background: var(--surface);">
              <th class="text-left px-4 py-2 font-medium" style="color: var(--text-muted);">SQL</th>
              <th class="text-right px-4 py-2 font-medium w-16" style="color: var(--text-muted);">Time</th>
              <th class="text-right px-4 py-2 font-medium w-10" style="color: var(--text-muted);">OK</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(q, i) in dbStats.queries" :key="i" style="border-top: 1px solid var(--border);">
              <td class="px-4 py-1.5 font-mono text-[11px] truncate max-w-0 w-full" style="color: var(--text-primary);" :title="q.sql">{{ q.sql }}</td>
              <td class="px-4 py-1.5 text-right font-mono whitespace-nowrap" style="color: var(--text-muted);">{{ Math.round(q.duration_ms * 10) / 10 }}ms</td>
              <td class="px-4 py-1.5 text-right font-semibold" :style="q.ok ? 'color: #22c55e;' : 'color: #ef4444;'">{{ q.ok ? '✓' : '✕' }}</td>
            </tr>
            <tr v-if="dbStats.queries.length === 0">
              <td colspan="3" class="px-4 py-8 text-center" style="color: var(--text-muted);">No queries recorded yet</td>
            </tr>
          </tbody>
        </table>
      </template>

      <!-- Plugins tab -->
      <template v-if="activeTab === 'plugins'">
        <div class="flex items-center justify-between px-4 py-2" style="border-bottom: 1px solid var(--border);">
          <span class="text-xs" style="color: var(--text-muted);">
            {{ plugins ? plugins.length + ' plugins registered' : 'Loading…' }}
          </span>
          <button class="text-xs hover:underline" style="color: var(--text-muted);" @click="fetchPlugins">Refresh</button>
        </div>
        <div v-if="pluginsError" class="mx-4 mt-3 px-4 py-3 text-xs rounded-lg" style="background: var(--surface); border: 1px solid var(--border); color: var(--text-muted);">
          {{ pluginsError }}
        </div>
        <div v-else-if="plugins" class="divide-y" style="border-color: var(--border);">
          <div
            v-for="p in plugins"
            :key="p.id"
            class="px-4 py-3"
          >
            <div class="flex items-center justify-between gap-2">
              <div>
                <span class="text-xs font-semibold font-mono" style="color: var(--text-primary);">{{ p.id }}</span>
                <span class="ml-2 text-xs" style="color: var(--text-muted);">{{ p.name }}</span>
              </div>
              <span class="text-[11px] px-2 py-0.5 rounded-full font-mono" style="background: var(--surface); color: var(--text-muted); border: 1px solid var(--border);">
                {{ p.schema.length }} field{{ p.schema.length === 1 ? '' : 's' }}
              </span>
            </div>
            <div v-if="p.schema.length > 0" class="mt-2 flex flex-wrap gap-1.5">
              <span
                v-for="f in p.schema"
                :key="f.key"
                class="text-[11px] px-1.5 py-0.5 rounded font-mono"
                :style="f.secret ? 'background: rgba(168,85,247,.1); color: #a855f7; border: 1px solid rgba(168,85,247,.25);' : 'background: var(--surface); color: var(--text-muted); border: 1px solid var(--border);'"
                :title="f.description"
              >{{ f.key }}{{ f.secret ? ' 🔒' : '' }}</span>
            </div>
          </div>
          <div v-if="plugins.length === 0" class="px-4 py-8 text-center text-xs" style="color: var(--text-muted);">
            No plugins registered
          </div>
        </div>
      </template>

    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useDebugStore } from '../stores/debug'
import { apiFetch } from '../api/client'

defineEmits<{ close: [] }>()

const debugStore = useDebugStore()
const activeTab = ref<'api' | 'db' | 'plugins'>('api')

const tabs = [
  { key: 'api' as const, label: 'API Calls' },
  { key: 'db' as const, label: 'DB Queries' },
  { key: 'plugins' as const, label: 'Plugins' },
]

interface DbStats {
  query_count: number
  avg_ms: number
  queries: { sql: string; duration_ms: number; ok: boolean; at: string }[]
}

interface PluginField {
  key: string
  description: string
  secret: boolean
}

interface PluginInfo {
  id: string
  name: string
  schema: PluginField[]
}

const dbStats = ref<DbStats | null>(null)
const dbError = ref<string | null>(null)
const plugins = ref<PluginInfo[] | null>(null)
const pluginsError = ref<string | null>(null)

async function fetchDbStats() {
  dbError.value = null
  try {
    dbStats.value = await apiFetch<DbStats>('/api/v1/internal/debug/stats')
  } catch {
    dbError.value = 'Debug endpoint unavailable. Start warden with WARDEN_DEBUG=true to enable DB query tracking.'
    dbStats.value = { query_count: 0, avg_ms: 0, queries: [] }
  }
}

async function fetchPlugins() {
  pluginsError.value = null
  try {
    const res = await apiFetch<{ plugins: PluginInfo[] }>('/api/v1/internal/admin/plugins?loaded_only=true')
    plugins.value = res.plugins ?? []
  } catch {
    pluginsError.value = 'Unable to load plugins. Requires instances:read permission.'
    plugins.value = []
  }
}

onMounted(() => {
  fetchDbStats()
  fetchPlugins()
})

function methodColor(method: string): string {
  const map: Record<string, string> = {
    GET: 'color: #22c55e;',
    POST: 'color: #3b82f6;',
    PUT: 'color: #f59e0b;',
    PATCH: 'color: #a78bfa;',
    DELETE: 'color: #ef4444;',
  }
  return map[method] ?? 'color: var(--text-muted);'
}

function statusColor(status: number): string {
  if (status < 300) return 'color: #22c55e;'
  if (status < 400) return 'color: #3b82f6;'
  if (status < 500) return 'color: #f59e0b;'
  return 'color: #ef4444;'
}
</script>
