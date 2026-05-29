<template>
  <div>
    <p class="text-sm mb-6" style="color: var(--text-muted);">
      Map SCIM push groups to Warden roles. When a group is mapped, member changes pushed by
      your identity provider automatically update that role's assignments.
    </p>

    <div v-if="loading" class="text-sm" style="color: var(--text-muted);">Loading…</div>

    <div v-else-if="error" class="text-sm" style="color: var(--error);">{{ error }}</div>

    <template v-else>
      <!-- Unmapped groups -->
      <section class="mb-8">
        <h2 class="text-base font-semibold mb-3" style="color: var(--text-primary);">
          Unmapped SCIM groups
          <span
            class="ml-2 text-xs font-normal px-2 py-0.5 rounded-full"
            style="background: var(--border); color: var(--text-muted);"
          >{{ unmapped.length }}</span>
        </h2>

        <div v-if="unmapped.length === 0" class="text-sm" style="color: var(--text-muted);">
          No unmapped groups — all pushed groups have a role assigned.
        </div>

        <div v-else class="rounded-xl overflow-hidden" style="border: 1px solid var(--border);">
          <table class="w-full text-sm">
            <thead>
              <tr style="background: var(--card); border-bottom: 1px solid var(--border);">
                <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Group</th>
                <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">External ID</th>
                <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Map to role</th>
                <th class="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="group in unmapped"
                :key="group.id"
                style="border-top: 1px solid var(--border);"
              >
                <td class="px-4 py-3 font-medium" style="color: var(--text-primary);">{{ group.name }}</td>
                <td class="px-4 py-3 font-mono text-xs" style="color: var(--text-muted);">{{ group.external_id }}</td>
                <td class="px-4 py-3">
                  <select
                    v-model="pending[group.id]"
                    class="text-sm rounded-lg px-3 py-1.5"
                    style="background: var(--background); border: 1px solid var(--border); color: var(--text-primary);"
                  >
                    <option value="">Select a role…</option>
                    <option v-for="role in roles" :key="role.id" :value="role.id">{{ role.name }}</option>
                  </select>
                </td>
                <td class="px-4 py-3 text-right">
                  <button
                    :disabled="!pending[group.id] || saving === group.id"
                    class="text-sm px-3 py-1.5 rounded-lg font-medium disabled:opacity-40"
                    style="background: var(--nav-active-bg); color: var(--nav-active-text);"
                    @click="map(group.id)"
                  >
                    {{ saving === group.id ? 'Saving…' : 'Map' }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- Mapped groups -->
      <section>
        <h2 class="text-base font-semibold mb-3" style="color: var(--text-primary);">
          Mapped groups
          <span
            class="ml-2 text-xs font-normal px-2 py-0.5 rounded-full"
            style="background: var(--border); color: var(--text-muted);"
          >{{ mapped.length }}</span>
        </h2>

        <div v-if="mapped.length === 0" class="text-sm" style="color: var(--text-muted);">
          No groups are mapped yet.
        </div>

        <div v-else class="rounded-xl overflow-hidden" style="border: 1px solid var(--border);">
          <table class="w-full text-sm">
            <thead>
              <tr style="background: var(--card); border-bottom: 1px solid var(--border);">
                <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Group</th>
                <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">External ID</th>
                <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Role</th>
                <th class="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="group in mapped"
                :key="group.id"
                style="border-top: 1px solid var(--border);"
              >
                <td class="px-4 py-3 font-medium" style="color: var(--text-primary);">{{ group.name }}</td>
                <td class="px-4 py-3 font-mono text-xs" style="color: var(--text-muted);">{{ group.external_id }}</td>
                <td class="px-4 py-3">
                  <span
                    class="text-xs font-medium px-2 py-1 rounded-full"
                    style="background: var(--nav-active-bg); color: var(--nav-active-text);"
                  >{{ group.role_name }}</span>
                </td>
                <td class="px-4 py-3 text-right">
                  <button
                    :disabled="saving === group.id"
                    class="text-sm px-3 py-1.5 rounded-lg font-medium disabled:opacity-40"
                    style="border: 1px solid var(--border); color: var(--text-muted);"
                    @click="unmap(group.id)"
                  >
                    {{ saving === group.id ? 'Saving…' : 'Unmap' }}
                  </button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { listSCIMGroups, updateSCIMGroupRole, listRoles } from '../../api/admin'
import type { SCIMGroup, Role } from '../../api/types'

const groups = ref<SCIMGroup[]>([])
const roles = ref<Role[]>([])
const loading = ref(true)
const error = ref('')
const saving = ref<string | null>(null)
const pending = ref<Record<string, string>>({})

const unmapped = computed(() => groups.value.filter((g) => !g.role_id))
const mapped = computed(() => groups.value.filter((g) => g.role_id))

async function load() {
  loading.value = true
  error.value = ''
  try {
    ;[groups.value, roles.value] = await Promise.all([listSCIMGroups(), listRoles()])
  } catch (e: any) {
    error.value = e?.message ?? 'Failed to load SCIM groups'
  } finally {
    loading.value = false
  }
}

async function map(groupId: string) {
  const roleId = pending.value[groupId]
  if (!roleId) return
  saving.value = groupId
  try {
    await updateSCIMGroupRole(groupId, roleId)
    delete pending.value[groupId]
    await load()
  } finally {
    saving.value = null
  }
}

async function unmap(groupId: string) {
  saving.value = groupId
  try {
    await updateSCIMGroupRole(groupId, null)
    await load()
  } finally {
    saving.value = null
  }
}

onMounted(load)
</script>
