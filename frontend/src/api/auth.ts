import { apiFetch } from './client'
import type { MeResponse, AuthConfig } from './types'

export async function getMe(): Promise<MeResponse> {
  return apiFetch<MeResponse>('/api/v1/internal/me')
}

export async function logout(): Promise<void> {
  await apiFetch<void>('/api/v1/internal/auth/logout', { method: 'POST' })
}

export function initiateLogin(): void {
  window.location.href = '/auth/login'
}

export async function getAuthConfig(): Promise<AuthConfig> {
  return apiFetch<AuthConfig>('/auth/config')
}

export async function localLogin(email: string, password: string): Promise<void> {
  await apiFetch<void>('/auth/local', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  })
}
