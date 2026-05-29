<template>
  <div>
    <h1 class="text-2xl font-bold mb-6" style="color: var(--text-primary);">Settings</h1>

    <!-- Tab navigation -->
    <div
      class="flex gap-1 p-1 rounded-xl mb-6 overflow-x-auto"
      style="background: var(--card); border: 1px solid var(--border);"
    >
      <button
        v-for="tab in availableTabs"
        :key="tab.key"
        class="px-4 py-2 rounded-lg text-sm font-medium whitespace-nowrap transition-colors flex-shrink-0"
        :style="activeTab === tab.key
          ? 'background: var(--nav-active-bg); color: var(--nav-active-text);'
          : 'color: var(--text-muted);'"
        @click="activeTab = tab.key"
      >
        {{ tab.label }}
      </button>
    </div>

    <!-- Tab content -->
    <UsersTab v-if="activeTab === 'users'" />
    <SCIMGroupsTab v-else-if="activeTab === 'groups'" />
    <RolesTab v-else-if="activeTab === 'roles'" />
    <InstancesTab v-else-if="activeTab === 'instances'" />
    <PBACTab v-else-if="activeTab === 'pbac'" />
    <HoldTemplatesTab v-else-if="activeTab === 'hold-templates'" />
    <VIPTab v-else-if="activeTab === 'vip'" />
    <SSOTab v-else-if="activeTab === 'sso'" />
    <InvitationsTab v-else-if="activeTab === 'invitations'" />
    <TokensTab v-else-if="activeTab === 'tokens'" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useAuthStore } from '../stores/auth'
import UsersTab from '../components/settings/UsersTab.vue'
import SCIMGroupsTab from '../components/settings/SCIMGroupsTab.vue'
import RolesTab from '../components/settings/RolesTab.vue'
import InstancesTab from '../components/settings/InstancesTab.vue'
import PBACTab from '../components/settings/PBACTab.vue'
import HoldTemplatesTab from '../components/settings/HoldTemplatesTab.vue'
import VIPTab from '../components/settings/VIPTab.vue'
import SSOTab from '../components/settings/SSOTab.vue'
import InvitationsTab from '../components/settings/InvitationsTab.vue'
import TokensTab from '../components/settings/TokensTab.vue'

const authStore = useAuthStore()
const activeTab = ref('users')

interface Tab {
  key: string
  label: string
  permission?: string
}

const allTabs: Tab[] = [
  { key: 'users', label: 'Users', permission: 'users:read' },
  { key: 'groups', label: 'Groups', permission: 'roles:read' },
  { key: 'roles', label: 'Roles', permission: 'roles:read' },
  { key: 'instances', label: 'Instances', permission: 'instances:write' },
  { key: 'pbac', label: 'PBAC Policies', permission: 'pbac_policies:write' },
  { key: 'hold-templates', label: 'Hold Templates', permission: 'hold_templates:write' },
  { key: 'vip', label: 'VIP Identities', permission: 'vip_identities:write' },
  { key: 'sso', label: 'SSO & SCIM', permission: 'instances:write' },
  { key: 'invitations', label: 'Invitations', permission: 'users:write' },
  { key: 'tokens', label: 'API Tokens' },
]

const availableTabs = computed(() =>
  allTabs.filter((t) => !t.permission || authStore.hasPermission(t.permission))
)
</script>
