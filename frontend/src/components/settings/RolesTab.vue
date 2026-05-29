<template>
  <div>
    <div class="flex items-center justify-between mb-4">
      <h2 class="font-semibold" style="color: var(--text-primary);">Roles</h2>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="openCreate"
      >
        + Create Role
      </button>
    </div>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <div class="space-y-3">
      <div
        v-for="role in roles"
        :key="role.name"
        class="rounded-xl p-4"
        style="background: var(--card); border: 1px solid var(--border);"
      >
        <div class="flex items-start justify-between gap-3 mb-2">
          <div class="flex-1 min-w-0">
            <div class="flex items-center gap-2 flex-wrap">
              <span class="font-semibold text-sm" style="color: var(--text-primary);">{{ role.name }}</span>
              <span
                v-if="role.is_builtin"
                class="px-2 py-0.5 rounded text-xs"
                style="background: var(--nav-active-bg); color: var(--nav-active-text);"
              >System</span>
            </div>
            <div v-if="role.description" class="text-xs mt-0.5" style="color: var(--text-muted);">{{ role.description }}</div>
          </div>
          <div class="flex gap-2 flex-shrink-0">
            <button
              v-if="!role.is_builtin"
              class="text-xs px-3 py-1.5 rounded-lg"
              style="background: var(--border); color: var(--text-primary);"
              @click="openEdit(role)"
            >
              Edit
            </button>
            <button
              v-if="!role.is_builtin"
              class="text-xs text-red-500 hover:underline"
              @click="confirmDelete(role)"
            >
              Delete
            </button>
          </div>
        </div>

        <div class="flex flex-wrap gap-1">
          <span
            v-for="perm in role.permissions"
            :key="perm"
            class="px-2 py-0.5 rounded text-xs font-mono"
            style="background: var(--surface); border: 1px solid var(--border); color: var(--text-muted);"
          >{{ perm }}</span>
          <span v-if="role.permissions.length === 0" class="text-xs italic" style="color: var(--text-muted);">No permissions</span>
        </div>
      </div>
    </div>

    <!-- Create / Edit modal -->
    <Teleport to="body">
      <div
        v-if="showModal"
        class="fixed inset-0 z-40 flex items-center justify-center p-4"
        style="background: rgba(0,0,0,0.6);"
        @click.self="showModal = false"
      >
        <div
          class="w-full max-w-lg rounded-2xl p-6 shadow-2xl overflow-y-auto"
          style="background: var(--card); border: 1px solid var(--border); max-height: 90vh;"
        >
          <h3 class="text-lg font-semibold mb-4" style="color: var(--text-primary);">
            {{ editingName ? 'Edit Role Permissions' : 'Create Role' }}
          </h3>

          <div class="space-y-4">
            <template v-if="!editingName">
              <div>
                <label for="role-name" class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Name *</label>
                <input
                  id="role-name"
                  v-model="form.name"
                  type="text"
                  placeholder="e.g. security_analyst"
                  class="w-full rounded-lg px-3 py-2 text-sm"
                  style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                />
              </div>
              <div>
                <label for="role-desc" class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Description</label>
                <input
                  id="role-desc"
                  v-model="form.description"
                  type="text"
                  class="w-full rounded-lg px-3 py-2 text-sm"
                  style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
                />
              </div>
            </template>

            <div>
              <div class="flex items-center justify-between mb-2">
                <span class="text-sm font-medium" style="color: var(--text-primary);">Permissions</span>
                <div class="flex gap-3 text-xs" style="color: var(--text-muted);">
                  <button class="hover:underline" @click="selectAll">All</button>
                  <button class="hover:underline" @click="selectNone">None</button>
                </div>
              </div>
              <div class="space-y-1 rounded-lg p-3" style="background: var(--surface); border: 1px solid var(--border); max-height: 320px; overflow-y: auto;">
                <label
                  v-for="perm in allPermissions"
                  :key="perm"
                  class="flex items-center gap-2 py-0.5 cursor-pointer"
                >
                  <input
                    type="checkbox"
                    :value="perm"
                    v-model="form.permissions"
                    class="rounded"
                  />
                  <span class="text-xs font-mono" style="color: var(--text-primary);">{{ perm }}</span>
                </label>
              </div>
              <div class="text-xs mt-1" style="color: var(--text-muted);">{{ form.permissions.length }} selected</div>
            </div>
          </div>

          <div class="flex gap-3 mt-6">
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm"
              style="background: var(--border); color: var(--text-primary);"
              @click="showModal = false"
            >Cancel</button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 disabled:opacity-50 disabled:cursor-not-allowed"
              :disabled="saving || (!editingName && !form.name)"
              @click="save"
            >
              <span v-if="saving">Saving...</span>
              <span v-else>{{ editingName ? 'Save Permissions' : 'Create Role' }}</span>
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listRoles, listPermissions, createRole, updateRolePermissions, deleteRole } from '../../api/admin'
import type { Role } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const roles = ref<Role[]>([])
const allPermissions = ref<string[]>([])
const loading = ref(false)
const error = ref<string | null>(null)
const showModal = ref(false)
const saving = ref(false)
const editingName = ref<string | null>(null)

const form = reactive({
  name: '',
  description: '',
  permissions: [] as string[],
})

function openCreate() {
  editingName.value = null
  form.name = ''
  form.description = ''
  form.permissions = []
  showModal.value = true
}

function openEdit(role: Role) {
  editingName.value = role.name
  form.name = role.name
  form.description = role.description
  form.permissions = [...role.permissions]
  showModal.value = true
}

function selectAll() { form.permissions = [...allPermissions.value] }
function selectNone() { form.permissions = [] }

async function save() {
  saving.value = true
  try {
    if (editingName.value) {
      await updateRolePermissions(editingName.value, form.permissions)
      toastStore.success('Permissions updated')
    } else {
      await createRole({ name: form.name, description: form.description, permissions: form.permissions })
      toastStore.success('Role created')
    }
    showModal.value = false
    await loadRoles()
  } catch {
    toastStore.error(editingName.value ? 'Failed to update permissions' : 'Failed to create role')
  } finally {
    saving.value = false
  }
}

async function confirmDelete(role: Role) {
  if (!confirm(`Delete role "${role.name}"? Users with this role will lose it.`)) return
  try {
    await deleteRole(role.name)
    toastStore.success('Role deleted')
    await loadRoles()
  } catch {
    toastStore.error('Failed to delete role')
  }
}

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

onMounted(async () => {
  const [, perms] = await Promise.allSettled([loadRoles(), listPermissions()])
  if (perms.status === 'fulfilled') allPermissions.value = perms.value
})
</script>
