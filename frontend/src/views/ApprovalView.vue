<template>
  <div>
    <h1 class="text-2xl font-bold mb-6" style="color: var(--text-primary);">Approvals</h1>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading && approvals.length === 0" class="py-12 flex items-center justify-center">
      <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div v-else-if="approvals.length === 0 && !loading" class="py-12 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No approval requests
    </div>

    <div class="space-y-3">
      <div
        v-for="approval in approvals"
        :key="approval.id"
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-4">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap mb-1">
              <span class="font-semibold text-sm" style="color: var(--text-primary);">
                {{ approval.action_key }}
              </span>
              <StatusBadge :status="approval.status" />
            </div>
            <div class="text-xs" style="color: var(--text-muted);">
              Target: <strong style="color: var(--text-primary);">{{ approval.target_email }}</strong>
              · Instance: {{ approval.instance_id }}
            </div>
            <div class="text-xs mt-0.5" style="color: var(--text-muted);">
              Requested by {{ approval.requested_by }} · {{ formatDate(approval.requested_at) }}
            </div>
            <div v-if="approval.reason" class="text-xs mt-1 italic" style="color: var(--text-muted);">
              "{{ approval.reason }}"
            </div>
            <div v-if="approval.note && approval.status !== 'pending'" class="text-xs mt-1" style="color: var(--text-muted);">
              Note: {{ approval.note }}
            </div>
          </div>

          <div v-if="approval.status === 'pending'" class="flex gap-2 flex-shrink-0">
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-lg text-white bg-green-500 hover:bg-green-600"
              @click="startReview(approval, 'approve')"
            >
              Approve
            </button>
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-lg text-white bg-red-500 hover:bg-red-600"
              @click="startReview(approval, 'reject')"
            >
              Reject
            </button>
          </div>
        </div>
      </div>
    </div>

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
          <h2 class="text-lg font-semibold mb-2" style="color: var(--text-primary);">
            {{ reviewModal.action === 'approve' ? 'Approve Request' : 'Reject Request' }}
          </h2>
          <p class="text-sm mb-4" style="color: var(--text-muted);">
            {{ reviewModal.action === 'approve'
              ? `Approve "${reviewModal.approval?.action_key}" for ${reviewModal.approval?.target_email}?`
              : `Reject "${reviewModal.approval?.action_key}" for ${reviewModal.approval?.target_email}?`
            }}
          </p>

          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Note (optional)</label>
          <textarea
            v-model="reviewModal.note"
            rows="3"
            placeholder="Add a review note..."
            class="w-full rounded-lg px-3 py-2 text-sm resize-none mb-4"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          />

          <ErrorBanner :message="reviewError" class="mb-3" />

          <div class="flex gap-3">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="reviewModal.open = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white"
              :class="reviewModal.action === 'approve' ? 'bg-green-500 hover:bg-green-600' : 'bg-red-500 hover:bg-red-600'"
              :disabled="submitting"
              @click="submitReview"
            >
              <span v-if="submitting">Submitting...</span>
              <span v-else>{{ reviewModal.action === 'approve' ? 'Approve' : 'Reject' }}</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listApprovals, approveRequest, rejectRequest } from '../api/approvals'
import { ApiError } from '../api/client'
import type { Approval } from '../api/types'
import { useToastStore } from '../stores/toast'
import { useApprovalsStore } from '../stores/approvals'
import StatusBadge from '../components/StatusBadge.vue'
import ErrorBanner from '../components/ErrorBanner.vue'

const toastStore = useToastStore()
const approvalsStore = useApprovalsStore()
const approvals = ref<Approval[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const submitting = ref(false)
const reviewError = ref<string | null>(null)

const reviewModal = reactive<{
  open: boolean
  action: 'approve' | 'reject'
  approval: Approval | null
  note: string
}>({
  open: false,
  action: 'approve',
  approval: null,
  note: '',
})

function startReview(approval: Approval, action: 'approve' | 'reject') {
  reviewModal.approval = approval
  reviewModal.action = action
  reviewModal.note = ''
  reviewModal.open = true
  reviewError.value = null
}

async function submitReview() {
  if (!reviewModal.approval) return
  submitting.value = true
  reviewError.value = null
  try {
    if (reviewModal.action === 'approve') {
      await approveRequest(reviewModal.approval.id, reviewModal.note)
      toastStore.success('Request approved')
    } else {
      await rejectRequest(reviewModal.approval.id, reviewModal.note)
      toastStore.success('Request rejected')
    }
    reviewModal.open = false
    await loadApprovals()
    await approvalsStore.fetchApprovals()
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      reviewError.value = 'Insufficient permissions'
    } else {
      reviewError.value = 'Failed to submit review'
    }
  } finally {
    submitting.value = false
  }
}

async function loadApprovals() {
  loading.value = true
  error.value = null
  try {
    approvals.value = await listApprovals()
  } catch {
    error.value = 'Failed to load approvals'
  } finally {
    loading.value = false
  }
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleString()
}

onMounted(loadApprovals)
</script>
