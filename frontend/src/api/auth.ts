import { apiFetch } from './client'
import type { MeResponse } from './types'

export async function getMe(): Promise<MeResponse> {
  return apiFetch<MeResponse>('/api/v1/internal/me')
}

export async function logout(): Promise<void> {
  await apiFetch<void>('/api/v1/internal/auth/logout', { method: 'POST' })
}

export function initiateLogin(): void {
  window.location.href = '/auth/login'
}
