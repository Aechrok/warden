<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-semibold" style="color: var(--text-primary);">Hold Templates</h2>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="openCreate"
      >
        + New Template
      </button>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div v-if="!loading && templates.length === 0" class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No hold templates defined
    </div>

    <div class="space-y-2">
      <div
        v-for="tpl in templates"
        :key="tpl.id"
        class="rounded-xl p-4 flex items-start justify-between gap-3"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div>
          <div class="font-medium text-sm" style="color: var(--text-primary);">{{ tpl.name }}</div>
          <div class="text-xs" style="color: var(--text-muted);">{{ tpl.description }}</div>
          <div class="text-xs mt-0.5" style="color: var(--text-muted);">
            Providers: <code class="font-mono">{{ tpl.provider_glob }}</code>
            <span v-if="tpl.default_expiry_days"> · {{ tpl.default_expiry_days }} day default expiry</span>
          </div>
        </div>
        <div class="flex gap-2 flex-shrink-0">
          <button
            class="text-xs px-3 py-1.5 rounded-lg"
            style="background: var(--border); color: var(--text-primary);"
            @click="openEdit(tpl)"
          >
            Edit
          </button>
          <button
            class="text-xs text-red-500 hover:underline"
            @click="deleteTpl(tpl.id)"
          >
            Delete
          </button>
        </div>
      </div>
    </div>

    <!-- Modal -->
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
            {{ editingId ? 'Edit Template' : 'New Template' }}
          </h3>

          <div class="space-y-3">
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Name *</label>
              <input v-model="form.name" type="text" class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Description</label>
              <textarea v-model="form.description" rows="2" class="w-full rounded-lg px-3 py-2 text-sm resize-none"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Provider Glob</label>
              <input v-model="form.provider_glob" type="text" placeholder="* or google_vault,m365"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);" />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Default Expiry (days)</label>
              <input v-model.number="form.default_expiry_days" type="number" min="1"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);" />
            </div>
          </div>

          <div class="flex gap-3 mt-6">
            <button class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="showModal = false">Cancel</button>
            <button class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
              :disabled="saving || !form.name" @click="save">
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
import { listHoldTemplates, createHoldTemplate, updateHoldTemplate, deleteHoldTemplate } from '../../api/admin'
import type { HoldTemplate } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const templates = ref<HoldTemplate[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showModal = ref(false)
const saving = ref(false)
const editingId = ref<string | null>(null)

const form = reactive({
  name: '',
  description: '',
  provider_glob: '*',
  default_expiry_days: 0,
})

function openCreate() {
  editingId.value = null
  form.name = ''
  form.description = ''
  form.provider_glob = '*'
  form.default_expiry_days = 0
  showModal.value = true
}

function openEdit(tpl: HoldTemplate) {
  editingId.value = tpl.id
  form.name = tpl.name
  form.description = tpl.description
  form.provider_glob = tpl.provider_glob
  form.default_expiry_days = tpl.default_expiry_days ?? 0
  showModal.value = true
}

async function save() {
  saving.value = true
  try {
    const data = {
      name: form.name,
      description: form.description,
      provider_glob: form.provider_glob,
      default_expiry_days: form.default_expiry_days || undefined,
    }
    if (editingId.value) {
      await updateHoldTemplate(editingId.value, data)
      toastStore.success('Template updated')
    } else {
      await createHoldTemplate(data)
      toastStore.success('Template created')
    }
    showModal.value = false
    await loadTemplates()
  } catch {
    toastStore.error('Failed to save template')
  } finally {
    saving.value = false
  }
}

async function deleteTpl(id: string) {
  try {
    await deleteHoldTemplate(id)
    await loadTemplates()
    toastStore.success('Template deleted')
  } catch {
    toastStore.error('Failed to delete template')
  }
}

async function loadTemplates() {
  loading.value = true
  try {
    templates.value = await listHoldTemplates()
  } catch {
    error.value = 'Failed to load templates'
  } finally {
    loading.value = false
  }
}

onMounted(loadTemplates)
</script>
