import { useEffect, useState } from 'react'
import { api } from './api'

export type EncounterUsageData = {
  calls: number
  failedCalls: number
  inputTokens: number
  outputTokens: number
  waitSeconds: number
  estimatedUSD?: number
  costComplete: boolean
}

// usageText describes one encounter's model use. A cost is shown only when
// every call reported a price; otherwise it says the cost is unknown.
export function usageText(u: EncounterUsageData): string {
  if (!u.calls) return 'No model calls yet.'
  const plural = u.calls === 1 ? '' : 's'
  const failed = u.failedCalls ? ' (' + u.failedCalls + ' failed)' : ''
  const calls = u.calls + ' model call' + plural + failed
  const tokens = (u.inputTokens + u.outputTokens).toLocaleString('en') + ' tokens'
  const wait = Math.round(u.waitSeconds) + ' s waiting'
  const cost =
    u.costComplete && u.estimatedUSD !== undefined
      ? 'about $' + u.estimatedUSD.toFixed(u.estimatedUSD < 0.1 ? 3 : 2) + ' (estimate)'
      : 'cost unknown'
  return [calls, tokens, wait, cost].join(' · ')
}

export function EncounterUsage({ id, revision }: { id: string; revision: number }) {
  const [usage, setUsage] = useState<EncounterUsageData>()
  useEffect(() => {
    const controller = new AbortController()
    api<EncounterUsageData>('/encounters/' + id + '/usage', 'GET', undefined, { signal: controller.signal })
      .then(setUsage)
      .catch(() => setUsage(undefined))
    return () => controller.abort()
  }, [id, revision])
  if (!usage) return null
  return (
    <>
      <dt>Model use</dt>
      <dd>{usageText(usage)}</dd>
    </>
  )
}
