<template>
  <div>
    <div class="flex items-center justify-between mb-2">
      <h2 class="font-semibold" style="color: var(--text-primary);">Magic Link Invitations</h2>
      <button
        class="px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600"
        @click="openCreate"
      >
        + Invite
      </button>
    </div>
    <p class="text-sm mb-4" style="color: var(--text-muted);">
      Generate a one-time login link for external users such as auditors. The link grants a session without a password.
    </p>

    <ErrorBanner :message="error" class="mb-4" />

    <div v-if="loading" class="py-8 flex items-center justify-center">
      <div class="animate-spin w-5 h-5 rounded-full border-2 border-indigo-500 border-t-transparent"></div>
    </div>

    <!-- Newly created link reveal -->
    <div
      v-if="newLink"
      class="mb-4 p-4 rounded-xl bg-green-500/10 border border-green-500/30"
    >
      <div class="text-sm font-semibold text-green-600 mb-1">Invitation link created — copy it now:</div>
      <code
        class="block px-3 py-2 rounded-lg text-xs font-mono break-all"
        style="background: var(--surface); color: var(--text-primary);"
      >{{ newLink }}</code>
      <button class="mt-2 text-xs text-green-600 hover:underline" @click="copyLink">Copy to clipboard</button>
    </div>

    <div
      v-if="!loading && invitations.length === 0"
      class="py-8 text-center text-sm rounded-xl"
      style="color: var(--text-muted); background: var(--card); border: 1px solid var(--border);"
    >
      No invitations yet
    </div>

    <div class="rounded-xl overflow-hidden" style="border: 1px solid var(--border);" v-else-if="!loading">
      <table class="w-full text-sm">
        <thead>
          <tr style="background: var(--card); border-bottom: 1px solid var(--border);">
            <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Email</th>
            <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Label</th>
            <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Role</th>
            <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Status</th>
            <th class="px-4 py-3 text-left font-medium" style="color: var(--text-muted);">Expires</th>
            <th class="px-4 py-3"></th>
          </tr>
        </thead>
        <tbody>
          <tr
            v-for="inv in invitations"
            :key="inv.id"
            style="border-top: 1px solid var(--border);"
          >
            <td class="px-4 py-3 font-medium" style="color: var(--text-primary);">{{ inv.email }}</td>
            <td class="px-4 py-3 text-xs" style="color: var(--text-muted);">{{ inv.label || '—' }}</td>
            <td class="px-4 py-3">
              <span
                v-if="inv.role_name"
                class="text-xs font-medium px-2 py-0.5 rounded-full"
                style="background: var(--nav-active-bg); color: var(--nav-active-text);"
              >{{ inv.role_name }}</span>
              <span v-else class="text-xs" style="color: var(--text-muted);">—</span>
            </td>
            <td class="px-4 py-3">
              <span
                v-if="inv.used_at"
                class="text-xs px-2 py-0.5 rounded-full"
                style="background: rgba(16,185,129,0.12); color: #10b981;"
              >Used</span>
              <span
                v-else-if="isExpired(inv.expires_at)"
                class="text-xs px-2 py-0.5 rounded-full"
                style="background: var(--border); color: var(--text-muted);"
              >Expired</span>
              <span
                v-else
                class="text-xs px-2 py-0.5 rounded-full"
                style="background: rgba(99,102,241,0.12); color: #6366f1;"
              >Pending</span>
            </td>
            <td class="px-4 py-3 text-xs" style="color: var(--text-muted);">{{ formatDate(inv.expires_at) }}</td>
            <td class="px-4 py-3 text-right flex items-center gap-2 justify-end">
              <button
                v-if="!inv.used_at && !isExpired(inv.expires_at)"
                class="text-xs text-indigo-500 hover:underline"
                @click="copyInviteLink(inv.token)"
              >Copy link</button>
              <button class="text-xs text-red-500 hover:underline" @click="remove(inv.id)">Delete</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Create modal -->
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
          <h3 class="text-lg font-semibold mb-4" style="color: var(--text-primary);">Create Invitation</h3>

          <div class="space-y-4">
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Email *</label>
              <input
                v-model="form.email"
                type="email"
                placeholder="auditor@example.com"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Label (optional)</label>
              <input
                v-model="form.label"
                type="text"
                placeholder="e.g. Q2 Audit"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              />
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Assign role (optional)</label>
              <select
                v-model="form.role_name"
                class="w-full rounded-lg px-3 py-2 text-sm"
                style="background: var(--input-bg); border: 1px solid var(--input-border); color: var(--text-primary);"
              >
                <option value="">No role</option>
                <option v-for="role in roles" :key="role.name" :value="role.name">{{ role.name }}</option>
              </select>
            </div>
            <div>
              <label class="block text-sm font-medium mb-1" style="color: var(--text-primary);">Expires after (hours)</label>
              <input
                v-model.number="form.expiry_hours"
                type="number"
                min="1"
                max="720"
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
            >Cancel</button>
            <button
              class="flex-1 px-4 py-2 rounded-lg text-sm font-medium text-white bg-indigo-500 hover:bg-indigo-600 disabled:opacity-50"
              :disabled="creating || !form.email.trim()"
              @click="create"
            >{{ creating ? 'Creating…' : 'Create' }}</button>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { listInvitations, createInvitation, deleteInvitation, listRoles } from '../../api/admin'
import type { Invitation, Role } from '../../api/types'
import { useToastStore } from '../../stores/toast'
import ErrorBanner from '../ErrorBanner.vue'

const toastStore = useToastStore()
const invitations = ref<Invitation[]>([])
const roles = ref<Role[]>([])
const loading = ref(false)
const creating = ref(false)
const error = ref<string | null>(null)
const showCreate = ref(false)
const newLink = ref<string | null>(null)

const form = reactive({
  email: '',
  label: '',
  role_name: '',
  expiry_hours: 168,
})

function inviteURL(token: string): string {
  return `${window.location.origin}/auth/magic?token=${encodeURIComponent(token)}`
}

function isExpired(ts: string): boolean {
  return new Date(ts) < new Date()
}

function formatDate(ts: string): string {
  return new Date(ts).toLocaleDateString()
}

function openCreate() {
  form.email = ''
  form.label = ''
  form.role_name = ''
  form.expiry_hours = 168
  newLink.value = null
  showCreate.value = true
}

async function load() {
  loading.value = true
  error.value = null
  try {
    const [inv, r] = await Promise.all([listInvitations(), listRoles()])
    invitations.value = inv
    roles.value = r
  } catch {
    error.value = 'Failed to load invitations'
  } finally {
    loading.value = false
  }
}

async function create() {
  creating.value = true
  try {
    const res = await createInvitation({
      email: form.email.trim(),
      role_name: form.role_name || undefined,
      label: form.label || undefined,
      expiry_hours: form.expiry_hours,
    })
    newLink.value = inviteURL(res.token)
    showCreate.value = false
    await load()
    toastStore.success('Invitation created')
  } catch {
    toastStore.error('Failed to create invitation')
  } finally {
    creating.value = false
  }
}

async function remove(id: string) {
  try {
    await deleteInvitation(id)
    await load()
    toastStore.success('Invitation deleted')
  } catch {
    toastStore.error('Failed to delete invitation')
  }
}

async function copyLink() {
  if (newLink.value) {
    await navigator.clipboard.writeText(newLink.value)
    toastStore.success('Copied to clipboard')
  }
}

async function copyInviteLink(token: string) {
  await navigator.clipboard.writeText(inviteURL(token))
  toastStore.success('Copied to clipboard')
}

onMounted(load)
</script>
