<template>
  <div>
    <!-- Header -->
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold" style="color: var(--text-primary);">Legal Holds</h1>
      <div class="flex items-center gap-3">
        <!-- View toggle -->
        <div class="flex rounded-lg overflow-hidden text-sm font-medium" style="border: 1px solid var(--border);">
          <button
            class="px-3 py-1.5 transition-colors"
            :style="view === 'holds'
              ? 'background: var(--nav-active-bg); color: var(--nav-active-text);'
              : 'background: var(--card); color: var(--text-muted);'"
            @click="view = 'holds'"
          >Holds</button>
          <button
            class="px-3 py-1.5 transition-colors"
            :style="view === 'custodians'
              ? 'background: var(--nav-active-bg); color: var(--nav-active-text);'
              : 'background: var(--card); color: var(--text-muted);'"
            @click="view = 'custodians'"
          >Custodians</button>
        </div>
        <button
          class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
          @click="openCreateModal"
        >
          + New Hold
        </button>
      </div>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-12 flex items-center justify-center">
      <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <!-- ── HOLDS VIEW ─────────────────────────────────────────────── -->
    <template v-else-if="view === 'holds'">
      <div v-if="holds.length === 0" class="py-12 text-center text-sm rounded-xl"
        style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
        No legal holds found
      </div>

      <div class="space-y-4">
        <div
          v-for="hold in holds"
          :key="hold.id"
          class="rounded-xl overflow-hidden"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <!-- Hold header -->
          <div class="p-4 flex items-start justify-between gap-4">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-1">
                <span class="font-semibold text-sm" style="color: var(--text-primary);">{{ hold.name }}</span>
                <StatusBadge :status="hold.status" />
              </div>
              <p v-if="hold.description" class="text-xs mb-1 truncate" style="color: var(--text-muted);">
                {{ hold.description }}
              </p>
              <div class="flex flex-wrap gap-3 text-xs" style="color: var(--text-muted);">
                <span>Placed by <strong style="color: var(--text-primary);">{{ hold.placed_by_name || 'Unknown' }}</strong></span>
                <span>{{ formatDate(hold.created_at) }}</span>
                <span v-if="hold.expires_at">Expires {{ formatDate(hold.expires_at) }}</span>
              </div>
            </div>
            <RouterLink
              :to="`/holds/${hold.id}`"
              class="text-xs px-3 py-1.5 rounded-lg flex-shrink-0"
              style="background: var(--border); color: var(--text-muted);"
            >Details →</RouterLink>
          </div>

          <!-- Custodians section -->
          <div style="border-top: 1px solid var(--border);">
            <div class="px-4 py-2 flex items-center justify-between">
              <span class="text-xs font-medium" style="color: var(--text-muted);">
                Custodians
                <span v-if="hold.custodians.length" class="ml-1">({{ hold.custodians.length }})</span>
              </span>
            </div>

            <!-- Custodian rows -->
            <div v-if="hold.custodians.length === 0" class="px-4 pb-3 text-xs" style="color: var(--text-muted);">
              No custodians added
            </div>
            <div v-else class="divide-y" style="border-color: var(--border);">
              <div
                v-for="custodian in hold.custodians"
                :key="custodian.id"
                class="px-4 py-2 flex items-center gap-3"
              >
                <span class="flex-1 text-xs font-mono" style="color: var(--text-primary);">{{ custodian.email }}</span>
                <!-- Sync status icon -->
                <component
                  :is="'span'"
                  class="flex items-center gap-1 text-xs flex-shrink-0"
                  :title="syncStatusLabel(custodian.email, hold)"
                >
                  <SyncStatusIcon :status="syncStatus(custodian.email, hold)" />
                </component>
                <button
                  v-if="hold.status === 'active'"
                  class="text-xs flex-shrink-0 hover:underline disabled:opacity-40 disabled:cursor-not-allowed"
                  style="color: var(--text-muted);"
                  :disabled="removingCustodians.has(custodian.id)"
                  @click="removeCustodian(hold.id, custodian.id)"
                >{{ removingCustodians.has(custodian.id) ? 'Removing…' : 'Remove' }}</button>
              </div>
            </div>

            <!-- Add custodian row (active holds only) -->
            <div v-if="hold.status === 'active'" class="px-4 py-2 flex gap-2" style="border-top: 1px solid var(--border);">
              <input
                v-model="holdDrafts[hold.id]"
                type="email"
                placeholder="Add custodian by email..."
                class="flex-1 rounded-lg px-3 py-1.5 text-xs"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                @keyup.enter="addCustodian(hold.id)"
              />
              <button
                class="px-3 py-1.5 rounded-lg text-xs font-medium text-white bg-indigo-500 hover:bg-indigo-600 flex-shrink-0"
                :disabled="!!holdAdding[hold.id]"
                @click="addCustodian(hold.id)"
              >
                <span v-if="holdAdding[hold.id]">Adding…</span>
                <span v-else>Add</span>
              </button>
            </div>
            <div v-if="holdErrors[hold.id]" class="px-4 pb-2 text-xs text-red-500">{{ holdErrors[hold.id] }}</div>
          </div>
        </div>
      </div>
    </template>

    <!-- ── CUSTODIANS VIEW ──────────────────────────────────────── -->
    <template v-else>
      <div v-if="uniqueCustodians.length === 0" class="py-12 text-center text-sm rounded-xl"
        style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
        No custodians on any hold
      </div>

      <div v-else class="rounded-xl overflow-hidden" style="background: var(--card); border: 1px solid var(--border);">
        <div
          v-for="(entry, idx) in uniqueCustodians"
          :key="entry.email"
          class="px-4 py-3 flex items-center gap-3"
          :style="idx < uniqueCustodians.length - 1 ? 'border-bottom: 1px solid var(--border);' : ''"
        >
          <span class="flex-1 text-sm font-mono" style="color: var(--text-primary);">{{ entry.email }}</span>
          <span class="text-xs px-2 py-0.5 rounded-full font-medium" style="background: rgba(99,102,241,0.1); color: #6366f1;">
            {{ entry.holdCount }} {{ entry.holdCount === 1 ? 'hold' : 'holds' }}
          </span>
          <div class="flex flex-wrap gap-1">
            <span
              v-for="hid in entry.holdIds"
              :key="hid"
              class="text-xs px-1.5 py-0.5 rounded font-medium truncate max-w-32"
              style="background: var(--border); color: var(--text-muted);"
              :title="holdName(hid)"
            >{{ holdName(hid) }}</span>
          </div>
        </div>
      </div>
    </template>

    <!-- ── CREATE HOLD MODAL ────────────────────────────────────── -->
    <Teleport to="body">
      <div
        v-if="showCreateModal"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="showCreateModal = false"
      >
        <div
          class="w-full max-w-lg rounded-2xl shadow-2xl flex flex-col"
          style="background: var(--card); border: 1px solid var(--border); max-height: 90vh;"
        >
          <div class="flex items-center justify-between p-6 pb-4 flex-shrink-0">
            <h2 class="text-lg font-semibold" style="color: var(--text-primary);">New Legal Hold</h2>
            <button @click="showCreateModal = false" style="color: var(--text-muted);">✕</button>
          </div>

          <div class="overflow-y-auto flex-1 px-6 space-y-4">
            <!-- Template selector -->
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">
                Template (optional)
              </label>
              <select
                v-model="newHold.template_id"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                @change="applyTemplate"
              >
                <option value="">From scratch</option>
                <option v-for="t in templates" :key="t.id" :value="t.id">
                  {{ t.name }}{{ t.is_default ? ' (Default)' : '' }}
                </option>
              </select>
            </div>

            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Name *</label>
              <input
                v-model="newHold.name"
                type="text"
                placeholder="Hold name..."
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>

            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Description</label>
              <textarea
                v-model="newHold.description"
                rows="2"
                class="w-full rounded-lg px-3 py-2 text-sm resize-none"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>

            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Expiry date (optional)</label>
              <input
                v-model="newHold.expires_at"
                type="datetime-local"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>

            <!-- Custodians section -->
            <div>
              <div class="flex items-center justify-between mb-1">
                <label class="text-sm font-medium" style="color: var(--text-primary);">
                  Custodians
                  <span v-if="custodianEmails.length" class="font-normal text-xs" style="color: var(--text-muted);">({{ custodianEmails.length }})</span>
                </label>
                <div class="flex gap-3 items-center">
                  <button
                    class="text-xs hover:underline"
                    style="color: var(--text-muted);"
                    @click="downloadCsvTemplate"
                  >
                    Download CSV template
                  </button>
                  <label class="text-xs cursor-pointer hover:underline" style="color: #6366f1;">
                    Upload CSV
                    <input
                      ref="csvInput"
                      type="file"
                      accept=".csv,text/csv"
                      class="hidden"
                      @change="handleCsvUpload"
                    />
                  </label>
                </div>
              </div>

              <div class="flex gap-2">
                <input
                  ref="emailInput"
                  v-model="emailDraft"
                  type="email"
                  placeholder="name@example.com"
                  class="flex-1 rounded-lg px-3 py-2 text-sm"
                  style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                  @keydown.enter.prevent="addEmail"
                  @keydown.","="onComma"
                />
                <button
                  class="px-3 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 flex-shrink-0"
                  @click="addEmail"
                >Add</button>
              </div>
              <div v-if="emailError" class="mt-1 text-xs text-red-500">{{ emailError }}</div>

              <div v-if="custodianEmails.length" class="mt-2 flex flex-wrap gap-1.5">
                <span
                  v-for="email in custodianEmails"
                  :key="email"
                  class="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full font-mono"
                  style="background: rgba(99,102,241,0.1); color: #6366f1;"
                >
                  {{ email }}
                  <button
                    class="ml-0.5 rounded-full hover:bg-indigo-200 flex-shrink-0 leading-none"
                    style="color: #6366f1;"
                    @click="removeEmail(email)"
                  >×</button>
                </span>
              </div>
            </div>

            <ErrorBanner :message="createError" />
          </div>

          <div class="flex gap-3 p-6 pt-4 flex-shrink-0">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium"
              style="background: var(--border); color: var(--text-primary);"
              @click="showCreateModal = false"
            >Cancel</button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
              :disabled="creating || !newHold.name.trim()"
              @click="createHold"
            >
              <span v-if="creating">Creating…</span>
              <span v-else>Create Hold</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { listHolds, createHold as apiCreateHold, addCustodian as apiAddCustodian, removeCustodian as apiRemoveCustodian } from '../api/holds'
