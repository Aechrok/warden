import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { getMe, logout as apiLogout } from '../api/auth'
import type { User } from '../api/types'

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(null)
  const permissions = ref<string[]>([])
  const roles = ref<string[]>([])
  const loading = ref(false)
  const initialized = ref(false)

  const isAuthenticated = computed(() => user.value !== null)

  function hasPermission(perm: string): boolean {
    if (permissions.value.includes('*') || permissions.value.includes('*:*')) return true
    if (permissions.value.includes(perm)) return true
    // Wildcard matching: "holds:*" matches "holds:write"
    const [resource] = perm.split(':')
    if (permissions.value.includes(`${resource}:*`)) return true
    return false
  }

  function hasRole(role: string): boolean {
    return roles.value.includes(role)
  }

  async function fetchMe(): Promise<boolean> {
    loading.value = true
    try {
      const data = await getMe()
      user.value = data.user
      permissions.value = data.permissions
      roles.value = data.roles
      initialized.value = true
      return true
    } catch {
      user.value = null
      permissions.value = []
      roles.value = []
      initialized.value = true
      return false
    } finally {
      loading.value = false
    }
  }

  async function logout(): Promise<void> {
    try {
      await apiLogout()
    } finally {
      user.value = null
      permissions.value = []
      roles.value = []
    }
  }

  return {
    user,
    permissions,
    roles,
    loading,
    initialized,
    isAuthenticated,
    hasPermission,
    hasRole,
    fetchMe,
    logout,
  }
})
