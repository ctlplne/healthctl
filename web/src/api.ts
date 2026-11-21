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
export async function api<T>(
  path: string,
  method = 'GET',
  body?: Fields | FormData,
): Promise<T> {
  let response: Response
  try {
    response = await fetch('/api' + path, {
      method,
      credentials: 'same-origin',
      cache: 'no-store',
      headers: {
        Accept: 'application/json',
        ...(body && !(body instanceof FormData)
          ? { 'Content-Type': 'application/json' }
          : {}),
        ...(method !== 'GET' ? { 'X-CSRF-Token': token } : {}),
      },
      ...(body
        ? { body: body instanceof FormData ? body : JSON.stringify(body) }
        : {}),
    })
  } catch {
    throw new RequestError(
      0,
      'The application could not be reached. Reconnect, then retry.',
    )
  }
  if (!response.ok) {
    const error = await response.json().catch(() => ({}))
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
