<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-semibold" style="color: var(--text-primary);">VIP Identities</h2>
    </div>

    <p class="text-sm mb-4" style="color: var(--text-muted);">
      VIP identities are protected by the <code class="font-mono">vip_protection</code> PBAC policy.
      Actions targeting these identities require elevated authorization.
    </p>

    <ErrorBanner :message="error" class="mb-4" />

    <!-- Add form -->
    <div
      class="rounded-xl p-4 mb-4"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <div class="flex flex-col sm:flex-row gap-3">
        <input
          v-model="newEmail"
          type="email"
          placeholder="email@example.com"
          class="flex-1 rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
        />
        <input
          v-model="newReason"
          type="text"
          placeholder="Reason (e.g., C-suite executive)"
          class="flex-1 rounded-lg px-3 py-2 text-sm"
          style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
        />
        <button
          class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
          :disabled="adding || !newEmail.trim() || !newReason.trim()"
          @click="addVIP"
        >
          <span v-if="adding">Adding...</span>
          <span v-else>Add</span>
        </button>
      </div>
    </div>

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div v-if="!loading && vips.length === 0" class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No VIP identities configured
    </div>

    <div class="space-y-2">
      <div
        v-for="vip in vips"
        :key="vip.id"
        class="rounded-xl p-3 flex items-center justify-between gap-3"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div>
          <div class="font-medium text-sm" style="color: var(--text-primary);">{{ vip.email }}</div>
          <div class="text-xs" style="color: var(--text-muted);">{{ vip.reason }}</div>
        </div>
        <button
          class="flex-shrink-0 text-xs text-red-500 hover:underline"
          @click="removeVIP(vip.id)"
        >
          Remove
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { listVIPIdentities, addVIPIdentity, removeVIPIdentity } from '../../api/admin'
import type { VIPIdentity } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const vips = ref<VIPIdentity[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const adding = ref(false)
const newEmail = ref('')
const newReason = ref('')

async function loadVIPs() {
  loading.value = true
  try {
    vips.value = await listVIPIdentities()
  } catch {
    error.value = 'Failed to load VIP identities'
  } finally {
    loading.value = false
  }
}

async function addVIP() {
  if (!newEmail.value.trim() || !newReason.value.trim()) return
  adding.value = true
  try {
    await addVIPIdentity(newEmail.value.trim(), newReason.value.trim())
    newEmail.value = ''
    newReason.value = ''
    await loadVIPs()
    toastStore.success('VIP identity added')
  } catch {
    toastStore.error('Failed to add VIP identity')
  } finally {
    adding.value = false
  }
}

async function removeVIP(id: string) {
  try {
    await removeVIPIdentity(id)
    await loadVIPs()
    toastStore.success('VIP identity removed')
  } catch {
    toastStore.error('Failed to remove VIP identity')
  }
}

onMounted(loadVIPs)
</script>
