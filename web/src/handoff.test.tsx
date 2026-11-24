import { describe, expect, it } from 'vitest'
import { render } from '@testing-library/react'
import { CompactList, medicineFindings } from './handoff'
import type { Fact } from './types'

describe('CompactList', () => {
  it('says how many items a shortened list leaves out', () => {
    const { container } = render(<CompactList values={['a', 'b', 'c', 'd', 'e', 'f', 'g']} limit={5} />)
    expect(container.querySelectorAll('li')).toHaveLength(5)
    expect(container.textContent).toContain('2 more in the provenance appendix.')
  })
  it('shows every item when there is room', () => {
    const { container } = render(<CompactList values={['Conflict · Fever']} />)
    expect(container.querySelectorAll('li')).toHaveLength(1)
    expect(container.textContent).not.toContain('more')
  })
})

describe('medicines and allergies in the brief', () => {
  it('come from recorded findings, not words in a quote', () => {
    const fact = (id: string, concept: string, quote: string, category?: 'medication' | 'allergy') =>
      ({ id, concept, quote, category, answerId: 'a1', status: 'reported_present', subject: 'self' }) as Fact
    const listed = medicineFindings([
      fact('f1', 'Current medicine or supplement use', 'I take a blue tablet', 'medication'),
      fact('f2', 'Penicillin allergy', 'I get hives from penicillin', 'allergy'),
      fact('f3', 'Headache', 'The headache medicine ad on TV made me think of it'),
    ])
    expect(listed.map((f) => f.id)).toEqual(['f1', 'f2'])
  })
})
