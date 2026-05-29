<template>
  <div
    class="min-h-screen flex items-center justify-center p-4"
    style="background: var(--surface);"
  >
    <div
      class="w-full max-w-sm rounded-2xl p-8 shadow-2xl"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <!-- Logo -->
      <div class="flex flex-col items-center mb-8">
        <div class="w-16 h-16 rounded-2xl bg-indigo-500 flex items-center justify-center text-white font-bold text-3xl mb-4 shadow-lg">
          W
        </div>
        <h1 class="text-2xl font-bold" style="color: var(--text-primary);">Warden</h1>
        <p class="text-sm mt-1" style="color: var(--text-muted);">IT Command &amp; Control</p>
      </div>

      <!-- Error -->
      <div
        v-if="error"
        class="mb-4 px-4 py-3 rounded-lg text-sm bg-red-50 text-red-700 border border-red-200"
      >
        {{ error }}
      </div>

      <!-- Loading session check -->
      <div v-if="checking" class="flex items-center justify-center py-4">
        <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
        <span class="ml-3 text-sm" style="color: var(--text-muted);">Checking session…</span>
      </div>

      <!-- SSO-only enforced -->
      <template v-else-if="authConfig?.enforce_sso">
        <button
          class="w-full py-3 rounded-xl text-white font-semibold text-sm bg-indigo-500 hover:bg-indigo-600 active:bg-indigo-700 transition-colors shadow-md"
          @click="signInSSO"
        >
          Sign in with SSO
        </button>
        <p class="mt-4 text-center text-xs" style="color: var(--text-muted);">
          SSO login is enforced by your organization.
        </p>
      </template>

      <!-- Normal: password form + optional SSO -->
      <template v-else>
        <form @submit.prevent="signInLocal" class="space-y-3">
          <div>
            <label class="block text-xs font-medium mb-1" style="color: var(--text-muted);">Email</label>
            <input
              v-model="email"
              type="email"
              autocomplete="email"
              required
              placeholder="you@example.com"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
          </div>
          <div>
            <label class="block text-xs font-medium mb-1" style="color: var(--text-muted);">Password</label>
            <input
              v-model="password"
              type="password"
              autocomplete="current-password"
              required
              placeholder="••••••••"
              class="w-full rounded-lg px-3 py-2 text-sm"
              style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            />
          </div>
          <button
            type="submit"
            :disabled="loggingIn"
            class="w-full py-3 rounded-xl text-white font-semibold text-sm bg-indigo-500 hover:bg-indigo-600 active:bg-indigo-700 transition-colors shadow-md disabled:opacity-50"
          >
            {{ loggingIn ? 'Signing in…' : 'Sign in' }}
          </button>
        </form>

        <div v-if="authConfig?.sso_enabled" class="mt-4">
          <div class="relative flex items-center">
            <div class="flex-1 border-t" style="border-color: var(--border);"></div>
            <span class="px-3 text-xs" style="color: var(--text-muted);">or</span>
            <div class="flex-1 border-t" style="border-color: var(--border);"></div>
          </div>
          <button
            class="mt-3 w-full py-2.5 rounded-xl text-sm font-medium transition-colors"
            style="border: 1px solid var(--border); color: var(--text-primary);"
            @click="signInSSO"
          >
            Continue with SSO
          </button>
        </div>
      </template>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { initiateLogin, getAuthConfig, localLogin } from '../api/auth'
import type { AuthConfig } from '../api/types'

const router = useRouter()
const authStore = useAuthStore()
const checking = ref(true)
const loggingIn = ref(false)
const error = ref<string | null>(null)
const email = ref('')
const password = ref('')
const authConfig = ref<AuthConfig | null>(null)

onMounted(async () => {
  if (authStore.isAuthenticated) {
    router.replace('/')
    return
  }
  const [ok] = await Promise.all([
    authStore.fetchMe(),
    getAuthConfig().then((c) => { authConfig.value = c }).catch(() => {}),
  ])
  if (ok) {
    router.replace('/')
    return
  }
  checking.value = false
})

async function signInLocal() {
  error.value = null
  loggingIn.value = true
  try {
    await localLogin(email.value, password.value)
    const ok = await authStore.fetchMe()
    if (ok) router.replace('/')
    else error.value = 'Login failed'
  } catch {
    error.value = 'Invalid email or password'
  } finally {
    loggingIn.value = false
  }
}

function signInSSO() {
  initiateLogin()
}
</script>
