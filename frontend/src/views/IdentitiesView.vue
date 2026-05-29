<template>
  <div class="flex flex-col gap-6">
    <h1 class="text-2xl font-bold" style="color: var(--text-primary);">Identities</h1>

    <!-- Search bar -->
    <div class="rounded-xl p-4" style="background: var(--card); border: 1px solid var(--border);">
      <div class="flex flex-col sm:flex-row gap-3">
        <input
          v-model="searchEmail"
          type="email"
          placeholder="Search by email..."
          class="flex-1 rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          @keyup.enter="doSearch"
        />
        <select
          v-model="selectedInstance"
          class="rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
        >
          <option value="">All instances</option>
          <option v-for="inst in instances" :key="inst.id" :value="inst.id">
            {{ inst.name }}
          </option>
        </select>
        <button
          class="px-4 py-2 rounded-lg text-sm font-medium text-white transition-colors"
          style="background: var(--accent);"
          :disabled="loading || !searchEmail.trim()"
          @click="doSearch"
        >
          <span v-if="loading">Searching...</span>
          <span v-else>Search</span>
        </button>
      </div>
      <ErrorBanner :message="searchError" class="mt-3" />
    </div>

    <!-- Identity detail section -->
    <template v-if="searched && results.length > 0">
      <!-- Identity header -->
      <div
        class="rounded-xl p-4 flex items-center gap-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div
          class="w-11 h-11 rounded-full flex-shrink-0 flex items-center justify-center font-bold text-white text-base"
          style="background: linear-gradient(135deg, #a855f7, #6366f1);"
        >
          {{ initials }}
        </div>
        <div class="flex-1 min-w-0">
          <div class="flex items-center gap-2 flex-wrap">
            <span class="font-semibold text-sm" style="color: var(--text-primary); font-family: var(--font-mono, monospace);">
              {{ lastSearchEmail }}
            </span>
            <span
              v-if="onHold"
              class="text-xs px-2 py-0.5 rounded-full font-medium"
              style="background: rgba(168,85,247,.15); color: #a855f7; border: 1px solid rgba(168,85,247,.3);"
            >
              ⚖ Legal Hold
            </span>
          </div>
          <div class="text-xs mt-1" style="color: var(--text-muted);">
            Lookup across {{ results.length }} integration instance{{ results.length !== 1 ? 's' : '' }}
          </div>
        </div>
        <button
          class="px-3 py-1.5 rounded-lg text-sm transition-colors"
          style="background: var(--surface); border: 1px solid var(--border); color: var(--text-muted);"
          :disabled="loading"
          @click="doSearch"
        >
          Refresh
        </button>
      </div>

      <!-- Tabs -->
      <div style="border-bottom: 1px solid var(--border); margin-bottom: -8px;">
        <div class="flex">
          <button
            class="px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap"
            :style="activeTab === 'overview'
              ? 'color: var(--accent); border-color: var(--accent);'
              : 'color: var(--text-muted); border-color: transparent;'"
            @click="switchTab('overview')"
          >
            Overview
          </button>
          <button
            v-for="id in results"
            :key="id.instance_id"
            class="px-4 py-2.5 text-sm font-medium border-b-2 -mb-px transition-colors whitespace-nowrap"
            :style="activeTab === id.instance_id
              ? 'color: var(--accent); border-color: var(--accent);'
              : 'color: var(--text-muted); border-color: transparent;'"
            @click="switchTab(id.instance_id)"
          >
            {{ id.instance_name }}
          </button>
        </div>
      </div>

      <!-- Overview tab -->
      <div v-if="activeTab === 'overview'" class="identity-layout pt-2">
        <!-- Left: cross-system status grid -->
        <div class="flex flex-col gap-4">
          <p class="text-xs font-semibold uppercase tracking-wider" style="color: var(--text-muted); letter-spacing: 0.08em;">
            Cross-System Status
          </p>
          <div class="grid grid-cols-2 gap-3">
            <div
              v-for="id in results"
              :key="id.instance_id"
              class="rounded-lg p-4 flex flex-col gap-3"
              style="background: var(--surface); border: 1px solid var(--border);"
            >
              <div class="flex items-start justify-between gap-2">
                <div>
                  <div class="text-sm font-semibold" style="color: var(--text-primary); font-family: monospace;">
                    {{ id.instance_name }}
                  </div>
                  <div class="text-xs" style="color: var(--text-muted);">{{ pluginForInstance(id.instance_id) }}</div>
                </div>
                <span
                  v-if="statusFromData(id.data)"
                  class="text-xs px-2 py-0.5 rounded-full font-medium flex-shrink-0"
                  :style="statusBadgeStyle(id.data)"
                >
                  {{ statusFromData(id.data) }}
                </span>
              </div>
              <div style="border-top: 1px solid var(--border); padding-top: 10px; display: flex; flex-direction: column; gap: 4px;">
                <div
                  v-for="[k, v] in topFields(id.data)"
                  :key="k"
                  class="text-xs flex gap-1 flex-wrap"
                >
                  <span style="color: var(--text-muted);">{{ formatKey(k) }}:</span>
                  <span style="color: var(--text-primary);">{{ formatVal(v) }}</span>
                </div>
                <div v-if="id.fetched_at" class="text-xs" style="color: var(--text-muted); margin-top: 2px;">
                  Cached {{ formatAge(id.fetched_at) }}
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Right: action panel (grouped by instance) -->
        <div class="action-panel-container">
          <ActionPanel
            :groups="overviewGroups"
            :on-hold="onHold"
            :executing-action="executingAction"
            :executing-identity="executingIdentity"
            :param-values="paramValues"
            :exec-error="execError"
            :executing="executing"
            @select="startExecution"
            @clear="clearExecution"
            @submit="submitExecution"
            @param-change="onParamChange"
          />
        </div>
      </div>

      <!-- Per-instance tabs -->
      <template v-for="id in results" :key="id.instance_id">
        <div v-if="activeTab === id.instance_id" class="identity-layout pt-2">
          <!-- Left: data card -->
          <div
            class="rounded-xl p-5"
            style="background: var(--card); border: 1px solid var(--border);"
          >
            <p class="text-xs font-semibold uppercase tracking-wider mb-4" style="color: var(--text-muted); letter-spacing: 0.08em;">
              {{ id.instance_name }} · Identity Details
            </p>
            <div class="grid grid-cols-2 gap-x-8 gap-y-4">
              <div v-for="[k, v] in allFields(id.data)" :key="k" class="flex flex-col gap-0.5">
                <div
                  class="text-xs"
                  style="color: var(--text-muted); font-family: monospace; text-transform: uppercase; letter-spacing: 0.05em;"
                >
                  {{ formatKey(k) }}
                </div>
                <div class="text-sm" style="color: var(--text-primary);">{{ formatVal(v) }}</div>
              </div>
              <div v-if="id.fetched_at" class="flex flex-col gap-0.5">
                <div class="text-xs" style="color: var(--text-muted); font-family: monospace; text-transform: uppercase; letter-spacing: 0.05em;">
                  Cached At
                </div>
                <div class="text-sm" style="color: var(--text-primary);">{{ formatVal(id.fetched_at) }}</div>
              </div>
            </div>
          </div>

          <!-- Right: action panel (single instance) -->
          <div class="action-panel-container">
            <ActionPanel
              :groups="[{ identity: id, actions: actionsForInstance(id) }]"
              :on-hold="onHold"
              :executing-action="executingAction"
              :executing-identity="executingIdentity"
              :param-values="paramValues"
              :exec-error="execError"
              :executing="executing"
              :single-instance="true"
              @select="startExecution"
              @clear="clearExecution"
              @submit="submitExecution"
              @param-change="onParamChange"
            />
          </div>
        </div>
      </template>
    </template>

    <!-- No results -->
    <div
      v-else-if="searched && !loading"
      class="py-12 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);"
    >
      No identities found for "{{ lastSearchEmail }}"
    </div>

    <!-- Confirm destructive action -->
    <ConfirmModal
      v-model="showConfirm"
      title="Confirm Destructive Action"
      :message="`Are you sure you want to execute '${executingAction?.label}' on ${lastSearchEmail}? This action is destructive and may be irreversible.`"
      danger
      confirm-label="Execute"
      :loading="executing"
      @confirm="doExecute"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, reactive, onMounted } from 'vue'
