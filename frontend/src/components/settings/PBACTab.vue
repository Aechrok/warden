<template>
  <div>
    <h2 class="font-semibold mb-4" style="color: var(--text-primary);">PBAC Policies</h2>
    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div class="space-y-3">
      <div
        v-for="policy in policies"
        :key="policy.name"
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-3 mb-2">
          <div>
            <div class="flex items-center gap-2">
              <span class="font-medium text-sm" style="color: var(--text-primary);">{{ policy.name }}</span>
              <span
                class="px-2 py-0.5 rounded-full text-xs font-medium"
                :class="policy.enabled ? 'bg-green-100 text-green-700' : 'bg-slate-100 text-slate-500'"
              >
                {{ policy.enabled ? 'Active' : 'Disabled' }}
              </span>
            </div>
            <div class="text-xs mt-0.5" style="color: var(--text-muted);">{{ policy.description }}</div>
          </div>

          <div class="flex gap-2 flex-shrink-0">
            <button
              class="text-xs px-3 py-1.5 rounded-lg"
              :class="policy.enabled
                ? 'bg-slate-100 text-slate-700 hover:bg-slate-200'
                : 'bg-green-100 text-green-700 hover:bg-green-200'"
              @click="togglePolicy(policy)"
            >
              {{ policy.enabled ? 'Disable' : 'Enable' }}
            </button>
          </div>
        </div>

        <!-- Config editor -->
        <div v-if="Object.keys(policy.config).length > 0">
          <details>
            <summary class="text-xs cursor-pointer" style="color: var(--text-muted);">Configuration</summary>
            <div class="mt-2">
              <textarea
                v-model="configDrafts[policy.name]"
                rows="4"
                class="w-full rounded-lg px-3 py-2 text-xs font-mono resize-none"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
              <button
                class="mt-1 text-xs px-3 py-1 rounded-lg text-white bg-indigo-500 hover:bg-indigo-600"
                @click="saveConfig(policy)"
              >
                Save Config
              </button>
            </div>
          </details>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listPBACPolicies, updatePBACPolicy } from '../../api/admin'
import type { PBACPolicy } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const policies = ref<PBACPolicy[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const configDrafts = reactive<Record<string, string>>({})

async function loadPolicies() {
  loading.value = true
  try {
    policies.value = await listPBACPolicies()
    policies.value.forEach((p) => {
      configDrafts[p.name] = JSON.stringify(p.config, null, 2)
    })
  } catch {
    error.value = 'Failed to load PBAC policies'
  } finally {
    loading.value = false
  }
}

async function togglePolicy(policy: PBACPolicy) {
  try {
    const updated = await updatePBACPolicy(policy.name, { enabled: !policy.enabled })
    const idx = policies.value.findIndex((p) => p.name === policy.name)
    if (idx !== -1) policies.value[idx] = updated
    toastStore.success(`Policy ${updated.enabled ? 'enabled' : 'disabled'}`)
  } catch {
    toastStore.error('Failed to update policy')
  }
}

async function saveConfig(policy: PBACPolicy) {
  try {
    const config = JSON.parse(configDrafts[policy.name])
    const updated = await updatePBACPolicy(policy.name, { config })
    const idx = policies.value.findIndex((p) => p.name === policy.name)
    if (idx !== -1) policies.value[idx] = updated
    toastStore.success('Config saved')
  } catch {
    toastStore.error('Invalid JSON or save failed')
  }
}

onMounted(loadPolicies)
</script>
