import { apiFetch } from './client'
import type { BreakGlassInvokeRequest, Incident } from './types'

export async function invokeBreakGlass(req: BreakGlassInvokeRequest): Promise<{ result?: string }> {
  return apiFetch<{ result?: string }>('/api/v1/internal/breakglass/invoke', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export async function listIncidents(): Promise<Incident[]> {
  const res = await apiFetch<{ incidents: Incident[] }>('/api/v1/internal/breakglass/incidents')
  return res.incidents ?? []
}

export async function reviewIncident(id: string, note: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/breakglass/incidents/${id}/review`, {
    method: 'POST',
    body: JSON.stringify({ note }),
  })
}
