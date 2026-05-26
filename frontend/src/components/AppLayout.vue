<template>
  <div class="min-h-screen flex" style="background: var(--surface);">
    <!-- Desktop sidebar -->
    <aside
      class="hidden md:flex flex-col fixed left-0 top-0 bottom-0 z-30 transition-all duration-200"
      :style="{
        width: sidebarCollapsed ? '64px' : '240px',
        background: 'var(--sidebar-bg)',
        borderRight: '1px solid var(--border)',
      }"
    >
      <!-- Logo -->
      <div class="flex items-center gap-3 px-4 py-5" style="border-bottom: 1px solid var(--border);">
        <div class="w-8 h-8 rounded-lg flex items-center justify-center flex-shrink-0 bg-indigo-500 text-white font-bold text-sm">W</div>
        <span v-if="!sidebarCollapsed" class="font-semibold text-sm" style="color: var(--text-primary);">Warden</span>
        <button
          class="ml-auto opacity-50 hover:opacity-100 flex-shrink-0"
          style="color: var(--text-muted);"
          :title="sidebarCollapsed ? 'Expand' : 'Collapse'"
          @click="sidebarCollapsed = !sidebarCollapsed"
        >
          <svg width="16" height="16" viewBox="0 0 16 16" fill="currentColor">
            <path v-if="!sidebarCollapsed" d="M6 8l4-4v8z"/>
            <path v-else d="M10 8L6 4v8z"/>
          </svg>
        </button>
      </div>

      <!-- Nav items -->
      <nav class="flex-1 py-3 overflow-y-auto overflow-x-hidden">
        <NavItem
          v-for="item in navItems"
          :key="item.to"
          :item="item"
          :collapsed="sidebarCollapsed"
        />
      </nav>

      <!-- Footer: theme toggle + user -->
      <div style="border-top: 1px solid var(--border);" class="p-3 space-y-1">
        <!-- Theme toggle -->
        <button
          class="w-full flex items-center gap-3 px-3 py-2 rounded-lg text-sm hover:opacity-80 transition-opacity"
          style="color: var(--text-muted);"
          @click="themeStore.toggle()"
          :title="themeStore.theme === 'dark' ? 'Switch to light' : 'Switch to dark'"
        >
          <span class="text-base flex-shrink-0">{{ themeStore.theme === 'dark' ? '☀' : '☽' }}</span>
          <span v-if="!sidebarCollapsed">{{ themeStore.theme === 'dark' ? 'Light mode' : 'Dark mode' }}</span>
        </button>

        <!-- User info + sign out -->
        <div v-if="authStore.user" class="flex items-center gap-3 px-3 py-2">
          <div
            class="w-7 h-7 rounded-full flex items-center justify-center text-xs font-bold text-white bg-indigo-500 flex-shrink-0"
          >
            {{ userInitial }}
          </div>
          <div v-if="!sidebarCollapsed" class="flex-1 min-w-0">
            <div class="text-xs font-medium truncate" style="color: var(--text-primary);">
              {{ authStore.user.name || authStore.user.email }}
            </div>
            <button
              class="text-xs hover:underline"
              style="color: var(--text-muted);"
              @click="handleSignOut"
            >
              Sign out
            </button>
          </div>
        </div>
      </div>
    </aside>

    <!-- Main content area -->
    <main
      class="flex-1 min-h-screen pb-16 md:pb-0"
      :style="{ marginLeft: isMd ? (sidebarCollapsed ? '64px' : '240px') : '0' }"
    >
      <!-- Mobile header -->
      <header
        class="md:hidden flex items-center justify-between px-4 py-3 sticky top-0 z-20"
        style="background: var(--card); border-bottom: 1px solid var(--border);"
      >
        <div class="flex items-center gap-2">
          <div class="w-7 h-7 rounded-lg bg-indigo-500 flex items-center justify-center text-white font-bold text-xs">W</div>
          <span class="font-semibold text-sm" style="color: var(--text-primary);">Warden</span>
        </div>
        <button
          class="p-2 rounded-lg"
          style="color: var(--text-muted);"
          @click="themeStore.toggle()"
        >
          {{ themeStore.theme === 'dark' ? '☀' : '☽' }}
        </button>
      </header>

      <div class="p-4 md:p-6 max-w-6xl mx-auto">
        <RouterView />
      </div>
    </main>

    <!-- Mobile bottom tab bar -->
    <nav
      class="md:hidden fixed bottom-0 left-0 right-0 z-30 flex items-stretch"
      style="background: var(--card); border-top: 1px solid var(--border); height: 56px;"
    >
      <RouterLink
        v-for="item in mobileNavItems"
        :key="item.to"
        :to="item.to"
        class="flex-1 flex flex-col items-center justify-center gap-0.5 text-xs transition-colors relative"
        :style="($route.path === item.to || $route.path.startsWith(item.to + '/') && item.to !== '/')
          ? 'color: var(--nav-active-text);'
          : 'color: var(--text-muted);'"
      >
        <span class="text-lg leading-none">{{ item.icon }}</span>
        <span class="text-[10px]">{{ item.label }}</span>
        <!-- Approval badge -->
        <span
          v-if="item.badge && approvalsStore.pendingCount > 0"
          class="absolute top-1 right-1/4 bg-red-500 text-white text-[9px] font-bold rounded-full min-w-[16px] h-4 flex items-center justify-center px-1"
        >
          {{ approvalsStore.pendingCount > 99 ? '99+' : approvalsStore.pendingCount }}
        </span>
      </RouterLink>
    </nav>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'
import { useApprovalsStore } from '../stores/approvals'
import NavItem from './NavItem.vue'

const authStore = useAuthStore()
const themeStore = useThemeStore()
const approvalsStore = useApprovalsStore()
const router = useRouter()

const sidebarCollapsed = ref(false)
const isMd = ref(window.innerWidth >= 768)

const userInitial = computed(() => {
  const u = authStore.user
  if (!u) return '?'
  return (u.name || u.email)[0].toUpperCase()
})

interface NavItemDef {
  to: string
  label: string
  icon: string
  badge?: boolean
  permission?: string
}

const navItems = computed<NavItemDef[]>(() => [
  { to: '/', label: 'Dashboard', icon: '◈' },
  { to: '/identities', label: 'Identities', icon: '◉' },
  { to: '/devices', label: 'Devices', icon: '▣' },
  { to: '/holds', label: 'Legal Holds', icon: '⊞' },
  { to: '/approvals', label: 'Approvals', icon: '✓', badge: true },
  { to: '/audit', label: 'Audit Log', icon: '≡' },
  { to: '/breakglass', label: 'Break Glass', icon: '⚡' },
  { to: '/settings', label: 'Settings', icon: '⚙' },
])

const mobileNavItems = computed<NavItemDef[]>(() => [
  { to: '/', label: 'Home', icon: '◈' },
  { to: '/identities', label: 'Identities', icon: '◉' },
  { to: '/holds', label: 'Holds', icon: '⊞' },
  { to: '/approvals', label: 'Approvals', icon: '✓', badge: true },
  { to: '/settings', label: 'More', icon: '⋯' },
])

async function handleSignOut() {
  await authStore.logout()
  router.push({ name: 'login' })
}

function handleResize() {
  isMd.value = window.innerWidth >= 768
}

// Poll approval count every 30s
let pollInterval: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  approvalsStore.fetchApprovals()
  pollInterval = setInterval(() => approvalsStore.fetchApprovals(), 30000)
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  if (pollInterval) clearInterval(pollInterval)
  window.removeEventListener('resize', handleResize)
})
</script>
