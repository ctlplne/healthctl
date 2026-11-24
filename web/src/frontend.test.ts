import { afterEach, describe, expect, it, vi } from 'vitest'
import { parseRoute, routePath } from './route'
import { pollDelay, whenVisible } from './poll'
import { readyNotice } from './turn'
import { api, latest, RequestError, SIGNED_OUT, updateToken, upload } from './api'

describe('routes', () => {
  it('round-trips every page and an encounter tab', () => {
    for (const page of ['journal', 'settings', 'intake', 'timeline', 'evidence'] as const) {
      expect(parseRoute(routePath({ page })).page).toBe(page)
    }
    expect(routePath({ page: 'record', encounterId: 'abc123', tab: 'history' })).toBe('/encounters/abc123/history')
    expect(parseRoute('/encounters/abc123/history')).toEqual({ page: 'record', encounterId: 'abc123', tab: 'history' })
    expect(parseRoute('/encounters/abc123')).toEqual({ page: 'record', encounterId: 'abc123', tab: undefined })
  })
  it('falls back to the journal for unknown or unsafe paths', () => {
    expect(parseRoute('/nowhere').page).toBe('journal')
    expect(parseRoute('/encounters/../../etc').page).toBe('journal')
    expect(parseRoute('/encounters/abc/not-a-tab').tab).toBeUndefined()
  })
})

describe('polling', () => {
  it('backs off and caps the delay', () => {
    expect(pollDelay(0)).toBe(1000)
    expect(pollDelay(1)).toBe(1500)
    expect(pollDelay(3)).toBeGreaterThan(pollDelay(2))
    expect(pollDelay(50)).toBe(10000)
  })
  it('pauses while the tab is hidden and resumes when it is shown', async () => {
    const listeners: (() => void)[] = []
    const doc = { hidden: true, addEventListener: (_: string, f: () => void) => listeners.push(f), removeEventListener: () => {} }
    let resumed = false
    const waiting = whenVisible(new AbortController().signal, doc as unknown as Document).then(() => (resumed = true))
    await Promise.resolve()
    expect(resumed).toBe(false)
    doc.hidden = false
    listeners.forEach((f) => f())
    await waiting
    expect(resumed).toBe(true)
  })
})

describe('a finished turn', () => {
  it('never moves the user and offers the way to the result', () => {
    expect(readyNotice({ state: 'interviewing', question: 'When did it start?' }, 'history')).toEqual({ message: 'Your next question is ready.', goTo: 'interview' })
    expect(readyNotice({ state: 'interviewing', question: 'When did it start?' }, 'interview')).toEqual({ message: 'Your next question is ready.' })
    expect(readyNotice({ state: 'history_review', question: '' }, 'report').goTo).toBe('history')
  })
})

describe('api', () => {
  afterEach(() => vi.unstubAllGlobals())
  it('times out instead of waiting forever', async () => {
    vi.stubGlobal('fetch', (_: string, init: RequestInit) => new Promise((_, reject) => init.signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')))))
    await expect(api('/slow', 'GET', undefined, { timeoutMs: 20 })).rejects.toThrow(/took too long/)
  })
  it('reports an ended session', async () => {
    vi.stubGlobal('fetch', async () => new Response('{"error":"Sign in again."}', { status: 401 }))
    const seen = vi.fn()
    window.addEventListener(SIGNED_OUT, seen)
    await expect(api('/encounters')).rejects.toBeInstanceOf(RequestError)
    window.removeEventListener(SIGNED_OUT, seen)
    expect(seen).toHaveBeenCalledOnce()
  })
  it('refreshes the CSRF token once and retries', async () => {
    updateToken('old')
    const sent: string[] = []
    vi.stubGlobal('fetch', async (url: string, init: RequestInit = {}) => {
      if (url === '/api/session') return new Response('{"authenticated":true,"csrf":"new"}')
      sent.push((init.headers as Record<string, string>)['X-CSRF-Token'])
      return sent.length === 1 ? new Response('{"error":"Refresh the page and try again."}', { status: 403 }) : new Response('{"ok":true}')
    })
    await expect(api('/encounters/x/actions', 'POST', { action: 'review' })).resolves.toEqual({ ok: true })
    expect(sent).toEqual(['old', 'new'])
  })
  it('drops a stale response', async () => {
    let release: (v: string) => void = () => {}
    const load = latest((v: string) => (v === 'slow' ? new Promise<string>((r) => (release = r)) : Promise.resolve(v)))
    const slow = load('slow')
    const fast = await load('fast')
    release('slow')
    expect(fast.current).toBe(true)
    expect((await slow).current).toBe(false)
  })
  it('reports upload progress and can cancel', async () => {
    const fake = {
      upload: {} as { onprogress?: (ev: ProgressEvent) => void },
      open: vi.fn(), setRequestHeader: vi.fn(), withCredentials: false, timeout: 0, status: 0, responseText: '',
      send() { this.upload.onprogress?.({ lengthComputable: true, loaded: 50, total: 100 } as ProgressEvent) },
      abort() { this.onabort?.() },
      onload: undefined as undefined | (() => void), onerror: undefined, ontimeout: undefined, onabort: undefined as undefined | (() => void),
    }
    const progress: number[] = []
    const controller = new AbortController()
    const sent = upload('/encounters/x/documents', new FormData(), { onProgress: (f) => progress.push(f), signal: controller.signal, makeRequest: () => fake as unknown as XMLHttpRequest })
    controller.abort()
    await expect(sent).rejects.toThrow('Upload cancelled.')
    expect(progress).toEqual([0.5])
  })
})
