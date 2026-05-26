<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-semibold" style="color: var(--text-primary);">Integration Instances</h2>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="openCreate"
      >
        + Add Instance
      </button>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div v-if="!loading && instances.length === 0" class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No instances configured
    </div>

    <div class="space-y-2">
      <div
        v-for="inst in instances"
        :key="inst.id"
        class="rounded-xl p-4 flex items-center justify-between gap-3"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div>
          <div class="flex items-center gap-2">
            <span class="font-medium text-sm" style="color: var(--text-primary);">{{ inst.name }}</span>
            <span
              class="px-2 py-0.5 rounded text-xs"
              style="background: var(--nav-active-bg); color: var(--nav-active-text);"
            >
              {{ inst.plugin }}
            </span>
            <span
              class="px-2 py-0.5 rounded-full text-xs font-medium"
              :class="inst.enabled ? 'bg-green-100 text-green-700' : 'bg-slate-100 text-slate-500'"
            >
              {{ inst.enabled ? 'Enabled' : 'Disabled' }}
            </span>
          </div>
          <div class="text-xs mt-0.5" style="color: var(--text-muted);">ID: {{ inst.id }}</div>
        </div>
        <div class="flex gap-2">
          <button
            class="text-xs px-3 py-1.5 rounded-lg"
            style="background: var(--border); color: var(--text-primary);"
            @click="openEdit(inst)"
          >
            Edit
          </button>
          <button
            class="text-xs text-red-500 hover:underline"
            @click="deleteInst(inst.id)"
          >
            Delete
          </button>
        </div>
      </div>
    </div>

    <!-- Create/Edit Modal -->
    <Teleport to="body">
      <div
        v-if="showModal"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="showModal = false"
      >
        <div
          class="w-full max-w-md rounded-2xl p-6 shadow-2xl"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <h3 class="text-lg font-semibold mb-4" style="color: var(--text-primary);">
            {{ editingId ? 'Edit Instance' : 'Add Instance' }}
          </h3>

          <div class="space-y-3">
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Name *</label>
              <input
                v-model="form.name"
                type="text"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Plugin *</label>
              <select
                v-model="form.plugin"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              >
                <option value="">Select plugin...</option>
                <option v-for="p in plugins" :key="p" :value="p">{{ p }}</option>
              </select>
            </div>
            <div class="flex items-center gap-2">
              <input v-model="form.enabled" type="checkbox" id="enabled-check" />
              <label for="enabled-check" class="text-sm" style="color: var(--text-primary);">Enabled</label>
            </div>
          </div>

          <div class="flex gap-3 mt-6">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="showModal = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
              :disabled="saving || !form.name || !form.plugin"
              @click="save"
            >
              <span v-if="saving">Saving...</span>
              <span v-else>{{ editingId ? 'Update' : 'Create' }}</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listInstances, createInstance, updateInstance, deleteInstance } from '../../api/admin'
import type { Instance } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const instances = ref<Instance[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showModal = ref(false)
const saving = ref(false)
const editingId = ref<string | null>(null)

const plugins = ['okta', 'google', 'google_vault', 'slack', 'm365', 'intune', 'jamf']

const form = reactive({
  name: '',
  plugin: '',
  enabled: true,
})

function openCreate() {
  editingId.value = null
  form.name = ''
  form.plugin = ''
  form.enabled = true
  showModal.value = true
}

function openEdit(inst: Instance) {
  editingId.value = inst.id
  form.name = inst.name
  form.plugin = inst.plugin
  form.enabled = inst.enabled
  showModal.value = true
}

async function save() {
  saving.value = true
  try {
    if (editingId.value) {
      await updateInstance(editingId.value, { name: form.name, plugin: form.plugin, enabled: form.enabled })
      toastStore.success('Instance updated')
    } else {
      await createInstance({ name: form.name, plugin: form.plugin, enabled: form.enabled })
      toastStore.success('Instance created')
    }
    showModal.value = false
    await loadInstances()
  } catch {
    toastStore.error('Failed to save instance')
  } finally {
    saving.value = false
  }
}

async function deleteInst(id: string) {
  try {
    await deleteInstance(id)
    await loadInstances()
    toastStore.success('Instance deleted')
  } catch {
    toastStore.error('Failed to delete instance')
  }
}

async function loadInstances() {
  loading.value = true
  try {
    instances.value = await listInstances()
  } catch {
    error.value = 'Failed to load instances'
  } finally {
    loading.value = false
  }
}

onMounted(loadInstances)
</script>
