export class ApiError extends Error {
  constructor(
    public readonly status: number,
    message: string,
    public readonly body?: unknown
  ) {
    super(message)
    this.name = 'ApiError'
  }
}

export async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const url = path.startsWith('/') ? path : `/api/v1/internal/${path}`

  const response = await fetch(url, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
  })

  if (!response.ok) {
    let body: unknown
    try { body = await response.json() } catch { /* ignore */ }
    throw new ApiError(response.status, `HTTP ${response.status}`, body)
  }

  // Handle empty responses (204, etc.)
  const text = await response.text()
  if (!text) return undefined as unknown as T
  return JSON.parse(text) as T
}

export function apiDownload(path: string): void {
  const url = path.startsWith('/') ? path : `/api/v1/internal/${path}`
  const a = document.createElement('a')
  a.href = url
  a.download = ''
  a.click()
}
