<template>
  <div>
    <h1 class="text-2xl font-bold mb-6" style="color: var(--text-primary);">Identities</h1>

    <!-- Search bar -->
    <div
      class="rounded-xl p-4 mb-6"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <div class="flex flex-col sm:flex-row gap-3">
        <input
          v-model="searchEmail"
          type="email"
          placeholder="Search by email..."
          class="flex-1 rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
          @keyup.enter="doSearch"
        />
        <select
          v-model="selectedInstance"
          class="rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
        >
          <option value="">All instances</option>
          <option v-for="inst in instances" :key="inst.id" :value="inst.id">
            {{ inst.name }}
          </option>
        </select>
        <button
          class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 transition-colors"
          :disabled="loading || !searchEmail.trim()"
          @click="doSearch"
        >
          <span v-if="loading">Searching...</span>
          <span v-else>Search</span>
        </button>
      </div>
      <ErrorBanner :message="searchError" class="mt-3" />
    </div>

    <!-- Results -->
    <div v-if="results.length > 0" class="space-y-3">
      <IdentityCard
        v-for="identity in results"
        :key="`${identity.instance_id}-${identity.id}`"
        :identity="identity"
        :actions="actionsForInstance(identity.instance_id)"
        @refresh="doSearch"
      />
    </div>

    <div
      v-else-if="searched && !loading"
      class="py-12 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);"
    >
      No identities found for "{{ lastSearchEmail }}"
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { searchIdentities } from '../api/identities'
import { listActions } from '../api/actions'
import { listInstances } from '../api/admin'
import type { Identity, ActionDef, Instance } from '../api/types'
import { ApiError } from '../api/client'
import ErrorBanner from '../components/ErrorBanner.vue'
import IdentityCard from '../components/IdentityCard.vue'

const searchEmail = ref('')
const selectedInstance = ref('')
const results = ref<Identity[]>([])
const actions = ref<ActionDef[]>([])
const instances = ref<Instance[]>([])
const loading = ref(false)
const searched = ref(false)
const lastSearchEmail = ref('')
const searchError = ref<string | null>(null)

function actionsForInstance(instanceId: string): ActionDef[] {
  return actions.value.filter((a) => a.instance_id === instanceId)
}

async function doSearch() {
  if (!searchEmail.value.trim()) return
  loading.value = true
  searchError.value = null
  lastSearchEmail.value = searchEmail.value.trim()
  try {
    results.value = await searchIdentities(
      searchEmail.value.trim(),
      selectedInstance.value || undefined
    )
    searched.value = true
  } catch (err) {
    if (err instanceof ApiError) {
      searchError.value = err.status === 403
        ? 'Insufficient permissions to search identities'
        : 'Search failed. Please try again.'
    } else {
      searchError.value = 'Unable to reach server'
    }
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  try {
    [instances.value, actions.value] = await Promise.all([listInstances(), listActions()])
  } catch {
    // non-critical
  }
})
</script>