import { searchIdentities } from '../api/identities'
import { listActions, executeAction } from '../api/actions'
import { listInstances } from '../api/admin'
import type { Identity, ActionDef, Instance } from '../api/types'
import { ApiError } from '../api/client'
import { useToastStore } from '../stores/toast'
import ErrorBanner from '../components/ErrorBanner.vue'
import ConfirmModal from '../components/ConfirmModal.vue'
import ActionPanel from '../components/IdentityActionPanel.vue'

// ── State ─────────────────────────────────────────────────────────────

const searchEmail = ref('')
const selectedInstance = ref('')
const results = ref<Identity[]>([])
const onHold = ref(false)
const actions = ref<ActionDef[]>([])
const instances = ref<Instance[]>([])
const loading = ref(false)
const searched = ref(false)
const lastSearchEmail = ref('')
const searchError = ref<string | null>(null)
const activeTab = ref('overview')

const executingAction = ref<ActionDef | null>(null)
const executingIdentity = ref<Identity | null>(null)
const paramValues = reactive<Record<string, unknown>>({})
const execError = ref<string | null>(null)
const executing = ref(false)
const showConfirm = ref(false)

const toastStore = useToastStore()

// ── Computed ──────────────────────────────────────────────────────────

const initials = computed(() => {
  const e = lastSearchEmail.value
  if (!e) return '?'
  const local = e.split('@')[0]
  const parts = local.split(/[._-]/).filter(Boolean)
  if (parts.length >= 2) return (parts[0][0] + parts[1][0]).toUpperCase()
  return local.slice(0, 2).toUpperCase()
})

