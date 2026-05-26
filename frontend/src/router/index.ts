import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { ApiError } from '../api/client'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/LoginView.vue'),
      meta: { requiresAuth: false },
    },
    {
      path: '/',
      component: () => import('../components/AppLayout.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/DashboardView.vue'),
        },
        {
          path: 'identities',
          name: 'identities',
          component: () => import('../views/IdentitiesView.vue'),
        },
        {
          path: 'devices',
          name: 'devices',
          component: () => import('../views/DevicesView.vue'),
        },
        {
          path: 'audit',
          name: 'audit',
          component: () => import('../views/AuditView.vue'),
        },
        {
          path: 'holds',
          name: 'holds',
          component: () => import('../views/LegalHoldView.vue'),
        },
        {
          path: 'holds/:id',
          name: 'hold-detail',
          component: () => import('../views/HoldDetailView.vue'),
        },
        {
          path: 'approvals',
          name: 'approvals',
          component: () => import('../views/ApprovalView.vue'),
        },
        {
          path: 'breakglass',
          name: 'breakglass',
          component: () => import('../views/BreakGlassView.vue'),
        },
        {
          path: 'settings',
          name: 'settings',
          component: () => import('../views/SettingsView.vue'),
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/',
    },
  ],
})

// Navigation guard
router.beforeEach(async (to) => {
  if (to.meta.requiresAuth === false) return true

  const auth = useAuthStore()

  if (!auth.initialized) {
    const ok = await auth.fetchMe()
    if (!ok) return { name: 'login' }
  }

  if (!auth.isAuthenticated) {
    return { name: 'login' }
  }

  return true
})

// Handle 401 globally — the API client throws ApiError; catch in the router too
export function handleApiError(err: unknown): void {
  if (err instanceof ApiError && err.status === 401) {
    const auth = useAuthStore()
    auth.logout()
    router.push({ name: 'login' })
  }
}

export default router
