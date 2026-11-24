import { describe, expect, it } from 'vitest'
import { groupWarnings } from './warnings'
import type { Warning } from './types'

const w = (quote: string, message = 'Seek local emergency care immediately.', level?: 'check', active = true): Warning => ({
  id: quote,
  message,
  quote,
  answerId: 'a1',
  active,
  createdAt: '2026-11-24T00:00:00Z',
  level,
})

describe('groupWarnings', () => {
  it('shows several urgent phrases in one banner, urgent before check', () => {
    const groups = groupWarnings([
      w('face drooped'),
      w('speech was slurred'),
      w('rash spreading', 'Get this checked promptly.', 'check'),
      w('face drooped'),
    ])
    expect(groups.map((g) => g.level)).toEqual(['urgent', 'check'])
    expect(groups[0].quotes).toEqual(['face drooped', 'speech was slurred'])
    expect(groups[0].messages).toEqual(['Seek local emergency care immediately.'])
  })

  it('keeps a distinct message such as a crisis line', () => {
    const [urgent] = groupWarnings([w('chest pain'), w('want to end my life', 'Call your crisis line now.')])
    expect(urgent.messages).toHaveLength(2)
  })

  it('leaves out warnings that were cleared', () => {
    expect(groupWarnings([w('face drooped', undefined, undefined, false)])).toEqual([])
  })
})
