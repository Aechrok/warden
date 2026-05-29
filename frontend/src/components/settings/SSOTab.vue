<template>
  <div class="space-y-8">
    <!-- SSO Configuration -->
    <section>
      <h2 class="text-base font-semibold mb-1" style="color: var(--text-primary);">SSO Configuration</h2>
      <p class="text-sm mb-4" style="color: var(--text-muted);">
        OIDC settings used by Warden's identity provider. Changes here do not hot-reload the server — restart after saving.
      </p>

      <ErrorBanner :message="configError" class="mb-4" />

      <div v-if="configLoading" class="py-6 flex items-center justify-center">
        <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
      </div>

      <div v-else class="rounded-xl p-5 space-y-5" style="background: var(--card); border: 1px solid var(--border);">
        <!-- Enable SSO toggle -->
        <label class="flex items-center gap-3 cursor-pointer select-none">
          <div
            class="relative w-10 h-6 rounded-full transition-colors"
            :style="form.sso_enabled ? 'background: #6366f1;' : 'background: var(--border);'"
            @click="form.sso_enabled = !form.sso_enabled"
          >
            <span
              class="absolute top-1 left-1 w-4 h-4 bg-white rounded-full shadow transition-transform"
              :style="form.sso_enabled ? 'transform: translateX(16px);' : ''"
            ></span>
          </div>
          <div>
            <div class="text-sm font-medium" style="color: var(--text-primary);">Enable SSO</div>
            <div class="text-xs" style="color: var(--text-muted);">Show "Continue with SSO" on the login page.</div>
          </div>
        </label>

        <!-- Enforce SSO -->
        <label
          class="flex items-start gap-3 cursor-pointer select-none"
          :class="!canEnforceSSO ? 'opacity-50 cursor-not-allowed' : ''"
        >
          <input
            type="checkbox"
            v-model="form.enforce_sso"
            :disabled="!canEnforceSSO"
            class="mt-0.5 rounded"
            style="accent-color: #6366f1;"
          />
          <div>
            <div class="text-sm font-medium" style="color: var(--text-primary);">Enforce SSO</div>
            <div class="text-xs" style="color: var(--text-muted);">
              Disable the password login form — users must authenticate via SSO.
              <span v-if="!canEnforceSSO" class="font-medium"> (You must be logged in via SSO to enable this.)</span>
            </div>
          </div>
        </label>

        <hr style="border-color: var(--border);" />

        <!-- OIDC fields -->
        <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">OIDC Issuer URL</label>
            <input
              v-model="form.oidc_issuer"
              type="url"
              placeholder="https://accounts.example.com"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Internal Issuer URL</label>
            <input
              v-model="form.oidc_internal_issuer"
              type="url"
              placeholder="http://dex:5556"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
            <p class="text-xs mt-1" style="color: var(--text-muted);">Optional. Used when the public issuer is unreachable from the backend.</p>
          </div>
          <div>
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Client ID</label>
            <input
              v-model="form.oidc_client_id"
              type="text"
              placeholder="warden"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
          </div>
          <div>
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">
              Client Secret
              <span v-if="config?.has_secret" class="ml-1 text-xs font-normal" style="color: var(--text-muted);">(stored — leave blank to keep)</span>
            </label>
            <input
              v-model="form.oidc_client_secret"
              type="password"
              :placeholder="config?.has_secret ? '••••••••' : 'Enter client secret'"
              autocomplete="new-password"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
          </div>
          <div class="sm:col-span-2">
            <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Redirect URL</label>
            <input
              v-model="form.oidc_redirect_url"
              type="url"
              placeholder="https://warden.example.com/auth/callback"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
          </div>
        </div>

        <div class="flex items-center gap-3 pt-1">
          <button
            :disabled="saving"
            class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 disabled:opacity-50"
            @click="saveConfig"
          >
            {{ saving ? 'Saving…' : 'Save' }}
          </button>
          <span v-if="config?.updated_at" class="text-xs" style="color: var(--text-muted);">
            Last updated {{ formatDate(config.updated_at) }}
          </span>
        </div>
      </div>
    </section>

    <!-- SCIM Provisioning -->
    <section>
      <h2 class="text-base font-semibold mb-1" style="color: var(--text-primary);">SCIM Provisioning</h2>
      <p class="text-sm mb-4" style="color: var(--text-muted);">
        Configure your identity provider to push groups and users to this endpoint. Generate a bearer token with the
        <code class="font-mono text-xs">scim:admin</code> scope and paste it into your IdP.
      </p>

      <div class="rounded-xl p-5 space-y-5" style="background: var(--card); border: 1px solid var(--border);">
        <!-- Endpoint -->
        <div>
          <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">SCIM Endpoint</label>
          <div class="flex items-center gap-2">
            <code
              class="flex-1 block px-3 py-2 rounded-lg text-xs font-mono break-all"
              style="background: var(--background); border: 1px solid var(--border); color: var(--text-primary);"
            >{{ scimEndpoint }}</code>
            <button
              class="text-xs px-3 py-2 rounded-lg flex-shrink-0"
              style="background: var(--border); color: var(--text-primary);"
              @click="copyEndpoint"
            >Copy</button>
          </div>
        </div>

        <!-- Generate token -->
        <div>
          <label class="block text-sm font-medium mb-2" style="color: var(--text-primary);">Generate SCIM Token</label>
          <div class="flex items-center gap-2">
            <input
              v-model="scimTokenName"
              type="text"
              placeholder="e.g. Okta SCIM"
              class="flex-1 rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
            <button
              :disabled="generatingToken || !scimTokenName.trim()"
              class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 disabled:opacity-50 flex-shrink-0"
              @click="generateSCIMToken"
            >
              {{ generatingToken ? 'Generating…' : 'Generate' }}
            </button>
          </div>
        </div>

        <!-- Newly generated token reveal -->
        <div v-if="generatedToken" class="p-4 rounded-xl bg-green-500/10 border border-green-500/30">
          <div class="text-sm font-semibold text-green-600 mb-2">Token created — copy it now, it won't be shown again:</div>
          <code
            class="block px-3 py-2 rounded-lg text-xs font-mono break-all"
            style="background: var(--surface); color: var(--text-primary);"
          >{{ generatedToken }}</code>
          <button class="mt-2 text-xs text-green-600 hover:underline" @click="copyGeneratedToken">Copy to clipboard</button>
        </div>

        <!-- Existing SCIM tokens -->
        <div v-if="scimTokens.length > 0">
          <div class="text-sm font-medium mb-2" style="color: var(--text-primary);">Active SCIM Tokens</div>
          <div class="space-y-2">
            <div
              v-for="token in scimTokens"
              :key="token.id"
              class="flex items-center justify-between gap-3 px-4 py-3 rounded-lg"
              style="background: var(--background); border: 1px solid var(--border);"
            >
              <div>
                <div class="text-sm font-medium" style="color: var(--text-primary);">{{ token.name }}</div>
                <div class="text-xs mt-0.5" style="color: var(--text-muted);">
                  Created {{ formatDate(token.created_at) }}
                  <span v-if="token.last_used_at"> · Last used {{ formatDate(token.last_used_at) }}</span>
                </div>
              </div>
              <button
                class="flex-shrink-0 text-xs text-red-500 hover:underline"
                @click="revokeScimToken(token.id)"
              >Revoke</button>
            </div>
          </div>
        </div>
        <div v-else-if="!tokensLoading" class="text-sm" style="color: var(--text-muted);">No SCIM tokens yet.</div>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, computed } from 'vue'
