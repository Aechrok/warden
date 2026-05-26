<template>
  <div>
    <div class="flex items-center justify-between mb-6">
      <h1 class="text-2xl font-bold" style="color: var(--text-primary);">Legal Holds</h1>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="showCreateModal = true"
      >
        + New Hold
      </button>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-12 flex items-center justify-center">
      <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div v-else-if="holds.length === 0" class="py-12 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No legal holds found
    </div>

    <div v-else class="space-y-3">
      <RouterLink
        v-for="hold in holds"
        :key="hold.id"
        :to="`/holds/${hold.id}`"
        class="block rounded-xl p-4 transition-all hover:shadow-md"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-3">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-semibold text-sm" style="color: var(--text-primary);">
                {{ hold.name }}
              </span>
              <StatusBadge :status="hold.status" />
            </div>
            <p v-if="hold.description" class="text-xs mt-1 truncate" style="color: var(--text-muted);">
              {{ hold.description }}
            </p>
            <div class="flex gap-3 text-xs mt-2" style="color: var(--text-muted);">
              <span>Placed by {{ hold.placed_by }}</span>
              <span>{{ formatDate(hold.created_at) }}</span>
              <span v-if="hold.expires_at">Expires {{ formatDate(hold.expires_at) }}</span>
            </div>
          </div>
          <span class="text-indigo-500 text-sm flex-shrink-0">→</span>
        </div>
      </RouterLink>
    </div>

    <!-- Create Hold Modal -->
    <Teleport to="body">
      <div
        v-if="showCreateModal"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="showCreateModal = false"
      >
        <div
          class="w-full max-w-lg rounded-2xl p-6 shadow-2xl"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <div class="flex items-center justify-between mb-4">
            <h2 class="text-lg font-semibold" style="color: var(--text-primary);">New Legal Hold</h2>
            <button @click="showCreateModal = false" style="color: var(--text-muted);">✕</button>
          </div>

          <div class="space-y-4">
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
                <option v-for="t in templates" :key="t.id" :value="t.id">{{ t.name }}</option>
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
                rows="3"
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

            <ErrorBanner :message="createError" />
          </div>

          <div class="flex gap-3 mt-6">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium"
              style="background: var(--border); color: var(--text-primary);"
              @click="showCreateModal = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
              :disabled="creating || !newHold.name.trim()"
              @click="createHold"
            >
              <span v-if="creating">Creating...</span>
              <span v-else>Create Hold</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { RouterLink } from 'vue-router'
import { listHolds, createHold as apiCreateHold } from '../api/holds'
import { listHoldTemplates } from '../api/admin'
import { ApiError } from '../api/client'
import type { Hold, HoldTemplate } from '../api/types'
import StatusBadge from '../components/StatusBadge.vue'
import ErrorBanner from '../components/ErrorBanner.vue'

const holds = ref<Hold[]>([])
const templates = ref<HoldTemplate[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showCreateModal = ref(false)
const creating = ref(false)
const createError = ref<string | null>(null)

const newHold = reactive({
  name: '',
  description: '',
  template_id: '',
  expires_at: '',
})

function applyTemplate() {
  const tpl = templates.value.find((t) => t.id === newHold.template_id)
  if (tpl) {
    newHold.name = tpl.name
    newHold.description = tpl.description
    if (tpl.default_expiry_days) {
      const d = new Date()
      d.setDate(d.getDate() + tpl.default_expiry_days)
      newHold.expires_at = d.toISOString().slice(0, 16)
    }
  }
}

async function createHold() {
  if (!newHold.name.trim()) return
  creating.value = true
  createError.value = null
  try {
    await apiCreateHold({
      name: newHold.name,
      description: newHold.description,
      template_id: newHold.template_id || undefined,
      expires_at: newHold.expires_at ? new Date(newHold.expires_at).toISOString() : undefined,
    })
    showCreateModal.value = false
    Object.assign(newHold, { name: '', description: '', template_id: '', expires_at: '' })
    await loadHolds()
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      createError.value = 'Insufficient permissions to create holds'
    } else {
      createError.value = 'Failed to create hold'
    }
  } finally {
    creating.value = false
  }
}

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
