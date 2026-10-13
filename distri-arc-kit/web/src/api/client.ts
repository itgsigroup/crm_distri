// Thin fetch wrapper for /api (proxied to arc api in dev). Errors carry the API error envelope.

export class ApiError extends Error {
  status: number
  code: string
  constructor(status: number, code: string, message: string) {
    super(message)
    this.status = status
    this.code = code
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch('/api' + path, {
    method,
    headers: { Accept: 'application/json', 'X-Requested-With': 'distri-arc', ...(body ? { 'Content-Type': 'application/json' } : {}) },
    body: body ? JSON.stringify(body) : undefined,
    credentials: 'same-origin',
  })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (res.status === 401 && !path.startsWith('/auth/') && typeof window !== 'undefined' && window.location.pathname !== '/login') {
    window.location.assign('/login?next=' + encodeURIComponent(window.location.pathname + window.location.search))
  }
  if (!res.ok) {
    const e = data?.error ?? {}
    throw new ApiError(res.status, e.code ?? 'error', e.message ?? res.statusText)
  }
  return data as T
}

/** POST multipart/form-data (a file from Chat); same error envelope as request. */
async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch('/api' + path, { method: 'POST', body: form, credentials: 'same-origin', headers: { Accept: 'application/json', 'X-Requested-With': 'distri-arc' } })
  const text = await res.text()
  const data = text ? JSON.parse(text) : null
  if (!res.ok) {
    const e = data?.error ?? {}
    throw new ApiError(res.status, e.code ?? 'error', e.message ?? (res.status === 413 ? 'File terlalu besar (maks. 25 MB)' : res.statusText))
  }
  return data as T
}

export const api = {
  upload,
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, body ?? {}),
  del: <T>(path: string) => request<T>('DELETE', path),
}

export type Items<T> = { items: T[] }
