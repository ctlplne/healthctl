import type { Warning } from './types'

export type WarningGroup = {
  level: 'urgent' | 'check'
  messages: string[]
  quotes: string[]
}

// groupWarnings shows each warning level once. Several urgent phrases in one
// answer used to stack identical banners; one banner now lists every reported
// wording, urgent first. Distinct messages, such as a crisis line, are kept.
export function groupWarnings(warnings: Warning[]): WarningGroup[] {
  const groups: WarningGroup[] = []
  for (const level of ['urgent', 'check'] as const) {
    const matching = warnings.filter((w) => w.active && (w.level === 'check' ? 'check' : 'urgent') === level)
    if (!matching.length) continue
    groups.push({
      level,
      messages: [...new Set(matching.map((w) => w.message))],
      quotes: [...new Set(matching.map((w) => w.quote))],
    })
  }
  return groups
}
