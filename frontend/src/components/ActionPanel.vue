<template>
  <div>
    <div v-if="actions.length === 0" class="text-sm py-2" style="color: var(--text-muted);">
      No actions available for this instance.
    </div>

    <div class="space-y-2">
      <button
        v-for="action in actions"
        :key="action.key"
        class="w-full flex items-center gap-3 px-3 py-2.5 rounded-lg text-sm font-medium text-left transition-colors"
        :class="action.destructive
          ? 'border border-red-500/30 text-red-500 hover:bg-red-500/10'
          : 'border hover:opacity-80'"
        :style="action.destructive ? '' : 'border-color: var(--border); color: var(--text-primary);'"
        @click="selectAction(action)"
      >
        <span v-if="action.destructive" class="text-red-500">⚠</span>
        <span class="flex-1">{{ action.label }}</span>
        <span class="text-xs" style="color: var(--text-muted);">{{ action.key }}</span>
      </button>
    </div>

    <!-- Action execution form -->
    <div
      v-if="selectedAction"
      class="mt-4 p-4 rounded-lg"
      style="background: var(--surface); border: 1px solid var(--border);"
    >
      <div class="flex items-center justify-between mb-3">
        <h4 class="font-medium text-sm" style="color: var(--text-primary);">
          {{ selectedAction.label }}
        </h4>
        <button class="text-xs" style="color: var(--text-muted);" @click="selectedAction = null">✕</button>
      </div>

      <p v-if="selectedAction.description" class="text-xs mb-3" style="color: var(--text-muted);">
        {{ selectedAction.description }}
      </p>

      <!-- Extra params -->
      <div v-for="param in selectedAction.params" :key="param.key" class="mb-3">
        <label class="block text-xs font-medium mb-1" style="color: var(--text-primary);">
          {{ param.label }}{{ param.required ? ' *' : '' }}
        </label>
        <select
          v-if="param.type === 'select'"
          v-model="paramValues[param.key]"
          class="w-full rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
        >
          <option v-for="opt in param.options" :key="opt" :value="opt">{{ opt }}</option>
        </select>
        <input
          v-else
          v-model="paramValues[param.key]"
          :type="param.type === 'boolean' ? 'checkbox' : 'text'"
          class="w-full rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
        />
      </div>

      <ErrorBanner :message="execError" class="mb-3" />

      <button
        class="w-full py-2 rounded-lg text-sm font-medium text-white transition-colors"
        :class="selectedAction.destructive ? 'bg-red-500 hover:bg-red-600' : 'bg-indigo-500 hover:bg-indigo-600'"
        :disabled="executing"
        @click="submitAction"
      >
        <span v-if="executing">Executing...</span>
        <span v-else>{{ selectedAction.destructive ? 'Execute (Destructive)' : 'Execute' }}</span>
      </button>
    </div>

    <!-- Confirm destructive modal -->
    <ConfirmModal
      v-model="showConfirm"
      title="Confirm Destructive Action"
      :message="`Are you sure you want to execute '${selectedAction?.label}' on ${identity.email}? This action is destructive and may be irreversible.`"
      danger
      confirm-label="Execute"
      :loading="executing"
      @confirm="executeConfirmed"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive } from 'vue'
import { executeAction } from '../api/actions'
import { ApiError } from '../api/client'
import type { Identity, ActionDef } from '../api/types'
import { useToastStore } from '../stores/toast'
import ErrorBanner from './ErrorBanner.vue'
import ConfirmModal from './ConfirmModal.vue'

const props = defineProps<{
  identity: Identity
  actions: ActionDef[]
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'done'): void
}>()

const toastStore = useToastStore()
const selectedAction = ref<ActionDef | null>(null)
const paramValues = reactive<Record<string, unknown>>({})
const executing = ref(false)
const execError = ref<string | null>(null)
const showConfirm = ref(false)

function selectAction(action: ActionDef) {
  selectedAction.value = action
  Object.keys(paramValues).forEach((k) => delete paramValues[k])
  execError.value = null
}

function submitAction() {
  if (!selectedAction.value) return
  if (selectedAction.value.destructive) {
    showConfirm.value = true
  } else {
    doExecute()
  }
}

function executeConfirmed() {
  showConfirm.value = false
  doExecute()
}

async function doExecute() {
  if (!selectedAction.value) return
  executing.value = true
  execError.value = null

  try {
    const res = await executeAction({
      instance_id: props.identity.instance_id,
      action_key: selectedAction.value.key,
      target_email: props.identity.email,
      params: Object.keys(paramValues).length > 0 ? { ...paramValues } : undefined,
    })

    if (res.status === 'pending_approval') {
      toastStore.info('Action submitted for approval')
    } else {
      toastStore.success(`Action "${selectedAction.value.label}" completed successfully`)
    }

    selectedAction.value = null
    emit('done')
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.status === 401) {
        execError.value = 'Session expired. Please sign in again.'
      } else if (err.status === 403) {
        execError.value = 'Insufficient permissions to perform this action'
      } else {
        execError.value = 'Action failed. Please try again.'
      }
    } else {
      execError.value = 'Unable to reach server'
    }
  } finally {
    executing.value = false
  }
}
</script>
