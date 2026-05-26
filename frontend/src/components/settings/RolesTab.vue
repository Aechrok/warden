<template>
  <div>
    <h2 class="font-semibold mb-4" style="color: var(--text-primary);">Roles & Assignments</h2>
    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div class="space-y-4">
      <div
        v-for="role in roles"
        :key="role.name"
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between mb-2">
          <div>
            <div class="font-semibold text-sm" style="color: var(--text-primary);">{{ role.name }}</div>
            <div class="text-xs" style="color: var(--text-muted);">{{ role.description }}</div>
          </div>
        </div>

        <!-- Permissions -->
        <div class="flex flex-wrap gap-1 mb-3">
          <span
            v-for="perm in role.permissions"
            :key="perm"
            class="px-2 py-0.5 rounded text-xs"
            style="background: var(--nav-active-bg); color: var(--nav-active-text);"
          >
            {{ perm }}
          </span>
        </div>

        <!-- Users -->
        <div v-if="role.users && role.users.length > 0" class="mb-3">
          <div class="text-xs font-medium mb-1" style="color: var(--text-muted);">Assigned users:</div>
          <div class="flex flex-wrap gap-2">
            <div
              v-for="user in role.users"
              :key="user.id"
              class="flex items-center gap-1 px-2 py-1 rounded-lg text-xs"
              style="background: var(--surface); border: 1px solid var(--border);"
            >
              <span style="color: var(--text-primary);">{{ user.email }}</span>
              <button
                class="text-red-500 hover:text-red-600 ml-1"
                @click="revokeRole(role.name, user.id)"
              >
                ✕
              </button>
            </div>
          </div>
        </div>

        <!-- Assign user -->
        <div class="flex gap-2">
          <input
            v-model="assignForms[role.name]"
            type="text"
            placeholder="User ID to assign..."
            class="flex-1 rounded-lg px-3 py-1.5 text-xs"
            style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
            @keyup.enter="assignRole(role.name)"
          />
          <button
            class="px-3 py-1.5 text-xs font-medium rounded-lg text-white bg-indigo-500 hover:bg-indigo-600"
            :disabled="!assignForms[role.name]"
            @click="assignRole(role.name)"
          >
            Assign
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listRoles, assignRole as apiAssignRole, revokeRole as apiRevokeRole } from '../../api/admin'
import type { Role } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const roles = ref<Role[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const assignForms = reactive<Record<string, string>>({})

async function loadRoles() {
  loading.value = true
  try {
    roles.value = await listRoles()
  } catch {
    error.value = 'Failed to load roles'
  } finally {
    loading.value = false
  }
}

async function assignRole(roleName: string) {
  const userId = assignForms[roleName]?.trim()
  if (!userId) return
  try {
    await apiAssignRole(roleName, userId)
    assignForms[roleName] = ''
    await loadRoles()
    toastStore.success('Role assigned')
  } catch {
    toastStore.error('Failed to assign role')
  }
}

async function revokeRole(roleName: string, userId: string) {
  try {
    await apiRevokeRole(roleName, userId)
    await loadRoles()
    toastStore.success('Role revoked')
  } catch {
    toastStore.error('Failed to revoke role')
  }
}

onMounted(loadRoles)
</script>