const overviewGroups = computed(() =>
  results.value.map(id => ({ identity: id, actions: actionsForInstance(id) }))
)

// ── Helpers ───────────────────────────────────────────────────────────

function actionsForInstance(identity: Identity): ActionDef[] {
  const status = (identity.data?.status as string | undefined)?.toUpperCase()
  return actions.value.filter(a => {
    if (a.instance_id !== identity.instance_id) return false
    if (!a.applicable_states?.length) return true
    return !!status && a.applicable_states.includes(status)
  })
}

function pluginForInstance(instanceId: string): string {
  const a = actions.value.find(a => a.instance_id === instanceId)
  return a?.plugin ?? ''
}

const STATUS_KEYS = ['status', 'Status', 'active', 'isActive', 'suspended', 'isEnabled', 'enabled']

function statusFromData(data?: Record<string, unknown>): string | null {
  if (!data) return null
  for (const key of STATUS_KEYS) {
    if (key in data) {
      const v = data[key]
      if (typeof v === 'boolean') return v ? 'Active' : 'Inactive'
      if (typeof v === 'string') return v
    }
  }
  return null
}

function statusBadgeStyle(data?: Record<string, unknown>): string {
  const s = (statusFromData(data) ?? '').toLowerCase()
  if (['active', 'true', 'enabled'].includes(s))
    return 'background: rgba(34,197,94,.12); color: #22c55e; border: 1px solid rgba(34,197,94,.25);'
  if (['inactive', 'false', 'disabled', 'suspended'].includes(s))
    return 'background: rgba(239,68,68,.12); color: #ef4444; border: 1px solid rgba(239,68,68,.25);'
  return 'background: var(--surface2); color: var(--text-muted); border: 1px solid var(--border);'
}

function topFields(data?: Record<string, unknown>): Array<[string, unknown]> {
  if (!data) return []
  const skip = new Set([...STATUS_KEYS, 'id', 'userId', 'objectGUID', 'externalId', 'rawStatus'])
  return Object.entries(data)
    .filter(([k, v]) => !skip.has(k) && typeof v !== 'object' && !Array.isArray(v))
    .slice(0, 4)
}

