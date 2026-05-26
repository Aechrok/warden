import { apiFetch } from './client'
import type { Token, CreateTokenRequest } from './types'

export async function listTokens(): Promise<Token[]> {
  const res = await apiFetch<{ tokens: Token[] }>('/api/v1/internal/tokens')
  return res.tokens ?? []
}

export async function createToken(req: CreateTokenRequest): Promise<Token> {
  const res = await apiFetch<{ token: Token }>('/api/v1/internal/tokens', {
    method: 'POST',
    body: JSON.stringify(req),
  })
  return res.token
}

export async function deleteToken(id: string): Promise<void> {
  await apiFetch<void>(`/api/v1/internal/tokens/${id}`, { method: 'DELETE' })
}
