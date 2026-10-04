import { useCallback, useEffect, useRef, useState } from 'react'
import { useUI } from '../state/ui'

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = { method, credentials: 'include', headers: {} }
  if (body !== undefined) {
    if (typeof body === 'string') {
      init.body = body
      ;(init.headers as Record<string, string>)['Content-Type'] = 'text/plain'
    } else {
      init.body = JSON.stringify(body)
      ;(init.headers as Record<string, string>)['Content-Type'] = 'application/json'
    }
  }
  const res = await fetch(path, init)
  const text = await res.text()
  let data: unknown = undefined
  try {
    data = text ? JSON.parse(text) : undefined
  } catch {
    data = text
  }
  if (!res.ok) {
    const msg = (data && typeof data === 'object' && 'error' in data ? String((data as { error: unknown }).error) : res.statusText) || 'Permintaan gagal'
    if (res.status === 401) window.dispatchEvent(new CustomEvent('arc:unauthorized'))
    throw new ApiError(res.status, msg)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, body ?? {}),
  del: <T>(path: string) => request<T>('DELETE', path),
}

// useApi fetches `path` and refetches whenever the global refresh counter changes
// (bumped after any decision/mutation) or `path` changes.
export function useApi<T>(path: string | null) {
  const { refreshKey } = useUI()
  const [data, setData] = useState<T | null>(null)
  const [error, setError] = useState<string | null>(null)
  const seq = useRef(0)
  const load = useCallback(() => {
    if (!path) return
    const n = ++seq.current
    api
      .get<T>(path)
      .then(d => {
        if (n === seq.current) {
          setData(d)
          setError(null)
        }
      })
      .catch(e => n === seq.current && setError(e instanceof Error ? e.message : String(e)))
  }, [path])
  useEffect(() => {
    load()
  }, [load, refreshKey])
  return { data, error, reload: load, setData }
}
