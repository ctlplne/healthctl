// Polling waits longer the longer a request runs, and pauses while the tab
// is hidden. A visible tab resumes at once.
export function pollDelay(attempt: number, base = 1000, max = 10000): number {
  return Math.min(max, Math.round(base * Math.pow(1.5, Math.max(0, attempt))))
}

export function whenVisible(signal: AbortSignal, doc: Pick<Document, 'hidden' | 'addEventListener' | 'removeEventListener'> = document): Promise<void> {
  if (!doc.hidden) return Promise.resolve()
  return new Promise((resolve) => {
    const done = () => {
      if (doc.hidden && !signal.aborted) return
      doc.removeEventListener('visibilitychange', done)
      signal.removeEventListener('abort', done)
      resolve()
    }
    doc.addEventListener('visibilitychange', done)
    signal.addEventListener('abort', done)
  })
}

// schedule runs check after the next delay, once the tab is visible.
export function schedule(signal: AbortSignal, attempt: number, check: () => void, base?: number): () => void {
  let timer = 0
  timer = window.setTimeout(() => {
    void whenVisible(signal).then(() => {
      if (!signal.aborted) check()
    })
  }, pollDelay(attempt, base))
  return () => clearTimeout(timer)
}
