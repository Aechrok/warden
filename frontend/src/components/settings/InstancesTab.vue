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
              {{ inst.plugin_id }}
            </span>
            <span
              class="px-2 py-0.5 rounded-full text-xs font-medium"
              :class="inst.is_active ? 'bg-green-100 text-green-700' : 'bg-slate-100 text-slate-500'"
            >
              {{ inst.is_active ? 'Enabled' : 'Disabled' }}
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
          class="w-full max-w-md rounded-2xl p-6 shadow-2xl overflow-y-auto"
          style="background: var(--card); border: 1px solid var(--border); max-height: 90vh;"
        >
          <h3 class="text-lg font-semibold mb-4" style="color: var(--text-primary);">
            {{ editingId ? 'Edit Instance' : 'Add Instance' }}
          </h3>

          <div class="space-y-3">
            <div>
              <label for="inst-name" class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Name *</label>
              <input
                id="inst-name"
                v-model="form.name"
                type="text"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>

            <div>
              <label for="inst-plugin" class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Plugin *</label>
              <select
                id="inst-plugin"
                v-model="form.plugin_id"
                :disabled="!!editingId"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              >
                <option value="">Select plugin...</option>
                <option v-for="p in pluginSchemas" :key="p.id" :value="p.id">{{ p.name }}</option>
              </select>
            </div>

            <div class="flex items-center gap-2">
              <input v-model="form.is_active" type="checkbox" id="inst-active" />
              <label for="inst-active" class="text-sm" style="color: var(--text-primary);">Enabled</label>
            </div>

            <!-- Dynamic credential fields -->
            <template v-if="currentFields.length > 0">
              <div class="pt-2 border-t" style="border-color: var(--border);">
                <div class="text-xs font-medium mb-2" style="color: var(--text-muted);">
                  Credentials{{ editingId ? ' (leave blank to keep existing)' : '' }}
                </div>
                <div class="space-y-3">
                  <div v-for="field in currentFields" :key="field.key">
                    <label :for="`cred-${field.key}`" class="block text-sm font-medium mb-1" style="color: var(--text-primary);">
                      {{ field.label }}<span v-if="field.required"> *</span>
                    </label>
                    <p v-if="field.description" class="text-xs mb-1" style="color: var(--text-muted);">{{ field.description }}</p>

                    <!-- bool field -->
                    <div v-if="field.type === 'bool'" class="flex items-center gap-2">
                      <input
                        :id="`cred-${field.key}`"
                        type="checkbox"
                        :checked="form.credentials[field.key] === 'true'"
                        @change="(e) => form.credentials[field.key] = (e.target as HTMLInputElement).checked ? 'true' : 'false'"
                      />
                      <label :for="`cred-${field.key}`" class="text-sm" style="color: var(--text-primary);">{{ field.label }}</label>
                    </div>

                    <!-- json field -->
                    <textarea
                      v-else-if="field.type === 'json'"
                      :id="`cred-${field.key}`"
                      v-model="form.credentials[field.key]"
                      rows="5"
                      :placeholder="`Paste ${field.label} JSON here`"
                      class="w-full rounded-lg px-3 py-2 text-sm resize-none font-mono"
                      style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                    />

                    <!-- string field (secret → password input) -->
                    <input
                      v-else
                      :id="`cred-${field.key}`"
                      v-model="form.credentials[field.key]"
                      :type="field.secret ? 'password' : 'text'"
                      :placeholder="field.label"
                      class="w-full rounded-lg px-3 py-2 text-sm"
                      style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                    />
                  </div>
                </div>
              </div>
            </template>
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
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 disabled:opacity-50 disabled:cursor-not-allowed"
              :disabled="saving || !form.name || !form.plugin_id"
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
import { ref, reactive, computed, watch, onMounted } from 'vue'
import { listInstances, createInstance, updateInstance, deleteInstance, listPlugins } from '../../api/admin'
import type { Instance, PluginSchema } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const instances = ref<Instance[]>([])
const pluginSchemas = ref<PluginSchema[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showModal = ref(false)
const saving = ref(false)
const editingId = ref<string | null>(null)

const form = reactive({
  name: '',
  plugin_id: '',
  is_active: true,
  credentials: {} as Record<string, string>,
})

const currentFields = computed(() => {
  const schema = pluginSchemas.value.find(p => p.id === form.plugin_id)
  return schema?.schema ?? []
})

watch(() => form.plugin_id, () => {
  form.credentials = {}
})

function openCreate() {
  editingId.value = null
  form.name = ''
  form.plugin_id = ''
  form.is_active = true
  form.credentials = {}
  showModal.value = true
}

function openEdit(inst: Instance) {
  editingId.value = inst.id
  form.name = inst.name
  form.plugin_id = inst.plugin_id
  form.is_active = inst.is_active
  form.credentials = {}
  showModal.value = true
}

function buildCredentials(): Record<string, string> | undefined {
  const filled = Object.fromEntries(
    Object.entries(form.credentials).filter(([, v]) => v !== '' && v !== undefined)
  )
  return Object.keys(filled).length > 0 ? filled : undefined
}

async function save() {
  saving.value = true
  try {
    const creds = buildCredentials()
    if (editingId.value) {
      await updateInstance(editingId.value, {
        name: form.name,
        is_active: form.is_active,
        ...(creds ? { credentials: creds } : {}),
      })
      toastStore.success('Instance updated')
    } else {
      await createInstance({
        name: form.name,
        plugin_id: form.plugin_id,
        ...(creds ? { credentials: creds } : {}),
      })
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

onMounted(async () => {
  const [, schemas] = await Promise.allSettled([loadInstances(), listPlugins()])
  if (schemas.status === 'fulfilled') pluginSchemas.value = schemas.value
})
</script>
