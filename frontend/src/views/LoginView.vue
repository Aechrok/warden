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

      <!-- Error state -->
      <div
        v-if="error"
        class="mb-4 px-4 py-3 rounded-lg text-sm bg-red-50 text-red-700 border border-red-200"
      >
        {{ error }}
      </div>

      <!-- Loading state -->
      <div v-if="checking" class="flex items-center justify-center py-4">
        <div class="animate-spin w-6 h-6 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
        <span class="ml-3 text-sm" style="color: var(--text-muted);">Checking session...</span>
      </div>

      <!-- Sign in button -->
      <button
        v-else
        class="w-full py-3 rounded-xl text-white font-semibold text-sm bg-indigo-500 hover:bg-indigo-600 active:bg-indigo-700 transition-colors shadow-md"
        @click="signIn"
      >
        Sign in with SSO
      </button>

      <p class="mt-6 text-center text-xs" style="color: var(--text-muted);">
        Secure access via your organization's identity provider
      </p>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { initiateLogin } from '../api/auth'

const router = useRouter()
const authStore = useAuthStore()
const checking = ref(true)
const error = ref<string | null>(null)

onMounted(async () => {
  // If already authenticated, redirect
  if (authStore.isAuthenticated) {
    router.replace('/')
    return
  }

  // Try to fetch session (handles post-OIDC-callback redirect)
  const ok = await authStore.fetchMe()
  if (ok) {
    router.replace('/')
  } else {
    checking.value = false
  }
})

function signIn() {
  initiateLogin()
}
</script>
