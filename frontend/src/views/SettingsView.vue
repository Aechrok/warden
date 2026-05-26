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
    <RolesTab v-if="activeTab === 'roles'" />
    <InstancesTab v-else-if="activeTab === 'instances'" />
    <PBACTab v-else-if="activeTab === 'pbac'" />
    <HoldTemplatesTab v-else-if="activeTab === 'hold-templates'" />
    <VIPTab v-else-if="activeTab === 'vip'" />
    <TokensTab v-else-if="activeTab === 'tokens'" />
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'
import { useAuthStore } from '../stores/auth'
import RolesTab from '../components/settings/RolesTab.vue'
import InstancesTab from '../components/settings/InstancesTab.vue'
import PBACTab from '../components/settings/PBACTab.vue'
import HoldTemplatesTab from '../components/settings/HoldTemplatesTab.vue'
import VIPTab from '../components/settings/VIPTab.vue'
import TokensTab from '../components/settings/TokensTab.vue'

const authStore = useAuthStore()
const activeTab = ref('tokens')

interface Tab {
  key: string
  label: string
  permission?: string
}

const allTabs: Tab[] = [
  { key: 'tokens', label: 'API Tokens' },
  { key: 'roles', label: 'Roles', permission: 'roles:write' },
  { key: 'instances', label: 'Instances', permission: 'instances:write' },
  { key: 'pbac', label: 'PBAC Policies', permission: 'pbac_policies:write' },
  { key: 'hold-templates', label: 'Hold Templates', permission: 'hold_templates:write' },
  { key: 'vip', label: 'VIP Identities', permission: 'vip_identities:write' },
]

const availableTabs = computed(() =>
  allTabs.filter((t) => !t.permission || authStore.hasPermission(t.permission))
)
</script>
