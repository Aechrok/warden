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

// DEV-only: lazily resolved after the debug store module loads.
// In production builds import.meta.env.DEV is false and this block is dead code.
type RecordFn = (method: string, path: string, status: number, ms: number) => void
let _record: RecordFn | undefined
if (import.meta.env.DEV) {
  import('../stores/debug').then(({ useDebugStore }) => {
    _record = (method, path, status, ms) => {
      try {
        useDebugStore().record({ method, path, status, durationMs: ms, at: new Date().toISOString() })
      } catch { /* store may not yet be initialized */ }
    }
  })
}

export async function apiFetch<T>(path: string, options: RequestInit = {}): Promise<T> {
  const url = path.startsWith('/') ? path : `/api/v1/internal/${path}`
  const t0 = Date.now()

  const response = await fetch(url, {
    ...options,
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers ?? {}),
    },
  })

  if (import.meta.env.DEV && _record) {
    _record((options.method ?? 'GET').toUpperCase(), url, response.status, Date.now() - t0)
  }

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
