<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-semibold" style="color: var(--text-primary);">API Tokens</h2>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="showCreate = true"
      >
        + Create Token
      </button>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <!-- New token reveal -->
    <div
      v-if="newToken"
      class="mb-4 p-4 rounded-xl bg-green-500/10 border border-green-500/30"
    >
      <div class="text-sm font-semibold text-green-600 mb-2">Token created — copy it now, it won't be shown again:</div>
      <code
        class="block px-3 py-2 rounded-lg text-xs font-mono break-all"
        style="background: var(--surface); color: var(--text-primary);"
      >
        {{ newToken }}
      </code>
      <button
        class="mt-2 text-xs text-green-600 hover:underline"
        @click="copyToken"
      >
        Copy to clipboard
      </button>
    </div>

    <div v-if="!loading && tokens.length === 0" class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No API tokens yet
    </div>

    <div class="space-y-2">
      <div
        v-for="token in tokens"
        :key="token.id"
        class="rounded-xl p-4 flex items-start justify-between gap-3"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div>
          <div class="font-medium text-sm" style="color: var(--text-primary);">{{ token.name }}</div>
          <div class="text-xs mt-0.5" style="color: var(--text-muted);">
            Scopes: {{ token.scopes.join(', ') || 'none' }}
          </div>
          <div class="text-xs" style="color: var(--text-muted);">
            Created {{ formatDate(token.created_at) }}
            <span v-if="token.expires_at"> · Expires {{ formatDate(token.expires_at) }}</span>
            <span v-if="token.last_used_at"> · Last used {{ formatDate(token.last_used_at) }}</span>
          </div>
        </div>
        <button
          class="flex-shrink-0 text-xs text-red-500 hover:underline"
          @click="revokeToken(token.id)"
        >
          Revoke
        </button>
      </div>
    </div>

    <!-- Create token modal -->
    <Teleport to="body">
      <div
        v-if="showCreate"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="showCreate = false"
      >
        <div
          class="w-full max-w-md rounded-2xl p-6 shadow-2xl"
          style="background: var(--card); border: 1px solid var(--border);"
        >
          <h3 class="text-lg font-semibold mb-4" style="color: var(--text-primary);">Create API Token</h3>

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Name *</label>
              <input
                v-model="createForm.name"
                type="text"
                placeholder="My integration token"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Scopes (comma-separated)</label>
              <input
                v-model="createForm.scopesRaw"
                type="text"
                placeholder="holds:read, actions:execute"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Expires at (optional)</label>
              <input
                v-model="createForm.expires_at"
                type="datetime-local"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>
          </div>

          <div class="flex gap-3 mt-6">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="showCreate = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
              :disabled="creating || !createForm.name.trim()"
              @click="createToken"
            >
              <span v-if="creating">Creating...</span>
              <span v-else>Create</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listTokens, createToken as apiCreateToken, deleteToken } from '../../api/tokens'
import type { Token } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const tokens = ref<Token[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showCreate = ref(false)
const creating = ref(false)
const newToken = ref<string | null>(null)

const createForm = reactive({
  name: '',
  scopesRaw: '',
  expires_at: '',
})

async function loadTokens() {
  loading.value = true
  try {
    tokens.value = await listTokens()
  } catch {
    error.value = 'Failed to load tokens'
  } finally {
    loading.value = false
  }
}

async function createToken() {
  if (!createForm.name.trim()) return
  creating.value = true
  try {
    const token = await apiCreateToken({
      name: createForm.name,
      scopes: createForm.scopesRaw.split(',').map((s) => s.trim()).filter(Boolean),
      expires_at: createForm.expires_at ? new Date(createForm.expires_at).toISOString() : undefined,
    })
    newToken.value = token.token ?? null
    showCreate.value = false
    createForm.name = ''
    createForm.scopesRaw = ''
    createForm.expires_at = ''
    await loadTokens()
    toastStore.success('Token created')
  } catch {
    toastStore.error('Failed to create token')
  } finally {
    creating.value = false
  }
}

async function revokeToken(id: string) {
  try {
    await deleteToken(id)
    await loadTokens()
    toastStore.success('Token revoked')
  } catch {
    toastStore.error('Failed to revoke token')
  }
}

async function copyToken() {
  if (newToken.value) {
    await navigator.clipboard.writeText(newToken.value)
    toastStore.success('Copied to clipboard')
  }
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleDateString()
}

onMounted(loadTokens)
</script>
