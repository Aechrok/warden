<template>
  <div>
    <div class="flex items-center gap-3 mb-6">
      <RouterLink to="/holds" class="text-sm hover:underline" style="color: var(--text-muted);">
        ← Legal Holds
      </RouterLink>
    </div>

    <div v-if="loading" class="py-12 flex items-center justify-center">
      <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <template v-if="hold">
      <!-- Hold metadata card -->
      <div
        class="rounded-xl p-5 mb-6"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-4">
          <div>
            <div class="flex items-center gap-2 flex-wrap mb-1">
              <h1 class="text-xl font-bold" style="color: var(--text-primary);">{{ hold.name }}</h1>
              <StatusBadge :status="hold.status" />
            </div>
            <p v-if="hold.description" class="text-sm mb-3" style="color: var(--text-muted);">
              {{ hold.description }}
            </p>
            <div class="grid grid-cols-2 gap-x-6 gap-y-1 text-xs" style="color: var(--text-muted);">
              <span>Placed by <strong style="color: var(--text-primary);">{{ hold.placed_by }}</strong></span>
              <span>Created {{ formatDate(hold.created_at) }}</span>
              <span v-if="hold.expires_at">Expires {{ formatDate(hold.expires_at) }}</span>
              <span v-if="hold.released_at">Released {{ formatDate(hold.released_at) }}</span>
            </div>
          </div>

          <button
            v-if="hold.status === 'active'"
            class="flex-shrink-0 px-4 py-2 rounded-lg text-sm font-medium text-white bg-red-500 hover:bg-red-600"
            @click="showReleaseModal = true"
          >
            Release Hold
          </button>
        </div>
      </div>

      <!-- Custodians -->
      <div
        class="rounded-xl p-5 mb-6"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-center justify-between mb-4">
          <h2 class="font-semibold" style="color: var(--text-primary);">Custodians</h2>
          <span class="text-xs" style="color: var(--text-muted);">{{ custodians.length }} total</span>
        </div>

        <!-- Add custodian form -->
        <div v-if="hold.status === 'active'" class="flex gap-2 mb-4">
          <input
            v-model="newCustodianEmail"
            type="email"
            placeholder="Add by email..."
            class="flex-1 rounded-lg px-3 py-2 text-sm"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            @keyup.enter="addCustodian"
          />
          <button
            class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
            :disabled="addingCustodian || !newCustodianEmail.trim()"
            @click="addCustodian"
          >
            <span v-if="addingCustodian">Adding...</span>
            <span v-else>Add</span>
          </button>
        </div>

        <ErrorBanner :message="custodianError" class="mb-3" />

        <div v-if="custodians.length === 0" class="py-4 text-center text-sm" style="color: var(--text-muted);">
          No custodians added yet
        </div>

        <div class="space-y-2">
          <div
            v-for="custodian in custodians"
            :key="custodian.id"
            class="flex items-start justify-between gap-3 py-2"
            style="border-bottom: 1px solid var(--border);"
          >
            <div class="flex-1 min-w-0">
              <div class="text-sm font-medium" style="color: var(--text-primary);">
                {{ custodian.email }}
              </div>
              <div class="text-xs mt-0.5" style="color: var(--text-muted);">
                Added {{ formatDate(custodian.added_at) }}
              </div>
              <!-- Cascade states for this custodian -->
              <div class="flex flex-wrap gap-1 mt-1">
                <StatusBadge
                  v-for="cs in cascadeStatesFor(custodian.id)"
                  :key="cs.id"
                  :status="cs.status"
                  :label="`${cs.provider}: ${cs.status}`"
                />
              </div>
            </div>

            <button
              v-if="hold.status === 'active'"
              class="flex-shrink-0 text-xs text-red-500 hover:underline"
              @click="removeCustodian(custodian.id)"
            >
              Remove
            </button>
          </div>
        </div>
      </div>
    </template>

    <!-- Release Hold Modal -->
    <Teleport to="body">
      <div
        v-if="showReleaseModal"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="showReleaseModal = false"
      >
        <div
          class="w-full max-w-md rounded-2xl p-6 shadow-2xl"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <h2 class="text-lg font-semibold mb-2" style="color: var(--text-primary);">Release Legal Hold</h2>
          <p class="text-sm mb-4" style="color: var(--text-muted);">
            This will release all custodians and remove preservation on all providers. This cannot be undone.
          </p>

          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Reason *</label>
          <textarea
            v-model="releaseReason"
            rows="3"
            placeholder="Enter reason for release..."
            class="w-full rounded-lg px-3 py-2 text-sm resize-none mb-4"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          />

          <ErrorBanner :message="releaseError" class="mb-3" />

          <div class="flex gap-3">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="showReleaseModal = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-red-500 hover:bg-red-600"
              :disabled="releasing || !releaseReason.trim()"
              @click="releaseHold"
            >
              <span v-if="releasing">Releasing...</span>
              <span v-else>Release Hold</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRoute, RouterLink } from 'vue-router'
import { getHold, addCustodian as apiAddCustodian, removeCustodian as apiRemoveCustodian, releaseHold as apiReleaseHold } from '../api/holds'
import { ApiError } from '../api/client'
import type { Hold, Custodian, CascadeState } from '../api/types'
import { useToastStore } from '../stores/toast'
import StatusBadge from '../components/StatusBadge.vue'
import ErrorBanner from '../components/ErrorBanner.vue'

const route = useRoute()
const toastStore = useToastStore()
const holdId = route.params.id as string

const hold = ref<Hold | null>(null)
const custodians = ref<Custodian[]>([])
const cascadeStates = ref<CascadeState[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

const newCustodianEmail = ref('')
const addingCustodian = ref(false)
const custodianError = ref<string | null>(null)

const showReleaseModal = ref(false)
const releaseReason = ref('')
const releasing = ref(false)
const releaseError = ref<string | null>(null)

function cascadeStatesFor(custodianId: string): CascadeState[] {
  return cascadeStates.value.filter((cs) => cs.custodian_id === custodianId)
}

async function loadHold() {
  loading.value = true
  error.value = null
  try {
    const data = await getHold(holdId)
    hold.value = data.hold
    custodians.value = data.custodians
    cascadeStates.value = data.cascade_states
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      error.value = 'Insufficient permissions'
    } else {
      error.value = 'Failed to load hold details'
    }
  } finally {
    loading.value = false
  }
}

async function addCustodian() {
  if (!newCustodianEmail.value.trim()) return
  addingCustodian.value = true
  custodianError.value = null
  try {
    await apiAddCustodian(holdId, newCustodianEmail.value.trim())
    newCustodianEmail.value = ''
    await loadHold()
    toastStore.success('Custodian added')
  } catch (err) {
    custodianError.value = err instanceof ApiError && err.status === 403
      ? 'Insufficient permissions'
      : 'Failed to add custodian'
  } finally {
    addingCustodian.value = false
  }
}

async function removeCustodian(custodianId: string) {
  try {
    await apiRemoveCustodian(holdId, custodianId)
    await loadHold()
    toastStore.success('Custodian removed')
  } catch {
    toastStore.error('Failed to remove custodian')
  }
}

async function releaseHold() {
  if (!releaseReason.value.trim()) return
  releasing.value = true
  releaseError.value = null
  try {
    await apiReleaseHold(holdId, releaseReason.value)
    showReleaseModal.value = false
    await loadHold()
    toastStore.success('Hold released')
  } catch (err) {
    releaseError.value = err instanceof ApiError && err.status === 403
      ? 'Insufficient permissions'
      : 'Failed to release hold'
  } finally {
    releasing.value = false
  }
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleString()
}

onMounted(loadHold)
</script>
