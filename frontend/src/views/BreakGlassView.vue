<template>
  <div>
    <h1 class="text-2xl font-bold mb-4" style="color: var(--text-primary);">Break Glass</h1>

    <!-- Alert banner -->
    <div class="flex items-start gap-3 px-4 py-3 rounded-xl mb-6 bg-red-500/10 border border-red-500/30">
      <span class="text-red-500 text-lg flex-shrink-0">⚠</span>
      <div>
        <div class="font-semibold text-sm text-red-500">Emergency Override</div>
        <div class="text-sm text-red-400 mt-0.5">
          This creates a permanent, immutable audit trail. Break-glass access bypasses all approval workflows and
          PBAC policies. Use only in genuine emergencies when normal procedures cannot be followed.
        </div>
      </div>
    </div>

    <!-- Invoke form -->
    <div
      class="rounded-xl p-5 mb-8"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <h2 class="font-semibold mb-4" style="color: var(--text-primary);">Invoke Break-Glass Action</h2>

      <div class="space-y-4">
        <div class="grid sm:grid-cols-2 gap-4">
          <div>
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Instance *</label>
            <select
              v-model="form.instance_id"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              @change="onInstanceChange"
            >
              <option value="">Select instance...</option>
              <option v-for="inst in instances" :key="inst.id" :value="inst.id">{{ inst.name }}</option>
            </select>
          </div>

          <div>
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Action *</label>
            <select
              v-model="form.action_key"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            >
              <option value="">Select action...</option>
              <option v-for="action in filteredActions" :key="action.key" :value="action.key">
                {{ action.label }}
              </option>
            </select>
          </div>
        </div>

        <div>
          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Target Email *</label>
          <input
            v-model="form.target_email"
            type="email"
            placeholder="target@example.com"
            class="w-full rounded-lg px-3 py-2 text-sm"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          />
        </div>

        <div>
          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">
            Reason * (minimum 20 characters)
          </label>
          <textarea
            v-model="form.reason"
            rows="4"
            placeholder="Describe the emergency situation and why break-glass access is required..."
            class="w-full rounded-lg px-3 py-2 text-sm resize-none"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          />
          <div class="text-xs mt-1" :style="form.reason.length < 20 ? 'color: var(--danger)' : 'color: var(--text-muted)'">
            {{ form.reason.length }}/20 characters minimum
          </div>
        </div>

        <ErrorBanner :message="formError" />

        <button
          class="w-full py-3 rounded-xl text-white font-semibold text-sm bg-red-500 hover:bg-red-600 active:bg-red-700 transition-colors"
          :disabled="!isFormValid || submitting"
          @click="showConfirm = true"
        >
          <span v-if="submitting">Invoking...</span>
          <span v-else>Invoke Break-Glass</span>
        </button>
      </div>
    </div>

    <!-- Incident list -->
    <div>
      <div class="flex items-center justify-between mb-4">
        <h2 class="font-semibold" style="color: var(--text-primary);">Incident History</h2>
      </div>

      <div v-if="incidentsLoading" class="py-8 flex items-center justify-center">
        <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
      </div>

      <div v-else-if="incidents.length === 0" class="py-8 text-center text-sm rounded-xl"
        style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
        No incidents recorded
      </div>

      <div class="space-y-3">
        <div
          v-for="incident in incidents"
          :key="incident.id"
          class="rounded-xl p-4"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <div class="flex items-start justify-between gap-3">
            <div class="flex-1 min-w-0">
              <div class="flex items-center gap-2 flex-wrap mb-1">
                <span class="font-semibold text-sm" style="color: var(--text-primary);">
                  {{ incident.action_key }}
                </span>
                <StatusBadge :status="incident.review_status" />
              </div>
              <div class="text-xs" style="color: var(--text-muted);">
                Target: <strong style="color: var(--text-primary);">{{ incident.target_email }}</strong>
                · Operator: {{ incident.operator_email }}
              </div>
              <div class="text-xs" style="color: var(--text-muted);">
                {{ formatDate(incident.invoked_at) }}
              </div>
              <div class="text-xs mt-1 italic" style="color: var(--text-muted);">
                "{{ incident.reason }}"
              </div>
              <div v-if="incident.review_note" class="text-xs mt-1" style="color: var(--text-muted);">
                Review note: {{ incident.review_note }}
              </div>
            </div>

            <button
              v-if="incident.review_status === 'pending'"
              class="flex-shrink-0 text-xs px-3 py-1.5 rounded-lg text-white bg-indigo-500 hover:bg-indigo-600"
              @click="openReview(incident)"
            >
              Review
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Confirm modal -->
    <ConfirmModal
      v-model="showConfirm"
      title="Confirm Break-Glass Action"
      :message="`You are about to execute '${form.action_key}' on ${form.target_email} via break-glass. This will create a permanent audit record and bypass all approval workflows. This cannot be undone.`"
      danger
      confirm-label="Invoke Break-Glass"
      :loading="submitting"
      @confirm="invokeBreakGlass"
    />

    <!-- Review modal -->
    <Teleport to="body">
      <div
        v-if="reviewModal.open"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="reviewModal.open = false"
      >
        <div
          class="w-full max-w-md rounded-2xl p-6 shadow-2xl"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <h2 class="text-lg font-semibold mb-4" style="color: var(--text-primary);">Review Incident</h2>
          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Review Note</label>
          <textarea
            v-model="reviewModal.note"
            rows="4"
            class="w-full rounded-lg px-3 py-2 text-sm resize-none mb-4"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          />
          <div class="flex gap-3">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="reviewModal.open = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
              :disabled="reviewModal.submitting"
              @click="submitReview"
            >
              Submit Review
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { invokeBreakGlass as apiInvoke, listIncidents, reviewIncident } from '../api/breakglass'
import { listActions } from '../api/actions'
import { listInstances } from '../api/admin'
import { ApiError } from '../api/client'
import type { ActionDef, Instance, Incident } from '../api/types'
import { useToastStore } from '../stores/toast'
import StatusBadge from '../components/StatusBadge.vue'
import ErrorBanner from '../components/ErrorBanner.vue'
import ConfirmModal from '../components/ConfirmModal.vue'

