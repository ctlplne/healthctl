import type { Fields } from './types'
let token = ''
export function updateToken(value: string) {
  token = value
}
export class RequestError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}

// SIGNED_OUT is dispatched on window when the server says the session ended,
// so the app can ask the user to sign in again without losing what is on screen.
export const SIGNED_OUT = 'healthctl:signed-out'

export type RequestOptions = {
  asyncModel?: boolean
  signal?: AbortSignal
  audioConsent?: boolean
  // timeoutMs bounds the whole request. Uploads get longer by default.
  timeoutMs?: number
  // retried marks the one retry after a CSRF token refresh.
  retried?: boolean
  // reauthed marks the one retry after the user entered the password again.
  reauthed?: boolean
}

// A sensitive action needs a password entered recently. The app registers a
// handler that asks for it; the request is then sent once more.
let reauthHandler: (() => Promise<boolean>) | null = null
export function setReauthHandler(handler: (() => Promise<boolean>) | null) {
  reauthHandler = handler
}

const defaultTimeout = (method: string, body?: Fields | FormData | Blob) =>
  body instanceof FormData || body instanceof Blob ? 300_000 : method === 'GET' ? 20_000 : 60_000

// api is the one way the app talks to its server: it times out, honors the
// caller's cancellation, reports an ended session, and refreshes the CSRF
// token once when the server asks for it.
export async function api<T>(
  path: string,
  method = 'GET',
  body?: Fields | FormData | Blob,
  options: RequestOptions = {},
): Promise<T> {
  const controller = new AbortController()
  let timedOut = false
  const timer = setTimeout(() => {
    timedOut = true
    controller.abort()
  }, options.timeoutMs ?? defaultTimeout(method, body))
  const cancel = () => controller.abort()
  if (options.signal?.aborted) controller.abort()
  options.signal?.addEventListener('abort', cancel, { once: true })
  let response: Response
  try {
    response = await fetch('/api' + path, {
      method,
      credentials: 'same-origin',
      cache: 'no-store',
      signal: controller.signal,
      headers: {
        Accept: 'application/json',
        ...(options.asyncModel ? { Prefer: 'respond-async' } : {}),
        ...(options.audioConsent ? { 'X-Audio-Consent': 'openai-api' } : {}),
        ...(body instanceof Blob ? { 'Content-Type': 'audio/wav' } : {}),
        ...(body && !(body instanceof FormData) && !(body instanceof Blob)
          ? { 'Content-Type': 'application/json' }
          : {}),
        ...(method !== 'GET' ? { 'X-CSRF-Token': token } : {}),
      },
      ...(body
        ? {
            body:
              body instanceof FormData || body instanceof Blob
                ? body
                : JSON.stringify(body),
          }
        : {}),
    })
  } catch (err) {
    if (timedOut) {
      throw new RequestError(0, 'The application took too long to answer. Your history is saved; retry in a moment.')
    }
    if (options.signal?.aborted) throw err
    throw new RequestError(
      0,
      'The application could not be reached. Reconnect, then retry.',
    )
  } finally {
    clearTimeout(timer)
    options.signal?.removeEventListener('abort', cancel)
  }
  if (response.status === 401 && path !== '/session' && path !== '/login') {
    window.dispatchEvent(new Event(SIGNED_OUT))
  }
  if (response.status === 403 && method !== 'GET' && !options.retried && (await refreshToken())) {
    return api<T>(path, method, body, { ...options, retried: true })
  }
  if (!response.ok) {
    const error = await response.json().catch(() => ({}))
    if (response.status === 403 && error.reauth && reauthHandler && !options.reauthed && (await reauthHandler())) {
      return api<T>(path, method, body, { ...options, reauthed: true })
    }
    throw new RequestError(
      response.status,
      typeof error.error === 'string'
        ? error.error
        : 'The request could not be completed.',
    )
  }
  const text = await response.text()
  return text ? (JSON.parse(text) as T) : (undefined as T)
}

async function refreshToken(): Promise<boolean> {
  try {
    const r = await fetch('/api/session', { credentials: 'same-origin', cache: 'no-store', headers: { Accept: 'application/json' } })
    const s = (await r.json()) as { csrf?: string; authenticated?: boolean }
    if (!s.authenticated || !s.csrf || s.csrf === token) return false
    token = s.csrf
    return true
  } catch {
    return false
  }
}

// latest wraps an async loader so only the newest call's result is used. An
// older response that arrives late is dropped instead of overwriting newer data.
export function latest<A extends unknown[], T>(load: (...args: A) => Promise<T>) {
  let sequence = 0
  return async (...args: A): Promise<{ value: T; current: boolean }> => {
    const mine = ++sequence
    const value = await load(...args)
    return { value, current: mine === sequence }
  }
}

// upload sends a form with progress and cancel. fetch can't report upload
// progress, so this one request uses XMLHttpRequest with the same headers.
export function upload<T>(
  path: string,
  data: FormData,
  { onProgress, signal, makeRequest = () => new XMLHttpRequest() }: { onProgress?: (fraction: number) => void; signal?: AbortSignal; makeRequest?: () => XMLHttpRequest } = {},
): Promise<T> {
  return new Promise((resolve, reject) => {
    const xhr = makeRequest()
    xhr.open('POST', '/api' + path)
    xhr.withCredentials = true
    xhr.timeout = defaultTimeout('POST', data)
    xhr.setRequestHeader('Accept', 'application/json')
    xhr.setRequestHeader('X-CSRF-Token', token)
    xhr.upload.onprogress = (ev) => {
      if (ev.lengthComputable && onProgress) onProgress(ev.loaded / ev.total)
    }
    xhr.onload = () => {
      let parsed: { error?: unknown } = {}
      try {
        parsed = xhr.responseText ? JSON.parse(xhr.responseText) : {}
      } catch {
        parsed = {}
      }
      if (xhr.status === 401) window.dispatchEvent(new Event(SIGNED_OUT))
      if (xhr.status >= 200 && xhr.status < 300) resolve(parsed as T)
      else reject(new RequestError(xhr.status, typeof parsed.error === 'string' ? parsed.error : 'The upload could not be completed.'))
    }
    xhr.onerror = () => reject(new RequestError(0, 'The application could not be reached. Reconnect, then retry.'))
    xhr.ontimeout = () => reject(new RequestError(0, 'The upload took too long. Your history is saved; retry in a moment.'))
    xhr.onabort = () => reject(new RequestError(0, 'Upload cancelled.'))
    signal?.addEventListener('abort', () => xhr.abort(), { once: true })
    xhr.send(data)
  })
}
