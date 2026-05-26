<template>
  <div>
    <h1 class="text-2xl font-bold mb-2" style="color: var(--text-primary);">Devices</h1>
    <p class="text-sm mb-6" style="color: var(--text-muted);">JAMF-managed devices. Search by user email to view their devices.</p>

    <!-- Search -->
    <div
      class="rounded-xl p-4 mb-6"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <div class="flex gap-3">
        <input
          v-model="searchEmail"
          type="email"
          placeholder="User email..."
          class="flex-1 rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          @keyup.enter="doSearch"
        />
        <button
          class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
          :disabled="loading || !searchEmail.trim()"
          @click="doSearch"
        >
          <span v-if="loading">Searching...</span>
          <span v-else>Search</span>
        </button>
      </div>
      <ErrorBanner :message="error" class="mt-3" />
    </div>

    <!-- Device list -->
    <div v-if="devices.length > 0" class="space-y-3">
      <div
        v-for="device in devices"
        :key="device.id"
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-3">
          <div>
            <div class="font-semibold text-sm" style="color: var(--text-primary);">
              {{ device.display_name || device.email }}
            </div>
            <div class="text-xs mt-1" style="color: var(--text-muted);">
              Provider: {{ device.provider }} · Status: {{ device.status }}
            </div>
          </div>
          <div class="flex gap-2 flex-shrink-0">
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-lg text-white bg-yellow-500 hover:bg-yellow-600"
              @click="triggerAction(device, 'lock')"
            >
              Lock
            </button>
            <button
              class="px-3 py-1.5 text-xs font-medium rounded-lg text-white bg-red-500 hover:bg-red-600"
              @click="triggerAction(device, 'wipe')"
            >
              Wipe
            </button>
          </div>
        </div>
      </div>
    </div>

    <div
      v-else-if="searched && !loading"
      class="py-12 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);"
    >
      No JAMF devices found for "{{ searchEmail }}"
    </div>

    <!-- Lock confirm -->
    <ConfirmModal
      v-model="showLockConfirm"
      title="Lock Device"
      message="This will lock the device remotely. The user will need a PIN to unlock it. Continue?"
      danger
      confirm-label="Lock Device"
      :loading="executing"
      @confirm="executeDeviceAction"
    />

    <!-- Wipe confirm -->
    <ConfirmModal
      v-model="showWipeConfirm"
      title="Wipe Device"
      message="WARNING: This will completely erase all data on the device. This is IRREVERSIBLE. Are you absolutely sure?"
      danger
      confirm-label="Wipe Device"
      :loading="executing"
      @confirm="executeDeviceAction"
    />
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { searchIdentities } from '../api/identities'
import { executeAction } from '../api/actions'
import { listActions } from '../api/actions'
import { ApiError } from '../api/client'
import type { Identity, ActionDef } from '../api/types'
import { useToastStore } from '../stores/toast'
import ErrorBanner from '../components/ErrorBanner.vue'
import ConfirmModal from '../components/ConfirmModal.vue'

const toastStore = useToastStore()
const searchEmail = ref('')
const devices = ref<Identity[]>([])
const actions = ref<ActionDef[]>([])
const loading = ref(false)
const searched = ref(false)
const error = ref<string | null>(null)
const executing = ref(false)
const showLockConfirm = ref(false)
const showWipeConfirm = ref(false)
const pendingAction = ref<{ device: Identity; actionKey: string } | null>(null)

async function doSearch() {
  if (!searchEmail.value.trim()) return
  loading.value = true
  error.value = null
  try {
    // Search all instances, then filter to JAMF results
    const all = await searchIdentities(searchEmail.value.trim())
    devices.value = all.filter((i) => i.provider?.toLowerCase().includes('jamf'))
    searched.value = true
    // Load actions for JAMF instances
    if (devices.value.length > 0) {
      const allActions = await listActions()
      actions.value = allActions.filter((a) =>
        devices.value.some((d) => d.instance_id === a.instance_id)
      )
    }
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      error.value = 'Insufficient permissions'
    } else {
      error.value = 'Unable to reach server'
    }
  } finally {
    loading.value = false
  }
}

function triggerAction(device: Identity, actionType: 'lock' | 'wipe') {
  // Find the action key for lock/wipe on this device's instance
  const matchKey = actions.value.find((a) =>
    a.instance_id === device.instance_id &&
    a.key.toLowerCase().includes(actionType)
  )?.key ?? actionType

  pendingAction.value = { device, actionKey: matchKey }
  if (actionType === 'lock') {
    showLockConfirm.value = true
  } else {
    showWipeConfirm.value = true
  }
}

async function executeDeviceAction() {
  if (!pendingAction.value) return
  executing.value = true
  try {
    const res = await executeAction({
      instance_id: pendingAction.value.device.instance_id,
      action_key: pendingAction.value.actionKey,
      target_email: pendingAction.value.device.email,
    })

    if (res.status === 'pending_approval') {
      toastStore.info('Action submitted for approval')
    } else {
      toastStore.success('Device action completed')
    }
  } catch (err) {
    if (err instanceof ApiError && err.status === 403) {
      toastStore.error('Insufficient permissions')
    } else {
      toastStore.error('Action failed. Please try again.')
    }
  } finally {
    executing.value = false
    showLockConfirm.value = false
    showWipeConfirm.value = false
    pendingAction.value = null
  }
}
</script>