const toastStore = useToastStore()

const instances = ref<Instance[]>([])
const allActions = ref<ActionDef[]>([])
const incidents = ref<Incident[]>([])
const incidentsLoading = ref(false)
const submitting = ref(false)
const showConfirm = ref(false)
const formError = ref<string | null>(null)

const form = reactive({
  instance_id: '',
  action_key: '',
  target_email: '',
  reason: '',
})

const reviewModal = reactive({
  open: false,
  incidentId: '',
  note: '',
  submitting: false,
})

const filteredActions = computed(() =>
  allActions.value.filter((a) => !form.instance_id || a.instance_id === form.instance_id)
)

const isFormValid = computed(() =>
  form.instance_id &&
  form.action_key &&
  form.target_email &&
  form.reason.length >= 20
)

function onInstanceChange() {
  form.action_key = ''
}

async function invokeBreakGlass() {
  showConfirm.value = false
  submitting.value = true
  formError.value = null
  try {
    await apiInvoke({
      action_key: form.action_key,
      instance_id: form.instance_id,
      target_email: form.target_email,
      reason: form.reason,
    })
    toastStore.success('Break-glass action executed. Incident recorded.')
    form.action_key = ''
    form.target_email = ''
    form.reason = ''
    await loadIncidents()
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      formError.value = 'Insufficient permissions for break-glass'
    } else {
      formError.value = 'Failed to invoke break-glass action'
    }
  } finally {
    submitting.value = false
  }
}

function openReview(incident: Incident) {
  reviewModal.incidentId = incident.id
  reviewModal.note = ''
  reviewModal.open = true
}

async function submitReview() {
  reviewModal.submitting = true
  try {
    await reviewIncident(reviewModal.incidentId, reviewModal.note)
    reviewModal.open = false
    toastStore.success('Incident reviewed')
    await loadIncidents()
  } catch {
    toastStore.error('Failed to submit review')
  } finally {
    reviewModal.submitting = false
  }
}

async function loadIncidents() {
  incidentsLoading.value = true
  try {
    incidents.value = await listIncidents()
  } catch {
    // non-critical
  } finally {
    incidentsLoading.value = false
  }
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleString()
}

onMounted(async () => {
  const [insts, acts] = await Promise.allSettled([listInstances(), listActions()])
  if (insts.status === 'fulfilled') instances.value = insts.value
  if (acts.status === 'fulfilled') allActions.value = acts.value
  await loadIncidents()
})
</script>