function allFields(data?: Record<string, unknown>): Array<[string, unknown]> {
  if (!data) return []
  return Object.entries(data).filter(([, v]) => typeof v !== 'object' || v === null)
}

function formatKey(key: string): string {
  return key
    .replace(/([A-Z])/g, ' $1')
    .replace(/_/g, ' ')
    .replace(/^\w/, c => c.toUpperCase())
    .trim()
}

function formatVal(val: unknown): string {
  if (val === null || val === undefined) return '—'
  if (typeof val === 'boolean') return val ? 'Yes' : 'No'
  if (typeof val === 'string') {
    if (val === '') return '—'
    const d = new Date(val)
    if (!isNaN(d.getTime()) && /\d{4}-\d{2}-\d{2}T/.test(val))
      return d.toLocaleString('en-US', { dateStyle: 'medium', timeStyle: 'short' })
    return val
  }
  if (Array.isArray(val)) return val.length ? val.join(', ') : '—'
  return String(val)
}

function formatAge(ts: string): string {
  const diff = Date.now() - new Date(ts).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.floor(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.floor(hrs / 24)}d ago`
}

// ── Actions ───────────────────────────────────────────────────────────

function switchTab(tab: string) {
  activeTab.value = tab
  clearExecution()
}

async function doSearch() {
  if (!searchEmail.value.trim()) return
  loading.value = true
  searchError.value = null
  lastSearchEmail.value = searchEmail.value.trim()
  clearExecution()
  try {
    const res = await searchIdentities(searchEmail.value.trim(), selectedInstance.value || undefined)
    results.value = res.identities
    onHold.value = res.on_hold
    searched.value = true
    activeTab.value = 'overview'
  } catch (err) {
    if (err instanceof ApiError) {
      searchError.value = err.status === 403
        ? 'Insufficient permissions to search identities'
        : 'Search failed. Please try again.'
    } else {
      searchError.value = 'Unable to reach server'
    }
  } finally {
    loading.value = false
  }
}

function startExecution(action: ActionDef, identity: Identity) {
  executingAction.value = action
  executingIdentity.value = identity
  Object.keys(paramValues).forEach(k => delete paramValues[k])
  execError.value = null
}

function clearExecution() {
  executingAction.value = null
  executingIdentity.value = null
  execError.value = null
  showConfirm.value = false
}

function onParamChange(key: string, value: unknown) {
  paramValues[key] = value
}

function submitExecution() {
  if (!executingAction.value) return
  if (executingAction.value.destructive) {
    showConfirm.value = true
  } else {
    doExecute()
  }
}

async function doExecute() {
  if (!executingAction.value || !executingIdentity.value) return
  showConfirm.value = false
  executing.value = true
  execError.value = null
  try {
    const res = await executeAction({
      instance_id: executingIdentity.value.instance_id,
      action_key: executingAction.value.key,
      target_email: lastSearchEmail.value,
      params: Object.keys(paramValues).length > 0 ? { ...paramValues } : undefined,
    })
    if (res.status === 'pending_approval') {
      toastStore.info('Action submitted for approval')
    } else {
      toastStore.success(`"${executingAction.value.label}" completed`)
    }
    clearExecution()
  } catch (err) {
    if (err instanceof ApiError) {
      execError.value = err.status === 401
        ? 'Session expired. Please sign in again.'
        : err.status === 403
          ? 'Insufficient permissions to perform this action'
          : 'Action failed. Please try again.'
    } else {
      execError.value = 'Unable to reach server'
    }
  } finally {
    executing.value = false
  }
}

onMounted(async () => {
  try {
    ;[instances.value, actions.value] = await Promise.all([listInstances(), listActions()])
  } catch {
    // non-critical
  }
})
</script>

<style scoped>
.identity-layout {
  display: grid;
  grid-template-columns: 1fr 300px;
  gap: 20px;
  align-items: start;
}

.action-panel-container {
  position: sticky;
  top: 20px;
}

@media (max-width: 900px) {
  .identity-layout {
    grid-template-columns: 1fr;
  }

  .action-panel-container {
    position: static;
  }
}
</style>
