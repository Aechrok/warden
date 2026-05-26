import { apiFetch } from './client'
import type { ActionDef, ActionExecuteRequest, ActionExecuteResponse } from './types'

export async function listActions(): Promise<ActionDef[]> {
  const res = await apiFetch<{ actions: ActionDef[] }>('/api/v1/internal/actions/')
  return res.actions ?? []
}

export async function executeAction(req: ActionExecuteRequest): Promise<ActionExecuteResponse> {
  return apiFetch<ActionExecuteResponse>('/api/v1/internal/actions/execute', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}