import { getSSOConfig, updateSSOConfig } from '../../api/admin'
import { createToken, listTokens, deleteToken } from '../../api/tokens'
import { useAuthStore } from '../../stores/auth'
import type { SSOConfig, Token } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const authStore = useAuthStore()

const config = ref<SSOConfig | null>(null)
const configLoading = ref(true)
const configError = ref<string | null>(null)
const saving = ref(false)

const scimTokens = ref<Token[]>([])
const tokensLoading = ref(false)
const scimTokenName = ref('')
const generatingToken = ref(false)
const generatedToken = ref<string | null>(null)

const form = reactive({
  oidc_issuer: '',
  oidc_internal_issuer: '',
  oidc_client_id: '',
  oidc_client_secret: '',
  oidc_redirect_url: '',
  sso_enabled: false,
  enforce_sso: false,
})

// Enforce SSO can only be toggled if the current user logged in via OIDC.
const canEnforceSSO = computed(() => authStore.user?.origin === 'oidc')

const scimEndpoint = computed(() => `${window.location.origin}/scim/v2/`)

async function loadConfig() {
  configLoading.value = true
  configError.value = null
  try {
    config.value = await getSSOConfig()
    form.oidc_issuer = config.value.oidc_issuer
    form.oidc_internal_issuer = config.value.oidc_internal_issuer
    form.oidc_client_id = config.value.oidc_client_id
    form.oidc_client_secret = ''
    form.oidc_redirect_url = config.value.oidc_redirect_url
    form.sso_enabled = config.value.sso_enabled
    form.enforce_sso = config.value.enforce_sso
  } catch {
    configError.value = 'Failed to load SSO configuration'
  } finally {
    configLoading.value = false
  }
}

async function loadSCIMTokens() {
  tokensLoading.value = true
  try {
    const all = await listTokens()
    scimTokens.value = all.filter((t) => t.scopes.includes('scim:admin'))
  } finally {
    tokensLoading.value = false
  }
}

async function saveConfig() {
  saving.value = true
  try {
    await updateSSOConfig({
      oidc_issuer: form.oidc_issuer,
      oidc_internal_issuer: form.oidc_internal_issuer,
      oidc_client_id: form.oidc_client_id,
      oidc_client_secret: form.oidc_client_secret || undefined,
      oidc_redirect_url: form.oidc_redirect_url,
      sso_enabled: form.sso_enabled,
      enforce_sso: form.enforce_sso,
    })
    form.oidc_client_secret = ''
    await loadConfig()
    toastStore.success('SSO configuration saved')
  } catch {
    toastStore.error('Failed to save SSO configuration')
  } finally {
    saving.value = false
  }
}

async function generateSCIMToken() {
  if (!scimTokenName.value.trim()) return
  generatingToken.value = true
  try {
    const token = await createToken({ name: scimTokenName.value.trim(), scopes: ['scim:admin'] })
    generatedToken.value = token.token ?? null
    scimTokenName.value = ''
    await loadSCIMTokens()
    toastStore.success('SCIM token created')
  } catch {
    toastStore.error('Failed to generate SCIM token')
  } finally {
    generatingToken.value = false
  }
}

async function revokeScimToken(id: string) {
  try {
    await deleteToken(id)
    await loadSCIMTokens()
    toastStore.success('Token revoked')
  } catch {
    toastStore.error('Failed to revoke token')
  }
}

async function copyEndpoint() {
  await navigator.clipboard.writeText(scimEndpoint.value)
  toastStore.success('Copied')
}

async function copyGeneratedToken() {
  if (generatedToken.value) {
    await navigator.clipboard.writeText(generatedToken.value)
    toastStore.success('Copied to clipboard')
  }
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleDateString()
}

onMounted(() => Promise.all([loadConfig(), loadSCIMTokens()]))
</script>