import { listHoldTemplates } from '../api/admin'
import { ApiError } from '../api/client'
import type { HoldEnriched, CascadeStatus, HoldTemplate } from '../api/types'
import StatusBadge from '../components/StatusBadge.vue'
import SyncStatusIcon from '../components/SyncStatusIcon.vue'
import ErrorBanner from '../components/ErrorBanner.vue'

const view = ref<'holds' | 'custodians'>('holds')
const holds = ref<HoldEnriched[]>([])
const templates = ref<HoldTemplate[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// Per-hold add-custodian state
const holdDrafts = ref<Record<string, string>>({})
const holdAdding = ref<Record<string, boolean>>({})
const holdErrors = ref<Record<string, string | null>>({})
const removingCustodians = ref<Set<string>>(new Set())

// Create modal state
const showCreateModal = ref(false)
const creating = ref(false)
const createError = ref<string | null>(null)
const custodianEmails = ref<string[]>([])
const emailDraft = ref('')
const emailError = ref<string | null>(null)
const emailInput = ref<HTMLInputElement | null>(null)
const csvInput = ref<HTMLInputElement | null>(null)

const newHold = reactive({
  name: '',
  description: '',
  template_id: '',
  expires_at: '',
})

// ── Custodian view helpers ────────────────────────────────────────

const uniqueCustodians = computed(() => {
  const map = new Map<string, { email: string; holdCount: number; holdIds: string[] }>()
  for (const hold of holds.value) {
    for (const c of hold.custodians) {
      const entry = map.get(c.email)
      if (entry) {
        entry.holdCount++
        entry.holdIds.push(hold.id)
      } else {
        map.set(c.email, { email: c.email, holdCount: 1, holdIds: [hold.id] })
      }
    }
  }
  return [...map.values()].sort((a, b) => b.holdCount - a.holdCount)
})

function holdName(holdId: string): string {
  return holds.value.find((h) => h.id === holdId)?.name ?? holdId.slice(0, 8)
}

// ── Sync status ───────────────────────────────────────────────────

function syncStatus(email: string, hold: HoldEnriched): CascadeStatus | 'none' {
  const states = hold.cascade_states.filter((cs) => cs.custodian_email === email)
  if (states.length === 0) return 'none'
  if (states.some((cs) => cs.status === 'failed')) return 'failed'
  if (states.some((cs) => cs.status === 'partial')) return 'partial'
  if (states.some((cs) => cs.status === 'in_progress')) return 'in_progress'
  if (states.some((cs) => cs.status === 'pending')) return 'pending'
  if (states.every((cs) => cs.status === 'completed')) return 'completed'
  return 'pending'
}

function syncStatusLabel(email: string, hold: HoldEnriched): string {
  const s = syncStatus(email, hold)
  const labels: Record<string, string> = {
    none: 'Not yet synced',
    pending: 'Sync pending',
    in_progress: 'Syncing…',
    completed: 'Synced',
    partial: 'Partially synced',
    failed: 'Sync failed',
  }
  return labels[s] ?? s
}

// ── Add / remove custodian on existing holds ──────────────────────

async function addCustodian(holdId: string) {
  const email = (holdDrafts.value[holdId] ?? '').trim().toLowerCase()
  holdErrors.value[holdId] = null
  if (!email) return
  if (!email.includes('@')) {
    holdErrors.value[holdId] = 'Enter a valid email address'
    return
  }
  holdAdding.value[holdId] = true
  try {
    await apiAddCustodian(holdId, email)
    holdDrafts.value[holdId] = ''
    await loadHolds()
  } catch (err) {
    holdErrors.value[holdId] = err instanceof ApiError && err.status === 403
      ? 'Insufficient permissions'
      : 'Failed to add custodian'
  } finally {
    holdAdding.value[holdId] = false
  }
}

async function removeCustodian(holdId: string, custodianId: string) {
  if (removingCustodians.value.has(custodianId)) return
  removingCustodians.value = new Set(removingCustodians.value).add(custodianId)
  try {
    await apiRemoveCustodian(holdId, custodianId)
    await loadHolds()
  } catch {
    error.value = 'Failed to remove custodian'
  } finally {
    const next = new Set(removingCustodians.value)
    next.delete(custodianId)
    removingCustodians.value = next
  }
}

// ── Create modal helpers ──────────────────────────────────────────

function addEmail() {
  const value = emailDraft.value.trim().toLowerCase()
  emailError.value = null
  if (!value) return
  if (!value.includes('@')) {
    emailError.value = 'Enter a valid email address'
    return
  }
  if (custodianEmails.value.includes(value)) {
    emailError.value = 'Already added'
    return
  }
  custodianEmails.value.push(value)
  emailDraft.value = ''
  emailInput.value?.focus()
}

function onComma(e: KeyboardEvent) {
  e.preventDefault()
  addEmail()
}

function removeEmail(email: string) {
  custodianEmails.value = custodianEmails.value.filter((e) => e !== email)
}

function applyTemplate() {
  const tpl = templates.value.find((t) => t.id === newHold.template_id)
  if (tpl) {
    newHold.name = tpl.name
    newHold.description = tpl.description
    if (tpl.expiration_days) {
      const d = new Date()
      d.setDate(d.getDate() + tpl.expiration_days)
      newHold.expires_at = d.toISOString().slice(0, 16)
    }
  }
}

function downloadCsvTemplate() {
  const blob = new Blob(['emails\n'], { type: 'text/csv' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'custodians-template.csv'
  a.click()
  URL.revokeObjectURL(url)
}

function handleCsvUpload(event: Event) {
  const file = (event.target as HTMLInputElement).files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = (e) => {
    const text = e.target?.result as string
    const lines = text.split('\n').map((l) => l.trim()).filter(Boolean)
    const dataLines = lines[0]?.toLowerCase() === 'emails' ? lines.slice(1) : lines
    for (const line of dataLines) {
      const email = line.toLowerCase()
      if (email.includes('@') && !custodianEmails.value.includes(email)) {
        custodianEmails.value.push(email)
      }
    }
  }
  reader.readAsText(file)
  if (csvInput.value) csvInput.value.value = ''
}

function openCreateModal() {
  Object.assign(newHold, { name: '', description: '', template_id: '', expires_at: '' })
  custodianEmails.value = []
  emailDraft.value = ''
  emailError.value = null
  createError.value = null

  const defaultTpl = templates.value.find((t) => t.is_default)
  if (defaultTpl) {
    newHold.template_id = defaultTpl.id
    applyTemplate()
  }

  showCreateModal.value = true
}

async function createHold() {
  if (!newHold.name.trim()) return
  if (emailDraft.value.trim()) addEmail()
  creating.value = true
  createError.value = null
  try {
    await apiCreateHold({
      name: newHold.name,
      description: newHold.description,
      template_id: newHold.template_id || undefined,
      expires_at: newHold.expires_at ? new Date(newHold.expires_at).toISOString() : undefined,
      custodian_emails: custodianEmails.value.length > 0 ? custodianEmails.value : undefined,
    })
    showCreateModal.value = false
    await loadHolds()
  } catch (err) {
    createError.value = err instanceof ApiError && err.status === 403
      ? 'Insufficient permissions to create holds'
      : 'Failed to create hold'
  } finally {
    creating.value = false
  }
}

// ── Data loading ──────────────────────────────────────────────────

async function loadHolds() {
  loading.value = true
  error.value = null
  try {
    holds.value = await listHolds()
  } catch {
    error.value = 'Failed to load holds'
  } finally {
    loading.value = false
  }
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleDateString()
}

onMounted(async () => {
  await loadHolds()
  try {
    templates.value = await listHoldTemplates()
  } catch {
    // non-critical
  }
})
</script>
