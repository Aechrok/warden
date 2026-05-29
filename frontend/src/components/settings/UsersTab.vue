<template>
  <div>
    <h2 class="font-semibold mb-4" style="color: var(--text-primary);">Users</h2>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div v-if="!loading && users.length === 0" class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);">
      No users found
    </div>

    <div class="space-y-2">
      <div
        v-for="user in users"
        :key="user.id"
        class="rounded-xl px-4 py-3"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <!-- Info row — single line -->
        <div class="flex items-center gap-2 mb-2 min-w-0">
          <span class="font-medium text-sm truncate" style="color: var(--text-primary);">{{ user.name }}</span>
          <span class="text-sm flex-shrink-0" style="color: var(--text-muted);">·</span>
          <span class="text-sm truncate" style="color: var(--text-muted);">{{ user.email }}</span>
          <span
            class="px-2 py-0.5 rounded text-xs font-medium uppercase tracking-wide flex-shrink-0"
            :class="user.origin === 'scim' ? 'bg-purple-100 text-purple-700' : 'bg-sky-100 text-sky-700'"
          >{{ user.origin }}</span>
          <span
            v-if="!user.is_active"
            class="px-2 py-0.5 rounded text-xs flex-shrink-0"
            style="background: var(--border); color: var(--text-muted);"
          >Inactive</span>
        </div>

        <!-- Roles row — pills + save -->
        <div class="flex items-center gap-2">
          <!-- Pills -->
          <div class="flex items-center gap-1.5 flex-wrap flex-1">
            <button
              v-for="role in allRoles"
              :key="role.name"
              class="px-3 py-1 rounded-full text-xs font-medium transition-colors"
              :class="getRoleActive(user, role.name)
                ? 'bg-indigo-500 text-white'
                : 'hover:opacity-80'"
              :style="!getRoleActive(user, role.name)
                ? 'background: var(--border); color: var(--text-muted);'
                : ''"
              @click="toggleRole(user, role.name)"
            >{{ role.name }}</button>
          </div>

          <!-- Save button — only visible when pending changes exist -->
          <button
            v-if="isDirty(user)"
            class="flex-shrink-0 px-4 py-1 rounded-full text-xs font-semibold text-white bg-green-600 hover:bg-green-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            :disabled="!!savingId[user.id]"
            @click="saveRoles(user)"
          >{{ savingId[user.id] ? 'Saving…' : 'Save' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listUsers, listRoles, assignRole, revokeRole } from '../../api/admin'
import type { UserWithRoles, Role } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const users = ref<UserWithRoles[]>([])
const allRoles = ref<Role[]>([])
const loading = ref(false)
const error = ref<string | null>(null)

// Per-user pending role sets — only populated when the user has made a change.
// Keyed by user.id, value is the locally modified role name array.
const pending = reactive<Record<string, string[]>>({})
const savingId = reactive<Record<string, boolean>>({})

function getRoleActive(user: UserWithRoles, roleName: string): boolean {
  if (user.id in pending) return pending[user.id].includes(roleName)
  return user.roles.includes(roleName)
}

function isDirty(user: UserWithRoles): boolean {
  if (!(user.id in pending)) return false
  const p = pending[user.id]
  const orig = user.roles
  if (p.length !== orig.length) return true
  const origSet = new Set(orig)
  return p.some((r) => !origSet.has(r))
}

function toggleRole(user: UserWithRoles, roleName: string) {
  pending[user.id] = [roleName]
}

async function saveRoles(user: UserWithRoles) {
  if (!isDirty(user)) return
  savingId[user.id] = true
  try {
    const next = new Set(pending[user.id])
    const prev = new Set(user.roles)

    // Assign newly added roles
    for (const r of next) {
      if (!prev.has(r)) await assignRole(r, user.id)
    }
    // Revoke removed roles
    for (const r of prev) {
      if (!next.has(r)) await revokeRole(r, user.id)
    }

    delete pending[user.id]
    toastStore.success('Roles updated')
    await load()
  } catch {
    toastStore.error('Failed to update roles')
  } finally {
    savingId[user.id] = false
  }
}

async function load() {
  loading.value = true
  try {
    const [u, r] = await Promise.all([listUsers(), listRoles()])
    users.value = u
    allRoles.value = r
  } catch {
    error.value = 'Failed to load users'
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>
