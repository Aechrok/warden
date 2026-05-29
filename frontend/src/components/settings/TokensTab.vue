<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-semibold" style="color: var(--text-primary);">API Tokens</h2>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="openCreate"
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
      >{{ newToken }}</code>
      <button class="mt-2 text-xs text-green-600 hover:underline" @click="copyToken">
        Copy to clipboard
      </button>
    </div>

    <div
      v-if="!loading && tokens.length === 0"
      class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);"
    >
      No API tokens yet
    </div>

    <div class="space-y-2">
      <div
        v-for="token in tokens"
        :key="token.id"
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-3 mb-2">
          <div>
            <div class="font-medium text-sm" style="color: var(--text-primary);">{{ token.name }}</div>
            <div class="text-xs mt-0.5" style="color: var(--text-muted);">
              Created {{ formatDate(token.created_at) }}
              <span v-if="token.expires_at"> · Expires {{ formatDate(token.expires_at) }}</span>
              <span v-if="token.last_used_at"> · Last used {{ formatDate(token.last_used_at) }}</span>
            </div>
          </div>
          <button class="flex-shrink-0 text-xs text-red-500 hover:underline" @click="revokeToken(token.id)">
            Revoke
          </button>
        </div>
        <!-- Scope chips -->
        <div class="flex flex-wrap gap-1">
          <span v-if="token.scopes.length === 0" class="text-xs" style="color: var(--text-muted);">no scopes</span>
          <span
            v-for="scope in token.scopes"
            :key="scope"
            class="text-xs font-mono px-2 py-0.5 rounded-full font-medium"
            :style="scopeStyle(scope)"
          >{{ scope }}</span>
        </div>
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
          class="w-full max-w-lg rounded-2xl p-6 shadow-2xl flex flex-col"
          style="background: var(--card); border: 1px solid var(--border); max-height: 90vh;"
        >
          <h3 class="text-lg font-semibold mb-4 flex-shrink-0" style="color: var(--text-primary);">Create API Token</h3>

          <div class="overflow-y-auto flex-1 space-y-4 pr-1">
            <!-- Name -->
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

            <!-- Permissions -->
            <div>
              <div class="flex items-center justify-between mb-2">
                <label class="text-sm font-medium" style="color: var(--text-primary);">Permissions</label>
                <div class="flex gap-3 text-xs" style="color: var(--text-muted);">
                  <button class="hover:underline" @click="selectAll">All</button>
                  <button class="hover:underline" @click="selectNone">None</button>
                  <span class="flex items-center gap-1">
                    <span class="inline-block w-2 h-2 rounded-full bg-blue-500"></span> Read
                  </span>
                  <span class="flex items-center gap-1">
                    <span class="inline-block w-2 h-2 rounded-full bg-orange-500"></span> Write
                  </span>
                  <span class="flex items-center gap-1">
                    <span class="inline-block w-2 h-2 rounded-full" style="background: var(--text-primary);"></span> Other
                  </span>
                </div>
              </div>
              <div
                class="rounded-xl p-3 grid grid-cols-2 gap-x-4 gap-y-2"
                style="background: var(--background); border: 1px solid var(--border);"
              >
                <label
                  v-for="perm in allPermissions"
                  :key="perm"
                  class="flex items-center gap-2 cursor-pointer select-none text-sm"
                  style="color: var(--text-primary);"
                >
                  <input
                    type="checkbox"
                    :checked="createForm.scopes.has(perm)"
                    class="rounded"
                    :style="checkboxStyle(perm)"
                    @change="toggleScope(perm)"
                  />
                  <span class="font-mono text-xs" :style="{ color: permColor(perm) }">{{ perm }}</span>
                </label>
              </div>
            </div>

            <!-- Expiry -->
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

          <div class="flex gap-3 mt-6 flex-shrink-0">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="showCreate = false"
            >
              Cancel
            </button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 disabled:opacity-50"
              :disabled="creating || !createForm.name.trim()"
              @click="createToken"
            >
              {{ creating ? 'Creating…' : 'Create' }}
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
import { listPermissions } from '../../api/admin'
import type { Token } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const tokens = ref<Token[]>([])
const allPermissions = ref<string[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showCreate = ref(false)
const creating = ref(false)
const newToken = ref<string | null>(null)

const createForm = reactive({
  name: '',
  scopes: new Set<string>(),
  expires_at: '',
})

function permCategory(perm: string): 'read' | 'write' | 'other' {
  if (perm.endsWith(':read')) return 'read'
  if (perm.endsWith(':write')) return 'write'
  return 'other'
}

function permColor(perm: string): string {
  const cat = permCategory(perm)
  if (cat === 'read') return '#3b82f6'
  if (cat === 'write') return '#f97316'
  return 'var(--text-primary)'
}

function scopeStyle(scope: string) {
  const cat = permCategory(scope)
  if (cat === 'read') return 'background: rgba(59,130,246,0.12); color: #3b82f6;'
  if (cat === 'write') return 'background: rgba(249,115,22,0.12); color: #f97316;'
  return 'background: var(--border); color: var(--text-primary);'
}

function checkboxStyle(perm: string) {
  const cat = permCategory(perm)
  if (cat === 'read') return 'accent-color: #3b82f6;'
  if (cat === 'write') return 'accent-color: #f97316;'
  return ''
}

function toggleScope(perm: string) {
  if (createForm.scopes.has(perm)) {
    createForm.scopes.delete(perm)
  } else {
    createForm.scopes.add(perm)
  }
}

function selectAll() {
  allPermissions.value.forEach((p) => createForm.scopes.add(p))
}

function selectNone() {
  createForm.scopes.clear()
}

function openCreate() {
  createForm.name = ''
  createForm.scopes.clear()
  createForm.expires_at = ''
  showCreate.value = true
}

async function loadTokens() {
  loading.value = true
  try {
    const all = await listTokens()
    tokens.value = all.filter((t) => !t.scopes.includes('scim:admin'))
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
      scopes: [...createForm.scopes],
      expires_at: createForm.expires_at ? new Date(createForm.expires_at).toISOString() : undefined,
    })
    newToken.value = token.token ?? null
    showCreate.value = false
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

onMounted(async () => {
  await Promise.all([
    loadTokens(),
    listPermissions().then((p) => { allPermissions.value = p.filter((perm) => perm !== 'scim:admin') }),
  ])
})
</script>
